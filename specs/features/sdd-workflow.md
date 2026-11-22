---
id: SDD
status: draft
version: V2
depends_on: [CFG, ROUTE, EXEC, HIST, TUI]
modules: [internal/app/sdd, internal/cli]
---
# SDD workflow

Goal: help users build their product with spec-driven development. Agent-factory scaffolds the SDD files in their project, then writes roadmaps and specs, validates specs, implements them, and reviews code. Each command runs on the right model, and every file change needs the user's approval (D-033).

Rules SDD-1..29 (earlier pipeline design) are removed and their IDs are not reused (D-033).

## Rules

### Project and commands

- SDD-30 The project dir MUST be the current working directory at launch. All SDD paths are relative to it.
- SDD-31 Subcommands are `init`, `roadmap <input>`, `spec <input>`, `spec <id> <input>` (SDD-64), `spec-validate <id>`, `implement <id>`, `spec-review <id>`, `review`. `/sdd` with no argument MUST list them and say whether the project is initialized. An unknown subcommand MUST be refused with that list.
- SDD-32 Every subcommand except `init` MUST be refused when `specs/index.md` does not exist, pointing to `/sdd init`.
- SDD-33 `<input>` MUST be read as a file when it is a single token naming an existing regular file (relative to the project dir or absolute; read-only, may be outside the project). Otherwise it is free text. Empty input MUST be refused.
- SDD-34 `<id>` MUST name a feature in `specs/index.md`. Unknown ids MUST be refused with the list of known ids.

### Init

