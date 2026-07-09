package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// BearerAuth provides OpenAI-compatible Bearer token authentication.
// The token is the claude.ai sessionKey.
// If an env-level session key is configured, the Bearer token is optional.
func BearerAuth(envSessionKey string) gin.HandlerFunc {
	return BrowserAuth(envSessionKey, "")
}

// BrowserAuth accepts a sessionKey plus an optional full claude.ai Cookie header.
func BrowserAuth(envSessionKey, envClaudeCookie string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract Bearer token from Authorization header
		authHeader := c.GetHeader("Authorization")
		var token string
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// If env session key is set, use it as fallback
		if token == "" && envSessionKey != "" {
			token = envSessionKey
		}

		cookie := c.GetHeader("X-Claude-Cookie")
		if cookie == "" {
			cookie = envClaudeCookie
		}
		if token == "" {
			token = sessionKeyFromCookie(cookie)
		}

		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Missing API key. Provide via Authorization: Bearer <sessionKey> or X-Claude-Cookie",
					"type":    "invalid_request_error",
				},
			})
			return
		}

		// Store in context for downstream handlers
		c.Set("sessionKey", token)
		c.Set("claudeCookie", cookie)
		c.Next()
	}
}

func sessionKeyFromCookie(cookie string) string {
	for _, part := range strings.Split(cookie, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 && kv[0] == "sessionKey" {
			return kv[1]
		}
	}
	return ""
}
