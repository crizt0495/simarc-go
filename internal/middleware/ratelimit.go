package middleware

import (
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type attempt struct {
	count    int
	firstTry time.Time
}

var (
	rateLimiters = make(map[string]*attempt)
	rateMu       sync.Mutex
)

// envInt reads a positive integer from the environment, falling back to def.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// The default window/limit suit a LAN deployment. They can be tuned with
// SIMARC_LOGIN_MAX_ATTEMPTS and SIMARC_LOGIN_WINDOW_SECONDS.
var (
	maxAttempts = envInt("SIMARC_LOGIN_MAX_ATTEMPTS", 5)
	windowTime  = time.Duration(envInt("SIMARC_LOGIN_WINDOW_SECONDS", 60)) * time.Second
)

// isLoopback reports whether ip is a local (loopback) address. The desktop
// app serves on localhost for a single user, so brute-force throttling there
// only hurts legitimate use (and local tooling/tests).
func isLoopback(ip string) bool {
	switch ip {
	case "127.0.0.1", "::1", "localhost", "::ffff:127.0.0.1":
		return true
	}
	return false
}

func cleanupRateLimiters() {
	rateMu.Lock()
	defer rateMu.Unlock()
	for ip, a := range rateLimiters {
		if time.Since(a.firstTry) > windowTime*2 {
			delete(rateLimiters, ip)
		}
	}
}

func init() {
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			cleanupRateLimiters()
		}
	}()
}

func LoginRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		// Local (desktop app / tooling) is not throttled.
		if isLoopback(ip) {
			c.Next()
			return
		}

		rateMu.Lock()
		a, exists := rateLimiters[ip]
		if !exists {
			rateLimiters[ip] = &attempt{count: 1, firstTry: time.Now()}
			rateMu.Unlock()
			c.Next()
			return
		}

		if time.Since(a.firstTry) > windowTime {
			a.count = 1
			a.firstTry = time.Now()
			rateMu.Unlock()
			c.Next()
			return
		}

		a.count++
		if a.count > maxAttempts {
			rateMu.Unlock()
			c.Header("Retry-After", "60")
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Terlalu banyak percobaan login. Silakan coba lagi dalam 1 menit.",
			})
			c.Abort()
			return
		}
		rateMu.Unlock()
		c.Next()
	}
}
