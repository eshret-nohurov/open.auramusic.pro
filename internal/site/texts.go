package site

import (
	"github.com/eshret-nohurov/open.auramusic.pro/internal/backend"
	"github.com/eshret-nohurov/open.auramusic.pro/internal/lang"
)

// Store links, as on auramusic.pro (web/src/lib/site.ts).
const (
	appStoreID     = "6503349020"
	appStoreURL    = "https://apps.apple.com/tm/app/belet-music/id" + appStoreID
	androidPackage = "com.sanlytilsimat.musicapp"
	googlePlayURL  = "https://play.google.com/store/apps/details?id=" + androidPackage
)

type texts struct {
	OpenInApp      string
	StoreAppleTop  string
	StoreGoogleTop string
	QRTitle        string
	QRText         string
	DesktopApps    string
	NotFoundTitle  string
	NotFoundText   string
	FailedText     string
	LangLabel      string
	QRAlt          string
	// HomeTitle is the main site's home title, the description for pages
	// that have no item to describe.
	HomeTitle string
	types     map[string]string
}

// Turkmen phrases marked "unchecked" were written by a non-native speaker
// and await the owner's review; the store lines come from auramusic.pro.
var dictionaries = map[lang.Lang]*texts{
	lang.TM: {
		OpenInApp:      "Belet Music-de açmak", // unchecked
		StoreAppleTop:  "Ýükläp alyň",
		StoreGoogleTop: "Ýükläp alyp bilersiňiz",
		QRTitle:        "Telefonyňyzyň kamerasyny QR-koda tutuň", // unchecked
		QRText:         "Salgy telefonyňyzda açylar",             // unchecked
		DesktopApps:    "iPhone we Android üçin Belet Music",     // unchecked
		NotFoundTitle:  "Tapylmady",                              // app key share_link_not_found
		NotFoundText:   "Salgy nädogry ýa-da indi işlemeýär.",    // unchecked
		FailedText:     "Ýükläp bolmady. Salgyny programmada açyň.",
		LangLabel:      "Dil",
		QRAlt:          "Bu sahypanyň QR-kody",
		HomeTitle:      "Aýdymlar. Wideolar. Podkastlar",
		// Type names are the app's translations; the comment is the key.
		types: map[string]string{
			backend.TypeTrack:         "Aýdym",          // track
			backend.TypeAlbum:         "Albom",          // album
			backend.TypePlaylist:      "Pleýlist",       // playlist
			backend.TypeArtist:        "Artist",         // artist
			backend.TypeVideo:         "Wideo",          // share_type_video
			backend.TypeVideoPlaylist: "Wideo pleýlist", // share_type_video_playlist
		},
	},
	lang.RU: {
		OpenInApp:      "Открыть в Belet Music",
		StoreAppleTop:  "Загрузить в",
		StoreGoogleTop: "Доступно в",
		QRTitle:        "Наведите камеру телефона на QR-код",
		QRText:         "Ссылка откроется на телефоне",
		DesktopApps:    "Belet Music для iPhone и Android",
		NotFoundTitle:  "Не найдено",
		NotFoundText:   "Ссылка неверная или больше не работает.",
		FailedText:     "Не удалось загрузить. Откройте ссылку в приложении.",
		LangLabel:      "Язык",
		QRAlt:          "QR-код этой страницы",
		HomeTitle:      "Музыка. Видео. Подкасты",
		types: map[string]string{
			backend.TypeTrack:         "Песня",
			backend.TypeAlbum:         "Альбом",
			backend.TypePlaylist:      "Плейлист",
			backend.TypeArtist:        "Артист",
			backend.TypeVideo:         "Видео",
			backend.TypeVideoPlaylist: "Видео-плейлист",
		},
	},
}

// cardListen is drawn on the 1200×630 card, which is always Turkmen.
const cardListen = "Belet Music-de diňläň" // unchecked

// TypeLabel is empty for types the site does not know yet.
func (t *texts) TypeLabel(itemType string) string { return t.types[itemType] }
