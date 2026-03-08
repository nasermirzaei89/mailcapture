package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	err := run()
	if err != nil {
		slog.Error("run failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	var smtpAddr string
	var httpAddr string
	var maxMessageBytes int
	var maxRecipients int
	var maxMessages int
	var smtpTLSCertFile string
	var smtpTLSKeyFile string
	var smtpAuthUsername string
	var smtpAuthPassword string
	var allowInsecureAuth bool
	var requireAuth bool

	flag.StringVar(&smtpAddr, "smtp-addr", ":1025", "SMTP listen address")
	flag.StringVar(&httpAddr, "http-addr", ":8080", "HTTP listen address")
	flag.IntVar(&maxMessageBytes, "max-message-bytes", 10*1024*1024, "Maximum accepted SMTP DATA size in bytes (0 disables limit)")
	flag.IntVar(&maxRecipients, "max-recipients", 100, "Maximum RCPT TO recipients per message (0 disables limit)")
	flag.IntVar(&maxMessages, "max-messages", 1000, "Maximum retained messages in memory (0 disables limit)")
	flag.StringVar(&smtpTLSCertFile, "smtp-tls-cert-file", "", "Path to TLS certificate PEM file for STARTTLS")
	flag.StringVar(&smtpTLSKeyFile, "smtp-tls-key-file", "", "Path to TLS private key PEM file for STARTTLS")
	flag.StringVar(&smtpAuthUsername, "smtp-auth-username", "", "SMTP AUTH username (enables AUTH when set with password)")
	flag.StringVar(&smtpAuthPassword, "smtp-auth-password", "", "SMTP AUTH password (enables AUTH when set with username)")
	flag.BoolVar(&allowInsecureAuth, "allow-insecure-auth", false, "Allow AUTH before TLS (development only)")
	flag.BoolVar(&requireAuth, "require-auth", false, "Require successful AUTH before accepting MAIL FROM")
	flag.Parse()

	logger := slog.Default()

	if (smtpTLSCertFile == "") != (smtpTLSKeyFile == "") {
		return fmt.Errorf("both --smtp-tls-cert-file and --smtp-tls-key-file must be provided together")
	}

	authConfigured := smtpAuthUsername != "" || smtpAuthPassword != ""
	if authConfigured && (smtpAuthUsername == "" || smtpAuthPassword == "") {
		return fmt.Errorf("both --smtp-auth-username and --smtp-auth-password must be provided together")
	}
	if requireAuth && !authConfigured {
		return fmt.Errorf("--require-auth requires --smtp-auth-username and --smtp-auth-password")
	}

	var smtpTLSConfig *tls.Config
	if smtpTLSCertFile != "" {
		cert, err := tls.LoadX509KeyPair(smtpTLSCertFile, smtpTLSKeyFile)
		if err != nil {
			return fmt.Errorf("load smtp tls keypair: %w", err)
		}

		smtpTLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	repo := NewInMemoryMessageRepositoryWithLimit(maxMessages)
	smtpServer := NewSMTPServerWithConfig(smtpAddr, repo, logger, SMTPConfig{
		MaxMessageBytes:   maxMessageBytes,
		MaxRecipients:     maxRecipients,
		TLSConfig:         smtpTLSConfig,
		AuthUsername:      smtpAuthUsername,
		AuthPassword:      smtpAuthPassword,
		AllowInsecureAuth: allowInsecureAuth,
		RequireAuth:       requireAuth,
	})
	webServer, err := NewWebServer(httpAddr, repo, logger)
	if err != nil {
		return fmt.Errorf("setup http server: %w", err)
	}

	err = smtpServer.Start()
	if err != nil {
		return fmt.Errorf("start smtp server: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		if serveErr := webServer.Start(); serveErr != nil {
			errCh <- serveErr
		}
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-signalCtx.Done():
		logger.Info("shutdown signal received")
	case serveErr := <-errCh:
		logger.Error("http server error", "error", serveErr)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if shutErr := smtpServer.Shutdown(shutdownCtx); shutErr != nil {
			logger.Error("smtp shutdown error", "error", shutErr)
		}
	}()
	go func() {
		defer wg.Done()
		if shutErr := webServer.Shutdown(shutdownCtx); shutErr != nil && shutErr != http.ErrServerClosed {
			logger.Error("http shutdown error", "error", shutErr)
		}
	}()
	wg.Wait()

	logger.Info("shutdown complete")

	return nil
}
