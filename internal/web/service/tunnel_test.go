package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/tunnel"
	wtprobe "github.com/mhsanaei/3x-ui/v3/internal/wattunnel/probe"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

// fakeHost stands in for the runtime of one host (this panel or a node): it keeps the ends it
// was given and reports them up once told the link works. Everything the service decides
// (order, tokens, ports, rollback) happens before and after these calls.
type fakeHost struct {
	runtime.Runtime
	mu        sync.Mutex
	ends      map[string]tunnel.EndConfig
	order     *[]string
	name      string
	state     string
	failApply error
	probes    []wtprobe.CarrierResult
	probeErr  error
	removed   []string
}

func newFakeHost(name string, order *[]string) *fakeHost {
	return &fakeHost{name: name, order: order, ends: map[string]tunnel.EndConfig{}, state: model.TunnelStateUp}
}

func (f *fakeHost) Name() string { return "fake:" + f.name }

func (f *fakeHost) ApplyTunnel(_ context.Context, e tunnel.EndConfig) (tunnel.EndResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failApply != nil {
		return tunnel.EndResult{}, f.failApply
	}
	if e.Role == spec.RoleKharej && !e.Reverse {
		if e.Carrier == spec.CarrierAuto && e.ListenPorts == nil {
			e.ListenPorts = map[string]int{"udp": 4443, "tcp": 4444}
		} else if e.ListenPorts == nil {
			e.ListenPorts = map[string]int{e.Carrier: 4443}
		}
	}
	f.ends[e.Uuid] = e
	*f.order = append(*f.order, f.name+":apply:"+e.Role)
	return tunnel.EndResult{End: e, Status: tunnel.EndStatus{Uuid: e.Uuid, State: f.state}}, nil
}

func (f *fakeHost) RemoveTunnel(_ context.Context, uuid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.ends, uuid)
	f.removed = append(f.removed, uuid)
	return nil
}

func (f *fakeHost) TunnelStatuses(context.Context) ([]tunnel.EndStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []tunnel.EndStatus
	for uuid, e := range f.ends {
		st := tunnel.EndStatus{Uuid: uuid, State: f.state, Alive: f.state == model.TunnelStateUp, SrttMs: 40}
		if !e.Enable {
			continue
		}
		out = append(out, st)
	}
	return out, nil
}

func (f *fakeHost) TunnelLogs(context.Context, string, int) ([]string, error) {
	return []string{"hello"}, nil
}
func (f *fakeHost) RestartTunnel(context.Context, string) error { return nil }
func (f *fakeHost) ProbeTunnel(context.Context, string) ([]wtprobe.CarrierResult, error) {
	return f.probes, f.probeErr
}

func (f *fakeHost) TunnelSpeedTest(context.Context, string, int) (tunnel.SpeedResult, error) {
	return tunnel.SpeedResult{DownMBps: 10, UpMBps: 9}, nil
}

type tunnelHarness struct {
	svc   *TunnelService
	local *fakeHost
	node  *fakeHost
	order []string
	node1 int
}

func newTunnelHarness(t *testing.T) *tunnelHarness {
	t.Helper()
	setupConflictDB(t)
	h := &tunnelHarness{svc: &TunnelService{}, node1: 1}
	h.local = newFakeHost("local", &h.order)
	h.node = newFakeHost("node", &h.order)
	if err := database.GetDB().Create(&model.Node{Id: 1, Name: "de-fra", Address: "198.51.100.9", Enable: true}).Error; err != nil {
		t.Fatal(err)
	}
	prev := runtime.GetManager()
	m := runtime.NewManager(runtime.LocalDeps{APIPort: func() int { return 0 }, SetNeedRestart: func() {}})
	m.SetLocalRuntimeOverride(h.local)
	m.SetRuntimeOverride(1, h.node)
	runtime.SetManager(m)
	t.Cleanup(func() { runtime.SetManager(prev) })
	SetLocalPublicIPv4(func() string { return "203.0.113.7" })
	return h
}

