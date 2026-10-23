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

// limiter is a fixed-window counter per client address. Addresses live only
// in memory and are swept once their window is over.
type limiter struct {
	now       func() time.Time
	mu        sync.Mutex
	hits      map[string]*window
	lastSweep time.Time
}

type window struct {
	start time.Time
	n     int
}

func (l *limiter) allow(ip string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string]*window{}
	}
	if now.Sub(l.lastSweep) >= beaconWindow || len(l.hits) >= limiterMaxEntries {
		for k, w := range l.hits {
			if now.Sub(w.start) >= beaconWindow {
				delete(l.hits, k)
			}
		}
		l.lastSweep = now
	}
	w := l.hits[ip]
	if w == nil || now.Sub(w.start) >= beaconWindow {
		if w == nil && len(l.hits) >= limiterMaxEntries {
			return false
		}
		l.hits[ip] = &window{start: now, n: 1}
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

		if !r.lim.allow(clientIP(req, trustProxy)) {
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

func clientIP(req *http.Request, trustProxy bool) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	if trustProxy && (host == "127.0.0.1" || host == "::1") {
		if v := strings.TrimSpace(req.Header.Get("X-Real-IP")); v != "" {
			if a, err := netip.ParseAddr(v); err == nil {
				return a.Unmap().String()
			}
		}
	}
	return host
}
