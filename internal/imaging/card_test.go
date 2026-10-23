package imaging

import (
	"context"
	"encoding/json"
	"image"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/mockbackend"
)

const listen = "Belet Music-de diňläň"

// Type labels as the app says them in Turkmen, keyed by app translation key.
var labels = map[string]string{
	"track":          "Aýdym",          // track
	"album":          "Albom",          // album
	"playlist":       "Pleýlist",       // playlist
	"artist":         "Artist",         // artist
	"video":          "Wideo",          // share_type_video
	"video_playlist": "Wideo pleýlist", // share_type_video_playlist
}

type fixture struct {
	code  string
	typ   string
	card  Card
	cover string // with {{mock}} still in it
}

func loadFixtures(t *testing.T) []fixture {
	t.Helper()
	paths, err := filepath.Glob("../../testdata/mock/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatal("no fixtures", err)
	}
	var out []fixture
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(p, err)
		}
		str := func(keys ...string) string {
			for _, k := range keys {
				if s, ok := m[k].(string); ok && s != "" {
					return s
				}
			}
			return ""
		}
		title := str("title_tm", "title", "title_ru")
		if title == "" {
			continue
		}
		typ := str("type")
		out = append(out, fixture{
			code:  strings.TrimSuffix(filepath.Base(p), ".json"),
			typ:   typ,
			card:  Card{Shape: ShapeFor(typ), Label: labels[typ], Title: title, Artist: str("artist_name"), Listen: listen},
			cover: str("artwork_url_tm", "artwork_url", "thumb_url"),
		})
	}
	return out
}

func checkLayout(t *testing.T, name string, lay cardLayout) {
	t.Helper()
	if len(lay.lines) == 0 {
		t.Errorf("%s: no text", name)
	}
	for _, l := range lay.lines {
		if l.x+l.width > textRight {
			t.Errorf("%s: %q ends at x=%d, past %d", name, l.text, l.x+l.width, textRight)
		}
		// Keep clear of the logo row at the bottom.
		if l.top < 0 || l.bottom > cardH-52-markSize {
			t.Errorf("%s: %q spans y %d..%d", name, l.text, l.top, l.bottom)
		}
	}
}

func TestCardLayoutAllFixtures(t *testing.T) {
	r := renderer(t)
	f := r.newFaces()
	for _, fx := range loadFixtures(t) {
		for _, shape := range []Shape{Square, Circle, Wide} {
			c := fx.card
			c.Shape = shape
			sl := r.statics.slots[shape]
			lay := layoutCard(f, c, sl.textX, sl.rect.Min.Y+sl.rect.Dy()/2)
			checkLayout(t, fx.code, lay)
		}
	}
}

func TestCardLongTitleIsCut(t *testing.T) {
	r := renderer(t)
	var long fixture
	for _, fx := range loadFixtures(t) {
		if fx.code == "LongTitle0000001" {
			long = fx
		}
	}
	if len(long.card.Title) < 200 {
		t.Fatal("long title fixture missing")
	}
	for _, shape := range []Shape{Square, Circle, Wide} {
		long.card.Shape = shape
		sl := r.statics.slots[shape]
		lay := layoutCard(r.newFaces(), long.card, sl.textX, 315)
		if !lay.truncated || lay.titleSize != titleSizeAlt {
			t.Errorf("shape %v: truncated=%v size=%d", shape, lay.truncated, lay.titleSize)
		}
		var title, artist []textLine
		for _, l := range lay.lines {
			switch l.color {
			case colText:
				title = append(title, l)
			case colArtist:
				artist = append(artist, l)
			}
		}
		if len(title) != 3 || !strings.HasSuffix(title[2].text, ellipsis) {
			t.Errorf("shape %v: title lines %d, last %q", shape, len(title), title[len(title)-1].text)
		}
		if len(artist) != 1 || !strings.HasSuffix(artist[0].text, ellipsis) {
			t.Errorf("shape %v: artist %v", shape, artist)
		}
	}
}

