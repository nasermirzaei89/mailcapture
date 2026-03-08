package main

import (
	"context"
)

// MessageRepository defines storage operations for received messages.
type MessageRepository interface {
	Create(ctx context.Context, message Message) (Message, error)
	GetByID(ctx context.Context, id string) (Message, bool, error)
	List(ctx context.Context) ([]Message, error)
	Count(ctx context.Context) (int, error)
}
