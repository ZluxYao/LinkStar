package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"
	"strings"

	"github.com/gin-gonic/gin"
)

type SearchHistoryAddRequest struct {
	Keyword string `json:"keyword"`
}

func (HomeApi) SearchHistoryGetView(c *gin.Context) {
	list := []string{}
	home.Runtime.Read(func(cfg *home.Config) {
		list = append(list, cfg.SearchHistory...)
	})
	res.OkWithData(list, c)
}

func (HomeApi) SearchHistoryAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[SearchHistoryAddRequest](c)
	keyword := strings.TrimSpace(cr.Keyword)
	if keyword == "" {
		res.FailWithMsg("keyword 不能为空", c)
		return
	}

	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		// 去重：移除已存在的
		out := make([]string, 0, len(cfg.SearchHistory)+1)
		out = append(out, keyword)
		for _, kw := range cfg.SearchHistory {
			if kw == keyword {
				continue
			}
			out = append(out, kw)
		}
		if len(out) > home.SearchHistoryMax {
			out = out[:home.SearchHistoryMax]
		}
		cfg.SearchHistory = out
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("已记录", c)
}

func (HomeApi) SearchHistoryClearView(c *gin.Context) {
	err := home.Runtime.WithLock(func(cfg *home.Config) error {
		cfg.SearchHistory = []string{}
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}
	res.OkWithMsg("已清空", c)
}
