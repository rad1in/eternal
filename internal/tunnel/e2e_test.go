//go:build !windows

package tunnel

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/probe"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

// These tests run the real wattun daemon. Point WATTUN_E2E_BINARY at a build of ./cmd/wattun;
// without it they skip, like the other end-to-end suites.
func e2eBinary(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("WATTUN_E2E_BINARY")
	if bin == "" {
		t.Skip("WATTUN_E2E_BINARY is not set")
	}
	return bin
}

func echoServer(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func roundTrip(t *testing.T, port int, msg string) (string, error) {
	t.Helper()
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 3*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err := c.Write([]byte(msg)); err != nil {
		return "", err
	}
	buf := make([]byte, len(msg))
	_, err = io.ReadFull(c, buf)
	return string(buf), err
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A Kharej end and an Iran end, each under its own manager and state dir, carry bytes from
// the Iran listener to a service on the Kharej side; a port added later is hot-reloaded and
// a port the Kharej end does not list stays unreachable.
func TestRealTunnelCarriesTraffic(t *testing.T) {
	bin := e2eBinary(t)
	kMgr := NewManager(t.TempDir(), func() string { return bin })
	iMgr := NewManager(t.TempDir(), func() string { return bin })
	t.Cleanup(kMgr.StopAll)
	t.Cleanup(iMgr.StopAll)

	echo1, echo2, forbidden := echoServer(t), echoServer(t), echoServer(t)
	carrier := freePort(t)
	key, err := spec.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	l1, l2, lForbidden := freePort(t), freePort(t), freePort(t)

	kharej := EndConfig{
		Uuid: "k", Name: "e2e", Role: spec.RoleKharej, Enable: true, Carrier: "tcp", Key: key,
		ListenPorts: map[string]int{"tcp": carrier},
		Ports:       []spec.PortMap{{Listen: echo1, Target: echo1}},
	}
	iran := EndConfig{
		Uuid: "i", Name: "e2e", Role: spec.RoleIran, Enable: true, Carrier: "tcp", Key: key,
		Peer:  "127.0.0.1:" + strconv.Itoa(carrier),
		Ports: []spec.PortMap{{Listen: l1, Target: echo1}, {Listen: lForbidden, Target: forbidden}},
	}
	if err := kMgr.Ensure(kharej); err != nil {
		t.Fatal(err)
	}
	if err := iMgr.Ensure(iran); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	eventually(t, "the link to come up", func() bool {
		st, ok := iMgr.Status(ctx, "i")
		return ok && st.State == "up"
	})
	if st, _ := kMgr.Status(ctx, "k"); st.Carrier != "tcp" {
		t.Errorf("Kharej reports carrier %q for its client, want tcp: %+v", st.Carrier, st)
	}

	got, err := roundTrip(t, l1, "hello through the tunnel")
	if err != nil || got != "hello through the tunnel" {
		t.Fatalf("echo through the tunnel = %q, %v", got, err)
	}
	if _, err := roundTrip(t, lForbidden, "nope"); err == nil {
		t.Fatal("a target the Kharej end does not list was reachable")
	}

	pid := mustStatus(t, iMgr, "i").Pid
	kharej.Ports = append(kharej.Ports, spec.PortMap{Listen: echo2, Target: echo2})
	iran.Ports = append(iran.Ports, spec.PortMap{Listen: l2, Target: echo2})
	if err := kMgr.Ensure(kharej); err != nil {
		t.Fatal(err)
	}
	if err := iMgr.Ensure(iran); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the added port to be hot-loaded", func() bool {
		got, err := roundTrip(t, l2, "second port")
		return err == nil && got == "second port"
	})
	if after := mustStatus(t, iMgr, "i"); after.Pid != pid || after.State != "up" {
		t.Fatalf("adding a port changed pid %d -> %d (state %s); it must not restart", pid, after.Pid, after.State)
	}
	if res, err := iMgr.SpeedTest(ctx, "i", 2); err != nil || res.Error != "" || res.DownMBps <= 0 {
		t.Errorf("speed test = %+v, %v", res, err)
	}
}

func mustStatus(t *testing.T, m *Manager, uuid string) EndStatus {
	t.Helper()
	st, ok := m.Status(context.Background(), uuid)
	if !ok {
		t.Fatalf("no status for %s", uuid)
	}
	return st
}

// The auto probe starts throwaway clients against a Kharej server that serves several
// carriers and must pick one that really connects and moves data.
func TestRealProbePicksAWorkingCarrier(t *testing.T) {
	bin := e2eBinary(t)
	dir := t.TempDir()
	key, _ := spec.NewKey()
	tcpPort, udpPort := freePort(t), freePort(t)
	server := spec.Spec{
		Role: spec.RoleKharej, Carrier: "tcp+udp", Key: key, KeyFile: filepath.Join(dir, "k.key"),
		Metrics: "127.0.0.1:" + strconv.Itoa(freePort(t)),
		Listen:  map[string]int{"tcp": tcpPort, "udp": udpPort},
	}
	text, err := server.Render()
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(server.KeyFile, []byte(key), 0o600)
	cfg := filepath.Join(dir, "k.yml")
	_ = os.WriteFile(cfg, []byte(text), 0o600)
	r := newRunner("probe-server", bin, []string{"-config", cfg})
	r.begin()
	t.Cleanup(r.stop)
	time.Sleep(time.Second)

	tok := spec.Token{
		Carrier: spec.CarrierAuto, Peer: "127.0.0.1", Key: key,
		Plan: spec.FormatPlan(map[string]int{"tcp": tcpPort, "udp": udpPort}),
	}
	res, err := probe.Run(context.Background(), probe.Options{Binary: bin, Token: tok, WorkDir: filepath.Join(dir, "p"), LinkWait: 6 * time.Second, MB: 2})
	if err != nil {
		t.Fatal(err)
	}
	best, ok := probe.Best(res)
	if !ok || (best.Carrier != "tcp" && best.Carrier != "udp") || best.DownMBps <= 0 {
		t.Fatalf("probe results = %+v", res)
	}
}
