package service

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

// recordEvents installs a bus for the test and returns what it has seen of the
// given types, after letting the asynchronous fan-out settle.
func recordEvents(t *testing.T) func() []eventbus.Event {
	t.Helper()
	bus := eventbus.New(0)
	var mu sync.Mutex
	var got []eventbus.Event
	bus.Subscribe("rec", func(e eventbus.Event) { mu.Lock(); got = append(got, e); mu.Unlock() })
	old := eventBus
	SetEventBus(bus)
	t.Cleanup(func() { SetEventBus(old); bus.Stop() })
	return func() []eventbus.Event {
		time.Sleep(150 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return append([]eventbus.Event(nil), got...)
	}
}

func TestTelegramReceiptPhotoGoesToReviewAndAnnouncesItself(t *testing.T) {
	f := newShopFixture(t, 25201)
	seen := recordEvents(t)
	o, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Gateway: GatewayCard, TgId: 900})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.orders.SubmitTelegramReceipt(901, o.Id, "", "", "FILE"); err == nil {
		t.Fatal("another Telegram account submitted a receipt for 900's order")
	} else {
		wantSalesError(t, err, ErrOrderNotFound)
	}
	_, err = f.orders.SubmitTelegramReceipt(900, o.Id, "  ", "", "")
	wantSalesError(t, err, ErrReceiptInvalid)

	got, err := f.orders.SubmitTelegramReceipt(900, o.Id, "", "paid 14:20", "AgACAgQAAxk")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.OrderStatusReview || got.ReceiptRef != "photo" || got.ReceiptFileId != "AgACAgQAAxk" || got.ReceiptNote != "paid 14:20" {
		t.Fatalf("order = %s %q file %q, want review with a photo receipt", got.Status, got.ReceiptRef, got.ReceiptFileId)
	}
	events := seen()
	if len(events) != 1 || events[0].Type != eventbus.EventOrderReceipt {
		t.Fatalf("events = %v, want one order.receipt", eventTypes(events))
	}
	if data := events[0].Data.(*eventbus.OrderEventData); data.OrderId != o.Id || data.TgId != 900 {
		t.Fatalf("event data = %+v", data)
	}
}

func TestOrderOutcomesAreAnnouncedOnce(t *testing.T) {
	f := newShopFixture(t, 25202)
	seen := recordEvents(t)
	approved := f.newOrder(t, "alice")
	rejected := f.newOrder(t, "bobby")

	if _, err := f.orders.FulfilOrder(&InboundService{}, approved.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.orders.RejectOrder(rejected.Id, "no payment found"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.orders.FulfilOrder(&InboundService{}, approved.Id); err == nil {
		t.Fatal("approving twice succeeded")
	}
	var events []eventbus.Event
	for _, e := range seen() {
		if strings.HasPrefix(string(e.Type), "order.") {
			events = append(events, e)
		}
	}
	want := []eventbus.EventType{eventbus.EventOrderFulfilled, eventbus.EventOrderRejected}
	if got := eventTypes(events); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestSweepExpiresStaleCardOrdersAndRemindsOnce(t *testing.T) {
	f := newShopFixture(t, 25203)
	seen := recordEvents(t)
	now := time.Now()
	mk := func(tg int64, age time.Duration) *model.Order {
		o, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Gateway: GatewayCard, TgId: tg})
		if err != nil {
			t.Fatal(err)
		}
		if err := database.GetDB().Model(&model.Order{}).Where("id = ?", o.Id).
			Update("created_at", now.Add(-age).UnixMilli()).Error; err != nil {
			t.Fatal(err)
		}
		return o
	}
	fresh := mk(1, 5*time.Minute)
	waiting := mk(2, 40*time.Minute)
	stale := mk(3, 49*time.Hour)

	for range 2 {
		if err := f.orders.SweepUnpaid(now); err != nil {
			t.Fatal(err)
		}
	}
	status := func(id int) string {
		o, _ := f.orders.GetOrder(id)
		return o.Status
	}
	if status(fresh.Id) != model.OrderStatusPending || status(waiting.Id) != model.OrderStatusPending || status(stale.Id) != model.OrderStatusCanceled {
		t.Fatalf("statuses = %s/%s/%s, want pending/pending/canceled", status(fresh.Id), status(waiting.Id), status(stale.Id))
	}
	got := map[eventbus.EventType][]int{}
	for _, e := range seen() {
		got[e.Type] = append(got[e.Type], e.Data.(*eventbus.OrderEventData).OrderId)
	}
	if len(got[eventbus.EventOrderReminder]) != 1 || got[eventbus.EventOrderReminder][0] != waiting.Id {
		t.Fatalf("reminders = %v, want only the 40-minute-old order, once despite two sweeps", got[eventbus.EventOrderReminder])
	}
	if len(got[eventbus.EventOrderExpired]) != 1 || got[eventbus.EventOrderExpired][0] != stale.Id {
		t.Fatalf("expired = %v, want only the 49-hour-old order", got[eventbus.EventOrderExpired])
	}
	if _, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Gateway: GatewayCard, Email: stale.ClientEmail}); err != nil {
		t.Fatalf("the expired order still holds its username: %v", err)
	}
}

func TestSalesSummaryCountsPaidPerCurrencyAndWhatIsWaiting(t *testing.T) {
	f := newShopFixture(t, 25204)
	a, b, c := f.newOrder(t, "alice"), f.newOrder(t, "bobby"), f.newOrder(t, "carol")
	for _, o := range []*model.Order{a, b} {
		if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.orders.SubmitReceipt(c.Token, "1234", ""); err != nil {
		t.Fatal(err)
	}
	f.newOrder(t, "dave")
	old := f.newOrder(t, "erin")
	if _, err := f.orders.FulfilOrder(&InboundService{}, old.Id); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(&model.Order{}).Where("id = ?", old.Id).Update("paid_at", time.Now().Add(-48*time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}

	sum, err := f.orders.SalesSummary(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Paid) != 1 || sum.Paid[0].Currency != "IRT" || sum.Paid[0].Orders != 2 || sum.Paid[0].Amount != 500000 {
		t.Fatalf("paid = %+v, want 2 orders / 500000 IRT (the 48-hour-old one is outside the day)", sum.Paid)
	}
	if sum.Pending != 1 || sum.Review != 1 {
		t.Fatalf("pending %d review %d, want 1 and 1", sum.Pending, sum.Review)
	}
	if got, _ := f.orders.OrdersInReview(10); len(got) != 1 || got[0].Id != c.Id {
		t.Fatalf("in review = %+v, want only carol's order", got)
	}
}
