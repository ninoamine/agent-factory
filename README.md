# agent-factory

Run several AI models as one team from a terminal. You choose the model for each request with an explicit routing command; nothing is chosen for you.

## Install and run

Requires Go (see `go.mod`).

```sh
CGO_ENABLED=0 go build -o agent-factory ./cmd/agent-factory
./agent-factory            # opens the interactive screen
./agent-factory --version
```

Release binaries (Linux amd64 and arm64) are attached to each GitHub Release for a `v*` tag.

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

## Current limitations

No provider adapters are built in yet, so a request to a model reports `model_unavailable`.
