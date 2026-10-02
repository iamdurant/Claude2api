package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBrowserAuthDoesNotMixExplicitBearerWithEnvCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrowserAuth("proxy-secret", "server-session", "server-cookie", true))
	r.GET("/", func(c *gin.Context) {
		token, _ := c.Get("sessionKey")
		cookie, _ := c.Get("claudeCookie")
		explicit, _ := c.Get("explicitCredentials")
		c.JSON(http.StatusOK, gin.H{"token": token, "cookie": cookie, "explicit": explicit})
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer proxy-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != `{"cookie":"server-cookie","explicit":false,"token":"server-session"}` {
		t.Fatalf("unexpected credentials: %s", got)
	}
}

func TestBrowserAuthRejectsMissingOrInvalidProxyKey(t *testing.T) {
	for _, header := range []string{"", "Bearer wrong", "Basic proxy-secret"} {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(BrowserAuth("proxy-secret", "server-session", "", true))
		r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("header %q: status = %d, body = %s", header, w.Code, w.Body.String())
		}
	}
}

func TestBrowserAuthDoesNotAcceptRequestClaudeCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrowserAuth("proxy-secret", "server-session", "server-cookie", true))
	r.GET("/", func(c *gin.Context) {
		token, _ := c.Get("sessionKey")
		cookie, _ := c.Get("claudeCookie")
		c.JSON(http.StatusOK, gin.H{"token": token, "cookie": cookie})
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer proxy-secret")
	req.Header.Set("X-Claude-Cookie", "sessionKey=request-session")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "request-session") {
		t.Fatalf("request credential overrode server credential: %s", w.Body.String())
	}
}
