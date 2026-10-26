package service

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// enableCustom turns the calculator on: 2000 per GB plus 1000 per day.
func (f *shopFixture) enableCustom(t *testing.T) {
	t.Helper()
	cfg := &SalesConfig{
		Card: CardPayConfig{Enabled: true, CardNumber: "6037991812345678"},
		Custom: CustomOfferConfig{
			Enabled: true, TemplateId: f.tpl.Id, Currency: "IRT", PricePerGB: 2000, PricePerDay: 1000,
			MinGB: 5, MaxGB: 100, MinDays: 7, MaxDays: 90, LimitIP: 3,
		},
	}
	if err := (&SalesConfigService{}).Save(cfg); err != nil {
		t.Fatalf("Save config: %v", err)
	}
}

func TestCustomOrderIsPricedByUnitsAndDeliversWhatWasChosen(t *testing.T) {
	f := newShopFixture(t, 25101)
	f.enableCustom(t)

	o, err := f.orders.CreateOrder(CreateOrderInput{Kind: model.OrderKindNew, Email: "carol", Gateway: GatewayCard, CustomGB: 20, CustomDays: 30})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if o.Amount != 20*2000+30*1000 || o.Currency != "IRT" || o.PlanId != 0 || o.PlanName != "20 GB / 30 d" {
		t.Fatalf("order = amount %d %s plan %d %q, want 70000 IRT for a custom 20 GB / 30 d", o.Amount, o.Currency, o.PlanId, o.PlanName)
	}
	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("FulfilOrder: %v", err)
	}
	rec := lookupClientRecord(t, "carol")
	if rec.TotalGB != 20<<30 || rec.LimitIP != 3 || rec.ExpiryTime < time.Now().Add(29*24*time.Hour).UnixMilli() {
		t.Fatalf("client = quota %d ip %d expiry %d, want 20 GB, 3 IPs and about 30 days", rec.TotalGB, rec.LimitIP, rec.ExpiryTime)
	}
}

func TestCustomOrderRefusals(t *testing.T) {
	f := newShopFixture(t, 25102)
	_, err := f.orders.CreateOrder(CreateOrderInput{Kind: model.OrderKindNew, Email: "carol", Gateway: GatewayCard, CustomGB: 20, CustomDays: 30})
	wantSalesError(t, err, ErrPlanUnavailable)

	f.enableCustom(t)
	cases := []struct {
		name     string
		gb, days int
	}{
		{"below the minimum quota", 4, 30},
		{"above the maximum quota", 101, 30},
		{"below the minimum days", 20, 6},
		{"above the maximum days", 20, 91},
		{"no quota at all on a new account", 0, 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.orders.CreateOrder(CreateOrderInput{Kind: model.OrderKindNew, Email: "carol", Gateway: GatewayCard, CustomGB: tc.gb, CustomDays: tc.days})
			wantSalesError(t, err, ErrCustomInvalid)
		})
	}
}

func TestCustomOrderIgnoresPlanOnlyDiscountCodes(t *testing.T) {
	f := newShopFixture(t, 25103)
	f.enableCustom(t)
	if _, err := (&DiscountService{}).SaveDiscount(&model.DiscountCode{
		Code: "ONLYPLAN", Kind: model.DiscountPercent, Value: 10, PlanIds: []int{f.plan.Id}, Enabled: true,
	}); err != nil {
		t.Fatalf("SaveDiscount: %v", err)
	}
	_, err := f.orders.CreateOrder(CreateOrderInput{
		Kind: model.OrderKindNew, Email: "carol", Gateway: GatewayCard, CustomGB: 20, CustomDays: 30, DiscountCode: "ONLYPLAN",
	})
	wantSalesError(t, err, ErrDiscountInvalid)
}

func seedUsedClient(t *testing.T, f *shopFixture, email string, total int64, expiry int64) {
	t.Helper()
	if _, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client: model.Client{Email: email, Enable: true, TotalGB: total, ExpiryTime: expiry}, InboundIds: f.tpl.InboundIds,
	}); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", email).
		Updates(map[string]any{"up": 3 << 30, "down": 1 << 30}).Error; err != nil {
		t.Fatalf("seed usage: %v", err)
	}
}

func TestAddonStacksQuotaAndDaysWithoutResettingUsage(t *testing.T) {
	f := newShopFixture(t, 25104)
	f.enableCustom(t)
	future := time.Now().UnixMilli() + 10*24*3600*1000
	seedUsedClient(t, f, "dave", 10<<30, future)

	o, err := f.orders.CreateOrder(CreateOrderInput{Kind: model.OrderKindAddon, Email: "dave", Gateway: GatewayCard, CustomGB: 5, CustomDays: 7})
	if err != nil {
		t.Fatalf("CreateOrder addon: %v", err)
	}
	if o.Amount != 5*2000+7*1000 {
		t.Fatalf("addon amount = %d, want 17000", o.Amount)
	}
	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("FulfilOrder: %v", err)
	}
	rec := lookupClientRecord(t, "dave")
	if rec.TotalGB != 15<<30 || rec.ExpiryTime != future+7*24*3600*1000 {
		t.Fatalf("client = quota %d expiry %d, want 15 GB and +7 days on the old expiry", rec.TotalGB, rec.ExpiryTime)
	}
	var tr xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", "dave").First(&tr).Error; err != nil {
		t.Fatal(err)
	}
	if tr.Up != 3<<30 || tr.Down != 1<<30 {
		t.Fatalf("usage = %d/%d, an add-on must keep what was already used", tr.Up, tr.Down)
	}
}

