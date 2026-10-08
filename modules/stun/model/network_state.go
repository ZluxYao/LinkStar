package model

type NetworkState struct {
	LocalIP  string `json:"localIP"`  // 本机内网IP（出口网卡上的地址）
	PublicIP string `json:"publicIP"` // 真实公网IP
	Iface    string `json:"iface"`    // 出口网卡名
	Gateway  string `json:"gateway"`  // 出口网关

	NatRouterList []NatRouterInfo `json:"natRouterList"` // 路由信息

	NatStatuUDP *NatDetectResult `json:"natStatuUDP,omitempty"`
	NatStatuTCP *NatDetectResult `json:"natStatuTCP,omitempty"`
}

// 每个Nat路由信息
type NatRouterInfo struct {
	NatLevel uint   `json:"natLevel"` // NAT层级
	LanIp    string `json:"lanIP"`    // LAN口IP地址
	IPType   string `json:"ipType"`   // IP类型：private或cgn
}

type NatDetectResult struct {
	NatType      string `json:"natType"`
	Mapping      string `json:"mapping"`
	Filtering    string `json:"filtering"`
	MappingErr   string `json:"mappingErr,omitempty"`
	FilteringErr string `json:"filteringErr,omitempty"`
}
