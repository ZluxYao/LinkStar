package auth_api

import (
	"errors"
	"linkstar/middleware"
	"linkstar/modules/auth"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type SetupRequest struct {
	Password   string `json:"password"`
	SetupToken string `json:"setupToken"`
}

// SetupView 公开但仅在未初始化时可用：设置 admin 密码，成功后返回 token 直接进后台。
// 本机可以直接设；从其他设备来必须带上启动日志里的口令，避免同网段抢先设密码。
func (AuthApi) SetupView(c *gin.Context) {
	cr := middleware.GetBindRequest[SetupRequest](c)
	if !auth.AllowsSetup(c.Request.RemoteAddr, cr.SetupToken) {
		res.FailWithMsg("从其他设备设置密码需要启动日志里的首次设置口令", c)
		return
	}
	if err := auth.Runtime.Setup(cr.Password); err != nil {
		if errors.Is(err, auth.ErrPasswordBusy) {
			res.FailTooManyRequests(c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}
	token, err := auth.Runtime.Login(cr.Password)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordBusy) {
			res.FailTooManyRequests(c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(gin.H{"token": token}, c)
}
