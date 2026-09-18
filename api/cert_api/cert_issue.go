package cert_api

import (
	"linkstar/middleware"
	"linkstar/modules/cert"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type CertIssueRequest struct {
	ID uint `json:"id" binding:"required"`
}

// CertIssueView 立即签发/续期。
// ACME 全流程（下发 TXT → 等 DNS 传播 → CA 验证）可能要几分钟，
// 所以这里只负责启动，结果由前端轮询列表里的 notAfter / lastError 获知。
func (CertApi) CertIssueView(c *gin.Context) {
	cr := middleware.GetBindRequest[CertIssueRequest](c)

	if err := cert.IssueNow(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已开始签发，请稍后刷新查看结果", c)
}
