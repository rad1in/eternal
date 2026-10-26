package spec

import (
	"reflect"
	"testing"
)

// Vectors come from `base64 -w0` over the pipe string wattun-manager builds, so the panel
// and the shell manager stay token-compatible both ways.
func TestParseTokenReadsManagerTokens(t *testing.T) {
	cases := []struct {
		name  string
		token string
		want  Token
	}{
		{
			"fixed carrier with allowed ports and reverse flag",
			"wat1:dGNwfDIwMy4wLjExMy43OjQ0NDN8azN5SzN5SzN5SzN5fHx8fDQ0Myw1My91ZHB8MQ==",
			Token{Carrier: "tcp", Peer: "203.0.113.7:4443", Key: "k3yK3yK3yK3y", Allow: "443,53/udp", Reverse: true},
		},
		{
			"auto token carries the plan in field 9",
			"wat1:YXV0b3wyMDMuMC4xMTMuN3xrM3lLM3lLM3lLM3l8L3dhdHxjZG4uZXhhbXBsZS5jb218fDQ0M3x8ZD11ZHA6NDQ0Myx0Y3A6NDQ0MztyPXRjcDoyNDQzMQ==",
			Token{Carrier: "auto", Peer: "203.0.113.7", Key: "k3yK3yK3yK3y", Path: "/wat", Sni: "cdn.example.com", Allow: "443", Plan: "d=udp:4443,tcp:4443;r=tcp:24431"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseToken(c.token)
			if err != nil {
				t.Fatalf("ParseToken: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("ParseToken = %+v, want %+v", got, c.want)
			}
			if enc := got.Encode(); enc != c.token {
				t.Fatalf("Encode round trip = %q, want %q", enc, c.token)
			}
		})
	}
}

func TestParseTokenRejectsMalformed(t *testing.T) {
	for name, tok := range map[string]string{
		"no prefix":      "dGNwfDF8aw==",
		"not base64":     "wat1:%%%",
		"missing key":    "wat1:dGNwfDEuMS4xLjE=",
		"empty carrier":  "wat1:fDEuMS4xLjF8a2V5a2V5a2V5",
		"unknown":        "wat1:Zm9vfDEuMS4xLjF8a2V5a2V5a2V5",
		"empty":          "",
		"six field junk": "wat1:YXxifGM=",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseToken(tok); err == nil {
				t.Fatalf("ParseToken(%q) accepted a malformed token", tok)
			}
		})
	}
}

func TestTokenPlanPorts(t *testing.T) {
	tok := Token{Carrier: "auto", Peer: "h", Key: "k", Plan: "d=udp:4443,tcp:4444;r=tcp:24431"}
	got, err := tok.DirectPlan()
	if err != nil {
		t.Fatalf("DirectPlan: %v", err)
	}
	want := map[string]int{"udp": 4443, "tcp": 4444}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DirectPlan = %v, want %v", got, want)
	}
	if _, err := (Token{Plan: "d=tcp:99999"}).DirectPlan(); err == nil {
		t.Fatal("DirectPlan accepted port 99999")
	}
}
