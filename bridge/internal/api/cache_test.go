package api

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func testCacheClient(ttl int) *Client {
	return &Client{
		Config:       &config.Config{CacheTTL: ttl},
		requestCache: map[string]cacheEntry{},
	}
}

func TestFetchCachedReusesFreshValue(t *testing.T) {
	client := testCacheClient(60)
	var calls int32

	first, err := FetchCached(client, "example", func() (string, error) {
		atomic.AddInt32(&calls, 1)
		return "fresh", nil
	})
	if err != nil {
		t.Fatalf("first FetchCached() returned error: %v", err)
	}

	second, err := FetchCached(client, "example", func() (string, error) {
		atomic.AddInt32(&calls, 1)
		return "new", nil
	})
	if err != nil {
		t.Fatalf("second FetchCached() returned error: %v", err)
	}

	if first != "fresh" || second != "fresh" {
		t.Fatalf("cached values = %q, %q; want both fresh", first, second)
	}
	if calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", calls)
	}
}

func TestFetchCachedDeduplicatesConcurrentMisses(t *testing.T) {
	client := testCacheClient(60)
	var calls int32
	var wg sync.WaitGroup
	results := make(chan string, 8)
	errorsCh := make(chan error, 8)

	fetch := func() (string, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond)
		return "shared", nil
	}

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := FetchCached(client, "shared-key", fetch)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- value
		}()
	}
	wg.Wait()
	close(results)
	close(errorsCh)

	for err := range errorsCh {
		t.Fatalf("FetchCached() returned error: %v", err)
	}
	for value := range results {
		if value != "shared" {
			t.Fatalf("FetchCached() = %q, want shared", value)
		}
	}
	if calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", calls)
	}
}

func TestFetchCachedDoesNotStoreFailedFetch(t *testing.T) {
	client := testCacheClient(60)
	var calls int32
	fetchErr := errors.New("controller unavailable")

	_, err := FetchCached(client, "failure", func() (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", fetchErr
	})
	if !errors.Is(err, fetchErr) {
		t.Fatalf("FetchCached() error = %v, want %v", err, fetchErr)
	}

	value, err := FetchCached(client, "failure", func() (string, error) {
		atomic.AddInt32(&calls, 1)
		return "recovered", nil
	})
	if err != nil {
		t.Fatalf("FetchCached() after failure returned error: %v", err)
	}
	if value != "recovered" {
		t.Fatalf("FetchCached() after failure = %q, want recovered", value)
	}
	if calls != 2 {
		t.Fatalf("fetch calls = %d, want 2", calls)
	}
}
