package handlers

import (
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"

	"claude2api/claude"
	"claude2api/config"
)

type accountClient struct {
	id            string
	client        *claude.Client
	active        atomic.Int64
	configured    atomic.Bool
	cooldownUntil atomic.Int64
}

type clientLease struct {
	accountID string
	client    *claude.Client
	release   func()
}

type clientPool struct {
	baseURL           string
	rateLimitCooldown time.Duration
	accountByID       map[string]*accountClient

	accounts []*accountClient
	next     atomic.Uint64
	routeMu  sync.Mutex
	affinity sync.Map // conversation id -> *accountClient

	clientsMu sync.Mutex
	clients   sync.Map // credential id -> *claude.Client
}

type accountRateLimitError struct {
	retryAfter time.Duration
}

func (e *accountRateLimitError) Error() string {
	return fmt.Sprintf("all configured accounts are rate-limited; retry after %s", e.retryAfter.Round(time.Millisecond))
}

type accountUnavailableError struct {
	accountID string
}

func (e *accountUnavailableError) Error() string {
	if e.accountID == "" {
		return "no configured account is available"
	}
	return fmt.Sprintf("configured account %s is unavailable", e.accountID)
}

func newClientPool(baseURL string, accounts []config.Account, cooldown ...time.Duration) (*clientPool, error) {
	rateLimitCooldown := 60 * time.Second
	if len(cooldown) > 0 && cooldown[0] > 0 {
		rateLimitCooldown = cooldown[0]
	}
	p := &clientPool{
		baseURL:           baseURL,
		rateLimitCooldown: rateLimitCooldown,
		accountByID:       make(map[string]*accountClient),
	}
	if _, err := p.reloadConfigured(accounts); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *clientPool) acquire(sessionKey, cookie string, explicit bool, conversationID string) (*clientLease, error) {
	if !explicit {
		if conversationID != "" {
			return p.acquireConfiguredConversation(conversationID)
		}
		return p.acquireConfigured()
	}
	if sessionKey == "" {
		return nil, fmt.Errorf("missing account credentials")
	}

	id := credentialID(sessionKey, cookie)
	if value, ok := p.clients.Load(id); ok {
		return &clientLease{accountID: id, client: value.(*claude.Client), release: func() {}}, nil
	}

	// tls-client construction is relatively expensive. Serialize cache misses so
	// concurrent first requests for the same credentials create exactly one client.
	p.clientsMu.Lock()
	defer p.clientsMu.Unlock()
	if value, ok := p.clients.Load(id); ok {
		return &clientLease{accountID: id, client: value.(*claude.Client), release: func() {}}, nil
	}
	client, err := claude.NewClient(p.baseURL, sessionKey, cookie)
	if err != nil {
		return nil, err
	}
	p.clients.Store(id, client)
	return &clientLease{accountID: id, client: client, release: func() {}}, nil
}

func (p *clientPool) acquireConfiguredConversation(conversationID string) (*clientLease, error) {
	p.routeMu.Lock()
	defer p.routeMu.Unlock()
	if value, ok := p.affinity.Load(conversationID); ok {
		return p.leaseConfiguredAccount(value.(*accountClient))
	}

	selected, err := p.selectConfigured()
	if err != nil {
		return nil, err
	}
	p.affinity.Store(conversationID, selected)
	return leaseAccount(selected), nil
}

func (p *clientPool) forgetConversation(conversationID string) {
	if conversationID != "" {
		p.affinity.Delete(conversationID)
	}
}

func (p *clientPool) acquireConfigured() (*clientLease, error) {
	p.routeMu.Lock()
	defer p.routeMu.Unlock()
	selected, err := p.selectConfigured()
	if err != nil {
		return nil, err
	}
	return leaseAccount(selected), nil
}

func (p *clientPool) leaseConfiguredAccount(account *accountClient) (*clientLease, error) {
	if !account.configured.Load() {
		return nil, &accountUnavailableError{accountID: account.id}
	}
	if remaining := account.cooldownRemaining(time.Now()); remaining > 0 {
		return nil, &accountRateLimitError{retryAfter: remaining}
	}
	return leaseAccount(account), nil
}

