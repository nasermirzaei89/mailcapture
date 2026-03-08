package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestInMemoryMessageRepositoryConcurrentCreateAndList(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryMessageRepository()
	ctx := context.Background()

	const n = 100
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg := Message{
				ID:         fmt.Sprintf("id-%d", i),
				ReceivedAt: time.Now().UTC(),
				Subject:    fmt.Sprintf("subject-%d", i),
			}

			err := repo.Create(ctx, msg)
			if err != nil {
				t.Errorf("create failed: %v", err)
			}
		}(i)
	}
	wg.Wait()

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != n {
		t.Fatalf("count mismatch: got %d want %d", count, n)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != n {
		t.Fatalf("list length mismatch: got %d want %d", len(list), n)
	}
}

func TestInMemoryMessageRepositoryRespectsMaxItems(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryMessageRepositoryWithLimit(2)
	ctx := context.Background()

	_ = repo.Create(ctx, Message{ID: "id-1", ReceivedAt: time.Now().UTC(), Subject: "one"})
	_ = repo.Create(ctx, Message{ID: "id-2", ReceivedAt: time.Now().UTC(), Subject: "two"})
	_ = repo.Create(ctx, Message{ID: "id-3", ReceivedAt: time.Now().UTC(), Subject: "three"})

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("count mismatch: got %d want 2", count)
	}

	_, err = repo.GetByID(ctx, "id-1")
	if err == nil {
		t.Fatalf("expected oldest message to be evicted")
	}
}

func TestInMemoryMessageRepositoryDeleteOperations(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryMessageRepository()
	ctx := context.Background()

	_ = repo.Create(ctx, Message{ID: "id-1", ReceivedAt: time.Now().UTC()})
	_ = repo.Create(ctx, Message{ID: "id-2", ReceivedAt: time.Now().UTC()})

	err := repo.DeleteByID(ctx, "id-1")
	if err != nil {
		t.Fatalf("delete by id failed: %v", err)
	}

	err = repo.DeleteByID(ctx, "does-not-exist")
	if !errors.As(err, &MessageNotFoundError{}) {
		t.Fatalf("expected MessageNotFoundError, got %v", err)
	}

	err = repo.DeleteAll(ctx)
	if err != nil {
		t.Fatalf("delete all failed: %v", err)
	}

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("count mismatch: got %d want 0", count)
	}
}
