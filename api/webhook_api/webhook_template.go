package webhook_api

import (
	"linkstar/middleware"
	"linkstar/modules/webhook"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type WebhookTemplateSaveRequest struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Config      webhook.WebhookConfig `json:"config"`
}

type WebhookTemplateDeleteRequest struct {
	ID string `json:"id"`
}

// WebhookTemplateListView 返回全部 webhook 模板
func (WebhookApi) WebhookTemplateListView(c *gin.Context) {
	res.OkWithData(webhook.Runtime.ListTemplates(), c)
}

// WebhookTemplateAddView 新增自定义 webhook 模板
func (WebhookApi) WebhookTemplateAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[WebhookTemplateSaveRequest](c)
	tpl, err := webhook.Runtime.AddTemplate(webhook.WebhookTemplate{
		ID:          cr.ID,
		Name:        cr.Name,
		Description: cr.Description,
		Config:      cr.Config,
	})
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(tpl, c)
}

// WebhookTemplateUpdateView 更新自定义 webhook 模板
func (WebhookApi) WebhookTemplateUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[WebhookTemplateSaveRequest](c)
	tpl, err := webhook.Runtime.UpdateTemplate(cr.ID, webhook.WebhookTemplate{
		Name:        cr.Name,
		Description: cr.Description,
		Config:      cr.Config,
	})
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(tpl, c)
}

// WebhookTemplateDeleteView 删除自定义 webhook 模板
func (WebhookApi) WebhookTemplateDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[WebhookTemplateDeleteRequest](c)
	if err := webhook.Runtime.DeleteTemplate(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("删除成功", c)
}
