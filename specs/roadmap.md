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

## Later candidates (unordered)

Task orchestration across models, agent teams, conversation context, usage-informed recommendations, cost tracking, automatic task classification, tool-assisted strategy writing, optional sync.
