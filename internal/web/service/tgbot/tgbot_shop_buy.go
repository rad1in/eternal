package tgbot

import (
	"errors"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/websocket"

	"github.com/mymmrac/telego"
)

const shopOrderListSize = 6

var shopGatewayButtons = map[string]string{
	service.GatewayCard: "card", service.GatewayWallet: "wallet", service.GatewayZarinpal: "zarinpal",
	service.GatewayIDPay: "idpay", service.GatewayCrypto: "crypto", service.GatewayStripe: "stripe",
	service.GatewayPayPal: "paypal",
}

func formatBytes(n int64) string { return common.FormatTraffic(n) }

type planGroup struct {
	name  string
	plans []service.ShopPlan
}

// groupPlans keeps the catalog order: a category appears where its first plan
// does, and plans without one form their own group.
func groupPlans(plans []service.ShopPlan) []planGroup {
	var groups []planGroup
	for _, p := range plans {
		i := 0
		for i < len(groups) && groups[i].name != p.Category {
			i++
		}
		if i == len(groups) {
			groups = append(groups, planGroup{name: p.Category})
		}
		groups[i].plans = append(groups[i].plans, p)
	}
	return groups
}

func (c *shopCtx) planButton(p service.ShopPlan) []telego.InlineKeyboardButton {
	return rowOf(btn(p.Name+" - "+c.formatAmount(p.Price, p.Currency), "shop:p:"+strconv.Itoa(p.Id)))
}

// shopStartBuying lists what is on sale. Categories come first when there is
// more than one group; the calculator is always the last entry when it is on.
func (c *shopCtx) shopStartBuying(sel *shopSelection) {
	setShopSelection(c.actor, sel)
	catalog, err := c.t.orderService.Catalog()
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	if len(catalog.Plans) == 0 && catalog.Custom == nil {
		c.show(c.tr("shopbot.plans.none"), keyboard(rowOf(btn(c.tr("shopbot.btn.menu"), "shop:menu"))))
		return
	}
	groups := groupPlans(catalog.Plans)
	rows := [][]telego.InlineKeyboardButton{}
	if len(groups) > 1 {
		for i, g := range groups {
			name := g.name
			if name == "" {
				name = c.tr("shopbot.plans.misc")
			}
			rows = append(rows, rowOf(btn("📁 "+name, "shop:cat:"+strconv.Itoa(i))))
		}
	} else if len(groups) == 1 {
		for _, p := range groups[0].plans {
			rows = append(rows, c.planButton(p))
		}
	}
	if catalog.Custom != nil {
		rows = append(rows, rowOf(btn(c.tr("shopbot.btn.custom"), "shop:cu")))
	}
	rows = append(rows, rowOf(btn(c.tr("shopbot.btn.menu"), "shop:menu")))
	c.show(c.tr("shopbot.plans.title"), keyboard(rows...))
}

func (c *shopCtx) shopShowCategory(arg string) {
	idx, err := strconv.Atoi(arg)
	catalog, cerr := c.t.orderService.Catalog()
	if err != nil || cerr != nil {
		return
	}
	groups := groupPlans(catalog.Plans)
	if idx < 0 || idx >= len(groups) {
		return
	}
	rows := [][]telego.InlineKeyboardButton{}
	for _, p := range groups[idx].plans {
		rows = append(rows, c.planButton(p))
	}
	rows = append(rows, rowOf(btn(c.tr("shopbot.btn.back"), "shop:buy")))
	c.show(c.tr("shopbot.plans.title"), keyboard(rows...))
}

func (c *shopCtx) shopPlan(id int) (*service.ShopPlan, error) {
	catalog, err := c.t.orderService.Catalog()
	if err != nil {
		return nil, err
	}
	for i := range catalog.Plans {
		if catalog.Plans[i].Id == id {
			return &catalog.Plans[i], nil
		}
	}
	return nil, &service.SalesError{Code: service.ErrPlanUnavailable}
}

func (c *shopCtx) planDetail(p *service.ShopPlan) string {
	quota := c.tr("shopbot.unlimited")
	if p.TotalGB > 0 {
		quota = formatBytes(p.TotalGB)
	}
	duration := c.tr("shopbot.noExpiry")
	if p.DurationDays > 0 {
		duration = strconv.Itoa(p.DurationDays) + " " + c.tr("tgbot.days")
	}
	desc := ""
	if p.Description != "" {
		desc = esc(p.Description) + "\n"
	}
	return c.tr("shopbot.plans.detail", "Name=="+esc(p.Name), "Description=="+desc, "Quota=="+quota,
		"Duration=="+duration, "Devices=="+strconv.Itoa(p.LimitIP), "Price=="+c.formatAmount(p.Price, p.Currency))
}

