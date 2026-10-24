package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
)

type setupHarness struct {
	engine *gin.Engine
	token  string
}

func newSetupHarness(t *testing.T, open bool) *setupHarness {
	t.Helper()
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	t.Cleanup(func() { global.SetSetupPending(false) })
	old := distFS
	SetDistFS(fstest.MapFS{"dist/setup.html": {Data: []byte(`<html><head></head><script src="/assets/setup.js"></script></html>`)}})
	t.Cleanup(func() { SetDistFS(old) })
	gin.SetMode(gin.TestMode)
	h := &setupHarness{engine: gin.New()}
	svc := panel.NewSetupService(func() {})
	newSetupController(h.engine.Group("/setup"), func(c *gin.Context) { c.Status(http.StatusNotFound) }, svc)
	if open {
		tok, _, err := svc.Init(0)
		if err != nil {
			t.Fatal(err)
		}
		h.token = tok
	}
	return h
}

func (h *setupHarness) do(method, path, token, body string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.9:5000"
	if token != "" {
		req.Header.Set(setupTokenHeader, token)
	}
	w := httptest.NewRecorder()
	h.engine.ServeHTTP(w, req)
	var reply map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	return w, reply
}

func TestSetupPageOpensOnlyForTheLinkWithTheToken(t *testing.T) {
	h := newSetupHarness(t, true)
	w, _ := h.do(http.MethodGet, "/setup?t="+h.token, "", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `src="/setup/assets/setup.js"`) || !strings.Contains(w.Body.String(), `window.X_UI_BASE_PATH="/setup/"`) {
		t.Fatalf("page = %d %q; want the bundle with assets and the runtime base path under /setup", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("the token is in the URL: the page must not be cached or leaked in referrers, headers = %v", w.Header())
	}
	for _, path := range []string{"/setup", "/setup/", "/setup?t=nope", "/setup?t=" + h.token + "x"} {
		if w, _ := h.do(http.MethodGet, path, "", ""); w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want it to look like any unknown path", path, w.Code)
		}
	}
}

func TestSetupIsInvisibleOnAPanelThatIsAlreadySetUp(t *testing.T) {
	h := newSetupHarness(t, false)
	if w, _ := h.do(http.MethodGet, "/setup?t=anything", "", ""); w.Code != http.StatusNotFound {
		t.Fatalf("page = %d, want 404 when no wizard is open", w.Code)
	}
	_, reply := h.do(http.MethodGet, "/setup/api/status", "anything", "")
	if reply["success"] != false || reply["msg"] != "already_done" {
		t.Fatalf("status = %v, want already_done", reply)
	}
}

func TestSetupAPIWantsTheTokenAndThrottlesGuesses(t *testing.T) {
	h := newSetupHarness(t, true)
	_, reply := h.do(http.MethodGet, "/setup/api/status", "", "")
	if reply["msg"] != "bad_token" {
		t.Fatalf("no token = %v, want bad_token", reply)
	}
	var last any
	for range setupFailLimit + 2 {
		_, r := h.do(http.MethodPost, "/setup/api/finish", "guess", `{}`)
		last = r["msg"]
	}
	if last != "rate_limited" {
		t.Fatalf("after many wrong tokens the answer is %v, want rate_limited", last)
	}
	_, ok := h.do(http.MethodGet, "/setup/api/status", h.token, "")
	if ok["success"] != true {
		t.Fatalf("the right token must still work once the guesser is throttled: %v", ok)
	}
}

func TestSetupWizardRunsFromStatusToFinish(t *testing.T) {
	h := newSetupHarness(t, true)
	_, st := h.do(http.MethodGet, "/setup/api/status", h.token, "")
	obj, _ := st["obj"].(map[string]any)
	if obj == nil || obj["pending"] != true || obj["suggestedPort"].(float64) < 1024 || !strings.HasPrefix(obj["suggestedBasePath"].(string), "/") {
		t.Fatalf("status = %v", st)
	}
	_, bad := h.do(http.MethodPost, "/setup/api/finish", h.token, `{"username":"ab","password":"long-enough-1","port":28000,"basePath":"/abcdef/"}`)
	if bad["success"] != false || bad["msg"] != "username_invalid" {
		t.Fatalf("bad username = %v", bad)
	}
	port := int(obj["suggestedPort"].(float64))
	body, _ := json.Marshal(map[string]any{"username": "owner_1", "password": "correct-horse-9", "port": port, "basePath": "/k3j9x2m1q8/"})
	_, done := h.do(http.MethodPost, "/setup/api/finish", h.token, string(body))
	res, _ := done["obj"].(map[string]any)
	if done["success"] != true || res == nil || res["basePath"] != "/k3j9x2m1q8/" {
		t.Fatalf("finish = %v", done)
	}
	_, again := h.do(http.MethodPost, "/setup/api/finish", h.token, string(body))
	if again["msg"] != "already_done" {
		t.Fatalf("a second finish = %v, want already_done", again)
	}
}

func TestSetupGateHidesTheWholePanelWhileTheWizardIsOpen(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { global.SetSetupPending(false) })
	engine := gin.New()
	engine.Use(SetupGate(func(c *gin.Context) { c.Status(http.StatusNotFound) }))
	for _, p := range []string{"/", "/login", "/panel/api/server/status", "/abc123/", "/shop/api/plans", "/setupx"} {
		engine.GET(p, func(c *gin.Context) { c.Status(http.StatusOK) })
	}
	engine.GET("/setup", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/setup/assets/x.js", func(c *gin.Context) { c.Status(http.StatusOK) })

	get := func(path string) int {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Code
	}
	global.SetSetupPending(true)
	for _, p := range []string{"/", "/login", "/panel/api/server/status", "/abc123/", "/shop/api/plans", "/setupx"} {
		if code := get(p); code != http.StatusNotFound {
			t.Errorf("GET %s = %d while the wizard is open, want 404", p, code)
		}
	}
	for _, p := range []string{"/setup", "/setup/assets/x.js"} {
		if code := get(p); code != http.StatusOK {
			t.Errorf("GET %s = %d, the wizard itself must stay reachable", p, code)
		}
	}
	global.SetSetupPending(false)
	if code := get("/login"); code != http.StatusOK {
		t.Errorf("GET /login = %d after setup, want the panel back", code)
	}
}

func TestSetupGateAnswers404WhenNoDecoyIsConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { global.SetSetupPending(false) })
	engine := gin.New()
	engine.Use(SetupGate(nil))
	engine.GET("/login", func(c *gin.Context) { c.Status(http.StatusOK) })
	global.SetSetupPending(true)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/login", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /login = %d with no decoy configured, want 404", w.Code)
	}
}
