package job

import (
	"context"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// tunnelTickBudget keeps one slow node from holding the job past the next tick.
const tunnelTickBudget = 45 * time.Second

// TunnelJob keeps the tunnel ends of this host running, refreshes the state of every end and
// publishes tunnel.down / tunnel.up when a link lost or regained its connection.
type TunnelJob struct {
	tunnels service.TunnelService
}

func NewTunnelJob() *TunnelJob { return &TunnelJob{} }

func (j *TunnelJob) Run() {
	ctx, cancel := context.WithTimeout(context.Background(), tunnelTickBudget)
	defer cancel()
	events := j.tunnels.Tick(ctx)
	if EventBus == nil {
		return
	}
	for _, e := range events {
		EventBus.Publish(e)
	}
}
