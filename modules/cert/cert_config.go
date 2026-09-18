package cert

import (
	"fmt"
	"linkstar/modules/cert/model"
	"linkstar/utils/utilsFile"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

const ConfigPath = "config/certConfig.json"

// DataDir 证书 PEM 落盘根目录，与 data/icon 同级
const DataDir = "data/cert"

// ReadConfig 读取证书配置，文件不存在则创建空配置
func ReadConfig() (model.CertConfig, error) {
	var config model.CertConfig

	if fileInfo, err := os.Stat(ConfigPath); os.IsNotExist(err) || (fileInfo != nil && fileInfo.Size() == 0) {
		return createConfig()
	}

	config, err := utilsFile.ReadJsonFile[model.CertConfig](ConfigPath)
	if err != nil {
		logrus.Error("CertConfig 读取失败：", err)
		return config, err
	}
	return config, nil
}

func createConfig() (model.CertConfig, error) {
	var config model.CertConfig
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()
	config.Certificates = []model.Certificate{}

	if err := os.MkdirAll(path.Dir(ConfigPath), 0755); err != nil {
		logrus.Error("创建 CertConfig 目录失败：", err)
		return config, err
	}

	if err := utilsFile.WriteJsonFile(ConfigPath, config); err != nil {
		logrus.Error("CertConfig 写入失败：", err)
		return config, err
	}
	return config, nil
}

// SaveConfig 把配置写入磁盘（纯存盘，自带更新时间戳）
func SaveConfig(config model.CertConfig) error {
	config.UpdatedAt = time.Now()

	if err := utilsFile.WriteJsonFile(ConfigPath, config); err != nil {
		logrus.Error("CertConfig 写入失败：", err)
		return err
	}
	return nil
}

// certDir 返回某张证书的数据目录 data/cert/{id}
func certDir(id uint) string {
	return filepath.Join(DataDir, fmt.Sprintf("%d", id))
}

// ensureCertDir 确保证书目录存在
func ensureCertDir(id uint) (string, error) {
	dir := certDir(id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建证书目录失败: %w", err)
	}
	return dir, nil
}

// managedPaths 返回由 LinkStar 自己管理的 PEM 路径（upload / acme-* 用）
func managedPaths(id uint) (certPath, keyPath string) {
	dir := certDir(id)
	return filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem")
}

// accountKeyPath ACME 账户私钥路径，按证书持久化复用，避免每次续期重新注册账户
func accountKeyPath(id uint) string {
	return filepath.Join(certDir(id), "account.key")
}

// effectivePaths 返回该证书实际生效的 cert/key 文件路径
// path 来源用用户填的路径，其余来源用 LinkStar 托管的路径
func effectivePaths(c model.Certificate) (certPath, keyPath string) {
	if c.Source == model.SourcePath {
		return c.CertFile, c.KeyFile
	}
	return managedPaths(c.ID)
}
