package auth_api

import (
	"linkstar/middleware"
	"linkstar/modules/auth"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type SetupRequest struct {
	Password string `json:"password"`
}

// SetupView 公开但仅在未初始化时可用：设置 admin 密码，成功后返回 token 直接进后台
func (AuthApi) SetupView(c *gin.Context) {
	cr := middleware.GetBindRequest[SetupRequest](c)
	if err := auth.Runtime.Setup(cr.Password); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	token, err := auth.Runtime.Login(cr.Password)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(gin.H{"token": token}, c)
}
