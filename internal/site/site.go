// Package site is the HTTP face of open.auramusic.pro: share pages, preview
// images, app association files and the small things around them.
package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"

	"github.com/eshret-nohurov/open.auramusic.pro/assets"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/config"
)

type Options struct {
	Config  config.Config
	Version string
	Logger  *slog.Logger
}

type Server struct {
	cfg       config.Config
	version   string
	log       *slog.Logger
	static    *staticSet
	wellknown map[string][]byte
	tmpl      *template.Template
	mux       *http.ServeMux
}

func New(o Options) (*Server, error) {
	s := &Server{cfg: o.Config, version: o.Version, log: o.Logger}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}

	staticFS, err := fs.Sub(assets.FS, "static")
	if err != nil {
		return nil, err
	}
	if s.static, err = loadStatic(staticFS); err != nil {
		return nil, err
	}

	s.wellknown = map[string][]byte{}
	for _, name := range []string{"apple-app-site-association.json", "assetlinks.json"} {
		b, err := fs.ReadFile(assets.FS, "wellknown/"+name)
		if err != nil {
			return nil, err
		}
		if !json.Valid(b) {
			return nil, fmt.Errorf("wellknown/%s is not valid JSON", name)
		}
		s.wellknown[name] = b
	}

	s.tmpl, err = template.New("").Funcs(template.FuncMap{"static": s.static.URL}).
		ParseFS(assets.FS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	s.routes()
	return s, nil
}

func (s *Server) routes() {
	m := http.NewServeMux()

	m.HandleFunc("GET /.well-known/apple-app-site-association", s.serveWellKnown("apple-app-site-association.json"))
	m.HandleFunc("GET /apple-app-site-association", s.serveWellKnown("apple-app-site-association.json"))
	m.HandleFunc("GET /.well-known/assetlinks.json", s.serveWellKnown("assetlinks.json"))

	m.HandleFunc("GET /robots.txt", serveRobots)
	m.HandleFunc("GET /favicon.ico", s.static.serveFixed("favicon.ico"))
	m.HandleFunc("GET /apple-touch-icon.png", s.static.serveFixed("brand/icon.png"))
	m.HandleFunc("GET /static/", s.static.serveHashed)
	m.HandleFunc("GET /healthz", s.serveHealth)

	m.HandleFunc("/", s.serveFallback)
	s.mux = m
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setCommonHeaders(w.Header())
	// ServeMux would answer a path like /s/code// with a redirect to its
	// clean form. The apps treat such links as broken, so the site does too.
	if !isClean(r.URL.Path) {
		s.serveFallback(w, r)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func isClean(p string) bool {
	if p == "/" {
		return true
	}
	c := path.Clean(p)
	return c == p || c+"/" == p
}

func setCommonHeaders(h http.Header) {
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), browsing-topics=()")
}

const contentSecurityPolicy = "default-src 'none'; img-src 'self'; style-src 'self'; font-src 'self'; " +
	"script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// writeHTML renders into memory first so a template error becomes a clean
// 500 and the response always carries Content-Length.
func (s *Server) writeHTML(w http.ResponseWriter, status int, cacheControl, name string, data any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		s.log.Error("render page", "template", name, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	h.Set("Cache-Control", cacheControl)
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

// serveWellKnown answers Apple and Google directly: both reject association
// files behind a redirect or authorization.
func (s *Server) serveWellKnown(name string) http.HandlerFunc {
	body := s.wellknown[name]
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "application/json")
		h.Set("Content-Length", strconv.Itoa(len(body)))
		h.Set("Cache-Control", "public, max-age=3600")
		w.Write(body)
	}
}

// robots.txt allows everything on purpose. Twitterbot and some other preview
// bots honour robots.txt: with Disallow they never read the og: tags and the
// chat shows a bare link. Search engines are kept out by noindex on every
// page instead, which they can only see if they are allowed to fetch it.
const robotsTxt = "User-agent: *\nAllow: /\n"

func serveRobots(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(len(robotsTxt)))
	h.Set("Cache-Control", "public, max-age=86400")
	w.Write([]byte(robotsTxt))
}

func (s *Server) serveHealth(w http.ResponseWriter, r *http.Request) {
	body := "ok " + s.version + "\n"
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	h.Set("Cache-Control", "no-store")
	w.Write([]byte(body))
}

func (s *Server) serveFallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.serveNotFound(w, r)
}

func (s *Server) serveNotFound(w http.ResponseWriter, r *http.Request) {
	s.writeHTML(w, http.StatusNotFound, "private, no-cache", "notfound.html", struct{ Lang string }{"tk"})
}

// Close finishes background work after the HTTP server has stopped.
func (s *Server) Close() {}
