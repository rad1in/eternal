package controller

import (
	_ "embed"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/tunnel"
)

//go:embed scripts/install-tunnel.sh
var installTunnelScript string

// panelURLPattern is what may be written into the script as the panel address: a Host header
// is client controlled, and the script runs as root on someone else's server.
var panelURLPattern = regexp.MustCompile(`^https?://[A-Za-z0-9.\-]+(:[0-9]{1,5})?(/[A-Za-z0-9._~\-/]*)?$`)

// NewTunnelInstallController mounts the public one-line installer for tunnel ends on g:
// the script itself, and this panel's own wattun binary for servers that cannot reach GitHub.
func NewTunnelInstallController(g *gin.RouterGroup) {
	g.GET("/install-tunnel.sh", serveInstallTunnelScript)
	g.GET("/install-tunnel/wattun/:arch", serveWattunBinary)
}

func serveInstallTunnelScript(c *gin.Context) {
	origin := panelOrigin(c)
	if !panelURLPattern.MatchString(origin) {
		origin = ""
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", []byte(strings.Replace(installTunnelScript, "__PANEL_URL__", origin, 1)))
}

func serveWattunBinary(c *gin.Context) {
	if runtime.GOOS != "linux" || c.Param("arch") != runtime.GOARCH || !tunnel.BinaryAvailable() {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	f, err := os.Open(tunnel.BinaryPath())
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.DataFromReader(http.StatusOK, st.Size(), "application/octet-stream", f, nil)
}
