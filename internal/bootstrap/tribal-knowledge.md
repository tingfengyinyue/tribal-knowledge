# Tribal Knowledge — Shared Ops Skill (bootstrap v1)

You are connected to a shared knowledge skill via MCP. This file tells you how to use it.

## Before you start

Run `git remote get-url origin` in the current directory. Pass the result as `client.git_remote` in `skill_open`.

## Workflow

1. **skill_open** — pass the user's question. You get back:
   - `instruction`: rules you MUST follow.
   - `items`: relevant notes from the shared knowledge base. Items with `stale: true` haven't been confirmed in 90 days — verify before relying on them.
   - `graph`: the map of entities and resources involved.
   - `repo_context`: whether the current repo is registered.

2. **Execute locally** — use your own tools and credentials. After each action, call **skill_log_step** with:
   - `step_type`: `env_check`, `plan`, `query`, or `note`.
   - For `query` steps: `result_class` is required (`ok`, `no_data`, `wrong_mapping`, `no_permission`, `env_mismatch`, `not_available`, `error`).

3. **skill_close** — when done, close the session with a conclusion:
   - `status`: `confirmed`, `likely`, `not_reproduced`, `blocked`, or `answered`.
   - If you have no evidence steps, `confirmed` → `likely` and `answered` → `blocked`.

4. **skill_get_session** — get the compressed trace, then reflect.

5. **skill_update** — submit what you learned:
   - `feedback`: rate items as `helpful`, `not_helpful`, or `outdated`.
   - `reflection`: what went well, what didn't, what to do next time.
   - `operations`: write-back new knowledge. Every write needs `evidence_steps` citing query steps with real results.

## Evidence rules

Only facts backed by this session's own observations can be written to the knowledge base:
- `evidence_steps` must cite step seqs where `result_class` is `ok`, `no_data`, or `wrong_mapping`.
- Sessions without any such step can only submit feedback and reflection.
- Guesses, recollections, and "matches a past case" go into reflection, not into the knowledge base.

## Version check

Compare `bootstrap_version` in the `skill_open` response to your local version. If different, call `skill_get_script` and update this file.
