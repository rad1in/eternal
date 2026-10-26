package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type joinFixture struct {
	svc   *NodeJoinService
	now   time.Time
	probe func(*NodeMutationRequest) error
}

func newJoinFixture(t *testing.T) *joinFixture {
	t.Helper()
	setupBulkDB(t)
	f := &joinFixture{now: time.UnixMilli(1760000000000)}
	f.svc = &NodeJoinService{
		now: func() time.Time { return f.now },
		reachable: func(_ context.Context, r *NodeMutationRequest) error {
			if f.probe != nil {
				return f.probe(r)
			}
			return nil
		},
	}
	return f
}

func joinReq() NodeJoinRequest {
	return NodeJoinRequest{Name: "de-fra-1", Scheme: "https", Address: "203.0.113.9", Port: 2053, BasePath: "/abc/", ApiToken: "node-token"}
}

func nodeCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	database.GetDB().Model(&model.Node{}).Count(&n)
	return n
}

func TestJoinCreatesTheNodeOnceAndSpendsTheToken(t *testing.T) {
	f := newJoinFixture(t)
	token, _, err := f.svc.Issue(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.svc.Join(token, joinReq())
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if view.Name != "de-fra-1" || view.Address != "203.0.113.9" || view.Port != 2053 || !view.Enable {
		t.Fatalf("node = %+v", view)
	}
	var stored model.Node
	database.GetDB().First(&stored, view.Id)
	if stored.TlsVerifyMode != "verify" && stored.TlsVerifyMode != "skip" {
		t.Fatalf("tls mode = %q", stored.TlsVerifyMode)
	}
	if _, err := f.svc.Join(token, NodeJoinRequest{Name: "second", Scheme: "https", Address: "203.0.113.10", Port: 2053, ApiToken: "t"}); err == nil {
		t.Fatal("a spent token joined a second node")
	}
	if nodeCount(t) != 1 {
		t.Fatalf("nodes = %d, want 1", nodeCount(t))
	}
}

func TestJoinRejectsUnknownAndExpiredTokens(t *testing.T) {
	f := newJoinFixture(t)
	if _, err := f.svc.Join("not-a-token", joinReq()); err == nil {
		t.Fatal("unknown token accepted")
	}
	token, _, _ := f.svc.Issue(time.Minute)
	f.now = f.now.Add(2 * time.Minute)
	if _, err := f.svc.Join(token, joinReq()); err == nil {
		t.Fatal("expired token accepted")
	}
	if nodeCount(t) != 0 {
		t.Fatal("a node was created by a rejected token")
	}
}

func TestJoinKeepsTheTokenWhenTheNodeCannotBeAdded(t *testing.T) {
	f := newJoinFixture(t)
	token, _, _ := f.svc.Issue(time.Hour)

	f.probe = func(*NodeMutationRequest) error { return errors.New("connection refused") }
	if _, err := f.svc.Join(token, joinReq()); err == nil {
		t.Fatal("an unreachable node joined")
	}
	if nodeCount(t) != 0 {
		t.Fatal("an unreachable node was stored")
	}

	f.probe = nil
	if _, err := f.svc.Join(token, joinReq()); err != nil {
		t.Fatalf("retry with the same token after the node came up: %v", err)
	}
}

func TestJoinCannotPickPrivilegedNodeSettings(t *testing.T) {
	f := newJoinFixture(t)
	token, _, _ := f.svc.Issue(time.Hour)
	req := joinReq()
	req.TlsVerifyMode = "mtls"
	if _, err := f.svc.Join(token, req); err == nil {
		t.Fatal("a joining node chose mtls")
	}

	var seen *NodeMutationRequest
	f.probe = func(r *NodeMutationRequest) error { seen = r; return nil }
	req.TlsVerifyMode = "skip"
	if _, err := f.svc.Join(token, req); err != nil {
		t.Fatalf("Join: %v", err)
	}
	if seen == nil || !seen.Enable || seen.OutboundTag != "" || seen.AllowPrivateAddress || seen.InboundSyncMode != "all" || seen.TlsVerifyMode != "skip" {
		t.Fatalf("node request = %+v, want enabled, direct, public-only, sync all", seen)
	}
}

func TestIssueStoresOnlyAHashAndPrunesExpiredTokens(t *testing.T) {
	f := newJoinFixture(t)
	token, expires, err := f.svc.Issue(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 32 || expires != f.now.Add(time.Minute).UnixMilli() {
		t.Fatalf("token %q expires %d", token, expires)
	}
	raw, _ := f.svc.settings.getString("nodeJoinTokens")
	if strings.Contains(raw, token) {
		t.Fatal("the plaintext token is stored")
	}
	f.now = f.now.Add(time.Hour)
	if _, _, err := f.svc.Issue(time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Join(token, joinReq()); err == nil {
		t.Fatal("the expired token still joins")
	}
	raw, _ = f.svc.settings.getString("nodeJoinTokens")
	if strings.Count(raw, `"expires"`) != 1 {
		t.Fatalf("expired token not pruned: %s", raw)
	}
}

func TestJoinCommandRunsTheScriptTheMasterServes(t *testing.T) {
	cmd := JoinCommand("https://panel.example.com:2053/abc/", "tok123", "de-fra-1", "")
	want := "curl -fsSL https://panel.example.com:2053/abc/install-node.sh | bash -s -- --panel https://panel.example.com:2053/abc --token tok123 --name 'de-fra-1'"
	if cmd != want {
		t.Fatalf("command =\n%s\nwant\n%s", cmd, want)
	}
}

func TestJoinCommandPinsAMasterWithASelfSignedCertificate(t *testing.T) {
	cmd := JoinCommand("https://203.0.113.5:2053/abc", "tok", "", "sha256//KEY=")
	want := "curl -fsSLk --pinnedpubkey 'sha256//KEY=' https://203.0.113.5:2053/abc/install-node.sh | bash -s -- --panel https://203.0.113.5:2053/abc --token tok --pin 'sha256//KEY='"
	if cmd != want {
		t.Fatalf("command =\n%s\nwant\n%s", cmd, want)
	}
}

func TestJoinCommandQuotesANameWithAQuote(t *testing.T) {
	quoted := JoinCommand("https://m.example/x/", "t", "it's", "")
	if !strings.Contains(quoted, `--name 'it'\''s'`) {
		t.Errorf("name not shell-quoted:\n%s", quoted)
	}
}