- SDD-35 `/sdd init` MUST NOT call a model. It MUST create these files from built-in, language-neutral templates (derived from this repo's specs): `AGENTS.md`, `specs/index.md`, `specs/roadmap.md`, `specs/decisions.md`, `specs/common/project.md`, `specs/common/architecture.md`, `specs/common/conventions.md`, `specs/templates/feature.md`, `specs/templates/change.md`, `specs/templates/roadmap.md`, and an empty `specs/features/` dir.
- SDD-36 `/sdd init` MUST list the files it will create and the existing files it will skip, then ask for confirmation (TUI-19). MUST NOT overwrite any existing file.

### Models

- SDD-37 Each model-using subcommand MUST map to one routing command through the optional `sdd` object in `strategies.json` (D-034). Defaults: `implement` → `/code`; `roadmap`, `spec`, `spec-validate`, `spec-review`, `review` → `/reason`.
- SDD-38 The model MUST be resolved from the mapped command with ROUTE's rules (enabled, assigned, first model, CFG-10). If it does not resolve, MUST refuse with the subcommand, command and ROUTE-4 reason; nothing is executed.
- SDD-39 Each model run MUST be one EXEC call on exactly one model (ARCH-5), with EXEC's streaming, cancellation, error and usage behavior (EXEC-2..5). MUST NOT retry or fall back (EXEC-4).
- SDD-40 Only one SDD subcommand MAY run or wait for approval at a time. While one does, other `/sdd` subcommands and routing requests MUST be refused like TUI-14. Read-only tool commands still run (TUI-13).

### Context sent to the model

- SDD-41 Each subcommand MUST send a built-in instruction plus exactly this context, each part labeled with its path:
  - `roadmap`: `AGENTS.md`, `specs/common/project.md`, `specs/roadmap.md`, `specs/templates/roadmap.md`, `specs/decisions.md`, the input.
  - `spec`: `AGENTS.md`, `specs/index.md`, `specs/roadmap.md`, `specs/common/*.md`, `specs/templates/feature.md`, the input.
  - `spec-validate`: the feature file, its `depends_on` feature files, `specs/index.md`, `specs/roadmap.md`, `specs/decisions.md`, `specs/common/*.md`.
  - `implement` and `spec-review`: `AGENTS.md`, the feature file, its `depends_on` feature files, `specs/common/*.md`, and the source files under the feature's `modules` folders (SDD-42).
  - `review`: `AGENTS.md`, `specs/common/conventions.md`, the uncommitted diff (SDD-56), and the feature files whose `modules` contain a changed file.
- SDD-42 Source files MUST be limited to the `modules` folders, MUST exclude `.git/`, binary files and git-ignored files, and MUST NOT include files outside the project dir.
- SDD-43 If the context exceeds the size cap (SDD-62), MUST refuse before calling the model, naming the largest contributing files.
- SDD-44 Normal routing requests stay independent (ROUTE-9, EXEC-7). Only SDD prompts carry file context.

### Proposals and approval

- SDD-45 A model's file changes MUST be returned in a fixed proposal format (full file content per path, SDD-61; exact format set at implementation). If the output cannot be parsed, MUST show it raw and write nothing.
- SDD-46 Each subcommand MAY only write its allowed paths: `roadmap` → `specs/roadmap.md`, `specs/decisions.md`; `spec` (new) → one new `specs/features/<name>.md`, `specs/index.md`; `spec <id>` (change) → that feature file, `specs/index.md`, `specs/decisions.md` (append only, SDD-64); `spec-validate` → the feature file, `specs/index.md`; `implement` → files under the feature's `modules` folders, tests included (SDD-65); `spec-review` → the feature file, `specs/index.md`; `review` → nothing. A proposal touching any other path, a path outside the project dir (after resolving `..` and symlinks), or `.git/` MUST be rejected whole, with the offending paths shown.
- SDD-47 MUST NOT delete files. A proposal requesting a deletion MUST be rejected whole.
- SDD-48 Before writing, MUST show each proposed file as new or changed, with a diff against its current content, then ask for approval.
- SDD-49 The approval answers are `yes`/`y` (write all files), `no`/`n` (discard), `redo [feedback]` (rerun the same subcommand with the previous proposal and the feedback added to the prompt), and `edit` (open the proposal in `$VISUAL`, else `$EDITOR`, then re-parse, re-check SDD-46 and ask again). Any other answer MUST show the valid answers and keep waiting. Ctrl+C at the prompt counts as `no`.
- SDD-50 `edit` MUST use a temp file in the OS temp dir with owner-only permissions, deleted after the editor exits. With no editor variable set, it MUST say so and keep waiting. An empty save or a non-zero exit leaves the proposal unchanged.
- SDD-51 Approved files MUST be written atomically (FILE-2). If any write fails, MUST report which files were and were not written.

### Status lifecycle

- SDD-52 Feature statuses are `draft`, `validated`, `implemented`, kept in both the feature front matter and `specs/index.md`. Status edits MUST be made by the tool itself (not taken from model text) and included in the proposal (SDD-48).
- SDD-53 `spec` MUST create the feature with status `draft`, a unique id not already in the index, and a matching index row. A proposal with a duplicate id or invalid front matter MUST be rejected.
- SDD-54 The model's verdict for `spec-validate`, `spec-review` and `review` MUST be read from a fixed marker (`pass` or `fail`); a missing or unreadable marker counts as `fail`.
  - `spec-validate`: `pass` → propose status `validated`. `fail` → show the problems and propose a corrected spec that stays `draft`.
  - `spec-review`: `pass` → propose status `implemented`. `fail` → show the gaps, write nothing.
  - `review`: show the verdict and findings, write nothing.
- SDD-55 `implement` MUST be refused unless the feature's status is `validated`. On an `implemented` feature, the refusal MUST say to change the spec first (SDD-63, SDD-64).

### Review

- SDD-56 `review` MUST get the diff by running `git diff HEAD` directly (no shell) in the project dir, plus untracked files that are not ignored. It MUST NOT run any other command. If the dir is not a git repo, MUST refuse. With no changes, MUST say so and not call the model.

### History and safety

- SDD-57 Every model run MUST produce one HIST record (EXEC-8) with an extra field `sdd` holding the subcommand. No prompt, file or output text is stored (HIST-3).
- SDD-58 MUST NOT run any command except SDD-56's git call. MUST NOT commit, stage or touch git state.
- SDD-59 MUST NOT write `models.json` or `strategies.json` (ARCH-7), or any file outside SDD-36/SDD-46 paths except `history.jsonl` and the SDD-50 temp file.
- SDD-60 Command handling, context building, proposal parsing, path checks and status edits MUST live in the application layer (ARCH-3). The editor and git calls MUST sit behind interfaces so tests use fakes (ARCH-8).

### Limits and spec changes (D-040)

- SDD-61 Proposals MUST contain full file content only; diffs or patches MUST NOT be accepted (rejected as unparsable, SDD-45). A file whose current content alone exceeds the size cap cannot be changed by `implement`; the refusal names it.
- SDD-62 The size cap is 400 KB of text per prompt (instruction, context and input together). No per-model limit in V2.
- SDD-63 Re-implementing an `implemented` feature MUST NOT be possible directly; the spec is changed first (SDD-64), which resets it to `draft`, then validated again.
- SDD-64 `/sdd spec <id> <input>` MUST be treated as a change to an existing feature when the first token exactly matches (case-sensitive) an id in `specs/index.md` and input follows. It MUST send the feature file, its `depends_on` feature files, `specs/templates/change.md`, `specs/index.md`, `specs/decisions.md`, `specs/common/*.md` and the input. The proposal MUST keep the feature id and MUST NOT renumber or reuse rule IDs; the tool MUST reset the status to `draft` in the front matter and the index (SDD-52). A `specs/decisions.md` change MUST only append lines; any other edit to it rejects the proposal whole.
- SDD-65 `implement` MUST NOT write tests or any file outside the feature's `modules` folders (SDD-46); tests live in those folders.

## Out of scope

Agentic tool-calling loops. Running tests, linters or builds. Deleting files. Git commit or stage. Diff or patch proposals. Multiple projects per session. Writing outside the allowed paths.

## Impacts (change specs needed before SDD becomes `ready`)

- `common/project.md` principle 2 and ARCH-7: user config files stay never-written; project files are written only with approval (D-035).
- `common/config-files.md`: new "project files" section (SDD-35, SDD-46).
- ROUTE-7, D-005: `/sdd` is a new tool command that runs models via ROUTE resolution and EXEC.
- ROUTE-9, EXEC-7: exception for SDD prompts (SDD-44).
- HIST-1: optional `sdd` field (SDD-57).
- CFG-5: `sdd` object in `strategies.json` (D-034).
- TUI-8: `/help` lists `/sdd`. TUI-19: SDD approval prompt variant (SDD-49). TUI: suspend the screen while the editor runs. TUI-14: SDD-40.
- `common/conventions.md`: SDD-56's git exec as the only allowed external command.

## Acceptance criteria

Tests use a temp project dir, a fake provider, a fake editor and a fake git (TEST-2).

- AC1 (SDD-31,32): `/sdd foo` lists subcommands. In an empty dir, `/sdd spec x` is refused and points to `/sdd init`.
- AC2 (SDD-35,36): in an empty dir, `/sdd init` then `yes` creates all SDD-35 files, and no model is called. With an existing `AGENTS.md`, it is listed as skipped and its bytes are unchanged. Answering `n` creates nothing.
- AC3 (SDD-33): `/sdd spec ../draft.txt` with that file present sends its content. `/sdd spec add login page` sends the text. `/sdd spec` is refused.
- AC4 (SDD-37,38): with no `sdd` object, `implement` uses `/code`'s model and `spec` uses `/reason`'s. With `"sdd": {"review": "/fast"}`, `review` uses `/fast`'s model. With `/code` disabled, `/sdd implement AUTH` is refused naming `/code` and "disabled".
- AC5 (SDD-41,42): `/sdd implement AUTH` with `modules: [src/auth]` sends `src/auth/login.go` but not `src/other/x.go`, a git-ignored `src/auth/.env`, or a binary `src/auth/logo.png`.
- AC6 (SDD-43): a module folder over the cap is refused before any provider call, naming the largest file.
- AC7 (SDD-45): unparsable model output is shown raw; no file changes.
- AC8 (SDD-46,47): an `implement` proposal for `README.md`, `../x`, `.git/config`, a symlink pointing outside, or a deletion is rejected whole, and nothing is written.
- AC9 (SDD-48,49,51): a `spec` proposal shows the new feature file and the index diff. `yes` writes both. `no` writes nothing. `redo shorter` calls the provider again with the previous proposal and `shorter` in the prompt. `maybe` shows the valid answers and keeps waiting.
- AC10 (SDD-49,50): the fake editor changes a path to `README.md`; the re-check rejects the proposal. With no editor variable set, `edit` shows a message and keeps waiting. The temp file is gone afterwards and had mode `0600`.
- AC11 (SDD-53): a `spec` proposal reusing id `AUTH` already in the index is rejected. A new spec is written with status `draft` in the front matter and the index.
- AC12 (SDD-54): `spec-validate AUTH` with verdict `pass` proposes `validated` in both places. With `fail`, it shows the problems and the proposed spec stays `draft`. With no marker, it is treated as `fail`.
- AC13 (SDD-54): `spec-review AUTH` with `pass` proposes `implemented`; with `fail`, nothing is written.
- AC14 (SDD-55): `/sdd implement AUTH` on a `draft` spec is refused.
- AC15 (SDD-56): with the fake git returning no diff, `review` calls no model. Outside a git repo, it is refused. With a diff touching `src/auth/x.go`, the prompt includes the AUTH spec. Nothing is written.
- AC16 (SDD-40): while `/sdd spec x` streams, `/sdd review` and `/code hi` are refused, and `/models` runs.
- AC17 (SDD-57): three SDD model runs produce three history lines with `sdd` set and no file or prompt text.
- AC18 (SDD-59): after a full init → spec → validate → implement → spec-review cycle, `models.json` and `strategies.json` are byte-identical, and only allowed paths changed in the project dir.
- AC19 (SDD-61): model output containing a unified diff instead of full content is shown raw and nothing is written. `implement` on a module with a 500 KB file is refused, naming it.
- AC20 (SDD-62): a context of 399 KB calls the provider; 401 KB is refused before any call.
- AC21 (SDD-55,63): `/sdd implement AUTH` on an `implemented` AUTH is refused with a message pointing to `/sdd spec AUTH <change>`.
- AC22 (SDD-64): `/sdd spec AUTH add rate limit` sends the AUTH file and `change.md`. After `yes`, AUTH's status is `draft` in the front matter and the index, and existing rule IDs are unchanged. A proposal that renames the id, or edits an existing line of `decisions.md`, is rejected whole. `/sdd spec auth add rate limit` (lowercase) creates a new spec instead.
- AC23 (SDD-65): an `implement` proposal for `test/auth_test.go` with `modules: [src/auth]` is rejected; `src/auth/auth_test.go` is accepted.

## Open questions

None.
