package main

import (
	"bufio"
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"
)

func TestSessionAcceptsMessageAndStoresIt(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	s := newSession(server, repo, slog.New(slog.DiscardHandler), DefaultSMTPConfig())
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "HELO localhost")
	readHasCode(t, r, "250")

	writeLine(t, w, "MAIL FROM:<from@example.com>")
	readHasCode(t, r, "250")

	writeLine(t, w, "RCPT TO:<to@example.com>")
	readHasCode(t, r, "250")

	writeLine(t, w, "DATA")
	readHasCode(t, r, "354")

	writeRaw(t, w, "Subject: Test\r\n")
	writeRaw(t, w, "From: from@example.com\r\n")
	writeRaw(t, w, "To: to@example.com\r\n")
	writeRaw(t, w, "\r\n")
	writeRaw(t, w, "Hello from test\r\n")
	writeRaw(t, w, ".\r\n")
	readHasCode(t, r, "250")

	count, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("count mismatch: got %d want 1", count)
	}

	messages, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if got := messages[0].Subject; got != "Test" {
		t.Fatalf("subject mismatch: got %q want %q", got, "Test")
	}

	writeLine(t, w, "QUIT")
	readHasCode(t, r, "221")
}

func TestSessionRejectsDataBeforeMailAndRcpt(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	s := newSession(server, repo, slog.New(slog.DiscardHandler), DefaultSMTPConfig())
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "HELO localhost")
	readHasCode(t, r, "250")

	writeLine(t, w, "DATA")

	resp := readLine(t, r)
	if !strings.HasPrefix(resp, "503") {
		t.Fatalf("expected 503 for DATA before MAIL/RCPT, got %q", resp)
	}
}

func TestSessionRejectsTooManyRecipients(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.MaxRecipients = 1
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "HELO localhost")
	readHasCode(t, r, "250")

	writeLine(t, w, "MAIL FROM:<from@example.com>")
	readHasCode(t, r, "250")

	writeLine(t, w, "RCPT TO:<to1@example.com>")
	readHasCode(t, r, "250")

	writeLine(t, w, "RCPT TO:<to2@example.com>")
	resp := readLine(t, r)
	if !strings.HasPrefix(resp, "452") {
		t.Fatalf("expected 452 for second RCPT, got %q", resp)
	}
}

func TestSessionRejectsOversizedData(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.MaxMessageBytes = 48
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "HELO localhost")
	readHasCode(t, r, "250")

	writeLine(t, w, "MAIL FROM:<from@example.com>")
	readHasCode(t, r, "250")

	writeLine(t, w, "RCPT TO:<to@example.com>")
	readHasCode(t, r, "250")

	writeLine(t, w, "DATA")
	readHasCode(t, r, "354")

	writeRaw(t, w, "Subject: Too Large\r\n")
	writeRaw(t, w, "\r\n")
	writeRaw(t, w, "0123456789012345678901234567890123456789\r\n")
	writeRaw(t, w, ".\r\n")

	resp := readLine(t, r)
	if !strings.HasPrefix(resp, "552") {
		t.Fatalf("expected 552 for oversized DATA, got %q", resp)
	}

	count, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 stored messages after oversized DATA, got %d", count)
	}
}

func TestSessionEHLOAdvertisesCapabilities(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.MaxMessageBytes = 2048
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "EHLO localhost")

	lines := readMultilineResponse(t, r, "250")
	if len(lines) < 4 {
		t.Fatalf("expected multiline EHLO response with capabilities, got %v", lines)
	}

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "PIPELINING") {
		t.Fatalf("expected PIPELINING capability in %q", joined)
	}
	if !strings.Contains(joined, "8BITMIME") {
		t.Fatalf("expected 8BITMIME capability in %q", joined)
	}
	if !strings.Contains(joined, "SIZE 2048") {
		t.Fatalf("expected SIZE 2048 capability in %q", joined)
	}
}

func TestSessionEHLOAdvertisesUnlimitedSize(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.MaxMessageBytes = 0
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "EHLO localhost")

	lines := readMultilineResponse(t, r, "250")
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "SIZE") {
		t.Fatalf("expected SIZE capability in %q", joined)
	}
	if strings.Contains(joined, "SIZE 0") {
		t.Fatalf("did not expect SIZE 0 capability in %q", joined)
	}
}

func readHasCode(t *testing.T, r *bufio.Reader, code string) {
	t.Helper()

	resp := readLine(t, r)
	if !strings.HasPrefix(resp, code) {
		t.Fatalf("expected response %s, got %q", code, resp)
	}
}

func readMultilineResponse(t *testing.T, r *bufio.Reader, code string) []string {
	t.Helper()

	lines := make([]string, 0, 4)
	for {
		line := readLine(t, r)
		if !strings.HasPrefix(line, code) {
			t.Fatalf("expected response %s, got %q", code, line)
		}
		if len(line) < 4 {
			t.Fatalf("invalid SMTP response line: %q", line)
		}

		lines = append(lines, line)
		if line[3] == ' ' {
			break
		}
		if line[3] != '-' {
			t.Fatalf("invalid SMTP multiline separator in line: %q", line)
		}
	}

	return lines
}

func readLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()

	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read response failed: %v", err)
	}

	return strings.TrimRight(line, "\r\n")
}

func writeLine(t *testing.T, w *bufio.Writer, line string) {
	t.Helper()
	writeRaw(t, w, line+"\r\n")
}

func writeRaw(t *testing.T, w *bufio.Writer, data string) {
	t.Helper()

	_, err := w.WriteString(data)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	err = w.Flush()
	if err != nil {
		t.Fatalf("flush failed: %v", err)
	}
}
