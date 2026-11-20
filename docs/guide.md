# agent-factory guide

This guide explains how the pieces fit together, exactly how `/suggest` decides, and walks through a complete setup with [Groq](https://groq.com) (free tier available).

## 1. The mental model

agent-factory never picks a model for you. You type a **routing command** and the tool sends your prompt to the model assigned to that command.

```
/code write a Go function that reverses a string
 │     └── the prompt (sent to the model, without the command)
 └── routing command → looks up strategies.json → one model
```

Three files in `~/.agent-factory/` (or `$AGENT_FACTORY_HOME`) drive everything:

| File | Who writes it | Role |
|---|---|---|
| `models.json` | you | **What exists**: providers (URL, key) and models (+ optional metadata) |
| `models-metadata.json` | the tool | **What we know**: capabilities, speed, pricing per model, generated from `models.json` + provider APIs |
| `strategies.json` | you (usually pasted from `/suggest`) | **What is used**: which model serves each routing command |

Flow: `models.json` → (`/refresh`) → `models-metadata.json` → (`/suggest`) → text you paste into `strategies.json` → (`/refresh`) → active.

Rules worth knowing:

- Routing commands are fixed: `/fast`, `/code`, `/reason`, `/creative`. Matching is case-sensitive.
- No command, or an unknown/disabled/unassigned one → the request is refused, nothing is sent.
- With several models listed for a command, only the **first** is used. No fallback, no retry.
- Every request is independent: no conversation memory.
- The tool never writes `models.json` or `strategies.json`. Edits only take effect after `/refresh`.

## 2. How `/suggest` decides

`/suggest <cost|speed|capability>` reads `models-metadata.json`, prints a `strategies.json` and the reason for each choice. It writes nothing and sends no request. With no or an invalid argument it prints usage; it never guesses.

Only models that have **some** metadata are considered. A model with no capabilities, speed or pricing at all is never suggested.

For each of the four commands (`/fast`, `/code`, `/reason`, `/creative`) it picks **one** model (or none):

### `/suggest cost`

- Cost of a model = `inputPer1M + outputPer1M` (USD per million tokens).
- Picks the model with the lowest cost. Ties: the first in the file.
- A model missing either price is ignored. If no model has both prices, every command is left **unassigned**; the tool will not claim a cost advantage it cannot prove.
- The same model is chosen for all four commands (cost does not depend on the command).

### `/suggest speed`

- Uses only the `speed` values `fast` > `medium` > `slow` (case-insensitive). Any other value counts as unknown.
- Picks the fastest. Ties: the first in the file. If no model has a valid speed, commands are left unassigned.
- Same model for all four commands.

### `/suggest capability`

Each command needs one **capability tag**:

| Command | Required tag |
|---|---|
| `/fast` | `speed` |
| `/code` | `code` |
| `/reason` | `reasoning` |
| `/creative` | `creative` |

- Picks the **first** model (in `models.json` order) whose `capabilities` list contains the tag (case-insensitive).
- If no model has the tag, that command is left unassigned and the reason says so.
- This is the only strategy that can give different models to different commands. **Order matters**: put your preferred model first.

> Note the tag for `/fast` is `speed` (not `fast`), and for `/reason` it is `reasoning` (not `reason`).

### Reading the output

```
Suggested strategies.json (capability):

{ ...JSON... }

Reasons:
- /fast: groq/gpt-oss-20b. First model with the "speed" capability tag.
- /creative: unassigned. No known model has the "creative" capability tag.

Paste into ~/.agent-factory/strategies.json, then run /refresh.
```

Unassigned commands appear as `"enabled": false, "models": []`.

### Where does metadata come from?

Per field (`capabilities`, `speed`, `pricing`), your values in `models.json` **always win** over what the provider API returns. A missing value stays unknown; nothing is invented. `models-metadata.json` records the `sources` (`api` or `user`) of each field. Many providers expose no capabilities or speed through their API, so **you should declare them yourself** (see below), otherwise `/suggest capability` and `/suggest speed` have nothing to work with.

```json
"metadata": {
  "capabilities": ["code", "reasoning"],
  "speed": "medium",
  "pricing": { "inputPer1M": 0.15, "outputPer1M": 0.6 }
}
```

All fields are optional. `pricing` replaces API pricing as a whole block.

## 3. Tutorial: first run with Groq

### Step 1: get a key

Create an API key at <https://console.groq.com/keys>.

### Step 2: install

Download the binary for your platform from the [GitHub Releases](https://github.com/ninoamine/agent-factory/releases) page (Linux, macOS, Windows), then:

```sh
chmod +x agent-factory-linux-amd64      # Linux/macOS only
./agent-factory-linux-amd64 --version
```

### Step 3: write `models.json`

```sh
mkdir -p ~/.agent-factory
```

`~/.agent-factory/models.json`:

```json
{
  "providers": {
    "groq": {
      "type": "openai",
      "baseUrl": "https://api.groq.com/openai/v1",
      "apiKey": "gsk_your_key_here",
      "timeoutSeconds": 120
    }
  },
  "models": [
    {
      "id": "groq/gpt-oss-120b",
      "provider": "groq",
      "model": "openai/gpt-oss-120b",
      "metadata": { "capabilities": ["code", "reasoning"], "speed": "medium" }
    },
    {
      "id": "groq/gpt-oss-20b",
      "provider": "groq",
      "model": "openai/gpt-oss-20b",
      "metadata": { "capabilities": ["speed", "code"], "speed": "fast" }
    },
    {
      "id": "groq/qwen3.8-27b",
      "provider": "groq",
      "model": "qwen/qwen3.8-27b",
      "metadata": { "capabilities": ["code", "creative"], "speed": "fast" }
    }
  ]
}
```

- `type: "openai"` means "any OpenAI-compatible server". Groq is one.
- `id` is the name you use in `strategies.json`. `model` is the exact name sent to Groq.
- Keep this file private: it contains your key. Never commit it.

### Step 4: start and generate metadata

```sh
./agent-factory-linux-amd64
```

Wait for the summary to appear. Metadata is generated at startup when absent. Run `/models` to see your models; none is assigned to a command yet.

### Step 5: ask for a suggestion

```
/suggest capability
```

With the file above you get: `/fast` → `groq/gpt-oss-20b`, `/code` → `groq/gpt-oss-120b` (first with `code`), `/reason` → `groq/gpt-oss-120b`, `/creative` → `groq/qwen3.8-27b`.

Try `/suggest cost` and `/suggest speed` to compare.

### Step 6: apply it

Copy the JSON block into `~/.agent-factory/strategies.json`, then in the tool:

```
/refresh
```

You can edit the file freely first, for example to force a different model on one command.

### Step 7: use it

```
/code write a Go function that reverses a string
/reason why is the sky blue?
/fast one-line summary of what a mutex is
```

`/history` lists past runs (model, token counts, status; never your prompts or answers).

### Keys

Enter sends. PgUp/PgDn scroll. Ctrl+C cancels a running request (exits when idle). Ctrl+D on an empty input exits.

## 4. Troubleshooting

| Symptom | Likely cause |
|---|---|
| `/suggest` says it cannot read `models-metadata.json` | Fix `models.json`, run `/refresh`, retry |
| A command is unassigned after `/suggest capability` | No model declares the matching tag (`speed`, `code`, `reasoning`, `creative`); add it in `models.json` metadata |
| `/suggest cost` assigns nothing | No model has both `inputPer1M` and `outputPer1M`; add `pricing` |
| Edits have no effect | Run `/refresh` |
| Refused: "unknown/disabled/unassigned command" | Check `strategies.json` has `"enabled": true` and a model for that command; commands are case-sensitive |
| Provider error mentions rejected key / unknown model | Check `apiKey` and the exact `model` name |
| `type` missing error | Every provider needs `"type"`: `anthropic`, `openai` or `ollama` |

Other providers (Anthropic, OpenRouter, Ollama, ...) are covered in the [README](../README.md#providers).
