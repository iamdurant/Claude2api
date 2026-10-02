# Secure Account Pool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Require a separate proxy API key, hot-reload server-side Claude accounts, and automatically avoid accounts during upstream 429 cooldowns.

**Architecture:** The auth middleware validates `PROXY_API_KEY` and always supplies only server-configured Claude credentials to the handlers. The account pool keeps stable account objects keyed by credential identity, so reloads can preserve clients, active counts, conversation affinity, and cooldown timestamps. Typed upstream HTTP errors carry status and `Retry-After` through the Claude client; the pool converts 429s into per-account cooldown state and the handlers return retryable 429 responses when no account is eligible.

**Tech Stack:** Go, Gin, `sync/atomic`, `httptest`, existing `go test ./...` suite, Docker Compose.

---

## File Map

- Modify `config/config.go`: add proxy key, account-file path, reload interval, cooldown duration, exported account loading, and duration parsing.
- Modify `middleware/auth.go`: validate the proxy Bearer key and remove request credentials from upstream account selection.
- Modify `middleware/auth_test.go`: cover valid, missing, invalid, and non-overriding proxy authentication.
- Modify `config/config_test.go`: cover defaults and duration configuration while preserving account parsing tests.
- Create `claude/errors.go`: typed upstream HTTP status errors and `Retry-After` parsing.
- Create `claude/errors_test.go`: test seconds, HTTP-date, invalid, and missing retry metadata.
- Modify `claude/client.go` and `claude/models.go`: return typed errors for all non-success upstream responses.
- Modify `handlers/client_pool.go`: stable account registry, reload replacement, cooldown tracking, and eligible-account selection.
- Modify `handlers/client_pool_test.go`: cover reload, cooldown skip/recovery, pinned conversations, and all-account cooldown behavior.
- Modify `handlers/common.go`, `handlers/chat.go`, `handlers/anthropic.go`, `handlers/responses.go`, and `handlers/conversations.go`: observe upstream rate limits and map pool availability errors to 429/503 responses.
- Modify `handlers/models_test.go`: use the proxy key and verify configured credentials, not request Bearer credentials, select the upstream account.
- Modify `main.go`: fail closed without proxy/upstream configuration and start the account-file reload loop.
- Modify `docker-compose.yml`, `README.md`, `README_EN.md`, `API.md`, and `API_EN.md`: document the new key, server-side credentials, hot reload, cooldown behavior, and deployment variables.

### Task 1: Lock Down Proxy Authentication

**Files:**
- Test: `middleware/auth_test.go`
- Test: `config/config_test.go`
- Modify: `middleware/auth.go`
- Modify: `config/config.go`

- [ ] **Step 1: Write failing authentication tests.** Replace the old tests that treat `env-token` as an upstream request token with tests for the new contract:

```go
func TestBrowserAuthAcceptsProxyKeyAndKeepsServerCredentials(t *testing.T) {
	r := authTestRouter(BrowserAuth("proxy-secret", "server-session", "server-cookie", true))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer proxy-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != `{"cookie":"server-cookie","explicit":false,"token":"server-session"}` {
		t.Fatalf("unexpected auth result: %d %s", w.Code, w.Body.String())
	}
}

func TestBrowserAuthRejectsMissingOrInvalidProxyKey(t *testing.T) {
	for _, header := range []string{"", "Bearer wrong", "Basic proxy-secret"} {
		r := authTestRouter(BrowserAuth("proxy-secret", "server-session", "", true))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("header %q: status = %d", header, w.Code)
		}
	}
}

func TestBrowserAuthDoesNotAcceptRequestClaudeCredentials(t *testing.T) {
	r := authTestRouter(BrowserAuth("proxy-secret", "server-session", "server-cookie", true))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer proxy-secret")
	req.Header.Set("X-Claude-Cookie", "sessionKey=request-session")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if strings.Contains(w.Body.String(), "request-session") {
		t.Fatalf("request credential overrode server credential: %s", w.Body.String())
	}
}
```

- [ ] **Step 2: Run the focused tests and verify they fail for the intended API-contract reason.**

Run: `go test ./middleware ./config`

Expected: FAIL to compile because `BrowserAuth` still has the old argument contract and the test context does not yet expose the new proxy-auth behavior.

