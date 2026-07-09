package config

import "os"

// Config holds application configuration
type Config struct {
	Port          string
	ClaudeBaseURL string
	SessionKey    string
	ClaudeCookie  string
	Timezone      string
	Locale        string
	DefaultModel  string
}

// New creates a Config from environment variables with sane defaults
func New() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	baseURL := os.Getenv("CLAUDE_BASE_URL")
	if baseURL == "" {
		baseURL = "https://claude.ai"
	}

	model := os.Getenv("DEFAULT_MODEL")
	if model == "" {
		model = "claude-sonnet-5"
	}

	locale := os.Getenv("CLAUDE_LOCALE")
	if locale == "" {
		locale = "en-US"
	}

	timezone := os.Getenv("CLAUDE_TIMEZONE")
	if timezone == "" {
		timezone = "Asia/Singapore"
	}

	return &Config{
		Port:          port,
		ClaudeBaseURL: baseURL,
		SessionKey:    os.Getenv("CLAUDE_SESSION_KEY"),
		ClaudeCookie:  os.Getenv("CLAUDE_COOKIE"),
		Timezone:      timezone,
		Locale:        locale,
		DefaultModel:  model,
	}
}

// SupportedModels is the set of models exposed by the API
var SupportedModels = map[string]string{
	"claude-fable-5":    "claude-fable-5",
	"claude-opus-4-8":   "claude-opus-4-8",
	"claude-haiku-4-5":  "claude-haiku-4-5",
	"claude-opus-4-7":   "claude-opus-4-7",
	"claude-opus-4-6":   "claude-opus-4-6",
	"claude-opus-3":     "claude-opus-3",
	"claude-sonnet-4-6": "claude-sonnet-4-6",
	"claude-sonnet-5":   "claude-sonnet-5",
}
