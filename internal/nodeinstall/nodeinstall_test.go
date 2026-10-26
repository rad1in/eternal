package nodeinstall

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScriptBakesTheMastersVersion(t *testing.T) {
	got := string(Script("1.4.0"))
	if !strings.Contains(got, `DEFAULT_VERSION="v1.4.0"`) || strings.Contains(got, versionPlaceholder) {
		t.Fatalf("default version line not baked in:\n%s", firstLines(got, 25))
	}
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

type fakeMaster struct {
	srv    *httptest.Server
	joined chan map[string]any
	reply  string
}

func newFakeMaster(t *testing.T, reply string) *fakeMaster {
	t.Helper()
	m := &fakeMaster{joined: make(chan map[string]any, 20), reply: reply}
	mux := http.NewServeMux()
	mux.HandleFunc("/secret/install-node.sh", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("#!/bin/bash\n")) })
	mux.HandleFunc("/secret/join/node", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var got map[string]any
		_ = json.Unmarshal(body, &got)
		m.joined <- got
		_, _ = w.Write([]byte(m.reply))
	})
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

func runScript(t *testing.T, env []string, args ...string) (string, error) {
	t.Helper()
	for _, bin := range []string{"bash", "curl"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not installed", bin)
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "install-node.sh")
	if err := os.WriteFile(path, Script("1.4.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{path}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func installResult(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "install-result.env")
	body := "XUI_ACCESS_URL='http://203.0.113.9:2053/nodepath'\nXUI_API_TOKEN='node-api-token'\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScriptRegistersTheNodeOnTheMasterWithWhatTheInstallerReported(t *testing.T) {
	m := newFakeMaster(t, `{"success":true}`)
	out, err := runScript(t,
		[]string{"XUI_SKIP_INSTALL=1", "XUI_ALLOW_NONROOT=1", "XUI_RESULT_FILE=" + installResult(t)},
		"--panel", m.srv.URL+"/secret/", "--token", "tok-123", "--name", "de-fra-1")
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	got := <-m.joined
	want := map[string]any{
		"token": "tok-123", "name": "de-fra-1", "scheme": "http", "address": "203.0.113.9",
		"port": float64(2053), "basePath": "/nodepath/", "apiToken": "node-api-token", "tlsVerifyMode": "verify",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("joined[%q] = %v, want %v", k, got[k], v)
		}
	}
	if !strings.Contains(out, "de-fra-1 is now a node") {
		t.Errorf("no success line in output:\n%s", out)
	}
}

func TestScriptStopsAtOnceWhenTheMasterRefusesTheToken(t *testing.T) {
	m := newFakeMaster(t, `{"success":false,"msg":"invalid or expired join token"}`)
	out, err := runScript(t,
		[]string{"XUI_SKIP_INSTALL=1", "XUI_ALLOW_NONROOT=1", "XUI_RESULT_FILE=" + installResult(t)},
		"--panel", m.srv.URL+"/secret", "--token", "stale")
	if err == nil {
		t.Fatalf("script succeeded with a refused token:\n%s", out)
	}
	if n := len(m.joined); n != 1 {
		t.Errorf("master was asked %d times, want 1 (a refused token must not be retried)", n)
	}
	if !strings.Contains(out, "The token was refused") {
		t.Errorf("no refusal message in output:\n%s", out)
	}
}

func TestScriptRefusesToRunWithoutAPanelAndAToken(t *testing.T) {
	out, err := runScript(t, []string{"XUI_ALLOW_NONROOT=1"}, "--panel", "http://127.0.0.1:1/x")
	if err == nil || !strings.Contains(out, "--token is required") {
		t.Fatalf("err = %v, output:\n%s", err, out)
	}
}

func TestDetectChangesNothingAndReportsAMissingInstall(t *testing.T) {
	out, err := runScript(t, []string{"XUI_FOLDER=" + t.TempDir()}, "--detect")
	if err != nil {
		t.Fatalf("detect failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "eternal       not installed") {
		t.Errorf("detect output:\n%s", out)
	}
}
