package tunnel

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

const (
	logRingLines    = 500
	maxBackoff      = 30 * time.Second
	healthyAfter    = 60 * time.Second
	gracefulWait    = 5 * time.Second
	forcedWait      = 2 * time.Second
	statsLinePrefix = "stats:"
)

// ring keeps the last lines a child printed. It is the process's stdout and stderr.
type ring struct {
	mu    sync.Mutex
	label string
	buf   string
	lines []string
}

func (r *ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf += string(p)
	for {
		i := strings.IndexByte(r.buf, '\n')
		if i < 0 {
			break
		}
		line := r.buf[:i]
		r.buf = r.buf[i+1:]
		r.addLocked(line)
	}
	return len(p), nil
}

func (r *ring) addLocked(line string) {
	line = strings.TrimSpace(strings.TrimRight(line, "\r"))
	if line == "" {
		return
	}
	r.lines = append(r.lines, line)
	if len(r.lines) > logRingLines {
		r.lines = r.lines[len(r.lines)-logRingLines:]
	}
	// The 10 s traffic line would drown the panel log; the ring keeps it for the log view.
	if !strings.HasPrefix(line, statsLinePrefix) {
		logger.Infof("tunnel %s | %s", r.label, line)
	}
}

func (r *ring) flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.buf != "" {
		r.addLocked(r.buf)
		r.buf = ""
	}
}

func (r *ring) tail(n int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 || n > len(r.lines) {
		n = len(r.lines)
	}
	return append([]string(nil), r.lines[len(r.lines)-n:]...)
}

func (r *ring) last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.lines) - 1; i >= 0; i-- {
		if !strings.HasPrefix(r.lines[i], statsLinePrefix) {
			return r.lines[i]
		}
	}
	return ""
}

// runner keeps one wattun process alive: it starts it, waits, and starts it again with a
// growing delay until stop is called.
type runner struct {
	binary string
	args   []string
	logs   *ring

	mu        sync.Mutex
	cmd       *exec.Cmd
	startedAt time.Time
	restarts  int
	lastExit  string
	quit      chan struct{}
	finished  chan struct{}
}

func newRunner(label, binary string, args []string) *runner {
	return &runner{
		binary: binary, args: args, logs: &ring{label: label},
		quit: make(chan struct{}), finished: make(chan struct{}),
	}
}

func (r *runner) begin() { go r.loop() }

func (r *runner) loop() {
	defer close(r.finished)
	backoff := time.Second
	for {
		cmd := exec.Command(r.binary, r.args...) //nolint:gosec // the panel's own wattun binary and generated args
		cmd.Stdout, cmd.Stderr = r.logs, r.logs
		started := time.Now()
		err := cmd.Start()
		if err == nil {
			attachChildLifetime(cmd)
			r.mu.Lock()
			r.cmd, r.startedAt = cmd, started
			r.mu.Unlock()
			err = cmd.Wait()
			r.logs.flush()
		}
		r.mu.Lock()
		r.cmd = nil
		if err != nil {
			r.lastExit = err.Error()
		} else {
			r.lastExit = "exited"
		}
		r.mu.Unlock()
		select {
		case <-r.quit:
			return
		default:
		}
		if time.Since(started) > healthyAfter {
			backoff = time.Second
		}
		select {
		case <-r.quit:
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
		r.mu.Lock()
		r.restarts++
		r.mu.Unlock()
	}
}

type runnerInfo struct {
	running  bool
	pid      int
	uptime   time.Duration
	restarts int
	lastExit string
}

func (r *runner) info() runnerInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	in := runnerInfo{restarts: r.restarts, lastExit: r.lastExit}
	if r.cmd != nil && r.cmd.Process != nil {
		in.running, in.pid, in.uptime = true, r.cmd.Process.Pid, time.Since(r.startedAt)
	}
	return in
}

// lastError is the most useful single line about why the process is not healthy.
func (r *runner) lastError() string {
	if line := r.logs.last(); line != "" {
		return line
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastExit
}

// stop ends the loop and the process, escalating from SIGTERM to kill.
func (r *runner) stop() {
	select {
	case <-r.quit:
	default:
		close(r.quit)
	}
	r.mu.Lock()
	cmd := r.cmd
	r.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		if runtime.GOOS == "windows" {
			_ = cmd.Process.Kill()
		} else if err := cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			_ = cmd.Process.Kill()
		}
	}
	select {
	case <-r.finished:
		return
	case <-time.After(gracefulWait):
	}
	logger.Warningf("tunnel %s: did not stop after SIGTERM, killing it", r.logs.label)
	r.mu.Lock()
	cmd = r.cmd
	r.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	select {
	case <-r.finished:
	case <-time.After(forcedWait):
	}
}
