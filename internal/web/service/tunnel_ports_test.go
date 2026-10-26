package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/tunnel"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

func seedTunnelEnd(t *testing.T, e model.TunnelEnd) {
	t.Helper()
	if e.Uuid == "" {
		e.Uuid = "seed-" + e.Name
	}
	if err := database.GetDB().Create(&e).Error; err != nil {
		t.Fatalf("seed tunnel end %s: %v", e.Name, err)
	}
}

func TestTunnelPortOwner(t *testing.T) {
	setupConflictDB(t)
	node := 3
	seedInboundConflict(t, "in-4443", "", 4443, model.VLESS, `{"network":"tcp"}`, ``)
	seedInboundConflictNode(t, "node-in-5000", "", 5000, model.VLESS, `{"network":"tcp"}`, ``, &node)
	seedTunnelEnd(t, model.TunnelEnd{Name: "other", Role: model.TunnelRoleKharej, Enable: true, Carrier: "udp", ListenPorts: map[string]int{"udp": 4500}})
	seedTunnelEnd(t, model.TunnelEnd{Name: "paused", Role: model.TunnelRoleKharej, Enable: false, Carrier: "tcp", ListenPorts: map[string]int{"tcp": 4600}})
	webPort, err := (&SettingService{}).GetPort()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		node   *int
		proto  string
		port   int
		ignore string
		want   string // substring of the owner, "" = free
	}{
		{"inbound on the same tcp port", nil, "tcp", 4443, "", "inbound 'in-4443'"},
		{"udp on an inbound's tcp port is fine", nil, "udp", 4443, "", ""},
		{"inbound of another node does not count", nil, "tcp", 5000, "", ""},
		{"same node as the inbound", &node, "tcp", 5000, "", "inbound"},
		{"panel web port", nil, "tcp", webPort, "", "panel web port"},
		{"another tunnel's carrier port", nil, "udp", 4500, "", "tunnel 'other'"},
		{"a disabled tunnel holds nothing", nil, "tcp", 4600, "", ""},
		{"an end never conflicts with itself", nil, "udp", 4500, "seed-other", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := tunnelPortOwner(database.GetDB(), c.node, c.proto, c.port, c.ignore, false)
			if err != nil {
				t.Fatal(err)
			}
			if c.want == "" && got != "" || !strings.Contains(got, c.want) {
				t.Fatalf("owner = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCheckTunnelBindsNamesTheClashingPort(t *testing.T) {
	setupConflictDB(t)
	seedInboundConflict(t, "in-8443", "", 8443, model.VLESS, `{"network":"tcp"}`, ``)
	e := tunnel.EndConfig{Uuid: "new", Name: "n", Role: spec.RoleIran, Carrier: "tcp", Ports: []spec.PortMap{{Listen: 8443, Target: 443}}}
	err := checkTunnelBinds(database.GetDB(), nil, e, nil)
	if err == nil || !strings.Contains(err.Error(), "8443") || !strings.Contains(err.Error(), "inbound") {
		t.Fatalf("checkTunnelBinds = %v, want a message naming port 8443 and the inbound", err)
	}
	e.Ports = []spec.PortMap{{Listen: 8444, Target: 443}}
	if err := checkTunnelBinds(database.GetDB(), nil, e, nil); err != nil {
		t.Fatalf("a free port was refused: %v", err)
	}
}

func TestResolveTunnelListenPicksFreeDistinctPorts(t *testing.T) {
	setupConflictDB(t)
	seedInboundConflict(t, "in-4443", "", 4443, model.VLESS, `{"network":"tcp"}`, ``)
	e := tunnel.EndConfig{Uuid: "k", Role: spec.RoleKharej, Carrier: spec.CarrierAuto}
	if err := resolveTunnelListen(database.GetDB(), nil, &e); err != nil {
		t.Fatalf("resolveTunnelListen: %v", err)
	}
	if e.ListenPorts["udp"] != 4443 {
		t.Errorf("udp = %d, want 4443 (the inbound holds tcp only)", e.ListenPorts["udp"])
	}
	if e.ListenPorts["tcp"] == 4443 || e.ListenPorts["tcp"] == 0 {
		t.Errorf("tcp = %d, must dodge the inbound on 4443", e.ListenPorts["tcp"])
	}
	seen := map[string]string{}
	for c, p := range e.ListenPorts {
		proto := "tcp"
		if c == "udp" {
			proto = "udp"
		}
		k := fmt.Sprintf("%s:%d", proto, p)
		if other, dup := seen[k]; dup {
			t.Errorf("%s and %s share %s port %d", c, other, proto, p)
		}
		seen[k] = c
	}
	if len(e.ListenPorts) != len(autoPortCarriers) {
		t.Errorf("ports = %v, want one per auto carrier", e.ListenPorts)
	}
	kept := tunnel.EndConfig{Uuid: "k2", Role: spec.RoleKharej, Carrier: "tcp", ListenPorts: map[string]int{"tcp": 9999}}
	if err := resolveTunnelListen(database.GetDB(), nil, &kept); err != nil || kept.ListenPorts["tcp"] != 9999 {
		t.Fatalf("an explicit port must be kept: %v %v", kept.ListenPorts, err)
	}
}

// An inbound saved onto a tunnel's carrier port would fail to bind at the next start.
func TestInboundCannotTakeATunnelPort(t *testing.T) {
	setupConflictDB(t)
	seedTunnelEnd(t, model.TunnelEnd{
		Name: "de-ir", Role: model.TunnelRoleIran, Enable: true, Carrier: "tcp",
		Ports: []spec.PortMap{{Listen: 9443, Target: 443}},
	})
	in := &model.Inbound{Port: 9443, Protocol: model.VLESS, StreamSettings: `{"network":"tcp"}`}
	conflict, err := checkPortConflictTx(database.GetDB(), in, 0)
	if err != nil {
		t.Fatal(err)
	}
	if conflict == nil || !strings.Contains(conflict.String(), "tunnel 'de-ir'") {
		t.Fatalf("conflict = %v, want one naming tunnel 'de-ir'", conflict)
	}
	in.StreamSettings = `{"network":"kcp"}`
	if conflict, err = checkPortConflictTx(database.GetDB(), in, 0); err != nil || conflict != nil {
		t.Fatalf("a udp inbound does not clash with a tcp-only tunnel port: %v %v", conflict, err)
	}
}
