package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds application configuration
type Config struct {
	Port          string
	ClaudeBaseURL string
	ProxyAPIKey   string
	SessionKey    string
	ClaudeCookie  string
	AccountsFile  string

	AccountReloadInterval    time.Duration
	AccountRateLimitCooldown time.Duration

	Timezone     string
	Locale       string
	DefaultModel string
	Effort       string
	Thinking     interface{}
	Accounts     []Account
}

// Account contains credentials for one claude.ai account. Cookie may contain
// the full browser Cookie header; SessionKey is used for Bearer-only mode.
type Account struct {
	SessionKey string
	Cookie     string
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

	proxyAPIKey := strings.TrimSpace(os.Getenv("PROXY_API_KEY"))
	sessionKey := os.Getenv("CLAUDE_SESSION_KEY")
	claudeCookie := os.Getenv("CLAUDE_COOKIE")
	accountsFile := strings.TrimSpace(os.Getenv("CLAUDE_ACCOUNTS_FILE"))
	if accountsFile == "" {
		accountsFile = "accounts.txt"
	}
	accounts := loadAccounts(accountsFile, sessionKey, claudeCookie)

	return &Config{
		Port:                     port,
		ClaudeBaseURL:            baseURL,
		ProxyAPIKey:              proxyAPIKey,
		SessionKey:               sessionKey,
		ClaudeCookie:             claudeCookie,
		AccountsFile:             accountsFile,
		AccountReloadInterval:    parseDurationEnv(os.Getenv("ACCOUNT_RELOAD_INTERVAL"), 5*time.Second),
		AccountRateLimitCooldown: parseDurationEnv(os.Getenv("ACCOUNT_RATE_LIMIT_COOLDOWN"), 60*time.Second),
		Timezone:                 timezone,
		Locale:                   locale,
		DefaultModel:             model,
		Effort:                   effort,
		Thinking:                 thinking,
		Accounts:                 accounts,
	}
}

func loadAccounts(path, fallbackSessionKey, fallbackCookie string) []Account {
	accounts, err := LoadAccounts(path, fallbackSessionKey, fallbackCookie)
	if err == nil {
		return accounts
	}
	return fallbackAccounts(fallbackSessionKey, fallbackCookie)
}

// LoadAccounts reads the configured account file and appends the optional
// environment-level credentials, preserving file order and deduplicating pairs.
// A missing file is valid; other read or scanner errors are returned.
func LoadAccounts(path, fallbackSessionKey, fallbackCookie string) ([]Account, error) {
	if strings.TrimSpace(path) == "" {
		path = "accounts.txt"
	}

	accounts := make([]Account, 0)
	seen := make(map[string]struct{})
	add := func(account Account) {
		account.SessionKey = strings.TrimSpace(account.SessionKey)
		account.Cookie = strings.TrimSpace(account.Cookie)
		if account.SessionKey == "" {
			account.SessionKey = sessionKeyFromCookie(account.Cookie)
		}
		if account.SessionKey == "" {
			return
		}
		key := account.SessionKey + "\x00" + account.Cookie
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		accounts = append(accounts, account)
	}

	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.Contains(line, "sessionKey=") {
				add(Account{Cookie: line})
			} else {
				add(Account{SessionKey: line})
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("scan accounts file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("open accounts file: %w", err)
	}

	add(Account{SessionKey: fallbackSessionKey, Cookie: fallbackCookie})
	return accounts, nil
}

func fallbackAccounts(sessionKey, cookie string) []Account {
	accounts := make([]Account, 0, 1)
	sessionKey = strings.TrimSpace(sessionKey)
	cookie = strings.TrimSpace(cookie)
	if sessionKey == "" {
		sessionKey = sessionKeyFromCookie(cookie)
	}
	if sessionKey != "" {
		accounts = append(accounts, Account{SessionKey: sessionKey, Cookie: cookie})
	}
	return accounts
}

func parseDurationEnv(value string, fallback time.Duration) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func sessionKeyFromCookie(cookie string) string {
	for _, part := range strings.Split(cookie, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 && kv[0] == "sessionKey" {
			return kv[1]
		}
	}
	return ""
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
