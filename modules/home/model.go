package home

import "time"

const SearchHistoryMax = 24

type Config struct {
	Title    string `json:"title"`
	ShowTime bool   `json:"showTime"`

	// 背景
	Wallpaper Wallpaper `json:"wallpaper"`

	// 布局
	LayoutMode string `json:"layoutMode"` // paged-horizontal/paged-vertical/paged-free/scroll

	// 网络入口偏好
	NetworkPrefer string `json:"networkPrefer"` // wanV4 / wanV6 / lan

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
	WANv4 string `json:"wanV4"` // 公网 IPv4 入口 / URL
	WANv6 string `json:"wanV6"` // 公网 IPv6 入口
	LAN   string `json:"lan"`   // 内网 IPv4 入口
}
