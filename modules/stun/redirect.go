package stun

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"linkstar/modules/ddns/dns"
	"linkstar/modules/stun/model"

	"github.com/sirupsen/logrus"
)

// RedirectSyncer 把「入口域名 → 当前外网地址」写到 DNS 服务商那边。
//
// 直接用 dns 包那个接口，不再照抄一份：成环的是 modules/ddns（它 import 了
// modules/stun），而 modules/ddns/dns 只认 DNS 协议那点事，谁都不依赖。
// 「按服务商 ID 取到实现」那一步要读 DDNS 配置，所以仍旧由 app.go 注入。
type RedirectSyncer = dns.RedirectRuleProvider

var redirectSyncerFactory func(providerID uint) (RedirectSyncer, error)

// RegisterRedirectSyncerFactory 注入「按服务商 ID 取同步器」的工厂
func RegisterRedirectSyncerFactory(f func(providerID uint) (RedirectSyncer, error)) {
	redirectSyncerFactory = f
}

// landingRecordEnsurer 保证落地域名自己也解析得到这台机器。同样由 app.go 注入。
var landingRecordEnsurer func(providerID uint, zoneDomain, host string) (bool, error)

// RegisterLandingRecordEnsurer 注入「给落地域名补 DDNS 记录」的实现
func RegisterLandingRecordEnsurer(f func(providerID uint, zoneDomain, host string) (bool, error)) {
	landingRecordEnsurer = f
}

// landingRecordReleaser 把当初自动补的那条 DDNS 记录收回去。同样由 app.go 注入。
var landingRecordReleaser func(host string) (bool, error)

// RegisterLandingRecordReleaser 注入「收回自动补的落地域名记录」的实现
func RegisterLandingRecordReleaser(f func(host string) (bool, error)) {
	landingRecordReleaser = f
}

// LandingRecordState 落地域名在 DDNS 那边由哪条记录管着、上次跑成什么样。
//
// 类型定义在这边而不是 ddns 那边：ddns import 了 stun，反过来会成环。
// 由 app.go 从 DDNS 的记录翻译过来。
type LandingRecordState struct {
	// Managed DDNS 里有记录管着这个域名。没有的话公网 IP 一变就得手动去改
	Managed bool `json:"managed"`
	// Name 那条记录的名字
	Name string `json:"name"`
	// LastIP 上次成功写进去的 IP
	LastIP string `json:"lastIP"`
	// Status 上次同步结果：success / failed / pending / skipped
	Status string `json:"status"`
	// Message 上次同步的说明，失败时是原因
	Message string `json:"message"`
	// At 上次同步时间
	At time.Time `json:"at"`
}

// landingRecordInspector 查落地域名归哪条 DDNS 记录管。由 app.go 注入。
var landingRecordInspector func(host string) (LandingRecordState, bool)

// RegisterLandingRecordInspector 注入「落地域名现在归谁管」的查询
func RegisterLandingRecordInspector(f func(host string) (LandingRecordState, bool)) {
	landingRecordInspector = f
}

// ensureLandingRecord 落地域名得有人替它跟着公网 IP 走。
//
// 重定向规则只管「入口域名 → 落地域名:端口」这一跳。落地域名解析到哪台机器，
// 规则管不着：公网 IP 一变，规则这边照样显示同步成功，访问的人却被送到别人家。
// 所以规则写完顺手在 DDNS 里把落地域名也接管上，已经有记录管着就不动。
func ensureLandingRecord(cfg model.RedirectConfig, target string) (host string, added bool, err error) {
	if landingRecordEnsurer == nil {
		return "", false, nil
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", false, nil // 目标地址是自己拼的，解析不了说明别处有更大的问题
	}
	host = u.Hostname()
	if host == "" {
		return "", false, nil
	}
	added, err = landingRecordEnsurer(cfg.ProviderID, redirectZone(cfg), host)
	return host, added, err
}

// ErrRedirectNotConfigured 这个服务没开入口重定向
var ErrRedirectNotConfigured = errors.New("这个服务没有开启入口重定向")

