// Package lang picks the page language: Turkmen by default, Russian when
// the visitor asks for it.
package lang

import (
	"strconv"
	"strings"
)

// Lang is the site's own code. Turkmen is "tm" here, as in the apps and on
// auramusic.pro/tm, but "tk" in HTML (ISO 639-1).
type Lang string

const (
	TM Lang = "tm"
	RU Lang = "ru"
)

// Parse accepts exactly the two values the switcher sends.
func Parse(s string) (Lang, bool) {
	switch Lang(s) {
	case TM, RU:
		return Lang(s), true
	}
	return "", false
}

func (l Lang) HTML() string {
	if l == RU {
		return "ru"
	}
	return "tk"
}

func (l Lang) OGLocale() string {
	if l == RU {
		return "ru_RU"
	}
	return "tk_TM"
}

func (l Lang) Other() Lang {
	if l == RU {
		return TM
	}
	return RU
}

// FromAcceptLanguage picks the highest-q tag among ru, tk and tm (with any
// region). Russian wins only when it is that tag; everything else, including
// a header without any of them, means Turkmen.
func FromAcceptLanguage(header string) Lang {
	best, bestQ := TM, -1.0
	for _, part := range strings.Split(header, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		primary, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
		var l Lang
		switch primary {
		case "ru":
			l = RU
		case "tk", "tm":
			l = TM
		default:
			continue
		}
		q := 1.0
		for _, p := range strings.Split(params, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
			if ok && strings.TrimSpace(k) == "q" {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					q = f
				}
			}
		}
		if q > 0 && q > bestQ {
			best, bestQ = l, q
		}
	}
	return best
}
