package stun_api

import (
	"linkstar/modules/stun"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// GetObservabilityView 返回当前网络、NAT 和 STUN 转发流量的运行时快照。
func (StunApi) GetObservabilityView(c *gin.Context) {
	res.OkWithData(stun.GetObservabilitySnapshot(), c)
}
