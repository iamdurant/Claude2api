package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"claude2api/config"
	"claude2api/handlers"
	"claude2api/middleware"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.New()

	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	h := handlers.NewHandler(cfg)

	// OpenAI-compatible endpoints
	v1 := r.Group("/v1")
	v1.Use(middleware.BrowserAuth(cfg.SessionKey, cfg.ClaudeCookie))
	{
		v1.GET("/models", h.ListModels)
		v1.POST("/chat/completions", h.ChatCompletion)
		v1.POST("/messages", h.AnthropicMessages)
		v1.POST("/responses", h.Responses)
	}

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("claude2api listening on :%s", cfg.Port)
		log.Printf("  Base URL : %s", cfg.ClaudeBaseURL)
		log.Printf("  Models   : %d", len(config.SupportedModels))
		if cfg.SessionKey != "" {
			log.Printf("  Auth     : env session key configured")
		} else {
			log.Printf("  Auth     : per-request Bearer token required")
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}
}
