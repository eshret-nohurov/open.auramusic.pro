package site

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
)

// staticSet serves files under /static/ with a content hash in the name,
// so they can be cached forever and a release never mixes old and new.
type staticSet struct {
	byURL  map[string]*asset // "/static/app.1a2b3c4d.css"
	byName map[string]*asset // "app.css"
}

type asset struct {
	url   string
	body  []byte
	ctype string
}

var contentTypes = map[string]string{
	".css":  "text/css; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".ico":  "image/x-icon",
	".woff": "font/woff",
}

func loadStatic(fsys fs.FS) (*staticSet, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if _, ok := contentTypes[path.Ext(name)]; !ok {
			return fmt.Errorf("static/%s: unknown content type", name)
		}
		b, err := fs.ReadFile(fsys, name)
		files[name] = b
		return err
	})
	if err != nil {
		return nil, err
	}

	s := &staticSet{byURL: map[string]*asset{}, byName: map[string]*asset{}}
	add := func(name string, body []byte) {
		ext := path.Ext(name)
		sum := sha256.Sum256(body)
		a := &asset{
			url:   "/static/" + strings.TrimSuffix(name, ext) + "." + hex.EncodeToString(sum[:4]) + ext,
			body:  body,
			ctype: contentTypes[ext],
		}
		s.byURL[a.url] = a
		s.byName[name] = a
	}

	var sheets []string
	for name, body := range files {
		if path.Ext(name) == ".css" {
			sheets = append(sheets, name)
			continue
		}
		add(name, body)
	}
	// Stylesheets point at fonts and images by their plain /static/ path;
	// swap in the hashed one so a font change also changes the CSS hash.
	names := make([]string, 0, len(s.byName))
	for name := range s.byName {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, sheet := range sheets {
		css := string(files[sheet])
		for _, name := range names {
			css = strings.ReplaceAll(css, `"/static/`+name+`"`, `"`+s.byName[name].url+`"`)
		}
		add(sheet, []byte(css))
	}
	return s, nil
}

// URL returns the hashed address of a static file. Templates call it, so an
// unknown name fails the render instead of shipping a broken link.
func (s *staticSet) URL(name string) (string, error) {
	a, ok := s.byName[name]
	if !ok {
		return "", fmt.Errorf("unknown static file %q", name)
	}
	return a.url, nil
}

func (s *staticSet) serveHashed(w http.ResponseWriter, r *http.Request) {
	a, ok := s.byURL[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeAsset(w, a, "public, max-age=31536000, immutable")
}

// serveFixed answers well-known icon paths that browsers ask for by name.
func (s *staticSet) serveFixed(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeAsset(w, s.byName[name], "public, max-age=86400")
	}
}

func writeAsset(w http.ResponseWriter, a *asset, cacheControl string) {
	h := w.Header()
	h.Set("Content-Type", a.ctype)
	h.Set("Content-Length", strconv.Itoa(len(a.body)))
	h.Set("Cache-Control", cacheControl)
	w.Write(a.body)
}
