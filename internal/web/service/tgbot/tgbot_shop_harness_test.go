package tgbot

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/mymmrac/telego"
)

type panelLang string

func (l panelLang) GetTgLang() (string, error) { return string(l), nil }

// loadRealBotLocale installs every shipped translation, so the assertions read
// what a customer reads, in any of the 13 languages.
func loadRealBotLocale(t *testing.T) {
	t.Helper()
	origLocalizer := locale.LocalizerBot
	if err := locale.InitLocalizer(os.DirFS(filepath.Join("..", "..")), panelLang("en-US")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		locale.LocalizerBot = origLocalizer
		_ = locale.SetBotOverrides("")
	})
}

// apiCall is one request the bot made to the fake Bot API.
type apiCall struct {
	method string
	body   map[string]any
}

func (c apiCall) text() string {
	for _, k := range []string{"text", "caption"} {
		if s, ok := c.body[k].(string); ok {
			return s
		}
	}
	return ""
}

func (c apiCall) chat() int64 {
	switch v := c.body["chat_id"].(type) {
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

// buttons lists "text|callback-or-url" for every inline button of the call.
func (c apiCall) buttons() []string {
	markup, _ := c.body["reply_markup"].(map[string]any)
	rows, _ := markup["inline_keyboard"].([]any)
	var out []string
	for _, row := range rows {
		for _, b := range row.([]any) {
			btn := b.(map[string]any)
			target := ""
			for _, k := range []string{"callback_data", "url"} {
				if s, ok := btn[k].(string); ok {
					target = s
				}
			}
			if app, ok := btn["web_app"].(map[string]any); ok {
				target = "webapp:" + app["url"].(string)
			}
			out = append(out, btn["text"].(string)+"|"+target)
		}
	}
	return out
}

type fakeAPI struct {
	mu      sync.Mutex
	calls   []apiCall
	members map[string]string // "chat/user" -> chat member status
	url     string
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{members: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		payload := map[string]any{}
		_ = json.Unmarshal(raw, &payload)
		method := strings.TrimPrefix(r.URL.Path, "/bot"+testBotToken+"/")
		f.mu.Lock()
		f.calls = append(f.calls, apiCall{method: method, body: payload})
		n := len(f.calls)
		f.mu.Unlock()
		var result any = map[string]any{"message_id": 100 + n, "date": 0, "chat": map[string]any{"id": 1, "type": "private"}}
		switch method {
		case "answerCallbackQuery":
			result = true
		case "getChatMember":
			key := toString(payload["chat_id"]) + "/" + toString(payload["user_id"])
			f.mu.Lock()
			status := f.members[key]
			f.mu.Unlock()
			if status == "error" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": 400, "description": "Bad Request: chat not found"})
				return
			}
			if status == "" {
				status = "member"
			}
			result = map[string]any{"status": status, "user": map[string]any{"id": 1, "is_bot": false, "first_name": "x"}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
	}))
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f
}

func toString(v any) string {
	switch x := v.(type) {
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case string:
		return x
	}
	return ""
}

func (f *fakeAPI) screens() []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []apiCall
	for _, c := range f.calls {
		switch c.method {
		case "sendMessage", "editMessageText", "sendPhoto", "editMessageCaption":
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeAPI) last() apiCall {
	s := f.screens()
	if len(s) == 0 {
		return apiCall{}
	}
	return s[len(s)-1]
}

func (f *fakeAPI) mark() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeAPI) since(mark int, methods ...string) []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []apiCall
	for _, c := range f.calls[mark:] {
		for _, m := range methods {
			if c.method == m {
				out = append(out, c)
			}
		}
	}
	return out
}

// shown is what the customer saw after mark: sent and edited screens in order.
func (f *fakeAPI) shown(mark int) []apiCall {
	return f.since(mark, "sendMessage", "editMessageText", "sendPhoto", "editMessageCaption")
}

func texts(calls []apiCall) []string {
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.text()
	}
	return out
}

type shopBot struct {
	tb     *Tgbot
	api    *fakeAPI
	planID int
	tplID  int
	inbID  int
}

