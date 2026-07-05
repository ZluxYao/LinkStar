package auth_api

import (
	"linkstar/middleware"
	"linkstar/modules/auth"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// ChangePasswordView 受保护：校验旧密码后更新
func (AuthApi) ChangePasswordView(c *gin.Context) {
	cr := middleware.GetBindRequest[ChangePasswordRequest](c)
	if err := auth.Runtime.ChangePassword(cr.OldPassword, cr.NewPassword); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("密码已更新", c)
}
