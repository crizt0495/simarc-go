package main

import (
	"fmt"
	"os"

	"arsippro/internal/config"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("Config error:", err)
		os.Exit(1)
	}
	mode := func() string {
		if cfg.ClientMode {
			return "CLIENT (hub: " + cfg.ServerURL + ")"
		}
		return "LOCAL (offline-first)"
	}
	fmt.Printf("SIMARC Desktop — Mode: %s | Window: %s | DB: %s | Debug: %v\n",
		mode(), cfg.WindowMode, cfg.DBDriver, cfg.Debug)

	fmt.Println("Starting embedded server at :8080 ...")
	go func() {
		r := gin.Default()
		r.GET("/", func(c *gin.Context) { c.String(200, "SIMARC Desktop — Offline-First Ready") })
		r.Run(":8080")
	}()

	// Keep process alive (desktop window stays open)
	select {}
}
