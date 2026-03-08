package main

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
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

func TestServerStartTLSAndAuth(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.TLSConfig = newTestTLSConfig(t)
	cfg.AuthUsername = "user"
	cfg.AuthPassword = "pass"
	cfg.RequireAuth = true
	server := NewSMTPServerWithConfig("127.0.0.1:0", repo, slog.New(slog.DiscardHandler), cfg)

	err := server.Start()
	if err != nil {
		t.Fatalf("start smtp server: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	client, err := smtp.Dial(server.Addr())
	if err != nil {
		t.Fatalf("smtp dial failed: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	host, _, err := net.SplitHostPort(server.Addr())
	if err != nil {
		t.Fatalf("split host/port failed: %v", err)
	}

	err = client.StartTLS(&tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("starttls failed: %v", err)
	}

	err = client.Auth(smtp.PlainAuth("", "user", "pass", host))
	if err != nil {
		t.Fatalf("auth failed: %v", err)
	}

	err = client.Mail("from@example.com")
	if err != nil {
		t.Fatalf("mail failed: %v", err)
	}

	err = client.Rcpt("to@example.com")
	if err != nil {
		t.Fatalf("rcpt failed: %v", err)
	}

	wc, err := client.Data()
	if err != nil {
		t.Fatalf("data failed: %v", err)
	}

	_, err = io.WriteString(wc, "Subject: TLS Auth Integration\r\nFrom: from@example.com\r\nTo: to@example.com\r\n\r\nhello\r\n")
	if err != nil {
		t.Fatalf("write data failed: %v", err)
	}

	err = wc.Close()
	if err != nil {
		t.Fatalf("close data writer failed: %v", err)
	}

	err = client.Quit()
	if err != nil {
		t.Fatalf("quit failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		count, countErr := repo.Count(context.Background())
		if countErr != nil {
			t.Fatalf("count failed: %v", countErr)
		}
		if count == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("message was not stored")
}
