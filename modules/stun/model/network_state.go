package model

type NetworkState struct {
	LocalIP  string `json:"localIP"`  // 本机内网IP
	PublicIP string `json:"publicIP"` // 真实公网IP

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
