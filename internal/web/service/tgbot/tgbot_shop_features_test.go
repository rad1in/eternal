package tgbot

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/payment"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"github.com/mymmrac/telego"
)

func withCalculator(tplID int) func(*service.SalesConfig) {
	return func(c *service.SalesConfig) {
		c.Custom = service.CustomOfferConfig{
			Enabled: true, TemplateId: tplID, Currency: "IRT", PricePerGB: 2000, PricePerDay: 1000,
			MinGB: 5, MaxGB: 100, MinDays: 7, MaxDays: 90, LimitIP: 3,
		}
	}
}

func TestCalculatorBuildsAnOrderFromPickedAndTypedValues(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, withCalculator(b.tplID))

	b.tap(777, "shop:buy")
	if buttons := strings.Join(b.api.last().buttons(), ","); !strings.Contains(buttons, "✨ Build your own plan|shop:cu") {
		t.Fatalf("plan list buttons = %s, want the calculator entry", buttons)
	}
	b.tap(777, "shop:cu")
	ask := b.api.last()
	if !strings.Contains(ask.text(), "5-100") || !strings.Contains(strings.Join(ask.buttons(), ","), "20 GB|shop:cg:20") {
		t.Fatalf("volume question = %q %v", ask.text(), ask.buttons())
	}

	b.tap(777, "shop:cg:20")
	if st, _ := userStateMgr.get(chatUser{chatID: 777, userID: 777}); st != shopStateDays {
		t.Fatalf("state = %q, want the days question pending", st)
	}
	if !b.say(777, "1000") {
		t.Fatal("the typed days were not consumed")
	}
	if got := b.api.last().text(); !strings.Contains(got, "between") && !strings.Contains(got, "7 and 90") {
		t.Fatalf("an out-of-range answer got %q, want the bounds repeated", got)
	}
	if st, _ := userStateMgr.get(chatUser{chatID: 777, userID: 777}); st != shopStateDays {
		t.Fatal("an out-of-range answer ended the question instead of repeating it")
	}
	if !b.say(777, "30") {
		t.Fatal("the typed days were not consumed")
	}
	confirm := b.api.last()
	if !strings.Contains(confirm.text(), "Custom plan: 20 GB + 30 Days") || !strings.Contains(confirm.text(), "70,000 Toman") {
		t.Fatalf("confirm = %q, want 20 GB + 30 days at 70,000", confirm.text())
	}

	b.tap(777, "shop:g:card")
	orders := b.orders(777)
	if len(orders) != 1 || orders[0].Amount != 70000 || orders[0].TotalGB != 20<<30 || orders[0].DurationDays != 30 || orders[0].PlanId != 0 {
		t.Fatalf("orders = %+v, want one custom order of 70000 for 20 GB / 30 days", orders)
	}
	if got := b.api.last().text(); !strings.Contains(got, "Custom plan: 20 GB + 30 Days") {
		t.Fatalf("order message = %q, want the localized item rather than the stored name", got)
	}
}

func TestAddonFromMyServicesKeepsUsageAndCanSkipOneDimension(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, withCalculator(b.tplID))
	future := time.Now().UnixMilli() + 10*24*3600*1000
	if _, err := (&service.ClientService{}).Create(&service.InboundService{}, &service.ClientCreatePayload{
		Client: model.Client{Email: "dave", Enable: true, TgID: 555, TotalGB: 10 << 30, ExpiryTime: future}, InboundIds: []int{b.inbID},
	}); err != nil {
		t.Fatal(err)
	}
	database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", "dave").Updates(map[string]any{"up": 3 << 30, "down": 1 << 30})
	var rec model.ClientRecord
	database.GetDB().Where("email = ?", "dave").First(&rec)
	rid := strconv.Itoa(rec.Id)

	b.tap(555, "shop:sv")
	card := b.api.last().text()
	if !strings.Contains(card, "4.00GB / 10.00GB") || !strings.Contains(card, "▓▓▓▓░░░░░░ 40%") || !strings.Contains(card, "Active") {
		t.Fatalf("service card = %q, want usage, a 40%% bar and the active state", card)
	}

	b.tap(555, "shop:ao:"+rid)
	if buttons := strings.Join(b.api.last().buttons(), ","); !strings.Contains(buttons, "Skip|shop:cg:0") {
		t.Fatalf("add-on volume question = %s, want a skip button", buttons)
	}
	b.tap(555, "shop:cg:0")
	if buttons := strings.Join(b.api.last().buttons(), ","); strings.Contains(buttons, "Skip") {
		t.Fatalf("skipping both dimensions was offered: %s", buttons)
	}
	b.tap(555, "shop:cd:7")
	confirm := b.api.last().text()
	if !strings.Contains(confirm, "Add-on: 7 Days") || !strings.Contains(confirm, "7,000 Toman") || !strings.Contains(confirm, "dave") {
		t.Fatalf("confirm = %q, want a 7-day add-on for dave at 7,000", confirm)
	}
	b.tap(555, "shop:g:card")
	orders := b.orders(555)
	if len(orders) != 1 || orders[0].Kind != model.OrderKindAddon || orders[0].ClientEmail != "dave" || orders[0].TotalGB != 0 || orders[0].DurationDays != 7 {
		t.Fatalf("orders = %+v, want one add-on of 7 days and no data for dave", orders)
	}
}

