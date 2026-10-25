package tgbot

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"

	"github.com/mymmrac/telego"
)

func TestShopCommandAndCallbacksStaySilentForStrangersWhileOff(t *testing.T) {
	b := newShopBot(t)
	b.off(t)

	if _, ok := b.tb.gateCommand(commandFrom(777, "/shop")); ok {
		t.Error("a stranger's /shop was admitted while the shop is off")
	}
	if _, ok := b.tb.gateCallback(&telego.CallbackQuery{ID: "q", From: telego.User{ID: 777}, Data: "shop:buy"}); ok {
		t.Error("a stranger's shop button was admitted while the shop is off")
	}
	b.configure(t)
	if _, ok := b.tb.gateCommand(commandFrom(777, "/shop")); !ok {
		t.Error("a stranger's /shop was refused while the shop is on")
	}
	if _, ok := b.tb.gateCallback(&telego.CallbackQuery{ID: "q", From: telego.User{ID: 777}, Data: "shop:buy"}); !ok {
		t.Error("a stranger's shop button was refused while the shop is on")
	}
	if _, ok := b.tb.gateCallback(&telego.CallbackQuery{ID: "q", From: telego.User{ID: 777}, Data: "client_sub_links x"}); ok {
		t.Error("turning the shop on opened a non-shop button to strangers")
	}
}

func TestStartShowsTheMenuToStrangersOnlyWhileTheShopIsOn(t *testing.T) {
	b := newShopBot(t)
	b.off(t)
	b.tb.answerCommand(commandFrom(777, "/start"), 777, false)
	if got := b.api.last().text(); !strings.Contains(got, "777") {
		t.Fatalf("shop off: /start = %q, want the old reply that shows the chat id", got)
	}

	b.configure(t)
	b.tb.answerCommand(commandFrom(777, "/start"), 777, false)
	got := b.api.last()
	if !strings.Contains(got.text(), "Hello") {
		t.Fatalf("shop on: /start = %q, want the shop menu", got.text())
	}
	want := []string{"🛒 Buy|shop:buy", "📦 My services|shop:sv", "👛 Wallet|shop:wallet", "🎁 Invite friends|shop:ref", "🆘 Support|shop:sup", "🧾 My orders|shop:orders", "🌐 Language|shop:lang"}
	if buttons := got.buttons(); strings.Join(buttons, ",") != strings.Join(want, ",") {
		t.Fatalf("menu buttons = %v, want %v", buttons, want)
	}
}

func TestStrangerBuysByCardWithAPhotoReceiptAndAnAdminApprovesIt(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	id := strconv.Itoa(b.planID)

	b.tap(777, "shop:buy")
	if got := b.api.last(); !strings.Contains(got.text(), "Choose a plan") || got.buttons()[0] != "Monthly - 250,000 Toman|shop:p:"+id {
		t.Fatalf("plans = %q %v", got.text(), got.buttons())
	}
	b.tap(777, "shop:p:"+id)
	if got := b.api.last().text(); !strings.Contains(got, "<b>Monthly</b>") || !strings.Contains(got, "250,000 Toman") {
		t.Fatalf("plan detail = %q", got)
	}
	b.tap(777, "shop:k:n")
	if got := b.api.last().text(); !strings.Contains(got, "tg777") || !strings.Contains(got, "250,000 Toman") {
		t.Fatalf("confirm = %q", got)
	}
	b.tap(777, "shop:g:card")
	got := b.api.last()
	if !strings.Contains(got.text(), "6037991812345678") || !strings.Contains(got.text(), "Awaiting payment") {
		t.Fatalf("card order = %q, want the card details and the pending status", got.text())
	}
	orders := b.orders(777)
	if len(orders) != 1 {
		t.Fatalf("orders of 777 = %d, want 1", len(orders))
	}
	orderID := strconv.Itoa(orders[0].Id)

	b.tap(777, "shop:rc:"+orderID)
	if st, _ := userStateMgr.get(chatUser{chatID: 777, userID: 777}); st != shopStateReceipt+orderID {
		t.Fatalf("state = %q, want a pending receipt", st)
	}
	if !b.sayMessage(777, &telego.Message{Caption: "paid 14:20", Photo: []telego.PhotoSize{{FileID: "small"}, {FileID: "big-file-id"}}}) {
		t.Fatal("the photo receipt was not consumed")
	}
	order := b.order(t, orders[0].Id)
	if order.Status != model.OrderStatusReview || order.ReceiptFileId != "big-file-id" || order.ReceiptNote != "paid 14:20" {
		t.Fatalf("order = %s file %q note %q, want review with the largest photo", order.Status, order.ReceiptFileId, order.ReceiptNote)
	}

	mark := b.api.mark()
	b.tb.handleShopEvent(eventbus.Event{Type: eventbus.EventOrderReceipt, Data: &eventbus.OrderEventData{OrderId: orders[0].Id, TgId: 777}})
	photos := b.api.since(mark, "sendPhoto")
	if len(photos) != 1 || photos[0].chat() != 1 || photos[0].body["photo"] != "big-file-id" {
		t.Fatalf("admin notification = %+v, want the receipt photo sent to admin 1", photos)
	}
	if want := []string{"✅ Approve|shop:ra:" + orderID, "❌ Reject|shop:rj:" + orderID}; strings.Join(photos[0].buttons(), ",") != strings.Join(want, ",") {
		t.Fatalf("admin buttons = %v, want %v", photos[0].buttons(), want)
	}
	if !strings.Contains(photos[0].text(), "tg://user?id=777") || !strings.Contains(photos[0].text(), "250,000 Toman") {
		t.Fatalf("admin caption = %q", photos[0].text())
	}

	b.tapIn(1, "shop:ra:"+orderID, 1, telego.ChatTypePrivate)
	after := b.order(t, orders[0].Id)
	if after.Status != model.OrderStatusPaid || after.FulfilledAt == 0 {
		t.Fatalf("order after approval = %s fulfilled %d, want delivered", after.Status, after.FulfilledAt)
	}
	if recs, _ := b.tb.clientService.GetRecordsByTgID(777); len(recs) != 1 {
		t.Fatalf("clients bound to 777 = %d, want the delivered one", len(recs))
	}
	mark = b.api.mark()
	b.tb.handleShopEvent(eventbus.Event{Type: eventbus.EventOrderFulfilled, Data: &eventbus.OrderEventData{OrderId: orders[0].Id, TgId: 777}})
	dm := b.api.since(mark, "sendMessage")
	if len(dm) < 2 || dm[0].chat() != 777 || !strings.Contains(dm[0].text(), "Payment confirmed for order #"+orderID) {
		t.Fatalf("buyer DMs = %v, want the confirmation then the service", texts(dm))
	}
	if !strings.Contains(strings.Join(texts(dm), "\n"), "Subscription link") {
		t.Fatalf("the delivery did not carry the subscription link: %q", texts(dm))
	}
}

