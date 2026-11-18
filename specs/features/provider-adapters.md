---
id: PROV
status: implemented
version: V1
depends_on: [CFG, META, EXEC]
modules: [internal/infra/providers, internal/app/configuration, cmd/agent-factory]
---
# Provider adapters

Goal: ship built-in adapters so a routed request really reaches an Anthropic, OpenAI-compatible, or Ollama model, streams its answer, reports usage, and fails with a readable message.

## Rules

### Configuration (provider and model entries in `models.json`)

- PROV-1 Each provider entry MUST have a `type` field with one of `anthropic`, `openai`, `ollama`. A missing or unknown `type` MUST be a validation problem naming the field path (CFG-5). The tool MUST NOT infer the type from the provider name (D-019).
- PROV-2 A provider entry MAY have `baseUrl`. If absent, the default MUST be: `anthropic` `https://api.anthropic.com`, `openai` `https://api.openai.com/v1`, `ollama` `http://localhost:11434`.
- PROV-3 `baseUrl` MUST be an absolute `http` or `https` URL with a host and no query or fragment. Otherwise it is a validation problem (CFG-5). A trailing `/` MUST be ignored.
- PROV-4 `apiKey` MUST be optional for every type. An empty or absent key MUST NOT be a validation problem.
- PROV-5 SHOULD warn at load when a provider of type `anthropic` has no `apiKey`, or when the `openai` type uses the default `baseUrl` and has no `apiKey`.
- PROV-6 SHOULD warn at load when `apiKey` is set and `baseUrl` is `http` with a host that is not loopback (`localhost`, `127.0.0.0/8`, `::1`): the key would be sent unencrypted. The warning MUST NOT contain the key (SEC-1).
- PROV-7 A provider entry MAY have `timeoutSeconds`: an integer from 1 to 3600. Other values are a validation problem (CFG-5). It replaces the default for both timeouts in PROV-20.
- PROV-8 A model entry MAY have `model`: the exact model name sent to the provider API. If absent, the model `id` MUST be sent unchanged. An empty string is a validation problem.
- PROV-9 A model entry MAY have `maxOutputTokens`: a positive integer. Other values are a validation problem (CFG-5).

### Wiring

- PROV-10 At startup and after each successful `/refresh` (REFRESH-3), MUST register one execution adapter and one metadata adapter per provider entry of the active configuration, built from that entry's `type`, `baseUrl`, `apiKey` and `timeoutSeconds`. Adapters from the previous configuration MUST NOT be used afterwards.
- PROV-11 Adapters MUST use plain `net/http` and the standard library only. No provider SDK (D-017, DEP-1). Types from adapter code MUST NOT leave `internal/infra/providers` (ARCH-9).
- PROV-12 Every execution adapter MUST implement streaming (EXEC-2) and provider cancel (EXEC-3).

### Requests

- PROV-13 `anthropic` MUST send `POST {baseUrl}/v1/messages` with headers `x-api-key` (only when `apiKey` is set), `anthropic-version: 2023-06-01`, `content-type: application/json`, and body `{"model", "max_tokens", "stream": true, "messages": [{"role": "user", "content": <prompt>}]}`.
- PROV-14 `anthropic` MUST send `max_tokens` = the model's `maxOutputTokens` if set, else `8192`.
- PROV-15 `openai` MUST send `POST {baseUrl}/chat/completions` with header `Authorization: Bearer <apiKey>` (only when `apiKey` is set) and body `{"model", "stream": true, "stream_options": {"include_usage": true}, "messages": [{"role": "user", "content": <prompt>}]}`, plus `max_tokens` only when `maxOutputTokens` is set. If a server rejects `stream_options`, the request fails as PROV-28; it MUST NOT be re-sent without it (EXEC-4).
- PROV-16 `ollama` MUST send `POST {baseUrl}/api/chat` with body `{"model", "stream": true, "messages": [{"role": "user", "content": <prompt>}]}`, plus `options.num_predict` only when `maxOutputTokens` is set. It MUST send `Authorization: Bearer <apiKey>` only when `apiKey` is set.
- PROV-17 Each request MUST contain exactly one user message with the stripped prompt and nothing else: no system prompt, no earlier messages (EXEC-7, D-008).
- PROV-18 Requests MUST send `User-Agent: agent-factory/<version>` (DOC-11).
- PROV-19 The API key MUST be sent only in a request header to the configured `baseUrl`. It MUST NOT appear in a URL. Adapters MUST NOT follow HTTP redirects; a `3xx` response is a failure (PROV-28).
- PROV-20 A request MUST fail with the timeout message (PROV-29) when no response headers arrive within the first-byte timeout, or no stream data arrives within the idle timeout after that. Both default to 60 seconds (or `timeoutSeconds`, PROV-7). There MUST NOT be a cap on total request duration.
- PROV-21 SHOULD honor the standard `HTTPS_PROXY`, `HTTP_PROXY` and `NO_PROXY` environment variables.

### Streaming and usage

