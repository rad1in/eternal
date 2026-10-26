package service

import (
	"context"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/tunnel"
)

const (
	// A change of link state must hold for this many ticks before it is announced.
	tunnelStateConfirmTicks = 2
	// A link that stays down this long gets its daemon restarted, at most once per healCooldown.
	tunnelHealAfter    = 5 * time.Minute
	tunnelHealCooldown = 10 * time.Minute
	tunnelPollTimeout  = 5 * time.Second
)

// Tick reconciles the ends of this host, refreshes the observed state of every end this
// panel knows and returns the events for links that went down or came back.
func (s *TunnelService) Tick(ctx context.Context) []eventbus.Event {
	var rows []*model.TunnelEnd
	if err := database.GetDB().Order("id").Find(&rows).Error; err != nil {
		logger.Warning("tunnel tick: load failed:", err)
		return nil
	}
	if len(rows) == 0 {
		return nil
	}
	s.reconcileLocal(ctx, rows)

	byHost := map[int][]*model.TunnelEnd{}
	for _, r := range rows {
		key := -1
		if r.NodeId != nil {
			key = *r.NodeId
		}
		byHost[key] = append(byHost[key], r)
	}
	for key, group := range byHost {
		var nodeID *int
		if key >= 0 {
			nodeID = &key
		}
		s.refreshHost(ctx, nodeID, group)
	}
	s.healStale(ctx, rows)
	return s.pairEvents(rows)
}

func (s *TunnelService) reconcileLocal(ctx context.Context, rows []*model.TunnelEnd) {
	local, err := tunnelRuntime(nil)
	if err != nil {
		return
	}
	for _, r := range rows {
		if r.NodeId != nil || !r.Enable {
			continue
		}
		e, err := s.endOf(r)
		if err != nil {
			logger.Warning("tunnel:", err)
			continue
		}
		if _, err := local.ApplyTunnel(ctx, e); err != nil {
			s.markRow(r, model.TunnelStateError, err.Error(), 0)
		}
	}
}

func (s *TunnelService) refreshHost(ctx context.Context, nodeID *int, group []*model.TunnelEnd) {
	remote, err := tunnelRuntime(nodeID)
	if err != nil {
		for _, r := range group {
			s.markRow(r, model.TunnelStateUnknown, err.Error(), 0)
		}
		return
	}
	cctx, cancel := context.WithTimeout(ctx, tunnelPollTimeout)
	defer cancel()
	statuses, err := remote.TunnelStatuses(cctx)
	if err != nil {
		for _, r := range group {
			s.markRow(r, model.TunnelStateUnknown, "the node did not answer: "+err.Error(), 0)
		}
		return
	}
	s.recordStatuses(group, statuses)
	if nodeID == nil {
		return
	}
	// A node that lost its database or was reinstalled forgets the ends it was given.
	known := map[string]bool{}
	for _, st := range statuses {
		known[st.Uuid] = true
	}
	for _, r := range group {
		if known[r.Uuid] || !r.Enable {
			continue
		}
		e, err := s.endOf(r)
		if err != nil {
			continue
		}
		if _, err := remote.ApplyTunnel(ctx, e); err != nil {
			s.markRow(r, model.TunnelStateError, err.Error(), 0)
		}
	}
}

func (s *TunnelService) recordStatuses(group []*model.TunnelEnd, statuses []tunnel.EndStatus) {
	byUuid := map[string]tunnel.EndStatus{}
	for _, st := range statuses {
		byUuid[st.Uuid] = st
	}
	for _, r := range group {
		if r == nil {
			continue
		}
		if !r.Enable {
			s.markRow(r, model.TunnelStateStopped, "", 0)
			continue
		}
		st, ok := byUuid[r.Uuid]
		if !ok {
			s.markRow(r, model.TunnelStateError, "the tunnel is not running on its host", 0)
			continue
		}
		s.markRow(r, st.State, st.LastError, st.SrttMs)
	}
}

