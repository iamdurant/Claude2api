# claude2api

`claude2api` is a Go/Gin proxy that exposes OpenAI-compatible and Anthropic-compatible HTTP endpoints backed by the claude.ai web API.

It reverse-proxies requests to `https://claude.ai` using a browser-like TLS/client environment and returns standard JSON or Server-Sent Events (SSE) responses for common API clients.

## Features

- OpenAI-compatible chat completions: `POST /v1/chat/completions`
- Anthropic Messages-compatible endpoint: `POST /v1/messages`
- OpenAI Responses-compatible endpoint: `POST /v1/responses`
- Model listing: `GET /v1/models`
- Streaming and non-streaming responses
- Persistent `conversation_id` mode for multi-turn conversation continuity
- Browser Cookie mode to match a real claude.ai browser session
- Multi-account pool via `accounts.txt`, routed by the lowest active request count
- Account-scoped TLS client, CookieJar, browser identity, and organization cache reuse
- Persistent conversations isolated by account, with concurrent turns serialized per `conversation_id`
- Separate proxy API key with server-side Claude credentials
- Dedicated local `tlsclient` module wrapping the Chrome-profile `github.com/bogdanfinn/tls-client` client, CookieJar, and common browser headers
- Completion requests include the claude.ai web `tools` payload reverse-engineered from a real browser request
- Referer is set dynamically to `/new` or `/chat/<conversation_id>` depending on the upstream request phase
- Datadog/RUM cookies and trace headers are generated following the browser SDK field structure
- In Bearer mode, the server generates frontend-like browser cookies/headers where possible; signed or Cloudflare cookies are not forged or sent

Image endpoints such as `/v1/images/generations`, `/v1/images/edits`, and `/v1/images/variations` are not supported.

## Supported Models

`GET /v1/models` reads `claude_ai_available_models.models[].model_id` from `/edge-api/bootstrap/{org_id}/app_start` on every request, using the selected account's browser credentials instead of a fixed model list.

IDs are deduplicated in upstream order. Upstream failures or responses without models return `502`; there is no static fallback. With multiple accounts, the response belongs to the selected server-side account; callers cannot select an upstream account. Completion model IDs are passed through for upstream validation; omitted IDs still use `DEFAULT_MODEL`.

The new HAR confirms a 200 response for this read-only GET. Requests use the captured query `statsig_hashing_algorithm=djb2&growthbook_format=sdk&cache_bust=1&include_system_prompts=false` and never change the model selector. This is a web model catalog, including models requiring higher subscription tiers; listing a model does not guarantee the account can use it.

## Requirements

- Go 1.26.4 or newer, matching `go.mod`
- A valid claude.ai browser session

## Build

```bash
go build -o claude2api.exe .
```

On non-Windows systems you can build without the `.exe` suffix:

```bash
go build -o claude2api .
```

## Configuration

The service is configured with environment variables.

| Variable | Default | Description |
| --- | --- | --- |
| `PORT` | `8080` | Local HTTP server port. |
| `CLAUDE_BASE_URL` | `https://claude.ai` | Upstream claude.ai base URL. |
| `PROXY_API_KEY` | required | Independent API key used by clients to call the proxy. |
| `CLAUDE_SESSION_KEY` | empty | Server-side claude.ai `sessionKey`; it is never used as the proxy API key. |
| `CLAUDE_COOKIE` | empty | Server-side full browser Cookie header from claude.ai. |
| `CLAUDE_ACCOUNTS_FILE` | `accounts.txt` | Multi-account file; one session key or full Cookie header per line. |
| `ACCOUNT_RELOAD_INTERVAL` | `5s` | Polling interval for account-file reloads. |
| `ACCOUNT_RATE_LIMIT_COOLDOWN` | `60s` | Fallback account cooldown when upstream 429 has no `Retry-After`. |
| `CLAUDE_TIMEZONE` | `Asia/Singapore` | Timezone sent to claude.ai completion requests. |
| `CLAUDE_LOCALE` | `en-US` | Locale sent to claude.ai completion requests. |
| `DEFAULT_MODEL` | `claude-sonnet-5` | Model used when a request omits `model`. Must be one of the supported models. |

