package stun_api

import (
	"linkstar/modules/stun"
	"linkstar/modules/stun/model"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type NatTypeViewResponse struct {
	UDP   *model.NatDetectResult `json:"udp"`
	TCP   *model.NatDetectResult `json:"tcp"`
	Error string                 `json:"error,omitempty"`
}

// GetNatTypeView 返回最近一次 UDP/TCP NAT 类型检测结果。
// 结果为 nil 表示启动时的异步检测尚未完成。
func (StunApi) GetNatTypeView(c *gin.Context) {
	udp, tcp := stun.GetNatDetectionResults()
	res.OkWithData(NatTypeViewResponse{
		UDP: udp,
		TCP: tcp,
	}, c)
}

// DetectNatTypeView 重新执行一轮 UDP/TCP NAT 类型检测并返回最新结果。
func (StunApi) DetectNatTypeView(c *gin.Context) {
	udp, tcp, err := stun.DetectAndClassifyNat()
	data := NatTypeViewResponse{UDP: udp, TCP: tcp}
	if err != nil {
		data.Error = err.Error()
	}
	res.OkWithData(data, c)
}
