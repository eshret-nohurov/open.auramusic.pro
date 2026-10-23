package preview

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/backend"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/mockbackend"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time          { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) advance(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

// scripted answers from a queue and counts calls.
type scripted struct {
	calls   atomic.Int64
	mu      sync.Mutex
	answers []error // nil means found
}

func (s *scripted) Fetch(ctx context.Context, code string) (backend.Preview, error) {
	s.calls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.answers[0]
	if len(s.answers) > 1 {
		s.answers = s.answers[1:]
	}
	if err != nil {
		return backend.Preview{}, err
	}
	return backend.Preview{Type: "track", Title: "x"}, nil
}

var errDown = &backend.Failure{Kind: "status", Status: 502}

func newStore(f Fetcher, c *clock) *Store {
	return New(Options{
		Fetcher: f, Now: c.now,
		FoundTTL: time.Hour, NotFoundTTL: 10 * time.Minute, FailTTL: 15 * time.Second, StaleMax: 24 * time.Hour,
	})
}

const code = "Xk3pQ9aB7mLzT2wR"

func TestFoundTTL(t *testing.T) {
	c := &clock{t: time.Unix(1e9, 0)}
	f := &scripted{answers: []error{nil}}
	s := newStore(f, c)
	s.Get(code)
	c.advance(59 * time.Minute)
	if r := s.Get(code); r.Outcome != Found || f.calls.Load() != 1 {
		t.Fatalf("within TTL: %+v, calls %d", r, f.calls.Load())
	}
	c.advance(2 * time.Minute)
	s.Get(code)
	if f.calls.Load() != 2 {
		t.Errorf("after TTL calls = %d", f.calls.Load())
	}
}

func TestNotFoundTTL(t *testing.T) {
	c := &clock{t: time.Unix(1e9, 0)}
	f := &scripted{answers: []error{backend.ErrNotFound}}
	s := newStore(f, c)
	if r := s.Get(code); r.Outcome != NotFound {
		t.Fatalf("%+v", r)
	}
	c.advance(9 * time.Minute)
	s.Get(code)
	if f.calls.Load() != 1 {
		t.Errorf("404 not cached")
	}
	c.advance(2 * time.Minute)
	s.Get(code)
	if f.calls.Load() != 2 {
		t.Errorf("404 cached too long")
	}
}

func TestFailurePause(t *testing.T) {
	c := &clock{t: time.Unix(1e9, 0)}
	f := &scripted{answers: []error{errDown}}
	s := newStore(f, c)
	if r := s.Get(code); r.Outcome != Failed {
		t.Fatalf("%+v", r)
	}
	c.advance(10 * time.Second)
	s.Get(code)
	if f.calls.Load() != 1 {
		t.Errorf("backend asked again during FAIL_TTL")
	}
	c.advance(6 * time.Second)
	s.Get(code)
	if f.calls.Load() != 2 {
		t.Errorf("no retry after FAIL_TTL")
	}
	if got := s.TakeCounters(); got.Failures != 2 || got.Hits != 1 || got.Misses != 2 {
		t.Errorf("counters %+v", got)
	}
	if got := s.TakeCounters(); got != (Counters{}) {
		t.Errorf("counters not reset: %+v", got)
	}
}

func TestStaleWhileBackendDown(t *testing.T) {
	c := &clock{t: time.Unix(1e9, 0)}
	f := &scripted{answers: []error{nil, errDown}}
	s := newStore(f, c)
	s.Get(code)

	c.advance(2 * time.Hour)
	r := s.Get(code)
	if r.Outcome != Found || !r.Stale || r.Preview.Title != "x" {
		t.Fatalf("stale not served: %+v", r)
	}
	// Still within FAIL_TTL: served from the failed entry, still stale.
	c.advance(5 * time.Second)
	if r := s.Get(code); r.Outcome != Found || !r.Stale {
		t.Errorf("stale lost inside FAIL_TTL: %+v", r)
	}

	c.advance(23 * time.Hour)
	if r := s.Get(code); r.Outcome != Failed {
		t.Errorf("stale served past STALE_MAX: %+v", r)
	}
}

func TestNotFoundIsNeverStale(t *testing.T) {
	c := &clock{t: time.Unix(1e9, 0)}
	f := &scripted{answers: []error{backend.ErrNotFound, errDown}}
	s := newStore(f, c)
	s.Get(code)
	c.advance(11 * time.Minute)
	if r := s.Get(code); r.Outcome != Failed || r.Stale {
		t.Errorf("%+v", r)
	}
}

func TestOneBackendRequestPerCode(t *testing.T) {
	m := mockbackend.New(os.DirFS("../../testdata/mock"), "t", nil)
	ts := httptest.NewServer(m)
	defer ts.Close()
	client := backend.New(backend.Options{BaseURL: ts.URL, Token: "t", Timeout: 2 * time.Second, MaxInflight: 4})
	s := New(Options{Fetcher: client, FoundTTL: time.Hour, NotFoundTTL: time.Minute, FailTTL: time.Second, StaleMax: time.Hour})

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]Result, 20)
	for i := range results {
		wg.Go(func() {
			<-start
			results[i] = s.Get("TrackCyrillic001")
		})
	}
	close(start)
	wg.Wait()
	for i, r := range results {
		if r.Outcome != Found || r.Preview.Title != "Эгоист" {
			t.Fatalf("goroutine %d: %+v", i, r)
		}
	}
	if n := m.DeeplinkCalls.Load(); n != 1 {
		t.Errorf("backend got %d requests, want 1", n)
	}
}

func TestErrNotFoundWrapped(t *testing.T) {
	c := &clock{t: time.Unix(1e9, 0)}
	f := &scripted{answers: []error{errors.Join(backend.ErrNotFound)}}
	if r := newStore(f, c).Get(code); r.Outcome != NotFound {
		t.Errorf("%+v", r)
	}
}
