package stun

import (
	"linkstar/modules/stun/model"
	"net"
	"time"

	"github.com/sirupsen/logrus"
)

// 预编译CIDR
var (
	_, private10, _  = net.ParseCIDR("10.0.0.0/8")
	_, private172, _ = net.ParseCIDR("172.16.0.0/12")
	_, private192, _ = net.ParseCIDR("192.168.0.0/16")
	_, cgnRange, _   = net.ParseCIDR("100.64.0.0/10")
)

// IP类型常量
const (
	IPTypePrivate = "private"
	IPTypeCGN     = "cgn"
	IPTypePublic  = "public"
)

func GetNatRouterList() ([]model.NatRouterInfo, error) {
	startTime := time.Now()

	logrus.Info("实时扫描网络层级")
	// 从出口网卡发探测包：开着 TUN 时不指定出口，第一跳就是 TUN，后面全是 *
	natChain, err := CurrentOutboundIface().traceHops("114.114.114.114", 10)
	if err != nil {
		return nil, err
	}

	endTime := time.Now()
	logrus.Infof("扫描耗时%vs", endTime.Sub(startTime))

	return natChain, nil
}

// collectHops 每跳递增 TTL 调一次 probe，记下私网 / CGN 路由，碰到 CGN 或公网就停。
// probe 返回这一跳回包的地址（空表示没回）以及是否已经到了目标；
// 怎么发包各平台不一样，见 outbound_iface_<os>.go 的 traceHops。
func collectHops(target string, maxHops int, probe func(ttl int) (hop string, reached bool, err error)) ([]model.NatRouterInfo, error) {
	var natChain []model.NatRouterInfo
	level := uint(0)

	for ttl := 1; ttl <= maxHops; ttl++ {
		// 运营商设备回 TTL 超时时有时无，没回就再问一次
		var ip string
		var reached bool
		for range 2 {
			var err error
			ip, reached, err = probe(ttl)
			if err != nil {
				return natChain, err
			}
			if ip != "" {
				break
			}
		}
		if reached || ip == target {
			break
		}
		if ip == "" {
			continue // 不回 ICMP 的路由器，跳过这一跳接着往下
		}

		ipType := classifyIP(ip)

		if ipType != IPTypePublic {
			level++
			natChain = append(natChain, model.NatRouterInfo{
				NatLevel: level,
				LanIp:    ip,
				IPType:   ipType,
			})
			if ipType == IPTypeCGN {
				logrus.Infof("探测到cgn出口: %s，终止扫描", ip)
				break
			}
		} else {
			logrus.Infof("探测到公网出口: %s，终止扫描", ip)
			break
		}
	}

	return natChain, nil
}

// classifyIP IP分类
func classifyIP(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return IPTypePrivate
	}
	if cgnRange.Contains(ip) {
		return IPTypeCGN
	}
	if private10.Contains(ip) || private172.Contains(ip) || private192.Contains(ip) {
		return IPTypePrivate
	}
	return IPTypePublic
}
