package tgbot

import (
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"

	"github.com/mymmrac/telego"
)

const adminReceiptListSize = 8

func (c *shopCtx) adminRoute(parts []string) {
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	switch sub {
	case "st":
		c.adminStats()
	case "rv":
		c.adminReceipts()
	case "ro":
		if len(parts) > 2 {
			c.adminResendReceipt(parts[2])
		}
	case "ban":
		c.t.setUserState(c.actor, shopStateBan)
		c.send(c.tr("shopbot.admin.askBan"), nil)
	case "unban":
		c.t.setUserState(c.actor, shopStateUnban)
		c.send(c.tr("shopbot.admin.askUnban"), nil)
	default:
		c.adminMenu()
	}
}

func (c *shopCtx) adminMenu() {
	c.show(c.tr("shopbot.admin.title"), keyboard(
		rowOf(btn(c.tr("shopbot.btn.today"), "shop:ad:st"), btn(c.tr("shopbot.btn.waiting"), "shop:ad:rv")),
		rowOf(btn(c.tr("shopbot.btn.ban"), "shop:ad:ban"), btn(c.tr("shopbot.btn.unban"), "shop:ad:unban")),
		rowOf(btn(c.tr("shopbot.btn.menu"), "shop:menu")),
	))
}

func (c *shopCtx) adminStats() {
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	sum, err := c.t.orderService.SalesSummary(midnight)
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	paid := c.tr("shopbot.admin.noSales")
	if len(sum.Paid) > 0 {
		lines := make([]string, len(sum.Paid))
		for i, d := range sum.Paid {
			lines[i] = c.tr("shopbot.admin.salesLine", "Orders=="+strconv.Itoa(d.Orders), "Amount=="+c.formatAmount(d.Amount, d.Currency))
		}
		paid = strings.Join(lines, "\n")
	}
	c.show(c.tr("shopbot.admin.stats", "Paid=="+paid, "Pending=="+strconv.FormatInt(sum.Pending, 10), "Review=="+strconv.FormatInt(sum.Review, 10)),
		keyboard(rowOf(btn(c.tr("shopbot.btn.waiting"), "shop:ad:rv")), rowOf(btn(c.tr("shopbot.btn.back"), "shop:ad"))))
}

func (c *shopCtx) adminReceipts() {
	orders, err := c.t.orderService.OrdersInReview(adminReceiptListSize)
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	back := rowOf(btn(c.tr("shopbot.btn.back"), "shop:ad"))
	if len(orders) == 0 {
		c.show(c.tr("shopbot.admin.noneWaiting"), keyboard(back))
		return
	}
	rows := make([][]telego.InlineKeyboardButton, 0, len(orders)+1)
	for i := range orders {
		o := &orders[i]
		rows = append(rows, rowOf(btn("#"+strconv.Itoa(o.Id)+" "+c.orderTitle(o)+" - "+c.formatAmount(o.Amount, o.Currency), "shop:ad:ro:"+strconv.Itoa(o.Id))))
	}
	c.show(c.tr("shopbot.admin.waitingTitle"), keyboard(append(rows, back)...))
}

func (c *shopCtx) adminResendReceipt(idText string) {
	id, err := strconv.Atoi(idText)
	if err != nil {
		return
	}
	order, err := c.t.orderService.GetOrder(id)
	if err != nil {
		c.send(c.shopError(err), nil)
		return
	}
	c.t.sendReceiptTo(c.chat, c.cfg, order)
}

// banFrom applies the Telegram id an admin typed to the ban list.
func (c *shopCtx) banFrom(text string, banned bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil || id <= 0 {
		c.send(c.tr("shopbot.admin.badID"), nil)
		return
	}
	if banned && checkAdmin(id) {
		c.send(c.tr("shopbot.admin.cannotBanAdmin"), nil)
		return
	}
	if err := c.t.botUsers.SetBanned(id, banned); err != nil {
		logger.Warning("tgbot shop: ban list:", err)
		c.send(c.tr("shop.errors.network"), nil)
		return
	}
	key := "shopbot.admin.unbanned"
	if banned {
		key = "shopbot.admin.banned"
	}
	c.send(c.tr(key, "ID=="+strconv.FormatInt(id, 10)), keyboard(rowOf(btn(c.tr("shopbot.btn.back"), "shop:ad"))))
}
