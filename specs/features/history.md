---
id: HIST
status: ready
version: V1
depends_on: [EXEC]
modules: [internal/app/history, internal/infra/storage]
---
# History

Goal: keep minimal local run metadata the user can view and delete.

## Rules

- HIST-1 MUST append one JSON line per request to `history.jsonl` with: id, timestamp, routing command, selected model, input tokens, output tokens, status.
- HIST-2 Status MUST be one of `success`, `error`, `interrupted`.
- HIST-3 MUST NOT store prompt or response text by default.
- HIST-4 Unavailable token counts MUST be stored as null.
- HIST-5 `/history` MUST list entries, each with its id.
- HIST-6 `/history delete <id>` MUST remove that entry. `/history clear` MUST remove all entries. Syntax is proposed (D-005); `clear` SHOULD ask for confirmation.
- HIST-7 Delete and clear MUST rewrite the file atomically (FILE-2).
- HIST-8 History MUST NOT influence routing or configuration.
- HIST-9 If `history.jsonl` is absent, create it on first write with no warning.
- HIST-10 A corrupted line MUST be skipped with a warning, not crash the app.

## Out of scope

Storing prompts/responses, analytics, cost, SQLite.

## Acceptance criteria

- AC1 (HIST-1,2,4): three outcomes produce three valid lines; missing usage is null.
- AC2 (HIST-3): no prompt text in the file.
- AC3 (HIST-6,7): delete one leaves others intact; clear empties; interrupted write cannot corrupt.
- AC4 (HIST-10): file with a bad line still lists the good ones.
