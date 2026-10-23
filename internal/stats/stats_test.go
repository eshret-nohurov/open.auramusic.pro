package stats

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/preview"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func (c *clock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

// cacheSource behaves like preview.Store.TakeCounters: every take returns
// what was counted since the previous one and starts again from zero.
type cacheSource struct {
	mu sync.Mutex
	c  preview.Counters
}

func (s *cacheSource) add(hits, misses, failures int64) {
	s.mu.Lock()
	s.c.Hits += hits
	s.c.Misses += misses
	s.c.Failures += failures
	s.mu.Unlock()
}

func (s *cacheSource) take() preview.Counters {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.c
	s.c = preview.Counters{}
	return c
}

func newClock() *clock { return &clock{t: time.Date(2026, 10, 21, 14, 5, 0, 0, time.UTC)} }

func lines(t *testing.T, dir string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "stats.jsonl"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

func TestFlushHourRolloverAndShape(t *testing.T) {
	dir := t.TempDir()
	c := newClock()
	cache := &cacheSource{}
	r := New(Options{DataDir: filepath.Join(dir, "sub"), Now: c.Now, Cache: cache.take})

	r.View("track", "ios", "tm")
	r.View("track", "ios", "tm")
	r.NotFound("ios")
	r.Failed("android")
	r.BotView("telegram")
	r.Click("app_store", "track", "ios")

	// Several minute ticks within hour 14: each take resets the source, so
	// the line must carry the sum of all of them, not the last one.
	for _, n := range [][3]int64{{4, 1, 0}, {0, 0, 0}, {6, 2, 0}} {
		cache.add(n[0], n[1], n[2])
		c.Add(time.Minute)
		if err := r.flush(false); err != nil {
			t.Fatal(err)
		}
	}
	if got := lines(t, filepath.Join(dir, "sub")); got != nil {
		t.Fatalf("running hour must not be written, got %v", got)
	}

	c.Add(time.Hour)
	r.View("album", "desktop", "ru") // belongs to hour 15
	cache.add(5, 0, 0)               // taken after hour 15 began: belongs to it
	if err := r.flush(false); err != nil {
		t.Fatal(err)
	}
	got := lines(t, filepath.Join(dir, "sub"))
	want := `{"hour":"2026-10-21T14","view":{"track.ios.tm":2},"not_found":{"ios":1},"failed":{"android":1},"bot_view":{"telegram":1},"click":{"app_store.track.ios":1},"cache":{"hits":10,"misses":3,"failures":0}}`
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}

	// Hour 16 is empty: nothing written for it.
	c.Add(time.Hour)
	c.Add(time.Hour)
	r.flush(false)
	got = lines(t, filepath.Join(dir, "sub"))
	want = `{"hour":"2026-10-21T15","view":{"album.desktop.ru":1},"cache":{"hits":5,"misses":0,"failures":0}}`
	if len(got) != 2 || got[1] != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	c.Add(time.Hour)
	r.flush(false)
	if got := lines(t, filepath.Join(dir, "sub")); len(got) != 2 {
		t.Fatalf("empty hour wrote a line: %v", got)
	}

	// Card images hit the cache without a page view: an hour with only cache
	// activity is still written. The take at the tick that ends the hour
	// goes to the hour that is ending.
	cache.add(0, 0, 2)
	c.Add(time.Hour)
	r.flush(false)
	got = lines(t, filepath.Join(dir, "sub"))
	want = `{"hour":"2026-10-21T18","cache":{"hits":0,"misses":0,"failures":2}}`
	if len(got) != 3 || got[2] != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	for _, l := range lines(t, filepath.Join(dir, "sub")) {
		if !json.Valid([]byte(l)) {
			t.Fatalf("invalid json: %s", l)
		}
	}
}

