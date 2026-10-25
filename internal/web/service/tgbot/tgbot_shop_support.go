package tgbot

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/mymmrac/telego"
)

const (
	supportTicketListSize = 5
	supportMessagesShown  = 6
	ticketSubjectRunes    = 60
)

func (c *shopCtx) ticketAuth(email string) service.TicketAuth {
	return service.TicketAuth{Email: email, TgId: c.from.ID}
}

// shopSupport is the entry to tickets. A ticket belongs to one of the
// customer's services, so with several the customer picks first.
func (c *shopCtx) shopSupport(arg string) {
	menu := rowOf(btn(c.tr("shopbot.btn.menu"), "shop:menu"))
	services, err := c.ownedServices()
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	if len(services) == 0 {
		c.show(c.tr("shopbot.support.noService"), keyboard(menu))
		return
	}
	chosen := services[0]
	if len(services) > 1 {
		id, convErr := strconv.Atoi(arg)
		found := false
		for _, s := range services {
			if convErr == nil && s.Id == id {
				chosen, found = s, true
			}
		}
		if !found {
			rows := make([][]telego.InlineKeyboardButton, 0, len(services)+1)
			for _, s := range services {
				rows = append(rows, rowOf(btn("📦 "+s.Email, "shop:sup:"+strconv.Itoa(s.Id))))
			}
			c.show(c.tr("shopbot.support.pick"), keyboard(append(rows, menu)...))
			return
		}
	}
	tickets, err := c.t.tickets.CustomerList(c.ticketAuth(chosen.Email))
	if err != nil {
		c.show(c.shopError(err), nil)
		return
	}
	rows := [][]telego.InlineKeyboardButton{rowOf(btn(c.tr("shopbot.btn.newTicket"), "shop:tn:"+strconv.Itoa(chosen.Id)))}
	for i, tk := range tickets {
		if i == supportTicketListSize {
			break
		}
		label := "#" + strconv.Itoa(tk.Id) + " " + tk.Subject + " - " + c.tr("shop.tickets.statuses."+tk.Status)
		rows = append(rows, rowOf(btn(label, "shop:tk:"+strconv.Itoa(tk.Id))))
	}
	c.show(c.tr("shopbot.support.title", "Email=="+esc(chosen.Email)), keyboard(append(rows, menu)...))
}

// ownTicket loads a ticket and the service it belongs to, refusing one that is
// not on an account bound to the sender.
func (c *shopCtx) ownTicket(arg string) (*service.TicketDetail, *model.ClientRecord, bool) {
	id, err := strconv.Atoi(arg)
	if err != nil {
		return nil, nil, false
	}
	detail, err := c.t.tickets.Get(id)
	if err != nil {
		return nil, nil, false
	}
	rec, err := c.t.clientService.GetRecordByEmail(nil, detail.Ticket.ClientEmail)
	if err != nil || rec.TgID != c.from.ID {
		return nil, nil, false
	}
	return detail, rec, true
}

func (c *shopCtx) shopShowTicket(arg string) {
	detail, rec, ok := c.ownTicket(arg)
	if !ok {
		c.show(c.shopError(&service.SalesError{Code: service.ErrTicketNotFound}), nil)
		return
	}
	var b strings.Builder
	b.WriteString(c.tr("shopbot.support.ticket", "Id=="+strconv.Itoa(detail.Ticket.Id), "Subject=="+esc(detail.Ticket.Subject),
		"Status=="+c.tr("shop.tickets.statuses."+detail.Ticket.Status)))
	msgs := detail.Messages
	if len(msgs) > supportMessagesShown {
		msgs = msgs[len(msgs)-supportMessagesShown:]
	}
	for _, m := range msgs {
		who := c.tr("shopbot.support.you")
		if m.Author == model.TicketByAdmin {
			who = c.tr("shopbot.support.staff")
		}
		b.WriteString("\n\n<b>" + who + "</b>\n" + esc(m.Body))
	}
	rows := [][]telego.InlineKeyboardButton{}
	if detail.Ticket.Status != model.TicketClosed {
		rows = append(rows, rowOf(btn(c.tr("shopbot.btn.reply"), "shop:tr:"+strconv.Itoa(detail.Ticket.Id))))
	}
	rows = append(rows, rowOf(btn(c.tr("shopbot.btn.back"), "shop:sup:"+strconv.Itoa(rec.Id)), btn(c.tr("shopbot.btn.menu"), "shop:menu")))
	c.show(b.String(), keyboard(rows...))
}

func (c *shopCtx) shopAskNewTicket(arg string) {
	rec, err := c.ownedRecord(arg)
	if err != nil {
		return
	}
	c.t.setUserState(c.actor, shopStateTicketNew+strconv.Itoa(rec.Id))
	c.send(c.tr("shopbot.support.askNew"), nil)
}

func (c *shopCtx) shopAskTicketReply(arg string) {
	detail, _, ok := c.ownTicket(arg)
	if !ok {
		return
	}
	c.t.setUserState(c.actor, shopStateTicketReply+strconv.Itoa(detail.Ticket.Id))
	c.send(c.tr("shopbot.support.askReply"), nil)
}

// subjectOf turns the first line of a new ticket into its subject.
func subjectOf(text string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	first = strings.TrimSpace(first)
	if utf8.RuneCountInString(first) > ticketSubjectRunes {
		first = string([]rune(first)[:ticketSubjectRunes])
	}
	return first
}

func (c *shopCtx) openTicketFrom(recID, text string) {
	rec, err := c.ownedRecord(recID)
	if err != nil {
		c.send(c.shopError(&service.SalesError{Code: service.ErrEmailUnknown}), nil)
		return
	}
	detail, err := c.t.tickets.Open(c.ticketAuth(rec.Email), subjectOf(text), text)
	if err != nil {
		c.send(c.shopError(err), nil)
		return
	}
	c.send(c.tr("shopbot.support.sent", "Id=="+strconv.Itoa(detail.Ticket.Id)),
		keyboard(rowOf(btn(c.tr("shopbot.btn.openTicket"), "shop:tk:"+strconv.Itoa(detail.Ticket.Id)))))
}

func (c *shopCtx) replyTicketFrom(ticketID, text string) {
	detail, _, ok := c.ownTicket(ticketID)
	if !ok {
		c.send(c.shopError(&service.SalesError{Code: service.ErrTicketNotFound}), nil)
		return
	}
	if _, err := c.t.tickets.CustomerReply(c.ticketAuth(detail.Ticket.ClientEmail), detail.Ticket.Id, text); err != nil {
		c.send(c.shopError(err), nil)
		return
	}
	c.send(c.tr("shopbot.support.replied"), keyboard(rowOf(btn(c.tr("shopbot.btn.openTicket"), "shop:tk:"+ticketID))))
}
