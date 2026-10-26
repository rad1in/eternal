package tunnel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

// startingGrace is how long a running process may take to connect before it reads as down.
const startingGrace = 30 * time.Second

type managed struct {
	end         EndConfig
	runner      *runner
	structuralF string
	metricsPort int
	configPath  string
	lastConfig  string
}

// Manager owns the wattun processes of this host, one per tunnel end, keyed by uuid.
type Manager struct {
	dir    string
	binary func() string

	mu    sync.Mutex
	ends  map[string]*managed
	swept bool
}

// NewManager builds a manager rooted at dir that starts the binary binary() names.
func NewManager(dir string, binary func() string) *Manager {
	return &Manager{dir: dir, binary: binary, ends: map[string]*managed{}}
}

var (
	managerOnce sync.Once
	manager     *Manager
)

// GetManager returns the process-wide manager.
func GetManager() *Manager {
	managerOnce.Do(func() {
		manager = NewManager("", BinaryPath)
	})
	return manager
}

func (m *Manager) stateDir() string {
	if m.dir != "" {
		return m.dir
	}
	return StateDir()
}

func (m *Manager) paths(uuid string) (cfg, key string) {
	d := m.stateDir()
	return filepath.Join(d, uuid+".yml"), filepath.Join(d, uuid+".key")
}

func freeLoopbackPort() (int, error) {
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", config.LoopbackIP()+":0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Ensure starts the end, or applies a changed config to the running one: a ports-only change
// is written to the config file, which the daemon hot-reloads, anything else restarts it.
func (m *Manager) Ensure(e EndConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureLocked(e)
}

func (m *Manager) ensureLocked(e EndConfig) error {
	m.sweepLocked()
	if !e.Enable {
		m.removeLocked(e.Uuid)
		return nil
	}
	sp, err := e.Spec(runtime.GOOS)
	if err != nil {
		return err
	}
	cfgPath, keyPath := m.paths(e.Uuid)
	sp.KeyFile = keyPath

	// The fingerprint ignores ports and the metrics port: neither needs a restart.
	probe := sp
	probe.Ports, probe.Metrics = nil, "127.0.0.1:0"
	structural, err := probe.Render()
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(structural + "\x00" + e.Key))
	fp := hex.EncodeToString(sum[:])

	if cur := m.ends[e.Uuid]; cur != nil {
		if cur.structuralF == fp {
			sp.Metrics = fmt.Sprintf("%s:%d", config.LoopbackIP(), cur.metricsPort)
			text, err := sp.Render()
			if err != nil {
				return err
			}
			if text != cur.lastConfig {
				if err := writeFileAtomic(cfgPath, []byte(text), 0o600); err != nil {
					return err
				}
				cur.lastConfig = text
			}
			cur.end = e
			return nil
		}
		cur.runner.stop()
		delete(m.ends, e.Uuid)
	}

	bin := m.binary()
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		return fmt.Errorf("the wattun binary is not installed at %s", bin)
	}
	if err := os.MkdirAll(m.stateDir(), 0o700); err != nil {
		return err
	}
	port, err := freeLoopbackPort()
	if err != nil {
		return err
	}
	sp.Metrics = fmt.Sprintf("%s:%d", config.LoopbackIP(), port)
	text, err := sp.Render()
	if err != nil {
		return err
	}
	if err := writeFileAtomic(keyPath, []byte(e.Key+"\n"), 0o600); err != nil {
		return err
	}
	if err := writeFileAtomic(cfgPath, []byte(text), 0o600); err != nil {
		return err
	}
	r := newRunner(e.Name+"/"+e.Role, bin, []string{"-config", cfgPath})
	r.begin()
	m.ends[e.Uuid] = &managed{end: e, runner: r, structuralF: fp, metricsPort: port, configPath: cfgPath, lastConfig: text}
	logger.Infof("tunnel: started %s end of %q", e.Role, e.Name)
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (m *Manager) sweepLocked() {
	if m.swept {
		return
	}
	m.swept = true
	if n := killStrayWattun(m.binary()); n > 0 {
		logger.Warningf("tunnel: terminated %d orphaned wattun process(es) from a previous run", n)
	}
}

// Remove stops the end and deletes its generated files and counters.
func (m *Manager) Remove(uuid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLocked(uuid)
}

func (m *Manager) removeLocked(uuid string) {
	if cur := m.ends[uuid]; cur != nil {
		cur.runner.stop()
		delete(m.ends, uuid)
		logger.Infof("tunnel: stopped end %s", uuid)
	}
	cfg, key := m.paths(uuid)
	for _, f := range []string{cfg, key, cfg + ".quota.json", cfg + ".totals.json"} {
		_ = os.Remove(f)
	}
}

