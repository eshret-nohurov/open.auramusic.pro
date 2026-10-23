package site

import (
	"context"
	"image"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/backend"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/imaging"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/lang"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/preview"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/sharecode"
)

type imageKind string

const (
	kindCover imageKind = "cover"
	kindBg    imageKind = "bg"
	kindOG    imageKind = "og"
)

var imageFiles = map[string]imageKind{"cover.jpg": kindCover, "bg.jpg": kindBg, "og.jpg": kindOG}

// imageVersion is the v= of cover and background URLs: it changes when the
// cover changes, so browsers and nginx can keep each version for a day.
func imageVersion(src string) string { return imaging.Version(src) }

// ogVersion also covers the card's text: chat apps cache og:image by URL,
// and a renamed track must not keep its old card.
func ogVersion(p backend.Preview) string {
	c := ogCard(p)
	return imaging.Version(p.ArtworkFor(lang.TM), c.Title, c.Artist, c.Label)
}

// ogCard is always Turkmen: chat apps do not tell the site who will read
// the preview.
func ogCard(p backend.Preview) imaging.Card {
	c := imaging.Card{
		Shape:  imaging.ShapeFor(p.Type),
		Label:  dictionaries[lang.TM].TypeLabel(p.Type),
		Title:  p.TitleFor(lang.TM),
		Listen: cardListen,
	}
	if p.Type != backend.TypeArtist {
		c.Artist = p.ArtistName
	}
	return c
}

// serveImage takes nothing from the request but the code and v: the cover
// address always comes from the backend's answer for that code, so the
// site never fetches a URL a visitor chose.
func (s *Server) serveImage(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	kind, ok := imageFiles[r.PathValue("file")]
	w.Header().Set("X-Robots-Tag", "noindex")
	if !ok || !sharecode.Valid(code) {
		http.NotFound(w, r)
		return
	}

	res := s.previews.Get(code)
	switch res.Outcome {
	case preview.NotFound:
		http.NotFound(w, r)
		return
	case preview.Failed:
		writeImage(w, s.fallback[kind], "public, max-age=60")
		return
	}

	p := res.Preview
	want := r.URL.Query().Get("v")
	var src, ver string
	var card imaging.Card
	if kind == kindOG {
		src, ver, card = p.ArtworkFor(lang.TM), ogVersion(p), ogCard(p)
	} else {
		src, ver = pickSource(p, want)
		card.Shape = imaging.ShapeFor(p.Type)
	}

	body, complete := s.renderImage(kind, ver, src, card)
	cache := "public, max-age=86400"
	switch {
	case !complete:
		// The cover failed to download: a placeholder now, a retry soon.
		cache = "public, max-age=60"
	case want != ver:
		// An old link: send the current picture but do not pin it under the
		// old address for long.
		cache = "public, max-age=300"
	}
	writeImage(w, body, cache)
}

// pickSource finds which language's cover the page asked for. The page
// puts the version of its own cover in v; anything else gets Turkmen.
func pickSource(p backend.Preview, v string) (src, ver string) {
	for _, l := range []lang.Lang{lang.RU, lang.TM} {
		if src := p.ArtworkFor(l); imageVersion(src) == v {
			return src, v
		}
	}
	src = p.ArtworkFor(lang.TM)
	return src, imageVersion(src)
}

type rendered struct {
	body     []byte
	complete bool
}

// renderImage builds one picture per key at a time and keeps finished ones
// in memory. A placeholder drawn because the download failed is not kept.
func (s *Server) renderImage(kind imageKind, ver, src string, card imaging.Card) ([]byte, bool) {
	key := string(kind) + "|" + strconv.Itoa(int(card.Shape)) + "|" + ver
	if b, ok := s.images.Get(key); ok {
		return b, true
	}
	v, _, _ := s.renders.Do(key, func() (any, error) {
		if b, ok := s.images.Peek(key); ok {
			return rendered{b, true}, nil
		}
		var img image.Image
		var fetchErr error
		if src != "" {
			img, fetchErr = s.fetchCover(src)
		}
		var body []byte
		var err error
		switch kind {
		case kindCover:
			body, err = s.render.Cover(img, card.Shape)
		case kindBg:
			body, err = s.render.Background(img, card.Shape)
		default:
			body, err = s.render.Card(img, card)
		}
		if err != nil {
			s.log.Error("render image", "kind", kind, "err", err)
			return rendered{s.fallback[kind], false}, nil
		}
		complete := fetchErr == nil
		if complete {
			s.images.Put(key, body)
		}
		return rendered{body, complete}, nil
	})
	out := v.(rendered)
	return out.body, out.complete
}

// fetchCover shares one download between the cover, background and card of
// the same item, which a page view asks for at once.
func (s *Server) fetchCover(src string) (image.Image, error) {
	v, err, _ := s.downloads.Do(src, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		return s.fetch.Fetch(ctx, src)
	})
	if err != nil {
		return nil, err
	}
	return v.(image.Image), nil
}

func writeImage(w http.ResponseWriter, body []byte, cacheControl string) {
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	h.Set("Cache-Control", cacheControl)
	w.Write(body)
}

// serveQR needs no backend: the QR holds only the share link itself.
func (s *Server) serveQR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Robots-Tag", "noindex")
	code, ok := strings.CutSuffix(r.PathValue("file"), ".svg")
	if !ok || !sharecode.Valid(code) {
		http.NotFound(w, r)
		return
	}
	svg, err := imaging.QRSVG(s.cfg.PublicOrigin + "/s/" + code)
	if err != nil {
		s.log.Error("render qr", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/svg+xml")
	h.Set("Content-Length", strconv.Itoa(len(svg)))
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(svg)
}