func TestLanguagePickerSwitchesEveryLaterScreenAndIsRemembered(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)

	b.tap(777, "shop:lang")
	langs := b.api.last().buttons()
	if len(langs) != 14 {
		t.Fatalf("language buttons = %d (%v), want the 13 shipped languages and a menu button", len(langs), langs)
	}
	b.tap(777, "shop:sl:fa-IR")
	if got := b.api.last(); !strings.Contains(got.text(), "سلام") || got.buttons()[0] != "🛒 خرید|shop:buy" {
		t.Fatalf("menu after choosing Persian = %q %v", got.text(), got.buttons())
	}
	b.tap(777, "shop:buy")
	if got := b.api.last().text(); !strings.Contains(got, "یک پلن انتخاب کنید") {
		t.Fatalf("plans after choosing Persian = %q", got)
	}
	if got := b.api.last().buttons()[0]; got != "Monthly - 250,000 تومان|shop:p:"+strconv.Itoa(b.planID) {
		t.Fatalf("plan button = %q, want the Persian currency name", got)
	}
	if u, _ := b.tb.botUsers.Get(777); u.Lang != "fa-IR" {
		t.Fatalf("stored language = %q", u.Lang)
	}

	b.tap(777, "shop:sl:xx-XX")
	if u, _ := b.tb.botUsers.Get(777); u.Lang != "fa-IR" {
		t.Fatalf("an unknown language replaced the choice: %q", u.Lang)
	}
}

func TestFirstLanguageFollowsTelegramThenTheBuyersChoice(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)

	b.tb.answerShopCallback(&telego.CallbackQuery{
		ID: "q", From: telego.User{ID: 31, FirstName: "Sasha", LanguageCode: "ru"}, Data: "shop:menu",
		Message: &telego.Message{MessageID: 5, Chat: telego.Chat{ID: 31, Type: telego.ChatTypePrivate}},
	})
	if got := b.api.last().text(); !strings.Contains(got, "Здравствуйте, Sasha") {
		t.Fatalf("a Russian-speaking Telegram account got %q", got)
	}
}

func TestAdminRewordedMenuTextIsUsedInTheBuyersLanguage(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	b.tb.botUsers.SetLang(777, "fa-IR")
	if err := locale.SetBotOverrides(`{"fa-IR":{"shopbot.menu.welcome":"به فروشگاه {{ .Name }} خوش آمدید"},"en-US":{"shopbot.menu.welcome":"English override"}}`); err != nil {
		t.Fatal(err)
	}
	b.tap(777, "shop:menu")
	if got := b.api.last().text(); got != "به فروشگاه Ali خوش آمدید" {
		t.Fatalf("menu = %q, want the Persian override only", got)
	}
}