// An event stamped with an hour older than the running one, from a clock
// stepped back or a caller that read the clock just before the hour turned,
// must not move the bucket back and split the hour into several lines.
func TestHourOnlyMovesForward(t *testing.T) {
	dir := t.TempDir()
	c := &clock{t: time.Date(2026, 10, 21, 11, 0, 0, 0, time.UTC)}
	r := New(Options{DataDir: dir, Now: c.Now})
	r.View("track", "ios", "tm")
	c.Set(time.Date(2026, 10, 21, 10, 59, 59, 999e6, time.UTC))
	r.View("track", "ios", "tm")
	r.flush(false)
	c.Set(time.Date(2026, 10, 21, 11, 0, 1, 0, time.UTC))
	r.View("track", "ios", "tm")
	if got := lines(t, dir); got != nil {
		t.Fatalf("hour 11 is still running, got %v", got)
	}
	c.Set(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	r.flush(false)
	got := lines(t, dir)
	if len(got) != 1 || got[0] != `{"hour":"2026-10-21T11","view":{"track.ios.tm":3}}` {
		t.Fatalf("got %v", got)
	}
}

func TestCloseWritesPartialHour(t *testing.T) {
	dir := t.TempDir()
	c := newClock()
	r := New(Options{DataDir: dir, Now: c.Now})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if lines(t, dir) != nil {
		t.Fatal("empty Close must not write")
	}
	r.View("playlist", "android", "ru")
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	got := lines(t, dir)
	if len(got) != 1 || got[0] != `{"hour":"2026-10-21T14","view":{"playlist.android.ru":1}}` {
		t.Fatalf("got %v", got)
	}
	r.Close() // second Close does not duplicate
	if len(lines(t, dir)) != 1 {
		t.Fatal("duplicate line after second Close")
	}
}

func TestInvalidValuesIgnored(t *testing.T) {
	dir := t.TempDir()
	r := New(Options{DataDir: dir, Now: newClock().Now})
	long := strings.Repeat("a", 100)
	r.View("song", "ios", "tm")
	r.View("track", "tv", "tm")
	r.View("track", "ios", "en")
	r.View(long, "ios", "tm")
	r.NotFound("tv")
	r.Failed(long)
	r.BotView("")
	r.BotView("Telegram")
	r.BotView("tele.gram")
	r.BotView(long)
	r.Click("hack", "track", "ios")
	r.Click("app_store", "song", "ios")
	r.Click("app_store", "track", "tv")
	r.Close()
	if lines(t, dir) != nil {
		t.Fatalf("invalid values were counted: %v", lines(t, dir))
	}
}

func beacon(h http.Handler, body, remote string, hdr map[string]string) int {
	req := httptest.NewRequest(http.MethodPost, "/e", strings.NewReader(body))
	req.RemoteAddr = remote
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Body.Len() != 0 {
		return -1
	}
	return rec.Code
}

const okBody = `{"e":"app_store","t":"track","p":"ios"}`

func clicks(t *testing.T, r *Recorder) int64 {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	return sum(r.cur.click)
}

func TestBeaconBodies(t *testing.T) {
	r := New(Options{DataDir: t.TempDir(), Now: newClock().Now})
	h := r.Beacon(false)
	if c := beacon(h, okBody, "203.0.113.5:1000", nil); c != 204 {
		t.Fatal(c)
	}
	if clicks(t, r) != 1 {
		t.Fatal("valid beacon not counted")
	}
	bad := []string{
		"", "not json", "[]", `{"e":"x","t":"track","p":"ios"}`,
		`{"e":"app_store","t":"song","p":"ios"}`, `{"e":"app_store","t":"track","p":"tv"}`,
		`{"e":"app_store","t":"track","p":"ios","pad":"` + strings.Repeat("x", 2000) + `"}`,
	}
	for i, b := range bad {
		// distinct clients so the limiter is not what drops them
		if c := beacon(h, b, "198.51.100."+string(rune('1'+i))+":1", nil); c != 204 {
			t.Fatalf("%q: %d", b, c)
		}
	}
	if clicks(t, r) != 1 {
		t.Fatalf("garbage was counted: %d", clicks(t, r))
	}
}

func TestBeaconRateLimit(t *testing.T) {
	c := newClock()
	r := New(Options{DataDir: t.TempDir(), Now: c.Now})
	h := r.Beacon(false)
	for i := 0; i < 25; i++ {
		if code := beacon(h, okBody, "203.0.113.5:1", nil); code != 204 {
			t.Fatal(code)
		}
	}
	if n := clicks(t, r); n != 20 {
		t.Fatalf("want 20 counted, got %d", n)
	}
	beacon(h, okBody, "203.0.113.6:1", nil)
	if n := clicks(t, r); n != 21 {
		t.Fatalf("other IP not counted: %d", n)
	}
	c.Add(61 * time.Second)
	beacon(h, okBody, "203.0.113.5:1", nil)
	if n := clicks(t, r); n != 22 {
		t.Fatalf("limit did not reset: %d", n)
	}
	// Both old entries are swept on the next call after the window.
	c.Add(2 * time.Minute)
	beacon(h, okBody, "203.0.113.7:1", nil)
	r.lim.mu.Lock()
	n := len(r.lim.hits)
	r.lim.mu.Unlock()
	if n != 1 {
		t.Fatalf("limiter kept %d addresses", n)
	}
}

func limiterSize(r *Recorder) int {
	r.lim.mu.Lock()
	defer r.lim.mu.Unlock()
	return len(r.lim.hits)
}

func TestBeaconLimiterFull(t *testing.T) {
	c := newClock()
	r := New(Options{DataDir: t.TempDir(), Now: c.Now})
	r.lim.max = 3
	h := r.Beacon(false)
	for _, ip := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.3"} {
		beacon(h, okBody, ip+":1", nil)
	}
	r.lim.mu.Lock()
	swept := r.lim.lastSweep
	r.lim.mu.Unlock()

	// Full: newcomers are refused, known clients still count, and no extra
	// sweep runs within the window.
	c.Add(30 * time.Second)
	for i := 0; i < 5; i++ {
		beacon(h, okBody, "198.51.100."+string(rune('1'+i))+":1", nil)
	}
	beacon(h, okBody, "203.0.113.1:1", nil)
	if n := clicks(t, r); n != 4 {
		t.Fatalf("want 4 counted, got %d", n)
	}
	r.lim.mu.Lock()
	again := r.lim.lastSweep
	r.lim.mu.Unlock()
	if !again.Equal(swept) || limiterSize(r) != 3 {
		t.Fatalf("swept within the window or grew past the cap: size %d", limiterSize(r))
	}

	// Once the window is over the sweep frees room for new clients.
	c.Add(31 * time.Second)
	beacon(h, okBody, "198.51.100.9:1", nil)
	if n := clicks(t, r); n != 5 {
		t.Fatalf("newcomer refused after the sweep: %d", n)
	}
	if limiterSize(r) != 1 {
		t.Fatalf("limiter kept %d addresses", limiterSize(r))
	}
}

