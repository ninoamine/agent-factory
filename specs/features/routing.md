---
id: ROUTE
status: implemented
version: V1
depends_on: [CFG]
modules: [internal/app/routing, internal/domain]
---
# Routing

Goal: turn an explicit routing command into exactly one model, or a clear refusal.

## Rules

- ROUTE-1 Routing commands are `/fast`, `/code`, `/reason`, `/creative`. MUST NOT add custom commands or allow renaming.
- ROUTE-2 A model request MUST start with a routing command that is enabled and has at least one assigned model.
- ROUTE-3 MUST NOT infer a command from the prompt. MUST NOT choose a default model.
- ROUTE-4 If the command is missing, unknown, disabled, or unassigned: explain which, list available commands, and MUST NOT execute.
- ROUTE-5 MUST remove the routing command from the prompt before forwarding the rest.
- ROUTE-6 MUST select exactly one model per request. With several assigned models, use the first listed (D-004, proposed).
- ROUTE-7 Tool commands (`/refresh`, `/suggest`, `/models`, `/history`, `/help`) MUST be handled before routing and never sent to a model.
- ROUTE-8 The routing engine MUST return a model reference only. It MUST NOT call or import provider code (ARCH-1).
- ROUTE-9 Requests are independent: no conversation context (D-008).
- ROUTE-10 Command with nothing after it (`/code`), refuse and ask for a prompt, no execution.
- ROUTE-11 Command match case-sensitive

## Out of scope

Automatic classification, fallback, model ranking at runtime, custom commands.

## Acceptance criteria

- AC1 (ROUTE-2,3): input without a command is refused, nothing executed.
- AC2 (ROUTE-4): unknown, disabled, and unassigned commands each give a distinct message plus the available list.
- AC3 (ROUTE-5): `/code fix this` forwards `fix this`.
- AC4 (ROUTE-6): command with two models selects the first.
- AC5 (ROUTE-7): `/refresh` never reaches routing.

Note (AC5): the routing package returns a tool-command result and never a model. The caller side (tool commands dispatched before routing) is wired and tested with the CLI in TUI.

## Open questions

None.

## V2 amendments (draft, not implemented; D-041)

Apply only when SDD is built, after `v1.0.0` (D-022). V1 rules above stay unchanged for V1. New IDs never reuse old ones.

- ROUTE-12 `/sdd` MUST be handled as a tool command before routing (extends ROUTE-7), and MUST NOT be sent to a model as a prompt.
- ROUTE-13 ROUTE MUST expose resolving a routing command to a model without a prompt, so SDD can resolve its mapped command with the same rules (ROUTE-2, ROUTE-4, ROUTE-6, CFG-10). The result is still a model reference only (ROUTE-8).
- ROUTE-14 ROUTE-9 is unchanged for normal requests; SDD prompts carry file context (SDD-44).

- AC6 (ROUTE-12): `/sdd init` never reaches routing. AC7 (ROUTE-13): resolving `/code` returns its first model; a disabled `/code` returns the ROUTE-4 reason.
