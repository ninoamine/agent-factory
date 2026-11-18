# Config files

Directory: `$AGENT_FACTORY_HOME` if set, else `~/.agent-factory/`.

| File | Owner | Tool may read | Tool may write |
|---|---|---|---|
| `models.json` | user | yes | NEVER |
| `strategies.json` | user | yes | NEVER |
| `models-metadata.json` | tool | yes | yes, atomically |
| `history.jsonl` | tool | yes | yes (append; atomic rewrite on delete/clear) |

- FILE-1 API keys live only in `models.json` (plain text, V1). Recommend `chmod 600`.
- FILE-2 Atomic write = write temp file in same dir, then rename.
- FILE-3 `models-metadata.json` and `history.jsonl` never contain API keys.

## Illustrative shapes (final schemas decided at implementation, D-009; provider and model fields: PROV-1..9)

`models.json`

```json
{
  "providers": { "provider-a": { "type": "openai", "baseUrl": "https://api.example.com/v1", "apiKey": "sk-..." } },
  "models": [
    { "id": "provider-a/model-x", "provider": "provider-a", "model": "model-x",
      "metadata": { "capabilities": ["code"], "speed": "fast" } }
  ]
}
```

`strategies.json`

```json
{
  "strategy": "capability",
  "commands": {
    "/fast": { "enabled": true, "models": ["provider-a/model-x"] },
    "/code": { "enabled": true, "models": [] }
  }
}
```

`history.jsonl` (one JSON object per line)

```json
{"id":"...","timestamp":"...","command":"/code","model":"provider-a/model-x","inputTokens":120,"outputTokens":340,"status":"success"}
```
