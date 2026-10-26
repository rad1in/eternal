package spec

import (
	"fmt"
	"strconv"
	"strings"
)

// PortMap is one forwarded port. Listen is where users connect on the Iran end, Target is the
// port on the Kharej host (127.0.0.1). Rate and quota are enforced on the Iran listener.
type PortMap struct {
	Listen   int     `json:"listen" example:"443"`
	Target   int     `json:"target" example:"443"`
	UDP      bool    `json:"udp" example:"false"`
	RateMBps float64 `json:"rateMBps,omitempty" example:"0"`
	QuotaGB  float64 `json:"quotaGB,omitempty" example:"0"`
}

func validPortNumber(n int) bool { return n >= 1 && n <= 65535 }

// ParsePort reads "443", "8443:443", "53/udp" or "5353:53/udp".
func ParsePort(s string) (PortMap, error) {
	s = strings.TrimSpace(s)
	var p PortMap
	if rest, ok := strings.CutSuffix(s, "/udp"); ok {
		s, p.UDP = rest, true
	} else if rest, ok := strings.CutSuffix(s, "/tcp"); ok {
		s = rest
	}
	l, t, mapped := strings.Cut(s, ":")
	listen, err := strconv.Atoi(strings.TrimSpace(l))
	if err != nil || !validPortNumber(listen) {
		return PortMap{}, fmt.Errorf("bad port %q", l)
	}
	p.Listen, p.Target = listen, listen
	if mapped {
		target, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil || !validPortNumber(target) {
			return PortMap{}, fmt.Errorf("bad port %q", t)
		}
		p.Target = target
	}
	return p, nil
}

// ParsePorts reads a comma separated list of ParsePort items.
func ParsePorts(s string) ([]PortMap, error) {
	var out []PortMap
	for item := range strings.SplitSeq(s, ",") {
		if strings.TrimSpace(item) == "" {
			continue
		}
		p, err := ParsePort(item)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := CheckPorts(out); err != nil {
		return nil, err
	}
	return out, nil
}

// CheckPorts rejects out-of-range numbers and a public port mapped twice.
func CheckPorts(ports []PortMap) error {
	seen := map[string]bool{}
	for _, p := range ports {
		if !validPortNumber(p.Listen) || !validPortNumber(p.Target) {
			return fmt.Errorf("port %d:%d is out of range", p.Listen, p.Target)
		}
		if p.RateMBps < 0 || p.QuotaGB < 0 {
			return fmt.Errorf("port %d: rate and quota cannot be negative", p.Listen)
		}
		k := fmt.Sprintf("%d/%t", p.Listen, p.UDP)
		if seen[k] {
			return fmt.Errorf("public port %d is mapped twice", p.Listen)
		}
		seen[k] = true
	}
	return nil
}

// AllowString is the token's allowed-ports field: target ports only, "443,53/udp".
func AllowString(ports []PortMap) string {
	var out []string
	seen := map[string]bool{}
	for _, p := range ports {
		item := strconv.Itoa(p.Target)
		if p.UDP {
			item += "/udp"
		}
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return strings.Join(out, ",")
}

// PortsFromAllow turns a token's allowed-ports field into plain listen==target mappings.
func PortsFromAllow(allow string) ([]PortMap, error) {
	return ParsePorts(allow)
}

// FormatPort is the inverse of ParsePort, without limits.
func FormatPort(p PortMap) string {
	s := strconv.Itoa(p.Listen)
	if p.Target != p.Listen {
		s += ":" + strconv.Itoa(p.Target)
	}
	if p.UDP {
		s += "/udp"
	}
	return s
}
