package auth

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"time"

	"linkstar/utils/utilsFile"

	"github.com/sirupsen/logrus"
)

const ConfigPath = "config/authConfig.json"

// 默认 token 有效期：7 天
const defaultTokenTTLHours = 24 * 7

// ReadConfig 读取鉴权配置，文件不存在则用默认值创建（含随机 JwtSecret）
func ReadConfig() (Config, error) {
	if fi, err := os.Stat(ConfigPath); os.IsNotExist(err) || (fi != nil && fi.Size() == 0) {
		return createConfig()
	}
	cfg, err := utilsFile.ReadJsonFile[Config](ConfigPath)
	if err != nil {
		logrus.Error("Auth Config 读取失败：", err)
		return cfg, err
	}
	// 兜底：老配置或被清空的 secret，补一个
	if cfg.JwtSecret == "" {
		cfg.JwtSecret = randomSecret()
	}
	if cfg.TokenTTLHours <= 0 {
		cfg.TokenTTLHours = defaultTokenTTLHours
	}
	return cfg, nil
}

// createConfig 首次启动写入默认值：未初始化 + 随机 JwtSecret
func createConfig() (Config, error) {
	cfg := Config{
		Initialized:   false,
		JwtSecret:     randomSecret(),
		TokenTTLHours: defaultTokenTTLHours,
		UpdatedAt:     time.Now().Format(time.RFC3339),
	}
	if err := os.MkdirAll("config", 0755); err != nil {
		logrus.Error("创建 config 目录失败：", err)
		return cfg, err
	}
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		logrus.Error("Auth Config 写入失败：", err)
		return cfg, err
	}
	return cfg, nil
}

// UpdateConfig 写回配置文件
func UpdateConfig(cfg Config) error {
	cfg.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		logrus.Error("Auth Config 写入失败：", err)
		return err
	}
	return nil
}

// randomSecret 生成 32 字节的 hex 随机密钥
func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败极罕见，用时间兜底避免空 secret
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}
