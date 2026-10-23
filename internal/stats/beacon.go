package stats

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const (
	beaconMaxBody = 1024
	beaconLimit   = 20
	beaconWindow  = time.Minute
	// A hard cap so a flood of spoofed addresses cannot grow the map without bound.
	limiterMaxEntries = 100_000
)

// limiter is a fixed-window counter per client network. Addresses live only
// in memory, as netip values rather than text, and are swept once their
// window is over.
type limiter struct {
	now       func() time.Time
	max       int // entries cap; limiterMaxEntries unless a test lowers it
	mu        sync.Mutex
	hits      map[netip.Prefix]*window
	lastSweep time.Time
}

type window struct {
	start time.Time
	n     int
}

func (l *limiter) allow(key netip.Prefix) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[netip.Prefix]*window{}
	}
	// The sweep walks the whole map, so it runs at most once per window. When
	// the map is full it must not run on every request: a flood of fresh
	// addresses would then cost a full scan each under the lock.
	if now.Sub(l.lastSweep) >= beaconWindow {
		for k, w := range l.hits {
			if now.Sub(w.start) >= beaconWindow {
				delete(l.hits, k)
			}
		}
		l.lastSweep = now
	}
	w := l.hits[key]
	if w == nil || now.Sub(w.start) >= beaconWindow {
		capacity := l.max
		if capacity <= 0 {
			capacity = limiterMaxEntries
		}
		// Full: refuse newcomers without scanning until the next sweep frees
		// room. Clients already in the map keep their budget.
		if w == nil && len(l.hits) >= capacity {
			return false
		}
		l.hits[key] = &window{start: now, n: 1}
		return true
	}
	w.n++
	return w.n <= beaconLimit
}

type beaconBody struct {
	E string `json:"e"`
	T string `json:"t"`
	P string `json:"p"`
}

// Beacon returns the handler for POST /e. It answers 204 to everything: the
// page must never wait for, or learn anything from, the counter.
func (r *Recorder) Beacon(trustProxy bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer w.WriteHeader(http.StatusNoContent)

		if !r.lim.allow(limiterKey(clientIP(req, trustProxy))) {
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, req.Body, beaconMaxBody))
		if err != nil {
			return
		}
		var b beaconBody
		if json.Unmarshal(raw, &b) != nil {
			return
		}
		r.Click(b.E, b.T, b.P)
	})
}

func clientIP(req *http.Request, trustProxy bool) netip.Addr {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		// Unparseable peers share one zero-value key, and so one budget.
		return netip.Addr{}
	}
	a = a.Unmap()
	if trustProxy && a.IsLoopback() {
		if v := strings.TrimSpace(req.Header.Get("X-Real-IP")); v != "" {
			if fwd, err := netip.ParseAddr(v); err == nil {
				return fwd.Unmap()
			}
		}
	}
	return a
}

// limiterKey groups IPv6 clients by /64: a subscriber is usually handed a
// whole /64 and could otherwise rotate through it to get a fresh budget on
// every request. IPv4 addresses are kept whole.
func limiterKey(a netip.Addr) netip.Prefix {
	if !a.IsValid() {
		return netip.Prefix{}
	}
	a = a.WithZone("")
	bits := 32
	if a.Is6() {
		bits = 64
	}
	p, err := a.Prefix(bits)
	if err != nil {
		return netip.Prefix{}
	}
	return p
}
