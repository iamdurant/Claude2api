package handlers

import (
	"sync"
	"time"
)

type conversationState struct {
	ClientConversationID string
	ClaudeConversationID string
	LastHumanUUID        string
	LastAssistantUUID    string
	UpdatedAt            time.Time
}

type conversationStore struct {
	mu   sync.RWMutex
	data map[string]*conversationState
}

func newConversationStore() *conversationStore {
	return &conversationStore{data: make(map[string]*conversationState)}
}

func (s *conversationStore) get(id string) (*conversationState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.data[id]
	return state, ok
}

func (s *conversationStore) set(state *conversationState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state.UpdatedAt = time.Now()
	s.data[state.ClientConversationID] = state
}

func (s *conversationStore) touch(id, humanUUID, assistantUUID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state, ok := s.data[id]; ok {
		state.LastHumanUUID = humanUUID
		state.LastAssistantUUID = assistantUUID
		state.UpdatedAt = time.Now()
	}
}

func (s *conversationStore) delete(id string) (*conversationState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.data[id]
	if ok {
		delete(s.data, id)
	}
	return state, ok
}
