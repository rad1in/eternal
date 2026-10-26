package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

// TestMain turns the test binary into a stand-in for wattun when WATTUN_FAKE is set: it
// serves /stats and /speedtest on the -metrics address the way the real daemon does.
func TestMain(m *testing.M) {
	if os.Getenv("WATTUN_FAKE") == "1" {
		runFakeDaemon()
		return
	}
	os.Exit(m.Run())
}

func runFakeDaemon() {
	var carrier, metrics string
	for i, a := range os.Args {
		switch a {
		case "-carrier":
			carrier = os.Args[i+1]
		case "-metrics":
			metrics = os.Args[i+1]
		}
	}
	speeds := map[string]float64{}
	for _, kv := range strings.Split(os.Getenv("WATTUN_FAKE_SPEEDS"), ",") {
		var c string
		var v float64
		if _, err := fmt.Sscanf(strings.Replace(kv, "=", " ", 1), "%s %f", &c, &v); err == nil {
			speeds[c] = v
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"alive": speeds[carrier] > 0, "srtt_ms": 40.0})
	})
	mux.HandleFunc("/speedtest", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mb_per_s": speeds[carrier]})
	})
	l, err := net.Listen("tcp", metrics)
	if err != nil {
		os.Exit(3)
	}
	_ = http.Serve(l, mux)
}

func fakeBinary(t *testing.T, speeds string) string {
	t.Helper()
	t.Setenv("WATTUN_FAKE", "1")
	t.Setenv("WATTUN_FAKE_SPEEDS", speeds)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func autoToken() spec.Token {
	return spec.Token{Carrier: "auto", Peer: "203.0.113.7", Key: "longenoughkey", Plan: "d=udp:4443,tcp:4444,tls:8443,wss:2053"}
}

func TestRunRanksWorkingCarriersBySpeed(t *testing.T) {
	bin := fakeBinary(t, "tcp=30,udp=55,wss=0")
	res, err := Run(context.Background(), Options{Binary: bin, Token: autoToken(), WorkDir: t.TempDir(), LinkWait: 1500 * time.Millisecond, MB: 1})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	best, ok := Best(res)
	if !ok || best.Carrier != "udp" || best.Peer != "203.0.113.7:4443" {
		t.Fatalf("Best = %+v, %v; want udp on 203.0.113.7:4443", best, ok)
	}
	byCarrier := map[string]CarrierResult{}
	for _, r := range res {
		byCarrier[r.Carrier] = r
	}
	if !byCarrier["tcp"].OK || byCarrier["tcp"].DownMBps != 30 {
		t.Errorf("tcp result = %+v, want ok at 30 MB/s", byCarrier["tcp"])
	}
	for _, dead := range []string{"tls", "wss"} {
		if byCarrier[dead].OK || byCarrier[dead].Why == "" {
			t.Errorf("%s result = %+v, want a failure with a reason", dead, byCarrier[dead])
		}
	}
	if len(res) < 4 || res[0].Carrier != "udp" || res[1].Carrier != "tcp" {
		t.Errorf("results are not ranked best first: %+v", res)
	}
}

func TestRunWithNothingAlive(t *testing.T) {
	bin := fakeBinary(t, "")
	res, err := Run(context.Background(), Options{Binary: bin, Token: autoToken(), WorkDir: t.TempDir(), LinkWait: 1500 * time.Millisecond})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if best, ok := Best(res); ok {
		t.Fatalf("Best = %+v with every carrier dead", best)
	}
}

func TestRunRejectsNonAutoToken(t *testing.T) {
	tok := spec.Token{Carrier: "tcp", Peer: "h:1", Key: "longenoughkey"}
	if _, err := Run(context.Background(), Options{Binary: "x", Token: tok, WorkDir: t.TempDir()}); err == nil {
		t.Fatal("Run accepted a token that is not auto")
	}
}
