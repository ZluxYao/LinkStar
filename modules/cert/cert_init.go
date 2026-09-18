package cert

import (
	"fmt"
	"linkstar/core"
	"linkstar/modules/cert/model"
	"os"

	"github.com/sirupsen/logrus"
)

// InitCert 读配置 → 加载已有证书 → 启动续期/热重载调度器
func InitCert() error {
	cfg, err := ReadConfig()
	if err != nil {
		return fmt.Errorf("读取证书配置失败: %w", err)
	}

	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.mu.Unlock()

	if err := os.MkdirAll(DataDir, 0755); err != nil {
		return fmt.Errorf("创建证书数据目录失败: %w", err)
	}

	core.OnShutdown(func() {
		if err := SaveConfig(Runtime.Snapshot()); err != nil {
			logrus.Error("保存证书配置失败：", err)
		}
	})

	// 先把磁盘上已有的证书全部装载起来，失败只记错不阻塞启动
	for _, c := range cfg.Certificates {
		if !c.Enabled {
			continue
		}
		if err := loadAndCommit(c); err != nil {
			logCertError(c, "启动加载失败", err)
			continue
		}
		logCertInfo(c, fmt.Sprintf("已加载，有效期至 %s", c.NotAfter.Format("2006-01-02")))
	}

	Runtime.Scheduler = NewScheduler()
	Runtime.Scheduler.Start()
	Runtime.Scheduler.Trigger()

	return nil
}

// ReloadCertificate 供 API 在增删改后调用：重新装载单张证书
func ReloadCertificate(c model.Certificate) {
	if !c.Enabled {
		Runtime.Manager.Remove(c.ID)
		return
	}

	// 自签改了域名（NotAfter 被清零）就地重签一张，旧的那张覆盖不到新域名了
	if c.Source == model.SourceSelfSigned && c.NotAfter.IsZero() {
		if err := GenerateSelfSigned(c); err != nil {
			logCertError(c, "重新生成自签证书失败", err)
		}
		return
	}

	// ACME 证书还没签发过时，磁盘上没有 PEM，交给调度器去签
	if err := loadAndCommit(c); err != nil {
		if model.IsACME(c.Source) {
			logCertInfo(c, "尚未签发，已交给调度器处理")
		} else {
			logCertError(c, "重新加载失败", err)
		}
		return
	}
	Runtime.Manager.SetEnabled(c.ID, c.Enabled, c.IsDefault)
}
