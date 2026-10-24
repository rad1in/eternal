package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
)

func parseLines(out string) map[string]string {
	kv := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			kv[k] = v
		}
	}
	return kv
}

func TestSetupInitPrintsWhatTheInstallerNeedsAndRefusesAConfiguredPanel(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	t.Cleanup(func() { _ = database.CloseDB(); global.SetSetupPending(false) })

	var out bytes.Buffer
	if code := runSetupInit(&out, 0); code != setupInitOK {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	kv := parseLines(out.String())
	if kv["setup_state"] != "pending" || len(kv["setup_token"]) < 32 || kv["setup_port"] == "" {
		t.Fatalf("output lacks state, port or token:\n%s", out.String())
	}

	var again bytes.Buffer
	if code := runSetupInit(&again, 0); code != setupInitOK {
		t.Fatalf("second run exit %d", code)
	}
	next := parseLines(again.String())
	if next["setup_port"] != kv["setup_port"] || next["setup_token"] == kv["setup_token"] {
		t.Fatalf("a re-run keeps the port and rotates the token: %v then %v", kv, next)
	}

	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	if err := (&panel.UserService{}).UpdateFirstUser("someone", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	var done bytes.Buffer
	if code := runSetupInit(&done, 0); code != setupInitConfigured || !strings.Contains(done.String(), "setup_state=configured") {
		t.Fatalf("a configured panel: exit %d, %q", code, done.String())
	}
}