- PROV-22 `anthropic` MUST read the SSE stream: each `content_block_delta` with a `text_delta` is one chunk. `input_tokens` comes from `message_start`, `output_tokens` from the last `message_delta`. Other event and delta types (for example thinking) MUST be ignored. An `error` event MUST end the request as a failure (PROV-28).
- PROV-23 `openai` MUST read the SSE stream: each non-empty `choices[0].delta.content` is one chunk; `data: [DONE]` ends the stream. Usage comes from `usage.prompt_tokens`, `usage.completion_tokens`, `usage.total_tokens` when a chunk includes them.
- PROV-24 `ollama` MUST read the NDJSON stream: each non-empty `message.content` is one chunk. The line with `"done": true` ends the stream; usage comes from its `prompt_eval_count` and `eval_count`. A line with an `error` field MUST end the request as a failure (PROV-28).
- PROV-25 Total tokens MUST be the provider's total when given, else input + output when both are known, else unavailable. A count the provider did not return MUST stay unavailable (EXEC-5).
- PROV-26 A stream that ends before its end marker (`message_stop`, `[DONE]`, `done: true`), or with a line that cannot be decoded, MUST be a failure (PROV-28). Text already shown stays on screen.
- PROV-27 Cancelling the request context MUST close the HTTP connection. `Cancel()` MUST return nil once the connection is closed, so no "could not be cancelled" notice appears (EXEC-3).

### Error mapping

- PROV-28 Adapter errors MUST map to the existing kinds only (EXEC-4, D-018), with a readable reason that names the provider:

  | Condition | Kind | Reason (shape) |
  |---|---|---|
  | HTTP 401, 403 | `auth` | `provider "<name>" rejected the API key (HTTP <n>); check providers.<name>.apiKey in models.json` |
  | HTTP 404, or the provider says the model does not exist | `model_unavailable` | `provider "<name>" has no model "<model>"`; for `ollama` add `run: ollama pull <model>` |
  | HTTP 429 | `provider_failure` | `rate limited by "<name>" (HTTP 429)`, plus `retry after <Retry-After>` when the header is present |
  | HTTP 5xx, 529 (Anthropic overloaded) | `provider_failure` | `"<name>" returned HTTP <n>: <provider message>` |
  | other HTTP 4xx, 3xx | `provider_failure` | `"<name>" returned HTTP <n>: <provider message>` |
  | connection refused, DNS, TLS error | `provider_failure` | `cannot reach "<name>" at <baseUrl>: <cause>`; for `ollama` add `is Ollama running?` |
  | `error` event or line in a stream, PROV-26 | `provider_failure` | `"<name>" stream failed: <provider message or "ended early">` |
  | context cancelled | `interrupted` | (EXEC-3) |

- PROV-29 A timeout from PROV-20 MUST be `provider_failure` with reason `no response from "<name>" for <n>s`.
- PROV-30 The provider message MUST be taken from the response's error field when one is present (Anthropic `error.message`, OpenAI `error.message`, Ollama `error`), else from the raw body. It MUST be cut to 300 characters, and control characters other than space (ANSI escapes included) MUST be removed before display.
- PROV-31 Adapters MUST read at most 64 KiB of an error response body.
- PROV-32 Error reasons MUST NOT contain the API key, even when the provider echoes it back (SEC-1).

### Metadata (META-2)

- PROV-33 `anthropic` metadata MUST call `GET {baseUrl}/v1/models/{model}`. Success returns empty API metadata (the endpoint has no capabilities, speed, or pricing; META-4).
- PROV-34 `openai` metadata MUST call `GET {baseUrl}/models` and look for the model name. Absent from the list is a fetch failure (META-8). If the list entry has `pricing.prompt` and `pricing.completion` as non-negative decimal strings (USD per token, as OpenRouter returns), MUST set `inputPer1M` and `outputPer1M` to those values × 1,000,000. Otherwise pricing MUST stay unknown.
- PROV-35 `openai` metadata SHOULD call `GET {baseUrl}/models` at most once per provider per load or refresh.
- PROV-36 `ollama` metadata MUST call `POST {baseUrl}/api/show` with `{"model": <model>}`. Success returns empty API metadata.
- PROV-37 Metadata adapters MUST NOT return capabilities or speed: no built-in provider returns them in our vocabulary (META-4). A metadata failure uses the PROV-28 reasons and follows META-8.
- PROV-38 Metadata fetches MUST use the timeouts from PROV-20 (META-8 also applies its own bound).

## Clarifications

- Example provider entries:

  ```json
  "providers": {
    "anthropic":  { "type": "anthropic", "apiKey": "sk-ant-..." },
    "openrouter": { "type": "openai", "baseUrl": "https://openrouter.ai/api/v1", "apiKey": "sk-or-..." },
    "lmstudio":   { "type": "openai", "baseUrl": "http://localhost:1234/v1" },
    "local":      { "type": "ollama", "timeoutSeconds": 300 }
  },
  "models": [
    { "id": "or/sonnet", "provider": "openrouter", "model": "anthropic/claude-sonnet-5-5" },
    { "id": "qwen", "provider": "local", "model": "qwen3:32b", "maxOutputTokens": 4096 }
  ]
  ```

