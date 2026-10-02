package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"claude2api/config"
	"claude2api/handlers"
	"claude2api/middleware"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.New()
	if strings.TrimSpace(cfg.ProxyAPIKey) == "" {
		log.Fatal("PROXY_API_KEY is required")
	}
	if len(cfg.Accounts) == 0 {
		log.Fatal("no Claude account configured; set CLAUDE_SESSION_KEY, CLAUDE_COOKIE, or CLAUDE_ACCOUNTS_FILE")
	}

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
	watchCtx, stopWatching := context.WithCancel(context.Background())
	go watchAccounts(watchCtx, h, cfg)

	// OpenAI-compatible endpoints
	v1 := r.Group("/v1")
	v1.Use(middleware.BrowserAuth(cfg.ProxyAPIKey, cfg.SessionKey, cfg.ClaudeCookie, true))
	{
		v1.GET("/models", h.ListModels)
		v1.POST("/chat/completions", h.ChatCompletion)
		v1.POST("/messages", h.AnthropicMessages)
		v1.POST("/responses", h.Responses)
		v1.DELETE("/conversations/:id", h.DeleteConversation)
	}

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("claude2api listening on :%s", cfg.Port)
		log.Printf("  Base URL : %s", cfg.ClaudeBaseURL)
		log.Printf("  Models   : fetched from claude.ai per request")
		if len(cfg.Accounts) > 1 {
			log.Printf("  Accounts : %d configured, least-loaded routing enabled", len(cfg.Accounts))
		} else if len(cfg.Accounts) == 1 {
			log.Printf("  Auth     : one configured account")
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	stopWatching()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}
}

func watchAccounts(ctx context.Context, h *handlers.Handler, cfg *config.Config) {
	interval := cfg.AccountReloadInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			accounts, err := config.LoadAccounts(cfg.AccountsFile, cfg.SessionKey, cfg.ClaudeCookie)
			if err != nil {
				log.Printf("accounts reload failed: %v", err)
				continue
			}
			if err := h.ReloadAccounts(accounts); err != nil {
				log.Printf("accounts reload rejected: %v", err)
			}
		}
	}
}