func TestRequiredChannelsGateTheShopUntilJoined(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, func(c *service.SalesConfig) {
		c.Bot.ForceJoin = []service.ForceJoinChat{{Chat: "@mychannel"}}
	})
	b.api.members["@mychannel/777"] = "left"

	b.tap(777, "shop:buy")
	gate := b.api.last()
	if !strings.Contains(gate.text(), "join our channel") {
		t.Fatalf("a non-member got %q, want the join request", gate.text())
	}
	if want := []string{"Join @mychannel|https://t.me/mychannel", "✅ I have joined|shop:menu"}; strings.Join(gate.buttons(), ",") != strings.Join(want, ",") {
		t.Fatalf("gate buttons = %v, want %v", gate.buttons(), want)
	}

	b.api.members["@mychannel/777"] = "member"
	b.tap(777, "shop:buy")
	if got := b.api.last().text(); !strings.Contains(got, "Choose a plan") {
		t.Fatalf("after joining: %q", got)
	}
	mark := b.api.mark()
	b.tap(777, "shop:orders")
	if n := len(b.api.since(mark, "getChatMember")); n != 0 {
		t.Fatalf("membership was re-checked %d times within the cache window", n)
	}

	b.api.members["@mychannel/1"] = "left"
	b.tap(1, "shop:buy")
	if got := b.api.last().text(); !strings.Contains(got, "Choose a plan") {
		t.Fatalf("an admin was held at the gate: %q", got)
	}
}

func TestJoinGateRefusesAKickedAccountButFailsOpenWhenTheChannelIsUnreadable(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, func(c *service.SalesConfig) {
		c.Bot.ForceJoin = []service.ForceJoinChat{{Chat: "@mychannel"}}
	})
	b.api.members["@mychannel/777"] = "kicked"
	b.tap(777, "shop:buy")
	if !strings.Contains(b.api.last().text(), "join our channel") {
		t.Fatal("a kicked account passed the gate")
	}

	b.api.members["@mychannel/778"] = "error"
	b.tap(778, "shop:buy")
	if got := b.api.last().text(); !strings.Contains(got, "Choose a plan") {
		t.Fatalf("an unreadable channel locked a customer out: %q", got)
	}
}

func TestBannedAccountIsRefusedEverywhereButAdminsAreNot(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	if err := b.tb.botUsers.SetBanned(777, true); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"shop:menu", "shop:buy", "shop:p:" + strconv.Itoa(b.planID), "shop:g:card", "shop:orders"} {
		mark := b.api.mark()
		b.tap(777, data)
		shown := b.api.shown(mark)
		if len(shown) != 1 || !strings.Contains(shown[0].text(), "cannot use the shop") {
			t.Fatalf("%s for a banned account showed %v", data, texts(shown))
		}
	}
	if len(b.orders(777)) != 0 {
		t.Fatal("a banned account created an order")
	}

	b.tapIn(1, "shop:ad:ban", 1, telego.ChatTypePrivate)
	if !b.say(1, "1") {
		t.Fatal("the ban id was not consumed")
	}
	if got := b.api.last().text(); got != "An admin cannot be banned." {
		t.Fatalf("banning an admin replied %q", got)
	}
	b.tapIn(1, "shop:ad:unban", 1, telego.ChatTypePrivate)
	b.say(1, "777")
	if b.tb.botUsers.IsBanned(777) {
		t.Fatal("unban by an admin did not lift the ban")
	}
	b.tapIn(1, "shop:ad:ban", 1, telego.ChatTypePrivate)
	b.say(1, "not a number")
	if got := b.api.last().text(); got != "That is not a Telegram id." {
		t.Fatalf("a bad id replied %q", got)
	}
}

func TestReferralLinkIsRememberedAndAttachedToTheFirstPurchase(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, func(c *service.SalesConfig) { c.Referral = service.ReferralConfig{Enabled: true, BonusPercent: 10} })
	seedClientRecord(t, "sponsor", "sub-sponsor", 4000)
	code, err := (&service.ReferralService{}).CodeFor("sponsor")
	if err != nil {
		t.Fatal(err)
	}

	b.tb.answerCommand(commandFrom(777, "/start ref_"+strings.ToLower(code)), 777, false)
	if u, _ := b.tb.botUsers.Get(777); u.ReferralCode != code {
		t.Fatalf("remembered referral = %q, want %q", u.ReferralCode, code)
	}
	if !strings.Contains(b.api.last().text(), "Hello") {
		t.Fatalf("the referral link did not open the shop: %q", b.api.last().text())
	}

	b.tap(777, "shop:p:"+strconv.Itoa(b.planID))
	b.tap(777, "shop:k:n")
	b.tap(777, "shop:g:card")
	orders := b.orders(777)
	if len(orders) != 1 || orders[0].ReferrerEmail != "sponsor" {
		t.Fatalf("order = %+v, want the sponsor attached", orders)
	}

	b.tb.answerCommand(commandFrom(4000, "/start ref_"+code), 4000, false)
	b.tap(4000, "shop:p:"+strconv.Itoa(b.planID))
	b.tap(4000, "shop:k:n")
	b.tap(4000, "shop:g:card")
	for _, o := range b.orders(4000) {
		if o.ReferrerEmail != "" {
			t.Fatalf("a sponsor referred their own purchase: %+v", o)
		}
	}
}

