package tutka

import (
	"container/list"
	"sync"
)

// lru is a small mutex-guarded LRU cache. Both things it holds — decoded frames
// and rendered PNGs — are re-derivable (from disk and from a decoded frame
// respectively), so eviction only ever costs work, never data.
type lru[K comparable, V any] struct {
	mu    sync.Mutex
	max   int
	ll    *list.List // front is most recently used
	items map[K]*list.Element
}

type lruEntry[K comparable, V any] struct {
	key   K
	value V
}

func newLRU[K comparable, V any](max int) *lru[K, V] {
	if max < 1 {
		max = 1
	}
	return &lru[K, V]{
		max:   max,
		ll:    list.New(),
		items: make(map[K]*list.Element, max),
	}
}

func (c *lru[K, V]) get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		return el.Value.(*lruEntry[K, V]).value, true
	}
	var zero V
	return zero, false
}

func (c *lru[K, V]) put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*lruEntry[K, V]).value = value
		c.ll.MoveToFront(el)
		return
	}
	c.items[key] = c.ll.PushFront(&lruEntry[K, V]{key: key, value: value})
	for c.ll.Len() > c.max {
		oldest := c.ll.Back()
		if oldest == nil {
			return
		}
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*lruEntry[K, V]).key)
	}
}

func (c *lru[K, V]) remove(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.Remove(el)
		delete(c.items, key)
	}
}
