package stun_api

import (
	"fmt"
	"linkstar/modules/stun"
	"linkstar/modules/stun/model"
	"linkstar/utils/res"
	"time"

	"github.com/gin-gonic/gin"
)

type StunConfigViewResponse struct {
	BestSTUN       string                `json:"bestStun"`
	CreatedAt      time.Time             `json:"createdAt"`
	UpdatedAt      time.Time             `json:"updatedAt"`
	Devices        []StunDeviceView      `json:"devices"`
	StunServerList []string              `json:"stunServerList"`
	LocalIP        string                `json:"localIP"`
	PublicIP       string                `json:"publicIP"`
	NatRouterList  []model.NatRouterInfo `json:"natRouterList"`
	Network        model.NetworkState    `json:"network"`
}

type StunDeviceView struct {
	model.Device
	Services []StunServiceView `json:"services"`
}

type StunServiceView struct {
	model.Service
	Phase        stun.ServicePhase `json:"phase"`
	PhaseStr     string            `json:"phaseStr"`
	RestartCount int               `json:"restartCount"`
	Logs         []stun.ServiceLog `json:"logs"`
	RuntimeAt    time.Time         `json:"runtimeAt"`
}

// 获取全部的stun配置文件信息
func (StunApi) GetStunConfigView(c *gin.Context) {
	config := stun.Runtime.Config
	data := StunConfigViewResponse{
		BestSTUN:       config.BestSTUN,
		CreatedAt:      config.CreatedAt,
		UpdatedAt:      config.UpdatedAt,
		Devices:        buildStunDeviceViews(config.Devices),
		StunServerList: config.StunServerList,
		LocalIP:        stun.Runtime.Network.LocalIP,
		PublicIP:       stun.Runtime.Network.PublicIP,
		NatRouterList:  stun.Runtime.Network.NatRouterList,
		Network:        stun.Runtime.Network,
	}

	if stun.Runtime.STUNService != nil && stun.Runtime.STUNService.BestSTUNServer != "" {
		data.BestSTUN = stun.Runtime.STUNService.BestSTUNServer
	}

	res.OkWithData(data, c)
}

func buildStunDeviceViews(devices []model.Device) []StunDeviceView {
	snapshotByKey := make(map[string]stun.StateEvent)
	if stun.Runtime.Scheduler != nil {
		for _, event := range stun.Runtime.Scheduler.Snapshot() {
			snapshotByKey[event.Key] = event
		}
	}

	views := make([]StunDeviceView, 0, len(devices))
	for _, device := range devices {
		deviceView := StunDeviceView{Device: device}
		deviceView.Services = make([]StunServiceView, 0, len(device.Services))
		for _, service := range device.Services {
			serviceView := StunServiceView{Service: service}
			if event, ok := snapshotByKey[fmt.Sprintf("%d-%d", device.DeviceID, service.ID)]; ok {
				serviceView.Phase = event.Phase
				serviceView.PhaseStr = event.PhaseStr
				serviceView.RestartCount = event.RestartCount
				serviceView.Logs = event.Logs
				serviceView.RuntimeAt = event.UpdatedAt
			}
			deviceView.Services = append(deviceView.Services, serviceView)
		}
		views = append(views, deviceView)
	}

	return views
}
