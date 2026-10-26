package spec

import (
	"reflect"
	"testing"
)

func TestParsePort(t *testing.T) {
	cases := []struct {
		in   string
		want PortMap
		bad  bool
	}{
		{in: "443", want: PortMap{Listen: 443, Target: 443}},
		{in: "8443:443", want: PortMap{Listen: 8443, Target: 443}},
		{in: "53/udp", want: PortMap{Listen: 53, Target: 53, UDP: true}},
		{in: "5353:53/udp", want: PortMap{Listen: 5353, Target: 53, UDP: true}},
		{in: " 443/tcp ", want: PortMap{Listen: 443, Target: 443}},
		{in: "0", bad: true},
		{in: "65536", bad: true},
		{in: "80:", bad: true},
		{in: "a:b", bad: true},
		{in: "", bad: true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParsePort(c.in)
			if c.bad {
				if err == nil {
					t.Fatalf("ParsePort(%q) = %+v, want an error", c.in, got)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("ParsePort(%q) = %+v, %v; want %+v", c.in, got, err, c.want)
			}
		})
	}
}

func TestParsePortsRejectsDuplicateListen(t *testing.T) {
	if _, err := ParsePorts("443,8443:443,443:80"); err == nil {
		t.Fatal("two mappings on public port 443/tcp were accepted")
	}
	got, err := ParsePorts("443,443/udp")
	if err != nil {
		t.Fatalf("tcp and udp on one number are distinct listeners: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ParsePorts = %+v", got)
	}
}

func TestAllowString(t *testing.T) {
	ports := []PortMap{{Listen: 8443, Target: 443}, {Listen: 443, Target: 443}, {Listen: 5353, Target: 53, UDP: true}}
	if got, want := AllowString(ports), "443,53/udp"; got != want {
		t.Fatalf("AllowString = %q, want %q (targets only, deduplicated)", got, want)
	}
	back, err := PortsFromAllow("443,53/udp")
	if err != nil || !reflect.DeepEqual(back, []PortMap{{Listen: 443, Target: 443}, {Listen: 53, Target: 53, UDP: true}}) {
		t.Fatalf("PortsFromAllow = %+v, %v", back, err)
	}
}
