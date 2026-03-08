package main

import (
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
