package prometheus

import (
	"context"
	"sync"
	"time"
)

type CacheEntry struct {
	value    float64
	fetchTime time.Time
}

type CachedClient struct {
	client PrometheusClient
	mu     sync.RWMutex
	cache  map[string]CacheEntry
	ttl    time.Duration
}

var _ PrometheusClient = (*CachedClient)(nil)

func NewCachedClient(client PrometheusClient, ttl time.Duration) *CachedClient {
	return &CachedClient{
		client: client,
		cache:  make(map[string]CacheEntry),
		ttl:    ttl,
	}
}

func (c *CachedClient) getCache(query string) (float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.cache[query]
	if !exists || time.Since(entry.fetchTime) > c.ttl {
		return 0, false
	}
	return entry.value, true
}

func (c *CachedClient) setCache(query string, value float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[query] = CacheEntry{
		value:    value,
		fetchTime: time.Now(),
	}
}

func (c *CachedClient) QuerySLI(ctx context.Context, query string) (float64, error) {
	if value, found := c.getCache(query); found {
		return value, nil
	}

	value, err := c.client.QuerySLI(ctx, query)
	if err != nil {
		return 0, err
	}
	c.setCache(query, value)
	return value, nil
}

func (c *CachedClient) QuerySLINotNormalized(ctx context.Context, query string) (float64, error) {
	if value, found := c.getCache(query); found {
		return value, nil
	}

	value, err := c.client.QuerySLINotNormalized(ctx, query)
	if err != nil {
		return 0, err
	}
	c.setCache(query, value)
	return value, nil
}

func (c *CachedClient) CheckConnection(ctx context.Context) error {
	return c.client.CheckConnection(ctx)
}

func (c *CachedClient) GetURL() string {
	return c.client.GetURL()
}