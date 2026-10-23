// Package lru is a size-bounded least-recently-used map, safe for
// concurrent use. Size is whatever the cost function says: one per entry
// for previews, bytes for rendered images.
package lru

import (
	"container/list"
	"sync"
)

type Cache[K comparable, V any] struct {
	mu      sync.Mutex
	maxCost int64
	cost    func(V) int64
	used    int64
	order   *list.List // front is most recent
	items   map[K]*list.Element
}

type item[K comparable, V any] struct {
	key  K
	val  V
	cost int64
}

// New returns a cache that evicts once the summed cost exceeds maxCost.
// A nil cost counts every entry as 1.
func New[K comparable, V any](maxCost int64, cost func(V) int64) *Cache[K, V] {
	if cost == nil {
		cost = func(V) int64 { return 1 }
	}
	return &Cache[K, V]{maxCost: maxCost, cost: cost, order: list.New(), items: map[K]*list.Element{}}
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*item[K, V]).val, true
	}
	var zero V
	return zero, false
}

// Peek reads without touching recency.
func (c *Cache[K, V]) Peek(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		return el.Value.(*item[K, V]).val, true
	}
	var zero V
	return zero, false
}

// Put stores val. A value costing more than the whole cache is not kept.
func (c *Cache[K, V]) Put(key K, val V) {
	cost := c.cost(val)
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.used -= el.Value.(*item[K, V]).cost
		c.order.Remove(el)
		delete(c.items, key)
	}
	if cost > c.maxCost {
		return
	}
	c.items[key] = c.order.PushFront(&item[K, V]{key: key, val: val, cost: cost})
	c.used += cost
	for c.used > c.maxCost {
		el := c.order.Back()
		it := el.Value.(*item[K, V])
		c.order.Remove(el)
		delete(c.items, it.key)
		c.used -= it.cost
	}
}

// Len and Used are for tests and logs.
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *Cache[K, V]) Used() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.used
}
