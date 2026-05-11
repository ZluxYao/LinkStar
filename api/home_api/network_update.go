package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type NetworkUpdateRequest struct {
	NetworkPrefer string `json:"networkPrefer"`
}

func (HomeApi) NetworkUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[NetworkUpdateRequest](c)

	switch cr.NetworkPrefer {
	case "wanV4", "wanV6", "lan":
	default:
		res.FailWithMsg("networkPrefer 只能是 wanV4 / wanV6 / lan", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		cfg.NetworkPrefer = cr.NetworkPrefer
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("更新成功", c)
}
