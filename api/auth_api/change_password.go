package auth_api

import (
	"errors"
	"linkstar/middleware"
	"linkstar/modules/auth"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// ChangePasswordView 受保护：校验旧密码后更新。
// 改完所有旧 token 都失效，所以这里回一个新 token 让当前页面接着用。
func (AuthApi) ChangePasswordView(c *gin.Context) {
	cr := middleware.GetBindRequest[ChangePasswordRequest](c)
	token, err := auth.Runtime.ChangePassword(cr.OldPassword, cr.NewPassword)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordBusy) {
			res.FailTooManyRequests(c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.Ok(gin.H{"token": token}, "密码已更新", c)
}
