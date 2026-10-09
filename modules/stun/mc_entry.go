package stun

import (
	"errors"
	"fmt"
	"linkstar/modules/ddns/dns"
	"linkstar/modules/stun/model"
	"strings"

	"github.com/sirupsen/logrus"
)

// MC 入口：维护 _minecraft._tcp.<域名> 这条 SRV，让它的端口跟着打洞的外部端口走。
//
// SRV 指向的主机就是用户填的那个域名本身，那个域名自己的 A 记录要跟着公网 IP 走——
// 和入口重定向的落地域名是同一件事，复用 ensure / cleanup 那一套（landingRecordEnsurer）。

var srvSyncerFactory func(providerID uint) (dns.SRVRecordProvider, error)

// RegisterSRVSyncerFactory 注入「按服务商 ID 取 SRV 客户端」的工厂，由 app.go 调
func RegisterSRVSyncerFactory(f func(providerID uint) (dns.SRVRecordProvider, error)) {
	srvSyncerFactory = f
}

func buildSRVSyncer(providerID uint) (dns.SRVRecordProvider, error) {
	if srvSyncerFactory == nil {
		return nil, errors.New("MC 入口未初始化")
	}
	if providerID == 0 {
		return nil, errors.New("请先选一个 DNS 服务商")
	}
	return srvSyncerFactory(providerID)
}

// mcSRVName 朋友填 mc.example.com，MC 会去查这条
func mcSRVName(host string) string {
	return "_minecraft._tcp." + strings.Trim(strings.TrimSpace(host), ".")
}

func mcEntryZone(cfg model.MCEntryConfig) string {
	return redirectZone(model.RedirectConfig{ZoneDomain: cfg.ZoneDomain, EntryHost: cfg.Host})
}

// mcEntryReady 开着、域名填了、端口有了
func mcEntryReady(cfg model.MCEntryConfig, port uint16) bool {
	return cfg.Enabled && strings.TrimSpace(cfg.Host) != "" && port != 0
}

// SyncMCEntry 把 SRV 记录指到当前外部端口，并保证域名本身有 DDNS 记录跟着公网 IP 走。
// 返回给人看的一句话；记录没变时 changed 为 false。
func SyncMCEntry(cfg model.MCEntryConfig, port uint16) (msg string, changed bool, err error) {
	if !mcEntryReady(cfg, port) {
		return "", false, errors.New("MC 入口没开、没填域名，或者还没拿到外部端口")
	}
	syncer, err := buildSRVSyncer(cfg.ProviderID)
	if err != nil {
		return "", false, err
	}
	host := strings.Trim(strings.TrimSpace(cfg.Host), ".")
	changed, err = syncer.SyncSRVRecord(mcEntryZone(cfg), mcSRVName(host), host, port)
	if err != nil {
		return "", false, err
	}
	msg = fmt.Sprintf("MC 入口已更新：%s → %s:%d", mcSRVName(host), host, port)

	// 域名本身指不到这台机器，SRV 再对也连不上；补不上不算失败，说出来就行
	if landingRecordEnsurer != nil {
		added, lerr := landingRecordEnsurer(cfg.ProviderID, mcEntryZone(cfg), host)
		switch {
		case lerr != nil:
			msg += fmt.Sprintf("；但 %s 自己的解析没能自动接管：%v", host, lerr)
		case added:
			msg += fmt.Sprintf("；%s 已加进 DDNS，会跟着公网 IP 走", host)
		}
	}
	return msg, changed, nil
}

// SyncMCEntryNow 「立即同步」按钮：用已保存的配置和调度器里的实时端口
func SyncMCEntryNow(deviceID, serviceID uint) (string, error) {
	_, svc := FindDeviceService(deviceID, serviceID)
	if svc == nil {
		return "", errors.New("服务不存在")
	}
	port := LiveExternalPort(deviceID, svc)
	if port == 0 {
		return "", errors.New("还没拿到外部端口，等服务跑起来再同步")
	}
	msg, _, err := SyncMCEntry(svc.MCEntry, port)
	if Runtime.Scheduler != nil {
		Runtime.Scheduler.recordMCEntryFor(deviceID, serviceID, port, err)
	}
	return msg, err
}

// CleanupMCEntry 服务删了 / MC 入口关了，把 LinkStar 建的那条 SRV 收回去。异步，只记日志。
//
// 域名自己的 A 记录交给 CleanupLandingRecord：它会先看还有没有别的服务用着。
func CleanupMCEntry(cfg model.MCEntryConfig) {
	if strings.TrimSpace(cfg.Host) == "" || cfg.ProviderID == 0 {
		return
	}
	go func() {
		syncer, err := buildSRVSyncer(cfg.ProviderID)
		if err != nil {
			return
		}
		removed, err := syncer.RemoveSRVRecord(mcEntryZone(cfg), mcSRVName(cfg.Host))
		if err != nil {
			logrus.WithError(err).Warnf("MC 入口的 SRV 记录没清掉：%s", mcSRVName(cfg.Host))
			return
		}
		if removed {
			logrus.Infof("已删除 MC 入口的 SRV 记录：%s", mcSRVName(cfg.Host))
		}
	}()
}

// MCEntryChanged 保存服务时判断旧的那条 SRV 要不要收：关掉了，或者换了域名 / 服务商
func MCEntryChanged(old, cur model.MCEntryConfig) bool {
	if !old.Enabled || strings.TrimSpace(old.Host) == "" {
		return false
	}
	return !cur.Enabled || !sameHost(old.Host, cur.Host) || old.ProviderID != cur.ProviderID
}