func TestAddonRefusals(t *testing.T) {
	f := newShopFixture(t, 25105)
	f.enableCustom(t)
	seedUsedClient(t, f, "erin", 0, 0)

	cases := []struct {
		name string
		in   CreateOrderInput
		code string
	}{
		{"quota added to an unlimited client", CreateOrderInput{Kind: model.OrderKindAddon, Email: "erin", Gateway: GatewayCard, CustomGB: 5}, ErrAddonUnsupported},
		{"days added to a client that never expires", CreateOrderInput{Kind: model.OrderKindAddon, Email: "erin", Gateway: GatewayCard, CustomDays: 7}, ErrAddonUnsupported},
		{"nothing added", CreateOrderInput{Kind: model.OrderKindAddon, Email: "erin", Gateway: GatewayCard}, ErrCustomInvalid},
		{"unknown client", CreateOrderInput{Kind: model.OrderKindAddon, Email: "ghost", Gateway: GatewayCard, CustomGB: 5}, ErrEmailUnknown},
		{"a stored plan cannot be an add-on", CreateOrderInput{Kind: model.OrderKindAddon, PlanId: f.plan.Id, Email: "erin", Gateway: GatewayCard}, ErrInvalidKind},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.orders.CreateOrder(tc.in)
			wantSalesError(t, err, tc.code)
		})
	}
}

func TestTelegramBuyerGetsAGeneratedUsernameAndIsCappedOnOpenOrders(t *testing.T) {
	f := newShopFixture(t, 25106)
	for i := range maxOpenOrdersPerTelegram {
		o, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Gateway: GatewayCard, TgId: 4242})
		if err != nil {
			t.Fatalf("order %d: %v", i, err)
		}
		if i == 0 && o.ClientEmail != "tg4242" {
			t.Fatalf("username = %q, want the generated tg4242", o.ClientEmail)
		}
	}
	_, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Gateway: GatewayCard, TgId: 4242})
	wantSalesError(t, err, ErrRateLimited)
	if _, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Gateway: GatewayCard, TgId: 4343}); err != nil {
		t.Fatalf("another buyer is capped too: %v", err)
	}
}

func TestCatalogCarriesCategoriesAndTheCalculatorBounds(t *testing.T) {
	f := newShopFixture(t, 25107)
	edited := *f.plan
	edited.Category = "  Monthly "
	if _, err := (&PlanService{}).SavePlan(&edited); err != nil {
		t.Fatal(err)
	}
	tooLong := edited
	tooLong.Category = string(make([]rune, 41))
	if _, err := (&PlanService{}).SavePlan(&tooLong); err == nil {
		t.Fatal("a 41-character category was saved")
	}

	cat, err := f.orders.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if cat.Custom != nil || cat.Plans[0].Category != "Monthly" {
		t.Fatalf("catalog = custom %v category %q, want no calculator and the trimmed category", cat.Custom, cat.Plans[0].Category)
	}
	f.enableCustom(t)
	cat, _ = f.orders.Catalog()
	if cat.Custom == nil || cat.Custom.PricePerGB != 2000 || cat.Custom.MaxDays != 90 {
		t.Fatalf("calculator = %+v", cat.Custom)
	}
}

func TestSalesConfigValidatesTheBotAndCalculatorSections(t *testing.T) {
	f := newShopFixture(t, 25108)
	svc := &SalesConfigService{}
	good := CustomOfferConfig{Enabled: true, TemplateId: f.tpl.Id, Currency: "IRT", PricePerGB: 1, MinGB: 1, MaxGB: 10, MinDays: 1, MaxDays: 30}
	cases := []struct {
		name string
		cfg  SalesConfig
	}{
		{"calculator without prices", SalesConfig{Custom: func() CustomOfferConfig { c := good; c.PricePerGB = 0; return c }()}},
		{"calculator with inverted bounds", SalesConfig{Custom: func() CustomOfferConfig { c := good; c.MaxGB = 0; return c }()}},
		{"calculator with an unknown template", SalesConfig{Custom: func() CustomOfferConfig { c := good; c.TemplateId = 9999; return c }()}},
		{"channel that is not a handle", SalesConfig{Bot: ShopBotConfig{ForceJoin: []ForceJoinChat{{Chat: "my channel"}}}}},
		{"private channel without a link", SalesConfig{Bot: ShopBotConfig{ForceJoin: []ForceJoinChat{{Chat: "-1001234567890"}}}}},
		{"menu link with a javascript URL", SalesConfig{Bot: ShopBotConfig{Links: []BotLink{{Label: "x", URL: "javascript:alert(1)"}}}}},
		{"mini app on plain http", SalesConfig{PublicBaseURL: "http://shop.example.com", Bot: ShopBotConfig{MiniApp: true}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := svc.Save(&tc.cfg); err == nil {
				t.Fatal("an invalid section was saved")
			}
		})
	}
	ok := SalesConfig{
		PublicBaseURL: "https://shop.example.com",
		Bot: ShopBotConfig{
			ForceJoin: []ForceJoinChat{{Chat: "@mychannel"}}, MiniApp: true,
			Links: []BotLink{{Label: "Tutorials", URL: "https://t.me/tutorials"}},
		},
		Custom: good,
	}
	if err := svc.Save(&ok); err != nil {
		t.Fatalf("a valid config was refused: %v", err)
	}
	got, _ := svc.Get()
	if got.Bot.ForceJoin[0].Link != "https://t.me/mychannel" {
		t.Fatalf("join link = %q, want it derived from the @handle", got.Bot.ForceJoin[0].Link)
	}
}
