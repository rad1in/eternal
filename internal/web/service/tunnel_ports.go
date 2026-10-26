package service

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/tunnel"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"

	"gorm.io/gorm"
)

// autoPortCarriers are the carriers a Kharej auto end opens a port for.
var autoPortCarriers = []string{"udp", "tcp", "tls", "ws", "wss", "h2", "h2c", "grpc"}

func protoBit(proto string) transportBits {
	if proto == "udp" {
		return transportUDP
	}
	return transportTCP
}

// tunnelPortOwner names what already holds proto/port on the host a tunnel end will run on,
// or "" when it is free. The end being applied is ignored: it may be holding the port itself.
// OS-level binding is only tested for the panel's own host, where the panel can try it.
func tunnelPortOwner(db *gorm.DB, nodeID *int, proto string, port int, ignoreUuid string, probeOS bool) (string, error) {
	bit := protoBit(proto)
	if nodeID == nil {
		if bit&transportTCP != 0 {
			settings := SettingService{}
			if web, err := settings.GetPort(); err == nil && web == port {
				return "the panel web port", nil
			}
			if sub, err := settings.GetSubPort(); err == nil && sub == port {
				return "the subscription port", nil
			}
			if port == reservedAPIPort() {
				return "the internal Xray API", nil
			}
		}
		owner, err := hostedPanelOwning(db, port)
		if err != nil {
			return "", err
		}
		if owner != nil {
			return fmt.Sprintf("hosted panel '%s'", owner.Name), nil
		}
	}
	var inbounds []*model.Inbound
	if err := db.Where("port = ?", port).Find(&inbounds).Error; err != nil {
		return "", err
	}
	for _, ib := range inbounds {
		if sameNode(ib.NodeID, nodeID) && inboundTransports(ib.Protocol, ib.StreamSettings, ib.Settings)&bit != 0 {
			name := ib.Remark
			if name == "" {
				name = ib.Tag
			}
			return fmt.Sprintf("inbound '%s'", name), nil
		}
	}
	other, err := tunnelEndHolding(db, nodeID, proto, port, ignoreUuid)
	if err != nil || other != "" {
		return other, err
	}
	if probeOS && nodeID == nil && osPortBusy(proto, port) {
		return "another program on this server", nil
	}
	return "", nil
}

// tunnelEndHolding finds another tunnel end on the same host that binds proto/port.
func tunnelEndHolding(db *gorm.DB, nodeID *int, proto string, port int, ignoreUuid string) (string, error) {
	var rows []*model.TunnelEnd
	q := db.Where("uuid != ?", ignoreUuid)
	if nodeID == nil {
		q = q.Where("node_id IS NULL")
	} else {
		q = q.Where("node_id = ?", *nodeID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return "", err
	}
	for _, r := range rows {
		if !r.Enable {
			continue
		}
		for _, b := range tunnel.EndFromRow(r, "").ListenedPorts("linux") {
			if b.Proto == proto && b.Port == port {
				return fmt.Sprintf("tunnel '%s'", r.Name), nil
			}
		}
	}
	return "", nil
}

// tunnelOwningPort is the inbound side of the same rule: the tunnel that binds proto-bits/port.
func tunnelOwningPort(db *gorm.DB, nodeID *int, port int, bits transportBits) (string, error) {
	var rows []*model.TunnelEnd
	q := db.Model(&model.TunnelEnd{})
	if nodeID == nil {
		q = q.Where("node_id IS NULL")
	} else {
		q = q.Where("node_id = ?", *nodeID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return "", err
	}
	for _, r := range rows {
		if !r.Enable {
			continue
		}
		for _, b := range tunnel.EndFromRow(r, "").ListenedPorts("linux") {
			if b.Port == port && protoBit(b.Proto)&bits != 0 {
				return r.Name, nil
			}
		}
	}
	return "", nil
}

func osPortBusy(proto string, port int) bool {
	addr := ":" + strconv.Itoa(port)
	if proto == "udp" {
		c, err := (&net.ListenConfig{}).ListenPacket(context.Background(), "udp", addr)
		if err != nil {
			return true
		}
		_ = c.Close()
		return false
	}
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	if err != nil {
		return true
	}
	_ = l.Close()
	return false
}

// checkTunnelBinds rejects an end whose ports collide with anything on its host. prior is the
// stored copy of the same end: ports it already holds are not re-tested against the OS.
func checkTunnelBinds(db *gorm.DB, nodeID *int, e tunnel.EndConfig, prior *model.TunnelEnd) error {
	held := map[string]bool{}
	if prior != nil {
		for _, b := range tunnel.EndFromRow(prior, "").ListenedPorts("linux") {
			held[b.Proto+strconv.Itoa(b.Port)] = true
		}
	}
	for _, b := range e.ListenedPorts("linux") {
		owner, err := tunnelPortOwner(db, nodeID, b.Proto, b.Port, e.Uuid, !held[b.Proto+strconv.Itoa(b.Port)])
		if err != nil {
			return err
		}
		if owner != "" {
			return fmt.Errorf("port %d/%s (%s) is already used by %s", b.Port, b.Proto, b.What, owner)
		}
	}
	return nil
}

// resolveTunnelListen fills the carrier ports an end needs but was not given, choosing free
// ones on its host.
func resolveTunnelListen(db *gorm.DB, nodeID *int, e *tunnel.EndConfig) error {
	var want []string
	switch {
	case e.Role == spec.RoleKharej && !e.Reverse && e.Carrier == spec.CarrierAuto:
		want = autoPortCarriers
	case e.Role == spec.RoleKharej && !e.Reverse, e.Role == spec.RoleIran && e.Reverse:
		want = []string{e.Carrier}
	}
	var missing []string
	for _, c := range want {
		if e.ListenPorts[c] == 0 && c != "icmp" {
			missing = append(missing, c)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	var firstErr error
	busy := func(proto string, port int) bool {
		owner, err := tunnelPortOwner(db, nodeID, proto, port, e.Uuid, true)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		return owner != "" || err != nil
	}
	picked, err := spec.PickListenPorts(missing, busy)
	if firstErr != nil {
		return firstErr
	}
	if err != nil {
		return err
	}
	if e.ListenPorts == nil {
		e.ListenPorts = map[string]int{}
	}
	for c, p := range picked {
		e.ListenPorts[c] = p
	}
	return nil
}
