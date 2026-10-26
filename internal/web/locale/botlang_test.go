package locale

import "testing"

func TestBotTextSpeaksTheAskedLanguageIndependentOfThePanelLanguage(t *testing.T) {
	initFor(t, "en-US")

	if got := BotText("fa-IR", "tgbot.days"); got != "روز" {
		t.Fatalf("fa-IR = %q", got)
	}
	if got := BotText("en-US", "tgbot.days"); got != "Days" {
		t.Fatalf("en-US = %q", got)
	}
	if got := BotText("xx-XX", "tgbot.days"); got != "Days" {
		t.Fatalf("an unknown language = %q, want the en-US text", got)
	}
	if got := BotText("fa-IR", "no.such.key"); got != "no.such.key" {
		t.Fatalf("an unknown key = %q, want the key itself so the gap is visible", got)
	}
}

func TestBotTextAppliesTheOverrideOfTheBuyersLanguageOnly(t *testing.T) {
	initFor(t, "en-US")
	if err := SetBotOverrides(`{"fa-IR":{"tgbot.commands.start":"سلام {{ .Firstname }}"}}`); err != nil {
		t.Fatal(err)
	}
	if got := BotText("fa-IR", "tgbot.commands.start", "Firstname==سارا"); got != "سلام سارا" {
		t.Fatalf("fa-IR = %q, want the override", got)
	}
	if got := BotText("ru-RU", "tgbot.commands.start", "Firstname==Sara"); got == "" || got == "سلام Sara" {
		t.Fatalf("ru-RU = %q, an override of another language leaked", got)
	}
}

func TestNormalizeBotLangMapsTelegramCodesToShippedLocales(t *testing.T) {
	initFor(t, "en-US")
	cases := map[string]string{
		"fa": "fa-IR", "FA": "fa-IR", "en": "en-US", "en-GB": "en-US", "ru": "ru-RU", "zh": "zh-CN", "zh-hans": "zh-CN",
		"zh-hant": "zh-TW", "zh-TW": "zh-TW", "pt-br": "pt-BR", "uk": "uk-UA", "ar": "ar-EG", "": "", "xx": "", "he": "",
	}
	for in, want := range cases {
		if got := NormalizeBotLang(in); got != want {
			t.Errorf("NormalizeBotLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBotLanguagesListsEveryShippedLocaleWithItsOwnName(t *testing.T) {
	initFor(t, "en-US")
	langs := BotLanguages()
	if len(langs) != 13 {
		t.Fatalf("%d languages, want 13", len(langs))
	}
	names := map[string]string{}
	for _, l := range langs {
		names[l.Code] = l.Name
	}
	if names["fa-IR"] != "فارسی" || names["en-US"] != "English" || names["zh-TW"] != "繁體中文" {
		t.Fatalf("names = %v", names)
	}
}
