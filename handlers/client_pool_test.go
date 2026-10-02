package handlers

import (
	"errors"
	"sync"
	"testing"
	"time"

	"claude2api/claude"
	"claude2api/config"
)

func TestClientPoolBalancesConfiguredAccounts(t *testing.T) {
	pool, err := newClientPool("https://claude.ai", []config.Account{
		{SessionKey: "account-a"},
		{SessionKey: "account-b"},
	})
	if err != nil {
		t.Fatalf("newClientPool: %v", err)
	}

	first, err := pool.acquireConfigured()
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	second, err := pool.acquireConfigured()
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	defer first.release()
	defer second.release()
	if first.accountID == second.accountID {
		t.Fatalf("expected least-loaded routing across accounts, got %q twice", first.accountID)
	}
}

func TestClientPoolPinsConversationToConfiguredAccount(t *testing.T) {
	pool, err := newClientPool("https://claude.ai", []config.Account{
		{SessionKey: "account-a"},
		{SessionKey: "account-b"},
	})
	if err != nil {
		t.Fatalf("newClientPool: %v", err)
	}

	first, err := pool.acquire("", "", false, "conversation-1")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	firstID := first.accountID
	first.release()

	// Occupy the pinned account so ordinary least-loaded routing would prefer the
	// other account. The same conversation must still retain account affinity.
	pinned, ok := pool.affinity.Load("conversation-1")
	if !ok {
		t.Fatal("conversation affinity was not stored")
	}
	occupied := leaseAccount(pinned.(*accountClient))
	defer occupied.release()

	second, err := pool.acquire("", "", false, "conversation-1")
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	defer second.release()
	if second.accountID != firstID {
		t.Fatalf("conversation moved from account %q to %q", firstID, second.accountID)
	}
}

func TestClientPoolForgetConversationAllowsRebalance(t *testing.T) {
	pool, err := newClientPool("https://claude.ai", []config.Account{
		{SessionKey: "account-a"},
		{SessionKey: "account-b"},
	})
	if err != nil {
		t.Fatalf("newClientPool: %v", err)
	}

	first, err := pool.acquire("", "", false, "conversation-1")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	firstID := first.accountID
	first.release()
	pinned, ok := pool.affinity.Load("conversation-1")
	if !ok {
		t.Fatal("conversation affinity was not stored")
	}
	pool.forgetConversation("conversation-1")

	occupied := leaseAccount(pinned.(*accountClient))
	defer occupied.release()

	second, err := pool.acquire("", "", false, "conversation-1")
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	defer second.release()
	if second.accountID == firstID {
		t.Fatalf("expected forgotten conversation to rebalance away from busy account %q", firstID)
	}
}

func TestClientPoolConcurrentConversationAffinity(t *testing.T) {
	pool, err := newClientPool("https://claude.ai", []config.Account{
		{SessionKey: "account-a"},
		{SessionKey: "account-b"},
		{SessionKey: "account-c"},
	})
	if err != nil {
		t.Fatalf("newClientPool: %v", err)
	}

	const workers = 64
	ids := make(chan string, workers)
	start := make(chan struct{})
	releaseAll := make(chan struct{})
	var acquired sync.WaitGroup
	var released sync.WaitGroup
	acquired.Add(workers)
	released.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer released.Done()
			<-start
			lease, err := pool.acquire("", "", false, "conversation-1")
			if err != nil {
				t.Errorf("acquire: %v", err)
				acquired.Done()
				return
			}
			ids <- lease.accountID
			acquired.Done()
			<-releaseAll
			lease.release()
		}()
	}
	close(start)
	acquired.Wait()
	close(ids)

	var expected string
	for id := range ids {
		if expected == "" {
			expected = id
		}
		if id != expected {
			t.Fatalf("conversation assigned to multiple accounts: %q and %q", expected, id)
		}
	}
	for _, account := range pool.accounts {
		want := int64(0)
		if account.id == expected {
			want = workers
		}
		if active := account.active.Load(); active != want {
			t.Fatalf("account %q active leases = %d, want %d", account.id, active, want)
		}
	}

	close(releaseAll)
	released.Wait()
	for _, account := range pool.accounts {
		if active := account.active.Load(); active != 0 {
			t.Fatalf("account %q leaked active leases: %d", account.id, active)
		}
	}
}

func TestClientPoolBalancesConcurrentConfiguredRequests(t *testing.T) {
	pool, err := newClientPool("https://claude.ai", []config.Account{
		{SessionKey: "account-a"},
		{SessionKey: "account-b"},
		{SessionKey: "account-c"},
		{SessionKey: "account-d"},
	})
	if err != nil {
		t.Fatalf("newClientPool: %v", err)
	}

	const workers = 64
	start := make(chan struct{})
	releaseAll := make(chan struct{})
	var acquired sync.WaitGroup
	var released sync.WaitGroup
	acquired.Add(workers)
	released.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer released.Done()
			<-start
			lease, err := pool.acquireConfigured()
			if err != nil {
				t.Errorf("acquire: %v", err)
				acquired.Done()
				return
			}
			acquired.Done()
			<-releaseAll
			lease.release()
		}()
	}
	close(start)
	acquired.Wait()

	for _, account := range pool.accounts {
		if active := account.active.Load(); active != workers/int64(len(pool.accounts)) {
			t.Fatalf("account %q active leases = %d, want %d", account.id, active, workers/len(pool.accounts))
		}
	}
	close(releaseAll)
	released.Wait()
}

