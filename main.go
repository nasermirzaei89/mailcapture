package main

import (
	"context"
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

	flag.StringVar(&smtpAddr, "smtp-addr", ":1025", "SMTP listen address")
	flag.StringVar(&httpAddr, "http-addr", ":8080", "HTTP listen address")
	flag.IntVar(&maxMessageBytes, "max-message-bytes", 10*1024*1024, "Maximum accepted SMTP DATA size in bytes (0 disables limit)")
	flag.IntVar(&maxRecipients, "max-recipients", 100, "Maximum RCPT TO recipients per message (0 disables limit)")
	flag.IntVar(&maxMessages, "max-messages", 1000, "Maximum retained messages in memory (0 disables limit)")
	flag.Parse()

	logger := slog.Default()

	repo := NewInMemoryMessageRepositoryWithLimit(maxMessages)
	smtpServer := NewSMTPServerWithConfig(smtpAddr, repo, logger, SMTPConfig{
		MaxMessageBytes: maxMessageBytes,
		MaxRecipients:   maxRecipients,
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
