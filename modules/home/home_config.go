package home

import (
	"linkstar/utils/utilsFile"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

const ConfigPath = "config/homeConfig.json"

// ReadConfig 读取 home 配置，文件不存在则用默认值创建
func ReadConfig() (Config, error) {
	if fi, err := os.Stat(ConfigPath); os.IsNotExist(err) || (fi != nil && fi.Size() == 0) {
		return createConfig()
	}
	cfg, err := utilsFile.ReadJsonFile[Config](ConfigPath)
	if err != nil {
		logrus.Error("Home Config 读取失败：", err)
		return cfg, err
	}
	return cfg, nil
}

// createConfig 首次启动时写入默认值
func createConfig() (Config, error) {
	now := time.Now()
	cfg := Config{
		Title:    "LinkStar",
		ShowTime: true,
		Wallpaper: Wallpaper{
			Mode:       "bing",
			Resolution: "1080",
			Blur:       0,
		},
		LayoutMode:    "paged-free",
		NetworkPrefer: "wanV4",

		DefaultSearchEngineID: "bing",
		SearchEngines:         defaultSearchEngines(),
		SearchHistory:         []string{},

		Categories: defaultCategories(now),
		Apps:       []App{},

		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := os.MkdirAll("config", 0755); err != nil {
		logrus.Error("创建 config 目录失败：", err)
		return cfg, err
	}
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		logrus.Error("Home Config 写入失败：", err)
		return cfg, err
	}
	return cfg, nil
}

// UpdateConfig 写回当前 Runtime.Config（调用者已自行加锁/修改 UpdatedAt 时除外）
func UpdateConfig(cfg Config) error {
	cfg.UpdatedAt = time.Now()
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		logrus.Error("Home Config 写入失败：", err)
		return err
	}
	return nil
}

func defaultSearchEngines() []SearchEngine {
	return []SearchEngine{
		{ID: "bing", Name: "必应", ShortName: "B", URL: "https://www.bing.com/search?q=", Color: "from-sky-500 to-blue-600", Icon: "/icons/bing.com.ico", Order: 1},
		{ID: "google", Name: "谷歌", ShortName: "G", URL: "https://www.google.com/search?q=", Color: "from-red-500 to-yellow-500", Icon: "/icons/google.com.svg", Order: 2},
		{ID: "duckduckgo", Name: "DuckDuckGo", ShortName: "D", URL: "https://duckduckgo.com/?q=", Color: "from-orange-500 to-red-500", Icon: "/icons/dackdackgo.com.svg", Order: 3},
		{ID: "brave", Name: "Brave", ShortName: "Br", URL: "https://search.brave.com/search?q=", Color: "from-orange-600 to-red-700", Icon: "/icons/brave.com.svg", Order: 4},
		{ID: "github", Name: "GitHub", ShortName: "Gh", URL: "https://github.com/search?q=", Color: "from-gray-700 to-gray-900", Icon: "/icons/github.com.svg", Order: 5},
		{ID: "zhihu", Name: "知乎", ShortName: "Zh", URL: "https://www.zhihu.com/search?type=content&q=", Color: "from-blue-500 to-indigo-600", Icon: "/icons/zhihu.com.ico", Order: 6},
		{ID: "bilibili", Name: "B站", ShortName: "B", URL: "https://search.bilibili.com/all?keyword=", Color: "from-pink-400 to-pink-600", Icon: "/icons/bilibili.com.ico", Order: 7},
	}
}

func defaultCategories(now time.Time) []Category {
	return []Category{
		{ID: "productivity", Name: "生产力", Order: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "tools", Name: "工具", Order: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "entertainment", Name: "娱乐", Order: 3, CreatedAt: now, UpdatedAt: now},
	}
}