func TestClientPoolSkipsRateLimitedAccount(t *testing.T) {
	pool, err := newClientPool("https://claude.ai", []config.Account{
		{SessionKey: "account-a"},
		{SessionKey: "account-b"},
	}, time.Minute)
	if err != nil {
		t.Fatalf("newClientPool: %v", err)
	}
	pool.observe(pool.accounts[0].id, &claude.HTTPError{
		StatusCode:    429,
		RetryAfter:    time.Minute,
		RetryAfterSet: true,
	})

	lease, err := pool.acquireConfigured()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer lease.release()
	if lease.accountID != pool.accounts[1].id {
		t.Fatalf("selected cooling account %q", lease.accountID)
	}
}

func TestClientPoolReenablesAccountAfterCooldown(t *testing.T) {
	pool, err := newClientPool("https://claude.ai", []config.Account{
		{SessionKey: "account-a"},
		{SessionKey: "account-b"},
	}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("newClientPool: %v", err)
	}
	pool.observe(pool.accounts[0].id, &claude.HTTPError{
		StatusCode:    429,
		RetryAfter:    10 * time.Millisecond,
		RetryAfterSet: true,
	})
	time.Sleep(20 * time.Millisecond)

	lease, err := pool.acquireConfigured()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer lease.release()
	if lease.accountID != pool.accounts[0].id {
		t.Fatalf("expired cooldown was not eligible: %q", lease.accountID)
	}
}

func TestClientPoolReturnsShortestRetryAfterWhenAllAccountsAreCooling(t *testing.T) {
	pool, err := newClientPool("https://claude.ai", []config.Account{
		{SessionKey: "account-a"},
		{SessionKey: "account-b"},
	}, time.Minute)
	if err != nil {
		t.Fatalf("newClientPool: %v", err)
	}
	pool.observe(pool.accounts[0].id, &claude.HTTPError{StatusCode: 429, RetryAfter: 80 * time.Millisecond, RetryAfterSet: true})
	pool.observe(pool.accounts[1].id, &claude.HTTPError{StatusCode: 429, RetryAfter: 160 * time.Millisecond, RetryAfterSet: true})

	_, err = pool.acquireConfigured()
	var rateLimitErr *accountRateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("error = %v, want account rate-limit error", err)
	}
	if rateLimitErr.retryAfter <= 0 || rateLimitErr.retryAfter > 100*time.Millisecond {
		t.Fatalf("unexpected retry delay: %v", rateLimitErr.retryAfter)
	}
}

func TestClientPoolPinnedConversationReturnsRateLimitWhileCooling(t *testing.T) {
	pool, err := newClientPool("https://claude.ai", []config.Account{
		{SessionKey: "account-a"},
		{SessionKey: "account-b"},
	}, time.Minute)
	if err != nil {
		t.Fatalf("newClientPool: %v", err)
	}
	first, err := pool.acquire("", "", false, "conversation-1")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	pinnedID := first.accountID
	first.release()
	pool.observe(pinnedID, &claude.HTTPError{StatusCode: 429, RetryAfter: time.Minute, RetryAfterSet: true})

	_, err = pool.acquire("", "", false, "conversation-1")
	var rateLimitErr *accountRateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("error = %v, want account rate-limit error", err)
	}
}

func TestClientPoolReloadAddsAndRemovesAccounts(t *testing.T) {
	pool, err := newClientPool("https://claude.ai", []config.Account{
		{SessionKey: "account-a"},
		{SessionKey: "account-b"},
	}, time.Minute)
	if err != nil {
		t.Fatalf("newClientPool: %v", err)
	}
	original := pool.accounts[0].client

	changed, err := pool.reloadConfigured([]config.Account{
		{SessionKey: "account-a"},
		{SessionKey: "account-c"},
	})
	if err != nil || !changed {
		t.Fatalf("reload: changed=%v err=%v", changed, err)
	}
	if len(pool.accounts) != 2 {
		t.Fatalf("configured accounts = %d, want 2", len(pool.accounts))
	}
	if pool.accounts[0].client != original {
		t.Fatal("unchanged account client was recreated")
	}
	if pool.accountByID[credentialID("account-b", "")].configured.Load() {
		t.Fatal("removed account remains configured")
	}
	if _, ok := pool.accountByID[credentialID("account-c", "")]; !ok {
		t.Fatal("new account was not registered")
	}
}

func TestConversationStoreScopesAccountsAndSerializesTurns(t *testing.T) {
	store := newConversationStore()
	accountA, _, releaseA := store.acquire("account-a", "conversation-1")
	accountA.ClaudeConversationID = "upstream-a"
	releaseA()

	accountB, createdB, releaseB := store.acquire("account-b", "conversation-1")
	if !createdB {
		t.Fatal("same client conversation ID must create independent state for another account")
	}
	accountB.ClaudeConversationID = "upstream-b"
	releaseB()

	locked, _, releaseLocked := store.acquire("account-a", "conversation-1")
	if locked.ClaudeConversationID != "upstream-a" {
		t.Fatalf("unexpected account A mapping: %q", locked.ClaudeConversationID)
	}

	acquired := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, release := store.acquire("account-a", "conversation-1")
		close(acquired)
		release()
	}()

	select {
	case <-acquired:
		t.Fatal("same conversation was acquired concurrently")
	default:
	}
	releaseLocked()
	wg.Wait()
}
