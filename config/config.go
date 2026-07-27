package config

import (
	"encoding/json"
	"os"
	"strings"
)

// Config holds application configuration
type Config struct {
	Port          string
	ClaudeBaseURL string
	SessionKey    string
	ClaudeCookie  string
	Timezone      string
	Locale        string
	DefaultModel  string
	Effort        string
	Thinking      interface{}
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

	// CLAUDE_CODE_EFFORT_LEVEL is the env var Claude Code reads; support it so
	// the same knob controls both. CLAUDE_EFFORT is the native override.
	effort := os.Getenv("CLAUDE_EFFORT")
	if effort == "" {
		effort = os.Getenv("CLAUDE_CODE_EFFORT_LEVEL")
	}
	if effort == "" {
		effort = "medium"
	}

	// CLAUDE_THINKING accepts: "auto" (default), "none", or a JSON object like
	// {"type":"enabled","budget_tokens":10000}. It is the proxy-level default;
	// per-request "thinking" from Claude Code overrides it.
	thinkingRaw := os.Getenv("CLAUDE_THINKING")
	thinking := parseThinkingEnv(thinkingRaw)

	return &Config{
		Port:          port,
		ClaudeBaseURL: baseURL,
		SessionKey:    os.Getenv("CLAUDE_SESSION_KEY"),
		ClaudeCookie:  os.Getenv("CLAUDE_COOKIE"),
		Timezone:      timezone,
		Locale:        locale,
		DefaultModel:  model,
		Effort:        effort,
		Thinking:      thinking,
	}
}

// parseThinkingEnv parses the CLAUDE_THINKING env var into a value suitable for
// Config.Thinking. Returns nil (auto), "none", or a map for enabled.
func parseThinkingEnv(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	lower := strings.ToLower(s)
	if lower == "none" || lower == "disabled" {
		return map[string]interface{}{"type": "disabled"}
	}
	if lower != "auto" && lower != "enabled" {
		// Try JSON.
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(s), &m); err == nil {
			return m
		}
	}
	return map[string]interface{}{"type": "enabled", "budget_tokens": 10000}
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
	"claude-opus-5":     "claude-opus-5",
}
