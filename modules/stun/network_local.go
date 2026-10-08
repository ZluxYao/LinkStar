package stun

import (
	"fmt"
	"time"

	"github.com/pion/stun"
)

type NetworkAddressInfo struct {
	// 基础网络信息
	LocalIP  string `json:"localIP"`  // 本机内网IP
	PublicIP string `json:"publicIP"` // 真实公网IP
}

// 获取网络基本信息
func GetPublicIPInfo(stunServer string) (NetworkAddressInfo, error) {
	var info NetworkAddressInfo

	// 获取本机ip
	LocalIP, err := GetLocalIP()
	if err != nil {
		fmt.Printf("获取本机ip失败：%s \n", err)
	}
	info.LocalIP = LocalIP

	// 获取真实公网ip
	PublicIP, err := GetPublicIP(stunServer)
	if err != nil {
		fmt.Printf("获取真实公网ip失败%s", err)
	}
	info.PublicIP = PublicIP

	return info, nil
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

	// 链接STUN服务器：从出口网卡发，不然开着 TUN 时问到的是代理节点的 IP
	conn, err := CurrentOutboundIface().DialTCP(stunServer, 3*time.Second)
	if err != nil {
		return "", fmt.Errorf("连接STUN服务器失败: %w", err)
	}
	defer conn.Close()

	// 发送STUN请求
	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err := conn.Write(msg.Raw); err != nil {
		return "", fmt.Errorf("发送STUN请求失败%s", err)
	}

	// 读取响应
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("读取响应失败：%s", err)
	}

	// 解析响应
	var response stun.Message
	response.Raw = buf[:n]
	if err = response.Decode(); err != nil {
		return "", fmt.Errorf("解码stun失败%s", err)
	}

	var xorAddr stun.XORMappedAddress
	if err = xorAddr.GetFrom(&response); err != nil {
		return "", fmt.Errorf("获取映射地址失败: %s", err)

	}

	return xorAddr.IP.String(), nil
}