- `type: openai` covers OpenAI, OpenRouter, Groq, Mistral, DeepSeek, Together, vLLM, LM Studio and any server that implements `/chat/completions` with SSE streaming. Provider-specific features are out of scope.
- "Model name" in PROV-13..16 and PROV-33..36 means the `model` field (PROV-8), else the `id`.
- Unknown fields stay rejected (CONV-5); `type`, `baseUrl`, `timeoutSeconds`, `model`, `maxOutputTokens` become known fields.
- Provider responses are untrusted (SEC-3): decode into typed structs; ignore unknown fields in provider JSON (providers add fields often), but reject wrong types for the fields we read.

## Out of scope

- Retries, backoff, fallback to another model or provider (ARCH-6).
- Tools/function calling, images, files, JSON mode, temperature or other sampling options.
- System prompts, multi-turn messages (D-008).
- Provider-native features beyond the three APIs above (Bedrock, Vertex, Azure-specific auth, Gemini native API).
- Reading API keys from environment variables or keychains.
- Automatic model discovery from `/models` listings (roadmap).
- Cost display (EXEC-6).

## Acceptance criteria

All tests use `httptest` servers; no network, no real keys (TEST-2, ARCH-8).

- AC1 (PROV-1): provider `{"apiKey": "x"}` with no `type` produces the problem `models.json providers.p.type: must be one of anthropic, openai, ollama`; the app still starts.
- AC2 (PROV-2,3): `type: ollama` with no `baseUrl` sends requests to `http://localhost:11434`; `baseUrl: "ftp://h"` and `baseUrl: "https://h/v1?x=1"` are validation problems; `https://h/v1/` sends to `https://h/v1/chat/completions`.
- AC3 (PROV-5): `type: anthropic` without `apiKey` loads with a warning; the model is still listed.
- AC4 (PROV-6): `apiKey: "sk-secret"` with `baseUrl: "http://10.0.0.5:8000/v1"` warns about unencrypted sending and the output contains no `sk-secret`; with `http://localhost:8000/v1` there is no warning.
- AC5 (PROV-7,20,29): `timeoutSeconds: 1` and a fake server that sends headers, one chunk, then stalls: the chunk is shown, then the error `provider_failure: no response from "p" for 1s`. A fake that sends a chunk every 0.5s for 3s finishes successfully.
- AC6 (PROV-8): model `{"id": "or/sonnet", "model": "anthropic/claude-x"}` sends `"model": "anthropic/claude-x"`; a model without `model` sends its `id`.
- AC7 (PROV-13,14,17): a fake Anthropic server receives exactly the documented path, headers, `max_tokens: 8192`, and a single user message equal to the stripped prompt; with `maxOutputTokens: 1000` it receives `1000`.
- AC8 (PROV-15): the fake OpenAI server receives `stream_options.include_usage: true`, no `max_tokens` when unset, and no `Authorization` header when `apiKey` is absent.
- AC9 (PROV-22,25): a recorded Anthropic stream (`message_start` input 12, two text deltas "Hel" "lo", a thinking delta, `message_delta` output 5, `message_stop`) shows "Hello" in two chunks and usage 12/5/17.
- AC10 (PROV-23,25): an OpenAI stream with no usage chunk shows the text and usage "unavailable"; with a usage chunk 3/4/7 shows 3/4/7.
- AC11 (PROV-24): an Ollama NDJSON stream ending `{"done":true,"prompt_eval_count":8,"eval_count":2}` shows usage 8/2/10.
- AC12 (PROV-26): an OpenAI stream closed before `[DONE]` gives `provider_failure: "p" stream failed: ended early`, and the text received so far stays visible.
- AC13 (PROV-27): Ctrl+C during a fake stream closes the server-side connection (the handler sees its request context done), the result is `interrupted`, and no "could not be cancelled" notice appears.
- AC14 (PROV-28): fake responses 401, 404, 429 with `Retry-After: 20`, 500, 529 and a refused connection map to `auth`, `model_unavailable`, `provider_failure` (with `retry after 20`), `provider_failure` ×2, `provider_failure: cannot reach ...`. Each request reaches the server exactly once (no retry).
- AC15 (PROV-19): a fake that answers 302 to another host: the second host receives no request and the result is `provider_failure`.
- AC16 (PROV-30,31,32): an error body of 1 MiB containing `\x1b[31m` and the API key: the displayed reason is at most 300 characters, has no ESC character, and does not contain the key.
- AC17 (PROV-33,36,37): metadata for an existing Anthropic or Ollama model succeeds with empty capabilities, speed, pricing; a 404 gives the META-8 warning naming the model.
- AC18 (PROV-34,35): an OpenRouter-shaped list with `pricing: {"prompt": "0.000003", "completion": "0.000015"}` gives `inputPer1M: 3`, `outputPer1M: 15`; a list without pricing gives unknown pricing; two models on the same provider cause one `GET /models`.
- AC19 (PROV-10): after `/refresh` changes a provider's `baseUrl` from fake server A to fake server B, the next request reaches B only.
- AC20 (PROV-11): `go list -deps ./...` shows no provider SDK module.
