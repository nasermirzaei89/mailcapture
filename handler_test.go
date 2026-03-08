package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleIndexRendersMessageList(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryMessageRepository()
	_, _ = repo.Create(t.Context(), Message{
		ID:         "msg-1",
		ReceivedAt: time.Now().UTC(),
		Subject:    "Hello",
	})

	s, err := NewWebServer(":0", repo, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("new server failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	s.handleIndex(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: got %d want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()

	if !strings.Contains(body, "Total: 1 email") {
		t.Fatalf("expected total count in response body")
	}

	if !strings.Contains(body, "Hello") {
		t.Fatalf("expected subject in response body")
	}
}

func TestAPIListAndGetMessage(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryMessageRepository()
	msg := Message{ID: "msg-1", ReceivedAt: time.Now().UTC(), Subject: "Hello API"}
	_, _ = repo.Create(t.Context(), msg)

	s, err := NewWebServer(":0", repo, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("new server failed: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/messages", nil)
	listRR := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("list status mismatch: got %d want %d", listRR.Code, http.StatusOK)
	}

	var listResp apiListMessagesResponse
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response failed: %v", err)
	}
	if listResp.Count != 1 {
		t.Fatalf("list count mismatch: got %d want 1", listResp.Count)
	}
	if len(listResp.Messages) != 1 || listResp.Messages[0].ID != "msg-1" {
		t.Fatalf("list payload mismatch: %+v", listResp.Messages)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/messages/msg-1", nil)
	getRR := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("get status mismatch: got %d want %d", getRR.Code, http.StatusOK)
	}

	var got Message
	if err := json.Unmarshal(getRR.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get response failed: %v", err)
	}
	if got.ID != "msg-1" {
		t.Fatalf("id mismatch: got %q want %q", got.ID, "msg-1")
	}
}

func TestAPIDeleteByIDAndClear(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryMessageRepository()
	_, _ = repo.Create(t.Context(), Message{ID: "msg-1", ReceivedAt: time.Now().UTC()})
	_, _ = repo.Create(t.Context(), Message{ID: "msg-2", ReceivedAt: time.Now().UTC()})

	s, err := NewWebServer(":0", repo, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("new server failed: %v", err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/messages/msg-1", nil)
	deleteRR := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(deleteRR, deleteReq)

	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("delete status mismatch: got %d want %d", deleteRR.Code, http.StatusNoContent)
	}

	count, err := repo.Count(t.Context())
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("count mismatch after delete: got %d want 1", count)
	}

	clearReq := httptest.NewRequest(http.MethodDelete, "/api/messages", nil)
	clearRR := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(clearRR, clearReq)

	if clearRR.Code != http.StatusNoContent {
		t.Fatalf("clear status mismatch: got %d want %d", clearRR.Code, http.StatusNoContent)
	}

	count, err = repo.Count(t.Context())
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("count mismatch after clear: got %d want 0", count)
	}
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryMessageRepository()
	s, err := NewWebServer(":0", repo, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("new server failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: got %d want %d", rr.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != "ok" {
		t.Fatalf("body mismatch: got %q want %q", body, "ok")
	}
}
