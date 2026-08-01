package handlers

import (
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"

	"claude2api/claude"
	"claude2api/config"
)

type accountClient struct {
	id     string
	client *claude.Client
	active atomic.Int64
}

type clientLease struct {
	accountID string
	client    *claude.Client
	release   func()
}

type clientPool struct {
	baseURL string

	accounts []*accountClient
	next     atomic.Uint64
	routeMu  sync.Mutex
	affinity sync.Map // conversation id -> *accountClient

	clientsMu sync.Mutex
	clients   sync.Map // credential id -> *claude.Client
}

func newClientPool(baseURL string, accounts []config.Account) (*clientPool, error) {
	p := &clientPool{baseURL: baseURL}
	for _, account := range accounts {
		client, err := claude.NewClient(baseURL, account.SessionKey, account.Cookie)
		if err != nil {
			return nil, fmt.Errorf("create account client: %w", err)
		}
		p.accounts = append(p.accounts, &accountClient{
			id:     credentialID(account.SessionKey, account.Cookie),
			client: client,
		})
	}
	return p, nil
}

func (p *clientPool) acquire(sessionKey, cookie string, explicit bool, conversationID string) (*clientLease, error) {
	if !explicit && len(p.accounts) > 0 {
		if conversationID != "" {
			return p.acquireConfiguredConversation(conversationID), nil
		}
		return p.acquireConfigured(), nil
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

func (p *clientPool) acquireConfiguredConversation(conversationID string) *clientLease {
	if value, ok := p.affinity.Load(conversationID); ok {
		return leaseAccount(value.(*accountClient))
	}

	p.routeMu.Lock()
	defer p.routeMu.Unlock()
	if value, ok := p.affinity.Load(conversationID); ok {
		return leaseAccount(value.(*accountClient))
	}
	selected := p.selectConfigured()
	p.affinity.Store(conversationID, selected)
	return leaseAccount(selected)
}

func (p *clientPool) forgetConversation(conversationID string) {
	if conversationID != "" {
		p.affinity.Delete(conversationID)
	}
}

func (p *clientPool) acquireConfigured() *clientLease {
	p.routeMu.Lock()
	defer p.routeMu.Unlock()
	return leaseAccount(p.selectConfigured())
}

func (p *clientPool) selectConfigured() *accountClient {
	start := int(p.next.Add(1)-1) % len(p.accounts)
	selected := p.accounts[start]
	selectedLoad := selected.active.Load()
	for i := 1; i < len(p.accounts); i++ {
		candidate := p.accounts[(start+i)%len(p.accounts)]
		if load := candidate.active.Load(); load < selectedLoad {
			selected = candidate
			selectedLoad = load
		}
	}
	return selected
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

func credentialID(sessionKey, cookie string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(sessionKey))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(cookie))
	return fmt.Sprintf("%016x", h.Sum64())
}
