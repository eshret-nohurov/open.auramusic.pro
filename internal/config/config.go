// Package config reads the site settings from the environment and an
// optional .env file.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Mode string

const (
	ModeProd Mode = "prod"
	ModeMock Mode = "mock"
)

// MockAddr is where `bm-open mock` listens and what MODE=mock talks to by
// default.
const (
	MockAddr  = "127.0.0.1:3399"
	MockToken = "mock-token"
)

type Config struct {
	Mode               Mode
	ListenAddr         string
	PublicOrigin       string // scheme://host, no trailing slash
	MainSiteURL        string // no trailing slash
	BackendBaseURL     string // no trailing slash
	SiteToken          string
	ImageHosts         []string // host or host:port, lower case
	AllowHTTPImages    bool     // only in mock mode, for the local fixture server
	PreviewTTL         time.Duration
	NotFoundTTL        time.Duration
	FailTTL            time.Duration
	StaleMax           time.Duration
	BackendTimeout     time.Duration
	BackendMaxInflight int
	ImageCacheBytes    int64
	DataDir            string
	TrustProxy         bool
	LogLevel           slog.Level
}

// Load builds the config from the process environment. Keys missing there
// are taken from envFile when it exists, so the environment always wins.
func Load(envFile string) (Config, error) {
	file, err := ReadDotEnv(envFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	return Parse(func(key string) (string, bool) {
		if v, ok := os.LookupEnv(key); ok {
			return v, true
		}
		v, ok := file[key]
		return v, ok
	})
}

// Parse validates the settings returned by lookup and fills in defaults.
func Parse(lookup func(string) (string, bool)) (Config, error) {
	p := parser{lookup: lookup}
	c := Config{
		Mode:               Mode(strings.ToLower(p.str("MODE", string(ModeProd)))),
		ListenAddr:         p.str("LISTEN_ADDR", "127.0.0.1:3310"),
		PublicOrigin:       strings.TrimRight(p.str("PUBLIC_ORIGIN", "https://open.auramusic.pro"), "/"),
		MainSiteURL:        strings.TrimRight(p.str("MAIN_SITE_URL", "https://auramusic.pro"), "/"),
		BackendBaseURL:     strings.TrimRight(p.str("BACKEND_BASE_URL", ""), "/"),
		SiteToken:          p.str("SITE_TOKEN", ""),
		ImageHosts:         p.list("IMAGE_HOSTS", "cache.ma.st.com.tm"),
		PreviewTTL:         p.duration("PREVIEW_TTL", time.Hour),
		NotFoundTTL:        p.duration("NOT_FOUND_TTL", 10*time.Minute),
		FailTTL:            p.duration("FAIL_TTL", 15*time.Second),
		StaleMax:           p.duration("STALE_MAX", 24*time.Hour),
		BackendTimeout:     p.duration("BACKEND_TIMEOUT", 2500*time.Millisecond),
		BackendMaxInflight: p.int("BACKEND_MAX_INFLIGHT", 16),
		ImageCacheBytes:    int64(p.int("IMAGE_CACHE_MB", 128)) << 20,
		DataDir:            p.str("DATA_DIR", "./data"),
		TrustProxy:         p.bool("TRUST_PROXY", true),
		LogLevel:           p.level("LOG_LEVEL", slog.LevelInfo),
	}

	switch c.Mode {
	case ModeMock:
		if c.BackendBaseURL == "" {
			c.BackendBaseURL = "http://" + MockAddr
		}
		if c.SiteToken == "" {
			c.SiteToken = MockToken
		}
		c.ImageHosts = append(c.ImageHosts, MockAddr)
		c.AllowHTTPImages = true
	case ModeProd:
		if c.BackendBaseURL == "" {
			p.fail("BACKEND_BASE_URL is required in MODE=prod")
		}
		if c.SiteToken == "" {
			p.fail("SITE_TOKEN is required in MODE=prod")
		}
		for _, h := range c.ImageHosts {
			if isLocalHost(h) {
				p.fail(fmt.Sprintf("IMAGE_HOSTS: local address %q is not allowed in MODE=prod", h))
			}
		}
	default:
		p.fail(fmt.Sprintf("MODE must be prod or mock, got %q", c.Mode))
	}

	p.checkOrigin("PUBLIC_ORIGIN", c.PublicOrigin)
	p.checkOrigin("MAIN_SITE_URL", c.MainSiteURL)
	if c.BackendBaseURL != "" {
		p.checkURL("BACKEND_BASE_URL", c.BackendBaseURL)
	}
	if len(c.ImageHosts) == 0 {
		p.fail("IMAGE_HOSTS must list at least one host")
	}
	if c.BackendMaxInflight < 1 {
		p.fail("BACKEND_MAX_INFLIGHT must be at least 1")
	}
	if c.ImageCacheBytes < 1<<20 {
		p.fail("IMAGE_CACHE_MB must be at least 1")
	}
	if len(p.errs) > 0 {
		return Config{}, errors.Join(p.errs...)
	}
	return c, nil
}

type parser struct {
	lookup func(string) (string, bool)
	errs   []error
}

func (p *parser) fail(msg string) { p.errs = append(p.errs, errors.New(msg)) }

func (p *parser) str(key, def string) string {
	if v, ok := p.lookup(key); ok {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return def
}

func (p *parser) list(key, def string) []string {
	var out []string
	for _, h := range strings.Split(p.str(key, def), ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			out = append(out, h)
		}
	}
	return out
}

func (p *parser) duration(key string, def time.Duration) time.Duration {
	s := p.str(key, "")
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		p.fail(fmt.Sprintf("%s: want a positive duration like 1h or 2500ms, got %q", key, s))
		return def
	}
	return d
}

func (p *parser) int(key string, def int) int {
	s := p.str(key, "")
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		p.fail(fmt.Sprintf("%s: want an integer, got %q", key, s))
		return def
	}
	return n
}

func (p *parser) bool(key string, def bool) bool {
	s := p.str(key, "")
	if s == "" {
		return def
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		p.fail(fmt.Sprintf("%s: want true or false, got %q", key, s))
		return def
	}
	return b
}

func (p *parser) level(key string, def slog.Level) slog.Level {
	s := p.str(key, "")
	if s == "" {
		return def
	}
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		p.fail(fmt.Sprintf("%s: want debug, info, warn or error, got %q", key, s))
		return def
	}
	return l
}

func (p *parser) checkURL(key, raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		p.fail(fmt.Sprintf("%s: want an absolute http(s) URL, got %q", key, raw))
		return nil
	}
	return u
}

// checkOrigin also rejects paths and queries: links built from these values
// append their own path.
func (p *parser) checkOrigin(key, raw string) {
	u := p.checkURL(key, raw)
	if u != nil && (u.Path != "" || u.RawQuery != "" || u.Fragment != "") {
		p.fail(fmt.Sprintf("%s: want scheme://host only, got %q", key, raw))
	}
}

func isLocalHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast())
}

// ReadDotEnv parses KEY=VALUE lines. It knows comments, blank lines, an
// optional `export ` prefix and quoted values; there is no interpolation.
func ReadDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("%s:%d: want KEY=VALUE", path, n)
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		out[key] = val
	}
	return out, sc.Err()
}
