# agent-factory

Run several AI models as one team from a terminal. You choose the model for each request with an explicit routing command; nothing is chosen for you.

## Install and run

Requires Go (see `go.mod`).

```sh
CGO_ENABLED=0 go build -o agent-factory ./cmd/agent-factory
./agent-factory            # opens the interactive screen
./agent-factory --version
```

Release binaries (Linux, macOS and Windows; amd64 and arm64 except Windows, amd64 only) are attached to each GitHub Release for a `v*` tag. Tags containing a `-` (such as `v1.0.0-rc.1`) are published as pre-releases.

The screen has a scrollable output area and an input box at the bottom. Configuration loads in the background after the screen opens; type once the summary appears.

## Files in `~/.agent-factory/`

Set `AGENT_FACTORY_HOME` to use another directory.

| File | Owner | Purpose |
|---|---|---|
| `models.json` | you | providers, API keys, models, optional metadata |
| `strategies.json` | you | which model serves each routing command |
| `models-metadata.json` | tool | generated model metadata |
| `history.jsonl` | tool | one line per request (no prompt or response text) |

The tool never writes `models.json` or `strategies.json`. Use `/suggest`, paste the result into `strategies.json`, then run `/refresh`.

## Commands

Routing commands start a request. The rest of the line is the prompt:
`/fast`, `/code`, `/reason`, `/creative`

Tool commands:

| Command | What it does |
|---|---|
| `/help` | list all commands |
| `/models` | list models, providers and assigned routing commands |
| `/suggest <cost\|speed\|capability>` | show a suggested `strategies.json` |
| `/refresh` | reload both files and refresh metadata |
| `/history` | list runs; `/history delete <id>`; `/history clear` (asks to confirm) |

Keys: Enter sends, PgUp/PgDn scroll, Ctrl+C cancels a running request (or exits when idle), Ctrl+D on an empty input exits.

## Providers

Each provider in `models.json` needs a `type`. Three are built in:

| `type` | Talks to | Default `baseUrl` |
|---|---|---|
| `anthropic` | Anthropic Messages API | `https://api.anthropic.com` |
| `openai` | any OpenAI-compatible `/chat/completions` server (OpenAI, OpenRouter, Groq, Mistral, DeepSeek, vLLM, LM Studio, ...) | `https://api.openai.com/v1` |
| `ollama` | Ollama native API | `http://localhost:11434` |

```json
{
  "providers": {
    "anthropic":  { "type": "anthropic", "apiKey": "<your key>" },
    "openrouter": { "type": "openai", "baseUrl": "https://openrouter.ai/api/v1", "apiKey": "<your key>" },
    "local":      { "type": "ollama", "timeoutSeconds": 300 }
  },
  "models": [
    { "id": "or/sonnet", "provider": "openrouter", "model": "anthropic/claude-sonnet-5-5" },
    { "id": "qwen", "provider": "local", "model": "qwen3:32b", "maxOutputTokens": 4096 }
  ]
}
```

Provider fields: `type` (required), `baseUrl`, `apiKey` (optional; local servers need none), `timeoutSeconds` (1 to 3600, default 60: how long the tool waits for the first byte and between streamed pieces; there is no limit on the total length).
Model fields: `id` (what you put in `strategies.json`), `provider`, `model` (the exact name sent to the provider; defaults to `id`), `maxOutputTokens` (sent when set; Anthropic defaults to 8192).

Failures are shown with the provider name and a reason (rejected key, unknown model, rate limit, unreachable, no response). The tool never retries and never switches to another model.

## Upgrading

`type` is now required on every provider in `models.json`. Add it, then run `/refresh`.