func (c *shopCtx) shopChoosePlan(arg string) {
	id, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	plan, err := c.shopPlan(id)
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	sel := getShopSelection(c.actor)
	if sel == nil {
		sel = &shopSelection{}
	}
	sel.planID, sel.custom, sel.gb, sel.days, sel.code = id, false, 0, 0, ""
	if sel.kind == model.OrderKindRenew && sel.email != "" {
		setShopSelection(c.actor, sel)
		c.shopShowConfirm(sel)
		return
	}
	sel.kind, sel.email = "", ""
	setShopSelection(c.actor, sel)
	rows := [][]telego.InlineKeyboardButton{rowOf(btn(c.tr("shopbot.btn.newAccount"), "shop:k:n"))}
	records, _ := c.t.clientService.GetRecordsByTgID(c.from.ID)
	for _, rec := range records {
		rows = append(rows, rowOf(btn(c.tr("shopbot.btn.renew", "Email=="+rec.Email), "shop:k:r"+strconv.Itoa(rec.Id))))
	}
	rows = append(rows, rowOf(btn(c.tr("shopbot.btn.back"), "shop:buy")))
	c.show(c.planDetail(plan), keyboard(rows...))
}

// shopChooseAccount fixes who the plan is for: a renewal must name one of the
// tapping account's own clients; a new purchase gets a generated username.
func (c *shopCtx) shopChooseAccount(arg string) {
	sel := getShopSelection(c.actor)
	if sel == nil {
		c.show(c.tr("shopbot.expired"), nil)
		return
	}
	if arg == "n" {
		name, err := c.t.orderService.TelegramUsername(c.from.ID)
		if err != nil {
			c.show(c.shopError(err), nil)
			return
		}
		sel.kind, sel.email = model.OrderKindNew, name
	} else {
		rec, err := c.ownedRecord(strings.TrimPrefix(arg, "r"))
		if err != nil {
			c.show(c.shopError(&service.SalesError{Code: service.ErrEmailUnknown}), nil)
			return
		}
		sel.kind, sel.email = model.OrderKindRenew, rec.Email
	}
	sel.code = ""
	setShopSelection(c.actor, sel)
	c.shopShowConfirm(sel)
}

// ownedRecord loads a client by record id and refuses one bound to another
// Telegram account.
func (c *shopCtx) ownedRecord(idText string) (*model.ClientRecord, error) {
	id, err := strconv.Atoi(idText)
	if err != nil {
		return nil, err
	}
	rec, err := c.t.clientService.GetByID(id)
	if err != nil {
		return nil, err
	}
	if rec.TgID != c.from.ID {
		return nil, errors.New("not the sender's client")
	}
	return rec, nil
}

// itemName is what the customer is buying, in words.
func (c *shopCtx) itemName(sel *shopSelection) string {
	if sel.custom {
		return c.customItem(sel.kind, sel.gb, sel.days)
	}
	if plan, err := c.shopPlan(sel.planID); err == nil {
		return esc(plan.Name)
	}
	return ""
}

func (c *shopCtx) customItem(kind string, gb, days int) string {
	parts := []string{}
	if gb > 0 {
		parts = append(parts, strconv.Itoa(gb)+" GB")
	}
	if days > 0 {
		parts = append(parts, strconv.Itoa(days)+" "+c.tr("tgbot.days"))
	}
	key := "shopbot.custom.item"
	if kind == model.OrderKindAddon {
		key = "shopbot.custom.addonItem"
	}
	return c.tr(key, "Parts=="+strings.Join(parts, " + "))
}

func (c *shopCtx) kindLabel(kind string) string {
	switch kind {
	case model.OrderKindRenew:
		return c.tr("shopbot.kind.renew")
	case model.OrderKindAddon:
		return c.tr("shopbot.kind.addon")
	default:
		return c.tr("shopbot.kind.new")
	}
}

func (c *shopCtx) quoteFor(sel *shopSelection) (*service.Quote, error) {
	return c.t.orderService.Quote(service.QuoteInput{
		PlanId: sel.planID, Kind: sel.kind, CustomGB: sel.gb, CustomDays: sel.days, DiscountCode: sel.code, Email: sel.email,
	})
}

