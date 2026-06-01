package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// ============ 配置读写 ============
//
// 参考 modules/stun/stun_config.go 的模式：
//   - 配置文件路径固定在 config/xxxConfig.json
//   - 首次启动文件不存在 → 创建带占位符的默认配置
//   - 读写用 json.Marshal/Unmarshal
//   - 维护 CreatedAt / UpdatedAt 时间戳

const ConfigPath = "config/ddnsConfig.json"

// Config DDNS demo 的完整配置
type Config struct {
	CreatedAt time.Time `json:"createdAt"` // 配置创建时间
	UpdatedAt time.Time `json:"updatedAt"` // 最后更新时间

	CF     CFConfig `json:"cf"`               // Cloudflare 业务配置
	Notify string   `json:"notify,omitempty"` // 可选：CF 更新成功后 POST 通知到这个 URL
}

// CFConfig Cloudflare 相关参数
type CFConfig struct {
	Token       string   `json:"token"`                 // CF API Token
	ZoneID      string   `json:"zoneID"`                // 可选；留空会自动从 recordName 反查
	RecordName  string   `json:"recordName,omitempty"`  // 兼容旧配置：单个完整域名
	RecordNames []string `json:"recordNames,omitempty"` // 多个完整域名，如 home.example.com
}

// 占位符：用于检测用户有没有改过默认值
const tokenPlaceholder = "把你的 CF API Token 填这里"

// readOrCreateConfig 读取配置；不存在就创建带占位符的默认配置
func readOrCreateConfig(path string) (Config, error) {
	if fi, err := os.Stat(path); os.IsNotExist(err) || (fi != nil && fi.Size() == 0) {
		return createDefaultConfig(path)
	}
	return readJsonFile[Config](path)
}

// createDefaultConfig 首次启动生成默认配置文件
func createDefaultConfig(path string) (Config, error) {
	now := time.Now()
	config := Config{
		CreatedAt: now,
		UpdatedAt: now,
		CF: CFConfig{
			Token:       tokenPlaceholder,
			ZoneID:      "",
			RecordNames: []string{"home.example.com"},
		},
		Notify: "",
	}

	// 确保 config 目录存在
	if err := os.MkdirAll("config", 0755); err != nil {
		return config, fmt.Errorf("创建 config 目录失败: %w", err)
	}

	if err := writeJsonFile(path, config); err != nil {
		return config, fmt.Errorf("写入默认配置失败: %w", err)
	}
	fmt.Printf("🆕 已创建默认配置: %s\n", path)
	fmt.Println("   请打开它，把 token / recordNames 填好后重新运行")
	return config, nil
}

// validateConfig 检查必填项是否齐全
func validateConfig(c Config) error {
	if c.CF.Token == "" || c.CF.Token == tokenPlaceholder {
		return fmt.Errorf("配置文件里的 cf.token 还是占位符，请先填好真实 Token")
	}
	if len(c.CF.Names()) == 0 {
		return fmt.Errorf("配置文件里的 cf.recordNames 不能为空")
	}
	return nil
}

// Names returns all configured record names and keeps compatibility with recordName.
func (c CFConfig) Names() []string {
	seen := make(map[string]struct{}, len(c.RecordNames)+1)
	names := make([]string, 0, len(c.RecordNames)+1)

	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	add(c.RecordName)
	for _, name := range c.RecordNames {
		add(name)
	}
	return names
}

// ============ 通用 JSON 读写（抄自 utils/utilsFile/utils_json.go）============

func readJsonFile[T any](path string) (T, error) {
	var result T
	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	return result, nil
}

func writeJsonFile[T any](path string, obj T) error {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
