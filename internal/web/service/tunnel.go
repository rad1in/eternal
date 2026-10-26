package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/mhsanaei/3x-ui/v3/internal/crypto/nodetoken"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/tunnel"
	wtprobe "github.com/mhsanaei/3x-ui/v3/internal/wattunnel/probe"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
	rt "github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

// Where a tunnel end runs.
const (
	TunnelHostLocal    = "local"
	TunnelHostNode     = "node"
	TunnelHostExternal = "external"
)

var tunnelNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,62}$`)

var localPublicIPv4 atomic.Pointer[func() string]

// SetLocalPublicIPv4 tells the tunnel service how to learn this server's public address.
func SetLocalPublicIPv4(fn func() string) { localPublicIPv4.Store(&fn) }

// TunnelService creates and manages WatTunnel links between the panel's server, its nodes
// and servers without a panel. State lives in tunnel_ends; the processes run through the
// tunnel runtime of the host each end belongs to.
type TunnelService struct {
	mu        sync.Mutex
	watch     map[string]*tunnelWatch
	lastHeal  map[string]time.Time
	downSince map[string]time.Time
}

type tunnelWatch struct {
	published string
	candidate string
	count     int
	downAt    time.Time
}

// TunnelEndpoint says where one end of a new tunnel runs.
type TunnelEndpoint struct {
	Kind    string `json:"kind" example:"node"`
	NodeId  *int   `json:"nodeId" example:"2"`
	Address string `json:"address" example:"203.0.113.7"`
}

// TunnelCreateRequest creates a link. Ports is what users reach on the Iran end.
type TunnelCreateRequest struct {
	Name       string          `json:"name" example:"de-ir"`
	Kharej     TunnelEndpoint  `json:"kharej"`
	Iran       TunnelEndpoint  `json:"iran"`
	Carrier    string          `json:"carrier" example:"auto"`
	ListenPort int             `json:"listenPort" example:"0"`
	Path       string          `json:"path" example:"/wat"`
	Sni        string          `json:"sni" example:""`
	IcmpType   string          `json:"icmpType" example:"echo"`
	Reverse    bool            `json:"reverse" example:"false"`
	Ports      []spec.PortMap  `json:"ports"`
	Options    spec.EndOptions `json:"options"`
	// Token is the Kharej end's pairing token, when that end is a server without a panel.
	Token string `json:"token" example:""`
}

// TunnelUpdateRequest changes what can change without re-pairing; nil fields stay.
type TunnelUpdateRequest struct {
	Name    *string          `json:"name" example:"de-ir"`
	Enable  *bool            `json:"enable" example:"true"`
	Ports   *[]spec.PortMap  `json:"ports"`
	Options *spec.EndOptions `json:"options"`
}

// TunnelEndView is one end of a link as the UI shows it.
type TunnelEndView struct {
	Uuid           string            `json:"uuid" example:"7d2c1f0e-5b1a-4e0e-9a43-0f0f3a9f8c11"`
	Role           string            `json:"role" example:"kharej"`
	Host           string            `json:"host" example:"local"`
	NodeId         *int              `json:"nodeId" example:"2"`
	NodeName       string            `json:"nodeName" example:"de-fra-1"`
	Enable         bool              `json:"enable" example:"true"`
	Address        string            `json:"address" example:"203.0.113.7"`
	Peer           string            `json:"peer" example:"203.0.113.7:4443"`
	Carrier        string            `json:"carrier" example:"tcp"`
	ListenPorts    map[string]int    `json:"listenPorts"`
	State          string            `json:"state" example:"up"`
	RttMs          float64           `json:"rttMs" example:"42"`
	LastError      string            `json:"lastError" example:""`
	StateChangedAt int64             `json:"stateChangedAt" example:"1760000000000"`
	Live           *tunnel.EndStatus `json:"live"`
}

// TunnelView is a whole link: its two ends and the settings they share. An end that runs on
// a server without a panel is nil.
type TunnelView struct {
	PairUuid  string                  `json:"pairUuid" example:"0a1b2c3d-5b1a-4e0e-9a43-0f0f3a9f8c11"`
	Name      string                  `json:"name" example:"de-ir"`
	Carrier   string                  `json:"carrier" example:"tcp"`
	Auto      bool                    `json:"auto" example:"true"`
	Reverse   bool                    `json:"reverse" example:"false"`
	Ports     []spec.PortMap          `json:"ports"`
	Options   spec.EndOptions         `json:"options"`
	State     string                  `json:"state" example:"up"`
	Kharej    *TunnelEndView          `json:"kharej"`
	Iran      *TunnelEndView          `json:"iran"`
	External  string                  `json:"external" example:"iran"`
	Probe     []wtprobe.CarrierResult `json:"probe"`
	CreatedAt int64                   `json:"createdAt" example:"1760000000000"`
}

// TunnelAccess is what a server without a panel needs to join a link. It carries the key,
// so only owners get it.
type TunnelAccess struct {
	Token   string `json:"token" example:"wat1:..."`
	Command string `json:"command" example:"curl -fsSL https://panel.example/install-tunnel.sh | bash -s -- --token wat1:..."`
}

func tunnelKeyBound(uuid string) []byte { return []byte("tunnel_ends/key/" + uuid) }

func sealTunnelKey(uuid, key string) (string, error) {
	return nodetoken.EncryptBound(tunnelKeyBound(uuid), key)
}

func openTunnelKey(r *model.TunnelEnd) (string, error) {
	return nodetoken.DecryptBound(tunnelKeyBound(r.Uuid), r.KeyEnc)
}

func tunnelRuntime(nodeID *int) (rt.TunnelRuntime, error) {
	m := rt.GetManager()
	if m == nil {
		return nil, errors.New("the runtime manager is not ready")
	}
	return m.TunnelRuntimeFor(nodeID)
}

func (s *TunnelService) endOf(r *model.TunnelEnd) (tunnel.EndConfig, error) {
	key, err := openTunnelKey(r)
	if err != nil {
		return tunnel.EndConfig{}, fmt.Errorf("tunnel %q: cannot read its key: %w", r.Name, err)
	}
	return tunnel.EndFromRow(r, key), nil
}

// saveEnd stores an end as the row for host nodeID (nil = this panel), keeping its observed
// state, and lets mutate set the master-only columns.
func (s *TunnelService) saveEnd(e tunnel.EndConfig, nodeID *int, mutate func(*model.TunnelEnd)) (*model.TunnelEnd, error) {
	db := database.GetDB()
	row := &model.TunnelEnd{}
	err := db.Where("uuid = ?", e.Uuid).First(row).Error
	switch {
	case database.IsNotFound(err):
		row = &model.TunnelEnd{CreatedAt: time.Now().UnixMilli(), State: model.TunnelStateUnknown}
	case err != nil:
		return nil, err
	}
	sealed, err := sealTunnelKey(e.Uuid, e.Key)
	if err != nil {
		return nil, err
	}
	e.Fill(row)
	row.NodeId, row.KeyEnc = nodeID, sealed
	if !e.Enable {
		row.State = model.TunnelStateStopped
	}
	if mutate != nil {
		mutate(row)
	}
	if err := db.Save(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s *TunnelService) priorRow(uuid string) *model.TunnelEnd {
	row := &model.TunnelEnd{}
	if err := database.GetDB().Where("uuid = ?", uuid).First(row).Error; err != nil {
		return nil
	}
	return row
}

// ApplyLocal runs an end on this host: it chooses missing ports, refuses clashes, starts the
// daemon and stores the row. The panel calls it for its own ends and a node's agent for
// the ends its master hands it.
func (s *TunnelService) ApplyLocal(ctx context.Context, e tunnel.EndConfig, mutate func(*model.TunnelEnd)) (tunnel.EndResult, error) {
	db := database.GetDB()
	if err := resolveTunnelListen(db, nil, &e); err != nil {
		return tunnel.EndResult{}, err
	}
	if _, err := e.Spec(runtime.GOOS); err != nil {
		return tunnel.EndResult{}, err
	}
	if e.Enable {
		if err := checkTunnelBinds(db, nil, e, s.priorRow(e.Uuid)); err != nil {
			return tunnel.EndResult{}, err
		}
	}
	local, err := tunnelRuntime(nil)
	if err != nil {
		return tunnel.EndResult{}, err
	}
	res, err := local.ApplyTunnel(ctx, e)
	if err != nil {
		return tunnel.EndResult{}, err
	}
	if _, err := s.saveEnd(res.End, nil, mutate); err != nil {
		_ = local.RemoveTunnel(ctx, e.Uuid)
		return tunnel.EndResult{}, err
	}
	return res, nil
}

// RemoveLocal stops an end on this host and forgets it.
func (s *TunnelService) RemoveLocal(ctx context.Context, uuid string) error {
	local, err := tunnelRuntime(nil)
	if err != nil {
		return err
	}
	if err := local.RemoveTunnel(ctx, uuid); err != nil {
		return err
	}
	return database.GetDB().Where("uuid = ?", uuid).Delete(&model.TunnelEnd{}).Error
}

func (s *TunnelService) applyEnd(ctx context.Context, nodeID *int, e tunnel.EndConfig, mutate func(*model.TunnelEnd)) (tunnel.EndResult, error) {
	if nodeID == nil {
		return s.ApplyLocal(ctx, e, mutate)
	}
	remote, err := tunnelRuntime(nodeID)
	if err != nil {
		return tunnel.EndResult{}, err
	}
	res, err := remote.ApplyTunnel(ctx, e)
	if err != nil {
		return tunnel.EndResult{}, err
	}
	if _, err := s.saveEnd(res.End, nodeID, mutate); err != nil {
		_ = remote.RemoveTunnel(ctx, e.Uuid)
		return tunnel.EndResult{}, err
	}
	return res, nil
}

func (s *TunnelService) removeEnd(ctx context.Context, nodeID *int, uuid string) error {
	if nodeID == nil {
		return s.RemoveLocal(ctx, uuid)
	}
	remote, err := tunnelRuntime(nodeID)
	if err != nil {
		return err
	}
	if err := remote.RemoveTunnel(ctx, uuid); err != nil {
		return err
	}
	return database.GetDB().Where("uuid = ?", uuid).Delete(&model.TunnelEnd{}).Error
}

// hostAddress is the address other servers use to reach the host of an endpoint.
func (s *TunnelService) hostAddress(ep TunnelEndpoint) (string, error) {
	if a := strings.TrimSpace(ep.Address); a != "" {
		return a, nil
	}
	switch ep.Kind {
	case TunnelHostNode:
		var n model.Node
		if err := database.GetDB().First(&n, *ep.NodeId).Error; err != nil {
			return "", fmt.Errorf("node %d not found", *ep.NodeId)
		}
		return n.Address, nil
	case TunnelHostLocal:
		if f := localPublicIPv4.Load(); f != nil {
			if ip := (*f)(); ip != "" && ip != "N/A" {
				return ip, nil
			}
		}
		return "", errors.New("this server's public address is not known yet; enter it by hand")
	}
	return "", errors.New("the address of the external server is required")
}

func (s *TunnelService) validateCreate(req *TunnelCreateRequest, mayUseExternal bool) error {
	req.Name = strings.TrimSpace(req.Name)
	if !tunnelNameRe.MatchString(req.Name) {
		return errors.New("the name must be 1-63 letters, digits, space, dot, dash or underscore")
	}
	var dup int64
	if err := database.GetDB().Model(&model.TunnelEnd{}).Where("name = ?", req.Name).Count(&dup).Error; err != nil {
		return err
	}
	if dup > 0 {
		return fmt.Errorf("a tunnel named %q already exists", req.Name)
	}
	if req.Carrier == "" {
		req.Carrier = spec.CarrierAuto
	}
	if req.Carrier != spec.CarrierAuto && !spec.ValidCarrier(req.Carrier) {
		return fmt.Errorf("unknown carrier %q", req.Carrier)
	}
	for _, ep := range []*TunnelEndpoint{&req.Kharej, &req.Iran} {
		switch ep.Kind {
		case TunnelHostLocal:
		case TunnelHostNode:
			if ep.NodeId == nil {
				return errors.New("choose the node")
			}
		case TunnelHostExternal:
			if !mayUseExternal {
				return errors.New("only the owner can pair a server without a panel: its setup command carries the tunnel key")
			}
		default:
			return fmt.Errorf("unknown end location %q", ep.Kind)
		}
	}
	if req.Kharej.Kind == TunnelHostExternal && req.Iran.Kind == TunnelHostExternal {
		return errors.New("at least one end must run on this panel or on a node")
	}
	if sameHost(req.Kharej, req.Iran) {
		return errors.New("the two ends must be on different servers")
	}
	if req.Kharej.Kind == TunnelHostExternal && strings.TrimSpace(req.Token) == "" {
		return errors.New("paste the token the Kharej server printed")
	}
	if len(req.Ports) == 0 {
		return errors.New("add at least one port to forward")
	}
	if err := spec.CheckPorts(req.Ports); err != nil {
		return err
	}
	if req.Reverse {
		if req.Carrier == spec.CarrierAuto || !(req.Carrier == "icmp" || spec.IsPoolCarrier(req.Carrier)) {
			return errors.New("reverse needs one connection carrier or icmp, not auto")
		}
	}
	if req.ListenPort < 0 || req.ListenPort > 65535 {
		return errors.New("the carrier port is out of range")
	}
	if req.Reverse && req.Iran.Kind == TunnelHostExternal && req.ListenPort == 0 {
		return errors.New("reverse to a server without a panel needs the carrier port it will listen on")
	}
	return req.Options.ForEnd(spec.RoleIran, false).ValidateOptions()
}

func sameHost(a, b TunnelEndpoint) bool {
	if a.Kind != b.Kind || a.Kind == TunnelHostExternal {
		return false
	}
	if a.Kind == TunnelHostLocal {
		return true
	}
	return a.NodeId != nil && b.NodeId != nil && *a.NodeId == *b.NodeId
}

func newTunnelEnd(role, name, pair string, ports []spec.PortMap, opts spec.EndOptions) tunnel.EndConfig {
	return tunnel.EndConfig{
		Uuid: uuid.NewString(), PairUuid: pair, Name: name, Role: role, Enable: true,
		Ports: ports, Options: opts,
	}
}

// TokenFor builds the pairing token a Kharej end hands to its Iran end.
func TokenFor(e tunnel.EndConfig) spec.Token {
	t := spec.Token{
		Carrier: e.Carrier, Peer: e.Address, Key: e.Key, Path: e.Path, Sni: e.Sni, IcmpType: e.IcmpType,
		Allow: spec.AllowString(e.Ports), Reverse: e.Reverse,
	}
	if e.Carrier == spec.CarrierAuto {
		t.Plan = spec.FormatPlan(e.ListenPorts)
	} else if p, ok := e.ListenPorts[e.Carrier]; ok {
		t.Peer = net.JoinHostPort(e.Address, strconv.Itoa(p))
	}
	return t
}

// iranEndFromToken derives the Iran end a Kharej token describes.
func iranEndFromToken(name, pair string, tok spec.Token, ports []spec.PortMap, opts spec.EndOptions) tunnel.EndConfig {
	e := newTunnelEnd(spec.RoleIran, name, pair, ports, opts)
	e.Carrier, e.Peer, e.Key, e.Path, e.Sni, e.IcmpType, e.Reverse = tok.Carrier, tok.Peer, tok.Key, tok.Path, tok.Sni, tok.IcmpType, tok.Reverse
	if tok.Reverse && spec.IsPoolCarrier(tok.Carrier) {
		_, port := spec.HostPort(tok.Peer)
		e.ListenPorts = map[string]int{tok.Carrier: port}
	}
	return e
}

func endpointNode(ep TunnelEndpoint) *int {
	if ep.Kind == TunnelHostNode {
		return ep.NodeId
	}
	return nil
}

// Create pairs two ends. mayUseExternal lets the caller name a server without a panel; the
// returned access is nil unless one end is external.
func (s *TunnelService) Create(ctx context.Context, req TunnelCreateRequest, origin string, mayUseExternal bool) (*TunnelView, *TunnelAccess, error) {
	if err := s.validateCreate(&req, mayUseExternal); err != nil {
		return nil, nil, err
	}
	pair := uuid.NewString()
	var applied []appliedEnd
	rollback := func() {
		for _, a := range applied {
			if err := s.removeEnd(ctx, a.node, a.uuid); err != nil {
				logger.Warningf("tunnel: rolling back end %s failed: %v", a.uuid, err)
			}
		}
	}
	fail := func(err error) (*TunnelView, *TunnelAccess, error) {
		rollback()
		return nil, nil, err
	}
	apply := func(ep TunnelEndpoint, e tunnel.EndConfig, mutate func(*model.TunnelEnd)) (tunnel.EndConfig, error) {
		res, err := s.applyEnd(ctx, endpointNode(ep), e, mutate)
		if err != nil {
			return e, fmt.Errorf("%s end: %w", e.Role, err)
		}
		applied = append(applied, appliedEnd{node: endpointNode(ep), uuid: e.Uuid})
		return res.End, nil
	}

	var kharej, iran tunnel.EndConfig
	kExternal, iExternal := req.Kharej.Kind == TunnelHostExternal, req.Iran.Kind == TunnelHostExternal
	var access *TunnelAccess
	var tok spec.Token

	switch {
	case kExternal:
		t, err := spec.ParseToken(req.Token)
		if err != nil {
			return nil, nil, err
		}
		tok = t
	case !req.Reverse:
		k, err := s.buildKharej(req, pair)
		if err != nil {
			return nil, nil, err
		}
		if k, err = apply(req.Kharej, k, func(r *model.TunnelEnd) { r.PeerExternal = iExternal; r.AutoPlan = autoPlanOf(k) }); err != nil {
			return fail(err)
		}
		kharej, tok = k, TokenFor(k)
	}

	if req.Reverse && !kExternal {
		var err error
		if iran, err = s.buildReverseIran(req, pair); err != nil {
			return nil, nil, err
		}
		if !iExternal {
			if iran, err = apply(req.Iran, iran, func(r *model.TunnelEnd) { r.PeerExternal = false }); err != nil {
				return fail(err)
			}
		}
		k, err := s.buildReverseKharej(req, pair, iran)
		if err != nil {
			return fail(err)
		}
		if k, err = apply(req.Kharej, k, func(r *model.TunnelEnd) { r.PeerExternal = iExternal }); err != nil {
			return fail(err)
		}
		kharej, tok = k, TokenFor(k)
	}

	var probeResults []wtprobe.CarrierResult
	if !iExternal && !(req.Reverse && !kExternal) {
		i := iranEndFromToken(req.Name, pair, tok, req.Ports, req.Options)
		i.Address = ""
		var err error
		if tok.Carrier == spec.CarrierAuto {
			if probeResults, err = s.probeInto(ctx, req.Iran, &i, tok); err != nil {
				return fail(err)
			}
		}
		plan := tok.Plan
		if iran, err = apply(req.Iran, i, func(r *model.TunnelEnd) {
			r.PeerExternal = kExternal
			r.AutoPlan = plan
			r.Probe = probeResults
		}); err != nil {
			return fail(err)
		}
	}
	if iExternal && kharej.Uuid != "" {
		access = s.accessFor(kharej, req.Ports, origin)
	}
	if iExternal && kharej.Uuid == "" {
		return fail(errors.New("nothing to pair"))
	}

	s.waitForLink(ctx, req, kharej, iran)
	view, err := s.Get(ctx, pair)
	return view, access, err
}

type appliedEnd struct {
	node *int
	uuid string
}

func autoPlanOf(e tunnel.EndConfig) string {
	if e.Carrier != spec.CarrierAuto {
		return ""
	}
	return spec.FormatPlan(e.ListenPorts)
}

func (s *TunnelService) buildKharej(req TunnelCreateRequest, pair string) (tunnel.EndConfig, error) {
	addr, err := s.hostAddress(req.Kharej)
	if err != nil {
		return tunnel.EndConfig{}, fmt.Errorf("kharej end: %w", err)
	}
	key, err := spec.NewKey()
	if err != nil {
		return tunnel.EndConfig{}, err
	}
	k := newTunnelEnd(spec.RoleKharej, req.Name, pair, req.Ports, req.Options)
	k.Carrier, k.Address, k.Key, k.Path, k.Sni, k.IcmpType = req.Carrier, addr, key, req.Path, req.Sni, req.IcmpType
	if k.Path == "" && k.Carrier != "tcp" && k.Carrier != "udp" && k.Carrier != "icmp" {
		k.Path = "/wat"
	}
	if req.ListenPort > 0 && req.Carrier != spec.CarrierAuto && req.Carrier != "icmp" {
		k.ListenPorts = map[string]int{req.Carrier: req.ListenPort}
	}
	return k, nil
}

// buildReverseIran: with reverse the Iran end listens for the carrier, so it is made first.
func (s *TunnelService) buildReverseIran(req TunnelCreateRequest, pair string) (tunnel.EndConfig, error) {
	addr, err := s.hostAddress(req.Iran)
	if err != nil {
		return tunnel.EndConfig{}, fmt.Errorf("iran end: %w", err)
	}
	key, err := spec.NewKey()
	if err != nil {
		return tunnel.EndConfig{}, err
	}
	i := newTunnelEnd(spec.RoleIran, req.Name, pair, req.Ports, req.Options)
	i.Carrier, i.Address, i.Key, i.Path, i.Sni, i.IcmpType, i.Reverse = req.Carrier, addr, key, req.Path, req.Sni, req.IcmpType, true
	if i.Path == "" && spec.IsPoolCarrier(i.Carrier) && i.Carrier != "tcp" && i.Carrier != "tls" {
		i.Path = "/wat"
	}
	if req.ListenPort > 0 && spec.IsPoolCarrier(req.Carrier) {
		i.ListenPorts = map[string]int{req.Carrier: req.ListenPort}
	}
	kAddr, err := s.hostAddress(req.Kharej)
	if err != nil {
		return tunnel.EndConfig{}, fmt.Errorf("kharej end: %w", err)
	}
	i.Peer = kAddr
	return i, nil
}

func (s *TunnelService) buildReverseKharej(req TunnelCreateRequest, pair string, iran tunnel.EndConfig) (tunnel.EndConfig, error) {
	kAddr, err := s.hostAddress(req.Kharej)
	if err != nil {
		return tunnel.EndConfig{}, fmt.Errorf("kharej end: %w", err)
	}
	k := newTunnelEnd(spec.RoleKharej, req.Name, pair, req.Ports, req.Options)
	k.Carrier, k.Address, k.Key, k.Path, k.Sni, k.IcmpType, k.Reverse = req.Carrier, kAddr, iran.Key, iran.Path, iran.Sni, iran.IcmpType, true
	port := iran.ListenPorts[iran.Carrier]
	if spec.IsPoolCarrier(iran.Carrier) {
		k.ListenPorts = map[string]int{iran.Carrier: port}
		k.Peer = net.JoinHostPort(iran.Address, strconv.Itoa(port))
	} else {
		k.Peer = iran.Address
	}
	return k, nil
}

// probeInto runs the carrier probe from the Iran host and sets the winner on the end.
func (s *TunnelService) probeInto(ctx context.Context, ep TunnelEndpoint, e *tunnel.EndConfig, tok spec.Token) ([]wtprobe.CarrierResult, error) {
	runtime, err := tunnelRuntime(endpointNode(ep))
	if err != nil {
		return nil, err
	}
	results, err := runtime.ProbeTunnel(ctx, tok.Encode())
	if err != nil {
		return nil, fmt.Errorf("probing carriers: %w", err)
	}
	best, ok := wtprobe.Best(results)
	if !ok {
		return results, fmt.Errorf("no carrier connected to %s: check that the Kharej firewall allows the carrier ports", tok.Peer)
	}
	e.Carrier, e.Peer = best.Carrier, best.Peer
	return results, nil
}

// waitForLink gives a fresh link a few seconds to connect so the answer shows its real state.
func (s *TunnelService) waitForLink(ctx context.Context, req TunnelCreateRequest, kharej, iran tunnel.EndConfig) {
	watch := iran
	ep := req.Iran
	if watch.Uuid == "" {
		watch, ep = kharej, req.Kharej
	}
	remote, err := tunnelRuntime(endpointNode(ep))
	if err != nil {
		return
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		sts, err := remote.TunnelStatuses(ctx)
		if err == nil {
			for _, st := range sts {
				if st.Uuid == watch.Uuid && (st.State == model.TunnelStateUp || st.State == model.TunnelStateError) {
					s.recordStatuses([]*model.TunnelEnd{s.priorRow(watch.Uuid)}, sts)
					return
				}
			}
		}
		time.Sleep(time.Second)
	}
}

// InstallCommand is the one line that sets up an end on a server without a panel.
func InstallCommand(origin, token string, ports []spec.PortMap, name string) string {
	var maps []string
	for _, p := range ports {
		maps = append(maps, spec.FormatPort(spec.PortMap{Listen: p.Listen, Target: p.Target, UDP: p.UDP}))
	}
	cmd := fmt.Sprintf("curl -fsSL %s/install-tunnel.sh | bash -s -- --token %s --map %s", strings.TrimRight(origin, "/"), token, strings.Join(maps, ","))
	if name != "" {
		cmd += " --name " + shellWord(name)
	}
	return cmd
}

var plainWord = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func shellWord(s string) string {
	if plainWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (s *TunnelService) accessFor(kharej tunnel.EndConfig, ports []spec.PortMap, origin string) *TunnelAccess {
	tok := TokenFor(kharej).Encode()
	return &TunnelAccess{Token: tok, Command: InstallCommand(origin, tok, ports, kharej.Name)}
}

func (s *TunnelService) rowsOfPair(pair string) ([]*model.TunnelEnd, error) {
	var rows []*model.TunnelEnd
	err := database.GetDB().Where("pair_uuid = ?", pair).Order("id").Find(&rows).Error
	if err == nil && len(rows) == 0 {
		return nil, fmt.Errorf("tunnel not found")
	}
	return rows, err
}

func pairEnds(rows []*model.TunnelEnd) (kharej, iran *model.TunnelEnd) {
	for _, r := range rows {
		if r.Role == spec.RoleKharej {
			kharej = r
		} else {
			iran = r
		}
	}
	return
}

// Access returns the join token and install command of a link whose Kharej end this panel
// runs, for the server without a panel on the other side.
func (s *TunnelService) Access(pair, origin string) (*TunnelAccess, error) {
	rows, err := s.rowsOfPair(pair)
	if err != nil {
		return nil, err
	}
	k, _ := pairEnds(rows)
	if k == nil {
		return nil, errors.New("the Kharej end of this tunnel is not managed here, so there is no token to show")
	}
	e, err := s.endOf(k)
	if err != nil {
		return nil, err
	}
	return s.accessFor(e, k.Ports, origin), nil
}

// List returns every link with the last observed state.
func (s *TunnelService) List() ([]TunnelView, error) {
	var rows []*model.TunnelEnd
	if err := database.GetDB().Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	names := s.nodeNames()
	groups := map[string][]*model.TunnelEnd{}
	var order []string
	for _, r := range rows {
		if _, seen := groups[r.PairUuid]; !seen {
			order = append(order, r.PairUuid)
		}
		groups[r.PairUuid] = append(groups[r.PairUuid], r)
	}
	out := make([]TunnelView, 0, len(order))
	for _, p := range order {
		out = append(out, viewOf(groups[p], names, nil))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

func (s *TunnelService) nodeNames() map[int]string {
	var nodes []model.Node
	_ = database.GetDB().Select("id", "name").Find(&nodes).Error
	m := make(map[int]string, len(nodes))
	for _, n := range nodes {
		m[n.Id] = n.Name
	}
	return m
}

// orEmpty keeps nil slices out of the JSON: the UI schemas want [] and not null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func endViewOf(r *model.TunnelEnd, names map[int]string, live *tunnel.EndStatus) *TunnelEndView {
	if r == nil {
		return nil
	}
	host := TunnelHostLocal
	name := ""
	if r.NodeId != nil {
		host, name = TunnelHostNode, names[*r.NodeId]
	}
	return &TunnelEndView{
		Uuid: r.Uuid, Role: r.Role, Host: host, NodeId: r.NodeId, NodeName: name, Enable: r.Enable,
		Address: r.Address, Peer: r.Peer, Carrier: r.Carrier, ListenPorts: listenOrEmpty(r.ListenPorts), State: r.State,
		RttMs: r.RttMs, LastError: r.LastError, StateChangedAt: r.StateChangedAt, Live: live,
	}
}

func viewOf(rows []*model.TunnelEnd, names map[int]string, live map[string]tunnel.EndStatus) TunnelView {
	k, i := pairEnds(rows)
	first := rows[0]
	lv := func(r *model.TunnelEnd) *tunnel.EndStatus {
		if r == nil || live == nil {
			return nil
		}
		if st, ok := live[r.Uuid]; ok {
			return &st
		}
		return nil
	}
	v := TunnelView{
		PairUuid: first.PairUuid, Name: first.Name, Reverse: first.Reverse, Ports: first.Ports, Options: first.Options,
		Kharej: endViewOf(k, names, lv(k)), Iran: endViewOf(i, names, lv(i)), CreatedAt: first.CreatedAt,
	}
	for _, r := range rows {
		if r.CreatedAt < v.CreatedAt {
			v.CreatedAt = r.CreatedAt
		}
	}
	switch {
	case i != nil:
		v.Carrier, v.Options, v.Ports, v.Probe = i.Carrier, i.Options, i.Ports, i.Probe
		v.Auto = i.AutoPlan != ""
		v.State = i.State
	default:
		v.Carrier, v.Options, v.Ports = k.Carrier, k.Options, k.Ports
		v.Auto = k.Carrier == spec.CarrierAuto
		v.State = k.State
	}
	v.Ports, v.Probe = orEmpty(v.Ports), orEmpty(v.Probe)
	switch {
	case k != nil && k.PeerExternal:
		v.External = spec.RoleIran
	case i != nil && i.PeerExternal:
		v.External = spec.RoleKharej
	}
	if i != nil && k != nil && (i.State == model.TunnelStateError || k.State == model.TunnelStateError) {
		v.State = model.TunnelStateError
	}
	return v
}

// Get returns one link with live counters from the hosts running its ends.
func (s *TunnelService) Get(ctx context.Context, pair string) (*TunnelView, error) {
	rows, err := s.rowsOfPair(pair)
	if err != nil {
		return nil, err
	}
	live := map[string]tunnel.EndStatus{}
	for _, r := range rows {
		remote, err := tunnelRuntime(r.NodeId)
		if err != nil {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		sts, err := remote.TunnelStatuses(cctx)
		cancel()
		if err != nil {
			continue
		}
		for _, st := range sts {
			if st.Uuid == r.Uuid {
				live[st.Uuid] = st
			}
		}
	}
	v := viewOf(rows, s.nodeNames(), live)
	return &v, nil
}

// Update changes the shared settings of a link on every managed end, undoing the ones already
// changed when a later one refuses.
func (s *TunnelService) Update(ctx context.Context, pair string, req TunnelUpdateRequest) (*TunnelView, error) {
	rows, err := s.rowsOfPair(pair)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if !tunnelNameRe.MatchString(n) {
			return nil, errors.New("the name must be 1-63 letters, digits, space, dot, dash or underscore")
		}
		var dup int64
		database.GetDB().Model(&model.TunnelEnd{}).Where("name = ? AND pair_uuid != ?", n, pair).Count(&dup)
		if dup > 0 {
			return nil, fmt.Errorf("a tunnel named %q already exists", n)
		}
		req.Name = &n
	}
	if req.Ports != nil {
		if len(*req.Ports) == 0 {
			return nil, errors.New("keep at least one port")
		}
		if err := spec.CheckPorts(*req.Ports); err != nil {
			return nil, err
		}
	}
	type done struct {
		row *model.TunnelEnd
		old tunnel.EndConfig
	}
	var changed []done
	for _, r := range rows {
		old, err := s.endOf(r)
		if err != nil {
			return nil, err
		}
		next := old
		if req.Name != nil {
			next.Name = *req.Name
		}
		if req.Enable != nil {
			next.Enable = *req.Enable
		}
		if req.Ports != nil {
			next.Ports = *req.Ports
		}
		if req.Options != nil {
			next.Options = *req.Options
		}
		if _, err := s.applyEnd(ctx, r.NodeId, next, nil); err != nil {
			for _, d := range changed {
				if _, rerr := s.applyEnd(ctx, d.row.NodeId, d.old, nil); rerr != nil {
					logger.Warningf("tunnel: could not undo the change of %q: %v", d.row.Name, rerr)
				}
			}
			return nil, fmt.Errorf("%s end: %w", r.Role, err)
		}
		changed = append(changed, done{row: r, old: old})
	}
	return s.Get(ctx, pair)
}

// Delete stops and forgets a link. With force, a node that cannot be reached is skipped.
func (s *TunnelService) Delete(ctx context.Context, pair string, force bool) error {
	rows, err := s.rowsOfPair(pair)
	if err != nil {
		return err
	}
	var firstErr error
	for _, r := range rows {
		if err := s.removeEnd(ctx, r.NodeId, r.Uuid); err != nil {
			if force {
				logger.Warningf("tunnel: forced delete skipped end %s: %v", r.Uuid, err)
				database.GetDB().Where("uuid = ?", r.Uuid).Delete(&model.TunnelEnd{})
				continue
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("%s end: %w", r.Role, err)
			}
		}
	}
	s.mu.Lock()
	delete(s.watch, pair)
	s.mu.Unlock()
	return firstErr
}

func (s *TunnelService) rowByRole(pair, role string) (*model.TunnelEnd, error) {
	rows, err := s.rowsOfPair(pair)
	if err != nil {
		return nil, err
	}
	k, i := pairEnds(rows)
	pick := map[string]*model.TunnelEnd{spec.RoleKharej: k, spec.RoleIran: i}[role]
	if pick == nil {
		return nil, fmt.Errorf("this panel does not run the %s end of the tunnel", role)
	}
	return pick, nil
}

// Restart bounces the daemon of every managed end of a link.
func (s *TunnelService) Restart(ctx context.Context, pair string) error {
	rows, err := s.rowsOfPair(pair)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if !r.Enable {
			continue
		}
		remote, err := tunnelRuntime(r.NodeId)
		if err != nil {
			return err
		}
		if err := remote.RestartTunnel(ctx, r.Uuid); err != nil {
			return fmt.Errorf("%s end: %w", r.Role, err)
		}
	}
	return nil
}

// Logs returns the tail of one end's daemon log.
func (s *TunnelService) Logs(ctx context.Context, pair, role string, lines int) ([]string, error) {
	r, err := s.rowByRole(pair, role)
	if err != nil {
		return nil, err
	}
	remote, err := tunnelRuntime(r.NodeId)
	if err != nil {
		return nil, err
	}
	return remote.TunnelLogs(ctx, r.Uuid, lines)
}

// SpeedTest moves mb megabytes each way through the Iran end.
func (s *TunnelService) SpeedTest(ctx context.Context, pair string, mb int) (tunnel.SpeedResult, error) {
	r, err := s.rowByRole(pair, spec.RoleIran)
	if err != nil {
		return tunnel.SpeedResult{}, errors.New("a speed test runs from the Iran end, which this panel does not manage")
	}
	remote, err := tunnelRuntime(r.NodeId)
	if err != nil {
		return tunnel.SpeedResult{}, err
	}
	return remote.TunnelSpeedTest(ctx, r.Uuid, min(max(mb, 1), 256))
}

// Reprobe tests every carrier again from the Iran end of an auto link and switches to the best.
func (s *TunnelService) Reprobe(ctx context.Context, pair string) (*TunnelView, error) {
	r, err := s.rowByRole(pair, spec.RoleIran)
	if err != nil || r.AutoPlan == "" {
		return nil, errors.New("only an auto tunnel whose Iran end runs on this panel can be probed again")
	}
	e, err := s.endOf(r)
	if err != nil {
		return nil, err
	}
	host, _ := spec.HostPort(r.Peer)
	tok := spec.Token{Carrier: spec.CarrierAuto, Peer: host, Key: e.Key, Path: e.Path, Sni: e.Sni, IcmpType: e.IcmpType, Plan: r.AutoPlan}
	ep := TunnelEndpoint{Kind: TunnelHostLocal}
	if r.NodeId != nil {
		ep = TunnelEndpoint{Kind: TunnelHostNode, NodeId: r.NodeId}
	}
	results, err := s.probeInto(ctx, ep, &e, tok)
	if err != nil {
		return nil, err
	}
	if _, err := s.applyEnd(ctx, r.NodeId, e, func(row *model.TunnelEnd) { row.Probe = results }); err != nil {
		return nil, err
	}
	return s.Get(ctx, pair)
}

// Capabilities tells the UI what this panel can do for tunnels.
type TunnelCapabilities struct {
	BinaryAvailable bool     `json:"binaryAvailable" example:"true"`
	Platform        string   `json:"platform" example:"linux"`
	PublicIPv4      string   `json:"publicIPv4" example:"203.0.113.7"`
	Carriers        []string `json:"carriers"`
	CanSeeToken     bool     `json:"canSeeToken" example:"true"`
}

// Capabilities reports whether the wattun binary is installed and which carriers apply here.
func (s *TunnelService) Capabilities(canSeeToken bool) TunnelCapabilities {
	caps := TunnelCapabilities{
		BinaryAvailable: tunnel.BinaryAvailable(), Platform: runtime.GOOS, CanSeeToken: canSeeToken,
	}
	for _, c := range spec.Carriers {
		if runtime.GOOS == "windows" && spec.NeedsRawSocket(c) {
			continue
		}
		caps.Carriers = append(caps.Carriers, c)
	}
	if f := localPublicIPv4.Load(); f != nil {
		if ip := (*f)(); ip != "N/A" {
			caps.PublicIPv4 = ip
		}
	}
	return caps
}

// The Agent methods answer a master panel that runs tunnel ends on this host, through the
// same local runtime the panel uses for its own ends.

func (s *TunnelService) AgentStatuses(ctx context.Context) ([]tunnel.EndStatus, error) {
	local, err := tunnelRuntime(nil)
	if err != nil {
		return nil, err
	}
	return local.TunnelStatuses(ctx)
}

func (s *TunnelService) AgentLogs(ctx context.Context, uuid string, lines int) ([]string, error) {
	local, err := tunnelRuntime(nil)
	if err != nil {
		return nil, err
	}
	return local.TunnelLogs(ctx, uuid, lines)
}

func (s *TunnelService) AgentRestart(ctx context.Context, uuid string) error {
	local, err := tunnelRuntime(nil)
	if err != nil {
		return err
	}
	return local.RestartTunnel(ctx, uuid)
}

func (s *TunnelService) AgentProbe(ctx context.Context, token string) ([]wtprobe.CarrierResult, error) {
	local, err := tunnelRuntime(nil)
	if err != nil {
		return nil, err
	}
	return local.ProbeTunnel(ctx, token)
}

func (s *TunnelService) AgentSpeedTest(ctx context.Context, uuid string, mb int) (tunnel.SpeedResult, error) {
	local, err := tunnelRuntime(nil)
	if err != nil {
		return tunnel.SpeedResult{}, err
	}
	return local.TunnelSpeedTest(ctx, uuid, min(max(mb, 1), 256))
}

func listenOrEmpty(m map[string]int) map[string]int {
	if m == nil {
		return map[string]int{}
	}
	return m
}
