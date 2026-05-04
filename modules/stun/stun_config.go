package stun

import (
	"linkstar/modules/stun/model"
	"linkstar/utils/utilsFile"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

const ConfigPath = "config/stunConfig.json"

// shutdownChan 用于接收退出信号
var shutdownChan = make(chan struct{})

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

	}
	return config, nil

}

func createConfig() (model.Config, error) {
	var config model.Config
	// 首次创建，设置创建时间
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	// 初始化stun服务器
	config.StunServerList = []string{"stun.radiojar.com:3478",
		"stun.ringostat.com:3478",
		"stun.irishvoip.com:3478",
		"stun.voipgate.com:3478",
		"stun.tula.nu:3478",
		"stun.yesdates.com:3478",
		"stun.telnyx.com:3478",
		"stun.vavadating.com:3478",
		"stun.bau-ha.us:3478",
		"stun.bridesbay.com:3478",
		"stun.3wayint.com:3478",
		"stun.romaaeterna.nl:3478",
		"stun.fitauto.ru:3478",
		"stun.antisip.com:3478",
		"stun.heeds.eu:3478",
		"stun.hot-chilli.net:3478",
		"stun.eurosys.be:3478",
		"stun.vincross.com:3478",
		"stun.cibercloud.com.br:3478",
		"stun.siptrunk.com:3478",
		"stun.chat.bilibili.com:3478",
		"stun.hitv.com:3478",
		"stun.miwifi.com:3478",
		"stun.cloudflare.com:3478",
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

// setupShutdownHook 监听退出信号，确保配置文件
func SetupShutdownHook(saveFn func()) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGABRT, syscall.SIGALRM)

	go func() {
		sig := <-signalChan
		logrus.Infof("收到退出信号：%v ,正在保存配置文件", sig)

		if saveFn != nil {
			saveFn()
		}

		logrus.Info("配置保存，程序退出")
		os.Exit(0)
	}()

}
