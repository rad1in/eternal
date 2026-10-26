package tgbot

import (
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
	shopCallbackPrefix = "shop:"
	shopSelectionTTL   = 30 * time.Minute
)

// Conversation states a typed message can answer. The id after the colon names
// the order, service or ticket the answer is for.
const (
	shopStateCode        = "shop_code"
	shopStateReceipt     = "shop_receipt:"
	shopStateGB          = "shop_gb"
	shopStateDays        = "shop_days"
	shopStateTicketNew   = "shop_ticket_new:"
	shopStateTicketReply = "shop_ticket_reply:"
	shopStateBan         = "shop_ban"
	shopStateUnban       = "shop_unban"
)

// shopSelection is what one customer has picked so far; callback data is too
// short to carry it, so it is kept here per chat user.
type shopSelection struct {
	planID int
	custom bool
	gb     int
	days   int
	kind   string
	email  string
	code   string
	at     time.Time
}

var (
	shopSelectionsMu sync.Mutex
	shopSelections   = map[chatUser]*shopSelection{}
)

func setShopSelection(actor chatUser, sel *shopSelection) {
	sel.at = time.Now()
	shopSelectionsMu.Lock()
	defer shopSelectionsMu.Unlock()
	for k, v := range shopSelections {
		if time.Since(v.at) > shopSelectionTTL {
			delete(shopSelections, k)
		}
	}
	shopSelections[actor] = sel
}

func getShopSelection(actor chatUser) *shopSelection {
	shopSelectionsMu.Lock()
	defer shopSelectionsMu.Unlock()
	sel := shopSelections[actor]
	if sel == nil || time.Since(sel.at) > shopSelectionTTL {
		return nil
	}
	copied := *sel
	return &copied
}

func clearShopSelection(actor chatUser) {
	shopSelectionsMu.Lock()
	delete(shopSelections, actor)
	shopSelectionsMu.Unlock()
}

func (t *Tgbot) shopEnabled() bool {
	cfg, err := t.salesConfig.Get()
	return err == nil && cfg.TelegramShop
}

func isShopCallback(data string) bool { return strings.HasPrefix(data, shopCallbackPrefix) }

// shopError answers a refusal in the customer's words; anything that is not a
// customer-safe SalesError is logged and shown as the generic failure.
func (c *shopCtx) shopError(err error) string {
	if code := service.SalesErrorCode(err); code != "" {
		key := "shop.errors." + code
		if text := c.tr(key); text != key {
			return text
		}
	}
	logger.Warning("tgbot shop:", err)
	return c.tr("shop.errors.network")
}

// handleShop answers /shop and the shop entry of /start. While the shop is off
// a stranger gets nothing, as for every other command it is not allowed.
func (t *Tgbot) handleShop(chatId int64, from *telego.User) {
	if !t.shopEnabled() {
		if t.levelOf(from.ID) != levelStranger {
			t.SendMsgToTgbot(chatId, locale.BotText(t.shopLangOf(from.ID), "shopbot.disabled"))
		}
		return
	}
	c, ok := t.newShopCtx(chatId, from, 0)
	if !ok {
		return
	}
	if missing := c.missingChannels(); len(missing) > 0 {
		c.showJoinGate(missing)
		return
	}
	c.showMenu()
}

func (c *shopCtx) trialAvailable() bool {
	enabled, err := c.t.settingService.GetTgTrialEnable()
	if err != nil || !enabled {
		return false
	}
	templateID, err := c.t.settingService.GetTgTrialTemplateId()
	return err == nil && templateID > 0 && !c.t.clientService.TrialUsed(c.from.ID)
}

// miniAppURL is the shop page Telegram opens inside the chat; Telegram only
// accepts https, and Bale has no Mini Apps.
func (c *shopCtx) miniAppURL() string {
	if !c.cfg.Bot.MiniApp || !c.cfg.PublicEnabled || baleMode.Load() || !strings.HasPrefix(c.cfg.PublicBaseURL, "https://") {
		return ""
	}
	return c.cfg.PublicBaseURL + "/shop/"
}

