package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"claude2api/claude"
	"claude2api/config"
	"claude2api/middleware"
	"claude2api/models"

	"github.com/gin-gonic/gin"
)

func modelsTestRouter(baseURL string, accounts []config.Account) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&config.Config{ClaudeBaseURL: baseURL, Accounts: accounts, DefaultModel: "claude-future"})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if len(accounts) > 0 && c.GetHeader("Authorization") == "" {
			c.Request.Header.Set("Authorization", "Bearer test-proxy")
		}
		c.Next()
	})
	r.Use(middleware.BrowserAuth("test-proxy", "", "", len(accounts) > 0))
	r.GET("/v1/models", h.ListModels)
	r.POST("/v1/chat/completions", h.ChatCompletion)
	r.POST("/v1/messages", h.AnthropicMessages)
	r.POST("/v1/responses", h.Responses)
	return r
}

func TestListModelsFetchesWebAPIAndRefreshes(t *testing.T) {
	var version atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method %s", r.Method)
		}
		cookie, err := r.Cookie("sessionKey")
		if err != nil || cookie.Value != "account-a" {
			t.Error("missing account cookie")
		}
		if r.URL.Path == "/api/organizations" {
			io.WriteString(w, `[{"uuid":"org-a"}]`)
			return
		}
		if r.URL.Path != "/edge-api/bootstrap/org-a/app_start" {
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if r.URL.Query().Get("include_system_prompts") != "false" || r.URL.Query().Get("cache_bust") != "1" || r.URL.Query().Get("statsig_hashing_algorithm") != "djb2" || r.URL.Query().Get("growthbook_format") != "sdk" {
			t.Errorf("unexpected bootstrap query %s", r.URL.RawQuery)
		}
		if version.Load() == 0 {
			io.WriteString(w, `{"model_selector_state":{"thinking_by_model":[{"id":"stale-selection"}]},"claude_ai_available_models":{"models":[{"model_id":"claude-future","minimum_tier":"pro"},{"model_id":"claude-future"},{"model_id":"claude-other","minimum_tier":"free"},{"model_id":" "}]}}`)
		} else {
			io.WriteString(w, `{"claude_ai_available_models":{"models":[{"model_id":"claude-new"}]}}`)
		}
	}))
	defer upstream.Close()
	r := modelsTestRouter(upstream.URL, []config.Account{{SessionKey: "account-a"}})
	for i, want := range [][]string{{"claude-future", "claude-other"}, {"claude-new"}} {
		version.Store(int32(i))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/models", nil))
		if w.Code != 200 {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var response models.ModelsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Object != "list" {
			t.Errorf("object=%s", response.Object)
		}
		ids := []string{}
		for _, m := range response.Data {
			ids = append(ids, m.ID)
			if m.Object != "model" || m.OwnedBy != "anthropic" {
				t.Errorf("invalid metadata: %+v", m)
			}
		}
		if !reflect.DeepEqual(ids, want) {
			t.Fatalf("ids=%v, want %v", ids, want)
		}
	}
}

