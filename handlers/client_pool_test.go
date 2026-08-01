package handlers

import (
	"sync"
	"testing"

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

	first := pool.acquireConfigured()
	second := pool.acquireConfigured()
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
			lease := pool.acquireConfigured()
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
