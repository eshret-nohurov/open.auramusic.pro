package site

import (
	"bytes"
	"encoding/json"
	"html"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

const (
	iPhoneUA  = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"
	androidUA = "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0 Mobile Safari/537.36"
	desktopUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0 Safari/537.36"
	telegram  = "TelegramBot (like TwitterBot)"
)

var metaRe = regexp.MustCompile(`<meta (?:property|name)="([^"]+)"\s+content="([^"]*)">`)

// metas maps meta names to their unescaped values.
func metas(page string) map[string]string {
	out := map[string]string{}
	for _, m := range metaRe.FindAllStringSubmatch(page, -1) {
		out[m[1]] = html.UnescapeString(m[2])
	}
	return out
}

func TestPlatform(t *testing.T) {
	for ua, want := range map[string]string{
		iPhoneUA: "ios",
		"Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148":                                      "ios",
		"Mozilla/5.0 (iPod touch; CPU iPhone OS 15_0 like Mac OS X)":                                                            "ios",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15": "desktop",
		androidUA: "android",
		"Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/25.0 Chrome/121.0 Mobile Safari/537.36": "android",
		desktopUA: "desktop",
		"":        "desktop",
	} {
		if got := platformFor(ua); got != want {
			t.Errorf("platformFor(%.40q) = %s, want %s", ua, got, want)
		}
	}
}

func TestBots(t *testing.T) {
	for ua, want := range map[string]string{
		"TelegramBot (like TwitterBot)":                                  "telegram",
		"WhatsApp/2.23.20.0 A":                                           "whatsapp",
		"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit)": "facebook",
		"Facebot":                                  "facebook",
		"meta-externalagent/1.1":                   "facebook",
		"Twitterbot/1.0":                           "twitter",
		"Slackbot-LinkExpanding 1.0":               "slack",
		"Mozilla/5.0 (compatible; Discordbot/2.0)": "discord",
		"LinkedInBot/1.0":                          "linkedin",
		"vkShare; +http://vk.com/dev/Share":        "vk",
		"Viber/21.0":                               "viber",
		"Mozilla/5.0 SkypeUriPreview Preview/0.5":  "skype",
		"Iframely/1.3.1":                           "iframely",
		"Pinterest/0.2":                            "pinterest",
		"redditbot/1.0":                            "reddit",
		"Applebot/0.1":                             "apple",
		"Googlebot/2.1":                            "google",
		"bingbot/2.0":                              "bing",
		"YandexBot/3.0":                            "yandex",
		"Embedly/0.2":                              "embedly",
		"http.rb/5.1 (Mastodon/4.2.0; +https://x.tm)": "mastodon",
		iPhoneUA:  "",
		desktopUA: "",
	} {
		if got := botFor(ua); got != want {
			t.Errorf("botFor(%q) = %q, want %q", ua, got, want)
		}
	}
	if len(previewBots) != 21 {
		t.Errorf("bot list has %d entries; the spec lists 21", len(previewBots))
	}
}

func TestFoundPage(t *testing.T) {
	s := newTestServer(t)
	res := do(t, s, "GET", "/s/TrackCyrillic001", "User-Agent", iPhoneUA)
	page := body(t, res)
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	for k, v := range map[string]string{
		"Content-Type":  "text/html; charset=utf-8",
		"Cache-Control": "private, no-cache",
		"Vary":          "User-Agent, Accept-Language, Cookie",
		"X-Robots-Tag":  "noindex, nofollow",
	} {
		if got := res.Header.Get(k); got != v {
			t.Errorf("%s = %q", k, got)
		}
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe") {
		t.Errorf("CSP = %q", csp)
	}

	m := metas(page)
	for k, v := range map[string]string{
		"og:site_name":        "Belet Music",
		"og:type":             "music.song",
		"og:title":            "Эгоист",
		"og:description":      "Aýdym · Navai · Belet Music",
		"og:url":              "https://open.auramusic.pro/s/TrackCyrillic001",
		"og:image:type":       "image/jpeg",
		"og:image:width":      "1200",
		"og:image:height":     "630",
		"og:image:alt":        "Эгоист",
		"og:locale":           "tk_TM",
		"og:locale:alternate": "ru_RU",
		"twitter:card":        "summary_large_image",
		"robots":              "noindex, nofollow",
		"apple-itunes-app":    "app-id=6503349020, app-argument=https://open.auramusic.pro/s/TrackCyrillic001",
	} {
		if m[k] != v {
			t.Errorf("%s = %q, want %q", k, m[k], v)
		}
	}
	if !regexp.MustCompile(`^https://open\.auramusic\.pro/i/TrackCyrillic001/og\.jpg\?v=[0-9a-f]{8}$`).MatchString(m["og:image"]) ||
		m["twitter:image"] != m["og:image"] {
		t.Errorf("og:image = %q, twitter:image = %q", m["og:image"], m["twitter:image"])
	}
	for _, want := range []string{
		`<html lang="tk">`,
		`<title>Эгоист · Navai | Belet Music</title>`,
		`data-platform="ios"`,
		`/i/TrackCyrillic001/cover.jpg?v=`,
		`/i/TrackCyrillic001/bg.jpg?v=`,
		`/qr/TrackCyrillic001.svg`,
		`href="intent://open.auramusic.pro/s/TrackCyrillic001#Intent;scheme=https;package=com.sanlytilsimat.musicapp;S.browser_fallback_url=https%3A%2F%2Fplay.google.com%2Fstore%2Fapps%2Fdetails%3Fid%3Dcom.sanlytilsimat.musicapp;end"`,
		`href="https://apps.apple.com/tm/app/belet-music/id6503349020"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if strings.Contains(page, "ZgotmplZ") {
		t.Error("a URL was rejected by html/template")
	}
	if len(page) > 20<<10 {
		t.Errorf("page is %d bytes, budget 20 KB", len(page))
	}
}

func TestPageTypes(t *testing.T) {
	s := newTestServer(t)
	for code, want := range map[string]struct{ ogType, title, kind, shape string }{
		"AlbumNavai000001": {"music.album", "Мне не нужен", "Albom", "art-square"},
		"PlaylistTmRu0001": {"music.playlist", "Hepdäniň hitleri", "Pleýlist", "art-square"},
		"ArtistNavai00001": {"profile", "Navai", "Artist", "art-circle"},
		"VideoClip0000001": {"video.other", "Эгоист (клип)", "Wideo", "art-wide"},
		"VideoPlaylist001": {"website", "Iň gowy klipler", "Wideo pleýlist", "art-wide"},
		"UnknownType00001": {"website", "Meniň pleýlistim", "", "art-square"},
	} {
		page := body(t, do(t, s, "GET", "/s/"+code))
		m := metas(page)
		if m["og:type"] != want.ogType || m["og:title"] != want.title {
			t.Errorf("%s: og:type %q og:title %q", code, m["og:type"], m["og:title"])
		}
		if want.kind != "" && !strings.Contains(page, `<p class="kind">`+want.kind+`</p>`) {
			t.Errorf("%s: no type label %q", code, want.kind)
		}
		if want.kind == "" && strings.Contains(page, `class="kind"`) {
			t.Errorf("%s: unknown type got a label", code)
		}
		if !strings.Contains(page, want.shape) {
			t.Errorf("%s: no %s", code, want.shape)
		}
	}
	page := body(t, do(t, s, "GET", "/s/ArtistNavai00001"))
	if strings.Contains(page, `class="artist"`) || metas(page)["og:description"] != "Artist · Belet Music" {
		t.Errorf("artist page shows a subtitle or wrong description: %q", metas(page)["og:description"])
	}
	page = body(t, do(t, s, "GET", "/s/NoImages00000001"))
	if strings.Contains(page, "cover.jpg") || strings.Contains(page, "bg.jpg") || !strings.Contains(page, "art-empty") {
		t.Error("page without artwork must use the placeholder")
	}
	if !strings.Contains(metas(page)["og:image"], "/i/NoImages00000001/og.jpg") {
		t.Error("og card is needed even without artwork")
	}
}

func TestEscaping(t *testing.T) {
	s := newTestServer(t)
	page := body(t, do(t, s, "GET", "/s/EscapeHtml000001"))
	if strings.Contains(page, "<script>alert") || strings.Contains(page, "<b>bold") {
		t.Error("backend text reached the page unescaped")
	}
	if got := metas(page)["og:title"]; got != `<script>alert(1)</script> "Quotes" & 'apostrophes'` {
		t.Errorf("og:title = %q", got)
	}
}

func TestLanguage(t *testing.T) {
	s := newTestServer(t)
	page := body(t, do(t, s, "GET", "/s/PlaylistTmRu0001", "Accept-Language", "ru-RU,ru;q=0.9"))
	if !strings.Contains(page, `<html lang="ru">`) || metas(page)["og:title"] != "Хиты недели" || metas(page)["og:locale"] != "ru_RU" {
		t.Errorf("Russian browser did not get Russian")
	}
	page = body(t, do(t, s, "GET", "/s/PlaylistTmRu0001", "Accept-Language", "tk", "Cookie", "lang=ru"))
	if !strings.Contains(page, `<html lang="ru">`) {
		t.Error("cookie must beat Accept-Language")
	}
	page = body(t, do(t, s, "GET", "/s/PlaylistTmRu0001", "User-Agent", telegram, "Accept-Language", "ru", "Cookie", "lang=ru"))
	if !strings.Contains(page, `<html lang="tk">`) || metas(page)["og:title"] != "Hepdäniň hitleri" {
		t.Error("preview bots must always get Turkmen")
	}
	if !strings.Contains(page, `aria-current="true">TM</a>`) {
		t.Error("current language not marked")
	}

	res := do(t, s, "GET", "/s/PlaylistTmRu0001/?lang=ru&fbclid=x")
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/s/PlaylistTmRu0001" {
		t.Fatalf("lang switch: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	c := res.Cookies()
	if len(c) != 1 || c[0].Name != "lang" || c[0].Value != "ru" || c[0].MaxAge != 31536000 || !c[0].HttpOnly ||
		c[0].SameSite != http.SameSiteLaxMode || c[0].Path != "/" || c[0].Secure {
		t.Errorf("cookie = %+v", c)
	}
}

func TestRootRedirect(t *testing.T) {
	s := newTestServer(t)
	for _, tc := range []struct{ al, cookie, want string }{
		{"ru", "", "https://auramusic.pro/ru"},
		{"en-US,en;q=0.9", "", "https://auramusic.pro/tm"},
		{"", "", "https://auramusic.pro/tm"},
		{"tk", "lang=ru", "https://auramusic.pro/ru"},
	} {
		res := do(t, s, "GET", "/", "Accept-Language", tc.al, "Cookie", tc.cookie)
		if res.StatusCode != http.StatusFound || res.Header.Get("Location") != tc.want {
			t.Errorf("%q/%q: %d %s", tc.al, tc.cookie, res.StatusCode, res.Header.Get("Location"))
		}
	}
}

func TestCodeAndPathRules(t *testing.T) {
	s := newTestServer(t)
	for path, want := range map[string]int{
		"/s/TrackCyrillic001":          200,
		"/s/TrackCyrillic001/":         200,
		"/s/TrackCyrillic001?fbclid=x": 200,
		"/s/TrackCyrillic001/?utm=1":   200,
		"/s/TrackCyrillic001//":        404,
		"/s/TrackCyrillic001/x":        404,
		"/s/":                          404,
		"/s":                           404,
		"/s/short":                     404,
		"/s/TrackCyrillic0011":         404,
		"/s/TrackCyrillic00-":          404,
		"/s/TrackCyrilli%2F01":         404,
		"/S/TrackCyrillic001":          404,
		"/s/trackcyrillic001":          404, // a different code: the stub has no such fixture
	} {
		if res := do(t, s, "GET", path); res.StatusCode != want {
			t.Errorf("%s: %d, want %d", path, res.StatusCode, want)
		}
	}
	before := s.mock.DeeplinkCalls.Load()
	for _, p := range []string{"/s/short", "/s/TrackCyrillic00-", "/s/abc%2Fdefghijklmnop", "/s/ЭгоистЭгоистЭго"} {
		do(t, s, "GET", p)
	}
	if n := s.mock.DeeplinkCalls.Load() - before; n != 0 {
		t.Errorf("invalid codes reached the backend %d times", n)
	}
	if res := do(t, s, "PUT", "/s/TrackCyrillic001"); res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("PUT: %d", res.StatusCode)
	}
}

func TestNotFoundAndFailedStates(t *testing.T) {
	s := newTestServer(t)
	res := do(t, s, "GET", "/s/NotFound00000404", "User-Agent", androidUA)
	page := body(t, res)
	if res.StatusCode != 404 || !strings.Contains(page, "Tapylmady") || strings.Contains(page, "intent://") {
		t.Errorf("not found: %d, open button present: %v", res.StatusCode, strings.Contains(page, "intent://"))
	}
	m := metas(page)
	if m["og:title"] != "Belet Music" || m["og:description"] != "Aýdymlar. Wideolar. Podkastlar" ||
		m["apple-itunes-app"] != "app-id=6503349020" || !strings.Contains(m["og:image"], "/static/og-default.") {
		t.Errorf("not found metas: %v", m)
	}
	if !strings.Contains(page, "<title>Tapylmady | Belet Music</title>") {
		t.Error("not found title")
	}

	for _, code := range []string{"ServerError00500", "Unauthorized0401", "NoTitle000000001"} {
		res := do(t, s, "GET", "/s/"+code, "User-Agent", androidUA)
		page := body(t, res)
		if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s: %d %s", code, res.StatusCode, res.Header.Get("Cache-Control"))
		}
		if !strings.Contains(page, "Ýükläp bolmady") || !strings.Contains(page, "intent://open.auramusic.pro/s/"+code) {
			t.Errorf("%s: failed page lacks text or open button", code)
		}
		if metas(page)["apple-itunes-app"] != "app-id=6503349020, app-argument=https://open.auramusic.pro/s/"+code {
			t.Errorf("%s: apple-itunes-app = %q", code, metas(page)["apple-itunes-app"])
		}
	}
}

func TestSlowBackendIsFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the backend timeout")
	}
	s := newTestServer(t)
	res := do(t, s, "GET", "/s/SlowBackend00004")
	if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("%d %s", res.StatusCode, res.Header.Get("Cache-Control"))
	}
}

func TestImages(t *testing.T) {
	s := newTestServer(t)
	page := body(t, do(t, s, "GET", "/s/TrackCyrillic001"))
	og := strings.TrimPrefix(metas(page)["og:image"], "https://open.auramusic.pro")

	res := do(t, s, "GET", og)
	b := body(t, res)
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/jpeg" ||
		res.Header.Get("Cache-Control") != "public, max-age=86400" || res.Header.Get("X-Robots-Tag") != "noindex" {
		t.Fatalf("og.jpg: %d %v", res.StatusCode, res.Header)
	}
	cfg, err := jpeg.DecodeConfig(strings.NewReader(b))
	if err != nil || cfg.Width != 1200 || cfg.Height != 630 || len(b) > 300<<10 {
		t.Errorf("og.jpg %dx%d, %d bytes, %v", cfg.Width, cfg.Height, len(b), err)
	}

	cover := regexp.MustCompile(`/i/TrackCyrillic001/cover\.jpg\?v=[0-9a-f]{8}`).FindString(page)
	res = do(t, s, "GET", cover)
	cfg, err = jpeg.DecodeConfig(res.Body)
	if res.StatusCode != 200 || err != nil || cfg.Width != 640 || res.Header.Get("Cache-Control") != "public, max-age=86400" {
		t.Errorf("cover: %d %dx%d %v %s", res.StatusCode, cfg.Width, cfg.Height, err, res.Header.Get("Cache-Control"))
	}
	if res := do(t, s, "GET", "/i/TrackCyrillic001/cover.jpg?v=00000000"); res.StatusCode != 200 || res.Header.Get("Cache-Control") != "public, max-age=300" {
		t.Errorf("stale v: %d %s", res.StatusCode, res.Header.Get("Cache-Control"))
	}

	for path, want := range map[string]int{
		"/i/NotFound00000404/og.jpg": 404,
		"/i/short/og.jpg":            404,
		"/i/TrackCyrillic001/x.jpg":  404,
		"/qr/short.svg":              404,
		"/qr/TrackCyrillic001.png":   404,
	} {
		if res := do(t, s, "GET", path); res.StatusCode != want {
			t.Errorf("%s: %d", path, res.StatusCode)
		}
	}
	res = do(t, s, "GET", "/i/ServerError00500/og.jpg")
	if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "public, max-age=60" {
		t.Errorf("failed og: %d %s", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	// Covers the site must refuse or cannot read: still a picture, kept briefly.
	for _, code := range []string{"BrokenCover00001", "HugeCover0000001", "ForeignHost00001", "RedirectCover001"} {
		res := do(t, s, "GET", "/i/"+code+"/og.jpg")
		b := body(t, res)
		if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "public, max-age=60" || len(b) == 0 {
			t.Errorf("%s: %d %s", code, res.StatusCode, res.Header.Get("Cache-Control"))
		}
	}

	res = do(t, s, "GET", "/qr/TrackCyrillic001.svg")
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/svg+xml" ||
		res.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("qr: %d %v", res.StatusCode, res.Header)
	}

	headCheck(t, s, og, cover, "/qr/TrackCyrillic001.svg", "/s/TrackCyrillic001")
}

func TestBeacon(t *testing.T) {
	s := newTestServer(t)
	for _, b := range []string{`{"e":"app_store","t":"track","p":"ios"}`, `garbage`} {
		req := httptest.NewRequest("POST", "/e", strings.NewReader(b))
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Errorf("%s: %d", b, rec.Code)
		}
	}
	if res := do(t, s, "GET", "/e"); res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /e: %d", res.StatusCode)
	}
}

// TestTokenNeverLeaks walks every page and picture of every fixture and
// checks responses and logs for the site token.
func TestTokenNeverLeaks(t *testing.T) {
	s := newTestServer(t)
	files, _ := filepath.Glob("../../testdata/mock/*.json")
	var paths []string
	for _, f := range files {
		code := strings.TrimSuffix(filepath.Base(f), ".json")
		if code == "SlowBackend00004" {
			continue
		}
		paths = append(paths, "/s/"+code, "/i/"+code+"/og.jpg", "/i/"+code+"/cover.jpg", "/i/"+code+"/bg.jpg", "/qr/"+code+".svg")
	}
	paths = append(paths, "/", "/nope", "/healthz", "/.well-known/assetlinks.json")
	for _, ua := range []string{iPhoneUA, androidUA, desktopUA, telegram} {
		for _, p := range paths {
			res := do(t, s, "GET", p, "User-Agent", ua)
			b := body(t, res)
			hdr, _ := json.Marshal(res.Header)
			if strings.Contains(b, testToken) || bytes.Contains(hdr, []byte(testToken)) {
				t.Fatalf("token in response to %s", p)
			}
		}
	}
	if strings.Contains(s.logs.String(), testToken) {
		t.Fatal("token in the log")
	}
}

func TestOneBackendRequestForABurst(t *testing.T) {
	s := newTestServer(t)
	var wg sync.WaitGroup
	for range 30 {
		wg.Go(func() { do(t, s, "GET", "/s/AlbumNavai000001", "User-Agent", telegram) })
	}
	wg.Wait()
	if n := s.mock.DeeplinkCalls.Load(); n != 1 {
		t.Errorf("backend got %d requests", n)
	}
}
