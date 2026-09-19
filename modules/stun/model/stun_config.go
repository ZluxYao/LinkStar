package model

import (
	"linkstar/modules/webhook"
	"time"
)

type Config struct {
	// 基础网络信息

	BestSTUN  string    `json:"bestStun"`  // 最快的STUN服务器
	CreatedAt time.Time `json:"createdAt"` // 配置创建时间
	UpdatedAt time.Time `json:"updatedAt"` // 最后更新时间

	Devices        []Device `json:"devices"`        // stun设备列表
	StunServerList []string `json:"stunServerList"` // stun服务器列表
}

type Device struct {
	DeviceID uint      `json:"id"`       // id
	Name     string    `json:"name"`     // "本机" / "群晖NAS" / "树莓派"
	IP       string    `json:"ip"`       // 设备ip
	Services []Service `json:"services"` // 该设备上的服务

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Service 单个服务配置
type Service struct {
	ID             uint   `json:"id"` // 服务唯一标识符
	StartupSuccess bool   `json:"-"`
	Name           string `json:"name"`         // 服务名称,如 "SSH" / "Web管理" / "照片库"
	InternalPort   uint16 `json:"internalPort"` // 内网端口,如 22
	Protocol       string `json:"protocol"`     // 传输协议 "TCP"/"UDP" (默认 TCP)
	Https          bool   `json:"https"`        // 仅影响链接展示成 http:// 还是 https://,不改变转发行为

	// 对外访问配置
	Domain       string `json:"domain"`       // 该服务对外域名,如 fw.example.com;留空回落公网 IP
	TLSTerminate bool   `json:"tlsTerminate"` // 由 LinkStar 在洞口终结 TLS(仅 TCP 有效)
	CertID       uint   `json:"certId"`       // 绑定的证书 ID,0 表示按域名自动匹配

	// BackendHTTPS LinkStar 拨内网时用 tls.Dial,等价于 nginx 的 proxy_pass https://。
	//
	// 它描述的不是「内网是不是 HTTPS」,而是「LinkStar 要不要自己去和内网握手」,
	// 只在 TLSTerminate 为真时才成立——那时洞口解了密,必须重新建一条连接进内网。
	// TLSTerminate 为假时洞是纯字节管道,握手是浏览器和内网服务之间的事,
	// 此时置真就成了双层 TLS,浏览器侧报 ERR_SSL_PROTOCOL_ERROR。
	//
	// 也不能复用上面的 Https:那个字段一直只管展示,老用户为了让首页链接显示成
	// https:// 就会勾它,而服务本身多半是明文。拿它去决定怎么拨号,
	// 等于让所有勾过的老配置在升级后直接转发失败。
	BackendHTTPS bool `json:"backendHttps"`

	// UPnP 相关配置
	UseUPnP        bool   `json:"useUpnp"`        // 是否启用 UPnP 自动端口映射 (默认 true)
	UPnPMappedPort uint16 `json:"upnpMappedPort"` // UPnP 实际映射成功的端口号

	Enabled     bool   `json:"enabled"`     // 服务是否启用 (默认 true)
	Description string `json:"description"` // 服务描述信息 (可选)

	WebHookConfig webhook.WebhookConfig `json:"webhookconfig"` // Webhook 配置文件
	Redirect      RedirectConfig        `json:"redirect"`      // 固定域名跟着外部端口走

	UpdatedAt time.Time `json:"updatedAt"` // 最后更新时间
}

// RedirectConfig 让一个固定的域名，始终指向这个服务当前的外网地址。
//
// 它替代的是「Cloudflare 重定向规则」那个 webhook 模板。用那个模板得自己去
// Cloudflare 后台抄三个 ID（zone / ruleset / rule）贴进 URL，还要手写整段 JSON，
// 而且在后台重建一次规则，rule ID 就变了，webhook 会静默失效。
//
// 这里三个 ID 一个都不用填：zone 用域名查，规则集走固定的 phase 入口，
// 自己那条规则按 description 认领。所以只剩下面三个字段要填。
type RedirectConfig struct {
	Enabled    bool   `json:"enabled"`
	ProviderID uint   `json:"providerId"` // 用 DDNS 里已经配好的服务商，token 不用再贴一遍
	EntryHost  string `json:"entryHost"`  // 外面访问用的名字，如 linkstar.example.com
	ZoneDomain string `json:"zoneDomain"` // 主域名；留空按 EntryHost 的后两段取
}
