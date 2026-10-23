package backend

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/eshret-nohurov/open.auramusic.pro/internal/lang"
)

// Item types the apps can share. The backend may add more (user playlists
// are planned); the site still shows those, just without a type label.
const (
	TypeTrack         = "track"
	TypeAlbum         = "album"
	TypePlaylist      = "playlist"
	TypeArtist        = "artist"
	TypeVideo         = "video"
	TypeVideoPlaylist = "video_playlist"
)

func KnownType(t string) bool {
	switch t {
	case TypeTrack, TypeAlbum, TypePlaylist, TypeArtist, TypeVideo, TypeVideoPlaylist:
		return true
	}
	return false
}

// Preview is the backend's answer for a code. Playlists and video playlists
// come with both languages; other types only fill Title.
type Preview struct {
	Type         string
	Title        string
	TitleRU      string
	TitleTM      string
	ArtistName   string
	ThumbURL     string
	ArtworkURL   string
	ThumbURLTM   string
	ArtworkURLTM string
}

const (
	maxTextRunes = 300
	maxURLBytes  = 2048
)

// parsePreview reads the JSON leniently: unknown fields are ignored, and a
// missing, null or non-string field is an empty string. The full format per
// type lives in the backend's docs/deeplink-api.md, which the site does not
// have yet, so nothing here may fail on a surprise.
func parsePreview(body []byte) (Preview, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return Preview{}, err
	}
	str := func(key string) string {
		var s string
		if json.Unmarshal(raw[key], &s) != nil {
			return ""
		}
		return strings.TrimSpace(s)
	}
	text := func(key string) string { return clip(str(key), maxTextRunes) }
	link := func(key string) string {
		if s := str(key); len(s) <= maxURLBytes {
			return s
		}
		return ""
	}
	return Preview{
		Type:         str("type"),
		Title:        text("title"),
		TitleRU:      text("title_ru"),
		TitleTM:      text("title_tm"),
		ArtistName:   text("artist_name"),
		ThumbURL:     link("thumb_url"),
		ArtworkURL:   link("artwork_url"),
		ThumbURLTM:   link("thumb_url_tm"),
		ArtworkURLTM: link("artwork_url_tm"),
	}, nil
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for range n - 1 {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return strings.TrimSpace(s[:i]) + "…"
}

// TitleFor prefers the title in the page language, then the neutral one,
// then the other language.
func (p Preview) TitleFor(l lang.Lang) string {
	if l == lang.RU {
		return first(p.TitleRU, p.Title, p.TitleTM)
	}
	return first(p.TitleTM, p.Title, p.TitleRU)
}

// ArtworkFor picks the cover the same way, full size before thumbnails.
func (p Preview) ArtworkFor(l lang.Lang) string {
	if l == lang.RU {
		return first(p.ArtworkURL, p.ArtworkURLTM, p.ThumbURL, p.ThumbURLTM)
	}
	return first(p.ArtworkURLTM, p.ArtworkURL, p.ThumbURLTM, p.ThumbURL)
}

func (p Preview) hasTitle() bool { return first(p.Title, p.TitleRU, p.TitleTM) != "" }

func first(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
