package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// BearerAuth provides proxy-level Bearer authentication.
func BearerAuth(proxyAPIKey string) gin.HandlerFunc {
	return BrowserAuth(proxyAPIKey, "", "")
}

// BrowserAuth authenticates callers with the proxy API key and keeps upstream
// Claude credentials server-side.
func BrowserAuth(proxyAPIKey, envSessionKey, envClaudeCookie string, _ ...bool) gin.HandlerFunc {
	expectedKey := strings.TrimSpace(proxyAPIKey)
	return func(c *gin.Context) {
		providedKey := bearerToken(c.GetHeader("Authorization"))
		if expectedKey == "" || providedKey == "" || subtle.ConstantTimeCompare([]byte(providedKey), []byte(expectedKey)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Missing or invalid proxy API key. Provide Authorization: Bearer <PROXY_API_KEY>",
					"type":    "invalid_request_error",
				},
			})
			return
		}

		// Store only server-side upstream credentials for downstream handlers.
		c.Set("sessionKey", strings.TrimSpace(envSessionKey))
		c.Set("claudeCookie", strings.TrimSpace(envClaudeCookie))
		c.Set("explicitCredentials", false)
		c.Next()
	}
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}
