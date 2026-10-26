package tgbot

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/skip2/go-qrcode"
)

const dayMillis = int64(24 * time.Hour / time.Millisecond)

// serviceState names where a service stands now: active, disabled by the
// operator, out of quota, or past its date.
func serviceState(s service.ShopService, now time.Time) string {
	switch {
	case !s.Enable:
		return "disabled"
	case s.Total > 0 && s.Up+s.Down >= s.Total:
		return "depleted"
	case s.ExpiryTime > 0 && s.ExpiryTime <= now.UnixMilli():
		return "expired"
	}
	return "active"
}

// serviceCard is a service as the customer sees it: state, usage with a bar,
// and when it ends.
func (c *shopCtx) serviceCard(s service.ShopService) string {
	used := s.Up + s.Down
	usage := formatBytes(used) + " / " + c.tr("shopbot.unlimited")
	bar := ""
	if s.Total > 0 {
		usage = formatBytes(used) + " / " + formatBytes(s.Total)
		bar = "\n" + usageBar(used, s.Total) + " " + strconv.FormatInt(min(used*100/s.Total, 100), 10) + "%"
	}
	now := time.Now()
	expiry := c.tr("shopbot.noExpiry")
	switch {
	case s.ExpiryTime < 0:
		expiry = c.tr("shopbot.svc.startsOnUse", "Days=="+strconv.FormatInt(-s.ExpiryTime/dayMillis, 10))
	case s.ExpiryTime > 0:
		left := (s.ExpiryTime - now.UnixMilli() + dayMillis - 1) / dayMillis
		expiry = time.UnixMilli(s.ExpiryTime).Format("2006-01-02")
		if left > 0 {
			expiry += " (" + c.tr("shopbot.svc.daysLeft", "Days=="+strconv.FormatInt(left, 10)) + ")"
		}
	}
	return c.tr("shopbot.svc.card", "Email=="+esc(s.Email), "State=="+c.tr("shopbot.svc.state."+serviceState(s, now)),
		"Usage=="+usage+bar, "Expiry=="+expiry)
}

func (c *shopCtx) ownedServices() ([]service.ShopService, error) {
	return c.t.orderService.OwnedServices(c.from.ID, "")
}

func (c *shopCtx) shopShowServices(arg string) {
	services, err := c.ownedServices()
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	menu := rowOf(btn(c.tr("shopbot.btn.menu"), "shop:menu"))
	if len(services) == 0 {
		c.show(c.tr("shopbot.svc.none"), keyboard(rowOf(btn(c.tr("shopbot.btn.buy"), "shop:buy")), menu))
		return
	}
	if id, err := strconv.Atoi(arg); err == nil {
		for _, s := range services {
			if s.Id == id {
				c.showServiceDetail(s)
				return
			}
		}
		return
	}
	if len(services) == 1 {
		c.showServiceDetail(services[0])
		return
	}
	rows := make([][]telego.InlineKeyboardButton, 0, len(services)+1)
	now := time.Now()
	for _, s := range services {
		rows = append(rows, rowOf(btn("📦 "+s.Email+" · "+c.tr("shopbot.svc.state."+serviceState(s, now)), "shop:sv:"+strconv.Itoa(s.Id))))
	}
	c.show(c.tr("shopbot.svc.list"), keyboard(append(rows, menu)...))
}

func (c *shopCtx) showServiceDetail(s service.ShopService) {
	id := strconv.Itoa(s.Id)
	rows := [][]telego.InlineKeyboardButton{
		rowOf(btn(c.tr("shopbot.btn.link"), "shop:lk:"+id), btn(c.tr("shopbot.btn.qr"), "shop:qr:"+id)),
		rowOf(btn(c.tr("shopbot.btn.renewThis"), "shop:rn:"+id), btn(c.tr("shopbot.btn.addon"), "shop:ao:"+id)),
		rowOf(btn(c.tr("shopbot.btn.support"), "shop:sup:"+id), btn(c.tr("shopbot.btn.refresh"), "shop:sv:"+id)),
		rowOf(btn(c.tr("shopbot.btn.menu"), "shop:menu")),
	}
	c.show(c.serviceCard(s), keyboard(rows...))
}

