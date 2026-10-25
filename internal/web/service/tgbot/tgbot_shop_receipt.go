package tgbot

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/websocket"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Presets an admin can reject with; the text is stored in the buyer's language.
var rejectReasonKeys = []string{"", "shopbot.reject.notFound", "shopbot.reject.amount", "shopbot.reject.duplicate"}

func (c *shopCtx) shopAskReceipt(arg string) {
	id, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	if _, err := c.t.orderService.TelegramOrder(c.from.ID, id); err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	c.t.setUserState(c.actor, shopStateReceipt+arg)
	c.send(c.tr("shopbot.askReceipt"), nil)
}

// submitReceipt takes the customer's photo or tracking number and puts the
// order in front of the admins. It reports whether the answer was consumed.
func (c *shopCtx) submitReceipt(orderID string, m *telego.Message) bool {
	id, err := strconv.Atoi(orderID)
	if err != nil {
		return true
	}
	fileID, ref, note := "", m.Text, m.Caption
	if n := len(m.Photo); n > 0 {
		fileID, ref = m.Photo[n-1].FileID, ""
	}
	order, err := c.t.orderService.SubmitTelegramReceipt(c.from.ID, id, ref, note, fileID)
	if err != nil {
		c.t.setUserState(c.actor, shopStateReceipt+orderID)
		c.send(c.shopError(err), nil)
		return true
	}
	c.send(c.tr("shopbot.receiptSent"), nil)
	c.sendOrder(order)
	return true
}

// --- admin side ---

func (t *Tgbot) receiptRecipients(cfg *service.SalesConfig) []int64 {
	chats := adminSnapshot()
	if id := cfg.Bot.ReceiptChatId; id != 0 && !slices.Contains(chats, id) {
		chats = append(chats, id)
	}
	return chats
}

func receiptKeyboard(c *shopCtx, id int) *telego.InlineKeyboardMarkup {
	sid := strconv.Itoa(id)
	approve := btn(c.tr("shopbot.btn.approve"), "shop:ra:"+sid)
	approve.Style = "success"
	reject := btn(c.tr("shopbot.btn.reject"), "shop:rj:"+sid)
	reject.Style = "danger"
	return keyboard(rowOf(approve, reject))
}

// receiptText describes an order in review to an admin, in their language.
func (c *shopCtx) receiptText(order *model.Order) string {
	buyer := c.tr("shopbot.admin.web")
	if order.TgId != 0 {
		buyer = `<a href="tg://user?id=` + strconv.FormatInt(order.TgId, 10) + `">` + strconv.FormatInt(order.TgId, 10) + `</a>`
		if u, err := c.t.botUsers.Get(order.TgId); err == nil && u.Username != "" {
			buyer += " @" + esc(u.Username)
		}
	}
	return c.tr("shopbot.admin.receipt", "Id=="+strconv.Itoa(order.Id), "Buyer=="+buyer, "Account=="+esc(order.ClientEmail),
		"Item=="+esc(c.orderTitle(order)), "Kind=="+c.kindLabel(order.Kind),
		"Amount=="+c.formatAmount(order.Amount, order.Currency), "Ref=="+esc(order.ReceiptRef), "Note=="+esc(order.ReceiptNote))
}

// adminCtx is a context for talking to an admin chat in the admin's own
// language; the sender is unknown, so only the chat matters.
func (t *Tgbot) adminCtx(chat int64, cfg *service.SalesConfig) *shopCtx {
	return &shopCtx{t: t, chat: chat, from: &telego.User{ID: chat}, actor: chatUser{chatID: chat, userID: chat}, lang: t.shopLangOf(chat), admin: true, cfg: cfg}
}

// sendReceiptTo puts one order in review in front of an admin chat, with the
// photo when the buyer sent one.
func (t *Tgbot) sendReceiptTo(chat int64, cfg *service.SalesConfig, order *model.Order) {
	c := t.adminCtx(chat, cfg)
	text, kb := c.receiptText(order), receiptKeyboard(c, order.Id)
	if order.ReceiptFileId != "" && bot != nil && t.IsRunning() {
		params := &telego.SendPhotoParams{ChatID: tu.ID(chat), Photo: telego.InputFile{FileID: order.ReceiptFileId}, Caption: text, ParseMode: telego.ModeHTML, ReplyMarkup: kb}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := bot.SendPhoto(ctx, params); err == nil {
			return
		} else {
			logger.Warning("tgbot shop: receipt photo:", err)
		}
	}
	t.SendMsgToTgbot(chat, text, kb)
}

// notifyReceipt tells every admin chat that an order waits for approval.
func (t *Tgbot) notifyReceipt(orderID int) {
	cfg, err := t.salesConfig.Get()
	if err != nil {
		return
	}
	order, err := t.orderService.GetOrder(orderID)
	if err != nil || order.Status != model.OrderStatusReview {
		return
	}
	for _, chat := range t.receiptRecipients(cfg) {
		t.sendReceiptTo(chat, cfg, order)
	}
}

