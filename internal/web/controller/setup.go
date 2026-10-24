package controller

import (
	"bytes"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
)

const (
	setupBodyLimit   = 4 << 10
	setupTokenHeader = "X-Setup-Token"
	setupFailLimit   = 20
	setupFailWindow  = 10 * time.Minute
)

// SetupController serves the first-run wizard. Everything under /setup answers
// like any unknown path unless the wizard is open and the caller holds its token.
type SetupController struct {
	setup    *panel.SetupService
	failures *windowLimiter
	notFound gin.HandlerFunc
}

// NewSetupController mounts the wizard page and its API on g; notFound answers
// strangers (the decoy site or a bare 404).
func NewSetupController(g *gin.RouterGroup, notFound gin.HandlerFunc) *SetupController {
	return newSetupController(g, notFound, &panel.SetupService{})
}

func newSetupController(g *gin.RouterGroup, notFound gin.HandlerFunc, setup *panel.SetupService) *SetupController {
	a := &SetupController{setup: setup, failures: newWindowLimiter(setupFailLimit, setupFailWindow), notFound: notFound}
	g.GET("", a.page)
	g.GET("/", a.page)
	api := g.Group("/api")
	api.Use(a.noStore, a.tokenGate)
	api.GET("/status", a.status)
	api.POST("/check-port", a.checkPort)
	api.POST("/finish", a.finish)
	return a
}

// SetupGate answers every route but the wizard's like an unknown path while the
// wizard is open, so a panel with its default login is never reachable.
func SetupGate(notFound gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if global.SetupPending() && path != "/setup" && !strings.HasPrefix(path, "/setup/") {
			notFound(c)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (a *SetupController) deny(c *gin.Context) {
	if a.notFound != nil {
		a.notFound(c)
	} else {
		c.Status(http.StatusNotFound)
	}
	c.Abort()
}

func (a *SetupController) noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Next()
}

// page serves the wizard bundle, but only to a visitor whose link carries the token.
func (a *SetupController) page(c *gin.Context) {
	if !a.setup.Pending() || !a.setup.VerifyToken(c.Query("t")) {
		a.deny(c)
		return
	}
	body, err := fs.ReadFile(distFS, "dist/setup.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "missing embedded page: setup.html")
		return
	}
	body = bytes.ReplaceAll(body, []byte(`="/assets/`), []byte(`="/setup/assets/`))
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Data(http.StatusOK, "text/html; charset=utf-8", body)
}

func setupFail(c *gin.Context, code string) {
	c.JSON(http.StatusOK, entity.Msg{Success: false, Msg: code})
}

// tokenGate lets a request through only with the wizard's token; wrong guesses are rate limited per address.
func (a *SetupController) tokenGate(c *gin.Context) {
	if !a.setup.Pending() {
		setupFail(c, "already_done")
		c.Abort()
		return
	}
	ip := getRemoteIp(c)
	if a.setup.VerifyToken(c.GetHeader(setupTokenHeader)) {
		c.Next()
		return
	}
	if !a.failures.allow(ip) {
		setupFail(c, "rate_limited")
		c.Abort()
		return
	}
	setupFail(c, "bad_token")
	c.Abort()
}

type setupStatus struct {
	Pending  bool   `json:"pending"`
	Port     int    `json:"suggestedPort"`
	BasePath string `json:"suggestedBasePath"`
	Host     string `json:"serverHost"`
	Version  string `json:"version"`
}

func (a *SetupController) status(c *gin.Context) {
	sug := a.setup.Suggest()
	host := c.Request.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	c.JSON(http.StatusOK, entity.Msg{Success: true, Obj: setupStatus{
		Pending: true, Port: sug.Port, BasePath: sug.BasePath,
		Host: strings.Trim(host, "[]"), Version: config.GetPanelVersion(),
	}})
}

func (a *SetupController) checkPort(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, setupBodyLimit)
	var in struct {
		Port int `json:"port"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		setupFail(c, "port_invalid")
		return
	}
	c.JSON(http.StatusOK, entity.Msg{Success: true, Obj: gin.H{"free": a.setup.PortFree(in.Port)}})
}

func (a *SetupController) finish(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, setupBodyLimit)
	var in panel.SetupInput
	if err := c.ShouldBindJSON(&in); err != nil {
		setupFail(c, "internal_error")
		return
	}
	res, err := a.setup.Finish(c.GetHeader(setupTokenHeader), in)
	if err != nil {
		var se *panel.SetupError
		if errors.As(err, &se) {
			setupFail(c, se.Code)
			return
		}
		logger.Warning("setup: finish failed:", err)
		setupFail(c, "internal_error")
		return
	}
	c.JSON(http.StatusOK, entity.Msg{Success: true, Obj: res})
}
