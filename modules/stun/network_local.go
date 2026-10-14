package stun

import (
	"fmt"
	"net"
	"strings"

	"github.com/sirupsen/logrus"
)

type NetworkAddressInfo struct {
	// 基础网络信息
	LocalIP  string `json:"localIP"`  // 本机内网IP
	PublicIP string `json:"publicIP"` // 真实公网IP
}

// 获取网络基本信息
func GetPublicIPInfo(stunServer string) (NetworkAddressInfo, error) {
	var info NetworkAddressInfo

	localIP, err := GetLocalIP()
	if err != nil {
		logrus.Warnf("获取本机 IP 失败: %v", err)
	} else {
		info.LocalIP = localIP
	}

	publicIP, err := GetPublicIP(stunServer)
	if err != nil {
		return info, fmt.Errorf("获取公网 IP 失败: %w", err)
	}
	if err := checkPublicIP(publicIP); err != nil {
		return info, err
	}
	info.PublicIP = publicIP
	return info, nil
}

// checkPublicIP 空地址、私网、回环、链路本地和 CGNAT 都不能写进公网 IP。
// 以前只拦「10.」前缀，192.168、172.16 和 100.64 会照样推进 DDNS。
func checkPublicIP(ip string) error {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil || isNonPublicIP(parsed) {
		return fmt.Errorf("STUN 返回的 %q 不是可用的公网地址", ip)
	}
	return nil
}

// 获取本机ip：出口网卡上的那个地址，怎么挑见 outbound_iface.go。
// 原来按名字过滤虚拟网卡取第一张，碰上 Mihomo 的 Meta 网卡、VMware 网卡就取错。
func GetLocalIP() (string, error) {
	o := CurrentOutboundIface()
	if o.LocalIP == "" {
		return "", errNoOutboundIface
	}
	return o.LocalIP, nil
}

// 获取公网ip
func GetPublicIP(stunServer string) (string, error) {

	// 从出口网卡发，不然开着 TUN 时问到的是代理节点的 IP。
	// 和选服务器一样：TCP Binding 完不成就再试 UDP，超时在写出请求之前就设好。
	ip, err := dialSTUNBinding(stunServer)
	if err != nil {
		return "", fmt.Errorf("连接STUN服务器失败: %w", err)
	}
	return ip.String(), nil
}
