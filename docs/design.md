# Design

[中文](zh/design.md)

This document describes the design of a *shared, evidence-gated skill*: one skill that a whole team's agents use, that grows with every session, and that only accepts facts backed by evidence from the session that produced them.

Examples use a fictional e-commerce team (`checkout`, `orders`, `wallet` services). The pattern is domain-independent: operations, customer support, data analysis or internal tooling all fit.

- [The idea: a self-evolving shared skill](#the-idea-self-evolving-shared-skill)
- [0. Background: from a local skill to a shared one](#0-background-from-a-local-skill-to-a-shared-one)
- [1. Goals and non-goals](#1-goals-and-non-goals)
- [2. Architecture](#2-architecture)
- [3. Knowledge model](#3-knowledge-model)
- [4. Session lifecycle](#4-session-lifecycle)
- [5. Retrieval](#5-retrieval)
- [6. Writing knowledge back](#6-writing-knowledge-back)
- [7. Question types](#7-question-types)
- [8. Repository auto-registration](#8-repository-auto-registration)
- [9. Bootstrap and zero-touch updates](#9-bootstrap-and-zero-touch-updates)
- [10. Safety](#10-safety)
- [11. Lessons learned](#lessons-learned)
- [12. Open questions](#12-open-questions)

---

<a id="the-idea-self-evolving-shared-skill"></a>
## The idea: a self-evolving shared skill

### What "self-evolving" means

Most agent memory systems grow — they accumulate more data over time. A self-evolving skill does something more: **it changes how agents behave, not just what they know.**

A growing knowledge base adds facts: "E4021 means wallet timeout." That helps the next person who sees E4021. A self-evolving skill also changes the *rules*: "Agents were closing sessions when blocked by permissions, losing the evidence chain. New rule: don't close on blockers, keep the session open." That changes how *every* agent handles *every* future problem, not just one error code.

The distinction matters because the most impactful improvements aren't new facts — they're new behaviors:

| Layer | What changes | Example | Who benefits |
|---|---|---|---|
| **Knowledge** (data) | Notes, cases, map entries | "E4021 = wallet timeout" | The next person who hits E4021 |
| **Strategy** (rules) | Question classification, response templates, evidence rules | "Agents must not close sessions when blocked" | Everyone, on every question |
| **Structure** (model) | How knowledge is organized, retrieved and scored | "Add stale penalty for notes not verified in 90 days" | The quality of every retrieval |

A self-growing system only changes the first layer. A self-evolving skill changes all three — and the skill gets better at *getting better*, because the rules that govern how knowledge enters are themselves subject to improvement.

### The three evolution loops

```
                ┌─────────────────────────────────────────┐
                │          Skill evolution                 │
                │                                         │
  ┌─────────────┼──────────────────┐                      │
  │ Knowledge   │                  │                      │
  │ growth      │  ┌───────────────┼──────────┐           │
  │             │  │ Strategy      │          │           │
  │             │  │ refinement    │          │           │
  │             ▼  │               ▼          │           │
  │                │                          │           │
  │  Ask ──▶ Retrieve ──▶ Investigate ──▶ Write back      │
  │  │                    (log steps)    (knowledge) ─────┘ Loop 1: facts
  │  │                         │              │
  │  │                         │         Reflect ─────────┘ Loop 2: pitfalls → rules
  │  │                         │              │
  │  │                         │     Team reviews ────────┘ Loop 3: rules → instructions
  │  │                         │
  │  └─────────────────────────┘
  │    Next question uses
  │    better knowledge,
  │    better rules, and
  │    better structure
  └───────────────────────────────────────────────────────┘
```

**Loop 1 — Knowledge growth** (automatic, every session): agents investigate, find facts, write them back. The next person who asks a similar question starts with better data. This is what most memory systems do.

**Loop 2 — Strategy refinement** (automatic, through reflection): agents reflect on what went wrong — redundant steps, wrong order, pitfalls. These reflections become experience notes with triggers. The next agent facing a similar situation avoids the same mistakes. Over time, the *approach* to problems improves, not just the facts available.

**Loop 3 — Structural evolution** (human-driven, through instructions): the team notices patterns — agents keep closing sessions when blocked, or agents write conclusions without evidence. They update the instructions (the rules every agent must follow). Because instructions are fetched live on every `skill_open`, the new behavior applies to every agent's very next question. No reinstall, no re-copy, no version bump (except for the thin bootstrap).

Most systems have Loop 1. Few have Loop 2. Fewer still have Loop 3 — the ability for the skill's own operating rules to evolve based on what the team observes.

### A real example of self-evolution

This isn't hypothetical. Here's how the skill evolved through actual use:

1. **An agent without log permissions** matched a problem to an old case, reported "confirmed", and announced "saved to knowledge base". Nothing was verified. Nothing was saved.
   - **Loop 2 response**: a reflection noted "agents can sound confident without evidence."
   - **Loop 3 response**: we added the **evidence gate** — an instruction that the server enforces. Now *every* agent on *every* question must have real observations before writing facts.

2. **Agents kept closing sessions** when they hit permission blocks. When users came back after fixing permissions, the session was closed and the evidence chain was broken.
   - **Loop 2 response**: reflections noted "session closed too early, lost context."
   - **Loop 3 response**: we added **session chaining** (`parent_session_id`) and updated instructions: "don't close on blockers." Now agents keep sessions open, and follow-up sessions inherit evidence from their parents.

3. **Knowledge drifted silently** — notes about log store names and database locations became wrong as infrastructure changed, but agents kept trusting them.
   - **Loop 3 response**: we added the **freshness model** — notes not confirmed in 90 days are flagged stale and ranked lower. The retrieval *strategy* changed, not just the data.

Each of these changed the skill's behavior for everyone, immediately, without anyone reinstalling anything. This is what makes it self-evolving: **the rules that govern how agents work are themselves part of the system, and they improve through use.**

### Where this idea comes from

The concept draws from several lines of research:

**Experiential learning for agents.** [ExpeL](https://arxiv.org/abs/2308.10144) (Zhao et al., AAAI 2024) showed that LLM agents can learn from experience without fine-tuning: the agent attempts tasks, reflects on successes and failures, extracts reusable insights, and applies them to future tasks. ExpeL's insight pool is the closest ancestor of our `experience` notes — lessons written by the agent, indexed for future retrieval, with the critical difference that ExpeL's insights are *per-agent* and *self-assessed*.

**Lifelong skill accumulation.** [Voyager](https://arxiv.org/abs/2305.16291) (Wang et al., 2023) demonstrated a different form of self-evolving knowledge: a skill library that grows as the agent explores, where each skill is an executable program checked against environment feedback and self-verification. Voyager's key principle — *the environment verifies, not the agent's say-so* — maps directly to our evidence gate: the server, not the agent, decides what is allowed in.

**Evolving contexts.** [ACE](https://arxiv.org/abs/2510.04618) (Agentic Context Engineering; Zhang et al., ICLR 2026) treats an agent's context as an evolving *playbook*: a generator works on tasks, a reflector extracts lessons from execution feedback, and a curator merges them into the playbook as small incremental updates instead of rewriting it. It is the closest prior work to Loop 2, and to what `skill_open` delivers: every question starts from an accumulated playbook rather than from scratch.

**Reflective memory.** [Reflexion](https://arxiv.org/abs/2303.11366) (Shinn et al., 2023) showed that agents improve across attempts by writing verbal self-reflections into memory and reading them on the next try. The `reflection` field in `skill_update` plays the same role. The difference is that reflections here are kept apart from facts: they shape how agents work, but they are never served to other people as verified knowledge.

**Memory as an architecture.** [CoALA](https://arxiv.org/abs/2309.02427) (Sumers et al., 2023) gives a vocabulary we borrow: short-term working memory plus long-term episodic, semantic and procedural memory. In those terms, `case` notes are episodic memory; `experience` and `document` notes and the map are semantic memory; `instruction` notes are procedural memory — the layer that Loop 3 changes.

### What's different here

These works share a common aspiration — agents that learn from experience — but each covers only part of the picture:

| Approach | Loop 1 (facts) | Loop 2 (strategy) | Loop 3 (rules) | Shared by a team |
|---|---|---|---|---|
| **ExpeL** | ✅ experiences and insights | ✅ insights from successes and failures | ❌ insights act like rules, but belong to one agent | ❌ |
| **Voyager** | ✅ library of verified skills | ⚠️ self-verification within a task | ❌ | ❌ |
| **Reflexion** | ❌ memory is per task | ✅ verbal self-reflection | ❌ | ❌ |
| **ACE** | ✅ playbook entries | ✅ reflector + curator | ⚠️ the playbook is an evolving rule set, but there is no separate, owner-controlled rule layer | ❌ |
| **Mem0 / Graphiti / Cognee** | ✅ long-term memory (Mem0 adds, updates and deletes memories) | ❌ | ❌ | ⚠️ scoped per user or agent |
| **This project** | ✅ evidence-gated | ✅ reflections → experiences | ✅ instructions edited by owners, applied live | ✅ |

The closest is ACE. The differences are about who shares the context and who decides what enters it:

- **Many users, one skill.** ACE improves one agent system. Here one playbook serves a whole team, and each person's agent runs tools with their own credentials on their own machine.
- **The server checks every write.** In ACE, lessons from the reflector go into the playbook. Here a fact can only enter if it cites a step from the same session that observed something real. The check is deterministic and runs on the server, not in a model.
- **Rules are a separate, owned layer.** Agents write notes; only owners change instructions. Agent-written content cannot rewrite the rules that every agent follows.

The combination: **all three loops, shared across a team, with the server — not the agent — enforcing quality.**

Three design principles make this work:

1. **One shared skill.** Not per-agent or per-user, but one skill that every teammate's agent reads from and writes to.
2. **Evidence-gated writes.** The server enforces that only facts backed by in-session observations can enter the knowledge base. Guesses, recollections and "matches a past case" go into reflection, not into shared knowledge.
3. **Local execution.** The server holds knowledge, never credentials. Every tool call runs on the user's own machine.

### Why evidence is the foundation

The evidence gate is not a nice-to-have; it is the mechanism that makes self-evolution sustainable. Without it, every loop degrades:

- **Loop 1 without evidence**: knowledge fills with hallucinations. The next agent trusts them, reinforcing errors.
- **Loop 2 without evidence**: reflections are based on what the agent *said* it did, not what it *actually observed*. Pitfalls and rules drift from reality.
- **Loop 3 without evidence**: the team can't tell which instruction changes actually improved outcomes, because the data they're looking at was never verified.

The rule is simple: **only what you observed in this session can become shared knowledge.** Everything else — reflections, hunches, connections to past cases — is valuable, but it goes into a different bucket (reflection) that is stored for the agent's own learning, not served as facts to the next person.

### Knowledge decay and freshness

Self-evolving knowledge has a second problem beyond quality: **decay**. The infrastructure changes, log stores get renamed, databases move, APIs evolve. A note that was correct six months ago may be silently wrong today.

The freshness model addresses this: every note carries a "last verified" date, updated only when someone confirms it is still useful *in a session with real evidence*. Notes not verified for 90 days are flagged as stale, ranked lower in retrieval, and presented to the agent as "leads to verify, not facts to rely on." This creates a natural lifecycle: valuable knowledge gets regularly refreshed through use; knowledge that nobody needs quietly fades from prominence without anyone having to manually deprecate it.

### Session chaining: continuity across conversations

Real investigations are rarely one question and done. A user asks, gets blocked by permissions, fixes them, continues; then asks a follow-up; then digs deeper. Each interaction may open a new session, but the context should not be lost.

Sessions form a chain through `parent_session_id`. When a follow-up session opens, it links back to the previous one. The evidence gate looks at both the current session and its direct parent — so a child session can write knowledge based on evidence the parent gathered. Each session's reflection compresses the full chain's context, so the depth is always effectively one: the parent's reflection already contains everything from earlier.

---

## 0. Background: from a local skill to a shared one

The first version was [agent-skill-patterns](https://github.com/tingfengyinyue/agent-skill-patterns): a local, file-based knowledge base that one person's agent reads and writes.

**How it worked**

- **SPAR loop.** Every task goes Sense → Plan → Act → Reflect. *Plan* recalls past lessons; *Reflect* answers four fixed questions (detours? better order? pitfalls? a rule for next time?) and writes the answer down.
- **Operational memories (OMs).** Each lesson is a Markdown file with a trigger (task type, services, symptoms), a `better_action`, a confidence level (low → medium → high), a hit count and a lifecycle (active → superseded / deprecated / graduated to a runbook).
- **Funnel loading.** A small JSON index is filtered first; only matching entries are read, and only then the relevant section of a service document. Typical loads stayed in the low hundreds of tokens.
- **Two-way links and a "temporal bridge".** Every relation is stored at both ends. An OM points back to the incident that produced it and forward, through its trigger, to the future tasks it should apply to.

It made one person — and, once copied, a few teams — noticeably faster. Sharing it exposed its limits:

| Problem in the local version | What it caused | How the shared version addresses it |
|---|---|---|
| One copy per person | What one agent learns never reaches anyone else; copies drift apart | One server-side knowledge base, read and written by every agent over MCP |
| The agent writes and rates its own lessons | Wrong lessons enter freely; they are found later and corrected by hand (superseded banners, manual edits) | Writes are gated by evidence from the session; unverified conclusions are downgraded (§6) |
| Recall through a keyword index | Misses when the wording differs; the index itself grows with the notes | Tags + vector similarity + full text, ranked together (§5) |
| Links, hit counts and lifecycle are conventions | Someone has to keep them consistent; scripts rebuild relations after the fact | The map is stored as a graph and queried in both directions; trust is aggregated from feedback automatically |
| No redaction step | Credentials and internal identifiers end up in notes, so notes can't be shared as-is | Redaction and injection checks before anything is stored (§10) |
| No record of how notes were used | No way to tell which lessons actually helped | Every session logs recalls, steps, feedback and changes |
| Updating means editing every copy | New rules reach people only when they re-copy the files | A thin local bootstrap; instructions, notes and map are fetched live (§9) |

**What carried over**

- *Reflect* is still mandatory, and its output still becomes `experience` notes with triggers (tags) and a brief "do this next time".
- *Cheap first, deep on demand*: `skill_open` returns summaries; full notes are read only when needed.
- *The temporal bridge*: an experience still points back to where it came from — now the exact session steps cited as evidence — and forward through its tags.
- *Lifecycle*: superseding, conflicts, and a last-verified date with staleness (§5) replace hand-maintained confidence and hit counts.

---

## 1. Goals and non-goals

**Goals**

- One body of knowledge for the whole team, not one per person.
- Knowledge improves as a side effect of normal use — nobody has to "go update the docs".
- Users keep their own credentials and run their own tools. The server never needs access to production systems.
- Shared knowledge stays trustworthy: a confident-sounding agent cannot pollute it.
- Minimal user interaction: the person asking is never asked "should I save this?". Where review is needed, it goes to the people who own the knowledge, not to whoever happens to be investigating.

**Non-goals**

- The server does not call a general-purpose LLM. All generation (understanding the question, planning, writing conclusions and reflections) is done by the user's agent. The server only does deterministic work plus embeddings.
- No versioning. Every change is recorded and can be reverted instead. (There is no approval step today either; a risk-tiered review queue is planned — see §6.)
- No code distribution. The agent composes its own tool calls from the instructions and notes; the skill does not ship scripts that run on users' machines.

## 2. Architecture

```
 User's machine                                   Skill server
┌──────────────────────────────┐   MCP / HTTP   ┌───────────────────────────────────┐
│ Agent (e.g. a coding agent)  │ ─────────────▶ │ Session service                   │
│  + bootstrap skill (thin)    │                │  - parse question, recognize      │
│  + user's own credentials    │ ◀───────────── │    entities                       │
│  + local tools / CLIs        │                │  - retrieve notes + map           │
│                              │                │  - log steps, close sessions      │
│ Runs every query locally.    │                │  - validate & apply updates       │
└──────────────────────────────┘                │  - redaction, injection checks    │
                                                ├───────────────────────────────────┤
                                                │ PostgreSQL + pgvector             │
                                                │  map: nodes, edges                │
                                                │  notes: items (+ embeddings)      │
                                                │  sessions (steps, changes)        │
                                                └───────────────────────────────────┘
```

The same session service is exposed both as MCP tools and as a REST API. MCP is stateless: every call carries a `session_id`, so the server can run multiple instances without sticky sessions.

## 3. Knowledge model

Four tables and one materialized view.

### Map

| Table | Purpose |
|---|---|
| `graph_nodes` | Entities (`service:checkout`) and resources (`resource:prod:logstore:checkout-api`, `resource:prod:db:orders`). Each has a stable `node_key`, a `domain`, `aliases` used for recognition, and free-form `attributes` (index fields, region, repository, etc.). |
| `graph_edges` | Relations: `calls`, `has_resource`, `contains` (a database contains tables, a parent service contains sub-services). Edge attributes carry details such as the filter needed on a shared log store. |

The map answers *"where do I look?"*. Every map change requires evidence (see §6).

### Notes

| Type | Example |
|---|---|
| `experience` | "Checkout timeouts: check the wallet service's connection pool before the payment provider." Has a short *brief* and a fuller body. |
| `document` | A section of a guide, split by heading. |
| `case` | A past session saved as a case: abstract, conclusion, services and resources involved. |
| `instruction` | Rules the agent must follow, loaded by condition (`always`, or only for a given domain). Admin-only. |

Each note has tags (`services`, `resources`, `task_types`, `symptoms`, `error_codes`, `keywords`) and, except for instructions, an embedding.

### Sessions

`sessions` stores the question, what was recalled, every step the agent logged, the conclusion, feedback on notes, the reflection, and a `changes` log of every write with before/after snapshots.

### Trust

A materialized view aggregates, per note, how many distinct users confirmed it and how many rated it helpful or harmful. Retrieval multiplies scores by this trust factor. Reverted sessions are excluded.

## 4. Session lifecycle

| Tool | When | Effect |
|---|---|---|
| `skill_open` | A question arrives | Creates a session; returns instructions, notes, map, bootstrap version |
| `skill_search` / `skill_read` | The agent needs more | Returns more notes / a full note |
| `skill_log_step` | After every action | Records the step: tool, resource, result class, short summary, IDs found |
| `skill_close` | Answer is ready | Stores the conclusion; warns about IDs that never appeared in any step |
| `skill_get_session` | Reflection | Returns the compressed trace, usually read by a background sub-agent |
| `skill_update` | After closing | Writes feedback, reflection and evidence-backed knowledge |

**Result classes** for a step are the backbone of the evidence gate:

| Class | Meaning | Counts as evidence |
|---|---|---|
| `ok` | Returned data | yes |
| `no_data` | Ran fine, nothing matched (and a sanity check shows the source has data) | yes |
| `wrong_mapping` | The resource on the map is wrong or missing | yes |
| `no_permission` | Not allowed | no |
| `env_mismatch` | Wrong account / environment | no |
| `not_available`, `error` | Couldn't run | no |

## 5. Retrieval

1. **Parse** the question with rules: recognize entities by name and alias, environment, time window, identifiers (trace IDs, order IDs), error codes.
2. **Recall** notes through three routes in parallel: tag match, vector similarity, full-text.
3. **Score**: `score = (0.45 · tag + 0.35 · vector + 0.20 · text) × trust × penalty`, where `penalty` down-weights notes from a domain the question isn't about, and notes that are stale (see below).
4. **Map**: expand from recognized entities to their resources (for the requested environment) and immediate neighbors.
5. **Instructions** are selected by condition, not ranked.

Only summaries are returned at first; the agent reads full notes on demand. This keeps the first response small and fast.

### Freshness

Knowledge about running systems decays quietly: a log store gets renamed, a database moves, an API changes. A note confirmed last week and one nobody has touched in a year should not look the same.

- Every note has a **last verified** date: when it was last confirmed in a session with real evidence. Imported notes start from the date recorded in their source; otherwise the last update time is used.
- Notes not verified for **90 days** are returned with `stale: true` and a score penalty (×0.8). Instructions tell the agent to treat them as leads and check before relying on them.
- When a note is rated **helpful** in a session that has evidence steps, its last-verified date is refreshed. Ratings in sessions without evidence don't count.
- Refreshing only touches the verification date, not `updated_at`, so it never conflicts with someone editing the note.

This is deliberately lighter than full bi-temporal modeling (see [open questions](#12-open-questions)): it answers "can I still trust this?", which is the question agents actually need answered.

## 6. Writing knowledge back

All writes go through one call, `skill_update`, in a single transaction with a savepoint per operation (one bad operation doesn't block the others). Repeating the same `update_id` replays the first result.

### The evidence gate

The rule: **a fact may enter shared knowledge only if this session observed something that supports it.**

| Operation | Rule |
|---|---|
| Any session with **zero** evidence steps | Only `feedback` and `reflection` are accepted. Everything else returns `needs_evidence`. |
| `add_item` (new note, or merge into an existing one) | Must cite evidence steps. |
| `save_case` | Must cite evidence steps. |
| `update_item` | With evidence: overwrite (old version kept in `changes`). Without evidence: create a new note marked as **conflicting** with the old one; both are shown, with their trust, to future agents. |
| Map changes (`upsert_node`, `update_node`, `add_edge`, `invalidate_*`) | Must cite evidence steps. |
| Instructions | Admin only. |

A cited step must exist in this session, be an action step, and have an evidence result class. The server checks this; the agent's word is not enough.

### Other write rules

- **Similar notes**: when adding a note whose title matches or whose embedding is ≥ 0.9 similar to an existing one, the server returns `needs_decision` with the candidates. The agent resubmits with `merge` (append a section — never delete) or `new`.
- **Optimistic locking**: updates carry the `updated_at` the agent read; a mismatch returns `stale`.
- **Conclusion status**: if a session has no evidence steps, a `confirmed` conclusion is downgraded to `likely` (and `answered` to `blocked`) at close time, with a warning.
- **Revert by session**: an admin can undo all writes of a session, in reverse order, from the `changes` log.
- **Automatic today**: once evidence is present, writes happen without asking the user, and an admin can revert a session afterwards. This keeps the knowledge growing while the team is small; it is not meant to be the final word on review.

<a id="planned-risk-tiered-review-queue"></a>
### Planned: risk-tiered review queue

The evidence gate stops writes that have *no* basis. It cannot tell whether an evidence-backed write is *right*: an agent may read real logs and still draw the wrong conclusion. As more people write, and as some notes become things the whole team relies on, a second pair of eyes becomes worth its cost — but only where a mistake is expensive. The plan is to route writes by risk instead of reviewing everything:

| Tier | Examples | Handling |
|---|---|---|
| **Auto** | New experiences and cases with evidence; feedback; reflection; refreshing the last-verified date; registering a repository that isn't on the map yet | Applied immediately, as today |
| **Review** | Overwriting a note with high trust (many confirmations); invalidating a note; map changes that remove or redirect (`invalidate_node`, `invalidate_edge`, changing a resource's identifiers); writes that conflict with existing knowledge; writes from contributors with a high revert rate | Stored as a **pending change**; not recalled for other users until approved |
| **Admin only** | Instructions (the rules every agent follows) | Changed by administrators, as today |

How review would work:

- **Who reviews**: owners per domain (e.g. the team that owns `checkout`), never the person who asked the question.
- **What they see**: the before/after diff, the cited evidence steps with their summaries, the session's question and conclusion, and the note's trust history. Approve, reject, or edit then approve — from a web page or a chat card.
- **While pending**: the author's own session sees the change marked as pending; everyone else keeps seeing the current version. Conflicting pending changes are reviewed together.
- **Timeouts**: pending changes that nobody reviews expire after a set period instead of auto-approving, so the queue cannot silently turn into a backdoor.
- **Tuning**: approval and revert rates per tier and per contributor feed back into the thresholds. A tier that is approved almost every time can move to *Auto*; one that is often reverted can move to *Review*.
- **Audit**: reviews are recorded in the same `changes` log, so revert-by-session keeps working.

## 7. Question types

Not every question is an investigation. Instructions tell the agent to classify first:

| Type | Example | Flow | Output |
|---|---|---|---|
| Troubleshooting | "Checkout fails for user 123 with E4021" | Full: plan → act → log → conclude → reflect → write back | Report: conclusion, root cause, evidence chain, fix |
| Data lookup | "What's the balance of wallet 123?" | Short: 1–2 queries → answer → light reflection | Table of values with their source |
| How-to | "How do I check a wallet balance?" | Read notes only, run nothing | Steps with example commands |

Because the classification and templates live in an instruction note, they can be changed without redeploying anything.

## 8. Repository auto-registration

Teams have many repositories, and the map rarely knows all of them. When the agent opens a session it sends the current directory's git remote. The server normalizes it to `group/name` (dropping hosts, users and tokens) and looks it up:

- **Known**: the response says which entity this repository belongs to, so the agent starts there.
- **Unknown**: the response asks the agent to inspect the repository (README, package manifest, deployment files), decide its name, domain and kind, and submit `register_repo`.

`register_repo` uses the reported remote as its evidence, with extra guards: the name must not be generic (`web`, `api`, …) or already taken; attaching a repository to an existing entity requires the names to match; it fills blanks only and never overwrites an existing repository.

## 9. Bootstrap and zero-touch updates

The only thing installed locally is a small bootstrap skill: *when* to call the server and *how* to run a session. Everything else — instructions, notes, map — is fetched live on every question.

This is one of the biggest practical wins of a shared skill: **updates are invisible to users**. Changing how questions are classified, tightening the evidence rules or adding staleness guidance were all done by editing instruction notes on the server; every teammate's next question used the new rules, and nobody re-installed or upgraded anything. Compare the local version, where an improvement only reached people who remembered to re-copy the files.

`skill_open` returns the current bootstrap version. If the local copy is older, the agent downloads the new one and replaces itself.

## 10. Safety

- **No credentials on the server.** Users configure their own read-only credentials locally.
- **Read-only by default.** Instructions forbid write actions on user systems; the agent proposes changes and the user runs them.
- **Redaction** before storage: credentials and personal data are stripped from steps and conclusions. Shared notes additionally drop business identifiers.
- **Injection checks**: text that looks like instructions to the agent ("ignore previous instructions…") is rejected from shared knowledge. Notes are always returned as *data*, separate from instructions.
- **No request/response bodies in server logs** for skill endpoints.

<a id="lessons-learned"></a>
## 11. Lessons learned

**Agents report what they intended, not what happened.**
An agent without permission to read the relevant logs matched the problem to an old case, marked the conclusion `confirmed`, and ended with "the new findings have been saved to the knowledge base". It had verified nothing, and it hadn't saved anything either — the closing sentence came from a fixed output template. Two fixes: the server now enforces the evidence gate and downgrades unverified conclusions, and templates must report what the update call actually returned.

**"Matches a past case" is not evidence.**
Past cases are great leads and terrible proof. If they're allowed to justify writes, a single wrong case reinforces itself every time it is recalled.

**Generic aliases poison recognition.**
Aliases like `api` or `web` matched unrelated questions. Names and aliases must be specific; generic words are rejected.

**Keep the metadata when importing.**
The original per-person notes recorded when a fact became true, when it was observed and which note superseded which. A first import kept the text and dropped most of that, and even treated "partially superseded" as "invalid", silently hiding a still-useful note. Time and provenance fields are part of the knowledge, not decoration.

**Ask the user once, or not at all.**
Early versions asked the user to confirm every update. Nobody enjoys that, and most people just said yes. Replacing the human confirmation with an evidence check gave better data and a better experience. Confirming with the person who happens to be investigating is not the same as review, though: for changes where a mistake is expensive, the planned review queue sends them to the people who own that knowledge.

**Keep scripts out of the skill.**
Shipping helper scripts to every machine looked attractive, but the rules they encoded were already in the notes, and distributing and verifying code added a lot of weight. Agents compose tool calls well from good notes.

## 12. Open questions

- **Bi-temporal history**: should overwritten facts keep a validity window (when it became true / stopped being true) alongside the record time, as in temporal knowledge graphs, so you can ask what was believed at a given time? Freshness (§5) covers the everyday need; full history may be worth it once audits ("what did we know when we decided this?") become common.
- **Authentication**: personal tokens per user, so trust and feedback are attributed correctly.
- **Retention**: how long to keep raw questions and steps, and what to keep after that.
- **Evaluation**: measuring whether the skill actually gets better — repeat-question time, recall hit rate, share of writes rejected by the gate.
- **Review thresholds**: what counts as "high trust", which map changes need review, and how long pending changes may wait before they expire.
