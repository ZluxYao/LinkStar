package webhook

import (
	"os"
	"path"
	"time"

	"linkstar/utils/utilsFile"

	"github.com/sirupsen/logrus"
)

// ConfigPath webhook 模块配置文件路径
const ConfigPath = "config/webhookConfig.json"

// ReadConfig 读取 webhook 配置文件，不存在或为空时创建默认配置
func ReadConfig() (Config, error) {
	if fi, err := os.Stat(ConfigPath); os.IsNotExist(err) || (fi != nil && fi.Size() == 0) {
		return createConfig()
	}
	cfg, err := utilsFile.ReadJsonFile[Config](ConfigPath)
	if err != nil {
		logrus.Error("Webhook Config 读取失败：", err)
		return cfg, err
	}
	return cfg, nil
}

// createConfig 创建带内置模板的默认配置并写入磁盘
func createConfig() (Config, error) {
	now := time.Now()
	cfg := Config{
		Templates: defaultTemplates(now),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := os.MkdirAll(path.Dir(ConfigPath), 0755); err != nil {
		logrus.Error("创建 webhook config 目录失败：", err)
		return cfg, err
	}
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		logrus.Error("Webhook Config 写入失败：", err)
		return cfg, err
	}
	return cfg, nil
}

// SaveConfig 把 webhook 配置写入磁盘文件（自带更新时间戳）
func SaveConfig(cfg Config) error {
	cfg.UpdatedAt = time.Now()
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		logrus.Error("Webhook Config 写入失败：", err)
		return err
	}
	return nil
}

// defaultTemplates 返回 webhook 内置模板列表
func defaultTemplates(now time.Time) []WebhookTemplate {
	return []WebhookTemplate{
		{
			ID:          "generic-json",
			Name:        "通用 JSON 通知",
			Description: "向任意 HTTP 接口推送 STUN 服务地址",
			Builtin:     true,
			Config: WebhookConfig{
				Enabled:             true,
				OnlyWhenChanged:     true,
				Method:              "POST",
				Headers:             "Content-Type: application/json",
				Body:                `{"service":"#{service_name}","device":"#{device_name}","address":"#{address}","protocol":"#{protocol}","phase":"#{phase}","time":"#{updated_at}"}`,
				DisableSuccessCheck: true,
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          "cloudflare-srv",
			Name:        "Cloudflare SRV 记录",
			Description: "把 STUN 外部端口写入 Cloudflare SRV 记录",
			Builtin:     true,
			Config: WebhookConfig{
				Enabled:         true,
				OnlyWhenChanged: true,
				URL:             "https://api.cloudflare.com/client/v4/zones/#{zone_id}/dns_records/#{record_id}",
				Method:          "PUT",
				Headers:         "Authorization: Bearer #{token}\nContent-Type: application/json",
				Body: `{
  "type": "SRV",
  "name": "_aa._tcp.istore",
  "ttl": 60,
  "data": {
    "service": "_aa",
    "proto": "_tcp",
    "name": "istore",
    "priority": 5,
    "weight": 0,
    "port": #{port},
    "target": "zlux.top"
  }
}`,
				DisableSuccessCheck: false,
				SuccessContains:     `"success":true`,
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          "cloudflare-redirect-rule",
			Name:        "Cloudflare 重定向规则",
			Description: "把访问规则重定向到当前 STUN 公网 IP",
			Builtin:     true,
			Config: WebhookConfig{
				Enabled:         true,
				OnlyWhenChanged: true,
				URL:             "https://api.cloudflare.com/client/v4/zones/#{zone_id}/rulesets/#{ruleset_id}/rules",
				Method:          "POST",
				Headers:         "Authorization: Bearer #{token}\nContent-Type: application/json",
				Body: `{
  "action": "redirect",
  "expression": "(http.host eq \"fn.zlux.top\")",
  "description": "fn",
  "action_parameters": {
    "from_value": {
      "status_code": 307,
      "target_url": {
        "expression": "concat(\"https://#{ipAddr}\", http.request.uri.path)"
      },
      "preserve_query_string": true
    }
  }
}`,
				DisableSuccessCheck: false,
				SuccessContains:     `"success":true`,
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}
