package spec

import "fmt"

var preferredListen = map[string][]int{
	"tcp":  {4443, 4444, 4445, 9444},
	"udp":  {4443, 4444, 4445, 9444},
	"tls":  {8443, 8444, 8445, 9443, 2096},
	"wss":  {2053, 2096, 8443, 443, 2083, 2087},
	"grpc": {2083, 2087, 2096, 8443, 2053, 443},
	"h2":   {2087, 2096, 8443, 2083, 2053, 443},
	"ws":   {2052, 2082, 2086, 2095, 8080, 8880},
	"h2c":  {2082, 2086, 2095, 8080, 8880, 2052},
}

// PickListenPorts chooses a listen port for each carrier that needs one, preferring the
// ports a CDN passes. busy reports a port that is taken for the given proto ("tcp"|"udp").
func PickListenPorts(carriers []string, busy func(proto string, port int) bool) (map[string]int, error) {
	taken := map[string]bool{}
	out := map[string]int{}
	for _, c := range carriers {
		if c == "icmp" {
			continue
		}
		proto := "tcp"
		if c == "udp" {
			proto = "udp"
		}
		candidates := append([]int{}, preferredListen[c]...)
		for p := 24440; p <= 24999; p++ {
			candidates = append(candidates, p)
		}
		picked := 0
		for _, p := range candidates {
			if taken[fmt.Sprintf("%s/%d", proto, p)] || busy(proto, p) {
				continue
			}
			picked = p
			break
		}
		if picked == 0 {
			return nil, fmt.Errorf("no free %s port left for the %s carrier", proto, c)
		}
		taken[fmt.Sprintf("%s/%d", proto, picked)] = true
		out[c] = picked
	}
	return out, nil
}
