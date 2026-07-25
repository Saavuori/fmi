package cache

import (
	"context"
	"sync"
	"time"
)

type memoryValue struct {
	payload   []byte
	expiresAt time.Time
}

// MemoryCache is the fallback used when Redis is disabled or unreachable.
// Expired entries are not swept in the background — GetValue simply refuses to
// return them — which is fine because the key set is small and bounded.
type MemoryCache struct {
	mu     sync.RWMutex
	values map[string]memoryValue
}

func NewMemoryCache() *MemoryCache {
	return &MemoryCache{values: make(map[string]memoryValue)}
}

func (m *MemoryCache) SetValue(ctx context.Context, key string, payload []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = memoryValue{payload: payload, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (m *MemoryCache) GetValue(ctx context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.values[key]
	if !ok || time.Now().After(v.expiresAt) {
		return nil, nil
	}
	return v.payload, nil
}

func (m *MemoryCache) Ping(ctx context.Context) error {
	return nil
}

func (m *MemoryCache) Close() error {
	return nil
}
