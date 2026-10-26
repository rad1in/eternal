package database_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

// The tunnel table has to exist after a plain InitDB, keep its nested columns intact and
// never serialise the sealed key.
func TestTunnelEndRoundTripsThroughTheDatabase(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	node := 7
	in := model.TunnelEnd{
		Uuid: "u-1", PairUuid: "p-1", Name: "de-ir", Role: model.TunnelRoleIran, NodeId: &node, Enable: true,
		Carrier: "tcp", Peer: "203.0.113.7:4443", KeyEnc: "enc:v1:secret",
		ListenPorts: map[string]int{"tcp": 4443},
		Ports:       []spec.PortMap{{Listen: 443, Target: 443, RateMBps: 5}, {Listen: 53, Target: 53, UDP: true}},
		Options:     spec.EndOptions{FakeSni: "www.cloudflare.com", FakeMode: "rst", TlsSmallCH: true},
	}
	if err := database.GetDB().Create(&in).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	local := model.TunnelEnd{Uuid: "u-2", PairUuid: "p-1", Name: "de-ir", Role: model.TunnelRoleKharej, State: model.TunnelStateUnknown}
	if err := database.GetDB().Create(&local).Error; err != nil {
		t.Fatalf("create local end: %v", err)
	}

	var got, gotLocal model.TunnelEnd
	if err := database.GetDB().Where("uuid = ?", "u-1").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Where("uuid = ?", "u-2").First(&gotLocal).Error; err != nil {
		t.Fatal(err)
	}
	if got.NodeId == nil || *got.NodeId != 7 || gotLocal.NodeId != nil {
		t.Fatalf("node ids = %v / %v, want 7 and nil (this panel)", got.NodeId, gotLocal.NodeId)
	}
	if !reflect.DeepEqual(got.Ports, in.Ports) || got.Options != in.Options || !reflect.DeepEqual(got.ListenPorts, in.ListenPorts) {
		t.Fatalf("nested columns changed: %+v", got)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "keyEnc") {
		t.Fatalf("the sealed key leaked into JSON: %s", raw)
	}
}
