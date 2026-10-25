package tgbot

import (
	"context"
	"html"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

const (
	shopActionsPerWindow = 40
	shopActionWindow     = time.Minute
	joinCheckTTL         = 5 * time.Minute
)

// shopCtx is one customer interaction: who, in which chat, in which language,
// and the message a tap came from so the screen can be edited in place.
type shopCtx struct {
	t     *Tgbot
	chat  int64
	from  *telego.User
	actor chatUser
	lang  string
	msgID int
	admin bool
	cfg   *service.SalesConfig
}

func (c *shopCtx) tr(key string, kv ...string) string { return locale.BotText(c.lang, key, kv...) }

func (c *shopCtx) send(text string, kb *telego.InlineKeyboardMarkup) {
	if kb == nil {
		c.t.SendMsgToTgbot(c.chat, text)
		return
	}
	c.t.SendMsgToTgbot(c.chat, text, kb)
}

// show replaces the screen the customer tapped in; a typed message, or a
// message that cannot be edited (a photo), gets a fresh one instead.
func (c *shopCtx) show(text string, kb *telego.InlineKeyboardMarkup) {
	if c.msgID == 0 || !c.t.tryEditMessage(c.chat, c.msgID, text, kb) {
		c.send(text, kb)
	}
}

// tryEditMessage reports whether the screen now shows text; a no-op edit
// counts, any other failure tells the caller to send a new message.
func (t *Tgbot) tryEditMessage(chat int64, msgID int, text string, kb *telego.InlineKeyboardMarkup) bool {
	if !t.IsRunning() || bot == nil || len(text) > telegramPageLimit {
		return false
	}
	text, parseMode := platformText(text)
	params := &telego.EditMessageTextParams{ChatID: tu.ID(chat), MessageID: msgID, Text: text, ParseMode: parseMode, ReplyMarkup: kb}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := bot.EditMessageText(ctx, params); err != nil {
		return isTelegramNotModifiedError(err)
	}
	return true
}

// newShopCtx identifies the customer and settles their language. It reports
// false for a barred account, which gets one polite refusal and nothing more.
func (t *Tgbot) newShopCtx(chat int64, from *telego.User, msgID int) (*shopCtx, bool) {
	cfg, err := t.salesConfig.Get()
	if err != nil {
		logger.Warning("tgbot shop: sales config:", err)
		return nil, false
	}
	c := &shopCtx{t: t, chat: chat, from: from, actor: chatUser{chatID: chat, userID: from.ID}, msgID: msgID, admin: checkAdmin(from.ID), cfg: cfg}
	c.lang = t.shopLang(from)
	if !c.admin && t.botUsers.IsBanned(from.ID) {
		c.send(c.tr("shopbot.banned"), nil)
		return c, false
	}
	return c, true
}

// shopLang is the customer's chosen language, else Telegram's guess, else the
// panel's bot language.
func (t *Tgbot) shopLang(from *telego.User) string {
	guess := locale.NormalizeBotLang(from.LanguageCode)
	if u, err := t.botUsers.Touch(from.ID, from.FirstName, from.Username, guess); err == nil && u.Lang != "" {
		return u.Lang
	}
	if guess != "" {
		return guess
	}
	lang, _ := t.settingService.GetTgLang()
	return lang
}

// shopLangOf reads the language of a customer who is not the current sender
// (an order's buyer being told an admin's decision).
func (t *Tgbot) shopLangOf(tgID int64) string {
	if u, err := t.botUsers.Get(tgID); err == nil && u.Lang != "" {
		return u.Lang
	}
	lang, _ := t.settingService.GetTgLang()
	return lang
}

// actionLimiter caps how fast one account may press shop buttons.
type actionLimiter struct {
	mu   sync.Mutex
	hits map[int64]*actionWindow
}

type actionWindow struct {
	start time.Time
	count int
}

var shopLimiter = &actionLimiter{hits: map[int64]*actionWindow{}}

func (l *actionLimiter) allow(id int64, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) > 10000 {
		for k, w := range l.hits {
			if now.Sub(w.start) > shopActionWindow {
				delete(l.hits, k)
			}
		}
	}
	w := l.hits[id]
	if w == nil || now.Sub(w.start) > shopActionWindow {
		l.hits[id] = &actionWindow{start: now, count: 1}
		return true
	}
	w.count++
	return w.count <= shopActionsPerWindow
}

