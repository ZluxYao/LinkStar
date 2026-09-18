package home

import (
	"fmt"
	"linkstar/modules/stun"
)

// AppView 给前端的统一视图,屏蔽 stun/static 来源差异
type AppView struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Icon        string       `json:"icon"`
	Color       string       `json:"color"`
	CategoryID  string       `json:"categoryId"`
	PagedOrder  int          `json:"pagedOrder"`
	ScrollOrder int          `json:"scrollOrder"`
	Type        string       `json:"type"`
	Addresses   AppAddresses `json:"addresses"`
	Online      *bool        `json:"online,omitempty"` // nil = 不展示绿点
	CreatedAt   string       `json:"createdAt,omitempty"`
	UpdatedAt   string       `json:"updatedAt,omitempty"`
}

// HydrateApps 把 Apps 合成成 AppView 列表
func HydrateApps(apps []App) []AppView {
	out := make([]AppView, 0, len(apps))
	for _, app := range apps {
		out = append(out, hydrateApp(app))
	}
	return out
}

func hydrateApp(app App) AppView {
	v := AppView{
		ID:          app.ID,
		Name:        app.Name,
		Icon:        app.Icon,
		Color:       app.Color,
		CategoryID:  app.CategoryID,
		PagedOrder:  app.PagedOrder,
		ScrollOrder: app.ScrollOrder,
		Type:        app.Type,
	}
	if !app.CreatedAt.IsZero() {
		v.CreatedAt = app.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if !app.UpdatedAt.IsZero() {
		v.UpdatedAt = app.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	switch app.Type {
	case "stun":
		v.Addresses, v.Online = stunAddresses(app)
	case "static":
		if app.Addresses != nil {
			v.Addresses = *app.Addresses
		}
	}
	return v
}

// stunAddresses 从 stun runtime 取设备/服务 + scheduler 的当前阶段拼地址
func stunAddresses(app App) (AppAddresses, *bool) {
	var addrs AppAddresses
	var online *bool

	device, service := stun.FindDeviceService(app.StunDeviceID, app.StunServiceID)
	if device == nil || service == nil {
		// stun 那边已经没了但 hook 还没跑到（极少数情况）
		falseVal := false
		return addrs, &falseVal
	}

	// LAN: 设备内网 IP + 内网端口。内网是直连，协议只取决于服务本身，
	// 与洞口是否终结 TLS 无关，所以这里和 WAN 的 scheme 会不一样。
	if device.IP != "" && service.InternalPort != 0 {
		addrs.LAN = fmt.Sprintf("%s://%s:%d", stun.InternalScheme(service), device.IP, service.InternalPort)
	}

	// WANv4: 服务域名（留空回落公网 IP）+ 实时外部端口
	addrs.WANv4 = stun.ServiceEndpoint(app.StunDeviceID, service).URL()

	// 在线状态：以 scheduler 当前阶段为准
	if alive, known := stun.ServiceOnline(app.StunDeviceID, app.StunServiceID); known {
		online = &alive
	}

	return addrs, online
}
