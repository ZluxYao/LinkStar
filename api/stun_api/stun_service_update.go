package stun_api

import (
	"linkstar/middleware"
	"linkstar/modules/stun"
	"linkstar/modules/stun/model"
	"linkstar/modules/webhook"
	"linkstar/utils/res"
	"time"

	"github.com/gin-gonic/gin"
)

type StunServiceUpdateViewRequest struct {
	DeviceID     uint   `json:"deviceId"`     // 设备ID
	ServiceID    uint   `json:"serviceId"`    // 服务ID
	Name         string `json:"name"`         // 服务名称
	InternalPort uint16 `json:"internalPort"` // 内网端口
	Protocol     string `json:"protocol"`     // 传输协议 "TCP"/"UDP"
	Https        bool   `json:"https"`        // 是否是 https 服务（影响前端跳转 scheme）

	// UPnP 相关配置
	UseUPnP bool `json:"useUpnp"`

	// 对外访问
	Domain       string `json:"domain"`       // 对外域名，如 fw.example.com；留空回落公网 IP
	TLSTerminate bool   `json:"tlsTerminate"` // 由 LinkStar 在洞口终结 TLS（仅 TCP）
	CertID       uint   `json:"certId"`       // 绑定证书 ID，0 表示按 SNI 自动匹配
	BackendHTTPS bool   `json:"backendHttps"` // 转发给内网时也用 HTTPS（tls.Dial），仅在 TLSTerminate 时成立

	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`

	WebHookConfig webhook.WebhookConfig `json:"webhookconfig"` // Webhook 配置文件
	Redirect      model.RedirectConfig  `json:"redirect"`      // 入口域名跟着外部端口走
	MCEntry       model.MCEntryConfig   `json:"mcEntry"`       // MC Java 版联机的 SRV 记录
}

func (StunApi) StunServiceUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[StunServiceUpdateViewRequest](c)

	// 查找目标设备
	deviceIndex := -1
	for i, device := range stun.Runtime.Config.Devices {
		if device.DeviceID == cr.DeviceID {
			deviceIndex = i
			break
		}
	}
	if deviceIndex == -1 {
		res.FailWithMsg("设备不存在", c)
		return
	}

	// 查找目标服务
	serviceIndex := -1
	for i, svc := range stun.Runtime.Config.Devices[deviceIndex].Services {
		if svc.ID == cr.ServiceID {
			serviceIndex = i
			break
		}
	}
	if serviceIndex == -1 {
		res.FailWithMsg("服务不存在", c)
		return
	}

	// 更新服务字段
	svc := &stun.Runtime.Config.Devices[deviceIndex].Services[serviceIndex]
	svc.Name = cr.Name
	svc.InternalPort = cr.InternalPort
	svc.Protocol = cr.Protocol
	svc.Https = cr.Https
	svc.UseUPnP = cr.UseUPnP
	svc.UPnPMappedPort = 0 // 从来没人往里写真值，留着只会让首页显示一个错的端口（见 stun.LiveExternalPort）
	svc.Domain, svc.TLSTerminate, svc.CertID = normalizeTLSFields(cr.Protocol, cr.Domain, cr.TLSTerminate, cr.CertID)
	svc.Domain = fillDomainFromCert(svc.Domain, svc.TLSTerminate, svc.CertID)
	svc.BackendHTTPS = normalizeBackendHTTPS(cr.Protocol, cr.BackendHTTPS, svc.TLSTerminate)
	svc.Enabled = cr.Enabled
	svc.Description = cr.Description
	svc.WebHookConfig = cr.WebHookConfig
	svc.Redirect = cr.Redirect
	oldMCEntry := svc.MCEntry
	svc.MCEntry = normalizeMCEntry(cr.Protocol, cr.MCEntry)
	svc.UpdatedAt = time.Now()

	// 持久化配置到文件
	if err := stun.UpdateConfig(stun.Runtime.Config); err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}

	// MC 入口关了或换了域名：旧的那条 SRV 收回去，不然朋友会被送到一个没人维护的端口
	if stun.MCEntryChanged(oldMCEntry, svc.MCEntry) {
		stun.CleanupMCEntry(oldMCEntry)
	}
	// 原来是「新建」、LinkStar 替联机域名补过 A 记录，现在改成指向别的域名或关掉了：
	// 那条自动建的收回去（还有别的服务用着就不动，CleanupLandingRecord 自己会看）
	if old := stun.MCEntryOwnedHost(oldMCEntry); old != "" && old != stun.MCEntryOwnedHost(svc.MCEntry) {
		stun.CleanupLandingRecord(old)
	}

	// 重启该服务的 STUN 穿透（停旧起新）
	// 修复：原版调用已删除的全局函数 stun.StartService，改为调度器实例方法
	device := &stun.Runtime.Config.Devices[deviceIndex]
	stun.Runtime.Scheduler.StartService(device, &device.Services[serviceIndex])

	res.OkWithData(*svc, c)
}