func TestReferralPageShowsTheLinkOnlyToCustomers(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, func(c *service.SalesConfig) { c.Referral = service.ReferralConfig{Enabled: true, BonusPercent: 15} })
	b.tap(777, "shop:ref")
	if got := b.api.last().text(); !strings.Contains(got, "Buy a service first") {
		t.Fatalf("a stranger got %q", got)
	}
	seedClientRecord(t, "mine", "sub-mine", 555)
	b.tap(555, "shop:ref")
	got := b.api.last().text()
	if !strings.Contains(got, "15%") || !strings.Contains(got, "?start=ref_") {
		t.Fatalf("a customer got %q, want the percentage and an invite link", got)
	}
}

func TestSupportTicketsRunFromTheBotAndAdminRepliesComeBack(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	seedClientRecord(t, "mine", "sub-mine", 555)
	var rec model.ClientRecord
	database.GetDB().Where("email = ?", "mine").First(&rec)

	b.tap(777, "shop:sup")
	if got := b.api.last().text(); !strings.Contains(got, "customers with a service") {
		t.Fatalf("a stranger got %q", got)
	}

	b.tap(555, "shop:sup")
	b.tap(555, "shop:tn:"+strconv.Itoa(rec.Id))
	if !b.say(555, "Cannot connect\nsince this morning, every server times out") {
		t.Fatal("the ticket text was not consumed")
	}
	list, err := (&service.TicketService{}).List("", 10, 0)
	if err != nil || list.Total != 1 || list.Items[0].Subject != "Cannot connect" || list.Items[0].ClientEmail != "mine" {
		t.Fatalf("tickets = %+v, %v; want one from mine with the first line as subject", list, err)
	}
	tid := list.Items[0].Id

	if _, err := (&service.TicketService{}).AdminReply(tid, "Please try the new link <b>now</b>"); err != nil {
		t.Fatal(err)
	}
	mark := b.api.mark()
	b.tb.handleShopEvent(eventbus.Event{Type: eventbus.EventTicketAnswered, Data: &eventbus.TicketEventData{TicketId: tid, Email: "mine", Subject: "Cannot connect", Excerpt: "Please try the new link <b>now</b>"}})
	dm := b.api.since(mark, "sendMessage")
	if len(dm) != 1 || dm[0].chat() != 555 || !strings.Contains(dm[0].text(), "&lt;b&gt;now&lt;/b&gt;") {
		t.Fatalf("answer DM = %v, want one escaped message to 555", texts(dm))
	}

	b.tap(555, "shop:tk:"+strconv.Itoa(tid))
	view := b.api.last().text()
	if !strings.Contains(view, "Cannot connect") || !strings.Contains(view, "Support") || !strings.Contains(view, "every server times out") {
		t.Fatalf("ticket view = %q", view)
	}
	b.tap(555, "shop:tr:"+strconv.Itoa(tid))
	b.say(555, "Works now, thanks")
	if detail, _ := (&service.TicketService{}).Get(tid); len(detail.Messages) != 3 || detail.Messages[2].Body != "Works now, thanks" {
		t.Fatalf("messages = %+v, want the customer's reply appended", detail.Messages)
	}

	b.tap(666, "shop:tk:"+strconv.Itoa(tid))
	if got := b.api.last().text(); got != "Ticket not found." {
		t.Fatalf("another account reading the ticket got %q", got)
	}
}

