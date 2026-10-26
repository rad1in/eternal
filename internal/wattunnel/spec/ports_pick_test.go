package spec

import "testing"

func TestPickListenPortsAvoidsBusyAndDuplicates(t *testing.T) {
	busy := func(proto string, port int) bool { return proto == "tcp" && (port == 4443 || port == 2053) }
	got, err := PickListenPorts([]string{"icmp", "udp", "tcp", "wss"}, busy)
	if err != nil {
		t.Fatalf("PickListenPorts: %v", err)
	}
	if _, ok := got["icmp"]; ok {
		t.Errorf("icmp has no port, got %v", got)
	}
	if got["udp"] != 4443 {
		t.Errorf("udp = %d, want its preferred 4443 (only tcp 4443 is busy)", got["udp"])
	}
	if got["tcp"] == 4443 || got["tcp"] == 0 {
		t.Errorf("tcp = %d, must dodge busy 4443", got["tcp"])
	}
	if got["wss"] == 2053 || got["wss"] == got["tcp"] {
		t.Errorf("wss = %d clashes with a busy or already chosen port (tcp=%d)", got["wss"], got["tcp"])
	}
}

func TestPickListenPortsFallsBackToHighRange(t *testing.T) {
	busy := func(_ string, port int) bool { return port < 24440 }
	got, err := PickListenPorts([]string{"tcp"}, busy)
	if err != nil || got["tcp"] < 24440 || got["tcp"] > 24999 {
		t.Fatalf("PickListenPorts = %v, %v; want a port in 24440-24999", got, err)
	}
	if _, err := PickListenPorts([]string{"tcp"}, func(string, int) bool { return true }); err == nil {
		t.Fatal("no free port must be an error")
	}
}
