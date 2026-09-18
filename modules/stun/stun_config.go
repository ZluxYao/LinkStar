package stun

import (
	"linkstar/modules/stun/model"
	"linkstar/utils/utilsFile"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

const ConfigPath = "config/stunConfig.json"

// 读取stun_config 配置文件
func ReadConfig() (model.Config, error) {
	var config model.Config

	//检测文件是否存在
	if fileInfo, err := os.Stat(ConfigPath); os.IsNotExist(err) || fileInfo.Size() == 0 {
		//不存在创建空配置文件
		return createConfig()

	} else {
		//文件存在读取配置文件
		config, err = utilsFile.ReadJsonFile[model.Config](ConfigPath)
		if err != nil {
			logrus.Error("Config读取失败：", err)
			return config, err
		}
		normalizeServices(&config)

	}
	return config, nil

}

// normalizeServices 清掉配置文件里那些自相矛盾、必然连不上的组合。
//
// 目前只有一条：没勾「洞口终结 TLS」却勾了「转发给内网时也用 HTTPS」。
// 洞口不终结时这个洞是纯字节管道，浏览器的 TLS 直达内网服务；
// 再让 LinkStar 去 tls.Dial，等于把浏览器的 ClientHello 当明文塞进
// LinkStar 自己那条 TLS，双层 TLS 必坏，浏览器报 ERR_SSL_PROTOCOL_ERROR。
// 新版 UI 已经不让这么勾，但旧配置文件里可能存着，在这里一次性收敛，
// 免得 PublicScheme、首页地址、实际转发三处各说各话。
func normalizeServices(config *model.Config) {
	for i := range config.Devices {
		for j := range config.Devices[i].Services {
			svc := &config.Devices[i].Services[j]
			if svc.BackendHTTPS && !svc.TLSTerminate {
				svc.BackendHTTPS = false
				logrus.Warnf("服务 %s 勾了内网 HTTPS 但洞口没终结 TLS，这个组合必然握手失败，已按纯管道处理", svc.Name)
			}
		}
	}
}

func createConfig() (model.Config, error) {
	var config model.Config
	// 首次创建，设置创建时间
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	// 初始化stun服务器
	config.StunServerList = []string{
		"stun.annatel.net:3478",
		"stun.antisip.com:3478",
		"stun.commpeak.com:3478",
		"stun.dcalling.de:3478",
		"stun.freeswitch.org:3478",
		"stun.ipfire.org:3478",
		"stun.sip.us:3478",
		"stun.siplogin.de:3478",
		"stun.sonetel.net:3478",
		"stun.voip.blackberry.com:3478",
		"stun.nextcloud.com:443",
		"stun.flashdance.cx:3478",
		"fwa.lifesizecloud.com:3478",
		"stun.nextcloud.com:3478",
		"stun.radiojar.com:3478",
		"stun.sonetel.com:3478",
		"stun.voipgate.com:3478",
	}

	// 确保 config 目录存在
	if err := os.MkdirAll("config", 0755); err != nil {
		logrus.Error("创建config目录失败：", err)
		return config, err
	}

	// 写入一个空的配置文件
	if err := utilsFile.WriteJsonFile(ConfigPath, config); err != nil {
		logrus.Error("Config写入失败：", err)
		return config, err
	}
	return config, nil
}

// UpdateConfig 更新stun配置文件
func UpdateConfig(config model.Config) error {
	const ConfigPath = "config/stunConfig.json"

	// 更新时间戳
	config.UpdatedAt = time.Now()

	// 写入配置文件
	if err := utilsFile.WriteJsonFile(ConfigPath, config); err != nil {
		logrus.Error("Config写入失败：", err)
		return err
	}

	logrus.Info("STUN配置文件已更新")
	return nil
}
