# Roadmap

Only the current version is buildable. Everything else needs a product decision first (add it to `decisions.md`).

## V1 (current)

Interactive Linux TUI that routes each request to exactly one configured model via an explicit routing command. Features: CFG, META, STRAT, ROUTE, EXEC, HIST, REFRESH, TUI, PROV. PROV (built-in Anthropic, OpenAI-compatible and Ollama adapters) is the V1 release gate.

## Not in V1 (do not build)

- GUI, hosted backend, accounts, central service, telemetry.
- Tool writing or editing `models.json` or `strategies.json`.
- Automatic model discovery or task classification.
- Default model or implicit routing.
- Custom or renamed routing commands.
- Multi-model collaboration, orchestration, agent teams.
- Conversation history or multi-turn context.
- Automatic fallback or retry to another model.
- Actual cost tracking or display.
- Usage-based learning or automatic strategy adaptation.
- Database for history.
- Encryption at rest for API keys.

## V2 (planned, specs only)

Not buildable until V1 is released as `v1.0.0` (D-022). The SDD spec is `ready`; code starts only after that release. Scope (D-042): SDD only.

- **SDD workflow** (SDD): `/sdd` scaffolds spec-driven development in the user's project (`init`) and drives it: `roadmap`, `spec`, `spec-validate`, `implement`, `spec-review`, `review`. Each subcommand runs on its mapped model (D-034); file changes are written only after the user approves the diff (D-033, D-035).

Still out of V2: fallback/retry to another model, agentic tool-calling loops, running tests or other commands (except `git diff`, D-037), agent teams, free-form multi-turn chat, automatic strategy adaptation, writing `models.json`/`strategies.json`, GUI, hosted backend, telemetry, database, encryption at rest.

## Later candidates (unordered)

Default model, automatic task classification, cost tracking, usage-informed recommendations, agent teams, free-form conversation context, fallback/retry, agentic implement with test runs, tool-assisted strategy writing, optional sync.
