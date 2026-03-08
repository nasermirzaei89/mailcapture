package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// SMTPServer is a minimal SMTP server for development mail capture.
type SMTPServer struct {
	addr   string
	repo   MessageRepository
	logger *slog.Logger
	config SMTPConfig

	listener net.Listener
	wg       sync.WaitGroup
}

type SMTPConfig struct {
	MaxMessageBytes int
	MaxRecipients   int
}

func DefaultSMTPConfig() SMTPConfig {
	return SMTPConfig{
		MaxMessageBytes: 10 * 1024 * 1024,
		MaxRecipients:   100,
	}
}

func NewSMTPServer(addr string, repo MessageRepository, logger *slog.Logger) *SMTPServer {
	return NewSMTPServerWithConfig(addr, repo, logger, DefaultSMTPConfig())
}

func NewSMTPServerWithConfig(addr string, repo MessageRepository, logger *slog.Logger, config SMTPConfig) *SMTPServer {
	if config.MaxRecipients < 0 {
		config.MaxRecipients = 0
	}
	if config.MaxMessageBytes < 0 {
		config.MaxMessageBytes = 0
	}

	return &SMTPServer{addr: addr, repo: repo, logger: logger, config: config}
}

func (s *SMTPServer) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen smtp: %w", err)
	}
	s.listener = ln
	s.logger.Info("smtp: listening", "address", ln.Addr().String())

	s.wg.Go(func() {
		s.acceptLoop()
	})

	return nil
}

func (s *SMTPServer) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *SMTPServer) Shutdown(ctx context.Context) error {
	if s.listener == nil {
		return nil
	}

	err := s.listener.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("close smtp listener: %w", err)
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (s *SMTPServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			s.logger.Error("smtp: accept failed", "error", err)
			continue
		}

		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			_ = c.SetDeadline(time.Now().Add(10 * time.Minute))
			sess := newSession(c, s.repo, s.logger, s.config)
			sess.run(context.Background())
		}(conn)
	}
}
