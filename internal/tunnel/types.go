package tunnel

import (
	"errors"
	"slices"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

// EndConfig is one tunnel end as it travels from the panel to the host that runs it. Key is the
// plaintext tunnel key; the transport to a node is TLS and the stored copy is sealed.
type EndConfig struct {
	Uuid        string          `json:"uuid" example:"7d2c1f0e-5b1a-4e0e-9a43-0f0f3a9f8c11"`
	PairUuid    string          `json:"pairUuid" example:"0a1b2c3d-5b1a-4e0e-9a43-0f0f3a9f8c11"`
	Name        string          `json:"name" example:"de-ir"`
	Role        string          `json:"role" example:"kharej"`
	Enable      bool            `json:"enable" example:"true"`
	Carrier     string          `json:"carrier" example:"tcp"`
	Address     string          `json:"address" example:"203.0.113.7"`
	Peer        string          `json:"peer" example:"203.0.113.7:4443"`
	ListenPorts map[string]int  `json:"listenPorts" example:"{\"tcp\":4443}"`
	Path        string          `json:"path" example:"/wat"`
	Sni         string          `json:"sni" example:""`
	IcmpType    string          `json:"icmpType" example:"echo"`
	Reverse     bool            `json:"reverse" example:"false"`
	Key         string          `json:"key" example:"redacted"`
	Ports       []spec.PortMap  `json:"ports"`
	Options     spec.EndOptions `json:"options"`
}

// ClientSession is one live client session on a Kharej end.
type ClientSession struct {
	IP      string `json:"ip" example:"198.51.100.20"`
	Carrier string `json:"carrier" example:"tcp"`
	AgeMs   int64  `json:"ageMs" example:"61000"`
}

// PortStat are the live counters of one forwarded port.
type PortStat struct {
	Port          int   `json:"port" example:"443"`
	BytesUp       int64 `json:"bytesUp" example:"1048576"`
	BytesDown     int64 `json:"bytesDown" example:"4194304"`
	ConnsActive   int64 `json:"connsActive" example:"3"`
	ConnsTotal    int64 `json:"connsTotal" example:"120"`
	RateUp        int64 `json:"rateUp" example:"20480"`
	RateDown      int64 `json:"rateDown" example:"81920"`
	QuotaUsed     int64 `json:"quotaUsed" example:"0"`
	QuotaLimit    int64 `json:"quotaLimit" example:"0"`
	QuotaExceeded bool  `json:"quotaExceeded" example:"false"`
}

// EndStatus is the observed state of one running end.
type EndStatus struct {
	Uuid        string          `json:"uuid" example:"7d2c1f0e-5b1a-4e0e-9a43-0f0f3a9f8c11"`
	State       string          `json:"state" example:"up"`
	Running     bool            `json:"running" example:"true"`
	Alive       bool            `json:"alive" example:"true"`
	Pid         int             `json:"pid" example:"4242"`
	UptimeSec   int64           `json:"uptimeSec" example:"3600"`
	Restarts    int             `json:"restarts" example:"0"`
	SrttMs      float64         `json:"srttMs" example:"42.5"`
	LastRxAgeMs int64           `json:"lastRxAgeMs" example:"900"`
	Sessions    int             `json:"sessions" example:"1"`
	Streams     int             `json:"streams" example:"3"`
	BytesIn     uint64          `json:"bytesIn" example:"123456"`
	BytesOut    uint64          `json:"bytesOut" example:"654321"`
	RateBps     float64         `json:"rateBps" example:"1000000"`
	Carrier     string          `json:"carrier" example:"tcp"`
	TlsMode     string          `json:"tlsMode" example:"chrome+ech"`
	Version     string          `json:"version" example:"2.10.12"`
	LastError   string          `json:"lastError" example:""`
	Clients     []ClientSession `json:"clients"`
	Ports       []PortStat      `json:"ports"`
}

// EndResult is what applying an end returns: the end with every port the host chose, and
// its first status.
type EndResult struct {
	End    EndConfig `json:"end"`
	Status EndStatus `json:"status"`
}

// SpeedResult is one speed test through a running client end.
type SpeedResult struct {
	DownMBps float64 `json:"downMBps" example:"31.5"`
	UpMBps   float64 `json:"upMBps" example:"28.2"`
	Error    string  `json:"error" example:""`
}

// EndFromRow converts a stored end plus its plaintext key into the wire form.
func EndFromRow(r *model.TunnelEnd, key string) EndConfig {
	return EndConfig{
		Uuid: r.Uuid, PairUuid: r.PairUuid, Name: r.Name, Role: r.Role, Enable: r.Enable,
		Carrier: r.Carrier, Address: r.Address, Peer: r.Peer, ListenPorts: r.ListenPorts,
		Path: r.Path, Sni: r.Sni, IcmpType: r.IcmpType, Reverse: r.Reverse,
		Key: key, Ports: r.Ports, Options: r.Options,
	}
}

// Fill copies the wire form's configuration onto a row, leaving ids and observed state alone.
func (e EndConfig) Fill(r *model.TunnelEnd) {
	r.Uuid, r.PairUuid, r.Name, r.Role, r.Enable = e.Uuid, e.PairUuid, e.Name, e.Role, e.Enable
	r.Carrier, r.Address, r.Peer, r.ListenPorts = e.Carrier, e.Address, e.Peer, e.ListenPorts
	r.Path, r.Sni, r.IcmpType, r.Reverse = e.Path, e.Sni, e.IcmpType, e.Reverse
	r.Ports, r.Options = e.Ports, e.Options
}

// Spec builds the daemon spec for this end. goos picks whether icmp may join a Kharej auto set.
func (e EndConfig) Spec(goos string) (spec.Spec, error) {
	s := spec.Spec{
		Role: e.Role, Key: e.Key, Peer: e.Peer, Path: e.Path, Sni: e.Sni, IcmpType: e.IcmpType,
		Reverse: e.Reverse, Listen: e.ListenPorts, Ports: e.Ports, Options: e.Options.ForEnd(e.Role, e.Reverse),
	}
	switch {
	case e.Carrier == spec.CarrierAuto && e.Role == spec.RoleKharej:
		if e.Reverse {
			return s, errors.New("reverse needs one carrier, not auto")
		}
		s.Carrier = strings.Join(AutoCarriers(e.ListenPorts, goos), "+")
	case e.Carrier == spec.CarrierAuto:
		return s, errors.New("the Iran end needs a carrier: run the auto probe first")
	default:
		s.Carrier = e.Carrier
	}
	return s, s.Validate(goos)
}

// AutoCarriers is the carrier set a Kharej auto end serves: every planned port, plus icmp
// where raw sockets exist.
func AutoCarriers(listen map[string]int, goos string) []string {
	var out []string
	for _, c := range spec.Carriers {
		if _, ok := listen[c]; ok || (c == "icmp" && goos != "windows") {
			out = append(out, c)
		}
	}
	return out
}

// ListenCarriers names the carriers an end opens listen ports for.
func (e EndConfig) ListenCarriers(goos string) []string {
	if e.Role == spec.RoleKharej && !e.Reverse {
		if e.Carrier == spec.CarrierAuto {
			return AutoCarriers(e.ListenPorts, goos)
		}
		return []string{e.Carrier}
	}
	if e.Role == spec.RoleIran && e.Reverse && spec.IsPoolCarrier(e.Carrier) {
		return []string{e.Carrier}
	}
	return nil
}

// ListenedPorts lists the (proto, port) pairs this end binds on its host, public mappings
// included. Reverse Iran and non-reverse Kharej bind a carrier port.
func (e EndConfig) ListenedPorts(goos string) []Bind {
	var out []Bind
	for _, c := range e.ListenCarriers(goos) {
		if p, ok := e.ListenPorts[c]; ok && c != "icmp" {
			proto := "tcp"
			if c == "udp" {
				proto = "udp"
			}
			out = append(out, Bind{Proto: proto, Port: p, What: "carrier " + c})
		}
	}
	if e.Role == spec.RoleIran {
		for _, p := range e.Ports {
			proto := "tcp"
			if p.UDP {
				proto = "udp"
			}
			out = append(out, Bind{Proto: proto, Port: p.Listen, What: "forwarded port"})
		}
	}
	slices.SortFunc(out, func(a, b Bind) int { return a.Port - b.Port })
	return out
}

// Bind is one port an end listens on.
type Bind struct {
	Proto string
	Port  int
	What  string
}