func TestOrderEventsReachTheBuyerInTheirLanguage(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	b.tb.botUsers.SetLang(777, "ru-RU")
	order := b.cardOrderInReview(t, 777)
	if _, err := (&service.OrderService{}).RejectOrder(order.Id, "Сумма не совпадает"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		typ  eventbus.EventType
		want string
	}{
		{eventbus.EventOrderRejected, "не принят"},
		{eventbus.EventOrderExpired, "истёк"},
		{eventbus.EventOrderReminder, "ждёт оплаты"},
	} {
		mark := b.api.mark()
		b.tb.handleShopEvent(eventbus.Event{Type: tc.typ, Data: &eventbus.OrderEventData{OrderId: order.Id, TgId: 777}})
		dm := b.api.since(mark, "sendMessage")
		if len(dm) != 1 || dm[0].chat() != 777 || !strings.Contains(dm[0].text(), tc.want) {
			t.Fatalf("%s: DMs = %v, want one Russian message containing %q", tc.typ, texts(dm), tc.want)
		}
	}
	mark := b.api.mark()
	b.tb.handleShopEvent(eventbus.Event{Type: eventbus.EventOrderRejected, Data: &eventbus.OrderEventData{OrderId: order.Id, TgId: 777}})
	if got := b.api.since(mark, "sendMessage")[0].text(); !strings.Contains(got, "Сумма не совпадает") {
		t.Fatalf("rejection DM = %q, want the reason", got)
	}

	web, err := (&service.OrderService{}).CreateOrder(service.CreateOrderInput{PlanId: b.planID, Kind: model.OrderKindNew, Email: "webbuyer", Gateway: service.GatewayCard})
	if err != nil {
		t.Fatal(err)
	}
	mark = b.api.mark()
	b.tb.handleShopEvent(eventbus.Event{Type: eventbus.EventOrderReminder, Data: &eventbus.OrderEventData{OrderId: web.Id}})
	if n := len(b.api.since(mark, "sendMessage")); n != 0 {
		t.Fatalf("an order without a Telegram buyer produced %d messages", n)
	}
}

func TestShopEventsAreIgnoredWhileTheShopIsOff(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	order := b.cardOrderInReview(t, 777)
	b.off(t)
	mark := b.api.mark()
	handled := b.tb.handleShopEvent(eventbus.Event{Type: eventbus.EventOrderReminder, Data: &eventbus.OrderEventData{OrderId: order.Id, TgId: 777}})
	if !handled || len(b.api.since(mark, "sendMessage")) != 0 {
		t.Fatalf("handled %v with %d messages, want the event consumed silently", handled, len(b.api.since(mark, "sendMessage")))
	}
}

func TestMenuOffersTrialMiniAppAndOperatorLinksWhenConfigured(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, func(c *service.SalesConfig) {
		c.PublicEnabled = true
		c.PublicBaseURL = "https://shop.example.com"
		c.Bot.MiniApp = true
		c.Bot.Links = []service.BotLink{{Label: "Tutorials", URL: "https://t.me/tutorials"}}
	})
	enableTrial(t)
	b.tap(777, "shop:menu")
	buttons := strings.Join(b.api.last().buttons(), ",")
	for _, want := range []string{"🧪 Free trial|shop:trial", "Tutorials|https://t.me/tutorials", "🚀 Open the shop app|webapp:https://shop.example.com/shop/"} {
		if !strings.Contains(buttons, want) {
			t.Fatalf("menu buttons = %s, want %q", buttons, want)
		}
	}
	if strings.Contains(buttons, "Admin") {
		t.Fatal("the admin button was shown to a stranger")
	}
	b.tap(777, "shop:trial")
	if _, err := b.tb.clientService.GetRecordByEmail(nil, "trial-777"); err != nil {
		t.Fatalf("the trial button did not create the trial client: %v", err)
	}
	b.tap(777, "shop:menu")
	if strings.Contains(strings.Join(b.api.last().buttons(), ","), "Free trial") {
		t.Fatal("the trial button is still offered after the trial was used")
	}
	b.tap(1, "shop:menu")
	if !strings.Contains(strings.Join(b.api.last().buttons(), ","), "🛠 Admin|shop:ad") {
		t.Fatal("the admin button is missing for an admin")
	}
}

