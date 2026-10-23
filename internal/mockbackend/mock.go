// Package mockbackend stands in for the Belet Music API during development
// and in tests. It speaks the same contract as /api/v1/internal/deeplink and
// serves covers for its fixtures, including the broken and hostile ones.
//
// A fixture is testdata/mock/{code}.json: the body to return, plus optional
// control keys that are stripped before sending:
//
//	"_status": 500    answer with this status instead of 200
//	"_delay":  "4s"   wait before answering
//
// The string "{{mock}}" in any value becomes this server's own address, so
// cover URLs work on whatever port the mock runs.
package mockbackend

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/sharecode"
)

type Server struct {
	fixtures fs.FS
	token    string
	log      *slog.Logger
	mux      *http.ServeMux

	// DeeplinkCalls counts deeplink requests, for tests that check how many
	// reached the "backend".
	DeeplinkCalls atomic.Int64

	coversMu sync.Mutex
	covers   map[string][]byte
}

func New(fixtures fs.FS, token string, log *slog.Logger) *Server {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	s := &Server{fixtures: fixtures, token: token, log: log, covers: map[string][]byte{}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/v1/internal/deeplink", s.deeplink)
	s.mux.HandleFunc("GET /covers/{name}", s.cover)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) deeplink(w http.ResponseWriter, r *http.Request) {
	s.DeeplinkCalls.Add(1)
	code := r.URL.Query().Get("code")
	status := s.answer(w, r, code)
	s.log.Info("deeplink", "code", code, "status", status)
}

func (s *Server) answer(w http.ResponseWriter, r *http.Request, code string) int {
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("Authorization") != "Bearer "+s.token {
		return writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}
	// The site must only ever ask by code; the old type/id form is banned.
	if q := r.URL.Query(); q.Has("type") || q.Has("id") || len(q) != 1 {
		return writeJSON(w, http.StatusBadRequest, map[string]any{"error": "only ?code= is allowed"})
	}
	if !sharecode.Valid(code) {
		return writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}

	fixture, err := s.load(code)
	if errors.Is(err, fs.ErrNotExist) {
		return writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
	if err != nil {
		return writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	if d, ok := fixture["_delay"].(string); ok {
		wait, _ := time.ParseDuration(d)
		select {
		case <-time.After(wait):
		case <-r.Context().Done():
			return 0
		}
	}
	status := http.StatusOK
	if n, ok := fixture["_status"].(float64); ok {
		status = int(n)
	}
	for k := range fixture {
		if strings.HasPrefix(k, "_") {
			delete(fixture, k)
		}
	}
	if status != http.StatusOK {
		return writeJSON(w, status, map[string]any{"error": http.StatusText(status)})
	}
	expand(fixture, "http://"+r.Host)
	return writeJSON(w, status, fixture)
}

func (s *Server) load(code string) (map[string]any, error) {
	b, err := fs.ReadFile(s.fixtures, code+".json")
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("fixture %s: %w", code, err)
	}
	return m, nil
}

func expand(m map[string]any, origin string) {
	for k, v := range m {
		if s, ok := v.(string); ok {
			m[k] = strings.ReplaceAll(s, "{{mock}}", origin)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) int {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
	return status
}
