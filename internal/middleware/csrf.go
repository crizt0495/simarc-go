package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

const CSRFKey = "csrf_token"

func generateCSRFToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CSRF middleware validates CSRF token for POST/PUT/DELETE requests
func CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" {
			session := GetSession(c)
			token, ok := session.Values[CSRFKey].(string)
			if !ok || token == "" {
				token = generateCSRFToken()
				session.Values[CSRFKey] = token
				SaveSession(c, session)
			}
			c.Set(CSRFKey, token)
			c.Next()
			return
		}

		session := GetSession(c)
		sessionToken, ok := session.Values[CSRFKey].(string)
		if !ok || sessionToken == "" {
			c.HTML(http.StatusForbidden, "errors/403.html", gin.H{"title": "CSRF token missing"})
			c.Abort()
			return
		}

		requestToken := c.GetHeader("X-CSRF-Token")
		if requestToken == "" {
			requestToken = c.PostForm("_token")
		}
		if requestToken == "" {
			requestToken = c.Query("_token")
		}

		if requestToken != sessionToken {
			c.HTML(http.StatusForbidden, "errors/403.html", gin.H{"title": "CSRF token mismatch"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// GetCSRFToken returns the CSRF token from context
func GetCSRFToken(c *gin.Context) string {
	if token, ok := c.Get(CSRFKey); ok {
		if s, ok := token.(string); ok {
			return s
		}
	}
	return ""
}
