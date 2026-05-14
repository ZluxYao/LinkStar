package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type IconFetchRequest struct {
	URL string `json:"url"`
}

// 从给定 URL 抓 favicon,返回保存到 data/icon/ 的相对路径。
func (HomeApi) IconFetchView(c *gin.Context) {
	cr := middleware.GetBindRequest[IconFetchRequest](c)
	path, err := home.FetchIconFromURL(cr.URL)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(gin.H{"path": path}, c)
}
