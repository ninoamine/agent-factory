---
id: CFG
status: implemented
version: V1
depends_on: []
modules: [internal/app/configuration, internal/infra/storage]
---
# Config loading

Goal: locate, check, and validate the user's files at startup without ever modifying them.

## Rules

- CFG-1 MUST resolve the directory as `$AGENT_FACTORY_HOME` if set, else `~/.agent-factory/` (D-001, D-007).
- CFG-2 MUST check that `models.json`, `strategies.json`, `models-metadata.json` exist at startup and show one warning per missing file, naming its full expected path.
- CFG-3 Missing `models.json`: warn that no model is usable and explain how to create it. MUST NOT crash. MUST NOT create it.
- CFG-4 Missing `strategies.json`: warn that routing commands are unavailable and point to `/suggest` then `/refresh`. MUST NOT crash. MUST NOT create it.
- CFG-5 MUST validate `models.json` and `strategies.json` (typed structs, unknown fields rejected, explicit `Validate()`; CONV-5). On invalid content, show file, field path, and reason. MUST NOT crash.
- CFG-6 MUST NOT write `models.json` or `strategies.json` (ARCH-7).
- CFG-7 MUST show a configuration summary (providers, models, enabled commands, strategy). API keys MUST NOT appear in it (SEC-1).
- CFG-8 SHOULD warn when `models.json` permissions are broader than owner-only (D-006).
- CFG-9 Missing metadata fields on a model MUST NOT make the model invalid.
- CFG-10 If an enabled `strategies.json` assignment names a model absent from `models.json`, that command MUST be unusable and a warning MUST be shown. Other commands stay usable.

## Clarifications

- CFG-5 checks: `models.json` rejects empty or duplicate model ids and models naming an undeclared provider; `strategies.json` requires `strategy` in `cost|speed|capability`, command keys in `/fast|/code|/reason|/creative`, no empty model id, and at most one model per command (D-004).
- CFG-5 also covers a file that exists but cannot be read: reported as a problem (path `$`), never a crash. Only context cancellation returns an error.
- CFG-8 warns when any group or other permission bit is set.
- CFG-10: one unknown model in an assignment makes that command unusable.
- A missing `models-metadata.json` gets a warning (CFG-2) saying it is generated at startup.

## Out of scope

Editing config from the CLI. Creating template files. Watching files for changes (that is `/refresh`).

## Acceptance criteria

- AC1 (CFG-2,3,4): empty home dir shows three warnings with paths; app still starts.
- AC2 (CFG-5): malformed JSON and schema violations produce an error naming file and field.
- AC3 (CFG-6): after any startup, `models.json` and `strategies.json` are byte-identical.
- AC4 (CFG-7): summary output contains no API key text.
- AC5 (CFG-9): a model with no metadata loads.
- AC6 (CFG-10): a command naming an unknown model is marked unusable with a warning; other commands still work.

## Open questions

None.
