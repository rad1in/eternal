//go:build !windows

package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

// TestMain doubles as a stand-in wattun: with WATTUN_FAKE=1 the test binary reads the
// -config file it is given and serves /stats and /speedtest on its metrics address.
func TestMain(m *testing.M) {
	if os.Getenv("WATTUN_FAKE") == "1" {
		runFakeDaemon()
		return
	}
	os.Exit(m.Run())
}

func runFakeDaemon() {
	var cfgPath string
	for i, a := range os.Args {
		if a == "-config" {
			cfgPath = os.Args[i+1]
		}
	}
	if os.Getenv("WATTUN_FAKE_MODE") == "die" {
		fmt.Println("config boom: unknown key")
		os.Exit(2)
	}
	text, err := os.ReadFile(cfgPath)
	if err != nil {
		os.Exit(3)
	}
	var metrics string
	for _, line := range strings.Split(string(text), "\n") {
		if v, ok := strings.CutPrefix(line, "metrics: "); ok {
			metrics = strings.TrimSpace(v)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"alive": os.Getenv("WATTUN_FAKE_MODE") != "dead", "srtt_ms": 12.5, "last_rx_age_ms": 800, "sessions": 1, "streams": 2,
			"bytes_in_total": 1000, "bytes_out_total": 2000, "version": "9.9.9", "tls_mode": "go",
			"clients": []map[string]any{{"ip": "198.51.100.20", "carrier": "tcp", "age_ms": 4000}},
			"ports": map[string]any{"443": map[string]any{
				"bytes_up_total": 10, "bytes_down_total": 20, "conns_active": 1, "conns_all_time": 5,
				"quota_limit": 100, "quota_used": 7, "rate_bps_up": 3, "rate_bps_down": 4,
			}},
		})
	})
	mux.HandleFunc("/speedtest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mb_per_s": map[string]float64{"down": 40, "up": 30}[r.URL.Query().Get("dir")]})
	})
	l, err := net.Listen("tcp", metrics)
	if err != nil {
		os.Exit(4)
	}
	_ = http.Serve(l, mux)
}

func testManager(t *testing.T, mode string) *Manager {
	t.Helper()
	t.Setenv("WATTUN_FAKE", "1")
	t.Setenv("WATTUN_FAKE_MODE", mode)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), func() string { return exe })
	t.Cleanup(m.StopAll)
	return m
}

func iranEnd() EndConfig {
	return EndConfig{
		Uuid: "iran-1", PairUuid: "p", Name: "t", Role: spec.RoleIran, Enable: true, Carrier: "tcp",
		Peer: "203.0.113.7:4443", Key: "longenoughkey1234", Ports: []spec.PortMap{{Listen: 8443, Target: 443}},
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func statusOfEnd(m *Manager, uuid string) EndStatus {
	st, _ := m.Status(context.Background(), uuid)
	return st
}

func TestEnsureStartsTheDaemonAndReportsItsStats(t *testing.T) {
	m := testManager(t, "")
	e := iranEnd()
	if err := m.Ensure(e); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	waitFor(t, "the daemon to answer", func() bool { return statusOfEnd(m, e.Uuid).State == "up" })
	st := statusOfEnd(m, e.Uuid)
	if !st.Alive || st.SrttMs != 12.5 || st.Version != "9.9.9" || st.BytesIn != 1000 || st.Streams != 2 {
		t.Fatalf("stats were not mapped: %+v", st)
	}
	if len(st.Ports) != 1 || st.Ports[0].Port != 443 || st.Ports[0].QuotaLimit != 100 || st.Ports[0].ConnsTotal != 5 {
		t.Fatalf("port stats were not mapped: %+v", st.Ports)
	}
	cfg, _ := m.paths(e.Uuid)
	text, err := os.ReadFile(cfg)
	if err != nil || !strings.Contains(string(text), "  - 8443:443\n") {
		t.Fatalf("config file = %q, %v", text, err)
	}
	if fi, _ := os.Stat(cfg); fi.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %v, want 0600 (it names the key file)", fi.Mode().Perm())
	}
}

func TestKharejStatusNamesTheCarrierClientsUse(t *testing.T) {
	m := testManager(t, "")
	e := EndConfig{
		Uuid: "k-1", Name: "t", Role: spec.RoleKharej, Enable: true, Carrier: "auto", Key: "longenoughkey1234",
		ListenPorts: map[string]int{"tcp": 4443, "udp": 4443},
	}
	if err := m.Ensure(e); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	waitFor(t, "the daemon", func() bool { return statusOfEnd(m, e.Uuid).Alive })
	if st := statusOfEnd(m, e.Uuid); st.Carrier != "tcp" || len(st.Clients) != 1 {
		t.Fatalf("status = %+v, want the connected client's carrier", st)
	}
}

func TestPortsOnlyChangeKeepsTheProcessButOthersRestartIt(t *testing.T) {
	m := testManager(t, "")
	e := iranEnd()
	if err := m.Ensure(e); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "start", func() bool { return statusOfEnd(m, e.Uuid).Pid != 0 })
	pid := statusOfEnd(m, e.Uuid).Pid

	e.Ports = append(e.Ports, spec.PortMap{Listen: 53, Target: 53, UDP: true})
	if err := m.Ensure(e); err != nil {
		t.Fatal(err)
	}
	cfg, _ := m.paths(e.Uuid)
	text, _ := os.ReadFile(cfg)
	if !strings.Contains(string(text), "  - 53/udp\n") {
		t.Fatalf("new port missing from the config the daemon hot-reloads:\n%s", text)
	}
	if got := statusOfEnd(m, e.Uuid).Pid; got != pid {
		t.Fatalf("editing ports restarted the daemon (pid %d -> %d); it would drop every live connection", pid, got)
	}

	e.Path = "/other"
	e.Carrier = "ws"
	if err := m.Ensure(e); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "restart on a structural change", func() bool {
		p := statusOfEnd(m, e.Uuid).Pid
		return p != 0 && p != pid
	})
}

