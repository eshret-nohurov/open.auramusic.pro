package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestProdRequiresBackendAndToken(t *testing.T) {
	_, err := Parse(env(map[string]string{"MODE": "prod"}))
	if err == nil {
		t.Fatal("prod without backend and token must fail")
	}
	for _, want := range []string{"BACKEND_BASE_URL", "SITE_TOKEN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestProdDefaults(t *testing.T) {
	c, err := Parse(env(map[string]string{
		"BACKEND_BASE_URL": "https://api.example.tm/",
		"SITE_TOKEN":       "secret",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeProd || c.ListenAddr != "127.0.0.1:3310" {
		t.Errorf("mode/listen = %q/%q", c.Mode, c.ListenAddr)
	}
	if c.BackendBaseURL != "https://api.example.tm" {
		t.Errorf("trailing slash kept: %q", c.BackendBaseURL)
	}
	if c.PublicOrigin != "https://open.auramusic.pro" || c.MainSiteURL != "https://auramusic.pro" {
		t.Errorf("origins = %q, %q", c.PublicOrigin, c.MainSiteURL)
	}
	if c.PreviewTTL != time.Hour || c.NotFoundTTL != 10*time.Minute || c.FailTTL != 15*time.Second ||
		c.StaleMax != 24*time.Hour || c.BackendTimeout != 2500*time.Millisecond {
		t.Errorf("ttl defaults wrong: %+v", c)
	}
	if c.BackendMaxInflight != 16 || c.ImageCacheBytes != 128<<20 || !c.TrustProxy {
		t.Errorf("limits wrong: %+v", c)
	}
	if len(c.ImageHosts) != 1 || c.ImageHosts[0] != "*.ma.st.com.tm" || c.AllowHTTPImages {
		t.Errorf("image hosts = %v, http=%v", c.ImageHosts, c.AllowHTTPImages)
	}
}

func TestMockFillsBackend(t *testing.T) {
	c, err := Parse(env(map[string]string{"MODE": "mock"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.BackendBaseURL != "http://"+MockAddr || c.SiteToken != MockToken || !c.AllowHTTPImages {
		t.Errorf("mock config = %+v", c)
	}
	if c.ImageHosts[len(c.ImageHosts)-1] != MockAddr {
		t.Errorf("mock host not allowed for images: %v", c.ImageHosts)
	}
}

func TestProdRejectsLocalImageHosts(t *testing.T) {
	for _, h := range []string{"127.0.0.1:3399", "localhost", "*.localhost", "10.0.0.5", "[::1]:80", "192.168.1.1"} {
		_, err := Parse(env(map[string]string{
			"BACKEND_BASE_URL": "https://api.example.tm",
			"SITE_TOKEN":       "x",
			"IMAGE_HOSTS":      "cache.ma.st.com.tm, " + h,
		}))
		if err == nil {
			t.Errorf("IMAGE_HOSTS with %s accepted in prod", h)
		}
	}
}

func TestImageHostWildcards(t *testing.T) {
	for _, h := range []string{"*", "*.tm", "*ma.st.com.tm", "**.ma.st.com.tm", "a.*.com.tm", "*.ma.st.*"} {
		if _, err := Parse(env(map[string]string{"MODE": "mock", "IMAGE_HOSTS": h})); err == nil {
			t.Errorf("IMAGE_HOSTS=%q accepted", h)
		}
	}
	c, err := Parse(env(map[string]string{"MODE": "mock", "IMAGE_HOSTS": "*.MA.st.com.tm, cdn.example.com"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ImageHosts[0] != "*.ma.st.com.tm" || c.ImageHosts[1] != "cdn.example.com" {
		t.Errorf("image hosts = %v", c.ImageHosts)
	}
}

func TestBadValues(t *testing.T) {
	base := map[string]string{"MODE": "mock"}
	for key, val := range map[string]string{
		"MODE":                 "dev",
		"PREVIEW_TTL":          "soon",
		"FAIL_TTL":             "-1s",
		"BACKEND_MAX_INFLIGHT": "0",
		"TRUST_PROXY":          "maybe",
		"LOG_LEVEL":            "loud",
		"PUBLIC_ORIGIN":        "https://open.auramusic.pro/s",
		"MAIN_SITE_URL":        "auramusic.pro",
	} {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		m[key] = val
		if _, err := Parse(env(m)); err == nil {
			t.Errorf("%s=%q accepted", key, val)
		}
	}
}

func TestEnvironmentWinsOverFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	body := "# comment\n\nMODE=mock\nexport LISTEN_ADDR='127.0.0.1:4000'\nDATA_DIR=\"/var/lib/x\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATA_DIR", "/from/env")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeMock || c.ListenAddr != "127.0.0.1:4000" {
		t.Errorf("file values not read: %+v", c)
	}
	if c.DataDir != "/from/env" {
		t.Errorf("DATA_DIR = %q, environment must win", c.DataDir)
	}
}

func TestMissingDotEnvIsFine(t *testing.T) {
	t.Setenv("MODE", "mock")
	if _, err := Load(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatal(err)
	}
}

func TestDotEnvSyntaxError(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("JUSTAWORD\n"), 0o600)
	if _, err := ReadDotEnv(path); err == nil {
		t.Fatal("line without = accepted")
	}
}
