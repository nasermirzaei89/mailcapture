package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
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

func TestSessionEHLOAdvertisesSTARTTLSAndHidesAUTHUntilTLS(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.TLSConfig = newTestTLSConfig(t)
	cfg.AuthUsername = "user"
	cfg.AuthPassword = "pass"
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "EHLO localhost")

	lines := readMultilineResponse(t, r, "250")
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "STARTTLS") {
		t.Fatalf("expected STARTTLS capability in %q", joined)
	}
	if strings.Contains(joined, "AUTH ") {
		t.Fatalf("did not expect AUTH capability before TLS in %q", joined)
	}
}

func TestSessionAUTHPlainSuccessAndStoreMessage(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.AuthUsername = "user"
	cfg.AuthPassword = "pass"
	cfg.AllowInsecureAuth = true
	cfg.RequireAuth = true
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "EHLO localhost")
	_ = readMultilineResponse(t, r, "250")

	authPayload := base64.StdEncoding.EncodeToString([]byte("\x00user\x00pass"))
	writeLine(t, w, "AUTH PLAIN "+authPayload)
	readHasCode(t, r, "235")

	writeLine(t, w, "MAIL FROM:<from@example.com>")
	readHasCode(t, r, "250")
	writeLine(t, w, "RCPT TO:<to@example.com>")
	readHasCode(t, r, "250")
	writeLine(t, w, "DATA")
	readHasCode(t, r, "354")
	writeRaw(t, w, "Subject: Auth Test\r\n\r\nhello\r\n.\r\n")
	readHasCode(t, r, "250")

	count, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("count mismatch: got %d want 1", count)
	}
}

func TestSessionAUTHLoginSuccess(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.AuthUsername = "user"
	cfg.AuthPassword = "pass"
	cfg.AllowInsecureAuth = true
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "EHLO localhost")
	_ = readMultilineResponse(t, r, "250")

	writeLine(t, w, "AUTH LOGIN")
	readHasCode(t, r, "334")
	writeLine(t, w, base64.StdEncoding.EncodeToString([]byte("user")))
	readHasCode(t, r, "334")
	writeLine(t, w, base64.StdEncoding.EncodeToString([]byte("pass")))
	readHasCode(t, r, "235")
}

func TestSessionRejectsMAILWhenAuthRequired(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.AuthUsername = "user"
	cfg.AuthPassword = "pass"
	cfg.AllowInsecureAuth = true
	cfg.RequireAuth = true
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "EHLO localhost")
	_ = readMultilineResponse(t, r, "250")

	writeLine(t, w, "MAIL FROM:<from@example.com>")
	resp := readLine(t, r)
	if !strings.HasPrefix(resp, "530") {
		t.Fatalf("expected 530 before AUTH, got %q", resp)
	}
}

func TestSessionRejectsAUTHWithoutTLSWhenInsecureDisabled(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.TLSConfig = newTestTLSConfig(t)
	cfg.AuthUsername = "user"
	cfg.AuthPassword = "pass"
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "EHLO localhost")
	_ = readMultilineResponse(t, r, "250")

	authPayload := base64.StdEncoding.EncodeToString([]byte("\x00user\x00pass"))
	writeLine(t, w, "AUTH PLAIN "+authPayload)
	resp := readLine(t, r)
	if !strings.HasPrefix(resp, "538") {
		t.Fatalf("expected 538 for AUTH without TLS, got %q", resp)
	}
}

