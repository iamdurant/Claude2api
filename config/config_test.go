package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAccounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.txt")
	contents := "# comment\naccount-a\n\nsessionKey=account-b; anthropic-device-id=device-b\naccount-a\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write accounts: %v", err)
	}

	accounts := loadAccounts(path, "account-c", "")
	if len(accounts) != 3 {
		t.Fatalf("expected 3 unique accounts, got %d: %#v", len(accounts), accounts)
	}
	if accounts[0].SessionKey != "account-a" || accounts[1].SessionKey != "account-b" || accounts[2].SessionKey != "account-c" {
		t.Fatalf("unexpected accounts: %#v", accounts)
	}
	if accounts[1].Cookie == "" {
		t.Fatal("full Cookie line was not preserved")
	}
}

func TestLoadAccountsAllowsMissingFileWithFallback(t *testing.T) {
	accounts, err := LoadAccounts(filepath.Join(t.TempDir(), "missing.txt"), "fallback", "")
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].SessionKey != "fallback" {
		t.Fatalf("unexpected fallback accounts: %#v", accounts)
	}
}

func TestLoadAccountsReportsReadErrors(t *testing.T) {
	path := t.TempDir()
	if _, err := LoadAccounts(path, "fallback", ""); err == nil {
		t.Fatal("expected directory read error")
	}
}

func TestParseDurationEnv(t *testing.T) {
	if got := parseDurationEnv("750ms", time.Second); got != 750*time.Millisecond {
		t.Fatalf("duration = %v", got)
	}
	if got := parseDurationEnv("invalid", time.Second); got != time.Second {
		t.Fatalf("invalid duration = %v", got)
	}
	if got := parseDurationEnv("", time.Second); got != time.Second {
		t.Fatalf("empty duration = %v", got)
	}
}

func TestNewLoadsProxyAndReloadConfiguration(t *testing.T) {
	accountsPath := filepath.Join(t.TempDir(), "accounts.txt")
	if err := os.WriteFile(accountsPath, []byte("account-a\n"), 0o600); err != nil {
		t.Fatalf("write accounts: %v", err)
	}
	t.Setenv("PROXY_API_KEY", "proxy-secret")
	t.Setenv("CLAUDE_ACCOUNTS_FILE", accountsPath)
	t.Setenv("ACCOUNT_RELOAD_INTERVAL", "2s")
	t.Setenv("ACCOUNT_RATE_LIMIT_COOLDOWN", "45s")

	cfg := New()
	if cfg.ProxyAPIKey != "proxy-secret" || cfg.AccountsFile != accountsPath {
		t.Fatalf("unexpected security config: %#v", cfg)
	}
	if cfg.AccountReloadInterval != 2*time.Second || cfg.AccountRateLimitCooldown != 45*time.Second {
		t.Fatalf("unexpected reload config: %#v", cfg)
	}
	if len(cfg.Accounts) != 1 || cfg.Accounts[0].SessionKey != "account-a" {
		t.Fatalf("unexpected loaded accounts: %#v", cfg.Accounts)
	}
}
