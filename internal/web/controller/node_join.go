package controller

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/nodeinstall"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

const (
	joinBodyLimit = 8 << 10
	joinLimit     = 20
	joinWindow    = 10 * time.Minute
)

// NodeJoinController is the one public door for adding a node: it takes no
// session, only a single-use token issued by an admin from the panel.
type NodeJoinController struct {
	joinService service.NodeJoinService
	limiter     *windowLimiter
}

// NewNodeJoinController mounts POST {basePath}/join/node and the installer script
// it is driven by, GET {basePath}/install-node.sh, on g.
func NewNodeJoinController(g *gin.RouterGroup) *NodeJoinController {
	a := &NodeJoinController{limiter: newWindowLimiter(joinLimit, joinWindow)}
	g.POST("/join/node", a.join)
	g.GET("/install-node.sh", a.installScript)
	return a
}

// installScript serves the node installer; it holds nothing secret, the token is the credential.
func (a *NodeJoinController) installScript(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", nodeinstall.Script(config.GetBaseVersion()))
}

func (a *NodeJoinController) join(c *gin.Context) {
	ip := getRemoteIp(c)
	if !a.limiter.allow(ip) {
		c.JSON(http.StatusTooManyRequests, entity.Msg{Msg: "too many attempts, try again later"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, joinBodyLimit)
	var req struct {
		Token string `json:"token"`
		service.NodeJoinRequest
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, entity.Msg{Msg: "invalid request"})
		return
	}
	if strings.TrimSpace(req.Address) == "" {
		req.Address = ip
	}
	view, err := a.joinService.Join(req.Token, req.NodeJoinRequest)
	if err != nil {
		c.JSON(http.StatusOK, entity.Msg{Msg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, entity.Msg{Success: true, Obj: view})
}

// joinCommand issues a join token and the command that installs a node with it.
func (a *NodeController) joinCommand(c *gin.Context) {
	var req struct {
		Name      string `json:"name"`
		MasterURL string `json:"masterUrl"`
	}
	_ = c.ShouldBindJSON(&req)
	master := strings.TrimSpace(req.MasterURL)
	if master == "" {
		master = requestOrigin(c) + c.GetString("base_path")
	}
	cmd, err := a.joinService.IssueCommand(master, strings.TrimSpace(req.Name), a.masterPin(c))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.add"), err)
		return
	}
	jsonObj(c, cmd, nil)
}

// requestOrigin is scheme://host as the admin's browser reached the panel.
func requestOrigin(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

// masterPin is the pin curl needs to trust this panel, or "" when its certificate
// is not self-signed (a CA-signed or proxied panel needs none).
func (a *NodeController) masterPin(c *gin.Context) string {
	if c.Request.TLS == nil {
		return ""
	}
	certFile, _ := a.settingService.GetCertFile()
	return masterPin(certFile)
}

func masterPin(certFile string) string {
	if certFile == "" {
		return ""
	}
	raw, err := os.ReadFile(certFile)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return ""
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil || leaf.CheckSignatureFrom(leaf) != nil {
		return ""
	}
	sum := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	return "sha256//" + base64.StdEncoding.EncodeToString(sum[:])
}
