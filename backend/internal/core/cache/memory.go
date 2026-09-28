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

// sweepInterval is how often a write also clears out expired entries.
const sweepInterval = time.Minute

// MemoryCache is the fallback used when Redis is disabled or unreachable.
//
// GetValue refuses expired entries, but that alone does not free them, and the
// key set is not bounded: place search writes one key per distinct query a
// visitor types. So writes sweep expired entries too, at most once per
// sweepInterval, which bounds the map by what is still live — Redis does the
// same job with its own expiry.
type MemoryCache struct {
	mu        sync.RWMutex
	values    map[string]memoryValue
	lastSweep time.Time
}

func NewMemoryCache() *MemoryCache {
	return &MemoryCache{values: make(map[string]memoryValue)}
}

func (m *MemoryCache) SetValue(ctx context.Context, key string, payload []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if now.Sub(m.lastSweep) >= sweepInterval {
		for k, v := range m.values {
			if now.After(v.expiresAt) {
				delete(m.values, k)
			}
		}
		m.lastSweep = now
	}
	m.values[key] = memoryValue{payload: payload, expiresAt: now.Add(ttl)}
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
