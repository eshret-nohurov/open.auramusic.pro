package imaging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/mockbackend"
)

// guard records every host the fetcher dials and refuses anything that is
// not the local test server, so a bug cannot reach the real network.
type guard struct {
	mu    sync.Mutex
	hosts []string
	next  http.RoundTripper
}

func (g *guard) RoundTrip(req *http.Request) (*http.Response, error) {
	g.mu.Lock()
	g.hosts = append(g.hosts, req.URL.Host)
	g.mu.Unlock()
	if !strings.HasPrefix(req.URL.Host, "127.0.0.1:") {
		return nil, errors.New("test tried to leave localhost: " + req.URL.Host)
	}
	return g.next.RoundTrip(req)
}

type fetchEnv struct {
	srv   *httptest.Server
	host  string
	f     *Fetcher
	g     *guard
	hits  *atomic.Int64
	logs  *bytes.Buffer
	cover func(name string) string
}

func newFetchEnv(t *testing.T, o FetchOptions) *fetchEnv {
	t.Helper()
	mock := mockbackend.New(os.DirFS("../../testdata/mock"), "t", nil)
	webp, err := os.ReadFile("testdata/cover.webp")
	if err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/hop":
			http.Redirect(w, r, "/covers/egoist.jpg", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/covers/cover.webp":
			w.Header().Set("Content-Type", "image/webp")
			w.Write(webp)
		default:
			mock.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	env := &fetchEnv{srv: srv, host: strings.TrimPrefix(srv.URL, "http://"), hits: &hits, logs: &bytes.Buffer{}}
	if o.Hosts == nil {
		o.Hosts = []string{env.host}
	}
	o.Logger = slog.New(slog.NewTextHandler(env.logs, nil))
	env.f = NewFetcher(o)
	env.g = &guard{next: env.f.http.Transport}
	env.f.http.Transport = env.g
	env.cover = func(name string) string { return srv.URL + "/covers/" + name }
	return env
}

func TestFetchJPEG(t *testing.T) {
	e := newFetchEnv(t, FetchOptions{AllowHTTP: true})
	img, err := e.f.Fetch(context.Background(), e.cover("egoist.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 1000 || b.Dy() != 1000 {
		t.Errorf("bounds %v", b)
	}
}

func TestFetchWebP(t *testing.T) {
	e := newFetchEnv(t, FetchOptions{AllowHTTP: true})
	img, err := e.f.Fetch(context.Background(), e.cover("cover.webp"))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 48 {
		t.Errorf("bounds %v", b)
	}
}

func TestFetchSameHostRedirect(t *testing.T) {
	e := newFetchEnv(t, FetchOptions{AllowHTTP: true})
	if _, err := e.f.Fetch(context.Background(), e.srv.URL+"/hop"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.f.Fetch(context.Background(), e.srv.URL+"/loop"); !errors.Is(err, errRedirect) {
		t.Errorf("redirect loop: %v", err)
	}
	if n := e.hits.Load(); n != 2+maxRedirects+1 {
		t.Errorf("server saw %d requests", n)
	}
}

func TestFetchHostPatterns(t *testing.T) {
	f := NewFetcher(FetchOptions{Hosts: []string{"*.ma.st.com.tm", "img.example.com:8443"}})
	for host, want := range map[string]bool{
		"storage2.ma.st.com.tm":          true,
		"cache.ma.st.com.tm":             true,
		"a.b.ma.st.com.tm":               true,
		"STORAGE2.MA.ST.COM.TM":          true,
		"img.example.com:8443":           true,
		"img.example.com":                false,
		"ma.st.com.tm":                   false,
		"evilma.st.com.tm":               false,
		"storage2.ma.st.com.tm:8443":     false,
		"storage2.ma.st.com.tm.evil.com": false,
		".ma.st.com.tm":                  false,
		"a..ma.st.com.tm":                false,
		"-a.ma.st.com.tm":                false,
		"a_b.ma.st.com.tm":               false,
		"evil.com/.ma.st.com.tm":         false,
		"evil.com@x.ma.st.com.tm":        false,
	} {
		u := &url.URL{Scheme: "https", Host: host, Path: "/cover.webp"}
		if got := f.check(u) == nil; got != want {
			t.Errorf("%s: allowed=%v, want %v", host, got, want)
		}
	}
}

func TestFetchRefusals(t *testing.T) {
	cases := []struct {
		name   string
		opts   FetchOptions
		url    func(e *fetchEnv) string
		want   error
		noHits bool
	}{
		{"host not allowed", FetchOptions{Hosts: []string{"cache.ma.st.com.tm"}, AllowHTTP: true},
			func(e *fetchEnv) string { return e.cover("egoist.jpg") }, errHost, true},
		{"http without AllowHTTP", FetchOptions{},
			func(e *fetchEnv) string { return e.cover("egoist.jpg") }, errScheme, true},
		{"other scheme", FetchOptions{AllowHTTP: true},
			func(e *fetchEnv) string { return "file:///etc/passwd" }, errScheme, true},
		{"user info", FetchOptions{AllowHTTP: true},
			func(e *fetchEnv) string { return "http://u:p@" + e.host + "/covers/egoist.jpg" }, errHost, true},
		{"redirect to other host", FetchOptions{AllowHTTP: true},
			func(e *fetchEnv) string { return e.cover("redirect.jpg") }, errRedirect, false},
		{"broken body", FetchOptions{AllowHTTP: true},
			func(e *fetchEnv) string { return e.cover("broken.jpg") }, errFormat, false},
		{"8000×8000 header", FetchOptions{AllowHTTP: true},
			func(e *fetchEnv) string { return e.cover("huge.png") }, errDims, false},
		{"over MaxBytes", FetchOptions{AllowHTTP: true, MaxBytes: 4 << 10},
			func(e *fetchEnv) string { return e.cover("egoist.jpg") }, errTooBig, false},
		{"over MaxDim", FetchOptions{AllowHTTP: true, MaxDim: 999},
			func(e *fetchEnv) string { return e.cover("egoist.jpg") }, errDims, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newFetchEnv(t, c.opts)
			img, err := e.f.Fetch(context.Background(), c.url(e))
			if img != nil || !errors.Is(err, c.want) {
				t.Errorf("got %v, %v; want %v", img, err, c.want)
			}
			if c.noHits && e.hits.Load() != 0 {
				t.Errorf("server was asked %d times", e.hits.Load())
			}
			for _, h := range e.g.hosts {
				if h != e.host {
					t.Errorf("dialled %s", h)
				}
			}
		})
	}
}

func TestFetchNotFound(t *testing.T) {
	e := newFetchEnv(t, FetchOptions{AllowHTTP: true})
	_, err := e.f.Fetch(context.Background(), e.cover("missing.jpg"))
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err %v", err)
	}
	if !strings.Contains(e.logs.String(), "status=404") {
		t.Errorf("log %q", e.logs.String())
	}
}

func TestFetchLogsHostOnlyAndThrottles(t *testing.T) {
	e := newFetchEnv(t, FetchOptions{AllowHTTP: true})
	secret := "?code=AbCdEfGh12345678"
	for range 5 {
		e.f.Fetch(context.Background(), e.cover("missing.jpg")+secret)
		e.f.Fetch(context.Background(), e.cover("redirect.jpg"))
	}
	logs := e.logs.String()
	if n := strings.Count(logs, "cover fetch failed"); n != 1 {
		t.Errorf("%d log lines, want 1 per host per minute:\n%s", n, logs)
	}
	for _, leak := range []string{"AbCdEfGh12345678", "/covers/", "missing.jpg", "example.com"} {
		if strings.Contains(logs, leak) {
			t.Errorf("log leaks %q: %s", leak, logs)
		}
	}
	if !strings.Contains(logs, "host="+e.host) {
		t.Errorf("log has no host: %s", logs)
	}
}
