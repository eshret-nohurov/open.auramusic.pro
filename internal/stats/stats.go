// Package stats counts page opens and button clicks per hour, without
// anything that identifies a visitor, and appends one JSON line per hour to
// stats.jsonl.
package stats

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/preview"
)

const hourLayout = "2006-01-02T15"

var (
	platforms = set("ios", "android", "desktop")
	langs     = set("tm", "ru")
	itemTypes = set("track", "album", "playlist", "artist", "video", "video_playlist", "unknown")
	// Clicks can also come from the not-found and failure pages.
	clickTypes  = set("track", "album", "playlist", "artist", "video", "video_playlist", "unknown", "not_found", "failed")
	clickEvents = set("open_app", "app_store", "google_play")
)

func set(v ...string) map[string]bool {
	m := make(map[string]bool, len(v))
	for _, s := range v {
		m[s] = true
	}
	return m
}

// Options configures a Recorder.
type Options struct {
	DataDir string
	Logger  *slog.Logger
	Now     func() time.Time
	Cache   func() preview.Counters
}

type bucket struct {
	hour     string
	view     map[string]int64
	notFound map[string]int64
	failed   map[string]int64
	botView  map[string]int64
	click    map[string]int64
}

func newBucket(hour string) *bucket {
	return &bucket{
		hour:     hour,
		view:     map[string]int64{},
		notFound: map[string]int64{},
		failed:   map[string]int64{},
		botView:  map[string]int64{},
		click:    map[string]int64{},
	}
}

func (b *bucket) empty() bool {
	return len(b.view)+len(b.notFound)+len(b.failed)+len(b.botView)+len(b.click) == 0
}

type cacheLine struct {
	Hits     int64 `json:"hits"`
	Misses   int64 `json:"misses"`
	Failures int64 `json:"failures"`
}

// line is the on-disk shape of one hour.
type line struct {
	Hour     string           `json:"hour"`
	View     map[string]int64 `json:"view,omitempty"`
	NotFound map[string]int64 `json:"not_found,omitempty"`
	Failed   map[string]int64 `json:"failed,omitempty"`
	BotView  map[string]int64 `json:"bot_view,omitempty"`
	Click    map[string]int64 `json:"click,omitempty"`
	Cache    *cacheLine       `json:"cache,omitempty"`
}

// Recorder is safe for concurrent use.
type Recorder struct {
	path  string
	dir   string
	log   *slog.Logger
	now   func() time.Time
	cache func() preview.Counters

	mu       sync.Mutex
	cur      *bucket
	finished []*bucket // hours that ended but are not written yet

	fileMu sync.Mutex // keeps lines from concurrent flushes in order

	lim limiter
}

func New(o Options) *Recorder {
	r := &Recorder{
		dir:   o.DataDir,
		path:  filepath.Join(o.DataDir, "stats.jsonl"),
		log:   o.Logger,
		now:   o.Now,
		cache: o.Cache,
	}
	if r.log == nil {
		r.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if r.now == nil {
		r.now = time.Now
	}
	r.lim.now = r.now
	r.cur = newBucket(r.hourOf(r.now()))
	return r
}

func (r *Recorder) hourOf(t time.Time) string { return t.UTC().Format(hourLayout) }

// add counts under the lock and moves a finished hour aside first, so a count
// made just after the hour changes never lands in the old hour's line.
func (r *Recorder) add(f func(b *bucket)) {
	h := r.hourOf(r.now())
	r.mu.Lock()
	r.rotateLocked(h)
	f(r.cur)
	r.mu.Unlock()
}

func (r *Recorder) rotateLocked(h string) {
	if r.cur.hour == h {
		return
	}
	if !r.cur.empty() {
		r.finished = append(r.finished, r.cur)
	}
	r.cur = newBucket(h)
}

func (r *Recorder) View(itemType, platform, lang string) {
	if !itemTypes[itemType] || !platforms[platform] || !langs[lang] {
		return
	}
	r.add(func(b *bucket) { b.view[itemType+"."+platform+"."+lang]++ })
}

func (r *Recorder) NotFound(platform string) {
	if !platforms[platform] {
		return
	}
	r.add(func(b *bucket) { b.notFound[platform]++ })
}

func (r *Recorder) Failed(platform string) {
	if !platforms[platform] {
		return
	}
	r.add(func(b *bucket) { b.failed[platform]++ })
}

func (r *Recorder) BotView(bot string) {
	if !validBot(bot) {
		return
	}
	r.add(func(b *bucket) { b.botView[bot]++ })
}

func validBot(s string) bool {
	if len(s) < 1 || len(s) > 24 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}

func (r *Recorder) Click(event, itemType, platform string) {
	if !clickEvents[event] || !clickTypes[itemType] || !platforms[platform] {
		return
	}
	r.add(func(b *bucket) { b.click[event+"."+itemType+"."+platform]++ })
}

// Run writes every completed hour until ctx is done.
func (r *Recorder) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.flush(false)
		}
	}
}

// Close writes whatever is counted so far, including the running hour.
func (r *Recorder) Close() error { return r.flush(true) }

// flush writes the finished hours, and the running one when all is set. The
// cache counters are taken here, so they are attributed to the last hour
// written; they are reset either way to keep every line self-contained.
func (r *Recorder) flush(all bool) error {
	r.mu.Lock()
	r.rotateLocked(r.hourOf(r.now()))
	out := r.finished
	r.finished = nil
	if all && !r.cur.empty() {
		out = append(out, r.cur)
		r.cur = newBucket(r.cur.hour)
	}
	r.mu.Unlock()

	var c preview.Counters
	if r.cache != nil {
		c = r.cache()
	}
	if len(out) == 0 {
		if c != (preview.Counters{}) {
			r.log.Info("stats: cache counters without page activity",
				"hits", c.Hits, "misses", c.Misses, "failures", c.Failures)
		}
		return nil
	}

	var firstErr error
	for i, b := range out {
		l := line{
			Hour: b.hour, View: nonEmpty(b.view), NotFound: nonEmpty(b.notFound),
			Failed: nonEmpty(b.failed), BotView: nonEmpty(b.botView), Click: nonEmpty(b.click),
		}
		if i == len(out)-1 && c != (preview.Counters{}) {
			l.Cache = &cacheLine{Hits: c.Hits, Misses: c.Misses, Failures: c.Failures}
		}
		if err := r.write(l); err != nil {
			r.log.Error("stats: write failed", "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		r.log.Info("stats: hour written", "hour", b.hour,
			"views", sum(b.view), "not_found", sum(b.notFound), "failed", sum(b.failed),
			"bot_views", sum(b.botView), "clicks", sum(b.click),
			"cache_hits", c.Hits, "cache_misses", c.Misses, "cache_failures", c.Failures)
	}
	return firstErr
}

func nonEmpty(m map[string]int64) map[string]int64 {
	if len(m) == 0 {
		return nil
	}
	return m
}

func sum(m map[string]int64) int64 {
	var n int64
	for _, v := range m {
		n += v
	}
	return n
}

func (r *Recorder) write(l line) error {
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	r.fileMu.Lock()
	defer r.fileMu.Unlock()
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
