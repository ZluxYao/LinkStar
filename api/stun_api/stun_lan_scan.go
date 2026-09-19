package stun_api

import (
	"linkstar/middleware"
	"linkstar/modules/stun"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// StunLanSubnetListView 本机接在哪几个局域网上，给扫描前的下拉框用。
//
// 只读本机网卡，不发一个包，所以做成 GET、随时能调。
func (StunApi) StunLanSubnetListView(c *gin.Context) {
	res.OkWithData(stun.ListLanSubnets(), c)
}

type StunLanScanRequest struct {
	// CIDR 要扫的网段，如 192.168.100.0/24；留空用本机所在的那一段
	CIDR string `json:"cidr"`
}

// StunLanScanView 扫一段局域网，把在线的机器和它们开着的端口找出来。
//
// 会往两百多个地址发 TCP 连接，一两秒才回得来，所以走 POST 而不是 GET：
// GET 容易被浏览器和中间层预取、重试，一次点击变成好几轮扫描。
//
// 用请求自己的 ctx：用户关掉页面或者点了取消，扫描当场停，
// 不然一个已经没人看的请求还在那儿占着连接数往路由器灌包。
func (StunApi) StunLanScanView(c *gin.Context) {
	cr := middleware.GetBindRequest[StunLanScanRequest](c)

	hosts, err := stun.ScanLan(c.Request.Context(), cr.CIDR)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(hosts, c)
}
