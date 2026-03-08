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
}

func NewInMemoryMessageRepository() *InMemoryMessageRepository {
	return &InMemoryMessageRepository{
		messages: make([]Message, 0, 64),
		index:    make(map[string]int),
	}
}

func (r *InMemoryMessageRepository) Create(_ context.Context, message Message) (Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

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

var _ MessageRepository = (*InMemoryMessageRepository)(nil)