func TestBeaconIPv6SharesSlash64(t *testing.T) {
	r := New(Options{DataDir: t.TempDir(), Now: newClock().Now})
	h := r.Beacon(false)
	// Twenty-five different addresses in one /64 share one budget.
	for i := 0; i < 25; i++ {
		beacon(h, okBody, fmt.Sprintf("[2001:db8:1:2::%x]:1", i+1), nil)
	}
	if n := clicks(t, r); n != 20 {
		t.Fatalf("same /64: want 20, got %d", n)
	}
	beacon(h, okBody, "[2001:db8:1:3::a]:1", nil)
	if n := clicks(t, r); n != 21 {
		t.Fatalf("other /64 not counted: %d", n)
	}
	// IPv4-mapped addresses are plain IPv4 and keep their own budget.
	beacon(h, okBody, "[::ffff:203.0.113.9]:1", nil)
	if n := clicks(t, r); n != 22 {
		t.Fatalf("mapped IPv4: %d", n)
	}
	if limiterSize(r) != 3 {
		t.Fatalf("want 3 keys, got %d", limiterSize(r))
	}
}

func TestBeaconRealIP(t *testing.T) {
	spam := func(h http.Handler, remote, real string) {
		hdr := map[string]string{}
		if real != "" {
			hdr["X-Real-IP"] = real
		}
		for i := 0; i < 25; i++ {
			beacon(h, okBody, remote, hdr)
		}
	}
	// Trusted proxy: two real clients behind 127.0.0.1 are limited separately.
	r := New(Options{DataDir: t.TempDir(), Now: newClock().Now})
	spam(r.Beacon(true), "127.0.0.1:5", "203.0.113.1")
	spam(r.Beacon(true), "[::1]:5", "203.0.113.2")
	if n := clicks(t, r); n != 40 {
		t.Fatalf("trusted: %d", n)
	}
	// Not trusted: header ignored, one peer shares one budget.
	r = New(Options{DataDir: t.TempDir(), Now: newClock().Now})
	spam(r.Beacon(false), "127.0.0.1:5", "203.0.113.1")
	spam(r.Beacon(false), "127.0.0.1:5", "203.0.113.2")
	if n := clicks(t, r); n != 20 {
		t.Fatalf("untrusted trustProxy=false: %d", n)
	}
	// Header from a non-loopback peer is ignored even with trustProxy.
	r = New(Options{DataDir: t.TempDir(), Now: newClock().Now})
	spam(r.Beacon(true), "198.51.100.9:5", "203.0.113.1")
	spam(r.Beacon(true), "198.51.100.9:5", "203.0.113.2")
	if n := clicks(t, r); n != 20 {
		t.Fatalf("non-loopback peer: %d", n)
	}
}

