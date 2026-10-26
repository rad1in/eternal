package spec

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// TokenPrefix starts every pairing token.
const TokenPrefix = "wat1:"

// Token is the pairing token the Kharej end hands to the Iran end. It carries the key, so
// it is a secret and is never logged.
type Token struct {
	Carrier  string
	Peer     string
	Key      string
	Path     string
	Sni      string
	IcmpType string
	Allow    string
	Reverse  bool
	Plan     string
}

// Encode renders the format wattun-manager reads: wat1:BASE64(carrier|peer|key|path|sni|icmp|allow[|reverse[|plan]]).
func (t Token) Encode() string {
	f := strings.Join([]string{t.Carrier, t.Peer, t.Key, t.Path, t.Sni, t.IcmpType, t.Allow}, "|")
	rev := ""
	if t.Reverse {
		rev = "1"
	}
	switch {
	case t.Plan != "":
		f += "|" + rev + "|" + t.Plan
	case t.Reverse:
		f += "|1"
	}
	return TokenPrefix + base64.StdEncoding.EncodeToString([]byte(f))
}

// ParseToken decodes a pairing token and checks the fields the daemon cannot work without.
func ParseToken(s string) (Token, error) {
	s = strings.TrimSpace(s)
	raw, ok := strings.CutPrefix(s, TokenPrefix)
	if !ok {
		return Token{}, errors.New("not a wat1: token")
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return Token{}, errors.New("token is not valid base64")
	}
	f := strings.Split(string(b), "|")
	if len(f) < 3 {
		return Token{}, errors.New("token has too few fields")
	}
	get := func(i int) string {
		if i < len(f) {
			return f[i]
		}
		return ""
	}
	t := Token{
		Carrier: get(0), Peer: get(1), Key: get(2), Path: get(3), Sni: get(4),
		IcmpType: get(5), Allow: get(6), Reverse: get(7) == "1", Plan: get(8),
	}
	if t.Carrier != CarrierAuto && !ValidCarrier(t.Carrier) {
		return Token{}, fmt.Errorf("token carrier %q is not known", t.Carrier)
	}
	if t.Peer == "" || t.Key == "" {
		return Token{}, errors.New("token has no address or key")
	}
	return t, nil
}

// DirectPlan reads the d= part of an auto token's plan: the Kharej listen port of each carrier.
func (t Token) DirectPlan() (map[string]int, error) {
	out := map[string]int{}
	for part := range strings.SplitSeq(t.Plan, ";") {
		v, ok := strings.CutPrefix(part, "d=")
		if !ok {
			continue
		}
		for item := range strings.SplitSeq(v, ",") {
			name, port, ok := strings.Cut(item, ":")
			n, err := strconv.Atoi(port)
			if !ok || err != nil || !validPortNumber(n) || !ValidCarrier(name) {
				return nil, fmt.Errorf("bad plan item %q", item)
			}
			out[name] = n
		}
	}
	return out, nil
}

// FormatPlan writes carrier:port pairs in a stable order as the d= plan.
func FormatPlan(direct map[string]int) string {
	var items []string
	for _, c := range Carriers {
		if p, ok := direct[c]; ok {
			items = append(items, c+":"+strconv.Itoa(p))
		}
	}
	if len(items) == 0 {
		return ""
	}
	return "d=" + strings.Join(items, ",") + ";r="
}

// HostPort splits "host:port" and "host", the two shapes a token's peer takes.
func HostPort(peer string) (host string, port int) {
	if i := strings.LastIndex(peer, ":"); i > 0 && !strings.Contains(peer[i:], "]") && strings.Count(peer, ":") == 1 {
		if n, err := strconv.Atoi(peer[i+1:]); err == nil {
			return peer[:i], n
		}
	}
	if strings.HasPrefix(peer, "[") {
		if h, p, ok := strings.Cut(strings.TrimPrefix(peer, "["), "]:"); ok {
			if n, err := strconv.Atoi(p); err == nil {
				return h, n
			}
		}
	}
	return strings.Trim(peer, "[]"), 0
}
