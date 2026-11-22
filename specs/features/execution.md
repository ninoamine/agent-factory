---
id: EXEC
status: implemented
version: V1
depends_on: [ROUTE]
modules: [internal/app/execution, internal/infra/providers]
---
# Execution

Goal: run one request on the selected model, stream it, report usage, never fall back.

## Rules

- EXEC-1 MUST call only the model selected by ROUTE, through the provider adapter (ARCH-2).
- EXEC-2 MUST stream chunks when the adapter supports streaming; otherwise show the full response when complete.
- EXEC-3 Ctrl+C MUST stop the request: cancel at the provider if the adapter supports it; otherwise stop displaying output and say the provider could not be cancelled.
- EXEC-4 On error, MUST show the normalized error (`auth`, `model_unavailable`, `provider_failure`, `interrupted`). MUST NOT retry, fall back to another model, or change strategy (ARCH-6).
- EXEC-5 After a successful run, MUST display model name, input tokens, output tokens, total tokens. If unavailable, say "unavailable". MUST NOT invent counts.
- EXEC-6 MUST NOT display monetary cost.
- EXEC-7 MUST send only the stripped prompt; no history, no context (ROUTE-9).
- EXEC-8 After every request (success, error, interrupted), MUST pass a record to HIST (HIST-1).
- EXEC-9 Error messages and logs MUST NOT contain API keys (SEC-1).

## Out of scope

Retries, fallback, multi-model calls, cost computation.

## Acceptance criteria

- AC1 (EXEC-4): mocked provider error shows error and calls no second model.
- AC2 (EXEC-2): mocked stream renders incrementally.
- AC3 (EXEC-3): cancel stops a mocked stream; non-cancellable adapter shows the notice.
- AC4 (EXEC-5): response without usage shows "unavailable".
- AC5 (EXEC-8): each outcome produces one history record.

## V2 amendments (draft, not implemented; D-041)

Apply only when SDD is built, after `v1.0.0` (D-022). V1 rules above stay unchanged for V1. New IDs never reuse old ones.

- EXEC-10 EXEC-7 applies to normal requests. An SDD run MAY send a prompt with file context built by SDD (SDD-41). EXEC MUST NOT add history or context of its own.
- EXEC-11 EXEC MUST accept the context size cap check result from SDD and MUST NOT enforce SDD limits itself (SDD-43).
- EXEC-12 Every other EXEC rule (EXEC-1..6, EXEC-8, EXEC-9) applies unchanged to SDD runs.

- AC6 (EXEC-10): an SDD prompt with file context reaches the fake provider unchanged; a normal `/code hi` still sends exactly `hi`.
