package api

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/config"
)

func TestFetchCachedReusesValueWithinTTL(t *testing.T) {
	client := &Client{
		Config:       &config.Config{CacheTTL: 1},
		requestCache: map[string]cacheEntry{},
	}

	var calls atomic.Int32
	fetch := func() (string, error) {
		call := calls.Add(1)
		return "value-" + string(rune('0'+call)), nil
	}

	first, err := FetchCached(client, "devices", fetch)
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}

	second, err := FetchCached(client, "devices", fetch)
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}

	if first != second {
		t.Fatalf("expected cached value to be reused, got %q and %q", first, second)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected fetch to be called once, got %d", calls.Load())
	}
}

func TestFetchCachedExpiresValue(t *testing.T) {
	client := &Client{
		Config:       &config.Config{CacheTTL: 1},
		requestCache: map[string]cacheEntry{},
	}

	var calls atomic.Int32
	fetch := func() (int, error) {
		return int(calls.Add(1)), nil
	}

	first, err := FetchCached(client, "clients", fetch)
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	second, err := FetchCached(client, "clients", fetch)
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}

	if first == second {
		t.Fatalf("expected cached value to expire, got %d and %d", first, second)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected fetch to be called twice, got %d", calls.Load())
	}
}
