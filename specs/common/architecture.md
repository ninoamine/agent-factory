# Architecture

Modular monolith, one local process. Layers depend downward only: `cli` to `application` to `domain`; `infrastructure` implements interfaces defined by `application`/`domain`. No circular dependencies.

| Layer | Contains | Must not |
|---|---|---|
| cli | TUI rendering, input parsing, Ctrl+C | hold business logic |
| application | configuration, model registry, metadata, strategies, routing, execution, history services | import provider SDKs |
| domain | model, provider, metadata, strategy, routing command, request, result, history entry | import UI or provider libs; do I/O |
| infrastructure | provider adapters, JSON/JSONL storage, atomic writer, logging | leak SDK types upward |

## Rules

- ARCH-1 Routing MUST NOT depend on any provider SDK or client library.
- ARCH-2 Provider integrations MUST be replaceable behind one internal adapter interface.
- ARCH-3 `cli` MUST NOT contain routing, strategy, or configuration logic.
- ARCH-4 Strategy generation MUST NOT execute requests or write files.
- ARCH-5 Each request MUST select exactly one model.
- ARCH-6 Provider failures MUST NOT trigger fallback.
- ARCH-7 The tool MUST NOT write `models.json` or `strategies.json`.
- ARCH-8 Core logic MUST be testable without network or real credentials.
- ARCH-9 Provider SDK types MUST NOT appear in domain or routing code.
- ARCH-10 One request execution is a single use-case function (keeps later orchestration possible).

## Provider adapter contract

Methods: `identify()`, `fetchMetadata()` (optional), `execute(prompt)`, `stream(prompt)` (optional), `cancel()` (optional), usage returned with the result when available. Errors normalized to: `auth`, `model_unavailable`, `provider_failure`, `interrupted`.

## Source tree

```
cmd/agent-factory/main.go
internal/
  cli/       app.go, commands/, components/              (layer: cli)
  app/       configuration/ models/ metadata/ strategies/ routing/ execution/ history/   (layer: application)
  domain/    model.go provider.go strategy.go routingcommand.go request.go
  infra/     providers/ storage/ logging/                (layer: infrastructure)
test/        integration tests (unit tests live beside code as *_test.go)
examples/    sample models.json, strategies.json (no real keys)
```

Interfaces are defined in the consuming package (application), implemented in `infra`. `internal/` keeps packages private to the module.

## Startup data flow

1. TUI opens. 2. CFG locates dir, warns on missing files, validates. 3. META fetches API metadata, merges, writes `models-metadata.json`. 4. User sends input: tool command handled directly; routing command goes ROUTE then EXEC then HIST.

## V2 amendments (draft, not implemented; D-041)

Apply only when SDD is built, after `v1.0.0` (D-022). V1 rules above stay unchanged for V1. New IDs never reuse old ones.

- ARCH-11 Writes outside the config dir MUST go only through one approved-proposal writer in the application layer (SDD-46..51). No other component writes project files. ARCH-7 is unchanged.
- ARCH-12 The only external process the tool may run is `git diff HEAD` (and listing untracked files) for `/sdd review` (SDD-56, D-037). It MUST sit behind an application-defined interface implemented in infrastructure, and MUST NOT use a shell.
- ARCH-13 The `sdd` application package MUST resolve models through ROUTE and run them through EXEC (ARCH-5, ARCH-10). It MUST NOT import provider code.
