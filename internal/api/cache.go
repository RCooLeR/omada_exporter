package api

import (
	"fmt"
	"time"
)

type cacheEntry struct {
	value     any
	expiresAt time.Time
}

func (c *Client) cacheTTL() time.Duration {
	if c == nil || c.Config == nil || c.Config.CacheTTL <= 0 {
		return 0
	}

	return time.Duration(c.Config.CacheTTL) * time.Second
}

func (c *Client) invalidateRequestCache() {
	c.cacheMu.Lock()
	c.requestCache = map[string]cacheEntry{}
	c.cacheMu.Unlock()
}

func FetchCached[T any](client *Client, key string, fetch func() (T, error)) (T, error) {
	var zero T
	ttl := client.cacheTTL()
	if ttl <= 0 {
		return fetch()
	}

	now := time.Now()
	client.cacheMu.RLock()
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

	value, err, _ := client.requestGroup.Do(key, func() (any, error) {
		now := time.Now()
		client.cacheMu.RLock()
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
		client.requestCache[key] = cacheEntry{
			value:     result,
			expiresAt: time.Now().Add(ttl),
		}
		client.cacheMu.Unlock()
		return result, nil
	})
	if err != nil {
		return zero, err
	}

	typed, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("cached value type mismatch for %s", key)
	}

	return typed, nil
}
