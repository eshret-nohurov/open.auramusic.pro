package site

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/backend"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/config"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/lang"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/preview"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/sharecode"
)

const (
	stateFound    = "found"
	stateNotFound = "notfound"
	stateFailed   = "failed"
)

type pageView struct {
	Lang      lang.Lang
	T         *texts
	State     string
	Platform  string
	Type      string // data-type for click beacons: item type, "unknown", "not_found" or "failed"
	TypeLabel string
	Title     string
	Artist    string
	Message   string
	Shape     string // square, circle, wide
	Cover     *coverImage
	Backdrop  string
	QR        string
	OpenApp   template.URL
	AppStore  string
	Play      string
	MainSite  string
	Langs     []langLink
	Meta      pageMeta
}

type coverImage struct {
	URL  string
	W, H int
}

type langLink struct {
	Label, Href, HTMLLang string
	Current               bool
}

type pageMeta struct {
	Title       string
	Description string
	OGType      string
	OGTitle     string
	URL         string
	Image       string
	ImageType   string
	ImageAlt    string
	AppleApp    string
}

// servePage is /s/{code} and /s/{code}/. Anything after the code, a bad
// code or a backend 404 gives the same "not found" page.
func (s *Server) servePage(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if !sharecode.Valid(code) {
		s.serveNotFound(w, r)
		return
	}
	if l, ok := lang.Parse(r.URL.Query().Get("lang")); ok {
		s.setLangCookie(w, l)
		http.Redirect(w, r, "/s/"+code, http.StatusSeeOther)
		return
	}

	ua := r.UserAgent()
	bot, platform := botFor(ua), platformFor(ua)
	l := s.langFor(r, bot)
	res := s.previews.Get(code)
	counted := r.Method == http.MethodGet

	setPageVary(w)
	switch res.Outcome {
	case preview.Found:
		v := s.foundView(code, res.Preview, l, platform)
		if counted {
			if bot != "" {
				s.stats.BotView(bot)
			} else {
				s.stats.View(v.Type, platform, string(l))
			}
		}
		s.writeHTML(w, http.StatusOK, "private, no-cache", "page.html", v)
	case preview.NotFound:
		if counted {
			s.countMiss(bot, platform, s.stats.NotFound)
		}
		s.writeHTML(w, http.StatusNotFound, "private, no-cache", "page.html", s.notFoundView(l, platform))
	default:
		if counted {
			s.countMiss(bot, platform, s.stats.Failed)
		}
		// The code is valid and the app can open it by itself; only the
		// backend is missing. Never cache this page anywhere.
		s.writeHTML(w, http.StatusOK, "no-store", "page.html", s.failedView(code, l, platform))
	}
}

func (s *Server) countMiss(bot, platform string, count func(string)) {
	if bot != "" {
		s.stats.BotView(bot)
		return
	}
	count(platform)
}

// The page differs by device, language and the lang cookie.
func setPageVary(w http.ResponseWriter) {
	w.Header().Set("Vary", "User-Agent, Accept-Language, Cookie")
}

func (s *Server) serveNotFound(w http.ResponseWriter, r *http.Request) {
	ua := r.UserAgent()
	l := s.langFor(r, botFor(ua))
	// No redirect here: the path is whatever the visitor typed, and
	// redirecting to it could point off-site (//host).
	if q, ok := lang.Parse(r.URL.Query().Get("lang")); ok {
		s.setLangCookie(w, q)
		l = q
	}
	setPageVary(w)
	s.writeHTML(w, http.StatusNotFound, "private, no-cache", "page.html", s.notFoundView(l, platformFor(ua)))
}

// serveRoot sends visitors to the main site in their language.
func (s *Server) serveRoot(w http.ResponseWriter, r *http.Request) {
	l, ok := lang.Parse(r.URL.Query().Get("lang"))
	if !ok {
		l = s.langFor(r, botFor(r.UserAgent()))
	}
	setPageVary(w)
	// 302: the main site's address may change.
	http.Redirect(w, r, s.cfg.MainSiteURL+"/"+string(l), http.StatusFound)
}

// langFor: preview bots never say what language the chat is in, so they
// get Turkmen; people get their cookie, then their browser language.
func (s *Server) langFor(r *http.Request, bot string) lang.Lang {
	if bot != "" {
		return lang.TM
	}
	if c, err := r.Cookie("lang"); err == nil {
		if l, ok := lang.Parse(c.Value); ok {
			return l
		}
	}
	return lang.FromAcceptLanguage(r.Header.Get("Accept-Language"))
}

func (s *Server) setLangCookie(w http.ResponseWriter, l lang.Lang) {
	http.SetCookie(w, &http.Cookie{
		Name:     "lang",
		Value:    string(l),
		Path:     "/",
		MaxAge:   365 * 24 * 3600,
		SameSite: http.SameSiteLaxMode,
		// Over plain http://localhost a Secure cookie would never be set.
		Secure:   s.cfg.Mode != config.ModeMock,
		HttpOnly: true,
	})
}

