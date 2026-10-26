package spec

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Roles of a tunnel end. Kharej is the server outside the filter, Iran the client inside it.
const (
	RoleKharej = "kharej"
	RoleIran   = "iran"
)

// CarrierAuto asks the Iran end to probe every carrier and keep the best one.
const CarrierAuto = "auto"

// Carriers lists the wire transports the daemon speaks, in the order auto mode tries them.
var Carriers = []string{"tcp", "udp", "tls", "ws", "wss", "h2", "h2c", "grpc", "icmp"}

// PoolCarriers are the connection-oriented carriers; they support reverse mode and a pool.
var PoolCarriers = []string{"tcp", "tls", "ws", "wss", "h2", "h2c", "grpc"}

// ValidCarrier reports whether name is one wire carrier.
func ValidCarrier(name string) bool { return slices.Contains(Carriers, name) }

// IsPoolCarrier reports whether name is a connection-pool carrier.
func IsPoolCarrier(name string) bool { return slices.Contains(PoolCarriers, name) }

// NeedsRawSocket reports carriers and options that open raw sockets, which Windows lacks.
func NeedsRawSocket(carrier string) bool { return carrier == "icmp" }

// EndOptions are the tuning and anti-censorship settings of the dialing end. Every field is
// optional; the zero value leaves the daemon default in place.
type EndOptions struct {
	MaxMBps         int    `json:"maxMBps,omitempty" example:"0"`
	Fec             string `json:"fec,omitempty" example:"off"`
	Sessions        int    `json:"sessions,omitempty" example:"1"`
	Conns           int    `json:"conns,omitempty" example:"0"`
	Bind            string `json:"bind,omitempty" example:"0.0.0.0"`
	FakeSni         string `json:"fakeSni,omitempty" example:"www.cloudflare.com"`
	FakeMode        string `json:"fakeMode,omitempty" example:"rst"`
	FakeTtl         int    `json:"fakeTtl,omitempty" example:"0"`
	PoolRotate      string `json:"poolRotate,omitempty" example:"30m"`
	PoolRotateBytes string `json:"poolRotateBytes,omitempty" example:"4GB"`
	Ech             string `json:"ech,omitempty" example:"auto"`
	Fp              string `json:"fp,omitempty" example:"chrome"`
	TlsSmallCH      bool   `json:"tlsSmallCh,omitempty" example:"false"`
	Governor        bool   `json:"governor,omitempty" example:"false"`
}

// IsZero reports whether no option is set.
func (o EndOptions) IsZero() bool { return o == EndOptions{} }

// Spec is everything the daemon needs to run one tunnel end. The supervisor fills KeyFile
// and Metrics; the rest comes from the stored tunnel.
type Spec struct {
	Role     string
	Carrier  string // one carrier, or several joined with "+" to bond them
	Key      string
	KeyFile  string
	Metrics  string
	Listen   map[string]int // carrier -> port the end listens on (server, or reverse client)
	Peer     string         // the address this end dials (client, or reverse server)
	Path     string
	Sni      string
	IcmpType string
	Reverse  bool
	Ports    []PortMap
	Options  EndOptions
}

