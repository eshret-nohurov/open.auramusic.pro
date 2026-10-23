// Package backend asks the Belet Music API what a share code points to.
package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ErrNotFound covers unknown, revoked and hidden codes: the backend answers
// all three with the same 404.
var ErrNotFound = errors.New("share code not found")

// Failure is any answer that says nothing about the code: the backend is
// down, slow, refuses the token or sends garbage.
type Failure struct {
	Kind   string // auth, rate_limited, status, network, busy, bad_body, no_title
	Status int
	Err    error
}

func (f *Failure) Error() string {
	switch {
	case f.Status != 0:
		return fmt.Sprintf("backend %s: HTTP %d", f.Kind, f.Status)
	case f.Err != nil:
		return fmt.Sprintf("backend %s: %v", f.Kind, f.Err)
	}
	return "backend " + f.Kind
}

func (f *Failure) Unwrap() error { return f.Err }

type Options struct {
	BaseURL     string
	Token       string
	Version     string
	Timeout     time.Duration
	MaxInflight int
	Logger      *slog.Logger
}

type Client struct {
	endpoint  string
	token     string
	userAgent string
	http      *http.Client
	slots     chan struct{}
	slotWait  time.Duration
	log       *slog.Logger
	throttle  throttle
}

const maxBody = 64 << 10

func New(o Options) *Client {
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Client{
		endpoint:  o.BaseURL + "/api/v1/internal/deeplink",
		token:     o.Token,
		userAgent: "BeletOpen/" + o.Version,
		http: &http.Client{
			Timeout: o.Timeout,
			Transport: &http.Transport{
				Proxy:                  http.ProxyFromEnvironment,
				DialContext:            (&net.Dialer{Timeout: o.Timeout}).DialContext,
				ForceAttemptHTTP2:      true,
				MaxIdleConns:           o.MaxInflight,
				MaxIdleConnsPerHost:    o.MaxInflight,
				MaxConnsPerHost:        o.MaxInflight,
				IdleConnTimeout:        90 * time.Second,
				TLSHandshakeTimeout:    o.Timeout,
				MaxResponseHeaderBytes: 16 << 10,
			},
			// The token rides in a header; following a redirect could hand
			// it to whatever host the Location names.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("redirect refused")
			},
		},
		slots:    make(chan struct{}, o.MaxInflight),
		slotWait: time.Second,
		log:      log,
		throttle: throttle{last: map[string]time.Time{}},
	}
}

// Fetch asks once, without retries: a chat app waits 3-5 s for the whole
// preview, and a retry would eat that time.
func (c *Client) Fetch(ctx context.Context, code string) (Preview, error) {
	p, err := c.fetch(ctx, code)
	var f *Failure
	if errors.As(err, &f) {
		c.report(f)
	}
	return p, err
}

func (c *Client) fetch(ctx context.Context, code string) (Preview, error) {
	wait := time.NewTimer(c.slotWait)
	defer wait.Stop()
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-wait.C:
		return Preview{}, &Failure{Kind: "busy"}
	case <-ctx.Done():
		return Preview{}, &Failure{Kind: "network", Err: ctx.Err()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?code="+url.QueryEscape(code), nil)
	if err != nil {
		return Preview{}, &Failure{Kind: "network", Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	res, err := c.http.Do(req)
	if err != nil {
		// url.Error repeats the request URL, and the URL holds the code.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return Preview{}, &Failure{Kind: "network", Err: err}
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))

	switch {
	case res.StatusCode == http.StatusNotFound:
		return Preview{}, ErrNotFound
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return Preview{}, &Failure{Kind: "auth", Status: res.StatusCode}
	case res.StatusCode == http.StatusTooManyRequests:
		return Preview{}, &Failure{Kind: "rate_limited", Status: res.StatusCode}
	case res.StatusCode != http.StatusOK:
		return Preview{}, &Failure{Kind: "status", Status: res.StatusCode}
	case err != nil:
		return Preview{}, &Failure{Kind: "network", Err: err}
	case len(body) > maxBody:
		return Preview{}, &Failure{Kind: "bad_body", Err: errors.New("body over 64 KB")}
	}

	p, err := parsePreview(body)
	if err != nil {
		return Preview{}, &Failure{Kind: "bad_body", Err: errors.New("not a JSON object")}
	}
	if !p.hasTitle() {
		return Preview{}, &Failure{Kind: "no_title"}
	}
	if !KnownType(p.Type) && c.throttle.allow("type:"+p.Type, time.Hour) {
		c.log.Warn("backend sent an unknown type, page shown without a type label", "type", p.Type)
	}
	return p, nil
}

// report logs failures at most once a minute per kind. Codes never go into
// these lines: bots probing codes would otherwise fill the log.
func (c *Client) report(f *Failure) {
	if !c.throttle.allow(f.Kind, time.Minute) {
		return
	}
	attrs := []any{"kind", f.Kind}
	if f.Status != 0 {
		attrs = append(attrs, "status", f.Status)
	}
	if f.Err != nil {
		attrs = append(attrs, "err", f.Err.Error())
	}
	if f.Kind == "auth" {
		c.log.Error("backend rejected the site token or this server's IP is not in its allowlist", attrs...)
		return
	}
	c.log.Warn("backend request failed", attrs...)
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
	t.last[key] = now
	return true
}