func TestACrashedDaemonIsStartedAgain(t *testing.T) {
	m := testManager(t, "")
	e := iranEnd()
	if err := m.Ensure(e); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "start", func() bool { return statusOfEnd(m, e.Uuid).State == "up" })
	pid := statusOfEnd(m, e.Uuid).Pid
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the supervisor to restart it", func() bool {
		st := statusOfEnd(m, e.Uuid)
		return st.State == "up" && st.Pid != pid && st.Restarts >= 1
	})
}

func TestADaemonThatCannotStartShowsItsLastLine(t *testing.T) {
	m := testManager(t, "die")
	e := iranEnd()
	if err := m.Ensure(e); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the failure to show", func() bool {
		st := statusOfEnd(m, e.Uuid)
		return st.State == "error" && strings.Contains(st.LastError, "config boom")
	})
	lines, err := m.Logs(e.Uuid, 10)
	if err != nil || len(lines) == 0 || !strings.Contains(lines[len(lines)-1], "config boom") {
		t.Fatalf("Logs = %v, %v", lines, err)
	}
}

func TestDisableAndRemoveStopTheProcessAndDeleteTheFiles(t *testing.T) {
	m := testManager(t, "")
	e := iranEnd()
	if err := m.Ensure(e); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "start", func() bool { return statusOfEnd(m, e.Uuid).Pid != 0 })
	pid := statusOfEnd(m, e.Uuid).Pid
	cfg, key := m.paths(e.Uuid)

	e.Enable = false
	if err := m.Ensure(e); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Status(context.Background(), e.Uuid); ok {
		t.Fatal("a disabled end must not be managed")
	}
	waitFor(t, "the process to exit", func() bool { return syscall.Kill(pid, 0) != nil })
	for _, f := range []string{cfg, key} {
		if _, err := os.Stat(f); err == nil {
			t.Fatalf("%s survived disabling the end", f)
		}
	}
}

func TestStopAllEndsEveryProcess(t *testing.T) {
	m := testManager(t, "")
	a, b := iranEnd(), iranEnd()
	b.Uuid = "iran-2"
	for _, e := range []EndConfig{a, b} {
		if err := m.Ensure(e); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "both up", func() bool { return statusOfEnd(m, a.Uuid).Pid != 0 && statusOfEnd(m, b.Uuid).Pid != 0 })
	pa, pb := statusOfEnd(m, a.Uuid).Pid, statusOfEnd(m, b.Uuid).Pid
	m.StopAll()
	waitFor(t, "processes gone", func() bool { return syscall.Kill(pa, 0) != nil && syscall.Kill(pb, 0) != nil })
}

func TestEnsureFailsClearlyWithoutTheBinary(t *testing.T) {
	m := NewManager(t.TempDir(), func() string { return "/nonexistent/wattun" })
	err := m.Ensure(iranEnd())
	if err == nil || !strings.Contains(err.Error(), "/nonexistent/wattun") {
		t.Fatalf("Ensure = %v, want an error naming the missing binary", err)
	}
}

func TestSpeedTestRunsOnlyFromTheIranEnd(t *testing.T) {
	m := testManager(t, "")
	i := iranEnd()
	k := EndConfig{Uuid: "k-1", Name: "t", Role: spec.RoleKharej, Enable: true, Carrier: "tcp", Key: "longenoughkey1234", ListenPorts: map[string]int{"tcp": 4443}}
	for _, e := range []EndConfig{i, k} {
		if err := m.Ensure(e); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "start", func() bool { return statusOfEnd(m, i.Uuid).Alive && statusOfEnd(m, k.Uuid).Alive })
	res, err := m.SpeedTest(context.Background(), i.Uuid, 4)
	if err != nil || res.DownMBps != 40 || res.UpMBps != 30 || res.Error != "" {
		t.Fatalf("SpeedTest = %+v, %v", res, err)
	}
	if _, err := m.SpeedTest(context.Background(), k.Uuid, 4); err == nil {
		t.Fatal("a speed test on a Kharej end must be refused")
	}
}

func TestReconcileRemovesWhatIsNoLongerDesired(t *testing.T) {
	m := testManager(t, "")
	a, b := iranEnd(), iranEnd()
	b.Uuid = "iran-2"
	m.Reconcile([]EndConfig{a, b})
	waitFor(t, "both", func() bool { return statusOfEnd(m, a.Uuid).Pid != 0 && statusOfEnd(m, b.Uuid).Pid != 0 })
	m.Reconcile([]EndConfig{a})
	if _, ok := m.Status(context.Background(), b.Uuid); ok {
		t.Fatal("iran-2 should have been stopped")
	}
	if _, ok := m.Status(context.Background(), a.Uuid); !ok {
		t.Fatal("iran-1 should keep running")
	}
}
