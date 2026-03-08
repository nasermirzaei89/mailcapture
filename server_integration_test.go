package main

import (
	"context"
	"log/slog"
	"net/smtp"
	"testing"
	"time"
)

func TestServerAcceptsNetSMTPClient(t *testing.T) {
	repo := NewInMemoryMessageRepository()
	server := NewSMTPServer("127.0.0.1:0", repo, slog.New(slog.DiscardHandler))

	err := server.Start()
	if err != nil {
		t.Fatalf("start smtp server: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	msg := []byte("Subject: Integration Test\r\nFrom: from@example.com\r\nTo: to@example.com\r\n\r\nHello integration\r\n")

	err = smtp.SendMail(server.Addr(), nil, "from@example.com", []string{"to@example.com"}, msg)
	if err != nil {
		t.Fatalf("smtp send failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		count, err := repo.Count(context.Background())
		if err != nil {
			t.Fatalf("count failed: %v", err)
		}
		if count == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("message was not stored")
}
