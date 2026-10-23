package site

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/config"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg, err := config.Parse(func(k string) (string, bool) {
		if k == "MODE" {
			return "mock", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{Config: cfg, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func do(t *testing.T, h http.Handler, method, target string, hdr ...string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func body(t *testing.T, res *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAppleAssociation(t *testing.T) {
	s := newTestServer(t)
	for _, p := range []string{"/.well-known/apple-app-site-association", "/apple-app-site-association"} {
		res := do(t, s, "GET", p)
		if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("%s: %d %s", p, res.StatusCode, res.Header.Get("Content-Type"))
		}
		if res.Header.Get("Location") != "" || res.Header.Get("Cache-Control") != "public, max-age=3600" {
			t.Errorf("%s: headers %v", p, res.Header)
		}
		var aasa struct {
			Applinks struct {
				Details []struct {
					AppIDs     []string            `json:"appIDs"`
					Components []map[string]string `json:"components"`
				} `json:"details"`
			} `json:"applinks"`
		}
		if err := json.Unmarshal([]byte(body(t, res)), &aasa); err != nil {
			t.Fatal(err)
		}
		d := aasa.Applinks.Details
		if len(d) != 1 {
			t.Fatalf("details = %+v", d)
		}
		wantIDs := []string{"GSPTVX7P4B.tm.com.sanlytilsimat.musicapp", "GSPTVX7P4B.tm.com.sanlytilsimat.musicapp.dev"}
		if !slices.Equal(d[0].AppIDs, wantIDs) {
			t.Errorf("appIDs = %v", d[0].AppIDs)
		}
		if len(d[0].Components) != 1 || d[0].Components[0]["/"] != "/s/*" {
			t.Errorf("components = %v", d[0].Components)
		}
	}
}

func TestAssetLinks(t *testing.T) {
	s := newTestServer(t)
	res := do(t, s, "GET", "/.well-known/assetlinks.json")
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("%d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	var links []struct {
		Relation []string `json:"relation"`
		Target   struct {
			Namespace    string   `json:"namespace"`
			Package      string   `json:"package_name"`
			Fingerprints []string `json:"sha256_cert_fingerprints"`
		} `json:"target"`
	}
	if err := json.Unmarshal([]byte(body(t, res)), &links); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"com.sanlytilsimat.musicapp": {
			"40:3B:18:EF:BD:56:C2:07:0B:A6:87:16:7D:B5:C4:60:41:92:1A:62:CB:45:F9:20:0B:B4:84:B1:62:B0:6A:CE",
			"2E:AF:0E:6E:AB:F4:63:65:33:E1:66:D0:B7:E4:E4:8E:A6:55:FD:2B:C7:F6:53:37:3A:C9:D2:D1:1C:F7:FC:BC",
		},
		"com.sanlytilsimat.musicapp.dev.debug": {
			"30:CA:6F:13:C0:67:D0:DF:83:43:73:E6:D4:D0:58:68:5B:C7:49:93:F8:10:4E:C7:F8:40:02:88:9D:BA:02:46",
		},
	}
	if len(links) != len(want) {
		t.Fatalf("got %d statements", len(links))
	}
	for _, l := range links {
		if !slices.Equal(l.Relation, []string{"delegate_permission/common.handle_all_urls"}) || l.Target.Namespace != "android_app" {
			t.Errorf("statement %+v", l)
		}
		if !slices.Equal(l.Target.Fingerprints, want[l.Target.Package]) {
			t.Errorf("%s fingerprints = %v", l.Target.Package, l.Target.Fingerprints)
		}
	}
}

// headCheck goes through a real server: ResponseRecorder keeps HEAD bodies.
func headCheck(t *testing.T, h http.Handler, paths ...string) {
	t.Helper()
	ts := httptest.NewServer(h)
	defer ts.Close()
	for _, p := range paths {
		get, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		getBody := body(t, get)
		get.Body.Close()
		head, err := http.Head(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		head.Body.Close()
		if head.StatusCode != get.StatusCode || head.Header.Get("Content-Type") != get.Header.Get("Content-Type") {
			t.Errorf("HEAD %s: %d %s, GET: %d %s", p, head.StatusCode, head.Header.Get("Content-Type"), get.StatusCode, get.Header.Get("Content-Type"))
		}
		if head.Header.Get("Content-Length") != strconv.Itoa(len(getBody)) {
			t.Errorf("HEAD %s: Content-Length %q, body is %d", p, head.Header.Get("Content-Length"), len(getBody))
		}
		if head.ContentLength != int64(len(getBody)) {
			t.Errorf("HEAD %s: length %d", p, head.ContentLength)
		}
	}
}

func TestHeadHasHeadersWithoutBody(t *testing.T) {
	headCheck(t, newTestServer(t), "/.well-known/apple-app-site-association", "/apple-app-site-association",
		"/.well-known/assetlinks.json", "/robots.txt", "/healthz", "/nope")
}

func TestSecurityHeadersEverywhere(t *testing.T) {
	s := newTestServer(t)
	for _, p := range []string{"/healthz", "/robots.txt", "/.well-known/assetlinks.json", "/nope", "/favicon.ico"} {
		res := do(t, s, "GET", p)
		for k, v := range map[string]string{
			"X-Content-Type-Options":     "nosniff",
			"Referrer-Policy":            "strict-origin-when-cross-origin",
			"X-Frame-Options":            "DENY",
			"Cross-Origin-Opener-Policy": "same-origin",
		} {
			if got := res.Header.Get(k); got != v {
				t.Errorf("%s: %s = %q", p, k, got)
			}
		}
		if !strings.Contains(res.Header.Get("Permissions-Policy"), "camera=()") {
			t.Errorf("%s: Permissions-Policy missing", p)
		}
	}
}

func TestNotFoundPage(t *testing.T) {
	s := newTestServer(t)
	for _, p := range []string{"/nope", "/S/Xk3pQ9aB7mLzT2wR", "/s/Xk3pQ9aB7mLzT2wR//", "/a/../b", "/static/app.css"} {
		res := do(t, s, "GET", p)
		if res.StatusCode != 404 {
			t.Errorf("%s: status %d", p, res.StatusCode)
		}
		if loc := res.Header.Get("Location"); loc != "" {
			t.Errorf("%s: redirected to %s", p, loc)
		}
	}
	res := do(t, s, "GET", "/nope")
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") ||
		res.Header.Get("X-Robots-Tag") != "noindex, nofollow" ||
		!strings.Contains(res.Header.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Errorf("404 headers: %v", res.Header)
	}
}

func TestUnknownMethod(t *testing.T) {
	s := newTestServer(t)
	for _, p := range []string{"/", "/nope", "/healthz", "/.well-known/assetlinks.json"} {
		if res := do(t, s, "DELETE", p); res.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("DELETE %s: %d", p, res.StatusCode)
		}
	}
}

func TestRobotsAllowsCrawling(t *testing.T) {
	s := newTestServer(t)
	b := body(t, do(t, s, "GET", "/robots.txt"))
	if !strings.Contains(b, "Allow: /") || strings.Contains(b, "Disallow") {
		t.Errorf("robots.txt = %q", b)
	}
}

func TestHealth(t *testing.T) {
	s := newTestServer(t)
	if b := body(t, do(t, s, "GET", "/healthz")); b != "ok test\n" {
		t.Errorf("healthz = %q", b)
	}
}

func TestStaticHashedAndImmutable(t *testing.T) {
	s := newTestServer(t)
	u, err := s.static.URL("fonts/jost-semibold.woff")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "/static/fonts/jost-semibold.") || !strings.HasSuffix(u, ".woff") {
		t.Fatalf("url = %s", u)
	}
	res := do(t, s, "GET", u)
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "font/woff" ||
		res.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("%s: %d %v", u, res.StatusCode, res.Header)
	}
	if res := do(t, s, "GET", "/apple-touch-icon.png"); res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" {
		t.Errorf("apple-touch-icon: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
}
