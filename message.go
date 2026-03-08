package main

import (
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
