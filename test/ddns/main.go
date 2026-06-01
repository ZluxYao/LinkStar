package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"ddns/cf"
	"ddns/webhook"
)

// 学习/测试 demo
//
// 当前 scope：
//  1. webhook.Do HTTP 工具函数
//  2. cf.UpdateRecord CF DDNS 业务
//  3. 配置走 JSON 文件（参考 modules/stun 的模式）
//
// 不在 scope（后面再做）：
//   - 多 webhook 列表 + CRUD
//   - 定时器 / stun 事件订阅
//   - 模板渲染
//
// 用法：
//
//	go run . -ip 1.2.3.4              # 用默认配置文件 config/ddnsConfig.json
//	go run . -config xxx.json -ip ... # 指定配置文件路径
//
// 首次运行会自动生成配置模板，编辑后再运行

func main() {
	var (
		flagConfig = flag.String("config", ConfigPath, "配置文件路径")
		flagIP     = flag.String("ip", "", "要写入的 IPv4（模拟 STUN 输出）")
	)
	flag.Parse()

	if *flagIP == "" {
		fmt.Fprintln(os.Stderr, "❌ 缺 -ip 参数")
		flag.Usage()
		os.Exit(1)
	}

	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║   DDNS Demo — webhook 工具 + cf 业务                     ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()

	// ━━━━━━━━━━ 步骤 1：加载配置 ━━━━━━━━━━
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📂 步骤 1: 加载配置文件")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	config, err := readOrCreateConfig(*flagConfig)
	if err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
	}
	if err := validateConfig(config); err != nil {
		log.Fatalf("❌ 配置无效: %v", err)
	}
	fmt.Printf("✅ 配置文件: %s\n", *flagConfig)
	fmt.Printf("   recordNames: %v\n", config.CF.Names())
	if config.CF.ZoneID == "" {
		fmt.Printf("   zoneID:     (自动反查)\n")
	} else {
		fmt.Printf("   zoneID:     %s\n", config.CF.ZoneID)
	}
	if config.Notify != "" {
		fmt.Printf("   notify:     %s\n", config.Notify)
	}
	fmt.Println()

	// ━━━━━━━━━━ 步骤 2：调 CF 业务更新 A 记录 ━━━━━━━━━━
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("☁️  步骤 2: 调 cf.UpdateRecord 更新 A 记录")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("   新 IP: %s\n\n", *flagIP)

	for _, recordName := range config.CF.Names() {
		fmt.Printf("   domain: %s\n", recordName)

		result, err := cf.UpdateRecord(config.CF.Token, config.CF.ZoneID, recordName, *flagIP)
		if err != nil {
			log.Fatalf("❌ CF 更新失败 (%s): %v", recordName, err)
		}

		switch result.Action {
		case cf.ActionSkipped:
			fmt.Printf("   ⏭️  跳过（IP 未变化）：%s\n", result.NewIP)
		case cf.ActionUpdated:
			fmt.Printf("   ✅ 更新成功：%s → %s\n", result.PrevIP, result.NewIP)
		case cf.ActionCreated:
			fmt.Printf("   🆕 新建成功：%s\n", result.NewIP)
		}
		fmt.Printf("   record_id: %s\n\n", result.RecordID)

		// ━━━━━━━━━━ 步骤 3（可选）：发通知 ━━━━━━━━━━
		if config.Notify == "" {
			continue
		}
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("📣 步骤 3: 调 webhook.Do 发通知（演示工具函数用法）")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("   URL: %s\n", config.Notify)
		fmt.Printf("   domain: %s\n\n", recordName)

		payload := map[string]any{
			"domain":  recordName,
			"ip":      *flagIP,
			"prev_ip": result.PrevIP,
			"action":  string(result.Action),
			"record":  result.RecordID,
		}
		body, status, err := webhook.Do("POST", config.Notify, nil, payload)
		if err != nil {
			log.Printf("⚠️  通知失败 (%s): %v", recordName, err)
			continue
		}
		if status >= 400 {
			log.Printf("⚠️  通知返回 %d (%s): %s", status, recordName, string(body))
			continue
		}
		fmt.Printf("✅ 通知发送成功 (status=%d)\n\n", status)
	}
}
