package site

import "strings"

const (
	platformIOS     = "ios"
	platformAndroid = "android"
	platformDesktop = "desktop"
)

// platformFor decides which buttons the server marks as primary. iPadOS
// Safari claims to be a Mac by default; app.js fixes that case on the
// client, where touch support is visible.
func platformFor(ua string) string {
	switch {
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPod"), strings.Contains(ua, "iPad"):
		return platformIOS
	case strings.Contains(ua, "Android"):
		return platformAndroid
	}
	return platformDesktop
}

// Link-preview bots, matched case-insensitively. Order matters where one UA
// names several: Telegram's says "TelegramBot (like TwitterBot)". iMessage
// fetches previews from the phone posing as facebookexternalhit/Twitterbot,
// so it is covered too.
var previewBots = []struct{ needle, name string }{
	{"telegrambot", "telegram"},
	{"whatsapp", "whatsapp"},
	{"facebookexternalhit", "facebook"},
	{"facebot", "facebook"},
	{"meta-externalagent", "facebook"},
	{"twitterbot", "twitter"},
	{"slackbot", "slack"},
	{"discordbot", "discord"},
	{"linkedinbot", "linkedin"},
	{"vkshare", "vk"},
	{"viber", "viber"},
	{"skypeuripreview", "skype"},
	{"iframely", "iframely"},
	{"pinterest", "pinterest"},
	{"redditbot", "reddit"},
	{"applebot", "apple"},
	{"googlebot", "google"},
	{"bingbot", "bing"},
	{"yandexbot", "yandex"},
	{"embedly", "embedly"},
	{"mastodon", "mastodon"},
}

// botFor returns the short bot name, or "" for a person.
func botFor(ua string) string {
	ua = strings.ToLower(ua)
	for _, b := range previewBots {
		if strings.Contains(ua, b.needle) {
			return b.name
		}
	}
	return ""
}
