// Command server runs SIMARC as a standalone HTTP server (local / VPS).
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"arsippro/internal/app"
	"arsippro/internal/config"
)

func getLANIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)
	if addr.IP.IsLoopback() {
		return "127.0.0.1"
	}
	return addr.IP.String()
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		cmd := exec.Command("open", url)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Start()
	case "linux":
		cmd := exec.Command("xdg-open", url)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Start()
	}
}

func main() {
	r, err := app.Init()
	if err != nil {
		log.Fatalf("Gagal menginisialisasi aplikasi: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = config.App.AppPort
	}
	addr := ":" + port
	lanIP := getLANIP()

	if config.IsVercel() {
		log.Printf("Vercel deployment detected, listening on :%s", port)
	} else {
		fmt.Println()
		fmt.Println("  ╔══════════════════════════════════════════════════════════╗")
		fmt.Println("  ║                                                          ║")
		fmt.Println("  ║            S I M A R C                                      ║")
		fmt.Println("  ║     Sistem Informasi Manajemen Arsip Record Center           ║")
		fmt.Println("  ║                                                              ║")
		fmt.Println("  ║                                                          ║")
		fmt.Println("  ╠══════════════════════════════════════════════════════════╣")
		fmt.Println("  ║                                                          ║")
		fmt.Printf("  ║   Local     :  http://127.0.0.1:%-5s                    ║\n", port)
		fmt.Printf("  ║   Network   :  http://%s:%-5s                   ║\n", lanIP, port)
		fmt.Println("  ║                                                          ║")
		fmt.Println("  ║   Client di jaringan yang sama bisa akses:               ║")
		fmt.Printf("  ║   -> http://%s:%-5s                            ║\n", lanIP, port)
		fmt.Println("  ║                                                          ║")
		fmt.Println("  ║   Tekan Ctrl+C untuk berhenti                            ║")
		fmt.Println("  ║                                                          ║")
		fmt.Println("  ╚══════════════════════════════════════════════════════════╝")
		fmt.Println()

		openBrowser(fmt.Sprintf("http://%s:%s", lanIP, port))
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.Printf("SIMARC starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited gracefully.")
}
