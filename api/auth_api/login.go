package auth_api

import (
	"linkstar/middleware"
	"linkstar/modules/auth"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Password string `json:"password"`
}

// LoginView 校验密码，成功返回 token
func (AuthApi) LoginView(c *gin.Context) {
	cr := middleware.GetBindRequest[LoginRequest](c)
	token, err := auth.Runtime.Login(cr.Password)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(gin.H{"token": token}, c)
}
