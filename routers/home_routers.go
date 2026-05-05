package routers

import (
	"linkstar/api"

	"github.com/gin-gonic/gin"
)

func HomeRouters(g *gin.RouterGroup) {
	var app = api.App.HomeApi

	g.GET(
		"home/bing-wallpaper",
		app.BingWallpaperView,
	)
}
