---
id: REFRESH
status: implemented
version: V1
depends_on: [CFG, META]
modules: [internal/app/configuration]
---
# Refresh

Goal: apply the user's file edits and fresh API metadata without restarting, safely (D-003).

## Rules

- REFRESH-1 `/refresh` MUST re-read `models.json` and `strategies.json`.
- REFRESH-2 MUST re-fetch API metadata and regenerate `models-metadata.json` (META-3, META-10).
- REFRESH-3 MUST build the new configuration completely, then swap it in only if everything validates.
- REFRESH-4 If any file fails validation, the previous active configuration MUST stay active; show errors per file (file, field, reason).
- REFRESH-5 A metadata fetch failure alone MUST NOT fail the refresh; follow META-8 and show the warning.
- REFRESH-6 On success MUST show the summary (CFG-7) and any missing-file warnings (CFG-2).
- REFRESH-7 File edits MUST have no effect until `/refresh` or restart.
- REFRESH-8 `/refresh` MUST NOT write `models.json` or `strategies.json`.
- REFRESH-9 `/refresh` while a request is running MUST be refused until the request finishes.

## Out of scope

File watching, automatic refresh, partial application.

## Acceptance criteria

- AC1 (REFRESH-1,3): valid edits are active after `/refresh`.
- AC2 (REFRESH-4): broken `strategies.json` leaves old routing working and shows the error.
- AC3 (REFRESH-5): fetch failure gives warning, refresh succeeds.
- AC4 (REFRESH-7): edit without `/refresh` changes nothing.
- AC5 (REFRESH-9): `/refresh` during a running request is refused, the request finishes unaffected, and the active configuration is unchanged.

## Open questions

None.

## V2 amendments (draft, not implemented; D-041)

Apply only when SDD is built, after `v1.0.0` (D-022). V1 rules above stay unchanged for V1. New IDs never reuse old ones.

- REFRESH-10 `/refresh` MUST be refused while an SDD subcommand is running or waiting for approval (SDD-40), with a message, so the models cannot change under a pending proposal. The active configuration is unchanged.

- AC6 (REFRESH-10): `/refresh` during an SDD proposal prompt is refused and the configuration fake is not called.
