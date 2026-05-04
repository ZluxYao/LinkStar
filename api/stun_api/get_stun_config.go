package stun_api

import (
	"linkstar/modules/stun"
	"linkstar/modules/stun/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type StunConfigViewResponse struct {
	model.Config
	LocalIP       string                `json:"localIP"`
	PublicIP      string                `json:"publicIP"`
	NatRouterList []model.NatRouterInfo `json:"natRouterList"`
	Network       model.NetworkState    `json:"network"`
}

// 获取全部的stun配置文件信息
func (StunApi) GetStunConfigView(c *gin.Context) {
	data := StunConfigViewResponse{
		Config:        stun.Runtime.Config,
		LocalIP:       stun.Runtime.Network.LocalIP,
		PublicIP:      stun.Runtime.Network.PublicIP,
		NatRouterList: stun.Runtime.Network.NatRouterList,
		Network:       stun.Runtime.Network,
	}

	if stun.Runtime.STUNService != nil && stun.Runtime.STUNService.BestSTUNServer != "" {
		data.BestSTUN = stun.Runtime.STUNService.BestSTUNServer
	}

	res.OkWithData(data, c)
}