// redirectRuleKey 服务商那边靠什么认出这条规则是 LinkStar 写的。
//
// 用 ID 不用服务名：服务一改名，名字就对不上了，上一条规则会变成没人认领的孤儿，
// 继续把访问的人送到一个早就没了的端口。ID 不会变，改名不影响。
func redirectRuleKey(deviceID, serviceID uint) string {
	return fmt.Sprintf("linkstar:%d-%d", deviceID, serviceID)
}

// redirectRuleLabel 规则名后面跟的那半截，只给人看。
//
// 光有 linkstar:1-2，用户去 Cloudflare 后台看见一排规则根本分不出哪条是哪个服务。
// 认领仍旧只看前面的 key，所以这里改名不会把旧规则变成孤儿。
func redirectRuleLabel(svc *model.Service) string {
	if svc == nil {
		return ""
	}
	return strings.TrimSpace(svc.Name)
}

// redirectZone 这条入口属于哪个主域名。
//
// 留空时取 EntryHost 的后两段：linkstar.example.com → example.com。
// 后两段猜不对的（example.co.uk 这种）在配置里把主域名填上。
func redirectZone(cfg model.RedirectConfig) string {
	if z := strings.TrimSpace(cfg.ZoneDomain); z != "" {
		return z
	}
	host := strings.Trim(strings.TrimSpace(cfg.EntryHost), ".")
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

func buildRedirectSyncer(providerID uint) (RedirectSyncer, error) {
	if redirectSyncerFactory == nil {
		return nil, errors.New("入口重定向未初始化")
	}
	if providerID == 0 {
		return nil, errors.New("请先选一个 DNS 服务商")
	}
	return redirectSyncerFactory(providerID)
}

// checkRedirect 取出服务和它的入口重定向配置，顺带把能当场判掉的错先判掉
func checkRedirect(deviceID, serviceID uint) (*model.Service, model.RedirectConfig, error) {
	_, svc := FindDeviceService(deviceID, serviceID)
	if svc == nil {
		return nil, model.RedirectConfig{}, errors.New("服务不存在")
	}
	cfg := svc.Redirect
	if !cfg.Enabled {
		return svc, cfg, ErrRedirectNotConfigured
	}
	if strings.TrimSpace(cfg.EntryHost) == "" {
		return svc, cfg, errors.New("入口域名不能为空")
	}
	return svc, cfg, nil
}

// RedirectSyncResult 一次同步实际做成了哪几件事，供界面把话说清楚
type RedirectSyncResult struct {
	// Target 入口域名现在跳去哪
	Target string
	// KeepPath 服务商有没有接受「保留原始路径」的写法；
	// 被拒时会自动降级成静态目标，此时从子路径进来会落到根路径
	KeepPath bool
	// EntryWarn 入口域名那条 DNS 记录没能确认。规则是写进去了，
	// 但记录要是真没有，访问入口域名就是「域名不存在」——不说出来就等于报了个假的成功
	EntryWarn string
	// LandingHost 落地域名
	LandingHost string
	// LandingAdded 这次顺手在 DDNS 里给落地域名建了记录
	LandingAdded bool
	// LandingWarn 落地域名还没人维护，也没能自动补上，原因在这
	LandingWarn string
}

// SyncRedirect 把服务当前的外网地址写到入口域名的重定向规则上，
// 并保证落地域名自己也有 DDNS 记录跟着公网 IP 走。
func SyncRedirect(deviceID, serviceID uint) (RedirectSyncResult, error) {
	var out RedirectSyncResult

	svc, cfg, err := checkRedirect(deviceID, serviceID)
	if err != nil {
		return out, err
	}

	// 和首页链接、307 索引用的是同一套推导：有域名用域名，没有回落公网 IP，
	// 端口取调度器的实时值，终结 TLS 时是 https。不在这里重写第二份。
	out.Target = ServiceEndpoint(deviceID, svc).URL()
	if out.Target == "" {
		return out, errors.New("还没拿到外网端口，等服务跑起来再同步")
	}

	syncer, err := buildRedirectSyncer(cfg.ProviderID)
	if err != nil {
		return out, err
	}
	out.KeepPath, out.EntryWarn, err = syncer.SyncRedirectRule(
		redirectZone(cfg),
		redirectRuleKey(deviceID, serviceID),
		redirectRuleLabel(svc),
		strings.TrimSpace(cfg.EntryHost),
		out.Target,
	)
	if err != nil {
		return out, err
	}

	// 落地域名补不上不算同步失败：规则已经写进去了，这条只是少了个自动维护的人
	host, added, lerr := ensureLandingRecord(cfg, out.Target)
	out.LandingHost = host
	out.LandingAdded = added
	if lerr != nil {
		out.LandingWarn = lerr.Error()
	}
	return out, nil
}

// RedirectInspection 入口重定向靠的那两条解析记录，现在各是什么样。
//
// 这功能能不能用，取决于两条记录同时到位，而它们的要求正好相反：
// 入口域名要开橙云（不开，重定向规则轮不到执行），落地域名要关橙云
// （开了，Cloudflare 的代理不转发 28469 这种端口）。两条都不报错，
// 错了也只体现为「访问不了」，所以得把真实状态查出来摆在一起看。
type RedirectInspection struct {
	// Entry 入口域名那条，现场去服务商查的
	Entry dns.EntryRecordState `json:"entry"`
	// LandingHost 落地域名，也就是 307 之后真正要连的那台机器
	LandingHost string `json:"landingHost"`
	// Landing 落地域名归哪条 DDNS 记录管
	Landing LandingRecordState `json:"landing"`
}

// InspectRedirect 查这两条记录现在的样子，只读，不改任何东西。
func InspectRedirect(deviceID, serviceID uint) (RedirectInspection, error) {
	var out RedirectInspection

	svc, cfg, err := checkRedirect(deviceID, serviceID)
	if err != nil {
		return out, err
	}

	// 落地域名按和同步时同一套推导来：有域名用域名，没有回落公网 IP。
	// 端口还没拿到时 URL() 是空的，这时退回域名本身——记录归谁管和端口无关。
	out.LandingHost = strings.TrimSpace(svc.Domain)
	if t := ServiceEndpoint(deviceID, svc).URL(); t != "" {
		if u, perr := url.Parse(t); perr == nil && u.Hostname() != "" {
			out.LandingHost = u.Hostname()
		}
	}
	if landingRecordInspector != nil && out.LandingHost != "" {
		out.Landing, _ = landingRecordInspector(out.LandingHost)
	}

	syncer, err := buildRedirectSyncer(cfg.ProviderID)
	if err != nil {
		return out, err
	}
	out.Entry, err = syncer.InspectEntryRecord(redirectZone(cfg), strings.TrimSpace(cfg.EntryHost))
	if err != nil {
		return out, err
	}
	return out, nil
}

// RemoveRedirect 把服务商那边 LinkStar 自己写的那条规则删掉，别人的规则不动。
//
// 不看 Enabled：关掉开关正是要删规则的时候，这时候再拦一道就删不掉了。
func RemoveRedirect(deviceID, serviceID uint) error {
	_, svc := FindDeviceService(deviceID, serviceID)
	if svc == nil {
		return errors.New("服务不存在")
	}
	return RemoveRedirectConfig(svc.Redirect, deviceID, serviceID)
}

// RemoveRedirectConfig 同上，但配置由调用方传进来。
//
// 删服务的时候用这个：服务已经从配置里摘掉了，再按 ID 去查就查不到了，
// 而这条规则还留在服务商那边，继续把访问的人送到一个早就没了的端口。
func RemoveRedirectConfig(cfg model.RedirectConfig, deviceID, serviceID uint) error {
	if strings.TrimSpace(cfg.EntryHost) == "" {
		return errors.New("入口域名不能为空")
	}
	syncer, err := buildRedirectSyncer(cfg.ProviderID)
	if err != nil {
		return err
	}
	return syncer.RemoveRedirectRule(redirectZone(cfg), redirectRuleKey(deviceID, serviceID))
}

// CleanupLandingRecord 服务被删掉时，把当初替它的落地域名自动补的那条 DDNS 记录也收回去。
//
// 必须在服务已经从配置里摘掉、并且存过盘之后再调：这里要靠「还有没有别的服务
// 用着这个域名」来决定删不删，配置没更新的话查出来的就是删之前的样子。
//
// 同一个域名常常挂着好几个服务（一台机器上的 NAS、PVE、Alist 都用同一个域名），
// 删掉其中一个就把记录删了，剩下那几个的公网 IP 就再没人维护——照样不报错，
// 只是过几天家宽 IP 一变，全都访问不了。
func CleanupLandingRecord(host string) {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" || landingRecordReleaser == nil {
		return
	}
	if domainInUse(host) {
		return
	}
	removed, err := landingRecordReleaser(host)
	if err != nil {
		logrus.WithError(err).Warnf("服务已删除，但落地域名 %s 的解析记录没清掉", host)
		return
	}
	if removed {
		logrus.Infof("服务已删除，顺带清掉自动添加的解析记录：%s", host)
	}
}