// sendServiceLinks sends the subscription link of one of the customer's
// services, with the QR button.
func (c *shopCtx) sendServiceLinks(email string) {
	subURL, jsonURL, err := c.t.buildSubscriptionURLs(email)
	if err != nil {
		logger.Warning("tgbot shop: subscription link:", err)
		c.send(c.tr("shopbot.svc.noLink"), nil)
		return
	}
	msg := c.tr("shopbot.svc.link", "URL=="+esc(subURL))
	if jsonURL != "" {
		msg += c.tr("shopbot.svc.jsonLink", "URL=="+esc(jsonURL))
	}
	rows := [][]telego.InlineKeyboardButton{}
	if rec, err := c.t.clientService.GetRecordByEmail(nil, email); err == nil {
		rows = append(rows, rowOf(btn(c.tr("shopbot.btn.qr"), "shop:qr:"+strconv.Itoa(rec.Id))))
	}
	rows = append(rows, rowOf(btn(c.tr("shopbot.btn.services"), "shop:sv"), btn(c.tr("shopbot.btn.menu"), "shop:menu")))
	c.send(msg, keyboard(rows...))
}

func (c *shopCtx) shopSendLink(arg string) {
	rec, err := c.ownedRecord(arg)
	if err != nil {
		return
	}
	c.sendServiceLinks(rec.Email)
}

func (c *shopCtx) shopSendQR(arg string) {
	rec, err := c.ownedRecord(arg)
	if err != nil || bot == nil {
		return
	}
	subURL, _, err := c.t.buildSubscriptionURLs(rec.Email)
	if err != nil {
		c.send(c.tr("shopbot.svc.noLink"), nil)
		return
	}
	png, err := qrcode.Encode(subURL, qrcode.Medium, 512)
	if err != nil {
		logger.Warning("tgbot shop: qr:", err)
		return
	}
	params := &telego.SendPhotoParams{
		ChatID: tu.ID(c.chat), Photo: tu.FileFromBytes(png, "subscription.png"),
		Caption: c.tr("shopbot.svc.qrCaption", "Email=="+esc(rec.Email)), ParseMode: telego.ModeHTML,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := bot.SendPhoto(ctx, params); err != nil {
		logger.Warning("tgbot shop: sending qr:", err)
	}
}

// shopRenewService starts a purchase whose account is already chosen.
func (c *shopCtx) shopRenewService(arg string) {
	rec, err := c.ownedRecord(arg)
	if err != nil {
		return
	}
	c.shopStartBuying(&shopSelection{kind: model.OrderKindRenew, email: rec.Email})
}

func (c *shopCtx) shopAddonService(arg string) {
	rec, err := c.ownedRecord(arg)
	if err != nil {
		return
	}
	if _, err := c.customBounds(); err != nil {
		c.show(c.tr("shopbot.addon.off"), keyboard(rowOf(btn(c.tr("shopbot.btn.back"), "shop:sv:"+arg))))
		return
	}
	sel := &shopSelection{kind: model.OrderKindAddon, email: rec.Email, custom: true}
	setShopSelection(c.actor, sel)
	c.askGB(sel)
}

// --- wallet and referral ---

func (c *shopCtx) shopShowWallet() {
	menu := rowOf(btn(c.tr("shopbot.btn.menu"), "shop:menu"))
	services, err := c.ownedServices()
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	if len(services) == 0 || !c.cfg.WalletEnabled {
		c.show(c.tr("shopbot.wallet.none"), keyboard(menu))
		return
	}
	var b strings.Builder
	b.WriteString(c.tr("shopbot.wallet.title"))
	for _, s := range services {
		balances, err := c.t.wallets.Balances(s.Email)
		if err != nil {
			c.show(c.shopError(err), nil)
			return
		}
		line := c.tr("shopbot.wallet.empty")
		if len(balances) > 0 {
			parts := make([]string, len(balances))
			for i, bal := range balances {
				parts[i] = c.formatAmount(bal.Amount, bal.Currency)
			}
			line = strings.Join(parts, ", ")
		}
		b.WriteString("\n📦 <code>" + esc(s.Email) + "</code>: " + line)
	}
	b.WriteString(c.tr("shopbot.wallet.hint"))
	c.show(b.String(), keyboard(rowOf(btn(c.tr("shopbot.btn.invite"), "shop:ref")), menu))
}

func (c *shopCtx) shopShowReferral() {
	menu := rowOf(btn(c.tr("shopbot.btn.menu"), "shop:menu"))
	services, err := c.ownedServices()
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	if !c.cfg.Referral.Enabled {
		c.show(c.tr("shopbot.ref.off"), keyboard(menu))
		return
	}
	if len(services) == 0 {
		c.show(c.tr("shopbot.ref.needService"), keyboard(rowOf(btn(c.tr("shopbot.btn.buy"), "shop:buy")), menu))
		return
	}
	code, err := c.t.referrals.CodeFor(services[0].Email)
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	link := "https://t.me/" + botUsername() + "?start=ref_" + code
	c.show(c.tr("shopbot.ref.info", "Percent=="+strconv.Itoa(c.cfg.Referral.BonusPercent), "Code=="+code, "Link=="+esc(link)), keyboard(menu))
}
