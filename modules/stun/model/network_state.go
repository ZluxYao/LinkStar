package model

type NetworkState struct {
	LocalIP  string `json:"localIP"`  // 本机内网IP
	PublicIP string `json:"publicIP"` // 真实公网IP

	NatRouterList []NatRouterInfo `json:"natRouterList"` // 路由信息
}

// 每个Nat路由信息
type NatRouterInfo struct {
	NatLevel uint   `json:"natLevel"` // NAT层级
	LanIp    string `json:"lanIP"`    // LAN口IP地址
}
