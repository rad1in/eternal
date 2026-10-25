package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

const miniAppBotToken = "123456:MINI-APP-TOKEN"

func signedLaunch(tg int64) string {
	fields := map[string]string{
		"auth_date": strconv.FormatInt(time.Now().Unix(), 10),
		"user":      `{"id":` + strconv.FormatInt(tg, 10) + `,"first_name":"Ali","language_code":"fa"}`,
	}
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
	secret.Write([]byte(miniAppBotToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	q := url.Values{}
	for k, v := range fields {
		q.Set(k, v)
	}
	q.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return q.Encode()
}

func (h *walletHarness) callAs(t *testing.T, token, method, path, body string) (int, shopReply) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(shopSessionHeader, token)
	req.RemoteAddr = "203.0.113.7:4444"
	w := httptest.NewRecorder()
	h.engine.ServeHTTP(w, req)
	var reply shopReply
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	return w.Code, reply
}

func (h *walletHarness) enableMiniApp(t *testing.T) {
	t.Helper()
	cfg := &service.SalesConfig{
		PublicEnabled: true, WalletEnabled: true, PublicBaseURL: "https://shop.example.com",
		Card: service.CardPayConfig{Enabled: true, CardNumber: "6037991812345678"},
		Bot:  service.ShopBotConfig{MiniApp: true},
	}
	if err := (&service.SalesConfigService{}).Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Save(&model.Setting{Key: "tgBotToken", Value: miniAppBotToken}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestMiniAppSessionBindsOrdersToTheVerifiedTelegramAccount(t *testing.T) {
	h := newWalletHarness(t)
	h.enableMiniApp(t)

	code, reply := h.call(t, http.MethodPost, "/shop/api/telegram/session", `{"initData":"hash=00&auth_date=1"}`)
	if code != http.StatusOK || reply.Success || reply.Msg != service.ErrProofInvalid {
		t.Fatalf("forged launch = %d %+v, want proof_invalid", code, reply)
	}

	body, _ := json.Marshal(map[string]string{"initData": signedLaunch(555)})
	_, reply = h.call(t, http.MethodPost, "/shop/api/telegram/session", string(body))
	var sess service.ShopSession
	if err := json.Unmarshal(reply.Obj, &sess); err != nil || !reply.Success || sess.TgId != 555 || sess.Token == "" {
		t.Fatalf("session = %+v %+v, %v", reply, sess, err)
	}

	order := `{"planId":` + strconv.Itoa(h.plan.Id) + `,"kind":"new","gateway":"card","tgId":999}`
	code, reply = h.callAs(t, sess.Token, http.MethodPost, "/shop/api/orders", order)
	if code != http.StatusOK || !reply.Success {
		t.Fatalf("order = %d %+v", code, reply)
	}
	orders, _ := (&service.OrderService{}).TelegramOrders(555, 5)
	if len(orders) != 1 || orders[0].ClientEmail != "tg555" {
		t.Fatalf("orders of 555 = %+v, want one with the generated tg555 username", orders)
	}
	if other, _ := (&service.OrderService{}).TelegramOrders(999, 5); len(other) != 0 {
		t.Fatal("the body's tgId bound an order: identity must come from the verified session only")
	}

	code, _ = h.callAs(t, sess.Token+"x", http.MethodPost, "/shop/api/orders", order)
	if code != http.StatusUnauthorized {
		t.Fatalf("tampered token = %d, want 401", code)
	}
	_, reply = h.call(t, http.MethodPost, "/shop/api/orders", `{"planId":`+strconv.Itoa(h.plan.Id)+`,"kind":"new","gateway":"card"}`)
	if reply.Success {
		t.Fatalf("an anonymous order with no username = %+v, want it refused", reply)
	}
}

func TestMiniAppWalletNeedsNoProofForOwnClientsOnly(t *testing.T) {
	h := newWalletHarness(t)
	h.enableMiniApp(t)
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "alice").Update("tg_id", 555).Error; err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"initData": signedLaunch(555)})
	_, reply := h.call(t, http.MethodPost, "/shop/api/telegram/session", string(body))
	var sess service.ShopSession
	_ = json.Unmarshal(reply.Obj, &sess)

	_, reply = h.callAs(t, sess.Token, http.MethodGet, "/shop/api/wallet?email=alice", "")
	if !reply.Success {
		t.Fatalf("own wallet = %+v", reply)
	}
	body, _ = json.Marshal(map[string]string{"initData": signedLaunch(556)})
	_, reply = h.call(t, http.MethodPost, "/shop/api/telegram/session", string(body))
	var other service.ShopSession
	_ = json.Unmarshal(reply.Obj, &other)
	_, reply = h.callAs(t, other.Token, http.MethodGet, "/shop/api/wallet?email=alice", "")
	if reply.Success || reply.Msg != service.ErrEmailUnknown {
		t.Fatalf("a stranger's view of alice's wallet = %+v, want email_unknown", reply)
	}
}
