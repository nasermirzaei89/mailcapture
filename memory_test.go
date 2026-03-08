package main

import (
	"context"
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

			_, err := repo.Create(ctx, msg)
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
