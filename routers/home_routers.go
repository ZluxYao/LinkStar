package routers

import (
	"linkstar/api"
	"linkstar/api/home_api"
	"linkstar/middleware"

	"github.com/gin-gonic/gin"
)

func HomeRouters(g *gin.RouterGroup) {
	var app = api.App.HomeApi

	// 整体读
	g.GET("home/config", app.GetHomeConfigView)
	g.GET("home/bing-wallpaper", app.BingWallpaperView)

	// 主页装饰
	g.PUT("home/wallpaper",
		middleware.BindJsonMiddleware[home_api.WallpaperUpdateRequest], app.WallpaperUpdateView)
	g.PUT("home/layout",
		middleware.BindJsonMiddleware[home_api.LayoutUpdateRequest], app.LayoutUpdateView)
	g.PUT("home/network",
		middleware.BindJsonMiddleware[home_api.NetworkUpdateRequest], app.NetworkUpdateView)

	// 搜索引擎
	g.POST("home/search-engine/add",
		middleware.BindJsonMiddleware[home_api.SearchEngineAddRequest], app.SearchEngineAddView)
	g.PUT("home/search-engine/update",
		middleware.BindJsonMiddleware[home_api.SearchEngineUpdateRequest], app.SearchEngineUpdateView)
	g.DELETE("home/search-engine/delete",
		middleware.BindJsonMiddleware[home_api.SearchEngineDeleteRequest], app.SearchEngineDeleteView)
	g.PUT("home/search-engine/reorder",
		middleware.BindJsonMiddleware[home_api.SearchEngineReorderRequest], app.SearchEngineReorderView)
	g.PUT("home/search-engine/default",
		middleware.BindJsonMiddleware[home_api.SearchEngineDefaultRequest], app.SearchEngineDefaultView)

	// 搜索历史
	g.GET("home/search-history", app.SearchHistoryGetView)
	g.POST("home/search-history/add",
		middleware.BindJsonMiddleware[home_api.SearchHistoryAddRequest], app.SearchHistoryAddView)
	g.DELETE("home/search-history/clear", app.SearchHistoryClearView)

	// 分类
	g.POST("home/category/add",
		middleware.BindJsonMiddleware[home_api.CategoryAddRequest], app.CategoryAddView)
	g.PUT("home/category/update",
		middleware.BindJsonMiddleware[home_api.CategoryUpdateRequest], app.CategoryUpdateView)
	g.DELETE("home/category/delete",
		middleware.BindJsonMiddleware[home_api.CategoryDeleteRequest], app.CategoryDeleteView)
	g.PUT("home/category/reorder",
		middleware.BindJsonMiddleware[home_api.CategoryReorderRequest], app.CategoryReorderView)

	// App (常用网站)
	g.POST("home/app/add",
		middleware.BindJsonMiddleware[home_api.AppAddRequest], app.AppAddView)
	g.PUT("home/app/update",
		middleware.BindJsonMiddleware[home_api.AppUpdateRequest], app.AppUpdateView)
	g.DELETE("home/app/delete",
		middleware.BindJsonMiddleware[home_api.AppDeleteRequest], app.AppDeleteView)
	g.PUT("home/app/reorder",
		middleware.BindJsonMiddleware[home_api.AppReorderRequest], app.AppReorderView)
	g.PUT("home/app/category",
		middleware.BindJsonMiddleware[home_api.AppCategoryRequest], app.AppCategoryView)

	// 图标上传
	g.POST("home/icon/upload", app.IconUploadView)
}