func (c *shopCtx) menuKeyboard() *telego.InlineKeyboardMarkup {
	rows := [][]telego.InlineKeyboardButton{
		rowOf(btn(c.tr("shopbot.btn.buy"), "shop:buy"), btn(c.tr("shopbot.btn.services"), "shop:sv")),
		rowOf(btn(c.tr("shopbot.btn.wallet"), "shop:wallet"), btn(c.tr("shopbot.btn.invite"), "shop:ref")),
	}
	row3 := []telego.InlineKeyboardButton{}
	if c.trialAvailable() {
		row3 = append(row3, btn(c.tr("shopbot.btn.trial"), "shop:trial"))
	}
	row3 = append(row3, btn(c.tr("shopbot.btn.support"), "shop:sup"))
	rows = append(rows, rowOf(row3...), rowOf(btn(c.tr("shopbot.btn.orders"), "shop:orders"), btn(c.tr("shopbot.btn.language"), "shop:lang")))
	for _, l := range c.cfg.Bot.Links {
		rows = append(rows, rowOf(urlBtn(l.Label, l.URL)))
	}
	if url := c.miniAppURL(); url != "" {
		rows = append(rows, rowOf(tu.InlineKeyboardButton(c.tr("shopbot.btn.miniapp")).WithWebApp(&telego.WebAppInfo{URL: url})))
	}
	if c.admin {
		rows = append(rows, rowOf(btn(c.tr("shopbot.btn.admin"), "shop:ad")))
	}
	return keyboard(rows...)
}

func (c *shopCtx) showMenu() {
	c.show(c.tr("shopbot.menu.welcome", "Name=="+esc(c.from.FirstName)), c.menuKeyboard())
}

// answerShopCallback serves every shop button. Each step re-checks the switch
// and derives the buyer from the tapping account, never from the data.
func (t *Tgbot) answerShopCallback(query *telego.CallbackQuery) {
	chat := query.Message.GetChat()
	parts := strings.Split(strings.TrimPrefix(query.Data, shopCallbackPrefix), ":")
	if !shopLimiter.allow(query.From.ID, time.Now()) {
		t.sendCallbackAnswerTgBot(query.ID, locale.BotText(t.shopLangOf(query.From.ID), "shopbot.slowDown"))
		return
	}
	t.sendCallbackAnswerTgBot(query.ID, "")
	if adminOnlyAction(parts[0]) {
		t.answerShopAdmin(query, parts)
		return
	}
	if !t.shopEnabled() {
		t.SendMsgToTgbot(chat.ID, locale.BotText(t.shopLangOf(query.From.ID), "shopbot.disabled"))
		return
	}
	if chat.Type != "" && chat.Type != telego.ChatTypePrivate {
		return
	}
	c, ok := t.newShopCtx(chat.ID, &query.From, query.Message.GetMessageID())
	if !ok {
		return
	}
	if parts[0] != "lang" && parts[0] != "sl" {
		if missing := c.missingChannels(); len(missing) > 0 {
			c.showJoinGate(missing)
			return
		}
	}
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}
	c.route(parts[0], arg)
}