- [ ] **Step 3: Implement the minimal configuration and middleware changes.** Add these fields to `config.Config`:

```go
ProxyAPIKey              string
AccountsFile             string
AccountReloadInterval    time.Duration
AccountRateLimitCooldown time.Duration
```

Load `PROXY_API_KEY`, default `CLAUDE_ACCOUNTS_FILE` to `accounts.txt`, default `ACCOUNT_RELOAD_INTERVAL` to `5s`, and default `ACCOUNT_RATE_LIMIT_COOLDOWN` to `60s`. Add `LoadAccounts(path, fallbackSessionKey, fallbackCookie) ([]Account, error)` and keep the existing parser semantics; a missing account file is allowed when fallback credentials are present, while other read/scanner errors are returned.

Change the middleware contract to:

```go
func BrowserAuth(proxyAPIKey, envSessionKey, envClaudeCookie string, accountPool ...bool) gin.HandlerFunc
```

Compare the supplied `Authorization: Bearer` value with `proxyAPIKey` using `crypto/subtle.ConstantTimeCompare`. On success, put only `envSessionKey` and `envClaudeCookie` in the context and set `explicitCredentials` to `false`; ignore/reject request `X-Claude-Cookie` instead of using it as an upstream credential. On failure, return 401 with a proxy-key error.

- [ ] **Step 4: Run the focused tests and verify they pass.**

Run: `go test ./middleware ./config`

Expected: PASS.

- [ ] **Step 5: Commit the authentication boundary.**

```bash
git add middleware/auth.go middleware/auth_test.go config/config.go config/config_test.go
git commit -m "feat: require separate proxy api key"
```

### Task 2: Preserve Upstream Status and Retry Metadata

**Files:**
- Test: `claude/errors_test.go`
- Create: `claude/errors.go`
- Modify: `claude/client.go`
- Modify: `claude/models.go`

- [ ] **Step 1: Write failing error tests.** Add tests for `parseRetryAfter` with a numeric value, an HTTP-date based on a fixed `now`, an invalid value, and an empty value. Add a test that `HTTPError` exposes `StatusCode == 429`, `RetryAfter`, and a useful operation/message.

```go
func TestParseRetryAfterSeconds(t *testing.T) {
	d, ok := parseRetryAfter("12", time.Unix(1000, 0))
	if !ok || d != 12*time.Second {
		t.Fatalf("got %v, %v", d, ok)
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	now := time.Unix(1000, 0)
	d, ok := parseRetryAfter(now.Add(25*time.Second).UTC().Format(http.TimeFormat), now)
	if !ok || d != 25*time.Second {
		t.Fatalf("got %v, %v", d, ok)
	}
}

func TestParseRetryAfterRejectsInvalidValue(t *testing.T) {
	if _, ok := parseRetryAfter("later", time.Unix(1000, 0)); ok {
		t.Fatal("invalid Retry-After value was accepted")
	}
}
```

- [ ] **Step 2: Run the error tests and verify they fail because the typed error helpers do not exist.**

Run: `go test ./claude -run 'TestParseRetryAfter|TestHTTPError' -v`

Expected: FAIL to compile with undefined helper/type names.

- [ ] **Step 3: Implement typed status errors.** Create `claude.HTTPError` with `Operation`, `StatusCode`, `RetryAfter`, and `RetryAfterSet` fields. Implement `Error()`, `Unwrap()` where needed, `IsStatus(err, code)`, `RetryAfterOf(err)`, and `parseRetryAfter(value, now)`. Build the error from an HTTP response, read and cap the response body, and preserve the retry duration for status 429.

Update every non-success response path in `GetOrganization`, `CreateConversation`, `DeleteConversation`, `SendMessage`, and `ListModels` to return the typed error with `%w` wrapping. Do not replace a wrapped upstream status error with a generic string.

- [ ] **Step 4: Run the error tests and the existing Claude tests.**

Run: `go test ./claude`

Expected: PASS.

- [ ] **Step 5: Commit typed upstream errors.**

```bash
git add claude/errors.go claude/errors_test.go claude/client.go claude/models.go
git commit -m "feat: preserve upstream rate limit metadata"
```

### Task 3: Add Cooldown-Aware Account Pool and Reload Replacement

**Files:**
- Test: `handlers/client_pool_test.go`
- Modify: `handlers/client_pool.go`

