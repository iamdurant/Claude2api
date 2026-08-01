package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBrowserAuthDoesNotMixExplicitBearerWithEnvCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrowserAuth("env-token", "sessionKey=env-token; anthropic-device-id=env-device", true))
	r.GET("/", func(c *gin.Context) {
		token, _ := c.Get("sessionKey")
		cookie, _ := c.Get("claudeCookie")
		c.JSON(http.StatusOK, gin.H{"token": token, "cookie": cookie})
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer request-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != `{"cookie":"","token":"request-token"}` {
		t.Fatalf("unexpected credentials: %s", got)
	}
}

func TestBrowserAuthAllowsConfiguredAccountPool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrowserAuth("", "", true))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
