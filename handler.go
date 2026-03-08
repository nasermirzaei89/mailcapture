package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

//go:embed templates/*.html static/*
var assets embed.FS

// WebServer exposes a no-JS HTML UI for stored messages.
type WebServer struct {
	httpServer *http.Server
	repo       MessageRepository
	logger     *slog.Logger
	templates  *template.Template
}

type listViewData struct {
	Messages []Message
	Count    int
}

type detailViewData struct {
	Message Message
}

func NewWebServer(addr string, repo MessageRepository, logger *slog.Logger) (*WebServer, error) {
	tpls, err := template.ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, fmt.Errorf("load static assets: %w", err)
	}

	s := &WebServer{repo: repo, logger: logger, templates: tpls}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/messages/", s.handleMessageRoute)

	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s, nil
}

func (s *WebServer) Start() error {
	s.logger.Info("http: listening", "address", s.httpServer.Addr)
	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen http: %w", err)
	}
	return nil
}

func (s *WebServer) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *WebServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	messages, err := s.repo.List(r.Context())
	if err != nil {
		http.Error(w, "failed to load messages", http.StatusInternalServerError)
		s.logger.Error("failed to load messages", "error", err)
		return
	}
	count, err := s.repo.Count(r.Context())
	if err != nil {
		http.Error(w, "failed to count messages", http.StatusInternalServerError)
		s.logger.Error("failed to count messages", "error", err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if execErr := s.templates.ExecuteTemplate(w, "list.html", listViewData{Messages: messages, Count: count}); execErr != nil {
		s.logger.Error("failed to render page", "error", execErr)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}

func (s *WebServer) handleMessageRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/messages/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	if before, ok := strings.CutSuffix(path, "/raw"); ok {
		id := before
		id = strings.TrimSuffix(id, "/")
		s.handleRaw(w, r, id)
		return
	}
	s.handleDetail(w, r, strings.TrimSuffix(path, "/"))
}

func (s *WebServer) handleDetail(w http.ResponseWriter, r *http.Request, id string) {
	message, found, err := s.repo.GetByID(r.Context(), id)
	if err != nil {
		s.logger.Error("failed to load message", "error", err)
		http.Error(w, "failed to load message", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if execErr := s.templates.ExecuteTemplate(w, "detail.html", detailViewData{Message: message}); execErr != nil {
		s.logger.Error("failed to render page", "error", execErr)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}

func (s *WebServer) handleRaw(w http.ResponseWriter, r *http.Request, id string) {
	message, found, err := s.repo.GetByID(r.Context(), id)
	if err != nil {
		s.logger.Error("failed to load message", "error", err)
		http.Error(w, "failed to load message", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if execErr := s.templates.ExecuteTemplate(w, "raw.html", detailViewData{Message: message}); execErr != nil {
		s.logger.Error("failed to render page", "error", execErr)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}