- [ ] **Step 1: Write failing pool tests.** Add tests with short configured cooldowns for:

```go
func TestClientPoolSkipsRateLimitedAccount(t *testing.T)
func TestClientPoolReenablesAccountAfterCooldown(t *testing.T)
func TestClientPoolReturnsShortestRetryAfterWhenAllAccountsAreCooling(t *testing.T)
func TestClientPoolPinnedConversationReturnsRateLimitWhileCooling(t *testing.T)
func TestClientPoolReloadAddsAndRemovesAccounts(t *testing.T)
```

The tests must assert that a cooled account is not selected for a stateless request, that it becomes selectable after its timestamp, that the all-cooled error contains the shortest remaining delay, that a pinned conversation is not migrated, that a reload adds new credentials, and that removed credentials are not selected for new conversations. Also assert that an unchanged account retains the same `*claude.Client` after reload.

- [ ] **Step 2: Run the pool tests and verify they fail for missing cooldown/reload behavior.**

Run: `go test ./handlers -run 'TestClientPool' -v`

Expected: FAIL because the pool has no reload API, cooldown state, or typed unavailable errors.

- [ ] **Step 3: Implement stable account state.** Extend `accountClient` with an atomic configured flag and atomic cooldown deadline. Add a credential-ID map to `clientPool`, plus `rateLimitCooldown time.Duration`.

Implement:

```go
func (p *clientPool) reloadConfigured(accounts []config.Account) (bool, error)
func (p *clientPool) observe(accountID string, err error)
func (p *clientPool) acquireConfigured() (*clientLease, error)
func (p *clientPool) acquireConfiguredConversation(conversationID string) (*clientLease, error)
```

`reloadConfigured` must reuse unchanged account objects and clients, mark removed objects unavailable for new routing, and keep pinned pointers available for re-addition without deleting their affinity. `selectConfigured` must skip cooldown accounts and return a typed rate-limit error with the shortest remaining duration when no account is eligible. A pinned conversation must return that same rate-limit error while its account is cooling and must never migrate.

`observe` must use `claude.RetryAfterOf(err)` for 429 errors and use the configured fallback when no valid upstream duration exists. Update the deadline with an atomic max operation.

- [ ] **Step 4: Update callers of the pool API and run the pool tests.** Adjust `acquire` and helper signatures for returned errors; preserve the existing least-active and conversation-affinity tests.

Run: `go test ./handlers -run 'TestClientPool' -v`

Expected: PASS.

- [ ] **Step 5: Commit cooldown and reload state.**

```bash
git add handlers/client_pool.go handlers/client_pool_test.go
git commit -m "feat: add account cooldown and hot reload state"
```

### Task 4: Return Correct Availability Responses and Observe 429s

**Files:**
- Test: `handlers/client_pool_test.go`
- Test: `handlers/models_test.go`
- Modify: `handlers/common.go`
- Modify: `handlers/chat.go`
- Modify: `handlers/anthropic.go`
- Modify: `handlers/responses.go`
- Modify: `handlers/conversations.go`

- [ ] **Step 1: Write failing response tests.** Add a handler-level test for a pool with all accounts cooling that checks HTTP 429, a positive `Retry-After` header, and an `upstream_error` body. Add a test that a request with the valid proxy key cannot make `/v1/models` use a request Bearer token as the upstream account.

- [ ] **Step 2: Run the focused handler tests and verify they fail.**

Run: `go test ./handlers -run 'Test.*(Cooldown|Proxy|Models)' -v`

Expected: FAIL because acquire errors are currently returned as 500 and handlers do not observe typed upstream 429 errors.

- [ ] **Step 3: Implement shared error mapping and observation.** Add handler error types/helpers that map account cooldown exhaustion to 429 with a rounded-up `Retry-After` seconds value, map removed pinned accounts to 503, and retain 500 for unexpected client construction errors. Call `h.clients.observe(accountID, err)` before returning upstream errors from model listing, conversation deletion, conversation creation, and message sending.

Refactor each API entry point to use the shared acquire-error writer. Keep SSE behavior compatible: once a stream has sent headers, emit the existing error event; before headers, set the retry header when possible.

- [ ] **Step 4: Run all handler tests.**

Run: `go test ./handlers`

Expected: PASS.

- [ ] **Step 5: Commit handler integration.**

