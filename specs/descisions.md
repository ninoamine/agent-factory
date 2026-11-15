# Decisions log
 
Short entries. Check here before asking the user again. Status: `decided` | `proposed` (needs user confirmation).
 
| ID | Decision | Status |
|---|---|---|
| D-001 | Tool and command name: `agent-factory`. Config dir: `~/.agent-factory/` | decided |
| D-002 | The tool never writes `strategies.json`; `/suggest` only displays content to copy-paste | decided |
| D-003 | `/refresh` re-reads files, re-fetches API metadata, regenerates `models-metadata.json`, applies without restart | decided |
| D-004 | Several models cannot be assigned to the same command: one command, at most one model. A model may be reused across commands | decided |
| D-005 | Tool commands: `/refresh`, `/suggest [cost\|speed\|capability]`, `/models`, `/history`, `/help` | proposed (only `/refresh` given by user) |
| D-006 | Warn when `models.json` permissions are broader than owner-only | proposed |
| D-007 | Env var `AGENT_FACTORY_HOME` overrides the config dir (used by tests) | proposed |
| D-008 | Requests stay independent (no context) and always need an explicit routing command in V1 | decided |
| D-009 | `strategies.json` schema is finalized at implementation; shape in `common/config-files.md` is illustrative | decided |
| D-010 | Language: Go (verify current stable at implementation). Stdlib-first (`encoding/json`, `log/slog`, `testing`). Single static binary. Replaces the earlier TypeScript stack | decided |
| D-011 | Pi (pi.dev) is inspiration for UX only. No dependency on Pi packages. TUI library and provider approach (plain HTTP or official SDKs) are chosen by a prototype; Bubble Tea is a candidate | decided |
| D-012 | Release on tag `v*`: build `linux/amd64` and `linux/arm64` static binaries, attach them plus checksums to a GitHub Release, embed the tag as version | proposed (user asked only that CI builds the CLI on tag creation) |
 
## Template for new entries
 
`| D-0NN | <one-line decision> | decided/proposed |`
 