func TestConcurrentCountAndFlush(t *testing.T) {
	dir := t.TempDir()
	c := newClock()
	r := New(Options{DataDir: dir, Now: c.Now})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				r.View("track", "ios", "tm")
				r.Click("open_app", "album", "android")
				if i%100 == 0 {
					c.Add(time.Hour)
					r.flush(false)
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			r.Close()
		}
	}()
	wg.Wait()
	cancel()
	<-done
	r.Close()

	var views int64
	for _, l := range lines(t, dir) {
		var v line
		if err := json.Unmarshal([]byte(l), &v); err != nil {
			t.Fatal(err)
		}
		views += sum(v.View)
	}
	if views != 8*500 {
		t.Fatalf("views written %d, want 4000", views)
	}
}

func TestNoIPInFileOrLogs(t *testing.T) {
	dir := t.TempDir()
	var logs bytes.Buffer
	r := New(Options{DataDir: dir, Now: newClock().Now,
		Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	h := r.Beacon(true)
	beacon(h, okBody, "127.0.0.1:1", map[string]string{"X-Real-IP": "203.0.113.77"})
	beacon(h, okBody, "192.0.2.44:9", nil)
	r.Close()
	b, _ := os.ReadFile(filepath.Join(dir, "stats.jsonl"))
	all := string(b) + logs.String()
	if logs.Len() == 0 || len(b) == 0 {
		t.Fatal("expected a log line and a file line")
	}
	for _, ip := range []string{"203.0.113.77", "192.0.2.44", "127.0.0.1"} {
		if strings.Contains(all, ip) {
			t.Fatalf("%s leaked", ip)
		}
	}
}

func TestReport(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "stats.jsonl")
	// 2026-10-21T20 UTC is 2026-10-22 01:00 in Turkmenistan.
	data := strings.Join([]string{
		`{"hour":"2026-10-21T10","view":{"track.ios.tm":6,"album.android.ru":2},"not_found":{"ios":1},"bot_view":{"telegram":3},"click":{"app_store.track.ios":2}}`,
		`{"hour":"2026-10-21T10","view":{"track.ios.tm":2},"failed":{"android":1},"click":{"google_play.album.android":1}}`,
		`{"hour":"2026-10-21T20","view":{"playlist.desktop.tm":5},"bot_view":{"telegram":1,"whatsapp":2},"click":{"open_app.playlist.desktop":1}}`,
		`garbage`,
		`{"hour":"2026-10-01T10","view":{"track.ios.tm":99}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	if err := Report(&out, p, 2, now); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	t.Log("\n" + s)
	rows := strings.Split(s, "\n")
	f0 := strings.Fields(rows[1])
	f1 := strings.Fields(rows[2])
	if f0[0] != "2026-10-22" || f0[1] != "5" || f0[4] != "5" || f0[11] != "1" || f0[14] != "3" {
		t.Fatalf("day 22: %v", f0)
	}
	// day 21: 10 views, 1 not_found, 1 failed of 12 opens
	if f1[0] != "2026-10-21" || f1[1] != "10" || f1[12] != "8.3" || f1[13] != "8.3" || f1[9] != "2" || f1[10] != "1" || f1[14] != "3" {
		t.Fatalf("day 21: %v", f1)
	}
	if !strings.Contains(s, "telegram  4") || !strings.Contains(s, "whatsapp  2") {
		t.Fatalf("bots: %s", s)
	}
	if strings.Contains(s, "99") {
		t.Fatal("old day included")
	}

	err := Report(&out, filepath.Join(dir, "none.jsonl"), 7, now)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing file: %v", err)
	}
}
