package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// telegramInitData signs launch data the documented way for the given bot token.
func telegramInitData(token string, fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + fields[k]
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	q := url.Values{}
	for k, v := range fields {
		q.Set(k, v)
	}
	q.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return q.Encode()
}

func enableMiniApp(t *testing.T, f *shopFixture, token string) {
	t.Helper()
	cfg := &SalesConfig{
		PublicBaseURL: "https://shop.example.com",
		Card:          CardPayConfig{Enabled: true, CardNumber: "6037991812345678"},
		Bot:           ShopBotConfig{MiniApp: true},
	}
	if err := (&SalesConfigService{}).Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := (&SettingService{}).saveSetting("tgBotToken", token); err != nil {
		t.Fatal(err)
	}
}

func launch(token string, tg int64, age time.Duration) string {
	return telegramInitData(token, map[string]string{
		"auth_date": strconv.FormatInt(time.Now().Add(-age).Unix(), 10),
		"user":      `{"id":` + strconv.FormatInt(tg, 10) + `,"first_name":"Ali","language_code":"fa"}`,
	})
}

func TestMiniAppExchangeSignsTheBuyerInAndListsOnlyTheirServices(t *testing.T) {
	f := newShopFixture(t, 25301)
	enableMiniApp(t, f, "123:TOKEN")
	seedBoundClient(t, f, "mine", 555)
	seedBoundClient(t, f, "theirs", 666)

	svc := &ShopSessionService{}
	sess, err := svc.Exchange(launch("123:TOKEN", 555, time.Minute), "shop.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if sess.TgId != 555 || sess.Name != "Ali" || sess.Lang != "fa" || len(sess.Services) != 1 || sess.Services[0].Email != "mine" {
		t.Fatalf("session = %+v, want 555 with only the service bound to 555", sess)
	}
	tg, err := svc.Verify(sess.Token)
	if err != nil || tg != 555 {
		t.Fatalf("Verify = %d, %v; want 555", tg, err)
	}
}

func TestMiniAppExchangeRefusals(t *testing.T) {
	f := newShopFixture(t, 25302)
	svc := &ShopSessionService{}

	if _, err := svc.Exchange(launch("123:TOKEN", 555, time.Minute), ""); !isSalesError(err, ErrGatewayUnavailable) {
		t.Fatalf("with the Mini App off: err = %v, want gateway_unavailable", err)
	}
	enableMiniApp(t, f, "123:TOKEN")
	for name, data := range map[string]string{
		"signed by another bot": launch("999:OTHER", 555, time.Minute),
		"stale":                 launch("123:TOKEN", 555, 30*time.Hour),
		"garbage":               "hash=00&auth_date=1",
	} {
		if _, err := svc.Exchange(data, ""); !isSalesError(err, ErrProofInvalid) {
			t.Fatalf("%s: err = %v, want proof_invalid", name, err)
		}
	}
	if err := (&BotUserService{}).SetBanned(555, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Exchange(launch("123:TOKEN", 555, time.Minute), ""); !isSalesError(err, ErrBanned) {
		t.Fatalf("banned buyer: err = %v, want banned", err)
	}
}

func TestSessionTokenRefusals(t *testing.T) {
	setupBulkDB(t)
	svc := &ShopSessionService{}
	token, err := svc.sign(555, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	expired, _ := svc.sign(555, time.Now().Add(-time.Second))
	parts := strings.Split(token, ".")
	forged := strings.Split(mustSign(t, svc, 556), ".")[0] + "." + parts[1]

	for name, tok := range map[string]string{"expired": expired, "forged id": forged, "empty": "", "no dot": "abc", "truncated": token[:len(token)-3]} {
		if _, err := svc.Verify(tok); !isSalesError(err, ErrProofInvalid) {
			t.Fatalf("%s: err = %v, want proof_invalid", name, err)
		}
	}
}

func mustSign(t *testing.T, svc *ShopSessionService, tg int64) string {
	t.Helper()
	tok, err := svc.sign(tg, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func seedBoundClient(t *testing.T, f *shopFixture, email string, tg int64) {
	t.Helper()
	if _, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client: model.Client{Email: email, Enable: true, TgID: tg, TotalGB: 10 << 30}, InboundIds: f.tpl.InboundIds,
	}); err != nil {
		t.Fatalf("seed client %s: %v", email, err)
	}
}

func TestVerifiedTelegramIdentityStandsInForTheSubscriptionProof(t *testing.T) {
	f := newShopFixture(t, 25303)
	seedBoundClient(t, f, "mine", 555)
	seedBoundClient(t, f, "theirs", 666)
	tickets := &TicketService{}

	if _, err := tickets.Open(TicketAuth{Email: "mine", TgId: 555}, "help", "body"); err != nil {
		t.Fatalf("own client: %v", err)
	}
	if _, err := tickets.Open(TicketAuth{Email: "theirs", TgId: 555}, "help", "body"); !isSalesError(err, ErrProofInvalid) {
		t.Fatalf("another account's client: err = %v, want proof_invalid", err)
	}
	if _, err := f.orders.TelegramWallet(555, "mine"); err != nil {
		t.Fatalf("own wallet: %v", err)
	}
	if _, err := f.orders.TelegramWallet(555, "theirs"); !isSalesError(err, ErrEmailUnknown) {
		t.Fatalf("another account's wallet: err = %v, want email_unknown", err)
	}
}

func TestBannedTelegramBuyerCannotOpenOrders(t *testing.T) {
	f := newShopFixture(t, 25304)
	if err := (&BotUserService{}).SetBanned(555, true); err != nil {
		t.Fatal(err)
	}
	_, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Gateway: GatewayCard, TgId: 555})
	wantSalesError(t, err, ErrBanned)
	if _, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Gateway: GatewayCard, TgId: 556}); err != nil {
		t.Fatalf("an unrelated buyer is refused: %v", err)
	}
}
