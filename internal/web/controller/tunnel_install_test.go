package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func installRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("base_path", "/base/"); c.Next() })
	NewTunnelInstallController(r.Group("/base"))
	return r
}

func TestInstallScriptCarriesThePanelAddressItWasFetchedFrom(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/base/install-tunnel.sh", nil)
	req.Host = "panel.example.com:2053"
	w := httptest.NewRecorder()
	installRouter().ServeHTTP(w, req)
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.HasPrefix(body, "#!/bin/bash") {
		t.Fatalf("response = %d %.60q", w.Code, body)
	}
	if !strings.Contains(body, `PANEL_URL="http://panel.example.com:2053/base"`) {
		t.Fatalf("the script does not know the panel address:\n%.400s", body)
	}
}

// The script runs as root; a crafted Host header must never become shell text.
func TestInstallScriptDropsAHostileHostHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/base/install-tunnel.sh", nil)
	req.Host = `x.example/";touch /tmp/pwned;"`
	w := httptest.NewRecorder()
	installRouter().ServeHTTP(w, req)
	if strings.Contains(w.Body.String(), "pwned") || !strings.Contains(w.Body.String(), `PANEL_URL=""`) {
		t.Fatalf("hostile Host reached the script:\n%.400s", w.Body.String())
	}
}

func TestWattunBinaryIsServedOnlyForThePanelsOwnArchitecture(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the panel serves its wattun on Linux only")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	r := installRouter()
	get := func(arch string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/base/install-tunnel/wattun/"+arch, nil))
		return w
	}
	if w := get(runtime.GOARCH); w.Code != http.StatusNotFound {
		t.Fatalf("no binary installed: %d, want 404 so the script falls back to GitHub", w.Code)
	}
	name := "wattun-linux-" + runtime.GOARCH
	if err := os.WriteFile(filepath.Join(dir, name), []byte("ELF"), 0o755); err != nil {
		t.Fatal(err)
	}
	if w := get(runtime.GOARCH); w.Code != http.StatusOK || w.Body.String() != "ELF" {
		t.Fatalf("own arch: %d %q", w.Code, w.Body.String())
	}
	other := "s390x"
	if runtime.GOARCH == "s390x" {
		other = "arm64"
	}
	if w := get(other); w.Code != http.StatusNotFound {
		t.Fatalf("serving a binary built for another CPU: %d", w.Code)
	}
	if w := get("../../etc/passwd"); w.Code != http.StatusNotFound {
		t.Fatalf("path tricks: %d", w.Code)
	}
}
