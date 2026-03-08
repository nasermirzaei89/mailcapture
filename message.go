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
	Create(ctx context.Context, message Message) (Message, error)
	GetByID(ctx context.Context, id string) (Message, bool, error)
	List(ctx context.Context) ([]Message, error)
	Count(ctx context.Context) (int, error)
	DeleteByID(ctx context.Context, id string) (bool, error)
	DeleteAll(ctx context.Context) error
}
