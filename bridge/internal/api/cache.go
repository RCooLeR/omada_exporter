package api

import (
	"errors"
	"fmt"
	"time"
)

var errCacheGenerationChanged = errors.New("request cache generation changed")

// cacheEntry stores a cached value and its expiration time.
type cacheEntry struct {
	value     any
	expiresAt time.Time
}

// cacheTTL returns the request cache lifetime configured for the client.
func (c *Client) cacheTTL() time.Duration {
	if c == nil || c.Config == nil || c.Config.CacheTTL <= 0 {
		return 0
	}

	return time.Duration(c.Config.CacheTTL) * time.Second
}

// invalidateRequestCache clears cached request results.
func (c *Client) invalidateRequestCache() {
	c.cacheMu.Lock()
	c.cacheGeneration++
	c.requestCache = map[string]cacheEntry{}
	c.cacheMu.Unlock()
}

// FetchCached returns a cached value or fetches, stores, and returns a fresh one.
func (client *Client) FetchCached[T any](key string, fetch func() (T, error)) (T, error) {
	var zero T
	ttl := client.cacheTTL()
	if ttl <= 0 {
		return fetch()
	}

	for {
		now := time.Now()
		client.cacheMu.RLock()
		generation := client.cacheGeneration
		entry, ok := client.requestCache[key]
		if ok && now.Before(entry.expiresAt) {
			client.cacheMu.RUnlock()
			value, typeOK := entry.value.(T)
			if !typeOK {
				return zero, fmt.Errorf("cached value type mismatch for %s", key)
			}
			return value, nil
		}
		client.cacheMu.RUnlock()

		// Scope each flight to the cache generation. A request that begins after
		// reauthentication must not join a pre-invalidation request, and an old
		// request must not repopulate the new generation when it eventually ends.
		flightKey := fmt.Sprintf("%d:%s", generation, key)
		value, err, _ := client.requestGroup.Do(flightKey, func() (any, error) {
			now := time.Now()
			client.cacheMu.RLock()
			if client.cacheGeneration != generation {
				client.cacheMu.RUnlock()
				return nil, errCacheGenerationChanged
			}
			entry, ok := client.requestCache[key]
			if ok && now.Before(entry.expiresAt) {
				client.cacheMu.RUnlock()
				return entry.value, nil
			}
			client.cacheMu.RUnlock()

			result, err := fetch()
			if err != nil {
				return nil, err
			}

			client.cacheMu.Lock()
			if client.cacheGeneration != generation {
				client.cacheMu.Unlock()
				return nil, errCacheGenerationChanged
			}
			client.requestCache[key] = cacheEntry{
				value:     result,
				expiresAt: time.Now().Add(ttl),
			}
			client.cacheMu.Unlock()
			return result, nil
		})
		if errors.Is(err, errCacheGenerationChanged) {
			continue
		}
		if err != nil {
			return zero, err
		}

		typed, ok := value.(T)
		if !ok {
			return zero, fmt.Errorf("cached value type mismatch for %s", key)
		}

		return typed, nil
	}
}
