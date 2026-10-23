package backend

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/lang"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/mockbackend"
)

const token = "test-token-7f3a9c"

type logBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}
func (l *logBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.buf.String() }

func newClient(t *testing.T, base string, timeout time.Duration) (*Client, *logBuf) {
	t.Helper()
	logs := &logBuf{}
	return New(Options{
		BaseURL: base, Token: token, Version: "test", Timeout: timeout, MaxInflight: 4,
		Logger: slog.New(slog.NewJSONHandler(logs, nil)),
	}), logs
}

func mock(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(mockbackend.New(os.DirFS("../../testdata/mock"), token, nil))
	t.Cleanup(ts.Close)
	return ts
}

func TestFixtures(t *testing.T) {
	ts := mock(t)
	c, logs := newClient(t, ts.URL, time.Second)
	ctx := context.Background()

	p, err := c.Fetch(ctx, "TrackCyrillic001")
	if err != nil || p.Type != TypeTrack || p.Title != "Эгоист" || p.ArtistName != "Navai" ||
		!strings.HasPrefix(p.ArtworkURL, ts.URL+"/covers/") {
		t.Errorf("track: %+v %v", p, err)
	}

	p, err = c.Fetch(ctx, "PlaylistTmRu0001")
	if err != nil || p.TitleFor(lang.TM) != "Hepdäniň hitleri" || p.TitleFor(lang.RU) != "Хиты недели" {
		t.Errorf("playlist titles: %+v %v", p, err)
	}
	if !strings.HasSuffix(p.ArtworkFor(lang.TM), "playlist-tm.jpg") || !strings.HasSuffix(p.ArtworkFor(lang.RU), "playlist-ru.jpg") {
		t.Errorf("playlist covers: tm %s, ru %s", p.ArtworkFor(lang.TM), p.ArtworkFor(lang.RU))
	}

	p, err = c.Fetch(ctx, "NullFields000001")
	if err != nil || p.ArtistName != "" || p.ThumbURL != "" || p.TitleRU != "" || p.Title != "Null meýdanlar" {
		t.Errorf("null fields: %+v %v", p, err)
	}

	p, err = c.Fetch(ctx, "UnknownType00001")
	if err != nil || p.Type != "user_playlist" || KnownType(p.Type) {
		t.Errorf("unknown type: %+v %v", p, err)
	}
	if !strings.Contains(logs.String(), `"type":"user_playlist"`) || strings.Contains(logs.String(), "UnknownType00001") {
		t.Errorf("unknown type log: %s", logs.String())
	}

	p, err = c.Fetch(ctx, "LongTitle0000001")
	if err != nil || utf8.RuneCountInString(p.Title) < 200 || utf8.RuneCountInString(p.Title) > maxTextRunes {
		t.Errorf("long title: %d runes, %v", utf8.RuneCountInString(p.Title), err)
	}

	if _, err := c.Fetch(ctx, "NotFound00000404"); !errors.Is(err, ErrNotFound) {
		t.Errorf("404 fixture: %v", err)
	}
	if _, err := c.Fetch(ctx, "ZZZZZZZZZZZZZZZZ"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing fixture: %v", err)
	}
}

func failureKind(err error) string {
	var f *Failure
	if errors.As(err, &f) {
		return f.Kind
	}
	return ""
}

func TestFailures(t *testing.T) {
	ts := mock(t)
	c, logs := newClient(t, ts.URL, 500*time.Millisecond)
	ctx := context.Background()

	for code, kind := range map[string]string{
		"ServerError00500": "status",
		"Unauthorized0401": "auth",
		"SlowBackend00004": "network",
		"NoTitle000000001": "no_title",
	} {
		_, err := c.Fetch(ctx, code)
		if failureKind(err) != kind {
			t.Errorf("%s: err %v, want kind %s", code, err, kind)
		}
	}
	out := logs.String()
	if !strings.Contains(out, `"level":"ERROR"`) || !strings.Contains(out, "allowlist") {
		t.Errorf("auth failure not logged as error: %s", out)
	}
	if strings.Contains(out, token) {
		t.Error("token leaked into the log")
	}
	for _, code := range []string{"ServerError00500", "Unauthorized0401", "SlowBackend00004"} {
		if strings.Contains(out, code) {
			t.Errorf("code %s leaked into the log", code)
		}
	}
}

func TestWrongTokenIsAuthFailure(t *testing.T) {
	ts := mock(t)
	c := New(Options{BaseURL: ts.URL, Token: "wrong", Timeout: time.Second, MaxInflight: 1})
	if _, err := c.Fetch(context.Background(), "TrackCyrillic001"); failureKind(err) != "auth" {
		t.Errorf("err = %v", err)
	}
}