func (c *shopCtx) shopShowConfirm(sel *shopSelection) {
	quote, err := c.quoteFor(sel)
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	catalog, err := c.t.orderService.Catalog()
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	discount := ""
	if quote.Discount > 0 {
		discount = c.tr("shopbot.discountLine", "List=="+c.formatAmount(quote.List, quote.Currency), "Code=="+esc(quote.DiscountCode))
	}
	msg := c.tr("shopbot.confirm", "Item=="+c.itemName(sel), "Kind=="+c.kindLabel(sel.kind), "Email=="+esc(sel.email),
		"Amount=="+c.formatAmount(quote.Final, quote.Currency), "Discount=="+discount)
	rows := [][]telego.InlineKeyboardButton{}
	for _, gw := range catalog.Methods {
		if gw == service.GatewayWallet && sel.kind == model.OrderKindNew {
			continue
		}
		rows = append(rows, rowOf(btn(c.tr("shopbot.gw."+shopGatewayButtons[gw]), "shop:g:"+gw)))
	}
	if len(rows) == 0 {
		c.show(c.shopError(&service.SalesError{Code: service.ErrGatewayUnavailable}), nil)
		return
	}
	rows = append(rows, rowOf(btn(c.tr("shopbot.btn.code"), "shop:code")), rowOf(btn(c.tr("shopbot.btn.cancel"), "shop:menu")))
	c.show(msg, keyboard(rows...))
}

// --- calculator ---

func (c *shopCtx) customBounds() (*service.ShopCustom, error) {
	catalog, err := c.t.orderService.Catalog()
	if err != nil {
		return nil, err
	}
	if catalog.Custom == nil {
		return nil, &service.SalesError{Code: service.ErrPlanUnavailable}
	}
	return catalog.Custom, nil
}

// shopStartCalculator begins a calculator order; a renewal or add-on already
// chose its account, a new one gets a generated username.
func (c *shopCtx) shopStartCalculator() {
	sel := getShopSelection(c.actor)
	if sel == nil {
		sel = &shopSelection{}
	}
	if sel.kind == "" || sel.kind == model.OrderKindNew {
		name, err := c.t.orderService.TelegramUsername(c.from.ID)
		if err != nil {
			c.show(c.shopError(err), nil)
			return
		}
		sel.kind, sel.email = model.OrderKindNew, name
	}
	sel.custom, sel.planID, sel.gb, sel.days, sel.code = true, 0, 0, 0, ""
	setShopSelection(c.actor, sel)
	c.askGB(sel)
}

var (
	gbPicks   = []int{10, 20, 50, 100, 200, 500}
	dayPicks  = []int{7, 30, 60, 90, 180, 365}
	cancelRow = func(c *shopCtx) []telego.InlineKeyboardButton {
		return rowOf(btn(c.tr("shopbot.btn.cancel"), "shop:menu"))
	}
)