func (s *Server) baseView(l lang.Lang, platform, state string) pageView {
	t := dictionaries[l]
	v := pageView{
		Lang:     l,
		T:        t,
		State:    state,
		Platform: platform,
		Shape:    "square",
		AppStore: appStoreURL,
		Play:     googlePlayURL,
		MainSite: s.cfg.MainSiteURL + "/" + string(l),
	}
	for _, o := range []lang.Lang{lang.TM, lang.RU} {
		v.Langs = append(v.Langs, langLink{
			Label:    strings.ToUpper(string(o)),
			Href:     "?lang=" + string(o),
			HTMLLang: o.HTML(),
			Current:  o == l,
		})
	}
	return v
}

func (s *Server) foundView(code string, p backend.Preview, l lang.Lang, platform string) pageView {
	v := s.baseView(l, platform, stateFound)
	v.Type = p.Type
	if !backend.KnownType(p.Type) {
		v.Type = "unknown"
	}
	v.TypeLabel = v.T.TypeLabel(p.Type)
	v.Title = p.TitleFor(l)
	if p.Type != backend.TypeArtist {
		v.Artist = p.ArtistName
	}
	v.Shape = shapeName(p.Type)
	if src := p.ArtworkFor(l); src != "" {
		ver := imageVersion(src)
		v.Cover = &coverImage{URL: "/i/" + code + "/cover.jpg?v=" + ver, W: 280, H: 280}
		if v.Shape == "wide" {
			v.Cover.W, v.Cover.H = 320, 180
		}
		v.Backdrop = "/i/" + code + "/bg.jpg?v=" + ver
	}
	s.addShareActions(&v, code)

	link := s.cfg.PublicOrigin + "/s/" + code
	title := v.Title
	if v.Artist != "" {
		title += " · " + v.Artist
	}
	v.Meta = pageMeta{
		Title:       title + " | Belet Music",
		Description: joinNonEmpty(" · ", v.TypeLabel, v.Artist, "Belet Music"),
		OGType:      ogType(p.Type),
		OGTitle:     v.Title,
		URL:         link,
		Image:       s.cfg.PublicOrigin + "/i/" + code + "/og.jpg?v=" + ogVersion(p),
		ImageType:   "image/jpeg",
		ImageAlt:    v.Title,
		AppleApp:    "app-id=" + appStoreID + ", app-argument=" + link,
	}
	return v
}

func (s *Server) notFoundView(l lang.Lang, platform string) pageView {
	v := s.baseView(l, platform, stateNotFound)
	v.Type = "not_found"
	v.Title = v.T.NotFoundTitle
	v.Message = v.T.NotFoundText
	v.Meta = s.genericMeta(v.T, v.T.NotFoundTitle+" | Belet Music")
	v.Meta.AppleApp = "app-id=" + appStoreID
	return v
}

func (s *Server) failedView(code string, l lang.Lang, platform string) pageView {
	v := s.baseView(l, platform, stateFailed)
	v.Type = "failed"
	v.Title = "Belet Music"
	v.Message = v.T.FailedText
	s.addShareActions(&v, code)
	v.Meta = s.genericMeta(v.T, "Belet Music")
	v.Meta.URL = s.cfg.PublicOrigin + "/s/" + code
	v.Meta.AppleApp = "app-id=" + appStoreID + ", app-argument=" + v.Meta.URL
	return v
}

// addShareActions adds what only makes sense with a code: the Android
// intent and the QR for a phone.
func (s *Server) addShareActions(v *pageView, code string) {
	v.QR = "/qr/" + code + ".svg"
	v.OpenApp = intentURL(s.cfg.PublicOrigin, code)
}

func (s *Server) genericMeta(t *texts, title string) pageMeta {
	img, _ := s.static.URL("og-default.png")
	return pageMeta{
		Title:       title,
		Description: t.HomeTitle,
		OGType:      "website",
		OGTitle:     "Belet Music",
		Image:       s.cfg.PublicOrigin + img,
		ImageType:   "image/png",
		ImageAlt:    "Belet Music",
	}
}

// intentURL opens the app on Android Chrome and Samsung Internet, falling
// back to Google Play when the app is missing. html/template would replace
// a non-http scheme with a placeholder, hence template.URL; every part is
// a constant or the validated code.
func intentURL(origin, code string) template.URL {
	host := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	return template.URL("intent://" + host + "/s/" + code +
		"#Intent;scheme=https;package=" + androidPackage +
		";S.browser_fallback_url=" + url.QueryEscape(googlePlayURL) + ";end")
}

func shapeName(itemType string) string {
	switch itemType {
	case backend.TypeArtist:
		return "circle"
	case backend.TypeVideo, backend.TypeVideoPlaylist:
		return "wide"
	}
	return "square"
}

func ogType(itemType string) string {
	switch itemType {
	case backend.TypeTrack:
		return "music.song"
	case backend.TypeAlbum:
		return "music.album"
	case backend.TypePlaylist:
		return "music.playlist"
	case backend.TypeArtist:
		return "profile"
	case backend.TypeVideo:
		return "video.other"
	}
	return "website"
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
