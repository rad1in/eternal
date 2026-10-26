package tgbot

import (
	"strings"
	"testing"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
)

func TestTunnelEventsAreFormatted(t *testing.T) {
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	_ = bundle.AddMessages(language.MustParse("en-US"),
		&i18n.Message{ID: "tgbot.messages.eventTunnelDown", Other: "tunnel {{ .Name }} is down"},
		&i18n.Message{ID: "tgbot.messages.eventTunnelUp", Other: "tunnel {{ .Name }} is up"},
		&i18n.Message{ID: "tgbot.messages.eventErrorDetail", Other: "Error: {{ .Error }}"},
	)
	orig := locale.LocalizerBot
	t.Cleanup(func() { locale.LocalizerBot = orig })
	locale.LocalizerBot = i18n.NewLocalizer(bundle, "en-US")
	tb := &Tgbot{}

	down := tb.formatEventMessage(eventbus.Event{Type: eventbus.EventTunnelDown, Source: "de-ir",
		Data: &eventbus.TunnelEventData{Error: "no link in 8 s"}})
	for _, want := range []string{"tunnel de-ir is down", "Error: no link in 8 s"} {
		if !strings.Contains(down, want) {
			t.Errorf("down message lacks %q:\n%s", want, down)
		}
	}
	if up := tb.formatEventMessage(eventbus.Event{Type: eventbus.EventTunnelUp, Source: "de-ir"}); !strings.Contains(up, "tunnel de-ir is up") {
		t.Errorf("up message = %s", up)
	}
}
