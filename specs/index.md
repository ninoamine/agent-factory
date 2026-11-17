# Spec index

Status: `draft` (not ready to implement) | `ready` | `implemented`. Version: see `roadmap.md`.

| ID | File | Status | Version | depends_on | Modules |
|---|---|---|---|---|---|
| CFG | features/config-loading.md | implemented | V1 | none | internal/app/configuration, internal/infra/storage |
| META | features/metadata.md | implemented | V1 | CFG | internal/app/metadata, internal/infra/providers |
| STRAT | features/strategy-suggestion.md | implemented | V1 | CFG, META | internal/app/strategies |
| ROUTE | features/routing.md | implemented | V1 | CFG | internal/app/routing, internal/domain |
| EXEC | features/execution.md | implemented | V1 | ROUTE | internal/app/execution, internal/infra/providers |
| HIST | features/history.md | implemented | V1 | EXEC | internal/app/history, internal/infra/storage |
| REFRESH | features/refresh.md | implemented | V1 | CFG, META | internal/app/configuration |
| TUI | features/tui.md | implemented | V1 | all | cmd/agent-factory, internal/cli |

Common (read for every task): `common/project.md`, `common/architecture.md`, `common/conventions.md`. File ownership: `common/config-files.md`.

Suggested implementation order: Go scaffold, spike TUI/provider prototype, CFG, META, ROUTE, EXEC, HIST, STRAT, REFRESH, TUI.