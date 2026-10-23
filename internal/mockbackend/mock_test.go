package mockbackend

import (
	"bytes"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/sharecode"
)

const dir = "../../testdata/mock"

func TestFixtureNamesAreValidCodes(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, f := range files {
		code := strings.TrimSuffix(filepath.Base(f), ".json")
		if !sharecode.Valid(code) {
			t.Errorf("fixture %s is not a valid share code", code)
		}
		b, _ := os.ReadFile(f)
		if !json.Valid(b) {
			t.Errorf("fixture %s is not valid JSON", code)
		}
	}
}

func get(t *testing.T, ts *httptest.Server, path, auth string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, b
}

func TestContract(t *testing.T) {
	ts := httptest.NewServer(New(os.DirFS(dir), "tok", nil))
	defer ts.Close()

	if res, _ := get(t, ts, "/api/v1/internal/deeplink?code=TrackCyrillic001", ""); res.StatusCode != 401 {
		t.Errorf("no token: %d", res.StatusCode)
	}
	if res, _ := get(t, ts, "/api/v1/internal/deeplink?type=track&id=1", "Bearer tok"); res.StatusCode != 400 {
		t.Errorf("type/id form: %d", res.StatusCode)
	}
	res, b := get(t, ts, "/api/v1/internal/deeplink?code=TrackCyrillic001", "Bearer tok")
	if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v", res.StatusCode, res.Header)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	if m["artwork_url"] != ts.URL+"/covers/egoist.jpg" {
		t.Errorf("{{mock}} not expanded: %v", m["artwork_url"])
	}
	if res, _ := get(t, ts, "/api/v1/internal/deeplink?code=ServerError00500", "Bearer tok"); res.StatusCode != 500 {
		t.Errorf("_status ignored: %d", res.StatusCode)
	}
	_, b = get(t, ts, "/api/v1/internal/deeplink?code=ServerError00500", "Bearer tok")
	if bytes.Contains(b, []byte("_status")) {
		t.Error("control keys leaked")
	}
}

func TestCovers(t *testing.T) {
	ts := httptest.NewServer(New(os.DirFS(dir), "tok", nil))
	defer ts.Close()
	for name, want := range map[string][2]int{
		"egoist.jpg":       {1000, 1000},
		"video-clip.jpg":   {1280, 720},
		"small.jpg":        {300, 300},
		"album-thumb.jpg":  {200, 200},
		"huge.png":         {8000, 8000},
		"playlist-tm.jpg":  {1000, 1000},
		"video-playlist-r": {0, 0},
	} {
		res, b := get(t, ts, "/covers/"+name, "")
		if want[0] == 0 {
			if res.StatusCode != 404 {
				t.Errorf("%s: %d", name, res.StatusCode)
			}
			continue
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
		if err != nil || cfg.Width != want[0] || cfg.Height != want[1] {
			t.Errorf("%s: %dx%d %v", name, cfg.Width, cfg.Height, err)
		}
	}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := noFollow.Get(ts.URL + "/covers/redirect.jpg")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 302 || !strings.HasPrefix(res.Header.Get("Location"), "https://example.com/") {
		t.Errorf("redirect.jpg: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	_, b := get(t, ts, "/covers/broken.jpg", "")
	if _, _, err := image.DecodeConfig(bytes.NewReader(b)); err == nil {
		t.Error("broken.jpg decodes")
	}
}