func adminOnlyAction(action string) bool {
	return action == "ra" || action == "rj" || action == "rb" || action == "ad"
}

// answerShopAdmin serves the buttons only an admin may press. The check is on
// the tapping account, whichever chat the keyboard sits in.
func (t *Tgbot) answerShopAdmin(query *telego.CallbackQuery, parts []string) {
	if !checkAdmin(query.From.ID) {
		return
	}
	chat := query.Message.GetChat()
	c, ok := t.newShopCtx(chat.ID, &query.From, query.Message.GetMessageID())
	if !ok {
		return
	}
	switch parts[0] {
	case "ra":
		c.approveReceipt(query, parts)
	case "rj":
		c.rejectReceipt(query, parts)
	case "rb":
		c.restoreReceiptKeyboard(query, parts)
	case "ad":
		c.adminRoute(parts)
	}
}

func orderIDOf(parts []string) (int, bool) {
	if len(parts) < 2 {
		return 0, false
	}
	id, err := strconv.Atoi(parts[1])
	return id, err == nil
}

func (c *shopCtx) approveReceipt(query *telego.CallbackQuery, parts []string) {
	id, ok := orderIDOf(parts)
	if !ok {
		return
	}
	needRestart, err := c.t.orderService.FulfilOrder(&c.t.inboundService, id)
	if needRestart {
		c.t.xrayService.SetToNeedRestart()
	}
	if err != nil {
		if service.SalesErrorCode(err) == service.ErrOrderState {
			c.send(c.tr("shopbot.admin.alreadyHandled", "Id=="+strconv.Itoa(id)), nil)
			return
		}
		logger.Warningf("tgbot shop: approving order %d: %v", id, err)
		c.send(c.tr("shopbot.admin.failed", "Id=="+strconv.Itoa(id)), nil)
		return
	}
	websocket.BroadcastInvalidate(websocket.MessageTypeClients)
	c.markDecision(query, id, c.tr("shopbot.admin.approvedBy", "Name=="+esc(c.from.FirstName)))
}

func (c *shopCtx) rejectReceipt(query *telego.CallbackQuery, parts []string) {
	id, ok := orderIDOf(parts)
	if !ok {
		return
	}
	if len(parts) < 3 {
		rows := make([][]telego.InlineKeyboardButton, 0, len(rejectReasonKeys)+1)
		for i, key := range rejectReasonKeys {
			label := c.tr("shopbot.reject.none")
			if key != "" {
				label = c.tr(key)
			}
			rows = append(rows, rowOf(btn(label, "shop:rj:"+parts[1]+":"+strconv.Itoa(i))))
		}
		rows = append(rows, rowOf(btn(c.tr("shopbot.btn.back"), "shop:rb:"+parts[1])))
		c.t.editReplyMarkup(c.chat, query.Message.GetMessageID(), keyboard(rows...))
		return
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil || n < 0 || n >= len(rejectReasonKeys) {
		return
	}
	reason := ""
	if key := rejectReasonKeys[n]; key != "" {
		reason = locale.BotText(c.t.buyerLang(id), key)
	}
	if _, err := c.t.orderService.RejectOrder(id, reason); err != nil {
		if service.SalesErrorCode(err) == service.ErrOrderState {
			c.send(c.tr("shopbot.admin.alreadyHandled", "Id=="+strconv.Itoa(id)), nil)
			return
		}
		logger.Warningf("tgbot shop: rejecting order %d: %v", id, err)
		c.send(c.tr("shopbot.admin.failed", "Id=="+strconv.Itoa(id)), nil)
		return
	}
	c.markDecision(query, id, c.tr("shopbot.admin.rejectedBy", "Name=="+esc(c.from.FirstName)))
}

func (c *shopCtx) restoreReceiptKeyboard(query *telego.CallbackQuery, parts []string) {
	if id, ok := orderIDOf(parts); ok {
		c.t.editReplyMarkup(c.chat, query.Message.GetMessageID(), receiptKeyboard(c, id))
	}
}

// buyerLang is the language an order's buyer reads; a web order has no
// Telegram account, so it falls back to the panel's bot language.
func (t *Tgbot) buyerLang(orderID int) string {
	if order, err := t.orderService.GetOrder(orderID); err == nil && order.TgId != 0 {
		return t.shopLangOf(order.TgId)
	}
	lang, _ := t.settingService.GetTgLang()
	return lang
}

// markDecision rewrites the admin message so the decision stays on record and
// the buttons stop working; an old or photo message is edited by caption.
func (c *shopCtx) markDecision(query *telego.CallbackQuery, orderID int, line string) {
	order, err := c.t.orderService.GetOrder(orderID)
	if err != nil {
		return
	}
	text := c.receiptText(order) + "\n\n" + line
	msg, _ := query.Message.(*telego.Message)
	if msg != nil && len(msg.Photo) > 0 && bot != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := bot.EditMessageCaption(ctx, &telego.EditMessageCaptionParams{
			ChatID: tu.ID(c.chat), MessageID: msg.MessageID, Caption: text, ParseMode: telego.ModeHTML, ReplyMarkup: &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{}},
		})
		if err != nil && !isTelegramNotModifiedError(err) {
			logger.Warning("tgbot shop: marking decision:", err)
		}
		return
	}
	empty := &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{}}
	if !c.t.tryEditMessage(c.chat, query.Message.GetMessageID(), text, empty) {
		c.send(text, nil)
	}
}

