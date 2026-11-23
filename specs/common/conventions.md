# Conventions
 
## Code (Go)
 
- CONV-1 Go modules, current stable Go (verify at implementation). Layout: `cmd/agent-factory/`, `internal/`. Code MUST be `gofmt`-formatted and pass `go vet`.
- CONV-2 Return `error`; wrap with context (`fmt.Errorf("loading models.json: %w", err)`). No panics for expected failures. Messages are actionable (what failed, where, what to do).
- CONV-3 Anything doing I/O or cancellable work takes `context.Context` as first parameter (used for EXEC-3).
- CONV-4 Define small interfaces where they are consumed (application layer), implement them in infra. Small functions, clear names.
- CONV-5 Validate untrusted input (config files, provider responses): decode into typed structs with unknown fields rejected, then an explicit `Validate()`. Never use unvalidated data.
- CONV-6 Build only the requested rules. No speculative abstractions, no unrelated refactors.
## Security
 
- SEC-1 Never print API keys in terminal output, logs, errors, snapshots, metadata files, or docs.
- SEC-2 Never commit keys or credentials; examples use placeholders.
- SEC-3 Treat config files and provider responses as untrusted; validate (CONV-5).
- SEC-4 No telemetry, no central backend. Plain-text keys in `models.json` are accepted in V1.
- SEC-5 Log with `log/slog` through a redacting handler; do not log prompts or responses.
## Testing
 
- TEST-1 Every rule ID has at least one test whose name or `t.Run` label contains the ID (for example `t.Run("ROUTE-4 rejects unknown command", ...)`), so `grep ROUTE-4` finds it.
- TEST-2 Standard `testing` package, table-driven where natural. No network, no real keys; fake providers via interfaces; use `t.Setenv("AGENT_FACTORY_HOME", t.TempDir())`.
- TEST-3 Cover success and failure paths: malformed config, missing files, provider errors, missing token usage.
- TEST-4 Add a test proving the tool never writes `models.json` or `strategies.json`.
## Dependencies
 
- DEP-1 Standard library first. Add a dependency only if it materially helps. Check maintenance, license, security, compatibility. Commit `go.mod` and `go.sum`.
- DEP-2 Dependency types MUST NOT appear in `internal/domain`.
- DEP-3 Choose the TUI library and the provider approach (plain `net/http` or official SDKs) with a small prototype before adopting (Bubble Tea is a candidate). Pi is inspiration only (D-011).
## README and CI (after every feature)
 
- DOC-1 After finishing a feature, MUST check whether `README.md` needs an update and update it if so. Triggers: new or changed user-visible command, config file, install or run step, or behavior. If `README.md` does not exist, create it.
- DOC-2 README content MUST describe only what is implemented now: what the tool is, install and run, the files in `~/.agent-factory/` with their owner, available commands. MUST NOT document planned features. MUST NOT contain API keys (SEC-1); use placeholders.
- DOC-3 After finishing a feature, MUST check whether CI needs an update and update it if so. Triggers: new build step, new tool or Go version requirement, new check. CI lives in `.github/workflows/`. If no workflow exists, create one.
- DOC-4 CI MUST run the same checks as local work: `gofmt` (fail if any file is unformatted), `go vet ./...`, `go test ./...`. MUST NOT require real API keys or network access to providers.
- DOC-5 If neither file needs a change, say so in the final report. Do not edit them without a reason.
- DOC-6 README and CI changes MUST stay within what the finished feature changed. No unrelated rewrites.
- DOC-7 CI MUST include a release workflow triggered when a tag matching `v*` is pushed (for example `v0.1.0`). If it does not exist, create it with the first feature (DOC-3).
- DOC-8 The release workflow MUST build the CLI from `cmd/agent-factory/` with `CGO_ENABLED=0`, for Linux (V1 is Linux only). Targets: `linux/amd64` and `linux/arm64` (D-012, proposed).
- DOC-9 The release workflow MUST run `gofmt`, `go vet ./...` and `go test ./...` before building, and MUST NOT build if they fail.
- DOC-10 The release workflow SHOULD attach the built binaries and a checksum file to a GitHub Release for that tag (D-012, proposed).
- DOC-11 The binary SHOULD report its version from the tag, set at build time with `-ldflags "-X ..."` (D-012, proposed).
- DOC-12 The release workflow MUST NOT need API keys for providers, and MUST NOT print secrets. It uses only the repository's built-in `GITHUB_TOKEN` with the minimum permissions needed.
- DOC-13 When a feature changes how the CLI is built (new build flag, new required Go version, new target), MUST update the release workflow (DOC-3) and note it in the README install section if relevant (DOC-1).
## Tooling and checks
 
`gofmt`, `go vet ./...`, `go test ./...`. Build with `CGO_ENABLED=0` for a static binary. A linter such as `golangci-lint` is optional (propose before adding). Run the checks before finishing a task.
 

## V2 amendments (draft, not implemented; D-041)

Apply only when SDD is built, after `v1.0.0` (D-022). V1 rules above stay unchanged for V1. New IDs never reuse old ones.

- CONV-7 Files written into the user's project (SDD) MUST be written atomically (FILE-2), only after path checks (SDD-46: inside the project dir after resolving `..` and symlinks, no `.git/`, no deletion).
- TEST-5 Add a test proving SDD writes only allowed paths and never `models.json` or `strategies.json` (SDD-59, AC18).
- SEC-6 SDD MUST NOT send files that match the project's ignore rules or binary files to a model (SDD-42), and MUST NOT run commands other than ARCH-12's (git, and the editor for `edit`).
- DOC-14 When SDD is built, the README MUST document `/sdd`, the `sdd` object in `strategies.json`, and that file writes need approval (DOC-1, DOC-2).
