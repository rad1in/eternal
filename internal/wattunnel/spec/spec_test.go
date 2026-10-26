package spec

import (
	"strings"
	"testing"
)

func TestRenderKharejSingleCarrier(t *testing.T) {
	s := Spec{
		Key: "longenoughkey", Role: RoleKharej, Carrier: "tcp", KeyFile: "/d/k.key", Metrics: "127.0.0.1:19700",
		Listen: map[string]int{"tcp": 4443},
		Ports:  []PortMap{{Listen: 8443, Target: 443}, {Listen: 53, Target: 53, UDP: true}},
	}
	got, err := s.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := `mode: server
key-file: "/d/k.key"
carrier: tcp
listen: ":4443"
metrics: 127.0.0.1:19700
ports:
  - 443
  - 53/udp
`
	if got != want {
		t.Fatalf("Render mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderKharejAutoListensOnEveryCarrier(t *testing.T) {
	s := Spec{
		Key: "longenoughkey", Role: RoleKharej, Carrier: "icmp+udp+tcp", KeyFile: "k", Metrics: "127.0.0.1:1",
		Listen: map[string]int{"udp": 4443, "tcp": 4444},
	}
	got, err := s.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, line := range []string{"carrier: icmp+udp+tcp", `listen-udp: ":4443"`, `listen-tcp: ":4444"`} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("config lacks %q:\n%s", line, got)
		}
	}
	if strings.Contains(got, "\nlisten:") || strings.Contains(got, "listen-icmp") {
		t.Errorf("bonded server must not use plain listen or listen-icmp:\n%s", got)
	}
	if !strings.HasSuffix(got, "ports:\n") {
		t.Errorf("a server without ports still needs an empty ports: list so nothing is reachable:\n%s", got)
	}
}

func TestRenderIranClientWithLimitsAndKit(t *testing.T) {
	s := Spec{
		Key: "longenoughkey", Role: RoleIran, Carrier: "wss", KeyFile: "k", Metrics: "127.0.0.1:2", Peer: "cdn.example.com:443",
		Path: "/wat", Sni: "cdn.example.com",
		Ports: []PortMap{{Listen: 443, Target: 443, RateMBps: 5, QuotaGB: 100}, {Listen: 80, Target: 8080}},
		Options: EndOptions{
			FakeSni: "www.cloudflare.com", FakeMode: "rst", PoolRotate: "30m", Ech: "auto", Fp: "chrome",
			TlsSmallCH: true, MaxMBps: 40, Sessions: 2, Bind: "127.0.0.1",
		},
	}
	got, err := s.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, line := range []string{
		"mode: client", "carrier: wss", "peer: cdn.example.com:443", "path: /wat", "sni: cdn.example.com", "host: cdn.example.com",
		"fake-sni: www.cloudflare.com", "fake-mode: rst", "pool-rotate: 30m", "ech: auto", "fp: chrome",
		"tls-small-ch: true", "max-mbps: 40", "sessions: 2", "bind: 127.0.0.1",
		"  - 443", "  - 80:8080", "  - 443 rate=5MB quota=100GB",
	} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("config lacks %q:\n%s", line, got)
		}
	}
	if strings.Contains(got, "  - 80 ") {
		t.Errorf("a port with no limits must not get a limits item:\n%s", got)
	}
}

func TestRenderReverse(t *testing.T) {
	iran := Spec{Key: "longenoughkey", Role: RoleIran, Carrier: "tcp", KeyFile: "k", Metrics: "m:1", Peer: "203.0.113.7:4443", Reverse: true, Listen: map[string]int{"tcp": 4443}}
	got, err := iran.Render()
	if err != nil {
		t.Fatalf("Render iran: %v", err)
	}
	if !strings.Contains(got, "reverse: true\n") || !strings.Contains(got, `listen: ":4443"`+"\n") {
		t.Errorf("reverse Iran must listen for Kharej:\n%s", got)
	}
	kharej := Spec{Key: "longenoughkey", Role: RoleKharej, Carrier: "tcp", KeyFile: "k", Metrics: "m:1", Peer: "198.51.100.9:4443", Reverse: true, Listen: map[string]int{"tcp": 4443}}
	got, err = kharej.Render()
	if err != nil {
		t.Fatalf("Render kharej: %v", err)
	}
	if !strings.Contains(got, "peer: 198.51.100.9:4443\n") || strings.Contains(got, "listen:") {
		t.Errorf("reverse Kharej dials Iran and listens on nothing:\n%s", got)
	}
}