// domainInUse 还有没有别的服务用着这个域名
func domainInUse(host string) bool {
	for _, dev := range Runtime.Config.Devices {
		for _, svc := range dev.Services {
			if sameHost(svc.Domain, host) {
				return true
			}
		}
	}
	return false
}

// CleanupRedirect 服务被删掉时顺手清掉它在服务商那边的规则。
//
// 异步 + 只记日志：删服务是本地操作，不该因为打不通 Cloudflare 就失败。
func CleanupRedirect(cfg model.RedirectConfig, deviceID, serviceID uint) {
	if !cfg.Enabled || strings.TrimSpace(cfg.EntryHost) == "" {
		return
	}
	// 入口域名那条记录删不删，得看还有没有别的服务用着同一个入口。
	// 这个判断必须现在做完：下面是甩出去的 goroutine，等它跑起来的时候，
	// 配置可能已经被别的请求改过了。
	dropEntryRecord := !entryHostInUse(cfg.EntryHost)

	go cleanupRedirectNow(cfg, deviceID, serviceID, dropEntryRecord)
}

// cleanupRedirectNow 真正去服务商那边收拾的那一段，同步跑。
//
// 单独拆出来只为一件事：这里的先后顺序有讲究，得能直接测。
func cleanupRedirectNow(cfg model.RedirectConfig, deviceID, serviceID uint, dropEntryRecord bool) {
	if err := RemoveRedirectConfig(cfg, deviceID, serviceID); err != nil {
		logrus.WithError(err).Warnf("服务已删除，但入口重定向规则没清掉：%s", redirectRuleKey(deviceID, serviceID))
		return // 规则还在就别删记录：少了那条记录，规则连执行的机会都没有
	}
	if !dropEntryRecord {
		return
	}
	removed, err := removeEntryRecord(cfg)
	if err != nil {
		logrus.WithError(err).Warnf("服务已删除，但入口域名 %s 的记录没清掉", cfg.EntryHost)
		return
	}
	if removed {
		logrus.Infof("服务已删除，顺带清掉入口域名的记录：%s", cfg.EntryHost)
	}
}