// quickPicks keeps the preset values inside the bounds, or the bounds
// themselves when no preset fits.
func quickPicks(presets []int, lo, hi int) []int {
	var out []int
	for _, v := range presets {
		if v >= lo && v <= hi {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		out = []int{lo}
		if hi != lo {
			out = append(out, hi)
		}
	}
	return out
}

func (c *shopCtx) pickRows(prefix string, values []int, unit string) [][]telego.InlineKeyboardButton {
	var rows [][]telego.InlineKeyboardButton
	var row []telego.InlineKeyboardButton
	for _, v := range values {
		row = append(row, btn(strconv.Itoa(v)+" "+unit, prefix+strconv.Itoa(v)))
		if len(row) == 3 {
			rows = append(rows, rowOf(row...))
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, rowOf(row...))
	}
	return rows
}

func (c *shopCtx) askGB(sel *shopSelection) {
	b, err := c.customBounds()
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	rows := c.pickRows("shop:cg:", quickPicks(gbPicks, b.MinGB, b.MaxGB), "GB")
	if sel.kind == model.OrderKindAddon {
		rows = append(rows, rowOf(btn(c.tr("shopbot.btn.skip"), "shop:cg:0")))
	}
	rows = append(rows, cancelRow(c))
	c.t.setUserState(c.actor, shopStateGB)
	c.show(c.tr("shopbot.custom.askGB", "Min=="+strconv.Itoa(b.MinGB), "Max=="+strconv.Itoa(b.MaxGB),
		"Price=="+c.formatAmount(b.PricePerGB, b.Currency)), keyboard(rows...))
}

func (c *shopCtx) askDays(sel *shopSelection) {
	b, err := c.customBounds()
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	rows := c.pickRows("shop:cd:", quickPicks(dayPicks, b.MinDays, b.MaxDays), c.tr("tgbot.days"))
	if sel.kind == model.OrderKindAddon && sel.gb > 0 {
		rows = append(rows, rowOf(btn(c.tr("shopbot.btn.skip"), "shop:cd:0")))
	}
	rows = append(rows, cancelRow(c))
	c.t.setUserState(c.actor, shopStateDays)
	c.show(c.tr("shopbot.custom.askDays", "Min=="+strconv.Itoa(b.MinDays), "Max=="+strconv.Itoa(b.MaxDays),
		"Price=="+c.formatAmount(b.PricePerDay, b.Currency)), keyboard(rows...))
}

func (t *Tgbot) setUserState(actor chatUser, state string) { userStateMgr.set(actor, state) }

// acceptGB stores the chosen volume. A value outside the bounds keeps the
// question open.
func (c *shopCtx) acceptGB(n int) {
	sel := getShopSelection(c.actor)
	b, err := c.customBounds()
	if sel == nil || !sel.custom || err != nil {
		c.send(c.tr("shopbot.expired"), nil)
		return
	}
	if !(n == 0 && sel.kind == model.OrderKindAddon) && (n < b.MinGB || n > b.MaxGB) {
		c.t.setUserState(c.actor, shopStateGB)
		c.send(c.tr("shopbot.custom.badGB", "Min=="+strconv.Itoa(b.MinGB), "Max=="+strconv.Itoa(b.MaxGB)), nil)
		return
	}
	sel.gb = n
	setShopSelection(c.actor, sel)
	c.askDays(sel)
}

func (c *shopCtx) acceptDays(n int) {
	sel := getShopSelection(c.actor)
	b, err := c.customBounds()
	if sel == nil || !sel.custom || err != nil {
		c.send(c.tr("shopbot.expired"), nil)
		return
	}
	if !(n == 0 && sel.kind == model.OrderKindAddon && sel.gb > 0) && (n < b.MinDays || n > b.MaxDays) {
		c.t.setUserState(c.actor, shopStateDays)
		c.send(c.tr("shopbot.custom.badDays", "Min=="+strconv.Itoa(b.MinDays), "Max=="+strconv.Itoa(b.MaxDays)), nil)
		return
	}
	sel.days = n
	setShopSelection(c.actor, sel)
	userStateMgr.clear(c.actor)
	c.shopShowConfirm(sel)
}

func (c *shopCtx) shopPickGB(arg string) {
	if n, err := strconv.Atoi(arg); err == nil {
		c.acceptGB(n)
	}
}

func (c *shopCtx) shopPickDays(arg string) {
	if n, err := strconv.Atoi(arg); err == nil {
		c.acceptDays(n)
	}
}

// --- discount code, checkout ---

func (c *shopCtx) shopAskCode() {
	if getShopSelection(c.actor) == nil {
		c.show(c.tr("shopbot.expired"), nil)
		return
	}
	c.t.setUserState(c.actor, shopStateCode)
	c.send(c.tr("shopbot.askCode"), nil)
}

// referralCodeFor is the code the buyer arrived with, kept only while it still
// names someone else's account: the order service refuses a self-referral.
func (c *shopCtx) referralCodeFor(sel *shopSelection) string {
	if !c.cfg.Referral.Enabled || sel.kind != model.OrderKindNew {
		return ""
	}
	u, err := c.t.botUsers.Get(c.from.ID)
	if err != nil || u.ReferralCode == "" {
		return ""
	}
	email, err := c.t.referrals.Referrer(u.ReferralCode)
	if err != nil {
		return ""
	}
	if rec, err := c.t.clientService.GetRecordByEmail(nil, email); err != nil || rec.TgID == c.from.ID {
		return ""
	}
	return u.ReferralCode
}

func (c *shopCtx) shopCheckout(gateway string) {
	sel := getShopSelection(c.actor)
	if sel == nil || sel.kind == "" {
		c.show(c.tr("shopbot.expired"), nil)
		return
	}
	order, needRestart, err := c.t.orderService.Checkout(&c.t.inboundService, service.CreateOrderInput{
		PlanId: sel.planID, CustomGB: sel.gb, CustomDays: sel.days, Kind: sel.kind, Email: sel.email, Gateway: gateway,
		DiscountCode: sel.code, ReferralCode: c.referralCodeFor(sel), TgId: c.from.ID,
	})
	if needRestart {
		c.t.xrayService.SetToNeedRestart()
	}
	if err != nil {
		c.show(c.shopError(err), keyboard(rowOf(btn(c.tr("shopbot.btn.menu"), "shop:menu"))))
		return
	}
	if order.FulfilledAt > 0 {
		websocket.BroadcastInvalidate(websocket.MessageTypeClients)
	}
	clearShopSelection(c.actor)
	c.sendOrder(order)
}

// --- orders ---

func (c *shopCtx) shopShowOrders() {
	orders, err := c.t.orderService.TelegramOrders(c.from.ID, shopOrderListSize)
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	menu := rowOf(btn(c.tr("shopbot.btn.menu"), "shop:menu"))
	if len(orders) == 0 {
		c.show(c.tr("shopbot.orders.none"), keyboard(menu))
		return
	}
	rows := make([][]telego.InlineKeyboardButton, 0, len(orders)+1)
	for _, o := range orders {
		label := "#" + strconv.Itoa(o.Id) + " " + c.orderTitle(&o) + " - " + c.tr("shop.statuses."+o.Status)
		rows = append(rows, rowOf(btn(label, "shop:o:"+strconv.Itoa(o.Id))))
	}
	c.show(c.tr("shopbot.orders.title"), keyboard(append(rows, menu)...))
}

// orderTitle names an order's item; a calculator order is described from its
// terms, in the customer's language, rather than from the stored name.
func (c *shopCtx) orderTitle(o *model.Order) string {
	if o.PlanId == 0 {
		gb := int(o.TotalGB >> 30)
		return plainText(c.customItem(o.Kind, gb, o.DurationDays))
	}
	return o.PlanName
}

func plainText(s string) string { return strings.NewReplacer("<b>", "", "</b>", "").Replace(s) }

func (c *shopCtx) shopShowOrder(arg string) {
	id, err := strconv.Atoi(arg)
	if err != nil {
		return
	}
	order, err := c.t.orderService.TelegramOrder(c.from.ID, id)
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	c.sendOrder(order)
}

// sendOrder shows an order the way the shop page does: card details or a
// payment link while payment is due, the service once it is delivered.
func (c *shopCtx) sendOrder(order *model.Order) {
	view, err := c.t.orderService.PublicView(order, "")
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	msg := c.tr("shopbot.order.head", "Id=="+strconv.Itoa(order.Id), "Item=="+esc(c.orderTitle(order)), "Kind=="+c.kindLabel(order.Kind),
		"Amount=="+c.formatAmount(order.Amount, order.Currency), "Status=="+c.tr("shop.statuses."+order.Status))
	rows := [][]telego.InlineKeyboardButton{}
	if view.Card != nil {
		msg += c.tr("shopbot.order.card", "Amount=="+c.formatAmount(order.Amount, order.Currency), "Card=="+esc(view.Card.CardNumber),
			"Holder=="+esc(view.Card.HolderName), "Bank=="+esc(view.Card.BankName), "Instructions=="+esc(view.Card.Instructions))
		if order.Status == model.OrderStatusPending || order.Status == model.OrderStatusReview {
			rows = append(rows, rowOf(btn(c.tr("shopbot.btn.receipt"), "shop:rc:"+strconv.Itoa(order.Id))))
		}
	}
	if view.PayURL != "" {
		rows = append(rows, rowOf(urlBtn(c.tr("shopbot.btn.pay"), view.PayURL)))
	}
	if order.Status == model.OrderStatusRejected && order.RejectReason != "" {
		msg += c.tr("shopbot.order.reason", "Reason=="+esc(order.RejectReason))
	}
	rows = append(rows, rowOf(btn(c.tr("shopbot.btn.refresh"), "shop:o:"+strconv.Itoa(order.Id)), btn(c.tr("shopbot.btn.menu"), "shop:menu")))
	c.show(msg, keyboard(rows...))
	if view.Fulfilled {
		c.sendDelivery(order)
	}
}

// sendDelivery is the message a delivered order ends with: the service it
// now gives and, for a new one, how to connect.
func (c *shopCtx) sendDelivery(order *model.Order) {
	c.send(c.tr("shopbot.delivered."+order.Kind), nil)
	c.sendServiceLinks(order.ClientEmail)
}