func TestWrapSplitsLongWord(t *testing.T) {
	f := renderer(t).newFaces()
	word := strings.Repeat("Ýüregimiňtöründe", 6)
	lines := wrap(f.title, word, 576)
	if len(lines) < 2 || strings.Join(lines, "") != word {
		t.Fatalf("lines %q", lines)
	}
	for _, l := range lines {
		if measure(f.title, l, 0) > 576 {
			t.Errorf("%q is too wide", l)
		}
	}
}

func TestCardSizeAndWeight(t *testing.T) {
	r := renderer(t)
	var texts []Card
	for _, fx := range loadFixtures(t) {
		switch fx.code {
		case "LongTitle0000001", "EscapeHtml000001", "TrackTurkmen0001", "TrackCyrillic001":
			texts = append(texts, fx.card)
		}
	}
	// Noise is the worst case for JPEG size.
	noisy := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	for i := range noisy.Pix {
		noisy.Pix[i] = uint8(i*7919 + i/4093*104729)
	}
	// The race detector makes pixel loops crawl; one text still covers every
	// shape with and without a cover.
	if raceEnabled {
		texts = texts[:1]
	}
	for _, c := range texts {
		for _, shape := range []Shape{Square, Circle, Wide} {
			for _, src := range []image.Image{nil, noisy} {
				c.Shape = shape
				b, err := r.Card(src, c)
				if err != nil {
					t.Fatal(err)
				}
				if got := jpegSize(t, b); got != image.Pt(cardW, cardH) {
					t.Errorf("%q: size %v", c.Title, got)
				}
				if len(b) > cardMaxBytes {
					t.Errorf("%q shape %v cover %v: %d bytes", c.Title, shape, src != nil, len(b))
				}
			}
		}
	}
}

func TestCardSpeed(t *testing.T) {
	r := renderer(t)
	src := gradient(1000, 1000)
	c := Card{Label: "Aýdym", Title: "Ýüregiňde söýgi — Gözel Aşgabat we Çäge", Artist: "Žanna Gurbanowa", Listen: listen}
	r.Card(src, c)
	start := time.Now()
	n := 5
	if raceEnabled {
		n = 1
	}
	for range n {
		if _, err := r.Card(src, c); err != nil {
			t.Fatal(err)
		}
	}
	per := time.Since(start) / time.Duration(n)
	t.Logf("card render: %v", per)
	if !raceEnabled && per > 400*time.Millisecond {
		t.Errorf("card takes %v, budget 400ms", per)
	}
}

// TestWriteCards renders every fixture through the mock CDN into
// testdata/out for a look by eye. IMAGING_OUT=1 turns it on.
func TestWriteCards(t *testing.T) {
	if os.Getenv("IMAGING_OUT") != "1" {
		t.Skip("set IMAGING_OUT=1 to write cards to testdata/out")
	}
	r := renderer(t)
	mock := httptest.NewServer(mockbackend.New(os.DirFS("../../testdata/mock"), "t", nil))
	defer mock.Close()
	host := strings.TrimPrefix(mock.URL, "http://")
	fe := NewFetcher(FetchOptions{Hosts: []string{host}, AllowHTTP: true})
	out := "../../testdata/out"
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, fx := range loadFixtures(t) {
		var src image.Image
		if fx.cover != "" {
			u := strings.ReplaceAll(fx.cover, "{{mock}}", mock.URL)
			if pu, _ := url.Parse(u); pu != nil && pu.Host == host {
				src, _ = fe.Fetch(context.Background(), u)
			}
		}
		for name, fn := range map[string]func() ([]byte, error){
			"og.jpg":    func() ([]byte, error) { return r.Card(src, fx.card) },
			"cover.jpg": func() ([]byte, error) { return r.Cover(src, fx.card.Shape) },
			"bg.jpg":    func() ([]byte, error) { return r.Background(src, fx.card.Shape) },
		} {
			b, err := fn()
			if err != nil {
				t.Fatal(fx.code, err)
			}
			if err := os.WriteFile(filepath.Join(out, fx.code+"-"+name), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(out, "default-og.jpg"), r.DefaultCard(), 0o644); err != nil {
		t.Fatal(err)
	}
}