## Authentication

Every `/v1/*` endpoint requires the proxy API key.

```http
Authorization: Bearer <PROXY_API_KEY>
```

Claude `sessionKey`, full Cookie values, and `accounts.txt` entries are server-side credentials. A caller-provided `X-Claude-Cookie` does not override them.

When the server uses a session key, it generates browser-like values such as:

- `sessionKeyLC`
- `anthropic-device-id`
- `activitySessionId`
- `ajs_anonymous_id`
- `__ssid`
- `_dd_s`
- `traceparent` / Datadog RUM headers
- selected UI / analytics cookies

## Run

Server-side session key configuration:

```bash
PROXY_API_KEY='your-proxy-key' CLAUDE_SESSION_KEY='your-session-key' PORT=8080 ./claude2api.exe
```

Full browser Cookie mode:

```bash
PROXY_API_KEY='your-proxy-key' CLAUDE_COOKIE='sessionKey=...; sessionKeyLC=...; anthropic-device-id=...; ...' PORT=8080 ./claude2api.exe
```

Multi-account mode: create `accounts.txt` in the working directory, with one session key or full Cookie header per line. Blank lines and `#` comments are ignored:

```text
sk-ant-sid01-...
sessionKey=sk-ant-sid02-...; sessionKeyLC=...; anthropic-device-id=...; ...
```

Use `CLAUDE_ACCOUNTS_FILE` to select another path. All `/v1/*` requests use the proxy API key, while upstream Claude credentials remain server-side. Account-file changes are loaded within the default 5-second polling interval; removed accounts stop receiving new requests.

Then use the local base URL:

```text
http://127.0.0.1:8080/v1
```

## Docker

GitHub Actions automatically builds and pushes Docker images to GitHub Container Registry:

```text
ghcr.io/aurora-develop/claude2api
```

Pull the image:

```bash
docker pull ghcr.io/aurora-develop/claude2api:latest
```

Run the published image:

```bash
docker run --rm -p 8080:8080 \
  -e PROXY_API_KEY='your-proxy-key' \
  -e CLAUDE_SESSION_KEY='your-session-key' \
  ghcr.io/aurora-develop/claude2api:latest
```

Build the image locally:

```bash
docker build -t claude2api .
```

Run with a session key:

```bash
docker run --rm -p 8080:8080 \
  -e PROXY_API_KEY='your-proxy-key' \
  -e CLAUDE_SESSION_KEY='your-session-key' \
  claude2api
```

Or run with Docker Compose:

```bash
PROXY_API_KEY='your-proxy-key' CLAUDE_SESSION_KEY='your-session-key' docker compose up --build
```

## Quick Test

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H 'Authorization: Bearer your-proxy-key' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "claude-sonnet-5",
    "messages": [{"role": "user", "content": "Reply with exactly: pong"}],
    "stream": false
  }'
```

Expected response shape:

```json
{
  "id": "chatcmpl-...",
  "object": "chat.completion",
  "model": "claude-sonnet-5",
  "choices": [
    {
      "index": 0,
      "message": {"role": "assistant", "content": "pong"},
      "finish_reason": "stop"
    }
  ]
}
```

## API Documentation

See [API_EN.md](API_EN.md) for endpoint details and examples.

## Notes

- Without `conversation_id`, the proxy creates a temporary claude.ai conversation for each completion and asynchronously deletes it after the response.
- Token usage values are approximate and currently based on output text length.
- When claude.ai returns `429`, that account enters cooldown and is temporarily removed from scheduling; other accounts can serve stateless requests.
- If every account is cooling down, the proxy returns `429` with `Retry-After`; clients should retry after that delay.
