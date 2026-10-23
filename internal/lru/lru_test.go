package lru

import "testing"

func TestEvictsLeastRecent(t *testing.T) {
	c := New[string, int](2, nil)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Get("a")
	c.Put("c", 3)
	if _, ok := c.Get("b"); ok {
		t.Error("b should be evicted")
	}
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Error("a lost")
	}
	if c.Len() != 2 {
		t.Errorf("len = %d", c.Len())
	}
}

func TestCostBound(t *testing.T) {
	c := New[string, []byte](10, func(b []byte) int64 { return int64(len(b)) })
	c.Put("a", make([]byte, 4))
	c.Put("b", make([]byte, 4))
	c.Put("c", make([]byte, 4))
	if c.Used() > 10 {
		t.Errorf("used = %d", c.Used())
	}
	if _, ok := c.Peek("a"); ok {
		t.Error("a should be evicted")
	}
	c.Put("huge", make([]byte, 11))
	if _, ok := c.Peek("huge"); ok {
		t.Error("value larger than the cache was kept")
	}
	c.Put("b", make([]byte, 1))
	if c.Used() != 5 {
		t.Errorf("replace did not update cost: %d", c.Used())
	}
}

func TestDelete(t *testing.T) {
	c := New[string, int](10, func(int) int64 { return 3 })
	c.Put("a", 1)
	c.Put("b", 2)
	c.Delete("a")
	c.Delete("missing")
	if _, ok := c.Peek("a"); ok || c.Len() != 1 || c.Used() != 3 {
		t.Errorf("after Delete: len %d used %d", c.Len(), c.Used())
	}
}
