package main

import (
	"context"
	"sync"
)

// InMemoryMessageRepository is a thread-safe message store.
type InMemoryMessageRepository struct {
	mu       sync.RWMutex
	messages []Message
	index    map[string]int
	maxItems int
}

func NewInMemoryMessageRepository() *InMemoryMessageRepository {
	return NewInMemoryMessageRepositoryWithLimit(0)
}

func NewInMemoryMessageRepositoryWithLimit(maxItems int) *InMemoryMessageRepository {
	return &InMemoryMessageRepository{
		messages: make([]Message, 0, 64),
		index:    make(map[string]int),
		maxItems: maxItems,
	}
}

func (r *InMemoryMessageRepository) Create(_ context.Context, message Message) (Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.maxItems > 0 && len(r.messages) >= r.maxItems {
		r.messages = r.messages[1:]
		r.rebuildIndex()
	}

	r.messages = append(r.messages, message)
	r.index[message.ID] = len(r.messages) - 1
	return message, nil
}

func (r *InMemoryMessageRepository) GetByID(_ context.Context, id string) (Message, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	idx, ok := r.index[id]
	if !ok {
		return Message{}, false, nil
	}
	return r.messages[idx], true, nil
}

func (r *InMemoryMessageRepository) List(_ context.Context) ([]Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Message, 0, len(r.messages))
	for i := len(r.messages) - 1; i >= 0; i-- {
		out = append(out, r.messages[i])
	}
	return out, nil
}

func (r *InMemoryMessageRepository) Count(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.messages), nil
}

func (r *InMemoryMessageRepository) DeleteByID(_ context.Context, id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	idx, ok := r.index[id]
	if !ok {
		return false, nil
	}

	r.messages = append(r.messages[:idx], r.messages[idx+1:]...)
	r.rebuildIndex()

	return true, nil
}

func (r *InMemoryMessageRepository) DeleteAll(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.messages = r.messages[:0]
	r.index = make(map[string]int)

	return nil
}

func (r *InMemoryMessageRepository) rebuildIndex() {
	r.index = make(map[string]int, len(r.messages))
	for i, msg := range r.messages {
		r.index[msg.ID] = i
	}
}

var _ MessageRepository = (*InMemoryMessageRepository)(nil)
