package stun_api

import (
	"linkstar/middleware"
	"linkstar/modules/stun"
	"linkstar/modules/stun/model"
	"linkstar/utils/res"
	"strings"

	"github.com/gin-gonic/gin"
)

// normalizeMCEntry MC 入口只对 TCP 有意义（Java 版走 TCP，基岩版不认 SRV）；
// 域名统一成小写、去掉首尾的点。接口是公开的，不能指望前端守规矩。
func normalizeMCEntry(protocol string, cfg model.MCEntryConfig) model.MCEntryConfig {
	if strings.EqualFold(strings.TrimSpace(protocol), "UDP") {
		cfg.Enabled = false
	}
	cfg.Host = strings.ToLower(strings.Trim(strings.TrimSpace(cfg.Host), "."))
	cfg.ZoneDomain = strings.ToLower(strings.Trim(strings.TrimSpace(cfg.ZoneDomain), "."))
	cfg.Target = strings.ToLower(strings.Trim(strings.TrimSpace(cfg.Target), "."))
	if cfg.Target == cfg.Host {
		cfg.Target = "" // 指向自己就是「新建」那条路，存成空省得两种写法同一个意思
	}
	return cfg
}

// StunMCEntrySyncView 立即把服务当前的外部端口写进 SRV。平时洞一通就会自动同步。
func (StunApi) StunMCEntrySyncView(c *gin.Context) {
	cr := middleware.GetBindRequest[StunRedirectRequest](c)
	msg, err := stun.SyncMCEntryNow(cr.DeviceID, cr.ServiceID)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg(msg, c)
}
