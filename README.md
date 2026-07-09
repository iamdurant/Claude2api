# Claude2api

> 中文文档 | [English README](README_EN.md) | [API 文档](API.md) | [English API](API_EN.md)

`Claude2api` 是一个基于 Go + Gin 的 claude.ai 网页 API 代理服务，可以把 claude.ai 网页会话包装成常见的 OpenAI / Anthropic 兼容接口。

项目会使用接近真实浏览器的 TLS / Header / Cookie 环境访问 `https://claude.ai`，并对外提供 JSON 与 SSE 流式响应。

## 功能特性

- OpenAI Chat Completions 兼容接口：`POST /v1/chat/completions`
- Anthropic Messages 兼容接口：`POST /v1/messages`
- OpenAI Responses 兼容接口：`POST /v1/responses`
- 模型列表接口：`GET /v1/models`
- 支持非流式与 SSE 流式返回
- 支持完整浏览器 Cookie 模式，更接近 claude.ai 浏览器请求环境
- 支持 Bearer sessionKey 模式，便于本地简单调用
- 使用 `github.com/bogdanfinn/tls-client` 的 Chrome 指纹请求上游
- 支持 Docker / Docker Compose 部署

## 支持的模型

`/v1/models` 只会返回以下模型，请求时也只允许使用这些模型：

- `claude-fable-5`
- `claude-opus-4-8`
- `claude-haiku-4-5`
- `claude-opus-4-7`
- `claude-opus-4-6`
- `claude-opus-3`
- `claude-sonnet-4-6`
- `claude-sonnet-5`

使用其他模型会返回 `invalid_request_error`。

## 环境要求

- Go 1.26.4 或更高版本
- 一个可用的 claude.ai 浏览器会话
- 可选：Docker / Docker Compose

## 快速开始

### 1. 构建

Windows：

```bash
go build -o claude2api.exe .
```

Linux / macOS：

```bash
go build -o claude2api .
```

### 2. 运行

使用 sessionKey：

```bash
CLAUDE_SESSION_KEY='你的-sessionKey' PORT=8080 ./claude2api.exe
```

使用完整浏览器 Cookie：

```bash
CLAUDE_COOKIE='sessionKey=...; sessionKeyLC=...; anthropic-device-id=...; ...' PORT=8080 ./claude2api.exe
```

服务地址：

```text
http://127.0.0.1:8080/v1
```

### 3. 测试

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H 'Authorization: Bearer 你的-sessionKey' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "claude-sonnet-5",
    "messages": [{"role": "user", "content": "Reply with exactly: pong"}],
    "stream": false
  }'
```

预期返回格式：

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

## Docker 部署

GitHub Actions 会自动构建 Docker 镜像并推送到 GitHub Container Registry：

```text
ghcr.io/aurora-develop/claude2api
```

拉取镜像：

```bash
docker pull ghcr.io/aurora-develop/claude2api:latest
```

运行远程镜像：

```bash
docker run --rm -p 8080:8080 \
  -e CLAUDE_SESSION_KEY='你的-sessionKey' \
  ghcr.io/aurora-develop/claude2api:latest
```

### Docker build

```bash
docker build -t claude2api .
```

### Docker run

使用 sessionKey：

```bash
docker run --rm -p 8080:8080 \
  -e CLAUDE_SESSION_KEY='你的-sessionKey' \
  claude2api
```

使用完整 Cookie：

```bash
docker run --rm -p 8080:8080 \
  -e CLAUDE_COOKIE='sessionKey=...; sessionKeyLC=...; anthropic-device-id=...; ...' \
  claude2api
```

### Docker Compose

```bash
CLAUDE_SESSION_KEY='你的-sessionKey' docker compose up --build
```

或者：

```bash
CLAUDE_COOKIE='sessionKey=...; sessionKeyLC=...; anthropic-device-id=...; ...' docker compose up --build
```

## 配置项

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PORT` | `8080` | 本地 HTTP 服务端口。 |
| `CLAUDE_BASE_URL` | `https://claude.ai` | claude.ai 上游地址。 |
| `CLAUDE_SESSION_KEY` | 空 | claude.ai 的 `sessionKey`。配置后请求端可以不传 Bearer。 |
| `CLAUDE_COOKIE` | 空 | 从浏览器复制的完整 claude.ai Cookie。推荐用于更接近浏览器环境。 |
| `CLAUDE_TIMEZONE` | `Asia/Singapore` | 发送给 claude.ai 的时区。 |
| `CLAUDE_LOCALE` | `en-US` | 发送给 claude.ai 的语言区域。 |
| `DEFAULT_MODEL` | `claude-sonnet-5` | 请求未指定模型时使用的默认模型。必须在支持模型列表内。 |

## 认证方式

所有 `/v1/*` 接口都需要认证。

### 方式一：Bearer sessionKey

```http
Authorization: Bearer <claude.ai sessionKey>
```

如果服务端已经配置 `CLAUDE_SESSION_KEY`，请求端可以不传这个 Header。

### 方式二：完整浏览器 Cookie

```http
X-Claude-Cookie: <从 claude.ai 浏览器请求中复制的完整 Cookie>
```

这种方式最接近浏览器行为。代理会复用 Cookie 中的：

- `sessionKey`
- `sessionKeyLC`
- `anthropic-device-id`
- `lastActiveOrg`
- `routingHint`
- Cloudflare 相关 Cookie

如果服务端已经配置 `CLAUDE_COOKIE`，请求端可以不传 `X-Claude-Cookie`。

## API 文档

详细接口说明见：[API.md](API.md)。

英文版：

- [English README](README_EN.md)
- [English API](API_EN.md)

## 注意事项

- 每次 completion 请求都会创建一个临时 claude.ai 会话，请求结束后会尝试删除。
- `usage` 中的 token 数是近似值，目前主要根据输出文本长度估算。
- 如果 Bearer sessionKey 模式和浏览器行为不一致，建议使用完整 Cookie 模式。
- 如果上游返回 `429`，说明 claude.ai 当前账号或会话触发了速率限制。
- 请不要把自己的 `sessionKey`、完整 Cookie、抓包文件提交到公开仓库。