func (c *shopCtx) route(action, arg string) {
	switch action {
	case "menu":
		c.showMenu()
	case "buy":
		c.shopStartBuying(&shopSelection{})
	case "cat":
		c.shopShowCategory(arg)
	case "p":
		c.shopChoosePlan(arg)
	case "k":
		c.shopChooseAccount(arg)
	case "cu":
		c.shopStartCalculator()
	case "cg":
		c.shopPickGB(arg)
	case "cd":
		c.shopPickDays(arg)
	case "code":
		c.shopAskCode()
	case "g":
		c.shopCheckout(arg)
	case "orders":
		c.shopShowOrders()
	case "o":
		c.shopShowOrder(arg)
	case "rc":
		c.shopAskReceipt(arg)
	case "sv":
		c.shopShowServices(arg)
	case "lk":
		c.shopSendLink(arg)
	case "qr":
		c.shopSendQR(arg)
	case "rn":
		c.shopRenewService(arg)
	case "ao":
		c.shopAddonService(arg)
	case "wallet":
		c.shopShowWallet()
	case "ref":
		c.shopShowReferral()
	case "trial":
		c.shopTrial()
	case "sup":
		c.shopSupport(arg)
	case "tk":
		c.shopShowTicket(arg)
	case "tn":
		c.shopAskNewTicket(arg)
	case "tr":
		c.shopAskTicketReply(arg)
	case "lang":
		c.shopShowLanguages()
	case "sl":
		c.shopSetLanguage(arg)
	}
}

func (c *shopCtx) shopTrial() {
	outcome, email := c.t.grantTrial(c.from.ID)
	back := keyboard(rowOf(btn(c.tr("shopbot.btn.menu"), "shop:menu")))
	switch outcome {
	case trialUnavailable:
		c.show(c.tr("tgbot.messages.trialDisabled"), back)
	case trialUsed:
		c.show(c.tr("tgbot.messages.trialUsed"), back)
	case trialFailed:
		c.show(c.tr("tgbot.messages.trialFailed"), back)
	case trialGranted:
		c.show(c.trialReadyText(), back)
		c.sendServiceLinks(email)
	}
}

func (c *shopCtx) trialReadyText() string {
	templateID, _ := c.t.settingService.GetTgTrialTemplateId()
	tpl, err := c.t.clientService.GetClientTemplate(templateID)
	if err != nil {
		return c.tr("tgbot.messages.trialFailed")
	}
	quota := c.tr("shopbot.unlimited")
	if tpl.TotalGB > 0 {
		quota = formatBytes(tpl.TotalGB)
	}
	return c.tr("tgbot.messages.trialReady", "Hours=="+strconv.Itoa(tpl.ExpiryDays*24), "Quota=="+quota)
}

func (c *shopCtx) shopShowLanguages() {
	langs := locale.BotLanguages()
	rows := make([][]telego.InlineKeyboardButton, 0, len(langs)/2+2)
	for i := 0; i < len(langs); i += 2 {
		row := []telego.InlineKeyboardButton{btn(langs[i].Name, "shop:sl:"+langs[i].Code)}
		if i+1 < len(langs) {
			row = append(row, btn(langs[i+1].Name, "shop:sl:"+langs[i+1].Code))
		}
		rows = append(rows, rowOf(row...))
	}
	rows = append(rows, rowOf(btn(c.tr("shopbot.btn.menu"), "shop:menu")))
	c.show(c.tr("shopbot.lang.choose"), keyboard(rows...))
}

func (c *shopCtx) shopSetLanguage(code string) {
	if err := locale.ValidBotLang(code); err != nil {
		return
	}
	if err := c.t.botUsers.SetLang(c.from.ID, code); err != nil {
		logger.Warning("tgbot shop: saving language:", err)
		return
	}
	c.lang = code
	c.showMenu()
}

const referralStartPrefix = "ref_"

// startWithReferral handles /start ref_CODE: the code is kept for the first
// purchase and the customer lands in the shop. It reports whether the
// argument was a referral link.
func (t *Tgbot) startWithReferral(chatId int64, from *telego.User, arg string) bool {
	code, ok := strings.CutPrefix(arg, referralStartPrefix)
	if !ok {
		return false
	}
	if !t.shopEnabled() {
		return false
	}
	if err := t.botUsers.RememberReferral(from.ID, code); err != nil {
		logger.Warning("tgbot shop: saving referral:", err)
	}
	t.handleShop(chatId, from)
	return true
}