func newShopBot(t *testing.T) *shopBot {
	t.Helper()
	loadRealBotLocale(t)
	api := newFakeAPI(t)
	swapTestBot(t, api.url)
	setBroadcastRunning(t, true)
	withAdmins(t, 1)
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	shopSelectionsMu.Lock()
	shopSelections = map[chatUser]*shopSelection{}
	shopSelectionsMu.Unlock()
	userStateMgr.reset()
	joinCacheMu.Lock()
	joinCache = map[int64]time.Time{}
	joinCacheMu.Unlock()
	shopLimiter.mu.Lock()
	shopLimiter.hits = map[int64]*actionWindow{}
	shopLimiter.mu.Unlock()

	ib := &model.Inbound{UserId: 1, Remark: "in", Port: 443, Protocol: model.VLESS, Enable: true, Tag: "in-443", Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	tpl, err := (&service.ClientService{}).SaveClientTemplate(&model.ClientTemplate{Name: "tpl", Group: "shop", InboundIds: []int{ib.Id}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := (&service.PlanService{}).SavePlan(&model.Plan{
		Name: "Monthly", Price: 250000, Currency: "IRT", TotalGB: 50 << 30, DurationDays: 30, LimitIP: 2, TemplateId: tpl.Id, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &shopBot{tb: &Tgbot{}, api: api, planID: plan.Id, tplID: tpl.Id, inbID: ib.Id}
}

// configure saves a sales config with the card on, then lets a test adjust it.
func (b *shopBot) configure(t *testing.T, mutate ...func(*service.SalesConfig)) {
	t.Helper()
	cfg := &service.SalesConfig{TelegramShop: true, WalletEnabled: true, Card: service.CardPayConfig{
		Enabled: true, CardNumber: "6037991812345678", HolderName: "Ali Rezaei",
	}}
	for _, m := range mutate {
		m(cfg)
	}
	if err := (&service.SalesConfigService{}).Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func (b *shopBot) off(t *testing.T) {
	t.Helper()
	b.configure(t, func(c *service.SalesConfig) { c.TelegramShop = false })
}

// tap presses a button of a private-chat message as the given account.
func (b *shopBot) tap(from int64, data string) { b.tapIn(from, data, from, telego.ChatTypePrivate) }

// tapIn presses a button of a message that sits in another chat, such as the
// receipts group.
func (b *shopBot) tapIn(from int64, data string, chat int64, kind string) {
	b.tb.answerShopCallback(&telego.CallbackQuery{
		ID: "q", From: telego.User{ID: from, FirstName: "Ali"}, Data: data,
		Message: &telego.Message{MessageID: 50, Chat: telego.Chat{ID: chat, Type: kind}},
	})
}

// say delivers a typed message the way the router does: to the pending shop
// state, if any.
func (b *shopBot) say(from int64, text string) bool {
	return b.sayMessage(from, &telego.Message{Text: text})
}

func (b *shopBot) sayMessage(from int64, m *telego.Message) bool {
	m.From = &telego.User{ID: from, FirstName: "Ali"}
	m.Chat = telego.Chat{ID: from, Type: telego.ChatTypePrivate}
	actor := messageActor(*m)
	state, ok := userStateMgr.get(actor)
	if !ok {
		return false
	}
	return b.tb.handleShopInput(m, actor, state)
}

func (b *shopBot) orders(tg int64) []model.Order {
	orders, _ := (&service.OrderService{}).TelegramOrders(tg, 20)
	return orders
}

func (b *shopBot) order(t *testing.T, id int) *model.Order {
	t.Helper()
	o, err := (&service.OrderService{}).GetOrder(id)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// cardOrderInReview buys a plan by card and sends a typed receipt.
func (b *shopBot) cardOrderInReview(t *testing.T, tg int64) *model.Order {
	t.Helper()
	b.tap(tg, "shop:p:"+strconv.Itoa(b.planID))
	b.tap(tg, "shop:k:n")
	b.tap(tg, "shop:g:card")
	orders := b.orders(tg)
	if len(orders) == 0 {
		t.Fatal("no order was created")
	}
	oid := strconv.Itoa(orders[0].Id)
	b.tap(tg, "shop:rc:"+oid)
	if !b.say(tg, "482913") {
		t.Fatal("receipt text was not consumed")
	}
	o := b.order(t, orders[0].Id)
	if o.Status != model.OrderStatusReview {
		t.Fatalf("order = %s, want review", o.Status)
	}
	return o
}