func TestListModelsErrorsDoNotReturnStaticModels(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"forbidden", 403, `private-upstream-body`},
		{"unsupported-get", 405, `private-upstream-body`},
		{"invalid-json", 200, `not json`},
		{"missing-models", 200, `{"model_selector_state":{"thinking_by_model":[{"id":"stale-selection"}]}}`},
		{"empty-models", 200, `{"claude_ai_available_models":{"models":[]}}`},
		{"null-models", 200, `{"claude_ai_available_models":null}`},
		{"wrong-type", 200, `{"claude_ai_available_models":{"models":"bad"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); io.WriteString(w, tc.body) }))
			defer upstream.Close()
			r := modelsTestRouter(upstream.URL, []config.Account{{Cookie: "sessionKey=account-a; lastActiveOrg=org-a", SessionKey: "account-a"}})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/models", nil))
			if w.Code != 502 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "private-upstream-body") || strings.Contains(w.Body.String(), `"data"`) {
				t.Fatalf("unexpected response %s", w.Body.String())
			}
		})
	}
}

func TestListModelsUsesConfiguredCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("sessionKey")
		if err != nil {
			t.Error(err)
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/api/organizations" {
			io.WriteString(w, `[{"uuid":"org"}]`)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"claude_ai_available_models": map[string]any{"models": []map[string]string{{"model_id": "model-" + cookie.Value}}}})
	}))
	defer upstream.Close()
	r := modelsTestRouter(upstream.URL, []config.Account{{SessionKey: "configured"}})
	for _, token := range []string{"account-a", "account-b"} {
		req := httptest.NewRequest("GET", "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer test-proxy")
		req.Header.Set("X-Claude-Cookie", "sessionKey="+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "model-configured") {
			t.Fatalf("wrong account response %d: %s", w.Code, w.Body.String())
		}
	}
	r = modelsTestRouter(upstream.URL, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/models", nil))
	if w.Code != 401 {
		t.Fatalf("unauthenticated status %d", w.Code)
	}
}

func TestListModelsReturnsRetryable429WhenAllAccountsCoolDown(t *testing.T) {
	h := NewHandler(&config.Config{
		ClaudeBaseURL:            "https://claude.ai",
		Accounts:                 []config.Account{{SessionKey: "account-a"}, {SessionKey: "account-b"}},
		AccountRateLimitCooldown: time.Minute,
		DefaultModel:             "claude-future",
	})
	for _, account := range h.clients.accounts {
		h.clients.observe(account.id, &claude.HTTPError{
			StatusCode:    http.StatusTooManyRequests,
			RetryAfter:    time.Minute,
			RetryAfterSet: true,
		})
	}

	r := gin.New()
	r.Use(middleware.BrowserAuth("test-proxy", "", "", true))
	r.GET("/v1/models", h.ListModels)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer test-proxy")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header")
	}
	if !strings.Contains(w.Body.String(), "upstream_error") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestListModelsRateLimitMovesNextRequestToAnotherAccount(t *testing.T) {
	var organizationCalls atomic.Int32
	var secondAccount atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("sessionKey")
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/organizations" {
			call := organizationCalls.Add(1)
			if call == 1 {
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			secondAccount.Store(cookie.Value)
			io.WriteString(w, `[{"uuid":"org"}]`)
			return
		}
		if r.URL.Path == "/edge-api/bootstrap/org/app_start" {
			io.WriteString(w, `{"claude_ai_available_models":{"models":[{"model_id":"model"}]}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	r := modelsTestRouter(upstream.URL, []config.Account{{SessionKey: "account-a"}, {SessionKey: "account-b"}})

	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if first.Code != http.StatusTooManyRequests {
		t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
	}

	second := httptest.NewRecorder()
	r.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, body = %s", second.Code, second.Body.String())
	}
	if got := secondAccount.Load(); got != "account-b" {
		t.Fatalf("second request used account %v, want account-b", got)
	}
}

func TestNewModelReachesAllCompletionAPIs(t *testing.T) {
	for _, route := range []struct{ path, body string }{
		{"/v1/chat/completions", `{"model":"claude-future","messages":[{"role":"user","content":"hello"}]}`},
		{"/v1/messages", `{"model":"claude-future","messages":[{"role":"user","content":"hello"}]}`},
		{"/v1/responses", `{"model":"claude-future","input":"hello"}`},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(route.path+map[bool]string{false: "/json", true: "/stream"}[stream], func(t *testing.T) {
				var completions atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.URL.Path == "/api/organizations":
						io.WriteString(w, `[{"uuid":"org"}]`)
					case strings.HasSuffix(r.URL.Path, "/completion"):
						var body struct {
							Model string `json:"model"`
						}
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
						}
						if body.Model != "claude-future" {
							t.Errorf("upstream model=%q", body.Model)
						}
						completions.Add(1)
						w.Header().Set("Content-Type", "text/event-stream")
						io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello-back\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
					case r.Method == "DELETE":
						w.WriteHeader(204)
					case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/chat_conversations"):
						io.WriteString(w, `{"uuid":"conversation"}`)
					default:
						t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
						w.WriteHeader(404)
					}
				}))
				defer upstream.Close()
				r := modelsTestRouter(upstream.URL, []config.Account{{SessionKey: "account"}})
				body := route.body
				if stream {
					body = strings.TrimSuffix(body, "}") + `,"stream":true}`
				}
				req := httptest.NewRequest("POST", route.path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if w.Code != 200 || !strings.Contains(w.Body.String(), "hello-back") || completions.Load() != 1 {
					t.Fatalf("completion failed: %d %s (calls=%d)", w.Code, w.Body.String(), completions.Load())
				}
			})
		}
	}
}

func TestResolveModelAllowsNewWebModels(t *testing.T) {
	for _, tc := range []struct{ requested, fallback, want string }{{"claude-future", "", "claude-future"}, {"", "claude-default", "claude-default"}, {"claude-haiku-4-5", "", "claude-haiku-4-5"}} {
		got, err := resolveModel(tc.requested, tc.fallback)
		if err != nil || got != tc.want {
			t.Fatalf("resolveModel=%q,%v; want %q", got, err, tc.want)
		}
	}
	if _, err := resolveModel("", ""); err == nil {
		t.Fatal("expected error for missing model")
	}
}
