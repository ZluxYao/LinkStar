package routers

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"linkstar/modules/stun"

	"github.com/gin-gonic/gin"
)

// 服务索引：把「服务名 → 当前 STUN 外部端口」这张表放在 LinkStar 自己手里。
//
// 为什么需要它：STUN 打出来的外部端口是漂移的，而 DNS A 记录里带不了端口。
// 以前每个服务都要在 Cloudflare 上挂一条重定向规则（免费版上限 10 条），
// 端口一变还得靠 webhook 回写。现在只需要把 LinkStar 自己这一个洞暴露出去，
// 其余服务全部由这里查表 307 过去，CF 规则从 N 条降到 1 条。
//
//	https://linkstar.zlux.top/fw   （CF 边缘，唯一一条重定向规则）
//	   → https://ls.zlux.top:21313/fw   （LinkStar 自己的洞）
//	   → 307 https://fw.zlux.top:34521/  （fw 的洞，LinkStar 在那儿终结 TLS）

// PortalPrefix 显式入口前缀，避免服务名和前端路由/静态资源撞车时无路可走
const PortalPrefix = "/go"

// PortalRouters 注册显式入口 /go/{服务名}/...
func PortalRouters(r *gin.Engine) {
	r.Any(PortalPrefix+"/*rest", func(c *gin.Context) {
		name, rest := splitFirstSegment(c.Param("rest"))
		if name == "" {
			portalNotFound(c, "")
			return
		}
		if !portalRedirect(c, name, rest) {
			portalNotFound(c, name)
		}
	})
}

// TryPortal 供 NoRoute 兜底调用，实现裸路径 linkstar.zlux.top/{服务名}。
// 命中并已响应返回 true；没有同名服务返回 false，由调用方继续走前端 SPA 兜底。
//
// 调用位置很关键：必须排在静态文件命中检查之后，否则一个叫 favicon.ico
// 的服务名会把真实静态资源顶掉。
func TryPortal(c *gin.Context) bool {
	name, rest := splitFirstSegment(c.Request.URL.Path)
	if name == "" {
		return false
	}
	return portalRedirect(c, name, rest)
}

// portalRedirect 查表并发出 307。服务不存在返回 false。
func portalRedirect(c *gin.Context, name, rest string) bool {
	device, service := stun.FindServiceByName(name)
	if device == nil || service == nil {
		return false
	}

	if !service.Enabled {
		portalUnavailable(c, service.Name, "该服务已被禁用")
		return true
	}

	endpoint := stun.ServiceEndpoint(device.DeviceID, service)
	if !endpoint.Valid() {
		// 洞还没打通就别发一个注定打不开的重定向，直接说清楚
		reason := "内网穿透尚未就绪，请稍后重试"
		if endpoint.Host == "" {
			reason = "该服务未配置对外域名，且当前没有可用的公网 IP"
		}
		portalUnavailable(c, service.Name, reason)
		return true
	}

	target := endpoint.URL() + rest
	if raw := c.Request.URL.RawQuery; raw != "" {
		target += "?" + raw
	}

	// 端口会漂移，绝不能让浏览器缓存这个跳转。
	// 同理只能用 307：301/308 是永久重定向，浏览器会把一个迟早失效的端口
	// 永久记住，用户只能清浏览器数据才能恢复。307 还能保留 method 和 body。
	c.Header("Cache-Control", "no-store")
	c.Redirect(http.StatusTemporaryRedirect, target)
	return true
}

// splitFirstSegment 把 /fw/library/x 拆成 ("fw", "/library/x")
func splitFirstSegment(p string) (name, rest string) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", ""
	}
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i], p[i:]
	}
	return p, ""
}

func portalUnavailable(c *gin.Context, name, reason string) {
	c.Header("Cache-Control", "no-store")
	portalPage(c, http.StatusServiceUnavailable, name+" 暂时不可用", reason)
}

func portalNotFound(c *gin.Context, name string) {
	msg := "没有找到这个服务，请检查名称是否正确"
	if name == "" {
		msg = "请在地址后面加上服务名，例如 " + PortalPrefix + "/fw"
	}
	portalPage(c, http.StatusNotFound, "服务未找到", msg)
}

func portalPage(c *gin.Context, status int, title, detail string) {
	body := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title></head>
<body style="font-family:system-ui,-apple-system,sans-serif;max-width:32rem;margin:15vh auto;padding:0 1.5rem;color:#333">
<h1 style="font-size:1.25rem;margin:0 0 .5rem">%s</h1>
<p style="color:#666;line-height:1.6;margin:0">%s</p>
</body></html>`, html.EscapeString(title), html.EscapeString(title), html.EscapeString(detail))

	c.Data(status, "text/html; charset=utf-8", []byte(body))
}