func (p *clientPool) selectConfigured() (*accountClient, error) {
	if len(p.accounts) == 0 {
		return nil, &accountUnavailableError{}
	}
	start := int(p.next.Add(1)-1) % len(p.accounts)
	var selected *accountClient
	var selectedLoad int64
	var shortestCooldown time.Duration
	now := time.Now()
	for i := 0; i < len(p.accounts); i++ {
		candidate := p.accounts[(start+i)%len(p.accounts)]
		if !candidate.configured.Load() {
			continue
		}
		if remaining := candidate.cooldownRemaining(now); remaining > 0 {
			if shortestCooldown == 0 || remaining < shortestCooldown {
				shortestCooldown = remaining
			}
			continue
		}
		load := candidate.active.Load()
		if selected == nil || load < selectedLoad {
			selected = candidate
			selectedLoad = load
		}
	}
	if selected != nil {
		return selected, nil
	}
	if shortestCooldown > 0 {
		return nil, &accountRateLimitError{retryAfter: shortestCooldown}
	}
	return nil, &accountUnavailableError{}
}

func leaseAccount(account *accountClient) *clientLease {
	account.active.Add(1)
	return &clientLease{
		accountID: account.id,
		client:    account.client,
		release: func() {
			account.active.Add(-1)
		},
	}
}

func (p *clientPool) reloadConfigured(accounts []config.Account) (bool, error) {
	p.routeMu.Lock()
	defer p.routeMu.Unlock()

	next := make([]*accountClient, 0, len(accounts))
	seen := make(map[string]struct{}, len(accounts))
	newAccounts := make(map[string]*accountClient)
	for _, account := range accounts {
		id := credentialID(account.SessionKey, account.Cookie)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		configured, ok := p.accountByID[id]
		if !ok {
			client, err := claude.NewClient(p.baseURL, account.SessionKey, account.Cookie)
			if err != nil {
				return false, fmt.Errorf("create account client: %w", err)
			}
			configured = &accountClient{id: id, client: client}
			newAccounts[id] = configured
		}
		next = append(next, configured)
	}

	changed := len(p.accounts) != len(next)
	if !changed {
		for i, account := range next {
			if p.accounts[i].id != account.id {
				changed = true
				break
			}
		}
	}
	for id, account := range newAccounts {
		p.accountByID[id] = account
	}
	configuredIDs := make(map[string]struct{}, len(next))
	for _, account := range next {
		account.configured.Store(true)
		configuredIDs[account.id] = struct{}{}
	}
	for id, account := range p.accountByID {
		if _, ok := configuredIDs[id]; !ok {
			account.configured.Store(false)
		}
	}
	p.accounts = next
	return changed, nil
}

func (p *clientPool) observe(accountID string, err error) {
	if !claude.IsStatus(err, 429) {
		return
	}

	p.routeMu.Lock()
	account := p.accountByID[accountID]
	p.routeMu.Unlock()
	if account == nil {
		return
	}

	delay, ok := claude.RetryAfterOf(err)
	if !ok || delay <= 0 {
		delay = p.rateLimitCooldown
	}
	until := time.Now().Add(delay).UnixNano()
	for {
		current := account.cooldownUntil.Load()
		if until <= current || account.cooldownUntil.CompareAndSwap(current, until) {
			return
		}
	}
}

func (p *clientPool) cooldownRemaining(accountID string) time.Duration {
	p.routeMu.Lock()
	account := p.accountByID[accountID]
	p.routeMu.Unlock()
	if account == nil {
		return 0
	}
	return account.cooldownRemaining(time.Now())
}

func (account *accountClient) cooldownRemaining(now time.Time) time.Duration {
	until := account.cooldownUntil.Load()
	if until == 0 {
		return 0
	}
	remaining := time.Unix(0, until).Sub(now)
	if remaining <= 0 {
		return 0
	}
	return remaining
}

func credentialID(sessionKey, cookie string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(sessionKey))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(cookie))
	return fmt.Sprintf("%016x", h.Sum64())
}
