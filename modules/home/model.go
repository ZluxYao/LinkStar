package home

import "time"

type Config struct {
	Title    string `json:"title"`
	ShowTime bool   `json:"showTime"`

	// 背景
	Wallpaper Wallpaper `json:"wallpaper"`

	// 布局 & 网络偏好
	LayoutMode  string `json:"layoutMode"`  // paged-horizontal/paged-vertical/paged-free/scroll
	NetworkMode string `json:"networkMode"` // wan / lan
	WANPrefer   string `json:"wanPrefer"`   // ipv4 / ipv6

	// 搜索
	DefaultSearchEngineID string         `json:"defaultSearchEngineId"`
	SearchEngines         []SearchEngine `json:"searchEngines"`
	SearchHistory         []string       `json:"searchHistory"` // 最多 24 条

	// 应用 & 分类
	Categories []Category `json:"categories"`
	Apps       []App      `json:"apps"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// 背景
type Wallpaper struct {
	Mode       string `json:"mode"`       // default / bing
	Resolution string `json:"resolution"` // 1080 / uhd
	Blur       int    `json:"blur"`       // 0-12
}

// 搜索
type SearchEngine struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
	URL       string `json:"url"`
	Color     string `json:"color"`
	Icon      string `json:"icon"` // 可选,/icons/xxx 或 data/icon/xxx
	Order     int    `json:"order"`
}

type Category struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Order int    `json:"order"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type App struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`       // data/icon/xxx
	Color      string `json:"color"`      // tailwind gradient,无图标时背景用
	CategoryID string `json:"categoryId"` // 空 = 未分类

	PagedOrder  int `json:"pagedOrder"`  // 翻页模式全局序
	ScrollOrder int `json:"scrollOrder"` // 整页模式分类内序

	Type string `json:"type"` // stun / static

	// type=stun:
	StunDeviceID  uint `json:"stunDeviceId,omitempty"`
	StunServiceID uint `json:"stunServiceId,omitempty"`

	// type=static:
	Addresses *AppAddresses `json:"addresses,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AppAddresses 应用的访问入口
type AppAddresses struct {
	WAN  string `json:"wan"`  //WAN:  公网地址 / 网址,如 https://github.com 或 http://1.2.3.4:8080
	LAN  string `json:"lan"`  //LAN:  内网 IPv4 入口(可选,本地服务才填)
	IPv6 string `json:"ipv6"` //- IPv6: IPv6 全球地址(可选,无需打洞)
}
