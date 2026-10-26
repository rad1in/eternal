package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/tunnel"
	"github.com/mhsanaei/3x-ui/v3/internal/wattunnel/spec"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// TunnelController serves the Tunnels page and the agent calls a master panel makes when it
// runs a tunnel end on this panel's host.
type TunnelController struct {
	tunnelService service.TunnelService
}

// NewTunnelController mounts the tunnel API on g (/panel/api/tunnels). publicIPv4 reports
// this server's public address.
func NewTunnelController(g *gin.RouterGroup, publicIPv4 func() string) *TunnelController {
	a := &TunnelController{}
	service.SetLocalPublicIPv4(publicIPv4)
	g.GET("/list", a.list)
	g.GET("/options", a.options)
	g.GET("/get/:pair", a.get)
	g.GET("/logs/:pair/:role", a.logs)
	g.GET("/token/:pair", a.token)
	g.POST("/create", a.create)
	g.POST("/update/:pair", a.update)
	g.POST("/del/:pair", a.del)
	g.POST("/restart/:pair", a.restart)
	g.POST("/reprobe/:pair", a.reprobe)
	g.POST("/speedtest/:pair", a.speedtest)

	agent := g.Group("/agent")
	agent.POST("/apply", a.agentApply)
	agent.POST("/remove/:uuid", a.agentRemove)
	agent.GET("/status", a.agentStatus)
	agent.GET("/logs/:uuid", a.agentLogs)
	agent.POST("/restart/:uuid", a.agentRestart)
	agent.POST("/probe", a.agentProbe)
	agent.POST("/speedtest/:uuid", a.agentSpeedtest)
	return a
}

// seesTunnelKeys is true for the owner and for API tokens, which act as the owner.
func seesTunnelKeys(c *gin.Context) bool {
	role := c.GetString(adminRoleKey)
	return role == "" || role == model.AdminRoleOwner
}

func panelOrigin(c *gin.Context) string {
	return strings.TrimRight(requestOrigin(c)+c.GetString("base_path"), "/")
}

func (a *TunnelController) list(c *gin.Context) {
	views, err := a.tunnelService.List()
	jsonObj(c, views, err)
}

func (a *TunnelController) options(c *gin.Context) {
	jsonObj(c, a.tunnelService.Capabilities(seesTunnelKeys(c)), nil)
}

func (a *TunnelController) get(c *gin.Context) {
	v, err := a.tunnelService.Get(c.Request.Context(), c.Param("pair"))
	jsonObj(c, v, err)
}

func (a *TunnelController) logs(c *gin.Context) {
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	out, err := a.tunnelService.Logs(c.Request.Context(), c.Param("pair"), c.Param("role"), lines)
	jsonObj(c, out, err)
}

func (a *TunnelController) token(c *gin.Context) {
	acc, err := a.tunnelService.Access(c.Param("pair"), panelOrigin(c))
	jsonObj(c, acc, err)
}

// tunnelCreated is the answer to a create: the link, plus the join command when the other
// end is a server without a panel and the caller may see keys.
type tunnelCreated struct {
	Tunnel *service.TunnelView   `json:"tunnel"`
	Access *service.TunnelAccess `json:"access"`
}

func (a *TunnelController) create(c *gin.Context) {
	var req service.TunnelCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	view, access, err := a.tunnelService.Create(c.Request.Context(), req, panelOrigin(c), seesTunnelKeys(c))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, tunnelCreated{Tunnel: view, Access: access}, nil)
}

func (a *TunnelController) update(c *gin.Context) {
	var req service.TunnelUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	v, err := a.tunnelService.Update(c.Request.Context(), c.Param("pair"), req)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, v, nil)
}

func (a *TunnelController) del(c *gin.Context) {
	var body struct {
		Force bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := a.tunnelService.Delete(c.Request.Context(), c.Param("pair"), body.Force); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *TunnelController) restart(c *gin.Context) {
	if err := a.tunnelService.Restart(c.Request.Context(), c.Param("pair")); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *TunnelController) reprobe(c *gin.Context) {
	v, err := a.tunnelService.Reprobe(c.Request.Context(), c.Param("pair"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, v, nil)
}

func (a *TunnelController) speedtest(c *gin.Context) {
	mb, _ := strconv.Atoi(c.DefaultQuery("mb", "10"))
	res, err := a.tunnelService.SpeedTest(c.Request.Context(), c.Param("pair"), mb)
	jsonObj(c, res, err)
}

func (a *TunnelController) agentApply(c *gin.Context) {
	var end tunnel.EndConfig
	if err := c.ShouldBindJSON(&end); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if end.Uuid == "" || (end.Role != spec.RoleKharej && end.Role != spec.RoleIran) {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("invalid tunnel end"))
		return
	}
	res, err := a.tunnelService.ApplyLocal(c.Request.Context(), end, nil)
	jsonObj(c, res, err)
}

func (a *TunnelController) agentRemove(c *gin.Context) {
	jsonObj(c, nil, a.tunnelService.RemoveLocal(c.Request.Context(), c.Param("uuid")))
}

func (a *TunnelController) agentStatus(c *gin.Context) {
	out, err := a.tunnelService.AgentStatuses(c.Request.Context())
	jsonObj(c, out, err)
}

func (a *TunnelController) agentLogs(c *gin.Context) {
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	out, err := a.tunnelService.AgentLogs(c.Request.Context(), c.Param("uuid"), lines)
	jsonObj(c, out, err)
}

func (a *TunnelController) agentRestart(c *gin.Context) {
	jsonObj(c, nil, a.tunnelService.AgentRestart(c.Request.Context(), c.Param("uuid")))
}

func (a *TunnelController) agentProbe(c *gin.Context) {
	var body struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	out, err := a.tunnelService.AgentProbe(c.Request.Context(), body.Token)
	jsonObj(c, out, err)
}

func (a *TunnelController) agentSpeedtest(c *gin.Context) {
	mb, _ := strconv.Atoi(c.DefaultQuery("mb", "10"))
	out, err := a.tunnelService.AgentSpeedTest(c.Request.Context(), c.Param("uuid"), mb)
	jsonObj(c, out, err)
}
