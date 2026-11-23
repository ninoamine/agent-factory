# agent-factory

Run several AI models as one team from a terminal. You choose the model for each request with an explicit routing command; nothing is chosen for you.

New here? Read the **[guide](docs/guide.md)**: how routing and `/suggest` work, and a step-by-step Groq tutorial.

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
| `/sdd` | spec-driven development in the current directory (see below) |

Keys: Enter sends, PgUp/PgDn scroll, Ctrl+C cancels a running request (or exits when idle), Ctrl+D on an empty input exits.

## Spec-driven development (`/sdd`)

`/sdd` helps you build a product with specs. The project dir is the directory you launched `agent-factory` from. Run `/sdd` to list the subcommands.

| Subcommand | What it does |
|---|---|
| `/sdd init` | creates `AGENTS.md` and a `specs/` skeleton from built-in templates; lists what it will create and skip, asks first, never overwrites; no model call |
| `/sdd roadmap <input>` | updates `specs/roadmap.md` (and appends to `specs/decisions.md`) |
| `/sdd spec <input>` | writes a new feature spec with status `draft` |
| `/sdd spec <id> <input>` | changes an existing spec and resets it to `draft` |
| `/sdd spec-validate <id>` | checks a spec; on `pass` proposes status `ready` |
| `/sdd implement <id>` | writes code and tests under the feature's `modules` folders; only for a `ready` feature |
| `/sdd spec-review <id>` | checks the code against the spec; on `pass` proposes status `implemented` |
| `/sdd review` | reviews the uncommitted changes (`git diff HEAD` plus untracked files); writes nothing |

`<input>` is free text, or a single file path (read only). `<id>` is a feature id from `specs/index.md`.

Every file change needs your approval. The tool shows each proposed file with a diff, then you answer `yes`, `no`, `redo [feedback]` or `edit` (opens the proposal in `$VISUAL` or `$EDITOR`). Nothing is written before `yes`, files are written atomically, deletions and paths outside the allowed ones for that subcommand are rejected, and `models.json` and `strategies.json` are never written. Only one `/sdd` subcommand runs at a time. Each model run is one request on one model and one line in `history.jsonl` (with an `sdd` field and no text).

Each subcommand uses a routing command. Defaults: `implement` uses `/code`; the others use `/reason`. Override them with an optional `sdd` object in your `strategies.json`:

```json
"sdd": { "implement": "/code", "review": "/fast" }
```

Keys: `roadmap`, `spec`, `spec-validate`, `implement`, `spec-review`, `review`. Values: `/fast`, `/code`, `/reason`, `/creative`. A prompt may hold at most 400 KB of text; the tool refuses before calling the model and names the largest files. Files ignored by `.gitignore` and binary files are never sent. The only external commands the tool runs are `git` for `/sdd review` and your editor for `edit`.

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
