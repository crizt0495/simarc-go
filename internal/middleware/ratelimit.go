package middleware

import (
	"net/http"
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

const (
	maxAttempts = 5
	windowTime  = 1 * time.Minute
)

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