// markRow stores an end's observed state, touching the database only when it changed.
func (s *TunnelService) markRow(r *model.TunnelEnd, state, lastError string, rtt float64) {
	lastError = truncate(lastError, 300)
	if state == model.TunnelStateUp {
		lastError = ""
	}
	if r.State == state && r.LastError == lastError && int(r.RttMs) == int(rtt) {
		return
	}
	changedAt := r.StateChangedAt
	if r.State != state || changedAt == 0 {
		changedAt = time.Now().UnixMilli()
	}
	r.State, r.LastError, r.RttMs, r.StateChangedAt = state, lastError, rtt, changedAt
	err := database.GetDB().Model(&model.TunnelEnd{}).Where("id = ?", r.Id).
		Updates(map[string]any{"state": state, "last_error": lastError, "rtt_ms": rtt, "state_changed_at": changedAt}).Error
	if err != nil {
		logger.Warning("tunnel: saving state failed:", err)
	}
}

// healStale restarts the daemon of a local end whose link stayed down for a while: a hung
// carrier connection is often cleared by a fresh start.
func (s *TunnelService) healStale(ctx context.Context, rows []*model.TunnelEnd) {
	local, err := tunnelRuntime(nil)
	if err != nil {
		return
	}
	now := time.Now()
	for _, r := range rows {
		if r.NodeId != nil || !r.Enable || r.State != model.TunnelStateDown {
			continue
		}
		if now.Sub(time.UnixMilli(r.StateChangedAt)) < tunnelHealAfter {
			continue
		}
		s.mu.Lock()
		if s.lastHeal == nil {
			s.lastHeal = map[string]time.Time{}
		}
		recent := now.Sub(s.lastHeal[r.Uuid]) < tunnelHealCooldown
		if !recent {
			s.lastHeal[r.Uuid] = now
		}
		s.mu.Unlock()
		if recent {
			continue
		}
		logger.Infof("tunnel: %q has been down for %s, restarting its %s end", r.Name, tunnelHealAfter, r.Role)
		if err := local.RestartTunnel(ctx, r.Uuid); err != nil {
			logger.Warning("tunnel: restart failed:", err)
		}
	}
}

// pairState is the state a link is judged by: its Iran end, or the only end there is.
func pairState(rows []*model.TunnelEnd) (state string, last *model.TunnelEnd) {
	k, i := pairEnds(rows)
	last = i
	if last == nil {
		last = k
	}
	s := last.State
	switch s {
	case model.TunnelStateUp:
		return "up", last
	case model.TunnelStateDown, model.TunnelStateError:
		return "down", last
	}
	return "", last
}

func (s *TunnelService) pairEvents(rows []*model.TunnelEnd) []eventbus.Event {
	groups := map[string][]*model.TunnelEnd{}
	for _, r := range rows {
		groups[r.PairUuid] = append(groups[r.PairUuid], r)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watch == nil {
		s.watch = map[string]*tunnelWatch{}
	}
	var events []eventbus.Event
	names := s.nodeNames()
	for pair, g := range groups {
		state, last := pairState(g)
		if state == "" {
			continue
		}
		w := s.watch[pair]
		if w == nil {
			s.watch[pair] = &tunnelWatch{published: state}
			continue
		}
		if state == w.published {
			w.candidate, w.count = "", 0
			continue
		}
		if state != w.candidate {
			w.candidate, w.count = state, 0
		}
		if w.count++; w.count < tunnelStateConfirmTicks {
			continue
		}
		data := &eventbus.TunnelEventData{PairUuid: pair, Name: last.Name, Carrier: last.Carrier}
		if last.NodeId != nil {
			data.Node = names[*last.NodeId]
		}
		now := time.Now()
		ev := eventbus.Event{Source: last.Name, Data: data, Timestamp: now}
		if state == "down" {
			ev.Type, data.Error, w.downAt = eventbus.EventTunnelDown, last.LastError, now
		} else {
			ev.Type = eventbus.EventTunnelUp
			if !w.downAt.IsZero() {
				data.DownSeconds = int64(now.Sub(w.downAt).Seconds())
			}
		}
		w.published, w.candidate, w.count = state, "", 0
		events = append(events, ev)
	}
	for pair := range s.watch {
		if _, ok := groups[pair]; !ok {
			delete(s.watch, pair)
		}
	}
	return events
}
