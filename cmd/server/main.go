package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Health handler
func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status":"ok","time":time.Now().Format(time.RFC3339)})
}

// Auth endpoints (placeholder — use JWT middleware from internal/auth)
func authRoutes(r *gin.Engine) {
	r.POST("/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"token":"jwt-placeholder"})
	})
	r.POST("/auth/refresh", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"token":"refreshed-jwt"})
	})
	r.POST("/auth/logout", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message":"logged out"})
	})
}

// Device endpoint
func deviceRoute(r *gin.Engine) {
	r.POST("/devices/register", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"device_id":"uuid-v7-placeholder","registered":true})
	})
}

// Sync endpoints (placeholder — connect to internal/sync)
func syncRoutes(r *gin.Engine) {
	r.GET("/sync/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status":"offline","pending":0,"acknowledged":0})
	})
	r.POST("/sync/push", func(c *gin.Context) {
		c.JSON(http.StatusAccepted, gin.H{"batch_id":"batch-uuid","items":0,"ack":false})
	})
	r.GET("/sync/pull", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"items":[]map[string]interface{}{},"cursor":""})
	})
	r.POST("/sync/ack", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ack":true})
	})
}

// Bootstrap endpoint (deliver initial schema / seed)
func bootstrapRoute(r *gin.Engine) {
	r.GET("/bootstrap", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"version":"1.0.0","seed":"ready"})
	})
}

func main() {
	r := gin.Default()
	r.GET("/health", healthHandler)
	authRoutes(r)
	deviceRoute(r)
	syncRoutes(r)
	bootstrapRoute(r)
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "SIMARC Server — Offline-First Sync Ready")
	})
	r.Run(":8080")
}
