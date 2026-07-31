package auth_api

import (
	"errors"
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
		if errors.Is(err, auth.ErrLoginLimited) {
			res.FailRateLimited(c)
			return
		}
		if errors.Is(err, auth.ErrPasswordBusy) {
			res.FailTooManyRequests(c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(gin.H{"token": token}, c)
}
