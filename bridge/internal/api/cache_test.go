package api

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
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

	first, err := client.FetchCached("example", func() (string, error) {
		atomic.AddInt32(&calls, 1)
		return "fresh", nil
	})
	if err != nil {
		t.Fatalf("first FetchCached() returned error: %v", err)
	}

	second, err := client.FetchCached("example", func() (string, error) {
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
	synctest.Test(t, func(t *testing.T) {
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
			wg.Go(func() {
				value, err := client.FetchCached("shared-key", fetch)
				if err != nil {
					errorsCh <- err
					return
				}
				results <- value
			})
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
	})
}

func TestFetchCachedDoesNotStoreFailedFetch(t *testing.T) {
	client := testCacheClient(60)
	var calls int32
	fetchErr := errors.New("controller unavailable")

	_, err := client.FetchCached("failure", func() (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", fetchErr
	})
	if !errors.Is(err, fetchErr) {
		t.Fatalf("FetchCached() error = %v, want %v", err, fetchErr)
	}

	value, err := client.FetchCached("failure", func() (string, error) {
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

func TestFetchCachedSeparatesFlightsAcrossInvalidation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		type result struct {
			value string
			err   error
		}

		client := testCacheClient(60)
		oldStarted := make(chan struct{})
		releaseOld := make(chan struct{})
		oldDone := make(chan result, 1)
		newDone := make(chan result, 1)
		var calls atomic.Int32

		go func() {
			value, err := client.FetchCached("contextual", func() (string, error) {
				calls.Add(1)
				close(oldStarted)
				<-releaseOld
				return "old-context", nil
			})
			oldDone <- result{value: value, err: err}
		}()
		<-oldStarted

		client.invalidateRequestCache()
		go func() {
			value, err := client.FetchCached("contextual", func() (string, error) {
				calls.Add(1)
				return "new-context", nil
			})
			newDone <- result{value: value, err: err}
		}()
		synctest.Wait()

		var newResult result
		newCompletedBeforeOld := false
		select {
		case newResult = <-newDone:
			newCompletedBeforeOld = true
		default:
		}
		close(releaseOld)
		oldResult := <-oldDone
		if !newCompletedBeforeOld {
			newResult = <-newDone
		}

		if !newCompletedBeforeOld {
			t.Error("post-invalidation fetch joined the stale in-flight request")
		}
		for name, got := range map[string]result{"old caller retry": oldResult, "new caller": newResult} {
			if got.err != nil {
				t.Errorf("%s error = %v", name, got.err)
			}
			if got.value != "new-context" {
				t.Errorf("%s value = %q, want new-context", name, got.value)
			}
		}
		if got := calls.Load(); got != 2 {
			t.Errorf("fetch calls = %d, want 2", got)
		}

		cached, err := client.FetchCached("contextual", func() (string, error) {
			calls.Add(1)
			return "unexpected", nil
		})
		if err != nil || cached != "new-context" {
			t.Fatalf("cached result = %q, %v; want new-context", cached, err)
		}
		if got := calls.Load(); got != 2 {
			t.Errorf("cached lookup increased fetch calls to %d", got)
		}
	})
}

func TestFetchCachedRejectsAKeyReusedWithAnotherType(t *testing.T) {
	client := testCacheClient(60)
	if _, err := client.FetchCached("typed-key", func() (string, error) { return "value", nil }); err != nil {
		t.Fatalf("FetchCached(string) error = %v", err)
	}

	_, err := client.FetchCached("typed-key", func() (int, error) { return 42, nil })
	if err == nil || !strings.Contains(err.Error(), "cached value type mismatch") {
		t.Fatalf("FetchCached(int) error = %v, want type mismatch", err)
	}
}