func TestAdminSeesTodaysSalesAndTheReceiptsWaiting(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	paid := b.cardOrderInReview(t, 777)
	if _, err := (&service.OrderService{}).FulfilOrder(&b.tb.inboundService, paid.Id); err != nil {
		t.Fatal(err)
	}
	waiting := b.cardOrderInReview(t, 778)

	b.tapIn(1, "shop:ad:st", 1, telego.ChatTypePrivate)
	stats := b.api.last().text()
	if !strings.Contains(stats, "1 paid order(s): 250,000 Toman") || !strings.Contains(stats, "Receipts waiting: 1") {
		t.Fatalf("stats = %q", stats)
	}
	b.tapIn(1, "shop:ad:rv", 1, telego.ChatTypePrivate)
	if buttons := b.api.last().buttons(); len(buttons) != 2 || !strings.HasSuffix(buttons[0], "|shop:ad:ro:"+strconv.Itoa(waiting.Id)) {
		t.Fatalf("waiting receipts = %v", buttons)
	}
	mark := b.api.mark()
	b.tapIn(1, "shop:ad:ro:"+strconv.Itoa(waiting.Id), 1, telego.ChatTypePrivate)
	if resent := b.api.since(mark, "sendMessage"); len(resent) != 1 || !strings.Contains(resent[0].text(), "Receipt for order #"+strconv.Itoa(waiting.Id)) {
		t.Fatalf("resent receipt = %v", texts(resent))
	}
}

func TestReceiptsAlsoGoToTheConfiguredGroupAndOnlyAdminsDecideThere(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, func(c *service.SalesConfig) { c.Bot.ReceiptChatId = -100777 })
	order := b.cardOrderInReview(t, 777)

	mark := b.api.mark()
	b.tb.notifyReceipt(order.Id)
	sent := b.api.since(mark, "sendMessage")
	if len(sent) != 2 || sent[0].chat() != 1 || sent[1].chat() != -100777 {
		t.Fatalf("recipients = %v, want admin 1 then the group", []int64{sent[0].chat(), sent[1].chat()})
	}

	oid := strconv.Itoa(order.Id)
	b.tapIn(2222, "shop:ra:"+oid, -100777, telego.ChatTypeSupergroup)
	if got := b.order(t, order.Id); got.Status != model.OrderStatusReview {
		t.Fatalf("a non-admin group member decided the order: %s", got.Status)
	}
	b.tapIn(1, "shop:ra:"+oid, -100777, telego.ChatTypeSupergroup)
	if got := b.order(t, order.Id); got.Status != model.OrderStatusPaid {
		t.Fatalf("an admin pressing in the group left the order %s", got.Status)
	}
}

func TestShopButtonsAreRateLimitedPerAccount(t *testing.T) {
	l := &actionLimiter{hits: map[int64]*actionWindow{}}
	now := time.Now()
	for i := range shopActionsPerWindow {
		if !l.allow(9, now) {
			t.Fatalf("tap %d was refused inside the allowance", i+1)
		}
	}
	if l.allow(9, now) {
		t.Fatal("the tap over the allowance was accepted")
	}
	if !l.allow(10, now) {
		t.Fatal("another account was throttled by 9")
	}
	if !l.allow(9, now.Add(shopActionWindow+time.Second)) {
		t.Fatal("the allowance did not reset after the window")
	}
}

func TestOnlyPrivateChatsRunCustomerFlows(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	mark := b.api.mark()
	b.tapIn(777, "shop:buy", -100123, telego.ChatTypeSupergroup)
	if n := len(b.api.shown(mark)); n != 0 {
		t.Fatalf("a group tap on a customer button produced %d messages", n)
	}
}