func TestValidateRejects(t *testing.T) {
	good := Spec{Role: RoleIran, Carrier: "tcp", Key: "longenoughkey", Peer: "h:1", Ports: []PortMap{{Listen: 1, Target: 1}}}
	cases := map[string]func(*Spec){
		"short key":          func(s *Spec) { s.Key = "short" },
		"key with newline":   func(s *Spec) { s.Key = "abcdefgh\nmode: x" },
		"unknown carrier":    func(s *Spec) { s.Carrier = "quic" },
		"bonded unknown":     func(s *Spec) { s.Carrier = "tcp+quic" },
		"path injection":     func(s *Spec) { s.Path = "/wat\nmode: server" },
		"no peer":            func(s *Spec) { s.Peer = "" },
		"bad fake mode":      func(s *Spec) { s.Options.FakeMode = "evil" },
		"fake sni newline":   func(s *Spec) { s.Options.FakeSni = "a\nb" },
		"pool rotate bad":    func(s *Spec) { s.Options.PoolRotate = "soon" },
		"ech garbage":        func(s *Spec) { s.Options.Ech = "not base64!" },
		"sessions too many":  func(s *Spec) { s.Options.Sessions = 40 },
		"bind not ip":        func(s *Spec) { s.Options.Bind = "evil.example" },
		"unknown role":       func(s *Spec) { s.Role = "mid" },
		"duplicate public":   func(s *Spec) { s.Ports = []PortMap{{Listen: 1, Target: 1}, {Listen: 1, Target: 2}} },
		"unknown icmp type":  func(s *Spec) { s.IcmpType = "ping" },
		"fp unknown":         func(s *Spec) { s.Options.Fp = "netscape" },
		"max mbps negative":  func(s *Spec) { s.Options.MaxMBps = -1 },
		"fake ttl too large": func(s *Spec) { s.Options.FakeTtl = 300 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := good
			mutate(&s)
			if err := s.Validate("linux"); err == nil {
				t.Fatalf("Validate accepted %+v", s)
			}
		})
	}
	if err := good.Validate("linux"); err != nil {
		t.Fatalf("Validate rejected a good spec: %v", err)
	}
}

func TestValidateWindowsHasNoRawSockets(t *testing.T) {
	s := Spec{Role: RoleKharej, Carrier: "icmp", Key: "longenoughkey", Listen: map[string]int{}}
	if err := s.Validate("windows"); err == nil {
		t.Fatal("icmp accepted on windows")
	}
	s = Spec{Role: RoleIran, Carrier: "tcp", Key: "longenoughkey", Peer: "h:1", Options: EndOptions{FakeSni: "a.example"}}
	if err := s.Validate("windows"); err == nil {
		t.Fatal("fake-sni accepted on windows")
	}
	s.Options = EndOptions{}
	if err := s.Validate("windows"); err != nil {
		t.Fatalf("tcp on windows must work: %v", err)
	}
}

func TestNewKeyIsAcceptedAndUnique(t *testing.T) {
	a, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewKey()
	s := Spec{Role: RoleIran, Carrier: "tcp", Key: a, Peer: "h:1"}
	if err := s.Validate("linux"); err != nil {
		t.Fatalf("a generated key must pass Validate: %v", err)
	}
	if a == b || len(a) != 40 {
		t.Fatalf("keys %q and %q must be 40 chars and differ", a, b)
	}
}

func TestOptionsForEndKeepsOnlyWhatTheEndUnderstands(t *testing.T) {
	all := EndOptions{
		MaxMBps: 10, Fec: "auto", Sessions: 3, Conns: 8, Bind: "127.0.0.1", FakeSni: "a.example", FakeMode: "rst", FakeTtl: 4,
		PoolRotate: "30m", PoolRotateBytes: "1GB", Ech: "auto", Fp: "chrome", TlsSmallCH: true, Governor: true,
	}
	cases := []struct {
		name    string
		role    string
		reverse bool
		want    EndOptions
	}{
		{"iran dials: everything", RoleIran, false, all},
		{"kharej serves: pace and fec only", RoleKharej, false, EndOptions{MaxMBps: 10, Fec: "auto"}},
		{
			"kharej dials in reverse: desync and fingerprint too", RoleKharej, true,
			EndOptions{MaxMBps: 10, Fec: "auto", FakeSni: "a.example", FakeMode: "rst", FakeTtl: 4, Fp: "chrome"},
		},
		{"iran listens in reverse: pace, fec and bind", RoleIran, true, EndOptions{MaxMBps: 10, Fec: "auto", Bind: "127.0.0.1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := all.ForEnd(c.role, c.reverse); got != c.want {
				t.Fatalf("ForEnd = %+v, want %+v", got, c.want)
			}
		})
	}
}