func (t *Tgbot) editReplyMarkup(chat int64, msgID int, kb *telego.InlineKeyboardMarkup) {
	if bot == nil || !t.IsRunning() {
		return
	}
	t.editMessageCallbackTgBot(chat, msgID, kb)
}

// --- events: tell customers what happened to their orders and tickets ---

// handleShopEvent consumes the shop's own events; it reports whether the event
// was one, so the admin notifier skips it.
func (t *Tgbot) handleShopEvent(e eventbus.Event) bool {
	switch e.Type {
	case eventbus.EventOrderReceipt, eventbus.EventOrderFulfilled, eventbus.EventOrderRejected,
		eventbus.EventOrderExpired, eventbus.EventOrderReminder:
		data, ok := e.Data.(*eventbus.OrderEventData)
		// Staff review receipts of the web shop too; only buyer messages need the bot shop on.
		if ok && (e.Type == eventbus.EventOrderReceipt || t.shopEnabled()) {
			t.onOrderEvent(e.Type, data)
		}
		return true
	case eventbus.EventTicketAnswered:
		if data, ok := e.Data.(*eventbus.TicketEventData); ok && t.shopEnabled() {
			t.onTicketAnswered(data)
		}
		return true
	}
	return false
}

// dmCtx speaks to a customer in their private chat, in their language.
func (t *Tgbot) dmCtx(tgID int64) (*shopCtx, bool) {
	cfg, err := t.salesConfig.Get()
	if err != nil || tgID == 0 {
		return nil, false
	}
	return &shopCtx{t: t, chat: tgID, from: &telego.User{ID: tgID}, actor: chatUser{chatID: tgID, userID: tgID}, lang: t.shopLangOf(tgID), cfg: cfg}, true
}

func (t *Tgbot) onOrderEvent(typ eventbus.EventType, data *eventbus.OrderEventData) {
	if typ == eventbus.EventOrderReceipt {
		t.notifyReceipt(data.OrderId)
		return
	}
	order, err := t.orderService.GetOrder(data.OrderId)
	if err != nil {
		return
	}
	c, ok := t.dmCtx(order.TgId)
	if !ok {
		return
	}
	id := strconv.Itoa(order.Id)
	view := keyboard(rowOf(btn(c.tr("shopbot.btn.openOrder"), "shop:o:"+id)))
	switch typ {
	case eventbus.EventOrderFulfilled:
		c.send(c.tr("shopbot.notify.fulfilled", "Id=="+id, "Item=="+esc(c.orderTitle(order))), nil)
		c.sendDelivery(order)
	case eventbus.EventOrderRejected:
		msg := c.tr("shopbot.notify.rejected", "Id=="+id)
		if order.RejectReason != "" {
			msg += c.tr("shopbot.order.reason", "Reason=="+esc(order.RejectReason))
		}
		c.send(msg, keyboard(rowOf(btn(c.tr("shopbot.btn.support"), "shop:sup"), btn(c.tr("shopbot.btn.openOrder"), "shop:o:"+id))))
	case eventbus.EventOrderExpired:
		c.send(c.tr("shopbot.notify.expired", "Id=="+id), keyboard(rowOf(btn(c.tr("shopbot.btn.buy"), "shop:buy"))))
	case eventbus.EventOrderReminder:
		c.send(c.tr("shopbot.notify.reminder", "Id=="+id, "Item=="+esc(c.orderTitle(order)), "Amount=="+c.formatAmount(order.Amount, order.Currency)), view)
	}
}

func (t *Tgbot) onTicketAnswered(data *eventbus.TicketEventData) {
	rec, err := t.clientService.GetRecordByEmail(nil, data.Email)
	if err != nil || rec.TgID == 0 {
		return
	}
	c, ok := t.dmCtx(rec.TgID)
	if !ok {
		return
	}
	id := strconv.Itoa(data.TicketId)
	c.send(c.tr("shopbot.notify.ticket", "Id=="+id, "Subject=="+esc(data.Subject), "Excerpt=="+esc(data.Excerpt)),
		keyboard(rowOf(btn(c.tr("shopbot.btn.openTicket"), "shop:tk:"+id))))
}
