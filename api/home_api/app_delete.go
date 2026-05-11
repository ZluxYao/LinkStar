package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type AppDeleteRequest struct {
	ID string `json:"id"`
}

// 删除 app:stun/static 都允许,删 stun 类型不影响 stun 那边
func (HomeApi) AppDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[AppDeleteRequest](c)
	if cr.ID == "" {
		res.FailWithMsg("id 不能为空", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		out := cfg.Apps[:0]
		found := false
		for _, a := range cfg.Apps {
			if a.ID == cr.ID {
				found = true
				continue
			}
			out = append(out, a)
		}
		if !found {
			return home.ErrNotFound
		}
		cfg.Apps = out
		return nil
	})
	if err == home.ErrNotFound {
		res.FailWithMsg("app 不存在", c)
		return
	}
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("删除成功", c)
}
