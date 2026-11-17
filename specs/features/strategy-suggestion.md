---
id: STRAT
status: implemented
version: V1
depends_on: [CFG, META]
modules: [internal/app/strategies]
---
# Strategy suggestion

Goal: show a suggested `strategies.json` the user can copy-paste. The tool changes nothing (D-002).

## Rules

- STRAT-1 `/suggest <cost|speed|capability>` MUST read `models-metadata.json` and display a JSON block shaped like `strategies.json`.
- STRAT-2 `/suggest` with no or an invalid argument MUST show usage listing the three strategies (proposed). MUST NOT guess one.
- STRAT-3 cost: prefer the lowest estimated cost only where usable pricing exists. Without pricing, MUST NOT claim a cost advantage.
- STRAT-4 speed: prefer fastest expected response only where reliable speed data allows comparison. MUST NOT fabricate a ranking.
- STRAT-5 capability: pick the best match per routing command from metadata and user metadata.
- STRAT-6 MUST show a short reason per assignment as text OUTSIDE the JSON block so the pasted JSON stays valid.
- STRAT-7 If no suitable model is known for a command, leave it unassigned (empty list) and say so.
- STRAT-8 Reuse of a model across commands is allowed. At most one model per command (D-004, CFG-5).
- STRAT-9 MUST NOT write any file. MUST NOT execute a request (ARCH-4).
- STRAT-10 MUST end with: paste into `<dir>/strategies.json`, then run `/refresh`.
- STRAT-11 The JSON block MUST pass the `strategies.json` schema (CFG-5).
- STRAT-12 capability: the match per routing command MUST be derived from the model capability tags (such as code, reasoning, creative, speed) in metadata and user metadata.

- STRAT-13 speed ranking uses only the values `fast` > `medium` > `slow` (case-insensitive). Any other value counts as unknown: never ranked, never claimed as an advantage (STRAT-4).

## Out of scope

Writing the file. Interactive review or validation in the TUI. Learning from history.

## Acceptance criteria

- AC1 (STRAT-3,4): fixtures with missing pricing/speed never produce a cost or speed claim for those models.
- AC2 (STRAT-6,11): extracted JSON block validates; reasons are outside it.
- AC3 (STRAT-7): command with no fitting model is empty and flagged.
- AC4 (STRAT-9): no file system writes during `/suggest`.

## Open questions

None.
