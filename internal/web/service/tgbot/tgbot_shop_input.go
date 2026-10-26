package tgbot

import (
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
)

// handleShopInput consumes the message a customer was asked for and reports
// whether the state was a shop one.
func (t *Tgbot) handleShopInput(message *telego.Message, actor chatUser, state string) bool {
	if !strings.HasPrefix(state, "shop_") {
		return false
	}
	userStateMgr.clear(actor)
	if message.From == nil || !t.shopEnabled() {
		return true
	}
	c, ok := t.newShopCtx(message.Chat.ID, message.From, 0)
	if !ok {
		return true
	}
	text := strings.TrimSpace(message.Text)
	switch {
	case state == shopStateCode:
		c.acceptCode(text)
	case state == shopStateGB, state == shopStateDays:
		n, err := strconv.Atoi(text)
		if err != nil {
			c.t.setUserState(actor, state)
			c.send(c.tr("shopbot.custom.notNumber"), nil)
			break
		}
		if state == shopStateGB {
			c.acceptGB(n)
		} else {
			c.acceptDays(n)
		}
	case strings.HasPrefix(state, shopStateReceipt):
		c.submitReceipt(strings.TrimPrefix(state, shopStateReceipt), message)
	case strings.HasPrefix(state, shopStateTicketNew):
		c.openTicketFrom(strings.TrimPrefix(state, shopStateTicketNew), message.Text)
	case strings.HasPrefix(state, shopStateTicketReply):
		c.replyTicketFrom(strings.TrimPrefix(state, shopStateTicketReply), message.Text)
	case state == shopStateBan, state == shopStateUnban:
		if c.admin {
			c.banFrom(text, state == shopStateBan)
		}
	}
	return true
}

func (c *shopCtx) acceptCode(text string) {
	sel := getShopSelection(c.actor)
	if sel == nil {
		c.send(c.tr("shopbot.expired"), nil)
		return
	}
	sel.code = text
	if _, err := c.quoteFor(sel); err != nil {
		c.send(c.shopError(err), nil)
		return
	}
	setShopSelection(c.actor, sel)
	c.shopShowConfirm(sel)
}
