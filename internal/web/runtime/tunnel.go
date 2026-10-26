package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/tunnel"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/probe"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

// slowTunnelCall bounds the calls that move real traffic (a carrier probe tries up to nine
// carriers one after another, a speed test pushes megabytes), far above remoteHTTPTimeout.
const slowTunnelCall = 3 * time.Minute

type callTimeoutKey struct{}

func withCallTimeout(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, callTimeoutKey{}, d)
}

func callTimeout(ctx context.Context) time.Duration {
	if d, ok := ctx.Value(callTimeoutKey{}).(time.Duration); ok {
		return d
	}
	return remoteHTTPTimeout
}

// TunnelRuntime runs WatTunnel ends on a host: this panel (Local) or a node (Remote).
type TunnelRuntime interface {
	// ApplyTunnel starts or updates an end and returns it with every port the host settled on.
	ApplyTunnel(ctx context.Context, end tunnel.EndConfig) (tunnel.EndResult, error)
	RemoveTunnel(ctx context.Context, uuid string) error
	TunnelStatuses(ctx context.Context) ([]tunnel.EndStatus, error)
	TunnelLogs(ctx context.Context, uuid string, lines int) ([]string, error)
	RestartTunnel(ctx context.Context, uuid string) error
	// ProbeTunnel tries every carrier of an auto token from this host, best first.
	ProbeTunnel(ctx context.Context, token string) ([]probe.CarrierResult, error)
	TunnelSpeedTest(ctx context.Context, uuid string, mb int) (tunnel.SpeedResult, error)
}

// TunnelRuntimeFor returns the tunnel runtime of this panel (nil) or of a node.
func (m *Manager) TunnelRuntimeFor(nodeID *int) (TunnelRuntime, error) {
	rt, err := m.RuntimeFor(nodeID)
	if err != nil {
		return nil, err
	}
	tr, ok := rt.(TunnelRuntime)
	if !ok {
		return nil, errors.New("this runtime does not run tunnels")
	}
	return tr, nil
}

func (l *Local) ApplyTunnel(ctx context.Context, end tunnel.EndConfig) (tunnel.EndResult, error) {
	mgr := tunnel.GetManager()
	if err := mgr.Ensure(end); err != nil {
		return tunnel.EndResult{}, err
	}
	st, _ := mgr.Status(ctx, end.Uuid)
	st.Uuid = end.Uuid
	return tunnel.EndResult{End: end, Status: st}, nil
}

func (l *Local) RemoveTunnel(_ context.Context, uuid string) error {
	tunnel.GetManager().Remove(uuid)
	return nil
}

func (l *Local) TunnelStatuses(ctx context.Context) ([]tunnel.EndStatus, error) {
	return tunnel.GetManager().Statuses(ctx), nil
}

func (l *Local) TunnelLogs(_ context.Context, uuid string, lines int) ([]string, error) {
	return tunnel.GetManager().Logs(uuid, lines)
}

func (l *Local) RestartTunnel(_ context.Context, uuid string) error {
	return tunnel.GetManager().Restart(uuid)
}

func (l *Local) ProbeTunnel(ctx context.Context, token string) ([]probe.CarrierResult, error) {
	tok, err := spec.ParseToken(token)
	if err != nil {
		return nil, err
	}
	if !tunnel.BinaryAvailable() {
		return nil, fmt.Errorf("the wattun binary is not installed at %s", tunnel.BinaryPath())
	}
	return probe.Run(ctx, probe.Options{
		Binary: tunnel.BinaryPath(), Token: tok, WorkDir: filepath.Join(tunnel.StateDir(), "probe"), LinkWait: tunnel.ProbeLinkWait,
	})
}

func (l *Local) TunnelSpeedTest(ctx context.Context, uuid string, mb int) (tunnel.SpeedResult, error) {
	return tunnel.GetManager().SpeedTest(ctx, uuid, mb)
}

const tunnelAgentPath = "panel/api/tunnels/agent/"

func decodeObj(env *envelope, dst any) error {
	if len(env.Obj) == 0 {
		return nil
	}
	return json.Unmarshal(env.Obj, dst)
}

func (r *Remote) ApplyTunnel(ctx context.Context, end tunnel.EndConfig) (tunnel.EndResult, error) {
	env, err := r.do(ctx, http.MethodPost, tunnelAgentPath+"apply", end)
	if err != nil {
		return tunnel.EndResult{}, err
	}
	var res tunnel.EndResult
	return res, decodeObj(env, &res)
}

func (r *Remote) RemoveTunnel(ctx context.Context, uuid string) error {
	_, err := r.do(ctx, http.MethodPost, tunnelAgentPath+"remove/"+url.PathEscape(uuid), nil)
	return err
}

func (r *Remote) TunnelStatuses(ctx context.Context) ([]tunnel.EndStatus, error) {
	env, err := r.do(ctx, http.MethodGet, tunnelAgentPath+"status", nil)
	if err != nil {
		return nil, err
	}
	var out []tunnel.EndStatus
	return out, decodeObj(env, &out)
}

func (r *Remote) TunnelLogs(ctx context.Context, uuid string, lines int) ([]string, error) {
	env, err := r.do(ctx, http.MethodGet, tunnelAgentPath+"logs/"+url.PathEscape(uuid)+"?lines="+strconv.Itoa(lines), nil)
	if err != nil {
		return nil, err
	}
	var out []string
	return out, decodeObj(env, &out)
}

func (r *Remote) RestartTunnel(ctx context.Context, uuid string) error {
	_, err := r.do(ctx, http.MethodPost, tunnelAgentPath+"restart/"+url.PathEscape(uuid), nil)
	return err
}

func (r *Remote) ProbeTunnel(ctx context.Context, token string) ([]probe.CarrierResult, error) {
	env, err := r.do(withCallTimeout(ctx, slowTunnelCall), http.MethodPost, tunnelAgentPath+"probe", map[string]string{"token": token})
	if err != nil {
		return nil, err
	}
	var out []probe.CarrierResult
	return out, decodeObj(env, &out)
}

func (r *Remote) TunnelSpeedTest(ctx context.Context, uuid string, mb int) (tunnel.SpeedResult, error) {
	path := tunnelAgentPath + "speedtest/" + url.PathEscape(uuid) + "?mb=" + strconv.Itoa(mb)
	env, err := r.do(withCallTimeout(ctx, slowTunnelCall), http.MethodPost, path, nil)
	if err != nil {
		return tunnel.SpeedResult{}, err
	}
	var out tunnel.SpeedResult
	return out, decodeObj(env, &out)
}
