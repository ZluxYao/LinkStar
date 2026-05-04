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
