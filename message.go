package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Message stores SMTP envelope data and parsed RFC5322 content.
type Message struct {
	ID           string
	ReceivedAt   time.Time
	EnvelopeFrom string
	EnvelopeTo   []string
	HeaderFrom   string
	HeaderTo     []string
	Subject      string
	Body         string
	Raw          string
}

func NewMessageID() (string, error) {
	var b [8]byte

	_, err := rand.Read(b[:])
	if err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}

	return hex.EncodeToString(b[:]), nil
}

// MessageRepository defines storage operations for received messages.
type MessageRepository interface {
	Create(ctx context.Context, message Message) (err error)
	GetByID(ctx context.Context, id string) (message Message, err error)
	List(ctx context.Context) (messages []Message, err error)
	Count(ctx context.Context) (count int, err error)
	DeleteByID(ctx context.Context, id string) (err error)
	DeleteAll(ctx context.Context) (err error)
}

type MessageNotFoundError struct {
	ID string
}

func (err MessageNotFoundError) Error() string {
	return fmt.Sprintf("message with id %q not found", err.ID)
}
