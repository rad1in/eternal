// Package probe finds which carrier works between an Iran end and its Kharej end by starting
// one throwaway wattun client per carrier, then ranks the ones that connect by speed.
package probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

// Options configures one probe run.
type Options struct {
	Binary   string
	Token    spec.Token // an auto token: its plan lists the Kharej port of each carrier
	WorkDir  string     // scratch space for the key file; created if missing
	LinkWait time.Duration
	MB       int // megabytes moved each way by the speed test
}

// CarrierResult is the outcome for one carrier.
type CarrierResult struct {
	Carrier  string  `json:"carrier" example:"tcp"`
	Peer     string  `json:"peer" example:"203.0.113.7:4443"`
	OK       bool    `json:"ok" example:"true"`
	DownMBps float64 `json:"downMBps" example:"31.5"`
	UpMBps   float64 `json:"upMBps" example:"28.2"`
	RttMs    float64 `json:"rttMs" example:"42"`
	Why      string  `json:"why,omitempty" example:""`
}

type attempt struct {
	res     *CarrierResult
	cmd     *exec.Cmd
	metrics int
}

// Run probes every carrier in the token's plan (and icmp) and returns the results best first.
func Run(ctx context.Context, o Options) ([]CarrierResult, error) {
	if o.Token.Carrier != spec.CarrierAuto {
		return nil, errors.New("only an auto token can be probed")
	}
	plan, err := o.Token.DirectPlan()
	if err != nil {
		return nil, err
	}
	host, _ := spec.HostPort(o.Token.Peer)
	if o.LinkWait <= 0 {
		o.LinkWait = 8 * time.Second
	}
	if o.MB <= 0 {
		o.MB = 8
	}
	if err := os.MkdirAll(o.WorkDir, 0o700); err != nil {
		return nil, err
	}
	keyFile := filepath.Join(o.WorkDir, "probe.key")
	if err := os.WriteFile(keyFile, []byte(o.Token.Key), 0o600); err != nil {
		return nil, err
	}
	defer os.Remove(keyFile)

	var attempts []*attempt
	defer func() {
		for _, a := range attempts {
			stop(a.cmd)
		}
	}()
	for _, c := range spec.Carriers {
		peer := host
		if c == "icmp" {
			if runtime.GOOS == "windows" {
				continue
			}
		} else if p, ok := plan[c]; ok {
			peer = net.JoinHostPort(host, strconv.Itoa(p))
		} else {
			continue
		}
		a := &attempt{res: &CarrierResult{Carrier: c, Peer: peer}}
		attempts = append(attempts, a)
		if err := a.start(ctx, o, keyFile); err != nil {
			a.res.Why = err.Error()
			a.cmd = nil
		}
	}

	deadline := time.Now().Add(o.LinkWait)
	var wg sync.WaitGroup
	for _, a := range attempts {
		if a.cmd == nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.waitAlive(ctx, deadline, o.LinkWait)
		}()
	}
	wg.Wait()
	for _, a := range attempts {
		if a.res.OK && ctx.Err() == nil {
			a.measure(ctx, o.MB)
		}
	}
	out := make([]CarrierResult, 0, len(attempts))
	for _, a := range attempts {
		out = append(out, *a.res)
	}
	rank(out)
	return out, ctx.Err()
}

func (a *attempt) start(ctx context.Context, o Options, keyFile string) error {
	mp, err := freePort()
	if err != nil {
		return err
	}
	lp, err := freePort()
	if err != nil {
		return err
	}
	a.metrics = mp
	args := []string{
		"-mode", "client", "-carrier", a.res.Carrier, "-key-file", keyFile, "-peer", a.res.Peer,
		"-metrics", "127.0.0.1:" + strconv.Itoa(mp), "-map", fmt.Sprintf("%d:1@127.0.0.1", lp),
	}
	switch a.res.Carrier {
	case "ws", "wss", "h2", "h2c", "grpc":
		args = append(args, "-path", pathOr(o.Token.Path))
	}
	if o.Token.Sni != "" && (a.res.Carrier == "tls" || a.res.Carrier == "wss" || a.res.Carrier == "h2" || a.res.Carrier == "grpc") {
		args = append(args, "-sni", o.Token.Sni, "-host", o.Token.Sni)
	}
	if a.res.Carrier == "icmp" && o.Token.IcmpType != "" {
		args = append(args, "-icmp-type", o.Token.IcmpType)
	}
	a.cmd = exec.CommandContext(ctx, o.Binary, args...) //nolint:gosec // binary is the panel's own wattun
	return a.cmd.Start()
}

func pathOr(p string) string {
	if p == "" {
		return "/wat"
	}
	return p
}

func (a *attempt) stats(ctx context.Context) (alive bool, rtt float64, err error) {
	var st struct {
		Alive bool    `json:"alive"`
		Srtt  float64 `json:"srtt_ms"`
	}
	if err := getJSON(ctx, fmt.Sprintf("http://127.0.0.1:%d/stats", a.metrics), 2*time.Second, &st); err != nil {
		return false, 0, err
	}
	return st.Alive, st.Srtt, nil
}

func (a *attempt) waitAlive(ctx context.Context, deadline time.Time, wait time.Duration) {
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if alive, rtt, err := a.stats(ctx); err == nil && alive {
			a.res.OK, a.res.RttMs = true, rtt
			return
		}
		time.Sleep(400 * time.Millisecond)
	}
	a.res.Why = fmt.Sprintf("no link in %d s", int(wait.Seconds()))
}

func (a *attempt) measure(ctx context.Context, mb int) {
	limit := time.Duration(max(20, mb*2)) * time.Second
	var r struct {
		OK   bool    `json:"ok"`
		MBps float64 `json:"mb_per_s"`
	}
	base := fmt.Sprintf("http://127.0.0.1:%d/speedtest?mb=%d&dir=", a.metrics, mb)
	if err := getJSON(ctx, base+"down", limit, &r); err != nil || !r.OK {
		a.res.OK, a.res.Why = false, "speed test failed or stalled"
		return
	}
	a.res.DownMBps = r.MBps
	r.OK, r.MBps = false, 0
	if err := getJSON(ctx, base+"up", limit, &r); err != nil || !r.OK {
		a.res.OK, a.res.Why = false, "speed test failed or stalled"
		return
	}
	a.res.UpMBps = r.MBps
	if alive, rtt, err := a.stats(ctx); err == nil && alive && rtt > 0 {
		a.res.RttMs = rtt
	}
}

func rank(rs []CarrierResult) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].OK != rs[j].OK {
			return rs[i].OK
		}
		si, sj := rs[i].DownMBps+rs[i].UpMBps, rs[j].DownMBps+rs[j].UpMBps
		if si != sj {
			return si > sj
		}
		return rs[i].RttMs < rs[j].RttMs
	})
}

// Best returns the fastest carrier that connected.
func Best(rs []CarrierResult) (CarrierResult, bool) {
	for _, r := range rs {
		if r.OK {
			return r, true
		}
	}
	return CarrierResult{}, false
}

func stop(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

func freePort() (int, error) {
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func getJSON(ctx context.Context, url string, timeout time.Duration, dst any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, strings.TrimSpace(resp.Status))
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}
