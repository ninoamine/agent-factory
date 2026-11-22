# Decisions log
 
Short entries. Check here before asking the user again. Status: `decided` | `proposed` (needs user confirmation).
 
| ID | Decision | Status |
|---|---|---|
| D-001 | Tool and command name: `agent-factory`. Config dir: `~/.agent-factory/` | decided |
| D-002 | The tool never writes `strategies.json`; `/suggest` only displays content to copy-paste | decided |
| D-003 | `/refresh` re-reads files, re-fetches API metadata, regenerates `models-metadata.json`, applies without restart | decided |
| D-004 | Several models cannot be assigned to the same command: one command, at most one model. A model may be reused across commands | decided |
| D-005 | Tool commands: `/refresh`, `/suggest [cost\|speed\|capability]`, `/models`, `/history`, `/help` | decided (only `/refresh` given by user) |
| D-006 | Warn when `models.json` permissions are broader than owner-only | decided |
| D-007 | Env var `AGENT_FACTORY_HOME` overrides the config dir (used by tests) | decided |
| D-008 | Requests stay independent (no context) and always need an explicit routing command in V1 | decided |
| D-009 | `strategies.json` schema is finalized at implementation; shape in `common/config-files.md` is illustrative | decided |
| D-010 | Language: Go (verify current stable at implementation). Stdlib-first (`encoding/json`, `log/slog`, `testing`). Single static binary. Replaces the earlier TypeScript stack | decided |
| D-011 | Pi (pi.dev) is inspiration for UX only. No dependency on Pi packages. TUI library and provider approach (plain HTTP or official SDKs) are chosen by a prototype; Bubble Tea is a candidate | decided |
| D-012 | Release on tag `v*`: build `linux/amd64` and `linux/arm64` static binaries, attach them plus checksums to a GitHub Release, embed the tag as version | decided (user asked only that CI builds the CLI on tag creation) |
| D-013 | Exit keys: Ctrl+C while idle exits, Ctrl+D on empty input exits. No `/quit` command | decided |
| D-014 | While a request runs, read-only tool commands run; a second routing request, `/refresh` and `/history delete`/`clear` are refused | decided |
| D-015 | Startup: the screen opens first and shows loading; config and metadata results appear in the output area when ready | decided |
| D-016 | TUI library is Bubble Tea. The user waived the prototype required by DEP-3; no prototype is built. Copy-paste of the `/suggest` JSON (TUI-23) is checked at implementation | decided |
| D-017 | Provider adapters use plain `net/http`, no provider SDKs. The user waived the DEP-3 prototype (as in D-016) | decided |
| D-018 | 429, 5xx, timeouts and network errors map to the existing `provider_failure` kind with a readable reason; no new error kinds (PROV-28) | decided |
| D-019 | Each provider in `models.json` declares a required `type` (`anthropic`, `openai`, `ollama`) and optional `baseUrl`, `timeoutSeconds`; each model may set `model` (API name) and `maxOutputTokens`. Anthropic `max_tokens` defaults to 8192 | decided |
| D-020 | Request timeouts are idle timeouts (60s to first byte, 60s between chunks), overridable per provider; no total cap. OpenAI-compatible requests always ask for streamed usage, with no resend if refused | decided |
| D-021 | The screen word-wraps (breaks at spaces; only a word wider than the window is split). A single string longer than the window in the `/suggest` JSON block cannot be kept intact when copied; accepted for V1 | proposed (needs user confirmation) |
| D-022 | V2 is specified in parallel with V1 testing; V2 code starts only after `v1.0.0`. V1 fixes go to `master` as `v1.0.x` | decided |
| D-023 | V2 themes: SDD workflow, default model, automatic task classification, cost tracking, usage-informed recommendations. Fallback/retry stays out | superseded by D-042 |
| D-024 | SDD steps (spec, plan, implement, validate) run as a pipeline: each step goes to its own model, output feeds the next step, user reviews/edits and approves between steps. Context exists only inside one pipeline run; D-008 still holds for normal requests | superseded by D-033 |
| D-025 | SDD implement step returns text (code/diffs) only. The tool never writes project files (principle 2 extends to project files) | superseded by D-033 |
| D-026 | V2 allows a default model for requests without a routing command; amends principle 1 and D-008 "always need an explicit routing command" | superseded by D-042 |
| D-027 | V2 adds automatic task classification of the routing command; mechanism (rules vs. classifier model) is open | dropped by D-042 |
| D-028 | V2 adds tool command `/sdd` with subcommands `start <idea>`, `next`, `redo [feedback]`, `edit`, `stop`, and no-arg status. Its handler runs steps via ROUTE resolution and EXEC; amends D-005 and ROUTE-7 | superseded by D-033 |
| D-029 | SDD steps map to existing routing commands through an optional `sdd` object in `strategies.json`; defaults spec/plan/validate → `/reason`, implement → `/code` | superseded by D-033 |
| D-030 | Each SDD step receives the idea plus all approved earlier outputs; exception to ROUTE-9/EXEC-7 for SDD prompts only | superseded by D-033 |
| D-031 | SDD review: approve (`next`), rerun with feedback (`redo`), edit in `$VISUAL`/`$EDITOR` via an owner-only temp file (`edit`), abort (`stop`) | superseded by D-033 |
| D-032 | SDD runs live in memory only; history gets one record per step with `runId` and `step`, no text (HIST-3 unchanged) | superseded by D-033 |
| D-033 | SDD redesigned from the user's draft (sdd-draft.txt): `/sdd` scaffolds and drives spec-driven development in the user's project with subcommands `init`, `roadmap`, `spec`, `spec-validate`, `implement`, `spec-review`, `review`. Replaces the pipeline design (D-024, D-028, D-030..D-032); SDD-1..29 removed | decided |
| D-034 | Each model-using `/sdd` subcommand maps to a routing command via an optional `sdd` object in `strategies.json`; default `implement` → `/code`, others → `/reason` | decided |
| D-035 | Reverses D-025: the tool may write files in the project dir (cwd), only after the user approves a shown diff (yes/no/redo/edit), only on per-subcommand allowed paths, never deleting. `models.json`/`strategies.json` stay never-written | decided |
| D-036 | `implement` returns file contents that the tool applies after approval; no agentic tool-calling loop. Context = the spec's `modules` folders + spec + dependencies | decided |
| D-037 | The only external command the tool runs is `git diff HEAD` (no shell) for `/sdd review`. No tests, linters or commits | decided |
| D-038 | SDD index is `specs/index.md` (markdown table), not YAML. Feature statuses: `draft` → `ready` → `implemented` (same as this project) | decided |
| D-039 | `/sdd init` copies built-in language-neutral templates without a model call and never overwrites existing files. `spec-validate` on failure proposes a corrected spec | decided |
| D-040 | SDD open questions resolved as proposed: full-file proposals only; 400 KB prompt cap; no direct re-implement of an `implemented` spec; `/sdd spec <id> <input>` changes a spec via `change.md`, resets it to `draft`, may append to decisions; `implement` writes only inside `modules` (tests included) | decided |
| D-041 | V2 changes to implemented V1 specs are added as a `V2 amendments` section at the end of each file, with new rule IDs; V1 rules stay untouched until V2 is built. `/refresh` is refused during an SDD run (REFRESH-10) | decided |
| D-042 | V2 scope is SDD only. Default model, automatic classification, cost tracking and usage recommendations move to later candidates; D-023 and D-026 are superseded, D-027 is dropped. SDD spec is `ready` (user reviewed) | decided |
 
## Template for new entries
 
`| D-0NN | <one-line decision> | decided/proposed |`
 