func TestFailureLogIsThrottled(t *testing.T) {
	ts := mock(t)
	c, logs := newClient(t, ts.URL, time.Second)
	for range 5 {
		c.Fetch(context.Background(), "ServerError00500")
	}
	if n := strings.Count(logs.String(), "backend request failed"); n != 1 {
		t.Errorf("logged %d times", n)
	}
}

func TestRequestShape(t *testing.T) {
	var got *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Write([]byte(`{"type":"track","title":"x"}`))
	}))
	defer ts.Close()
	c, _ := newClient(t, ts.URL, time.Second)
	if _, err := c.Fetch(context.Background(), "Xk3pQ9aB7mLzT2wR"); err != nil {
		t.Fatal(err)
	}
	if got.URL.Path != "/api/v1/internal/deeplink" || got.URL.RawQuery != "code=Xk3pQ9aB7mLzT2wR" {
		t.Errorf("url = %s", got.URL)
	}
	if got.Header.Get("Authorization") != "Bearer "+token || got.Header.Get("Accept") != "application/json" ||
		got.Header.Get("User-Agent") != "BeletOpen/test" {
		t.Errorf("headers = %v", got.Header)
	}
}

func TestRedirectNotFollowed(t *testing.T) {
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("Authorization") != ""
		w.Write([]byte(`{"type":"track","title":"x"}`))
	}))
	defer other.Close()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusFound)
	}))
	defer ts.Close()
	c, _ := newClient(t, ts.URL, time.Second)
	if _, err := c.Fetch(context.Background(), "Xk3pQ9aB7mLzT2wR"); failureKind(err) != "network" {
		t.Errorf("redirect: err = %v", err)
	}
	if leaked {
		t.Error("token sent to the redirect target")
	}
}

func TestBadBodies(t *testing.T) {
	for name, body := range map[string]string{
		"html":     "<html>oops</html>",
		"array":    `[1,2]`,
		"too big":  `{"type":"track","title":"` + strings.Repeat("a", maxBody) + `"}`,
		"no title": `{"type":"track","title":"  ","title_ru":null}`,
	} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(body))
		}))
		c, _ := newClient(t, ts.URL, time.Second)
		if _, err := c.Fetch(context.Background(), "Xk3pQ9aB7mLzT2wR"); failureKind(err) == "" {
			t.Errorf("%s: err = %v", name, err)
		}
		ts.Close()
	}
}

func TestBusyWhenAllSlotsTaken(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.Write([]byte(`{"type":"track","title":"x"}`))
	}))
	defer ts.Close()
	defer close(release)
	c := New(Options{BaseURL: ts.URL, Token: token, Timeout: 5 * time.Second, MaxInflight: 1})
	c.slotWait = 50 * time.Millisecond
	go c.Fetch(context.Background(), "AAAAAAAAAAAAAAAA")
	time.Sleep(20 * time.Millisecond)
	if _, err := c.Fetch(context.Background(), "BBBBBBBBBBBBBBBB"); failureKind(err) != "busy" {
		t.Errorf("err = %v", err)
	}
}

func TestTitleAndArtworkOrder(t *testing.T) {
	p := Preview{Title: "n", TitleRU: "r", TitleTM: "t"}
	if p.TitleFor(lang.TM) != "t" || p.TitleFor(lang.RU) != "r" {
		t.Error("language title not preferred")
	}
	p = Preview{TitleRU: "r"}
	if p.TitleFor(lang.TM) != "r" {
		t.Error("tm should fall back to ru")
	}
	p = Preview{ThumbURL: "th", ThumbURLTM: "thtm"}
	if p.ArtworkFor(lang.TM) != "thtm" || p.ArtworkFor(lang.RU) != "th" {
		t.Error("thumb fallback order wrong")
	}
	p = Preview{ArtworkURLTM: "atm", ThumbURL: "th"}
	if p.ArtworkFor(lang.RU) != "atm" {
		t.Error("ru should take tm artwork before thumbnails")
	}
}

func TestParseTrimsAndClips(t *testing.T) {
	p, err := parsePreview([]byte(`{"type":" track ","title":"  ` + strings.Repeat("ý", 400) + `  ","artist_name":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != "track" || utf8.RuneCountInString(p.Title) != maxTextRunes || !utf8.ValidString(p.Title) || p.ArtistName != "" {
		t.Errorf("%+v", p)
	}
}
