package locale

import (
	"sort"
	"strings"
)

// BotLanguage is a language a buyer can pick in the shop bot, named in itself.
type BotLanguage struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

var botLanguageNames = map[string]string{
	"ar-EG": "العربية", "en-US": "English", "es-ES": "Español", "fa-IR": "فارسی", "id-ID": "Bahasa Indonesia",
	"ja-JP": "日本語", "pt-BR": "Português", "ru-RU": "Русский", "tr-TR": "Türkçe", "uk-UA": "Українська",
	"vi-VN": "Tiếng Việt", "zh-CN": "简体中文", "zh-TW": "繁體中文",
}

// telegramLangTags maps the language_code Telegram reports (lower-cased) to a
// shipped locale; the first subtag is tried when the full tag is unknown.
var telegramLangTags = map[string]string{
	"ar": "ar-EG", "en": "en-US", "es": "es-ES", "fa": "fa-IR", "id": "id-ID", "ja": "ja-JP", "pt": "pt-BR",
	"ru": "ru-RU", "tr": "tr-TR", "uk": "uk-UA", "vi": "vi-VN", "zh": "zh-CN", "zh-hans": "zh-CN",
	"zh-hant": "zh-TW", "zh-tw": "zh-TW", "zh-hk": "zh-TW", "zh-cn": "zh-CN",
}

// NormalizeBotLang turns a Telegram language_code into a shipped locale code,
// or "" when none fits.
func NormalizeBotLang(tag string) string {
	tag = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"))
	if code, ok := telegramLangTags[tag]; ok {
		return code
	}
	primary, _, _ := strings.Cut(tag, "-")
	return telegramLangTags[primary]
}

// BotLanguages lists the shipped locales for the language picker.
func BotLanguages() []BotLanguage {
	rawMu.RLock()
	defer rawMu.RUnlock()
	out := make([]BotLanguage, 0, len(rawTranslations))
	for code := range rawTranslations {
		name := botLanguageNames[code]
		if name == "" {
			name = code
		}
		out = append(out, BotLanguage{Code: code, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// BotText renders a bot message in the buyer's language, with the admin's
// reworded text for that language when there is one. A language or key the
// bundle lacks falls back to en-US, then to the key itself.
func BotText(lang, key string, params ...string) string {
	data := createTemplateData(params)
	if text, ok := botOverrideForLang(lang, key, data); ok {
		return text
	}
	for _, l := range []string{lang, "en-US"} {
		if l == "" {
			continue
		}
		if text := I18nForLang(l, key, params...); text != key && text != "" {
			return text
		}
	}
	return key
}
