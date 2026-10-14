package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
	// 老配置可能是 0644。读鉴权配置时收紧，而不是让所有 JSON 读取都去改父目录。
	if err := utilsFile.ProtectConfigFile(ConfigPath); err != nil {
		logrus.Error("Auth Config 权限收紧失败：", err)
		return cfg, err
	}
	// 兜底：老配置或被清空的 secret，补一个
	if cfg.JwtSecret == "" {
		secret, err := randomSecret()
		if err != nil {
			return cfg, err
		}
		cfg.JwtSecret = secret
	}
	if cfg.TokenTTLHours <= 0 {
		cfg.TokenTTLHours = defaultTokenTTLHours
	}
	return cfg, nil
}

// createConfig 首次启动写入默认值：未初始化 + 随机 JwtSecret
func createConfig() (Config, error) {
	secret, err := randomSecret()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Initialized:   false,
		JwtSecret:     secret,
		TokenTTLHours: defaultTokenTTLHours,
		UpdatedAt:     time.Now().Format(time.RFC3339),
	}
	if err := os.MkdirAll("config", 0700); err != nil {
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

// randomSecret 生成 32 字节的 hex 随机密钥。
// crypto/rand 失败时直接返回错误，不用时间戳凑一把可预测的密钥。
func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成签名密钥失败: %w", err)
	}
	return hex.EncodeToString(b), nil
}
