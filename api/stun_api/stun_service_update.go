package stun_api

import (
	"linkstar/middleware"
	"linkstar/modules/stun"
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
	TLS          bool   `json:"tls"`          // 证书

	// UPnP 相关配置
	UseUPnP        bool   `json:"useUpnp"`
	UPnPMappedPort uint16 `json:"upnpMappedPort"`

	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
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
	svc.TLS = cr.TLS
	svc.UseUPnP = cr.UseUPnP
	svc.UPnPMappedPort = cr.UPnPMappedPort
	svc.Enabled = cr.Enabled
	svc.Description = cr.Description
	svc.UpdatedAt = time.Now()

	// 持久化配置到文件
	if err := stun.UpdateConfig(stun.Runtime.Config); err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}

	// 重启该服务的 STUN 穿透（停旧起新）
	// 修复：原版调用已删除的全局函数 stun.StartService，改为调度器实例方法
	device := &stun.Runtime.Config.Devices[deviceIndex]
	stun.Runtime.Scheduler.StartService(device, &device.Services[serviceIndex])

	res.OkWithData(*svc, c)
}
