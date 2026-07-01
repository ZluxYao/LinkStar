package routers

import (
	"linkstar/api"
	"linkstar/api/webhook_api"
	"linkstar/middleware"

	"github.com/gin-gonic/gin"
)

func WebhookRouters(g *gin.RouterGroup) {
	var app = api.App.WebhookApi

	// webhook 模板：查 / 增 / 改 / 删
	g.GET(
		"webhook/templates",
		app.WebhookTemplateListView,
	)
	g.POST(
		"webhook/template/add",
		middleware.BindJsonMiddleware[webhook_api.WebhookTemplateSaveRequest],
		app.WebhookTemplateAddView,
	)
	g.PUT(
		"webhook/template/update",
		middleware.BindJsonMiddleware[webhook_api.WebhookTemplateSaveRequest],
		app.WebhookTemplateUpdateView,
	)
	g.DELETE(
		"webhook/template/delete",
		middleware.BindJsonMiddleware[webhook_api.WebhookTemplateDeleteRequest],
		app.WebhookTemplateDeleteView,
	)
}