func TestWalletPaysARenewalAtOnceAndOnlyForTheBuyersOwnService(t *testing.T) {
	b := newShopBot(t)
	b.configure(t)
	seedClientRecord(t, "mine", "sub-mine", 555)
	if _, err := (&service.WalletService{}).Adjust("mine", "IRT", 300000, "top up"); err != nil {
		t.Fatal(err)
	}
	var rec model.ClientRecord
	database.GetDB().Where("email = ?", "mine").First(&rec)

	b.tap(555, "shop:wallet")
	if got := b.api.last().text(); !strings.Contains(got, "mine") || !strings.Contains(got, "300,000 Toman") {
		t.Fatalf("wallet = %q", got)
	}
	b.tap(555, "shop:rn:"+strconv.Itoa(rec.Id))
	b.tap(555, "shop:p:"+strconv.Itoa(b.planID))
	confirm := b.api.last()
	if !strings.Contains(strings.Join(confirm.buttons(), ","), "👛 Wallet|shop:g:wallet") {
		t.Fatalf("a renewal offers %v, want the wallet", confirm.buttons())
	}
	mark := b.api.mark()
	b.tap(555, "shop:g:wallet")
	orders := b.orders(555)
	if len(orders) != 1 || orders[0].Status != model.OrderStatusPaid || orders[0].FulfilledAt == 0 {
		t.Fatalf("orders = %+v, want one paid and delivered at once", orders)
	}
	if bal, _ := (&service.WalletService{}).Balances("mine"); len(bal) != 1 || bal[0].Amount != 50000 {
		t.Fatalf("balance = %+v, want 300000 - 250000", bal)
	}
	if got := strings.Join(texts(b.api.shown(mark)), "\n"); !strings.Contains(got, "Paid and renewed") {
		t.Fatalf("messages = %q, want the renewal confirmation", got)
	}

	b.tap(777, "shop:p:"+strconv.Itoa(b.planID))
	b.tap(777, "shop:k:n")
	if strings.Contains(strings.Join(b.api.last().buttons(), ","), "shop:g:wallet") {
		t.Fatal("a new account was offered a wallet payment")
	}
}

type fixedPayURL struct{}

func (fixedPayURL) Start(context.Context, payment.StartRequest) (*payment.StartResult, error) {
	return &payment.StartResult{Ref: "AUTH-1", PayURL: "https://pay.example/AUTH-1"}, nil
}

func TestOnlineGatewayOrderShowsAPayButton(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, func(c *service.SalesConfig) {
		c.PublicBaseURL = "https://shop.example.com"
		c.Zarinpal = service.ZarinpalConfig{Enabled: true, MerchantId: "11111111-2222-3333-4444-555555555555"}
	})
	b.tb.orderService.SetGatewayFactory(func(string, *service.SalesConfig) payment.Starter { return fixedPayURL{} })

	b.tap(777, "shop:p:"+strconv.Itoa(b.planID))
	b.tap(777, "shop:k:n")
	if !strings.Contains(strings.Join(b.api.last().buttons(), ","), "Zarinpal|shop:g:zarinpal") {
		t.Fatalf("payment buttons = %v, want Zarinpal", b.api.last().buttons())
	}
	b.tap(777, "shop:g:zarinpal")
	got := b.api.last()
	if !strings.Contains(strings.Join(got.buttons(), ","), "💳 Pay now|https://pay.example/AUTH-1") {
		t.Fatalf("order buttons = %v, want a URL button to the gateway", got.buttons())
	}
	if strings.Contains(got.text(), "6037991812345678") {
		t.Fatal("card details were shown for a gateway order")
	}
}

func TestWebShopReceiptsReachAdminsEvenWhenTheBotShopIsOff(t *testing.T) {
	b := newShopBot(t)
	b.configure(t, func(c *service.SalesConfig) { c.TelegramShop = false })
	orders := &service.OrderService{}
	o, err := orders.CreateOrder(service.CreateOrderInput{PlanId: b.planID, Kind: model.OrderKindNew, Email: "webbuyer", Gateway: service.GatewayCard})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orders.SubmitReceipt(o.Token, "99887", ""); err != nil {
		t.Fatal(err)
	}

	mark := b.api.mark()
	b.tb.handleShopEvent(eventbus.Event{Type: eventbus.EventOrderReceipt, Data: &eventbus.OrderEventData{OrderId: o.Id}})
	sent := b.api.since(mark, "sendMessage")
	if len(sent) != 1 || sent[0].chat() != 1 || !strings.Contains(sent[0].text(), "web shop") {
		t.Fatalf("admin notification = %v, want one message to admin 1 naming the web shop", texts(sent))
	}
	b.tapIn(1, "shop:ra:"+strconv.Itoa(o.Id), 1, telego.ChatTypePrivate)
	if got := b.order(t, o.Id); got.Status != model.OrderStatusPaid {
		t.Fatalf("approving from Telegram left the web order %s", got.Status)
	}
}
