package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

func setupOutput(t *testing.T, args ...string) map[string]string {
	t.Helper()
	var out bytes.Buffer
	if err := cmdSetup(args, &out); err != nil {
		t.Fatalf("setup %v: %v", args, err)
	}
	kv := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		k, v, _ := strings.Cut(line, "=")
		kv[k] = v
	}
	return kv
}

// A Kharej end made by `wattun setup` must hand out a token that the Iran setup turns into a
// config the daemon's own parser accepts, with the same key on both sides.
func TestSetupKharejThenIranPairUp(t *testing.T) {
	kdir, idir := t.TempDir(), t.TempDir()
	k := setupOutput(t, "-dir", kdir, "-name", "k", "-role", "kharej", "-carrier", "tcp", "-addr", "198.51.100.4", "-listen", "24999", "-ports", "443,53/udp")
	tok, err := spec.ParseToken(k["TOKEN"])
	if err != nil {
		t.Fatalf("kharej token: %v", err)
	}
	if tok.Peer != "198.51.100.4:24999" || tok.Allow != "443,53/udp" {
		t.Fatalf("token = %+v, want peer 198.51.100.4:24999 and allow 443,53/udp", tok)
	}
	kcfg, err := os.ReadFile(k["CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	parsedK, err := parseConfig(string(kcfg), nil)
	if err != nil {
		t.Fatalf("daemon cannot parse the Kharej config: %v\n%s", err, kcfg)
	}
	if got := parsedK.tcpPorts(); len(got) != 1 || got[0] != 443 {
		t.Fatalf("Kharej allow list = %v/%v, want tcp 443 and udp 53", got, parsedK.udpPorts())
	}

	i := setupOutput(t, "-dir", idir, "-name", "i", "-token", k["TOKEN"], "-map", "8443:443,5353:53/udp")
	icfg, err := os.ReadFile(i["CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	parsedI, err := parseConfig(string(icfg), nil)
	if err != nil {
		t.Fatalf("daemon cannot parse the Iran config: %v\n%s", err, icfg)
	}
	if m := parsedI.tcpMaps(); len(m) != 1 || m[0] != (portMap{8443, 443}) {
		t.Fatalf("Iran tcp maps = %v, want 8443->443", m)
	}
	if v, _ := parsedI.get("peer"); v != "198.51.100.4:24999" {
		t.Fatalf("Iran peer = %q", v)
	}
	kkey, _ := os.ReadFile(filepath.Join(kdir, "k.key"))
	ikey, _ := os.ReadFile(filepath.Join(idir, "i.key"))
	if len(kkey) < 40 || string(kkey) != string(ikey) {
		t.Fatalf("both ends must hold the same key, got %q and %q", kkey, ikey)
	}
}

func TestSetupRefusesWithoutPorts(t *testing.T) {
	var out bytes.Buffer
	tok := spec.Token{Carrier: "tcp", Peer: "h:1", Key: "longenoughkey"}.Encode()
	if err := cmdSetup([]string{"-dir", t.TempDir(), "-token", tok}, &out); err == nil {
		t.Fatal("an Iran end with nothing to publish was accepted")
	}
	if err := cmdSetup([]string{"-dir", t.TempDir(), "-role", "kharej", "-carrier", "tcp"}, &out); err == nil {
		t.Fatal("a Kharej end without its public address was accepted")
	}
}