func TestSessionSTARTTLSUpgradeRequiresNewEHLO(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.TLSConfig = newTestTLSConfig(t)
	cfg.AuthUsername = "user"
	cfg.AuthPassword = "pass"
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "EHLO localhost")
	beforeTLS := strings.Join(readMultilineResponse(t, r, "250"), "\n")
	if !strings.Contains(beforeTLS, "STARTTLS") {
		t.Fatalf("expected STARTTLS capability before upgrade in %q", beforeTLS)
	}

	writeLine(t, w, "STARTTLS")
	readHasCode(t, r, "220")

	tlsClient := tls.Client(client, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
	if err := tlsClient.Handshake(); err != nil {
		t.Fatalf("tls handshake failed: %v", err)
	}

	tr := bufio.NewReader(tlsClient)
	tw := bufio.NewWriter(tlsClient)

	writeLine(t, tw, "MAIL FROM:<from@example.com>")
	resp := readLine(t, tr)
	if !strings.HasPrefix(resp, "503") {
		t.Fatalf("expected 503 before post-TLS EHLO, got %q", resp)
	}

	writeLine(t, tw, "EHLO localhost")
	afterTLS := strings.Join(readMultilineResponse(t, tr, "250"), "\n")
	if strings.Contains(afterTLS, "STARTTLS") {
		t.Fatalf("did not expect STARTTLS after TLS upgrade in %q", afterTLS)
	}
	if !strings.Contains(afterTLS, "AUTH PLAIN LOGIN") {
		t.Fatalf("expected AUTH capability after TLS upgrade in %q", afterTLS)
	}
}

func TestSessionSTARTTLSWithoutConfig(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	s := newSession(server, repo, slog.New(slog.DiscardHandler), DefaultSMTPConfig())
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "EHLO localhost")
	_ = readMultilineResponse(t, r, "250")

	writeLine(t, w, "STARTTLS")
	resp := readLine(t, r)
	if !strings.HasPrefix(resp, "502") {
		t.Fatalf("expected 502 when STARTTLS is disabled, got %q", resp)
	}
}

func TestSessionSTARTTLSRequiresGreeting(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.TLSConfig = newTestTLSConfig(t)
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "STARTTLS")
	resp := readLine(t, r)
	if !strings.HasPrefix(resp, "503") {
		t.Fatalf("expected 503 before EHLO/HELO for STARTTLS, got %q", resp)
	}
}

func TestSessionAUTHUnsupportedMechanism(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.AuthUsername = "user"
	cfg.AuthPassword = "pass"
	cfg.AllowInsecureAuth = true
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "EHLO localhost")
	_ = readMultilineResponse(t, r, "250")

	writeLine(t, w, "AUTH CRAM-MD5")
	resp := readLine(t, r)
	if !strings.HasPrefix(resp, "501") {
		t.Fatalf("expected 501 for unsupported AUTH mechanism, got %q", resp)
	}
}

func TestSessionAUTHInvalidCredentials(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.AuthUsername = "user"
	cfg.AuthPassword = "pass"
	cfg.AllowInsecureAuth = true
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "EHLO localhost")
	_ = readMultilineResponse(t, r, "250")

	authPayload := base64.StdEncoding.EncodeToString([]byte("\x00user\x00wrong"))
	writeLine(t, w, "AUTH PLAIN "+authPayload)
	resp := readLine(t, r)
	if !strings.HasPrefix(resp, "535") {
		t.Fatalf("expected 535 for invalid AUTH credentials, got %q", resp)
	}
}

func TestSessionAUTHAlreadyAuthenticated(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()

	repo := NewInMemoryMessageRepository()
	cfg := DefaultSMTPConfig()
	cfg.AuthUsername = "user"
	cfg.AuthPassword = "pass"
	cfg.AllowInsecureAuth = true
	s := newSession(server, repo, slog.New(slog.DiscardHandler), cfg)
	go s.run(context.Background())

	r := bufio.NewReader(client)
	w := bufio.NewWriter(client)

	readHasCode(t, r, "220")
	writeLine(t, w, "EHLO localhost")
	_ = readMultilineResponse(t, r, "250")

	authPayload := base64.StdEncoding.EncodeToString([]byte("\x00user\x00pass"))
	writeLine(t, w, "AUTH PLAIN "+authPayload)
	readHasCode(t, r, "235")

	writeLine(t, w, "AUTH PLAIN "+authPayload)
	resp := readLine(t, r)
	if !strings.HasPrefix(resp, "503") {
		t.Fatalf("expected 503 when AUTH repeated after success, got %q", resp)
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

func newTestTLSConfig(t *testing.T) *tls.Config {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key failed: %v", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		t.Fatalf("generate serial number failed: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		DNSNames:              []string{"localhost"},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate failed: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("load x509 key pair failed: %v", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
}
