package main

// Panel-facing subcommands, added by Eternal. The engine itself is always started with flags;
// a first argument that is not a flag selects one of these instead.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/probe"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

func runSubcommand() {
	if len(os.Args) < 2 || strings.HasPrefix(os.Args[1], "-") {
		return
	}
	var err error
	switch os.Args[1] {
	case "setup":
		err = cmdSetup(os.Args[2:], os.Stdout)
	case "probe":
		err = cmdProbe(os.Args[2:], os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "wattun: unknown command %q (engine options start with '-', see wattun -h; commands: setup, probe)\n", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wattun:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func cmdProbe(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	token := fs.String("token", "", "auto token (wat1:...)")
	mb := fs.Int("mb", 8, "megabytes moved each way by the speed test")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tok, err := spec.ParseToken(*token)
	if err != nil {
		return err
	}
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "wattun-probe-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	res, err := probe.Run(context.Background(), probe.Options{Binary: bin, Token: tok, WorkDir: dir, MB: *mb})
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(res)
}

// cmdSetup writes the key and config of one tunnel end under -dir, the way the panel does.
// Iran: -token (and -map); Kharej: -carrier, -addr, -ports. It prints CONFIG= and, for Kharej, TOKEN=.
func cmdSetup(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	var (
		dir     = fs.String("dir", ".", "directory for the key and config")
		name    = fs.String("name", "tunnel", "tunnel name (file name)")
		role    = fs.String("role", "", "iran or kharej (default: iran when -token is given)")
		token   = fs.String("token", "", "pairing token from the Kharej end")
		mapArg  = fs.String("map", "", "iran: listen:target[/udp],... ports users connect to")
		portArg = fs.String("ports", "", "kharej: ports the tunnel may reach on this server, 443,53/udp")
		carrier = fs.String("carrier", "tcp", "kharej: carrier or auto")
		addr    = fs.String("addr", "", "kharej: public address Iran dials")
		listen  = fs.Int("listen", 0, "kharej: listen port (0 = pick)")
		path    = fs.String("path", "/wat", "kharej: URL path of ws/wss/h2/h2c/grpc")
		sni     = fs.String("sni", "", "kharej: server name Iran sends")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *role == "" {
		*role = spec.RoleKharej
		if *token != "" {
			*role = spec.RoleIran
		}
	}
	if err := os.MkdirAll(*dir, 0o700); err != nil {
		return err
	}
	metrics, err := freeLoopbackPort()
	if err != nil {
		return err
	}
	s := spec.Spec{Role: *role, KeyFile: filepath.Join(*dir, *name+".key"), Metrics: "127.0.0.1:" + strconv.Itoa(metrics)}
	var tok string
	if *role == spec.RoleIran {
		if s, err = iranSpecFromToken(s, *token, *mapArg); err != nil {
			return err
		}
	} else {
		if s, tok, err = kharejSpec(s, *carrier, *addr, *listen, *path, *sni, *portArg); err != nil {
			return err
		}
	}
	yml, err := s.Render()
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.KeyFile, []byte(s.Key+"\n"), 0o600); err != nil {
		return err
	}
	cfg := filepath.Join(*dir, *name+".yml")
	if err := os.WriteFile(cfg, []byte(yml), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(out, "CONFIG=%s\nCARRIER=%s\n", cfg, s.Carrier)
	for c, p := range s.Listen {
		proto := "tcp"
		if c == "udp" {
			proto = "udp"
		}
		fmt.Fprintf(out, "LISTEN=%d/%s\n", p, proto)
	}
	if tok != "" {
		fmt.Fprintf(out, "TOKEN=%s\n", tok)
	}
	return nil
}

func iranSpecFromToken(s spec.Spec, token, mapArg string) (spec.Spec, error) {
	tok, err := spec.ParseToken(token)
	if err != nil {
		return s, err
	}
	ports, err := spec.ParsePorts(mapArg)
	if err != nil {
		return s, err
	}
	if len(ports) == 0 {
		return s, fmt.Errorf("-map needs at least one listen:target port")
	}
	s.Key, s.Carrier, s.Peer, s.Path, s.Sni, s.IcmpType, s.Reverse, s.Ports =
		tok.Key, tok.Carrier, tok.Peer, tok.Path, tok.Sni, tok.IcmpType, tok.Reverse, ports
	if tok.Carrier == spec.CarrierAuto {
		bin, err := os.Executable()
		if err != nil {
			return s, err
		}
		work, err := os.MkdirTemp("", "wattun-probe-")
		if err != nil {
			return s, err
		}
		defer os.RemoveAll(work)
		res, err := probe.Run(context.Background(), probe.Options{Binary: bin, Token: tok, WorkDir: work})
		if err != nil {
			return s, err
		}
		best, ok := probe.Best(res)
		if !ok {
			return s, fmt.Errorf("no carrier connected to %s; check the Kharej firewall", tok.Peer)
		}
		s.Carrier, s.Peer = best.Carrier, best.Peer
	}
	if s.Reverse && spec.IsPoolCarrier(s.Carrier) {
		_, port := spec.HostPort(s.Peer)
		s.Listen = map[string]int{s.Carrier: port}
	}
	return s, nil
}

func kharejSpec(s spec.Spec, carrier, addr string, listen int, path, sni, ports string) (spec.Spec, string, error) {
	if addr == "" {
		return s, "", fmt.Errorf("-addr (the public address of this server) is required")
	}
	allow, err := spec.ParsePorts(ports)
	if err != nil {
		return s, "", err
	}
	key, err := spec.NewKey()
	if err != nil {
		return s, "", err
	}
	s.Key, s.Path, s.Sni, s.Ports = key, path, sni, allow
	carriers := []string{carrier}
	if carrier == spec.CarrierAuto {
		carriers = append([]string{"icmp"}, "udp", "tcp", "tls", "ws", "wss", "h2", "h2c", "grpc")
		if runtime.GOOS == "windows" {
			carriers = carriers[1:]
		}
	}
	s.Carrier = strings.Join(carriers, "+")
	if listen > 0 && len(carriers) == 1 {
		s.Listen = map[string]int{carrier: listen}
	} else if s.Listen, err = spec.PickListenPorts(carriers, portBusy); err != nil {
		return s, "", err
	}
	t := spec.Token{Carrier: carrier, Peer: addr, Key: key, Path: path, Sni: sni, Allow: spec.AllowString(allow)}
	if carrier == spec.CarrierAuto {
		t.Plan = spec.FormatPlan(s.Listen)
	} else if p, ok := s.Listen[carrier]; ok {
		t.Peer = net.JoinHostPort(addr, strconv.Itoa(p))
	}
	return s, t.Encode(), nil
}

func portBusy(proto string, port int) bool {
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

func freeLoopbackPort() (int, error) {
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
