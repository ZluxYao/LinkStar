package proxy

import (
	"os"
	"path"
	"time"

	"linkstar/modules/proxy/model"
	"linkstar/utils/utilsFile"

	"github.com/sirupsen/logrus"
)

const ConfigPath = "config/proxyConfig.json"

// ReadConfig 读取反代配置，文件不存在或为空则创建空配置
func ReadConfig() (model.ProxyConfig, error) {
	var config model.ProxyConfig

	if fileInfo, err := os.Stat(ConfigPath); os.IsNotExist(err) || (fileInfo != nil && fileInfo.Size() == 0) {
		return createConfig()
	}

	config, err := utilsFile.ReadJsonFile[model.ProxyConfig](ConfigPath)
	if err != nil {
		logrus.Error("ProxyConfig 读取失败：", err)
		return config, err
	}
	migrateSinglePort(&config)
	migrateSiteHosts(&config)
	return config, nil
}

// migrateSiteHosts 把旧版站点的单个 host 搬进 hosts 数组
func migrateSiteHosts(cfg *model.ProxyConfig) {
	for i := range cfg.Sites {
		s := &cfg.Sites[i]
		if len(s.Hosts) == 0 && s.LegacyHost != "" {
			s.Hosts = []string{s.LegacyHost}
		}
		s.LegacyHost = ""
	}
}

// migrateSinglePort 把旧版的「一个端口 + 一个 TLS 开关」搬到新的双入口模型上。
//
// 旧配置里 listenPort 是唯一的监听端口，tls 决定它是 http 还是 https。
// 搬过来就是：https 的话放进 HTTPSPort，并且把已有站点都勾上 HTTPS——
// 否则升级完这些站点会从 443 上消失，用户只会看到「升级把我的反代搞坏了」。
func migrateSinglePort(cfg *model.ProxyConfig) {
	if cfg.LegacyPort <= 0 || cfg.HTTPPort > 0 || cfg.HTTPSPort > 0 {
		cfg.LegacyPort, cfg.LegacyTLS = 0, false
		return
	}

	if cfg.LegacyTLS {
		cfg.HTTPSPort = cfg.LegacyPort
		for i := range cfg.Sites {
			cfg.Sites[i].HTTPS = true
		}
	} else {
		cfg.HTTPPort = cfg.LegacyPort
	}

	logrus.Infof("[proxy] 已把旧的单端口配置（%d）迁移到新的双入口模型", cfg.LegacyPort)
	cfg.LegacyPort, cfg.LegacyTLS = 0, false
}

func createConfig() (model.ProxyConfig, error) {
	var config model.ProxyConfig
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()
	// 预填端口但不启用：用户在页面上点「开启」之前，不该有任何端口被占上
	config.HTTPPort = model.DefaultHTTPPort
	config.HTTPSPort = model.DefaultHTTPSPort
	config.Sites = []model.Site{}

	if err := os.MkdirAll(path.Dir(ConfigPath), 0755); err != nil {
		logrus.Error("创建 ProxyConfig 目录失败：", err)
		return config, err
	}

	if err := utilsFile.WriteJsonFile(ConfigPath, config); err != nil {
		logrus.Error("ProxyConfig 写入失败：", err)
		return config, err
	}
	return config, nil
}

// SaveConfig 把配置写入磁盘（纯存盘，自带更新时间戳）
func SaveConfig(config model.ProxyConfig) error {
	config.UpdatedAt = time.Now()

	if err := utilsFile.WriteJsonFile(ConfigPath, config); err != nil {
		logrus.Error("ProxyConfig 写入失败：", err)
		return err
	}
	return nil
}