var (
	joinCacheMu sync.Mutex
	joinCache   = map[int64]time.Time{}
)

// missingChannels lists the required channels the customer has not joined. A
// channel the bot cannot read counts as joined, so a misconfiguration never
// locks every customer out.
func (c *shopCtx) missingChannels() []service.ForceJoinChat {
	required := c.cfg.Bot.ForceJoin
	if len(required) == 0 || c.admin {
		return nil
	}
	joinCacheMu.Lock()
	at, ok := joinCache[c.from.ID]
	joinCacheMu.Unlock()
	if ok && time.Since(at) < joinCheckTTL {
		return nil
	}
	var missing []service.ForceJoinChat
	for _, j := range required {
		if !c.t.isChannelMember(j.Chat, c.from.ID) {
			missing = append(missing, j)
		}
	}
	if len(missing) == 0 {
		joinCacheMu.Lock()
		joinCache[c.from.ID] = time.Now()
		joinCacheMu.Unlock()
	}
	return missing
}

func (t *Tgbot) isChannelMember(chat string, userID int64) bool {
	if bot == nil {
		return true
	}
	target := tu.Username(chat)
	if id, err := strconv.ParseInt(chat, 10, 64); err == nil {
		target = tu.ID(id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	member, err := bot.GetChatMember(ctx, &telego.GetChatMemberParams{ChatID: target, UserID: userID})
	if err != nil {
		logger.Warningf("tgbot shop: cannot check membership of %s in %s: %v", strconv.FormatInt(userID, 10), chat, err)
		return true
	}
	switch m := member.(type) {
	case *telego.ChatMemberRestricted:
		return m.IsMember
	default:
		status := member.MemberStatus()
		return status == telego.MemberStatusCreator || status == telego.MemberStatusAdministrator || status == telego.MemberStatusMember
	}
}

func (c *shopCtx) showJoinGate(missing []service.ForceJoinChat) {
	rows := make([][]telego.InlineKeyboardButton, 0, len(missing)+1)
	for _, j := range missing {
		label := j.Chat
		if !strings.HasPrefix(label, "@") {
			label = c.tr("shopbot.join.channel")
		}
		rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(c.tr("shopbot.join.open", "Channel=="+label)).WithURL(j.Link)))
	}
	rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(c.tr("shopbot.join.done")).WithCallbackData("shop:menu")))
	c.show(c.tr("shopbot.join.required"), tu.InlineKeyboard(rows...))
}

func btn(text, data string) telego.InlineKeyboardButton {
	return tu.InlineKeyboardButton(text).WithCallbackData(data)
}

func urlBtn(text, url string) telego.InlineKeyboardButton {
	return tu.InlineKeyboardButton(text).WithURL(url)
}

func rowOf(buttons ...telego.InlineKeyboardButton) []telego.InlineKeyboardButton {
	return tu.InlineKeyboardRow(buttons...)
}

func keyboard(rows ...[]telego.InlineKeyboardButton) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(rows...)
}

func esc(s string) string { return html.EscapeString(s) }

// formatAmount prints a minor-unit amount with its currency's decimals and
// thousands separators, and the currency in the customer's words.
func (c *shopCtx) formatAmount(amount int64, currency string) string {
	return groupDigits(amount, currencyDecimals(currency)) + " " + c.currencyName(currency)
}

func currencyDecimals(currency string) int {
	d, _ := service.CurrencyDecimals(currency)
	return d
}

func (c *shopCtx) currencyName(currency string) string {
	key := "shopbot.currency." + currency
	if name := c.tr(key); name != key {
		return name
	}
	return currency
}

func groupDigits(amount int64, decimals int) string {
	neg := amount < 0
	if neg {
		amount = -amount
	}
	digits := strconv.FormatInt(amount, 10)
	frac := ""
	if decimals > 0 {
		for len(digits) <= decimals {
			digits = "0" + digits
		}
		frac = "." + digits[len(digits)-decimals:]
		digits = digits[:len(digits)-decimals]
	}
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := b.String() + frac
	if neg {
		out = "-" + out
	}
	return out
}

// usageBar draws used/total as ten blocks; an unlimited quota has no bar.
func usageBar(used, total int64) string {
	if total <= 0 {
		return ""
	}
	filled := int(used * 10 / total)
	if used > 0 && filled == 0 {
		filled = 1
	}
	filled = min(filled, 10)
	return strings.Repeat("▓", filled) + strings.Repeat("░", 10-filled)
}
