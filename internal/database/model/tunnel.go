package model

import (
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/probe"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

// Tunnel end roles; they mirror the daemon's server (Kharej) and client (Iran).
const (
	TunnelRoleKharej = spec.RoleKharej
	TunnelRoleIran   = spec.RoleIran
)

// Observed tunnel states.
const (
	TunnelStateUnknown = "unknown"
	TunnelStateStopped = "stopped"
	TunnelStateStart   = "starting"
	TunnelStateUp      = "up"
	TunnelStateDown    = "down"
	TunnelStateError   = "error"
)

// TunnelEnd is one end of a WatTunnel link: a wattun daemon on this panel's host (NodeId nil)
// or on a node. The two ends of a link share PairUuid; an end on a plain server without a
// panel has no row, and PeerExternal says so on the other one.
type TunnelEnd struct {
	Id       int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	Uuid     string `json:"uuid" gorm:"uniqueIndex;not null" example:"7d2c1f0e-5b1a-4e0e-9a43-0f0f3a9f8c11"`
	PairUuid string `json:"pairUuid" gorm:"index;not null;default:''" example:"0a1b2c3d-5b1a-4e0e-9a43-0f0f3a9f8c11"`
	Name     string `json:"name" gorm:"not null" example:"de-ir"`
	Role     string `json:"role" gorm:"not null" validate:"oneof=kharej iran" example:"kharej"`
	NodeId   *int   `json:"nodeId" gorm:"column:node_id;index" example:"2"`
	Enable   bool   `json:"enable" gorm:"not null" example:"true"`

	// Carrier is "auto" on a Kharej end that listens on every carrier, else one carrier.
	Carrier string `json:"carrier" gorm:"not null;default:''" example:"tcp"`
	// Address is the public host the other end dials; Peer is what this end dials itself.
	Address     string         `json:"address" gorm:"not null;default:''" example:"203.0.113.7"`
	Peer        string         `json:"peer" gorm:"not null;default:''" example:"203.0.113.7:4443"`
	ListenPorts map[string]int `json:"listenPorts" gorm:"serializer:json;column:listen_ports" example:"{\"tcp\":4443}"`
	Path        string         `json:"path" gorm:"not null;default:''" example:"/wat"`
	Sni         string         `json:"sni" gorm:"not null;default:''" example:""`
	IcmpType    string         `json:"icmpType" gorm:"column:icmp_type;not null;default:''" example:"echo"`
	// AutoPlan is the Kharej end's carrier-to-port plan; an Iran end keeps it to probe again.
	AutoPlan string `json:"autoPlan" gorm:"column:auto_plan;not null;default:''" example:"d=udp:4443,tcp:4443;r="`
	Reverse  bool   `json:"reverse" gorm:"not null;default:false" example:"false"`

	// KeyEnc is the tunnel key sealed with the node-token codec; it never leaves the server.
	KeyEnc       string                `json:"-" gorm:"column:key_enc;not null;default:''"`
	Ports        []spec.PortMap        `json:"ports" gorm:"serializer:json"`
	Options      spec.EndOptions       `json:"options" gorm:"serializer:json"`
	PeerExternal bool                  `json:"peerExternal" gorm:"column:peer_external;not null;default:false" example:"false"`
	Probe        []probe.CarrierResult `json:"probe" gorm:"serializer:json"`

	// Observed state, refreshed by the tunnel job.
	State          string  `json:"state" gorm:"not null;default:unknown" example:"up"`
	RttMs          float64 `json:"rttMs" gorm:"column:rtt_ms;not null;default:0" example:"42"`
	LastError      string  `json:"lastError" gorm:"column:last_error;not null;default:''" example:""`
	StateChangedAt int64   `json:"stateChangedAt" gorm:"column:state_changed_at;not null;default:0" example:"1760000000000"`
	CreatedAt      int64   `json:"createdAt" gorm:"not null;default:0" example:"1760000000000"`
}

// TableName keeps the table name stable and clear of the inbound "tunnel" protocol.
func (TunnelEnd) TableName() string { return "tunnel_ends" }
