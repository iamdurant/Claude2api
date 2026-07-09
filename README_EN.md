# claude2api

`claude2api` is a Go/Gin proxy that exposes OpenAI-compatible and Anthropic-compatible HTTP endpoints backed by the claude.ai web API.

It reverse-proxies requests to `https://claude.ai` using a browser-like TLS/client environment and returns standard JSON or Server-Sent Events (SSE) responses for common API clients.

## Features

- OpenAI-compatible chat completions: `POST /v1/chat/completions`
- Anthropic Messages-compatible endpoint: `POST /v1/messages`
- OpenAI Responses-compatible endpoint: `POST /v1/responses`
- Model listing: `GET /v1/models`
- Streaming and non-streaming responses
- Browser Cookie mode to match a real claude.ai browser session
- Bearer session key mode for simple local use

Image endpoints such as `/v1/images/generations`, `/v1/images/edits`, and `/v1/images/variations` are not supported.

## Supported Models

Only these model IDs are accepted and returned by `/v1/models`:

- `claude-fable-5`
- `claude-opus-4-8`
- `claude-haiku-4-5`
- `claude-opus-4-7`
- `claude-opus-4-6`
- `claude-opus-3`
- `claude-sonnet-4-6`
- `claude-sonnet-5`

Requests using any other model return an `invalid_request_error`.

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
| `CLAUDE_SESSION_KEY` | empty | Optional claude.ai `sessionKey`. If set, API requests do not need a Bearer token. |
| `CLAUDE_COOKIE` | empty | Optional full browser Cookie header from claude.ai. Recommended when you want behavior closest to the browser. |
| `CLAUDE_TIMEZONE` | `Asia/Singapore` | Timezone sent to claude.ai completion requests. |
| `CLAUDE_LOCALE` | `en-US` | Locale sent to claude.ai completion requests. |
| `DEFAULT_MODEL` | `claude-sonnet-5` | Model used when a request omits `model`. Must be one of the supported models. |

## Authentication

Every `/v1/*` endpoint requires either a session key or a full browser Cookie.

### Option 1: Bearer session key

```http
Authorization: Bearer <claude.ai sessionKey>
```

You can also set `CLAUDE_SESSION_KEY` so callers do not need to pass the Bearer header on each request.

### Option 2: Full browser Cookie

```http
X-Claude-Cookie: <full Cookie header copied from claude.ai>
```

This mode is closest to browser behavior. The proxy reuses values such as `sessionKey`, `sessionKeyLC`, `anthropic-device-id`, `lastActiveOrg`, routing and Cloudflare cookies if they are present.

You can also set `CLAUDE_COOKIE` to use the same browser Cookie for all requests.

## Run

Bearer session key mode:

```bash
CLAUDE_SESSION_KEY='your-session-key' PORT=8080 ./claude2api.exe
```

Full browser Cookie mode:

```bash
CLAUDE_COOKIE='sessionKey=...; sessionKeyLC=...; anthropic-device-id=...; ...' PORT=8080 ./claude2api.exe
```

Then use the local base URL:

```text
http://127.0.0.1:8080/v1
```

## Docker

Build the image:

```bash
docker build -t claude2api .
```

Run with a session key:

```bash
docker run --rm -p 8080:8080 \
  -e CLAUDE_SESSION_KEY='your-session-key' \
  claude2api
```

Or run with Docker Compose:

```bash
CLAUDE_SESSION_KEY='your-session-key' docker compose up --build
```

## Quick Test

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H 'Authorization: Bearer your-session-key' \
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

- The proxy creates a temporary claude.ai conversation for each completion and deletes it after the request.
- Token usage values are approximate and currently based on output text length.
- If browser and Bearer modes behave differently, prefer full Cookie mode because it carries the same request environment as the browser.
- A `429` response is returned by claude.ai when the upstream account/session is rate-limited.
