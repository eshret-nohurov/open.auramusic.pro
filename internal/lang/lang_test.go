package lang

import "testing"

func TestFromAcceptLanguage(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   Lang
	}{
		{"ru-RU,ru;q=0.9", RU},
		{"en-US,en;q=0.9,ru;q=0.8", RU},
		{"tk,ru;q=0.5", TM},
		{"tk-TM", TM},
		{"tm", TM},
		{"ru;q=0.4,tk;q=0.6", TM},
		{"tk;q=0.4,ru;q=0.6", RU},
		{"en", TM},
		{"de-DE,de;q=0.9", TM},
		{"", TM},
		{"ru;q=0", TM},
		{"RU-ru", RU},
		{"*", TM},
		{"ru;q=abc", RU},
	} {
		if got := FromAcceptLanguage(tc.header); got != tc.want {
			t.Errorf("FromAcceptLanguage(%q) = %s, want %s", tc.header, got, tc.want)
		}
	}
}

func TestParse(t *testing.T) {
	for in, ok := range map[string]bool{"tm": true, "ru": true, "tk": false, "RU": false, "": false, "en": false} {
		if _, got := Parse(in); got != ok {
			t.Errorf("Parse(%q) ok = %v", in, got)
		}
	}
	if TM.HTML() != "tk" || RU.HTML() != "ru" || TM.OGLocale() != "tk_TM" || RU.OGLocale() != "ru_RU" {
		t.Error("html/og codes wrong")
	}
}
