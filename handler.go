package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
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

type apiListMessagesResponse struct {
	Count    int       `json:"count"`
	Messages []Message `json:"messages"`
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
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /messages/{id}", s.handleDetail)
	mux.HandleFunc("GET /messages/{id}/raw", s.handleRaw)
	mux.HandleFunc("GET /api/messages", s.handleAPIListMessages)
	mux.HandleFunc("DELETE /api/messages", s.handleAPIClearMessages)
	mux.HandleFunc("GET /api/messages/{id}", s.handleAPIGetMessage)
	mux.HandleFunc("DELETE /api/messages/{id}", s.handleAPIDeleteMessage)

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
	messages, err := s.repo.List(r.Context())
	if err != nil {
		s.logger.Error("failed to load messages", "error", err)
		http.Error(w, "failed to load messages", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	err = s.templates.ExecuteTemplate(w, "list.html", listViewData{Messages: messages, Count: len(messages)})
	if err != nil {
		s.logger.Error("failed to render page", "error", err)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}

func (s *WebServer) handleDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	message, err := s.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.As(err, &MessageNotFoundError{}) {
			http.NotFound(w, r)
			return
		}

		s.logger.Error("failed to load message", "error", err)
		http.Error(w, "failed to load message", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	err = s.templates.ExecuteTemplate(w, "detail.html", detailViewData{Message: message})
	if err != nil {
		s.logger.Error("failed to render page", "error", err)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}

func (s *WebServer) handleRaw(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	message, err := s.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.As(err, &MessageNotFoundError{}) {
			http.NotFound(w, r)
			return
		}

		s.logger.Error("failed to load message", "error", err)
		http.Error(w, "failed to load message", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	err = s.templates.ExecuteTemplate(w, "raw.html", detailViewData{Message: message})
	if err != nil {
		s.logger.Error("failed to render page", "error", err)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
	}
}

func (s *WebServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *WebServer) handleAPIListMessages(w http.ResponseWriter, r *http.Request) {
	messages, err := s.repo.List(r.Context())
	if err != nil {
		s.logger.Error("failed to list messages", "error", err)
		http.Error(w, "failed to list messages", http.StatusInternalServerError)
		return
	}

	response := apiListMessagesResponse{Count: len(messages), Messages: messages}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *WebServer) handleAPIClearMessages(w http.ResponseWriter, r *http.Request) {
	err := s.repo.DeleteAll(r.Context())
	if err != nil {
		s.logger.Error("failed to clear messages", "error", err)
		http.Error(w, "failed to clear messages", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *WebServer) handleAPIGetMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	message, err := s.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.As(err, &MessageNotFoundError{}) {
			http.NotFound(w, r)
			return
		}

		s.logger.Error("failed to load message", "id", id, "error", err)
		http.Error(w, "failed to load message", http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, http.StatusOK, message)
}

func (s *WebServer) handleAPIDeleteMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := s.repo.DeleteByID(r.Context(), id)
	if err != nil {
		if errors.As(err, &MessageNotFoundError{}) {
			http.NotFound(w, r)
			return
		}

		s.logger.Error("failed to delete message", "id", id, "error", err)
		http.Error(w, "failed to delete message", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *WebServer) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.logger.Error("failed to write json response", "error", err)
	}
}
