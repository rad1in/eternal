package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
)

const metricsTimeout = 2 * time.Second

// rawStats is the part of the daemon's /stats JSON the panel reads.
type rawStats struct {
	Alive     bool               `json:"alive"`
	Srtt      float64            `json:"srtt_ms"`
	LastRxAge int64              `json:"last_rx_age_ms"`
	Sessions  int                `json:"sessions"`
	Streams   int                `json:"streams"`
	BytesIn   uint64             `json:"bytes_in_total"`
	BytesOut  uint64             `json:"bytes_out_total"`
	RateBps   float64            `json:"rate_bps"`
	Version   string             `json:"version"`
	TlsMode   string             `json:"tls_mode"`
	Clients   []rawClient        `json:"clients"`
	Ports     map[string]rawPort `json:"ports"`
}

type rawClient struct {
	IP      string `json:"ip"`
	Carrier string `json:"carrier"`
	AgeMs   int64  `json:"age_ms"`
}

type rawPort struct {
	BytesUp       int64 `json:"bytes_up_total"`
	BytesDown     int64 `json:"bytes_down_total"`
	ConnsActive   int64 `json:"conns_active"`
	ConnsTotal    int64 `json:"conns_all_time"`
	RateUp        int64 `json:"rate_bps_up"`
	RateDown      int64 `json:"rate_bps_down"`
	QuotaUsed     int64 `json:"quota_used"`
	QuotaLimit    int64 `json:"quota_limit"`
	QuotaExceeded bool  `json:"quota_exceeded"`
}

func metricsURL(port int, path string) string {
	return fmt.Sprintf("http://%s:%d%s", config.LoopbackIP(), port, path)
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
		return fmt.Errorf("metrics: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// fetchStats reads and converts /stats of one running end.
func fetchStats(ctx context.Context, port int, role string) (EndStatus, error) {
	var raw rawStats
	if err := getJSON(ctx, metricsURL(port, "/stats"), metricsTimeout, &raw); err != nil {
		return EndStatus{}, err
	}
	st := EndStatus{
		Alive: raw.Alive, SrttMs: raw.Srtt, LastRxAgeMs: raw.LastRxAge, Sessions: raw.Sessions,
		Streams: raw.Streams, BytesIn: raw.BytesIn, BytesOut: raw.BytesOut, RateBps: raw.RateBps,
		Version: raw.Version, TlsMode: raw.TlsMode,
	}
	st.Clients, st.Ports = []ClientSession{}, []PortStat{}
	for _, c := range raw.Clients {
		st.Clients = append(st.Clients, ClientSession{IP: c.IP, Carrier: c.Carrier, AgeMs: c.AgeMs})
	}
	if role == spec.RoleKharej && len(st.Clients) > 0 {
		st.Carrier = st.Clients[0].Carrier
	}
	for k, p := range raw.Ports {
		n, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		st.Ports = append(st.Ports, PortStat{
			Port: n, BytesUp: p.BytesUp, BytesDown: p.BytesDown, ConnsActive: p.ConnsActive, ConnsTotal: p.ConnsTotal,
			RateUp: p.RateUp, RateDown: p.RateDown, QuotaUsed: p.QuotaUsed, QuotaLimit: p.QuotaLimit, QuotaExceeded: p.QuotaExceeded,
		})
	}
	sort.Slice(st.Ports, func(i, j int) bool { return st.Ports[i].Port < st.Ports[j].Port })
	return st, nil
}

// speedTest moves mb megabytes each way through a running client end.
func speedTest(ctx context.Context, port, mb int) SpeedResult {
	limit := time.Duration(max(20, mb*2)) * time.Second
	var out SpeedResult
	var r struct {
		OK    bool    `json:"ok"`
		MBps  float64 `json:"mb_per_s"`
		Error string  `json:"error"`
	}
	for _, dir := range []string{"down", "up"} {
		r.OK, r.MBps, r.Error = false, 0, ""
		url := metricsURL(port, fmt.Sprintf("/speedtest?dir=%s&mb=%d", dir, mb))
		if err := getJSON(ctx, url, limit, &r); err != nil {
			out.Error = err.Error()
			return out
		}
		if !r.OK {
			out.Error = r.Error
			if out.Error == "" {
				out.Error = "the speed test failed"
			}
			return out
		}
		if dir == "down" {
			out.DownMBps = r.MBps
		} else {
			out.UpMBps = r.MBps
		}
	}
	return out
}