// removeEntryRecord 把当初替入口域名建的那条占位记录收回去。
//
// 规则没了这条记录就纯属多余：它指向 192.0.2.1，访问的人看到的是 Cloudflare 的错误页。
func removeEntryRecord(cfg model.RedirectConfig) (bool, error) {
	syncer, err := buildRedirectSyncer(cfg.ProviderID)
	if err != nil {
		return false, err
	}
	return syncer.RemoveEntryRecord(redirectZone(cfg), strings.TrimSpace(cfg.EntryHost))
}

// entryHostInUse 还有没有别的服务开着重定向、用着这个入口域名。
//
// 和 domainInUse 一样，得在服务已经从配置里摘掉、存过盘之后再问。
//
// 只算开着的：关掉重定向的服务在服务商那边本来就没有规则，那条记录对它没用；
// 以后真要再打开，同步时会自己把记录补回来。
func entryHostInUse(host string) bool {
	if strings.TrimSpace(host) == "" {
		return false
	}
	for _, dev := range Runtime.Config.Devices {
		for _, svc := range dev.Services {
			if svc.Redirect.Enabled && sameHost(svc.Redirect.EntryHost, host) {
				return true
			}
		}
	}
	return false
}

// sameHost 两个域名算不算同一个：前后空白、末尾的点、大小写都不算数
func sameHost(a, b string) bool {
	return strings.EqualFold(
		strings.TrimSuffix(strings.TrimSpace(a), "."),
		strings.TrimSuffix(strings.TrimSpace(b), "."),
	)
}
