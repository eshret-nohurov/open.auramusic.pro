// Package preview caches what the backend says about each code, so a burst
// of chat-app bots costs one backend request and a backend outage does not
// blank out links that worked an hour ago.
package preview

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/backend"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/lru"
)

type Outcome int

const (
	Found Outcome = iota + 1
	NotFound
	Failed
)

type Result struct {
	Outcome Outcome
	Preview backend.Preview
	// Stale marks a found preview served past its TTL because the backend
	// is failing right now.
	Stale bool
}

type Fetcher interface {
	Fetch(ctx context.Context, code string) (backend.Preview, error)
}

type Options struct {
	Fetcher     Fetcher
	FoundTTL    time.Duration
	NotFoundTTL time.Duration
	FailTTL     time.Duration
	StaleMax    time.Duration
	MaxEntries  int64
	Now         func() time.Time // tests
}

type Store struct {
	o Options
	// entries holds answers worth keeping: found previews, and failures
	// that still carry one to serve stale. negative holds "not found" and
	// plain failures. Apart, a flood of random codes cannot push out the
	// previews that stale serving depends on.
	entries  *lru.Cache[string, entry]
	negative *lru.Cache[string, entry]
	group    singleflight.Group

	hits, misses, failures atomic.Int64
}

type entry struct {
	outcome Outcome
	expires time.Time
	// preview and goodAt hold the last found answer. A failed entry keeps
	// them from the entry it replaced, which is what makes stale serving
	// possible.
	preview backend.Preview
	goodAt  time.Time
}

func New(o Options) *Store {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.MaxEntries == 0 {
		o.MaxEntries = 50_000
	}
	return &Store{
		o:        o,
		entries:  lru.New[string, entry](o.MaxEntries, nil),
		negative: lru.New[string, entry](max(o.MaxEntries/5, 1), nil),
	}
}

func (s *Store) lookup(code string, peek bool) (entry, bool) {
	get, getNeg := s.entries.Get, s.negative.Get
	if peek {
		get, getNeg = s.entries.Peek, s.negative.Peek
	}
	if e, ok := get(code); ok {
		return e, true
	}
	return getNeg(code)
}

// put keeps a code in exactly one of the two caches, so a revoked code's old
// preview cannot outlive its "not found".
func (s *Store) put(code string, e entry) {
	if e.goodAt.IsZero() {
		s.entries.Delete(code)
		s.negative.Put(code, e)
		return
	}
	s.negative.Delete(code)
	s.entries.Put(code, e)
}

// Get never returns an error: a failure is an outcome the page shows. The
// caller must have validated the code.
func (s *Store) Get(code string) Result {
	now := s.o.Now()
	if e, ok := s.lookup(code, false); ok && now.Before(e.expires) {
		s.hits.Add(1)
		return e.result(now, s.o.StaleMax)
	}
	s.misses.Add(1)
	v, _, _ := s.group.Do(code, func() (any, error) {
		// A flight that finished between our lookup and Do already stored
		// a fresh entry; asking the backend again would double the load.
		now := s.o.Now()
		if e, ok := s.lookup(code, true); ok && now.Before(e.expires) {
			return e.result(now, s.o.StaleMax), nil
		}
		return s.refresh(code), nil
	})
	return v.(Result)
}

// refresh runs detached from any one request: it serves everyone waiting
// on this code, so one bot hanging up must not cancel it for the rest. The
// backend client's own timeout bounds it.
func (s *Store) refresh(code string) Result {
	p, err := s.o.Fetcher.Fetch(context.Background(), code)
	now := s.o.Now()

	var e entry
	switch {
	case err == nil:
		e = entry{outcome: Found, expires: now.Add(s.o.FoundTTL), preview: p, goodAt: now}
	case errors.Is(err, backend.ErrNotFound):
		e = entry{outcome: NotFound, expires: now.Add(s.o.NotFoundTTL)}
	default:
		s.failures.Add(1)
		e = entry{outcome: Failed, expires: now.Add(s.o.FailTTL)}
		if prev, ok := s.entries.Peek(code); ok && !prev.goodAt.IsZero() {
			e.preview, e.goodAt = prev.preview, prev.goodAt
		}
	}
	s.put(code, e)
	return e.result(now, s.o.StaleMax)
}

func (e entry) result(now time.Time, staleMax time.Duration) Result {
	if e.outcome != Failed {
		return Result{Outcome: e.outcome, Preview: e.preview}
	}
	if !e.goodAt.IsZero() && now.Sub(e.goodAt) <= staleMax {
		return Result{Outcome: Found, Preview: e.preview, Stale: true}
	}
	return Result{Outcome: Failed}
}

type Counters struct {
	Hits, Misses, Failures int64
}

// TakeCounters returns the counts since the previous call and resets them.
func (s *Store) TakeCounters() Counters {
	return Counters{Hits: s.hits.Swap(0), Misses: s.misses.Swap(0), Failures: s.failures.Swap(0)}
}