func TestOnlyAnAdminCanApproveOrRejectAReceipt(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	order := b.cardOrderInReview(t, 777)
	oid := strconv.Itoa(order.Id)

	for _, data := range []string{"shop:ra:" + oid, "shop:rj:" + oid + ":0", "shop:ad:st"} {
		b.tapIn(888, data, -100500, telego.ChatTypeSupergroup)
		b.tap(777, data)
	}
	if got := b.order(t, order.Id); got.Status != model.OrderStatusReview {
		t.Fatalf("order = %s after non-admins pressed the admin buttons, want review", got.Status)
	}
	if strings.Contains(b.api.last().text(), "Today") {
		t.Fatal("a non-admin was shown the admin statistics")
	}
}

func TestRejectionStoresTheReasonInTheBuyersLanguage(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	if err := b.tb.botUsers.SetLang(777, "fa-IR"); err != nil {
		t.Fatal(err)
	}
	order := b.cardOrderInReview(t, 777)
	oid := strconv.Itoa(order.Id)

	mark0 := b.api.mark()
	b.tapIn(1, "shop:rj:"+oid, 1, telego.ChatTypePrivate)
	picker := b.api.since(mark0, "editMessageReplyMarkup")
	if len(picker) != 1 {
		t.Fatalf("rejecting made %d keyboard edits, want the reason picker", len(picker))
	}
	reasons := picker[0].buttons()
	if len(reasons) != 5 || !strings.HasPrefix(reasons[1], "No payment was found|") {
		t.Fatalf("reason picker = %v, want four reasons in the admin's language and a back button", reasons)
	}
	b.tapIn(1, "shop:rj:"+oid+":1", 1, telego.ChatTypePrivate)

	got := b.order(t, order.Id)
	if got.Status != model.OrderStatusRejected || got.RejectReason != "پرداختی پیدا نشد" {
		t.Fatalf("order = %s %q, want rejected with the Persian reason for the buyer", got.Status, got.RejectReason)
	}
	mark := b.api.mark()
	b.tapIn(1, "shop:rj:"+oid+":2", 1, telego.ChatTypePrivate)
	b.tapIn(1, "shop:ra:"+oid, 1, telego.ChatTypePrivate)
	if n := len(b.api.since(mark, "sendMessage")); n != 2 {
		t.Fatalf("a decided order answered %d messages to two more presses, want 2 'already handled' notes", n)
	}
	if after := b.order(t, order.Id); after.Status != model.OrderStatusRejected {
		t.Fatalf("a rejected order was changed to %s by a late tap", after.Status)
	}
}

func TestShopOrderIsInvisibleToOtherAccounts(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	b.tap(777, "shop:p:"+strconv.Itoa(b.planID))
	b.tap(777, "shop:k:n")
	b.tap(777, "shop:g:card")
	orders := b.orders(777)
	mark := b.api.mark()

	b.tap(888, "shop:o:"+strconv.Itoa(orders[0].Id))
	got := b.api.shown(mark)
	if len(got) != 1 || got[0].text() != "Order not found." {
		t.Fatalf("foreign order reply = %q, want only the not-found refusal", texts(got))
	}
}

func TestShopRenewalNamesOnlyTheTappersOwnClient(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	seedClientRecord(t, "mine", "sub-mine", 555)
	seedClientRecord(t, "theirs", "sub-theirs", 666)
	var theirs model.ClientRecord
	database.GetDB().Where("email = ?", "theirs").First(&theirs)

	b.tap(555, "shop:p:"+strconv.Itoa(b.planID))
	b.tap(555, "shop:k:r"+strconv.Itoa(theirs.Id))
	if got := b.api.last().text(); got != "There is no account with that username." {
		t.Fatalf("renewing a foreign client replied %q", got)
	}
	if orders := b.orders(555); len(orders) != 0 {
		t.Fatalf("a foreign renewal opened %d orders", len(orders))
	}
	for _, action := range []string{"sv", "lk", "qr", "rn", "ao", "tn"} {
		mark := b.api.mark()
		b.tap(555, "shop:"+action+":"+strconv.Itoa(theirs.Id))
		for _, c := range b.api.shown(mark) {
			if strings.Contains(c.text(), "theirs") || strings.Contains(c.text(), "sub-theirs") {
				t.Fatalf("%s leaked another account's service: %q", action, c.text())
			}
		}
	}
	if _, pending := userStateMgr.get(chatUser{chatID: 555, userID: 555}); pending {
		t.Fatal("tapping 'new ticket' on a foreign service left a pending conversation")
	}
}
