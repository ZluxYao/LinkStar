package home_api

import (
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

type HomeConfigViewResponse struct {
	Title    string `json:"title"`
	ShowTime bool   `json:"showTime"`

	Wallpaper home.Wallpaper `json:"wallpaper"`

	LayoutMode    string `json:"layoutMode"`
	NetworkPrefer string `json:"networkPrefer"`

	DefaultSearchEngineID string              `json:"defaultSearchEngineId"`
	SearchEngines         []home.SearchEngine `json:"searchEngines"`
	SearchHistory         []string            `json:"searchHistory"`

	Categories []home.Category `json:"categories"`
	Apps       []home.AppView  `json:"apps"`
}

// GetHomeConfigView 整体读,含 hydrated apps
func (HomeApi) GetHomeConfigView(c *gin.Context) {
	var resp HomeConfigViewResponse
	home.Runtime.Read(func(cfg *home.Config) {
		// 注意:必须用 []T{} 作为 base,append 空切片到 []T(nil) 仍返回 nil,
		// JSON 会序列化成 null,前端拿到 null.length 直接崩
		resp = HomeConfigViewResponse{
			Title:                 cfg.Title,
			ShowTime:              cfg.ShowTime,
			Wallpaper:             cfg.Wallpaper,
			LayoutMode:            cfg.LayoutMode,
			NetworkPrefer:         cfg.NetworkPrefer,
			DefaultSearchEngineID: cfg.DefaultSearchEngineID,
			SearchEngines:         append([]home.SearchEngine{}, cfg.SearchEngines...),
			// 导航页本身公开，但搜索词不是。没登录就给空列表，避免前端拿到 null。
			SearchHistory: searchHistoryFor(c, cfg.SearchHistory),
			Categories:    append([]home.Category{}, cfg.Categories...),
			Apps:          home.HydrateApps(cfg.Apps),
		}
	})
	res.OkWithData(resp, c)
}

func searchHistoryFor(c *gin.Context, history []string) []string {
	if !middleware.IsAuthed(c) {
		return []string{}
	}
	return append([]string{}, history...)
}
