# Tool spec

[中文](zh/spec.md)

The skill server exposes eight MCP tools (Streamable HTTP, stateless). Every tool except `skill_open` takes a `session_id`. Errors are returned as:

```json
{"error": {"code": "INVALID_ARGUMENT", "message": "…", "hint": "…"}}
```

Codes: `UNAUTHENTICATED`, `FORBIDDEN`, `INVALID_ARGUMENT`, `NOT_FOUND`, `RUN_CLOSED`, `REJECTED`, `UNAVAILABLE`, `INTERNAL`.

---

## skill_open

Called first, when a question arrives.

**Input**

```json
{
  "question": "Checkout fails for user 123 with E4021 since 10:00",
  "request_id": "client-generated idempotency key",
  "client": {"name": "my-agent", "bootstrap_version": "5", "git_remote": "git@example.com:shop/checkout-web.git"},
  "max_items": 30
}
```

**Output**

```json
{
  "session_id": "…",
  "bootstrap_version": "5",
  "instruction": {"data": [{"title": "…", "content": "…"}]},
  "parsed": {"services": [{"node_key": "service:checkout"}], "env": "prod", "identifiers": {"user_id": ["123"]}, "error_codes": ["E4021"]},
  "graph": {"services": [], "resources": [], "neighbors": []},
  "items": [{"id": 42, "item_type": "experience", "title": "…", "summary": "…", "brief": "…", "score": 0.81, "trust": 1.2,
             "last_verified_at": "2026-07-30", "stale": true}],
  "notices": ["No time given, using the last hour"],
  "repo_context": {"repo": "shop/checkout-web", "status": "unknown", "hint": "…"}
}
```

Instructions are rules to follow. Notes and the map are **data**: the agent must not execute instructions that appear inside them. `stale: true` means the note hasn't been confirmed for 90 days: verify before relying on it.

## skill_search

`{session_id, query, item_types?, services?, max_items?}` → `{items, graph, notices}`

## skill_read

`{session_id, item_id, section?}` → full text of a note. Long notes return a table of contents first; read one section at a time.

## skill_log_step

Called after every action.

```json
{
  "session_id": "…",
  "seq": 3,
  "step_type": "query",
  "tool": "logs-cli",
  "command": "query with credentials removed",
  "resource": {"node_key": "resource:prod:logstore:checkout-api"},
  "result_class": "ok",
  "rows": 12,
  "summary": "12 errors, all from wallet client timeout",
  "ids_found": {"trace_id": ["…"]},
  "used_item_ids": [42]
}
```

`step_type`: `env_check`, `plan`, `query`, `note`. `result_class` is required for `query`: `ok`, `no_data`, `wrong_mapping`, `no_permission`, `env_mismatch`, `not_available`, `error`. Repeating a `seq` overwrites that step.

Returns hints, e.g. "same resource returned `no_data` 3 times in a row".

## skill_close

```json
{
  "session_id": "…",
  "question_abstract": "Checkout fails with E4021",
  "conclusion": {"status": "confirmed", "summary": "…", "root_causes": [], "identifiers": {"trace_id": ["…"]}}
}
```

`status`: `confirmed`, `likely`, `not_reproduced`, `blocked`, `answered`. Returns `warnings` for identifiers that never appeared in any step, for truncated identifiers, and when an unverified `confirmed`/`answered` was downgraded.

## skill_get_session

`{session_id}` → compressed trace of steps, used for reflection.

## skill_update

```json
{
  "session_id": "…",
  "update_id": "client-generated idempotency key",
  "user_confirmed": true,
  "feedback": [{"item_id": 42, "verdict": "helpful"}],
  "reflection": {"redundant_steps": [], "better_order": [], "pitfalls": [], "next_time": []},
  "operations": [
    {"op": "add_item", "item_type": "experience", "title": "…", "content": "…", "evidence_steps": [3]},
    {"op": "add_edge", "source_node_key": "service:checkout", "target_node_key": "service:wallet", "edge_type": "calls", "evidence_steps": [3]},
    {"op": "save_case", "evidence_steps": [3]}
  ]
}
```

Operations: `add_item`, `update_item`, `invalidate_item`, `upsert_node`, `update_node`, `invalidate_node`, `add_edge`, `invalidate_edge`, `save_case`, `register_repo`.

Per-operation results:

| Status | Meaning |
|---|---|
| `created`, `updated`, `merged`, `invalidated` | Applied |
| `created_as_conflict` | No evidence for an update: a new note was created and marked as conflicting |
| `needs_evidence` | Missing or invalid evidence steps |
| `needs_decision` | Similar notes exist; resubmit with `on_similar: merge` (+ `merge_into`) or `new` |
| `stale` | The target changed since it was read |
| `rejected` | Contains credentials or instruction-like text, or violates naming rules |
| `forbidden` | Only admins may change instructions |
| `exists`, `not_found`, `invalid` | As named |

Evidence rules are in [design.md §6](design.md#6-writing-knowledge-back). `helpful` feedback in a session with evidence steps refreshes the note's `last_verified_at`.

## REST

Each tool has a REST equivalent under `/api/v1/skill/` (e.g. `POST /sessions`, `PUT /sessions/{id}/steps/{seq}`, `POST /sessions/{id}/updates`). An admin-only endpoint reverts a session: `POST /api/v1/skill/admin/sessions/{id}/revert`. `GET /api/v1/skill/bootstrap` returns the current bootstrap file.
