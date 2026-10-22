# tribal-knowledge

**Turn your team's tribal knowledge into a living skill that every agent shares — and that only learns from evidence.**

[中文说明](README.zh-CN.md)

> Status: reference implementation (Go) + design docs. Run it with `docker compose up && make run`.

---

## The problem

Every team has *tribal knowledge*: the things that live only in people's heads.

- "The search index on that log store only covers the `message` field — filter on that, not `level`."
- "Orders in that region are in a different database."
- "Error E4021 means the wallet service, not the payment provider."

Coding agents are now good enough to investigate problems on their own, but each agent starts from zero. The lesson one teammate's agent learned yesterday is gone today, and the next teammate pays for it again.

The usual fixes don't hold up:

| Approach | What goes wrong |
|---|---|
| A static skill file / runbook | Nobody updates it; it drifts from reality |
| Per-user agent memory | Knowledge stays siloed with one person |
| "Remember everything" shared memory | Fills up with guesses, hallucinations and stale facts — and the next agent trusts them |

## Where this came from

This started as a personal setup: [agent-skill-patterns](https://github.com/tingfengyinyue/agent-skill-patterns), a local, file-based knowledge base for my own agent. Lessons were written as Markdown "operational memories" after every task (a Sense → Plan → Act → Reflect loop), loaded through a cheap index, and linked to incidents and services in both directions. It worked well enough to be shared with other teams.

Sharing it is where it broke down:

- **Every copy drifts.** Each person runs their own copy; what one agent learns never reaches anyone else's.
- **Nothing checks what gets written.** The agent writes its own lessons and rates its own confidence. Wrong lessons were only caught later, by hand.
- **Keyword indexes don't scale.** Recall depends on exact tags; as notes pile up, relevant ones get missed.
- **Maintenance falls on people.** Links, hit counts and lifecycle fields are conventions that someone has to keep consistent.
- **Secrets creep in.** With no redaction step, local notes ended up holding credentials and internal details, so they couldn't be shared as-is.
- **Updates don't reach people.** Improving a rule means everyone re-copying files, so most copies quietly stay on old versions.

This project keeps what worked (the reflection loop, lessons with triggers, cheap-first loading, lifecycle) and moves the knowledge to a server that every agent shares, with the server — not the agent — deciding what is allowed in. Details: [docs/design.md](docs/design.md#0-background-from-a-local-skill-to-a-shared-one).

## The idea

**One shared skill for the whole team, hosted on a server, that grows every time someone uses it — but only writes down what was actually verified.**

Four principles:

1. **One shared skill.** Knowledge lives on a server. Every teammate's agent reads from and writes to the same skill through [MCP](https://modelcontextprotocol.io).
2. **Execute locally.** Tools and credentials stay on each user's machine. The server holds knowledge, never secrets, and never runs the user's tools.
3. **Evidence-gated writes.** A fact can only be written if the current session actually ran an operation that produced a result supporting it. Guesses, recollections and "it matches a past case" go into reflection, not into the knowledge base.
4. **Write back by default, review what's risky.** The person asking is never interrupted. Today, every evidence-backed write is applied immediately and can be reverted per session. Planned: a **risk-tiered review queue** — low-risk additions still go straight in, while risky changes (overwriting well-trusted notes, invalidating knowledge, changing the map) wait for a domain owner's approval. See [design §6](docs/design.md#planned-risk-tiered-review-queue).

## How it works

```mermaid
flowchart LR
  subgraph Local["User's machine (agent + own credentials)"]
    A["1. Ask a question"]
    C["3. Run tools locally"]
    D["4. Answer / report"]
  end
  subgraph Server["Skill server (shared knowledge)"]
    B["2. Return rules, notes, map"]
    L["Log each step"]
    E["5. Write back only evidence-backed facts"]
  end
  A --> B --> C --> D --> E
  C -. step summaries .-> L
  E -. recalled next time .-> B
```

1. The user asks their agent a question. A thin local *bootstrap* skill tells the agent to call `skill_open`.
2. The server parses the question, recognizes which entities it involves, and returns: instructions to follow, the most relevant notes, and a **map** of the entities and resources involved.
3. The agent runs tools **locally** with the user's own credentials, and logs a short summary of every step back to the server (`skill_log_step`).
4. The agent answers, then closes the session (`skill_close`).
5. The agent submits what it learned (`skill_update`). The server accepts a fact only if it cites steps from this session that returned real results.

The next teammate who asks a similar question gets those facts back in step 2.

## Knowledge model: map + notes

- **Map**: entities and resources, and the relations between them (`calls`, `has_resource`, `contains`). Example: `service:checkout` → `has_resource` → `resource:prod:logstore:checkout-api`. The map tells the agent *where to look*.
- **Notes**: free text with tags — experiences ("next time check X first"), document sections, past cases, and instructions. Notes tell the agent *what we've learned*.

Notes are retrieved by a mix of tag matching, vector similarity and full-text search. The map is expanded from the entities recognized in the question.

## Evidence gate: keeping the shared brain clean

A shared knowledge base is only as good as its worst entry. Agents are very good at sounding confident, so the server enforces the rules instead of trusting the agent:

- Every write must cite steps from **this session** whose result was a real observation (`ok`, `no_data`, `wrong_mapping`).
- A session with **no** such step can only submit feedback and reflection. Everything else is rejected with `needs_evidence`.
- Changing an existing note **with** evidence overwrites it (the old version is kept and can be reverted). Changing it **without** evidence creates a new note marked as conflicting, so both sides stay visible.
- A conclusion marked `confirmed` in a session with no real observations is automatically downgraded to `likely`.

We learned this the hard way: an agent without permission to read the logs "solved" a problem by matching it to an old case, reported it as confirmed, and announced that "the new findings have been saved". Nothing had been verified. See [docs/design.md](docs/design.md#lessons-learned).

## More in the design

- **Question types**: troubleshooting, data lookup and how-to questions follow different flows and output templates.
- **Auto-registering repositories**: the agent reports the current directory's git remote; if the repository isn't on the map yet, it analyzes it and registers it.
- **Review queue (planned)**: risky changes wait for a domain owner, with the diff and the cited evidence shown side by side; everything else stays automatic.
- **Zero-touch updates**: instructions, notes and the map are fetched live on every question, so a rule fixed on the server applies to everyone's very next question — nobody re-installs anything. The only local file is a thin bootstrap that refreshes itself when its version changes.
- **Freshness**: every note carries the date it was last confirmed. Notes not confirmed for 90 days are flagged `stale`, ranked lower, and treated as leads to verify. A note marked helpful in a session with real evidence gets its date refreshed automatically.
- **Revert by session**: every change records before/after, so a whole session's writes can be undone.
- **Redaction and injection checks** before anything enters shared knowledge.

Full design: [docs/design.md](docs/design.md) · Tool spec: [docs/spec.md](docs/spec.md) · 中文: [设计](docs/zh/design.md) / [接口](docs/zh/spec.md)

## Quick start

Requires Go 1.23+ and Docker.

```bash
# 1. Start PostgreSQL with pgvector (port 5433; the schema is created automatically)
docker compose up -d

# 2. Optional: enable vector search (OpenAI-compatible or DashScope).
#    Without it, notes are found by tags and full text.
export TK_EMBEDDING_DRIVER=openai          # or "dashscope"
export TK_EMBEDDING_API_KEY=sk-...

# 3. Run the server
make run
# → listening on :8080, MCP at /mcp, REST at /api/v1/skill/*

# 4. Connect your agent
# In Claude Code:  claude mcp add tribal-knowledge --transport http http://localhost:8080/mcp
```

> **Security**: the reference server has no user authentication yet — run it on a trusted network. Admin endpoints (revert a session) require `TK_ADMIN_TOKEN` and are disabled when it isn't set.

### Project structure

```
cmd/server/main.go           Entry point
internal/
  model/                     Core types: nodes, edges, items, sessions, steps
  store/                     PostgreSQL + pgvector persistence
  session/                   Session lifecycle, evidence gate, conclusion downgrade, repo registration
  retrieval/                 Three-way search (tag + vector + full-text), stale penalty
  mcpserver/                 MCP Streamable HTTP — 8 tools
  bootstrap/                 Embedded bootstrap skill file, version self-update
  embedding/                 Pluggable embedders: OpenAI-compatible, DashScope
  web/                       REST API handlers, MCP HTTP wrapper
  config/                    Environment-based configuration
migrations/                  PostgreSQL schema
docker-compose.yml           One-command PG + pgvector
```

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `TK_PORT` | `8080` | Server port |
| `TK_DATABASE_URL` | `postgres://tk:tk_dev@localhost:5433/tribal_knowledge?sslmode=disable` | PostgreSQL DSN |
| `TK_EMBEDDING_DRIVER` | empty | `openai`, `dashscope`, or empty (no vector search) |
| `TK_EMBEDDING_API_KEY` | — | API key for the embedding service |
| `TK_EMBEDDING_MODEL` | `text-embedding-3-small` / `text-embedding-v3` | Embedding model name |
| `TK_EMBEDDING_URL` | `https://api.openai.com/v1` | Base URL for any OpenAI-compatible embedding API |
| `TK_ADMIN_TOKEN` | — | Bearer token for admin endpoints; unset = disabled |

## Related work

| Project | Similar | Different |
|---|---|---|
| [HolmesGPT](https://github.com/HolmesGPT/holmesgpt) | Agent investigates incidents using runbooks | Runbooks are static; no shared knowledge that grows |
| [Graphiti](https://github.com/getzep/graphiti) | Temporal knowledge graph with provenance, MCP server | General memory; no evidence gate on writes |
| [Cognee](https://github.com/topoteretes/cognee) | Graph + vector memory, distills sessions into long-term memory | General memory layer; no local-execution / evidence model |
| [Mem0](https://github.com/mem0ai/mem0) | Long-term memory for agents; adds, updates and deletes memories | Scoped per user or agent; an LLM decides updates, with no evidence requirement |
| [ACE](https://arxiv.org/abs/2510.04618) | Context as an evolving playbook (generate → reflect → curate) | One agent system; lessons enter without a per-write evidence check; no separate rule layer |
| [ExpeL](https://github.com/LeapLabTHU/ExpeL), [Reflexion](https://arxiv.org/abs/2303.11366) | Insights / reflections that improve later attempts | Single agent, not shared by a team |

What seems uncommon here is the combination: **one skill shared by a team, tools executed locally with each user's own credentials, and writes gated by in-session evidence.**

## License

MIT