```bash
git add handlers/common.go handlers/chat.go handlers/anthropic.go handlers/responses.go handlers/conversations.go handlers/models_test.go handlers/client_pool_test.go
git commit -m "feat: route around rate-limited accounts"
```

### Task 5: Start Fail-Closed Server and Account File Watcher

**Files:**
- Test: `config/config_test.go`
- Modify: `main.go`
- Modify: `handlers/common.go`
- Modify: `config/config.go`

- [ ] **Step 1: Write failing configuration tests.** Test that the duration parser returns the fallback for invalid/empty values and that the configured accounts path is retained. Test `LoadAccounts` returns an error for a non-missing unreadable file while still allowing a missing default file with fallback credentials.

- [ ] **Step 2: Run the focused configuration tests and verify the missing behavior.**

Run: `go test ./config -run 'Test.*(Duration|LoadAccounts|Config)' -v`

Expected: FAIL until the new fields and exported loader are implemented.

- [ ] **Step 3: Implement startup validation and polling reload.** In `main.go`, reject empty `PROXY_API_KEY` and an empty initial account set with `log.Fatal`. Mount middleware as:

```go
v1.Use(middleware.BrowserAuth(cfg.ProxyAPIKey, cfg.SessionKey, cfg.ClaudeCookie, true))
```

Start a context-cancelled ticker using `cfg.AccountReloadInterval`. On each tick call `config.LoadAccounts`; log and retain the previous pool on errors, and call `h.ReloadAccounts` on successful results. Cancel the watcher during graceful shutdown.

Add `func (h *Handler) ReloadAccounts(accounts []config.Account) error` as a thin wrapper over the pool replacement method.

- [ ] **Step 4: Run configuration and integration tests.**

Run: `go test ./config ./middleware ./handlers`

Expected: PASS.

- [ ] **Step 5: Commit startup and watcher integration.**

```bash
git add main.go config/config.go config/config_test.go handlers/common.go
git commit -m "feat: hot reload configured accounts"
```

### Task 6: Update Deployment and API Documentation

**Files:**
- Modify: `docker-compose.yml`
- Modify: `README.md`
- Modify: `README_EN.md`
- Modify: `API.md`
- Modify: `API_EN.md`

- [ ] **Step 1: Update deployment configuration.** Add `PROXY_API_KEY`, `ACCOUNT_RATE_LIMIT_COOLDOWN`, and `ACCOUNT_RELOAD_INTERVAL` to Docker Compose environment forwarding. Keep the accounts bind mount read-only so the operator changes the host file and the container reloads it.

- [ ] **Step 2: Replace credential documentation.** Document `Authorization: Bearer <PROXY_API_KEY>` for clients. State that Claude `sessionKey` and Cookie values are server-side only and that `X-Claude-Cookie` is no longer a caller authentication mechanism. Document the 5-second reload default, removal behavior, and 429 `Retry-After` semantics.

- [ ] **Step 3: Verify documentation and examples.**

Run: `rg -n 'Bearer <claude.ai sessionKey>|X-Claude-Cookie|PROXY_API_KEY|ACCOUNT_RATE_LIMIT_COOLDOWN|ACCOUNT_RELOAD_INTERVAL' README.md README_EN.md API.md API_EN.md docker-compose.yml`

Expected: client examples use `PROXY_API_KEY`; upstream credential references appear only in server configuration sections.

- [ ] **Step 4: Commit deployment documentation.**

```bash
git add docker-compose.yml README.md README_EN.md API.md API_EN.md
git commit -m "docs: document proxy authentication and account cooldowns"
```

### Task 7: Full Verification

**Files:**
- Test: all existing Go tests

- [ ] **Step 1: Run formatting and static checks.**

Run: `gofmt -w config middleware claude handlers main.go && git diff --check`

Expected: no diff-check errors.

- [ ] **Step 2: Run the complete test suite.**

Run: `go test ./...`

Expected: PASS for every package.

- [ ] **Step 3: Build the deployable binary.**

Run: `go build ./...`

Expected: exit code 0.

- [ ] **Step 4: Inspect the final diff and status.**

Run: `git diff --stat HEAD~7..HEAD && git status --short --branch`

Expected: only the authentication, account pool, upstream error, startup, deployment, and documentation files are changed; no credentials or generated artifacts are present.