func (h *tunnelHarness) request() TunnelCreateRequest {
	return TunnelCreateRequest{
		Name:    "de-ir",
		Kharej:  TunnelEndpoint{Kind: TunnelHostNode, NodeId: &h.node1},
		Iran:    TunnelEndpoint{Kind: TunnelHostLocal},
		Carrier: "tcp",
		Ports:   []spec.PortMap{{Listen: 18443, Target: 443}},
	}
}

func rowByRole(t *testing.T, role string) *model.TunnelEnd {
	t.Helper()
	var r model.TunnelEnd
	if err := database.GetDB().Where("role = ?", role).First(&r).Error; err != nil {
		t.Fatalf("no %s row: %v", role, err)
	}
	return &r
}

func TestCreateHandsTheKharejTokenToTheIranEnd(t *testing.T) {
	h := newTunnelHarness(t)
	view, access, err := h.svc.Create(context.Background(), h.request(), "https://panel.example", true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if access != nil {
		t.Fatalf("both ends are managed, nothing should be handed out: %+v", access)
	}
	if got := strings.Join(h.order, ","); got != "node:apply:kharej,local:apply:iran" {
		t.Fatalf("apply order = %s, want the Kharej end first because the Iran end needs its token", got)
	}
	k, i := rowByRole(t, spec.RoleKharej), rowByRole(t, spec.RoleIran)
	if k.NodeId == nil || *k.NodeId != 1 || i.NodeId != nil {
		t.Fatalf("hosts = %v / %v, want node 1 and this panel", k.NodeId, i.NodeId)
	}
	if k.PairUuid == "" || k.PairUuid != i.PairUuid {
		t.Fatalf("pair ids %q / %q must match", k.PairUuid, i.PairUuid)
	}
	kk, _ := h.svc.endOf(k)
	ik, _ := h.svc.endOf(i)
	if kk.Key == "" || kk.Key != ik.Key {
		t.Fatalf("both ends need the same key, got %q and %q", kk.Key, ik.Key)
	}
	if i.Peer != "198.51.100.9:4443" || i.Carrier != "tcp" {
		t.Fatalf("Iran peer/carrier = %q/%q, want the node's address and the port the Kharej host chose", i.Peer, i.Carrier)
	}
	if view.Kharej == nil || view.Iran == nil || view.External != "" || view.Kharej.NodeName != "de-fra" {
		t.Fatalf("view = %+v", view)
	}
	if view.State != model.TunnelStateUp {
		t.Fatalf("state = %q, want up once the Iran end reports in", view.State)
	}
}

func TestCreateAutoPicksTheFastestCarrierAndRollsBackWhenNoneConnects(t *testing.T) {
	h := newTunnelHarness(t)
	req := h.request()
	req.Carrier = spec.CarrierAuto
	h.local.probes = []wtprobe.CarrierResult{
		{Carrier: "udp", Peer: "198.51.100.9:4443", OK: true, DownMBps: 50, UpMBps: 40},
		{Carrier: "tcp", Peer: "198.51.100.9:4444", OK: true, DownMBps: 20, UpMBps: 20},
	}
	view, _, err := h.svc.Create(context.Background(), req, "", true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if view.Carrier != "udp" || !view.Auto || len(view.Probe) != 2 {
		t.Fatalf("view carrier=%q auto=%v probe=%d, want udp, auto, two probe rows", view.Carrier, view.Auto, len(view.Probe))
	}
	if i := rowByRole(t, spec.RoleIran); i.Peer != "198.51.100.9:4443" || i.AutoPlan == "" {
		t.Fatalf("Iran row = peer %q plan %q", i.Peer, i.AutoPlan)
	}

	h2 := newTunnelHarness(t)
	req2 := h2.request()
	req2.Carrier = spec.CarrierAuto
	h2.local.probes = []wtprobe.CarrierResult{{Carrier: "tcp", Why: "no link in 8 s"}}
	if _, _, err := h2.svc.Create(context.Background(), req2, "", true); err == nil || !strings.Contains(err.Error(), "no carrier connected") {
		t.Fatalf("Create = %v, want the no-carrier error", err)
	}
	if len(h2.node.ends) != 0 || len(h2.node.removed) != 1 {
		t.Fatalf("the Kharej end must be taken down again, ends=%d removed=%d", len(h2.node.ends), len(h2.node.removed))
	}
	var n int64
	database.GetDB().Model(&model.TunnelEnd{}).Count(&n)
	if n != 0 {
		t.Fatalf("%d rows survived a failed create", n)
	}
}

func TestCreateWithAnExternalIranEndReturnsTheTokenAndCommand(t *testing.T) {
	h := newTunnelHarness(t)
	req := h.request()
	req.Iran = TunnelEndpoint{Kind: TunnelHostExternal}
	req.Ports = []spec.PortMap{{Listen: 443, Target: 443}, {Listen: 53, Target: 53, UDP: true}}

	if _, _, err := h.svc.Create(context.Background(), req, "https://panel.example/base", false); err == nil {
		t.Fatal("an admin who cannot see tokens must not create a tunnel whose setup command carries one")
	}
	view, access, err := h.svc.Create(context.Background(), req, "https://panel.example/base", true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if access == nil || view.Iran != nil || view.External != spec.RoleIran {
		t.Fatalf("access=%v iran=%v external=%q", access, view.Iran, view.External)
	}
	tok, err := spec.ParseToken(access.Token)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if tok.Peer != "198.51.100.9:4443" || tok.Allow != "443,53/udp" {
		t.Fatalf("token = %+v", tok)
	}
	want := "curl -fsSL https://panel.example/base/install-tunnel.sh | bash -s -- --token " + access.Token + " --map 443,53/udp --name de-ir"
	if access.Command != want {
		t.Fatalf("command = %q\nwant      %q", access.Command, want)
	}
	again, err := h.svc.Access(view.PairUuid, "https://panel.example/base")
	if err != nil || again.Token != access.Token {
		t.Fatalf("Access = %+v, %v; the token must be reproducible from the stored key", again, err)
	}
}

func TestCreateFromAPastedTokenBuildsOnlyTheIranEnd(t *testing.T) {
	h := newTunnelHarness(t)
	tok := spec.Token{Carrier: "wss", Peer: "cdn.example.com:443", Key: "k3yK3yK3yK3yK3y", Path: "/wat", Sni: "cdn.example.com", Allow: "443"}
	req := h.request()
	req.Kharej = TunnelEndpoint{Kind: TunnelHostExternal}
	req.Token = tok.Encode()
	view, access, err := h.svc.Create(context.Background(), req, "", true)
	if err != nil || access != nil {
		t.Fatalf("Create = %v, access %v", err, access)
	}
	if view.Kharej != nil || view.External != spec.RoleKharej {
		t.Fatalf("view = %+v", view)
	}
	i := rowByRole(t, spec.RoleIran)
	if i.Carrier != "wss" || i.Peer != "cdn.example.com:443" || i.Sni != "cdn.example.com" || !i.PeerExternal {
		t.Fatalf("Iran row = %+v", i)
	}
	if got := strings.Join(h.order, ","); got != "local:apply:iran" {
		t.Fatalf("applied %s", got)
	}
}

func TestCreateReverseStartsTheListeningIranEndFirst(t *testing.T) {
	h := newTunnelHarness(t)
	req := h.request()
	req.Reverse = true
	req.ListenPort = 24500
	if _, _, err := h.svc.Create(context.Background(), req, "", true); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := strings.Join(h.order, ","); got != "local:apply:iran,node:apply:kharej" {
		t.Fatalf("apply order = %s, want Iran (it listens) before Kharej (it dials in)", got)
	}
	k := rowByRole(t, spec.RoleKharej)
	if !k.Reverse || k.Peer != "203.0.113.7:24500" {
		t.Fatalf("Kharej row reverse=%v peer=%q, want it to dial the panel's public address on the carrier port", k.Reverse, k.Peer)
	}
}

func TestCreateRefusesBadRequests(t *testing.T) {
	h := newTunnelHarness(t)
	mutate := map[string]func(*TunnelCreateRequest){
		"same server twice": func(r *TunnelCreateRequest) { r.Kharej = TunnelEndpoint{Kind: TunnelHostLocal} },
		"no ports":          func(r *TunnelCreateRequest) { r.Ports = nil },
		"duplicate public":  func(r *TunnelCreateRequest) { r.Ports = []spec.PortMap{{Listen: 1, Target: 1}, {Listen: 1, Target: 2}} },
		"unknown carrier":   func(r *TunnelCreateRequest) { r.Carrier = "quic" },
		"bad name":          func(r *TunnelCreateRequest) { r.Name = "x\ny" },
		"reverse with auto": func(r *TunnelCreateRequest) { r.Reverse, r.Carrier = true, "auto" },
		"both external": func(r *TunnelCreateRequest) {
			r.Kharej, r.Iran = TunnelEndpoint{Kind: "external"}, TunnelEndpoint{Kind: "external"}
		},
		"node without an id":  func(r *TunnelCreateRequest) { r.Kharej = TunnelEndpoint{Kind: TunnelHostNode} },
		"bad fake mode":       func(r *TunnelCreateRequest) { r.Options.FakeMode = "evil" },
		"kharej token absent": func(r *TunnelCreateRequest) { r.Kharej = TunnelEndpoint{Kind: "external"} },
	}
	for name, m := range mutate {
		t.Run(name, func(t *testing.T) {
			req := h.request()
			m(&req)
			if _, _, err := h.svc.Create(context.Background(), req, "", true); err == nil {
				t.Fatalf("Create accepted %+v", req)
			}
		})
	}
	if len(h.order) != 0 {
		t.Fatalf("a refused request must not touch any host, applied %v", h.order)
	}
	if _, _, err := h.svc.Create(context.Background(), h.request(), "", true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.svc.Create(context.Background(), h.request(), "", true); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("a second tunnel with the same name: %v", err)
	}
}

func TestCreateRefusesAPublicPortAnInboundHolds(t *testing.T) {
	h := newTunnelHarness(t)
	seedInboundConflict(t, "in-18443", "", 18443, model.VLESS, `{"network":"tcp"}`, ``)
	_, _, err := h.svc.Create(context.Background(), h.request(), "", true)
	if err == nil || !strings.Contains(err.Error(), "18443") {
		t.Fatalf("Create = %v, want a message naming port 18443", err)
	}
	if len(h.node.ends) != 0 {
		t.Fatalf("the Kharej end made before the refusal must be removed again, %d left", len(h.node.ends))
	}
}

func TestUpdateReachesBothEndsAndUndoesOnFailure(t *testing.T) {
	h := newTunnelHarness(t)
	view, _, err := h.svc.Create(context.Background(), h.request(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	ports := []spec.PortMap{{Listen: 18443, Target: 443}, {Listen: 18080, Target: 80, RateMBps: 5}}
	if _, err := h.svc.Update(context.Background(), view.PairUuid, TunnelUpdateRequest{Ports: &ports}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	for _, role := range []string{spec.RoleKharej, spec.RoleIran} {
		if got := rowByRole(t, role).Ports; len(got) != 2 {
			t.Fatalf("%s ports = %+v, want both mappings", role, got)
		}
	}

	h.local.failApply = errors.New("port clash")
	disable := false
	if _, err := h.svc.Update(context.Background(), view.PairUuid, TunnelUpdateRequest{Enable: &disable}); err == nil {
		t.Fatal("Update succeeded although the Iran host refused")
	}
	if !rowByRole(t, spec.RoleKharej).Enable {
		t.Fatal("the Kharej end was switched off and not switched back after the Iran end failed")
	}
	for _, e := range h.node.ends {
		if !e.Enable {
			t.Fatal("the node still runs the Kharej end disabled")
		}
	}
}

func TestDeleteRemovesBothEnds(t *testing.T) {
	h := newTunnelHarness(t)
	view, _, err := h.svc.Create(context.Background(), h.request(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Delete(context.Background(), view.PairUuid, false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var n int64
	database.GetDB().Model(&model.TunnelEnd{}).Count(&n)
	if n != 0 || len(h.node.ends) != 0 || len(h.local.ends) != 0 {
		t.Fatalf("rows=%d node ends=%d local ends=%d, want none", n, len(h.node.ends), len(h.local.ends))
	}
}

func TestTickAnnouncesALinkOnlyAfterTheChangeHolds(t *testing.T) {
	h := newTunnelHarness(t)
	if _, _, err := h.svc.Create(context.Background(), h.request(), "", true); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if ev := h.svc.Tick(ctx); len(ev) != 0 {
		t.Fatalf("first observation must stay silent, got %v", ev)
	}
	h.local.state, h.node.state = model.TunnelStateDown, model.TunnelStateDown
	if ev := h.svc.Tick(ctx); len(ev) != 0 {
		t.Fatalf("a single down tick may be a blip, got %v", ev)
	}
	h.local.state, h.node.state = model.TunnelStateUp, model.TunnelStateUp
	if ev := h.svc.Tick(ctx); len(ev) != 0 {
		t.Fatalf("a blip that recovers must not alert, got %v", ev)
	}
	h.local.state, h.node.state = model.TunnelStateDown, model.TunnelStateDown
	h.svc.Tick(ctx)
	ev := h.svc.Tick(ctx)
	if len(ev) != 1 || ev[0].Type != eventbus.EventTunnelDown || ev[0].Source != "de-ir" {
		t.Fatalf("events = %+v, want one tunnel.down for de-ir", ev)
	}
	if again := h.svc.Tick(ctx); len(again) != 0 {
		t.Fatalf("an unchanged state must not repeat the alert: %v", again)
	}
	h.local.state, h.node.state = model.TunnelStateUp, model.TunnelStateUp
	h.svc.Tick(ctx)
	ev = h.svc.Tick(ctx)
	if len(ev) != 1 || ev[0].Type != eventbus.EventTunnelUp {
		t.Fatalf("events = %+v, want one tunnel.up", ev)
	}
}

func TestTickReappliesAnEndANodeForgot(t *testing.T) {
	h := newTunnelHarness(t)
	if _, _, err := h.svc.Create(context.Background(), h.request(), "", true); err != nil {
		t.Fatal(err)
	}
	h.node.ends = map[string]tunnel.EndConfig{}
	before := len(h.order)
	h.svc.Tick(context.Background())
	if len(h.node.ends) != 1 || len(h.order) <= before {
		t.Fatalf("the node lost its end and the tick did not give it back (ends=%d)", len(h.node.ends))
	}
}

func TestTickMarksAnUnreachableNodeUnknownNotDown(t *testing.T) {
	h := newTunnelHarness(t)
	if _, _, err := h.svc.Create(context.Background(), h.request(), "", true); err != nil {
		t.Fatal(err)
	}
	m := runtime.GetManager()
	m.SetRuntimeOverride(1, &brokenHost{})
	h.svc.Tick(context.Background())
	if got := rowByRole(t, spec.RoleKharej).State; got != model.TunnelStateUnknown {
		t.Fatalf("state = %q; a node outage is the node's alert, the tunnel is just unknown", got)
	}
}

type brokenHost struct{ runtime.Runtime }

func (*brokenHost) TunnelStatuses(context.Context) ([]tunnel.EndStatus, error) {
	return nil, errors.New("connection refused")
}

func (*brokenHost) ApplyTunnel(context.Context, tunnel.EndConfig) (tunnel.EndResult, error) {
	return tunnel.EndResult{}, errors.New("connection refused")
}