var (
	safeValue   = regexp.MustCompile(`^[A-Za-z0-9._:/@+=,\[\]-]*$`)
	hostName    = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)
	rotateRe    = regexp.MustCompile(`^[0-9]+(ms|s|m|h)?$`)
	rotateBytes = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?([kKmMgGtT][bB]?)?$`)
	echConfig   = regexp.MustCompile(`^[A-Za-z0-9+/=_-]{16,}$`)
	fakeModes   = []string{"seq", "ttl", "sum", "seqsum", "rst", "auto", "split", "hlen0", "fakeip", "synrst", "all", "ff"}
	fingerprint = []string{"go", "chrome", "firefox", "safari", "edge", "random"}
	icmpTypes   = []string{"echo", "timestamp", "info", "mask"}
)

// CarrierList splits the bonded carrier string.
func (s Spec) CarrierList() []string { return strings.Split(s.Carrier, "+") }

// Validate rejects a spec the daemon would refuse or that could smuggle config lines.
// goos is the platform the daemon will run on.
func (s Spec) Validate(goos string) error {
	if s.Role != RoleKharej && s.Role != RoleIran {
		return fmt.Errorf("unknown role %q", s.Role)
	}
	if len(s.Key) < 8 || !safeValue.MatchString(s.Key) {
		return errors.New("the key must be at least 8 characters of letters, digits and . _ - + = /")
	}
	carriers := s.CarrierList()
	for _, c := range carriers {
		if !ValidCarrier(c) {
			return fmt.Errorf("unknown carrier %q", c)
		}
		if goos == "windows" && NeedsRawSocket(c) {
			return fmt.Errorf("the %s carrier needs raw sockets, which Windows does not have", c)
		}
	}
	if strings.ContainsAny(s.KeyFile, "\"\n\r#") {
		return errors.New("the key file path has characters a config file cannot hold")
	}
	for _, v := range []string{s.Path, s.Sni, s.Peer, s.IcmpType} {
		if !safeValue.MatchString(v) {
			return fmt.Errorf("value %q has characters a config file cannot hold", v)
		}
	}
	if s.Path != "" && !strings.HasPrefix(s.Path, "/") {
		return errors.New("the path must start with /")
	}
	if s.IcmpType != "" && !slices.Contains(icmpTypes, s.IcmpType) {
		return fmt.Errorf("unknown ICMP type %q", s.IcmpType)
	}
	if (s.Role == RoleIran || s.Reverse) && s.Peer == "" {
		return errors.New("the peer address is missing")
	}
	for c, p := range s.Listen {
		if !ValidCarrier(c) || !validPortNumber(p) {
			return fmt.Errorf("bad listen port %s:%d", c, p)
		}
	}
	if err := CheckPorts(s.Ports); err != nil {
		return err
	}
	return s.Options.validate(goos)
}

// ValidateOptions checks the option values on their own, for the platform they will run on.
func (o EndOptions) ValidateOptions() error { return o.validate("linux") }

func (o EndOptions) validate(goos string) error {
	checks := []struct {
		bad bool
		msg string
	}{
		{o.MaxMBps < 0 || o.MaxMBps > 9999, "max speed must be 0-9999 MB/s"},
		{o.Fec != "" && o.Fec != "off" && o.Fec != "auto", "fec must be off or auto"},
		{o.Sessions < 0 || o.Sessions > 16, "sessions must be 0-16"},
		{o.Conns < 0 || o.Conns > 128, "conns must be 0-128"},
		{o.Bind != "" && net.ParseIP(o.Bind) == nil, "bind must be an IP address"},
		{o.FakeSni != "" && !hostName.MatchString(o.FakeSni), "fake SNI must be a host name"},
		{o.FakeMode != "" && !slices.Contains(fakeModes, o.FakeMode), "unknown fake mode"},
		{o.FakeTtl < 0 || o.FakeTtl > 255, "fake TTL must be 0-255"},
		{o.PoolRotate != "" && o.PoolRotate != "off" && !rotateRe.MatchString(o.PoolRotate), "pool rotate must look like 30m or 4h"},
		{o.PoolRotateBytes != "" && o.PoolRotateBytes != "off" && !rotateBytes.MatchString(o.PoolRotateBytes), "pool rotate bytes must look like 500MB"},
		{o.Ech != "" && !slices.Contains([]string{"off", "auto", "ladder"}, o.Ech) && !echConfig.MatchString(o.Ech), "ech must be off, auto, ladder or a base64 config"},
		{o.Fp != "" && !slices.Contains(fingerprint, o.Fp), "unknown TLS fingerprint"},
	}
	for _, c := range checks {
		if c.bad {
			return errors.New(c.msg)
		}
	}
	if goos == "windows" && o.FakeSni != "" {
		return errors.New("fake SNI needs raw sockets, which Windows does not have")
	}
	return nil
}

// Render writes the daemon's YAML-subset config file. The ports list comes last because the
// daemon hot-reloads it without a restart.
func (s Spec) Render() (string, error) {
	if err := s.Validate("linux"); err != nil {
		return "", err
	}
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	client := s.Role == RoleIran
	if client {
		w("mode: client")
	} else {
		w("mode: server")
	}
	w(`key-file: "%s"`, s.KeyFile)
	w("carrier: %s", s.Carrier)
	carriers := s.CarrierList()
	// The end that accepts carrier connections listens; with reverse the Iran client does.
	if client == s.Reverse {
		if len(carriers) == 1 {
			if p, ok := s.Listen[carriers[0]]; ok && carriers[0] != "icmp" {
				w("listen: %q", ":"+strconv.Itoa(p))
			}
		} else {
			for _, c := range carriers {
				if p, ok := s.Listen[c]; ok && c != "icmp" {
					w("listen-%s: %q", c, ":"+strconv.Itoa(p))
				}
			}
		}
	}
	if s.Peer != "" {
		w("peer: %s", s.Peer)
	}
	if s.Path != "" {
		w("path: %s", s.Path)
	}
	if s.Sni != "" {
		w("sni: %s", s.Sni)
		w("host: %s", s.Sni)
	}
	if s.IcmpType != "" && s.IcmpType != "echo" {
		w("icmp-type: %s", s.IcmpType)
	}
	if s.Reverse {
		w("reverse: true")
	}
	s.Options.render(&b)
	w("metrics: %s", s.Metrics)
	w("ports:")
	for _, p := range s.Ports {
		if client {
			w("  - %s", FormatPort(PortMap{Listen: p.Listen, Target: p.Target, UDP: p.UDP}))
		} else {
			w("  - %s", FormatPort(PortMap{Listen: p.Target, Target: p.Target, UDP: p.UDP}))
		}
	}
	if client {
		var limits []string
		for _, p := range s.Ports {
			if p.RateMBps == 0 && p.QuotaGB == 0 {
				continue
			}
			item := strconv.Itoa(p.Listen)
			if p.RateMBps > 0 {
				item += " rate=" + strconv.FormatFloat(p.RateMBps, 'f', -1, 64) + "MB"
			}
			if p.QuotaGB > 0 {
				item += " quota=" + strconv.FormatFloat(p.QuotaGB, 'f', -1, 64) + "GB"
			}
			limits = append(limits, item)
		}
		if len(limits) > 0 {
			w("limits:")
			for _, l := range limits {
				w("  - %s", l)
			}
		}
	}
	return b.String(), nil
}

func (o EndOptions) render(b *strings.Builder) {
	kv := func(k string, v any) { fmt.Fprintf(b, "%s: %v\n", k, v) }
	if o.FakeSni != "" {
		kv("fake-sni", o.FakeSni)
		if o.FakeMode != "" {
			kv("fake-mode", o.FakeMode)
		}
		if o.FakeMode == "ttl" && o.FakeTtl > 0 {
			kv("fake-ttl", o.FakeTtl)
		}
	}
	if o.PoolRotate != "" && o.PoolRotate != "off" {
		kv("pool-rotate", o.PoolRotate)
	}
	if o.PoolRotateBytes != "" && o.PoolRotateBytes != "off" {
		kv("pool-rotate-bytes", o.PoolRotateBytes)
	}
	if o.Ech != "" && o.Ech != "off" {
		kv("ech", o.Ech)
	}
	if o.Fp != "" && o.Fp != "go" {
		kv("fp", o.Fp)
	}
	if o.TlsSmallCH {
		kv("tls-small-ch", true)
	}
	if o.Governor {
		kv("governor", true)
	}
	if o.Bind != "" && o.Bind != "0.0.0.0" {
		kv("bind", o.Bind)
	}
	if o.MaxMBps > 0 {
		kv("max-mbps", o.MaxMBps)
	}
	if o.Fec == "auto" {
		kv("fec", "auto")
	}
	if o.Sessions > 1 {
		kv("sessions", o.Sessions)
	}
	if o.Conns > 0 {
		kv("conns", o.Conns)
	}
}

// ForEnd drops the options the given end would ignore or reject: a serving end takes no
// dial-side tuning, and with reverse the dialing end is the Kharej one.
func (o EndOptions) ForEnd(role string, reverse bool) EndOptions {
	dials := (role == RoleIran) != reverse
	out := EndOptions{MaxMBps: o.MaxMBps, Fec: o.Fec}
	switch {
	case dials && role == RoleIran:
		return o
	case dials: // reverse Kharej: the server dials Iran
		out.FakeSni, out.FakeMode, out.FakeTtl, out.Fp = o.FakeSni, o.FakeMode, o.FakeTtl, o.Fp
	case role == RoleIran: // reverse Iran: a listener
		out.Bind = o.Bind
	}
	return out
}
