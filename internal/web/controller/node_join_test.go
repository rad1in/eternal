package controller

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
)

func joinRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	r := gin.New()
	NewNodeJoinController(r.Group("/base"))
	return r
}

func postJoin(r *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/base/join/node", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.7:4000"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestJoinEndpointRefusesAnUnknownToken(t *testing.T) {
	r := joinRouter(t)
	w := postJoin(r, `{"token":"nope","name":"n1","address":"203.0.113.9","port":2053,"apiToken":"x"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"success":false`) || !strings.Contains(w.Body.String(), "invalid or expired join token") {
		t.Fatalf("response = %d %s", w.Code, w.Body.String())
	}
}

func TestJoinEndpointRejectsMalformedAndOversizedBodies(t *testing.T) {
	r := joinRouter(t)
	if w := postJoin(r, `not json`); w.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: %d", w.Code)
	}
	huge := `{"token":"` + strings.Repeat("a", 20<<10) + `"}`
	if w := postJoin(r, huge); w.Code != http.StatusBadRequest {
		t.Fatalf("oversized body: %d, want 400", w.Code)
	}
}

func TestJoinEndpointThrottlesGuessing(t *testing.T) {
	r := joinRouter(t)
	var last int
	for range joinLimit + 3 {
		last = postJoin(r, `{"token":"guess"}`).Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("attempt %d answered %d, want 429", joinLimit+3, last)
	}
}

func TestInstallScriptIsServedWithThisPanelsVersionBakedIn(t *testing.T) {
	r := joinRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/base/install-node.sh", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	want := `DEFAULT_VERSION="v` + config.GetBaseVersion() + `"`
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Body.String(), "#!/usr/bin/env bash") || !strings.Contains(w.Body.String(), want) {
		t.Fatalf("response = %d, want a script containing %s:\n%.300s", w.Code, want, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/x-shellscript") {
		t.Errorf("content type = %q", ct)
	}
}

func writeCert(t *testing.T, parent *x509.Certificate, parentKey *ecdsa.PrivateKey, cn string) (*x509.Certificate, *ecdsa.PrivateKey, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
	}
	signer, signerKey := tmpl, key
	if parent != nil {
		signer, signerKey = parent, parentKey
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	path := filepath.Join(t.TempDir(), cn+".pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return cert, key, path
}

func TestMasterPinOnlyForASelfSignedPanelCertificate(t *testing.T) {
	self, _, selfPath := writeCert(t, nil, nil, "self")
	ca, caKey, _ := writeCert(t, nil, nil, "ca")
	_, _, leafPath := writeCert(t, ca, caKey, "leaf")

	sum := sha256.Sum256(self.RawSubjectPublicKeyInfo)
	if got, want := masterPin(selfPath), "sha256//"+base64.StdEncoding.EncodeToString(sum[:]); got != want {
		t.Errorf("self-signed pin = %q, want %q", got, want)
	}
	if got := masterPin(leafPath); got != "" {
		t.Errorf("a CA-signed certificate must not be pinned, got %q", got)
	}
	if got := masterPin(""); got != "" {
		t.Errorf("no certificate must not be pinned, got %q", got)
	}
}