// Reconcile makes the running set equal to desired.
func (m *Manager) Reconcile(desired []EndConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked()
	want := map[string]bool{}
	for _, e := range desired {
		want[e.Uuid] = true
	}
	for uuid := range m.ends {
		if !want[uuid] {
			m.removeLocked(uuid)
		}
	}
	for _, e := range desired {
		if err := m.ensureLocked(e); err != nil {
			logger.Warningf("tunnel: reconcile of %q failed: %v", e.Name, err)
		}
	}
}

// StopAll stops every process, on panel shutdown. Files stay for the next start.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	var wg sync.WaitGroup
	for uuid, cur := range m.ends {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cur.runner.stop()
		}()
		delete(m.ends, uuid)
	}
	wg.Wait()
}

// Restart bounces the process of one end.
func (m *Manager) Restart(uuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := m.ends[uuid]
	if cur == nil {
		return errors.New("the tunnel is not running")
	}
	e := cur.end
	cur.runner.stop()
	delete(m.ends, uuid)
	return m.ensureLocked(e)
}

// Logs returns the last lines the end's process printed.
func (m *Manager) Logs(uuid string, lines int) ([]string, error) {
	m.mu.Lock()
	cur := m.ends[uuid]
	m.mu.Unlock()
	if cur == nil {
		return nil, errors.New("the tunnel is not running")
	}
	return cur.runner.logs.tail(lines), nil
}

// SpeedTest runs a speed test through a running Iran end.
func (m *Manager) SpeedTest(ctx context.Context, uuid string, mb int) (SpeedResult, error) {
	m.mu.Lock()
	cur := m.ends[uuid]
	m.mu.Unlock()
	if cur == nil {
		return SpeedResult{}, errors.New("the tunnel is not running")
	}
	if cur.end.Role != spec.RoleIran || cur.end.Reverse && cur.end.Carrier == "icmp" {
		return SpeedResult{}, errors.New("a speed test runs from the Iran end")
	}
	return speedTest(ctx, cur.metricsPort, mb), nil
}

// Status reports one end; ok is false when no process is managed for it.
func (m *Manager) Status(ctx context.Context, uuid string) (EndStatus, bool) {
	m.mu.Lock()
	cur := m.ends[uuid]
	m.mu.Unlock()
	if cur == nil {
		return EndStatus{}, false
	}
	return statusOf(ctx, cur), true
}

// Statuses reports every managed end, polling their metrics concurrently.
func (m *Manager) Statuses(ctx context.Context) []EndStatus {
	m.mu.Lock()
	all := make([]*managed, 0, len(m.ends))
	for _, cur := range m.ends {
		all = append(all, cur)
	}
	m.mu.Unlock()
	out := make([]EndStatus, len(all))
	var wg sync.WaitGroup
	for i, cur := range all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = statusOf(ctx, cur)
		}()
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].Uuid < out[j].Uuid })
	return out
}

func statusOf(ctx context.Context, cur *managed) EndStatus {
	info := cur.runner.info()
	st := EndStatus{Uuid: cur.end.Uuid, Clients: []ClientSession{}, Ports: []PortStat{}, Running: info.running, Pid: info.pid, Restarts: info.restarts, UptimeSec: int64(info.uptime.Seconds())}
	if info.running {
		if live, err := fetchStats(ctx, cur.metricsPort, cur.end.Role); err == nil {
			live.Uuid, live.Running, live.Pid, live.Restarts, live.UptimeSec = st.Uuid, true, st.Pid, st.Restarts, st.UptimeSec
			st = live
		}
	}
	// A server with one carrier does not label its clients; the carrier is the configured one.
	if st.Carrier == "" && cur.end.Carrier != spec.CarrierAuto {
		st.Carrier = cur.end.Carrier
	}
	switch {
	case !info.running:
		st.State = model.TunnelStateError
		st.LastError = cur.runner.lastError()
		if st.LastError == "" {
			st.LastError = "the process is not running"
		}
	case st.Alive:
		st.State = model.TunnelStateUp
	case info.uptime < startingGrace:
		st.State = model.TunnelStateStart
	default:
		st.State = model.TunnelStateDown
		st.LastError = cur.runner.lastError()
	}
	return st
}

// metricsPortOf is for tests.
func (m *Manager) metricsPortOf(uuid string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur := m.ends[uuid]; cur != nil {
		return cur.metricsPort
	}
	return 0
}
