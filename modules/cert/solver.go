package cert

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"linkstar/modules/cert/model"

	"github.com/sirupsen/logrus"
)

// TXTWriter DNS-01 需要的最小能力。
//
// 这里刻意不直接依赖 modules/ddns：ddns 已经 import 了 modules/stun，
// 而 modules/stun 要 import 本包终结 TLS，直接依赖会成环。
// 由 app.go（同时 import 两边）在启动时把实现注入进来。
type TXTWriter interface {
	AddTXTRecord(domain, fqdn, value string) error
	RemoveTXTRecord(domain, fqdn, value string) error
}

// ErrDNSSolverUnavailable 找不到可用的 DNS-01 解决器
var ErrDNSSolverUnavailable = errors.New("DNS-01 不可用")

var dnsSolverFactory func(providerID uint) (TXTWriter, error)

// RegisterDNSSolverFactory 由 app.go 在启动时调用，把 DDNS 服务商接进来。
// 工厂内部延迟到签发时才真正查 ddns，因此不要求 ddns 已初始化完成。
func RegisterDNSSolverFactory(f func(providerID uint) (TXTWriter, error)) {
	dnsSolverFactory = f
}

// resolveDNSSolver 取某个 DDNS 服务商对应的 TXT 写入器
func resolveDNSSolver(providerID uint) (TXTWriter, error) {
	if dnsSolverFactory == nil {
		return nil, fmt.Errorf("%w: DNS 服务商尚未接入", ErrDNSSolverUnavailable)
	}
	if providerID == 0 {
		return nil, fmt.Errorf("%w: 未选择 DNS 服务商", ErrDNSSolverUnavailable)
	}
	w, err := dnsSolverFactory(providerID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDNSSolverUnavailable, err)
	}
	return w, nil
}

const (
	propagationTimeout  = 3 * time.Minute
	propagationInterval = 5 * time.Second
)

// waitTXTPropagation 直接向权威 NS 查询 TXT，确认记录已生效后才让 CA 来验。
//
// 不能查本地 resolver：本地有缓存，既可能查到过期的否定结果（假阴），
// 也可能因为上游缓存而在记录实际未生效时返回旧值（假阳）。
func waitTXTPropagation(ctx context.Context, rootDomain, fqdn, value string) error {
	resolvers, err := authoritativeResolvers(ctx, rootDomain)
	if err != nil || len(resolvers) == 0 {
		// 拿不到权威 NS 就退化成固定等待，交给 CA 自己重试
		logrus.Warnf("[cert] 无法获取 %s 的权威 NS（%v），改为固定等待 30s", rootDomain, err)
		select {
		case <-time.After(30 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	deadline := time.Now().Add(propagationTimeout)
	for {
		allSeen := true
		for _, r := range resolvers {
			if !txtSeen(ctx, r, fqdn, value) {
				allSeen = false
				break
			}
		}
		if allSeen {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("等待 TXT 记录 %s 生效超时（%s）", fqdn, propagationTimeout)
		}

		select {
		case <-time.After(propagationInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// authoritativeResolvers 为域名的每个权威 NS 构造一个定向 resolver
func authoritativeResolvers(ctx context.Context, rootDomain string) ([]*net.Resolver, error) {
	nss, err := net.DefaultResolver.LookupNS(ctx, rootDomain)
	if err != nil {
		return nil, err
	}

	out := make([]*net.Resolver, 0, len(nss))
	for _, ns := range nss {
		addr := net.JoinHostPort(strings.TrimSuffix(ns.Host, "."), "53")
		out = append(out, &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, network, addr)
			},
		})
	}
	return out, nil
}

// txtSeen 该 resolver 上是否已能查到目标 TXT 值
func txtSeen(ctx context.Context, r *net.Resolver, fqdn, value string) bool {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	records, err := r.LookupTXT(qctx, fqdn)
	if err != nil {
		return false
	}
	for _, rec := range records {
		if strings.TrimSpace(rec) == value {
			return true
		}
	}
	return false
}

// rootDomainOf 从 challenge 域名推断注册域名，用于调 provider 的 zone 查询。
// 通配符前缀先剥掉；provider 侧（如 Cloudflare 的 getZones）会做 zone 匹配，
// 这里给出的是「尽力而为」的候选，实在推断不出就原样返回。
func rootDomainOf(identifier string, configured []string) string {
	name := strings.ToLower(strings.TrimPrefix(strings.TrimSuffix(identifier, "."), "*."))

	// 优先用用户配置里最短的那个域名（通常就是注册域名）
	best := ""
	for _, d := range configured {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSuffix(d, "."), "*."))
		if d == "" || !strings.HasSuffix(name, d) {
			continue
		}
		if best == "" || len(d) < len(best) {
			best = d
		}
	}
	if best != "" {
		return best
	}
	return name
}

// challengeFQDN DNS-01 的 TXT 记录名
func challengeFQDN(identifier string) string {
	return "_acme-challenge." + strings.TrimPrefix(strings.TrimSuffix(identifier, "."), "*.")
}

func logCertInfo(c model.Certificate, msg string) {
	logrus.Infof("[cert] %s(id=%d): %s", c.Name, c.ID, msg)
}

func logCertError(c model.Certificate, msg string, err error) {
	logrus.Errorf("[cert] %s(id=%d): %s: %v", c.Name, c.ID, msg, err)
}
