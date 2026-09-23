package main

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// httpGetWithRetry 发起 GET，最多尝试 maxAttempts 次，对 5xx / 429 / 网络错误做指数退避。
// 每次尝试使用独立的 attemptTimeout，避免冷启动的大响应被整体超时连带掐断。
func httpGetWithRetry(parent context.Context, client *http.Client, url string, headers map[string]string, maxAttempts int, attemptTimeout time.Duration) ([]byte, int, error) {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(500*(1<<attempt-1)) * time.Millisecond // 500ms,1s,2s...
			select {
			case <-parent.Done():
				return nil, 0, parent.Err()
			case <-time.After(backoff):
			}
		}
		ctx, cancel := context.WithTimeout(parent, attemptTimeout)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			cancel()
			return nil, 0, err
		}
		req.Header.Set("User-Agent", browserUA)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = &httpStatusError{code: resp.StatusCode}
			continue
		}
		return body, resp.StatusCode, nil
	}
	if lastErr == nil {
		lastErr = io.ErrUnexpectedEOF
	}
	return nil, 0, lastErr
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return "upstream HTTP error status " + http.StatusText(e.code) }

// ---- 极简 TTL 缓存 ----

type cacheItem struct {
	value  any
	expire time.Time
}

type ttlCache struct {
	mu sync.RWMutex
	m  map[string]cacheItem
}

func newTTLCache() *ttlCache {
	return &ttlCache{m: make(map[string]cacheItem)}
}

func (c *ttlCache) get(key string) (any, bool) {
	c.mu.RLock()
	item, ok := c.m[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(item.expire) {
		c.mu.Lock()
		delete(c.m, key)
		c.mu.Unlock()
		return nil, false
	}
	return item.value, true
}

func (c *ttlCache) set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	c.m[key] = cacheItem{value: value, expire: time.Now().Add(ttl)}
	c.mu.Unlock()
}
