# Project: Agent Factory

## Vision

Run multiple AI models as one team, eventually a factory that orchestrates each task to the best model. Users bring their own providers and models. Long-term only; see `roadmap.md` for what is buildable now.

## V1 in one paragraph

Interactive terminal app: center area for tool and model output, bottom input box. The user owns `models.json` and `strategies.json` in `~/.agent-factory/`. The tool generates `models-metadata.json` (API metadata merged with user metadata), displays suggested strategies for copy-paste, and reloads with `/refresh`. Each request starts with an explicit routing command and goes to exactly one model.

## Principles

1. Explicit over implicit: every request has a valid routing command.
2. User control: the tool never writes user-owned files.
3. Honest metadata: unknown stays unknown; never invent.
4. Predictable execution: one request, one model, visible failures.
5. Local-first: files stay on the user's machine.
6. Replaceable integrations: provider code isolated behind adapters.
7. Simple V1: no premature abstraction.

## Glossary (use these words only)

| Term | Meaning |
|---|---|
| routing command | `/fast`, `/code`, `/reason`, `/creative`; selects the model for one request |
| tool command | `/refresh`, `/suggest`, `/models`, `/history`, `/help`; controls the app, never routed |
| API metadata | model info fetched from the provider |
| user metadata | optional model info the user writes in `models.json`; wins over API metadata |
| strategy | one global optimization goal: cost, speed, or capability |
| assignment | routing command to list of models, stored in `strategies.json` |
| active configuration | the last successfully validated set of files held in memory |

## V2 amendments (draft, not implemented; D-041)

Apply only when SDD is built, after `v1.0.0` (D-022). V1 rules above stay unchanged for V1. New IDs never reuse old ones.

- Principle 2 (V2): the tool never writes `models.json` or `strategies.json`. It MAY write files in the user's project dir, only through an approved SDD proposal (SDD-46..51, D-035).
- Principle 1 (V2): every request has a valid routing command, except `/sdd` subcommands (mapped by D-034).
- New glossary terms: `SDD` (spec-driven development: `/sdd` subcommands), `proposal` (file changes returned by a model, shown as a diff, written only after approval), `project dir` (working directory at launch).
