package imaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/webp"
)

var (
	errScheme   = errors.New("scheme not allowed")
	errHost     = errors.New("host not allowed")
	errRedirect = errors.New("redirect refused")
	errTooBig   = errors.New("body too large")
	errDims     = errors.New("image dimensions too large")
	errFormat   = errors.New("unsupported image format")
)

type FetchOptions struct {
	Hosts     []string // host or host:port, lower case
	AllowHTTP bool     // mock mode only
	Timeout   time.Duration
	MaxBytes  int64
	MaxDim    int
	Logger    *slog.Logger
}

type Fetcher struct {
	hosts     map[string]bool
	allowHTTP bool
	maxBytes  int64
	maxDim    int
	http      *http.Client
	log       *slog.Logger
	throttle  throttle
}

const maxRedirects = 3

func NewFetcher(o FetchOptions) *Fetcher {
	if o.Timeout == 0 {
		o.Timeout = 5 * time.Second
	}
	if o.MaxBytes == 0 {
		o.MaxBytes = 15 << 20
	}
	if o.MaxDim == 0 {
		o.MaxDim = 6000
	}
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	f := &Fetcher{
		hosts:     map[string]bool{},
		allowHTTP: o.AllowHTTP,
		maxBytes:  o.MaxBytes,
		maxDim:    o.MaxDim,
		log:       log,
		throttle:  throttle{last: map[string]time.Time{}},
	}
	for _, h := range o.Hosts {
		f.hosts[strings.ToLower(h)] = true
	}
	f.http = &http.Client{
		Timeout: o.Timeout,
		Transport: &http.Transport{
			Proxy:                  http.ProxyFromEnvironment,
			DialContext:            (&net.Dialer{Timeout: o.Timeout}).DialContext,
			ForceAttemptHTTP2:      true,
			MaxIdleConnsPerHost:    8,
			IdleConnTimeout:        90 * time.Second,
			TLSHandshakeTimeout:    o.Timeout,
			MaxResponseHeaderBytes: 16 << 10,
		},
		// A CDN may bounce between its own edges, but a hop to another host
		// would let whoever controls the cover URL point us anywhere.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return errRedirect
			}
			if !strings.EqualFold(req.URL.Host, via[0].URL.Host) || f.check(req.URL) != nil {
				return errRedirect
			}
			return nil
		},
	}
	return f
}

func (f *Fetcher) check(u *url.URL) error {
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && f.allowHTTP:
	default:
		return errScheme
	}
	if u.User != nil || !f.hosts[strings.ToLower(u.Host)] {
		return errHost
	}
	return nil
}

func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (image.Image, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		f.report("", 0, errors.New("bad url"))
		return nil, errors.New("bad cover url")
	}
	img, status, err := f.fetch(ctx, u)
	if err != nil {
		f.report(u.Host, status, err)
	}
	return img, err
}

func (f *Fetcher) fetch(ctx context.Context, u *url.URL) (image.Image, int, error) {
	if err := f.check(u); err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "image/webp,image/jpeg,image/png")
	res, err := f.http.Do(req)
	if err != nil {
		// url.Error repeats the URL, whose path may be anything the backend
		// sent; the log line carries the host instead.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, res.StatusCode, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if res.ContentLength > f.maxBytes {
		return nil, res.StatusCode, errTooBig
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, f.maxBytes+1))
	if err != nil {
		return nil, res.StatusCode, err
	}
	if int64(len(body)) > f.maxBytes {
		return nil, res.StatusCode, errTooBig
	}
	img, err := f.decode(body)
	return img, res.StatusCode, err
}

// decode reads the header first: a few hundred bytes of PNG can claim
// 8000×8000 and make a full decode allocate a quarter gigabyte.
func (f *Fetcher) decode(body []byte) (image.Image, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return nil, errFormat
	}
	switch format {
	case "jpeg", "png", "webp":
	default:
		return nil, errFormat
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > f.maxDim || cfg.Height > f.maxDim {
		return nil, errDims
	}
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", format, err)
	}
	return img, nil
}

// report logs at most once a minute per host: a dead CDN would otherwise
// write a line for every page view.
func (f *Fetcher) report(host string, status int, err error) {
	if !f.throttle.allow(strings.ToLower(host), time.Minute) {
		return
	}
	attrs := []any{"host", host}
	if status != 0 {
		attrs = append(attrs, "status", status)
	}
	attrs = append(attrs, "err", err.Error())
	f.log.Warn("cover fetch failed, drawing the placeholder", attrs...)
}

type throttle struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func (t *throttle) allow(key string, every time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if last, ok := t.last[key]; ok && now.Sub(last) < every {
		return false
	}
	// Hosts come from backend data; keep a flood of odd ones from growing
	// the map without bound.
	if len(t.last) > 1000 {
		clear(t.last)
	}
	t.last[key] = now
	return true
}
