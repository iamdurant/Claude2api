# Secure Account Pool Design

## Goal

Make the proxy suitable for server deployment without changing the existing OpenAI/Anthropic-compatible request shapes. The proxy must authenticate callers with a credential that is independent from Claude credentials, reload configured accounts without a restart, and avoid routing requests to an account during an upstream rate-limit cooldown.

## Scope

- Add a required `PROXY_API_KEY` used only to authenticate callers to `/v1/*`.
- Stop accepting request Bearer tokens or `X-Claude-Cookie` values as upstream Claude credentials. Claude credentials come only from server-side environment variables and `accounts.txt`.
- Reload the configured account file while the process is running. The file remains the only account-management surface; no HTTP CRUD endpoint is added.
- Detect upstream HTTP 429 responses, honor `Retry-After` when present, and apply a configurable fallback cooldown when it is absent.
- Exclude cooling-down accounts from ordinary scheduling and return a retryable 429 when no eligible account remains.

## Authentication

`Authorization: Bearer <PROXY_API_KEY>` is required for every `/v1/*` route. Missing or invalid keys return 401. The public `/health` endpoint remains unauthenticated.

The middleware always passes the server-configured `CLAUDE_SESSION_KEY` and `CLAUDE_COOKIE` to the handler context. Request credentials never override those values, so callers cannot select or inject an upstream Claude account. Startup fails when `PROXY_API_KEY` is empty or when no Claude account is configured.

## Account hot reload

The process watches the configured accounts path (`CLAUDE_ACCOUNTS_FILE`, default `accounts.txt`) on a short polling interval. A successful file change loads the complete account list using the existing parsing and deduplication rules.

- New credentials create account clients and become eligible for new requests.
- Unchanged credentials retain their existing client, active count, cooldown state, and conversation affinity.
- Removed credentials are excluded from new account selection. Leases already acquired may finish; a conversation still pinned to a removed account returns an account-unavailable error until that credential is re-added.
- A read/parse failure keeps the last known good account set and logs the failure.

The reload loop does not expose credentials over HTTP and does not interrupt in-flight requests.

## Rate-limit cooldown

Each configured account stores an `availableAt` timestamp. Any upstream 429 associated with that account updates it to the later of the current value and:

- the duration/date from `Retry-After`, when valid;
- the configured fallback `ACCOUNT_RATE_LIMIT_COOLDOWN`, defaulting to 60 seconds.

Stateless requests skip accounts whose cooldown has not expired. A request with a pinned `conversation_id` stays on its original account and receives a 429 with `Retry-After` while that account is cooling down; it is never migrated to another account. If every selectable account is cooling down, the proxy returns 429 with the shortest remaining delay. After the delay expires, the account is automatically eligible again.

The Claude client exposes typed upstream status errors so 429 status and retry metadata survive wrapping through organization lookup, model listing, conversation creation/deletion, and message sending.

## Configuration

- `PROXY_API_KEY`: required proxy credential.
- `CLAUDE_ACCOUNTS_FILE`: account file path, default `accounts.txt`.
- `ACCOUNT_RATE_LIMIT_COOLDOWN`: fallback 429 cooldown, default `60s`.
- `ACCOUNT_RELOAD_INTERVAL`: account file polling interval, default `5s`.

## Verification

Tests will cover proxy-key acceptance/rejection, prevention of request credential override, account reload behavior, removed-account routing, Retry-After parsing, cooldown exclusion/recovery, pinned-conversation behavior, and the all-accounts-cooling response.
