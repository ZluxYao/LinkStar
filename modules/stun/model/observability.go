package model

import "time"

// ObservabilitySnapshot 是管理后台网络观测页使用的运行时快照。
// 这些数据只存在于内存中，不写入 STUN 配置文件。
type ObservabilitySnapshot struct {
	SampledAt time.Time          `json:"sampledAt"`
	Network   NetworkObservation `json:"network"`
	NAT       NATObservation     `json:"nat"`
	Traffic   TrafficObservation `json:"traffic"`
}

// NetworkObservation 是当前实际使用的出口网络信息。
type NetworkObservation struct {
	LocalIP  string `json:"localIP"`
	PublicIP string `json:"publicIP"`
	Iface    string `json:"iface"`
	Gateway  string `json:"gateway"`
	BestSTUN string `json:"bestStun"`
}

// NATObservation 汇总 NAT 拓扑、行为探测和 LinkStar 自己管理的端口资源。
type NATObservation struct {
	Routers         []NatRouterInfo  `json:"routers"`
	UDP             *NatDetectResult `json:"udp"`
	TCP             *NatDetectResult `json:"tcp"`
	RouterUpdatedAt time.Time        `json:"routerUpdatedAt,omitempty"`
	DetectedAt      time.Time        `json:"detectedAt,omitempty"`
	Resources       []NatResource    `json:"resources"`
}

// NatResource 是一个穿透服务当前对外暴露的运行时资源。
// 它描述 LinkStar 自己知道的端口，不代表路由器完整 NAT 表。
type NatResource struct {
	Key               string    `json:"key"`
	DeviceName        string    `json:"deviceName"`
	ServiceName       string    `json:"serviceName"`
	Protocol          string    `json:"protocol"`
	LocalIP           string    `json:"localIP"`
	LocalPort         uint16    `json:"localPort"`
	ExternalIP        string    `json:"externalIP"`
	ExternalPort      uint16    `json:"externalPort"`
	Source            string    `json:"source"`
	Status            string    `json:"status"`
	ActiveConnections int       `json:"activeConnections"`
	LastError         string    `json:"lastError,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt,omitempty"`
}

// TrafficObservation 是 LinkStar 转发面的累计流量和当前速率。
type TrafficObservation struct {
	InBytes           uint64           `json:"inBytes"`
	OutBytes          uint64           `json:"outBytes"`
	InBytesPerSecond  uint64           `json:"inBytesPerSecond"`
	OutBytesPerSecond uint64           `json:"outBytesPerSecond"`
	ActiveConnections int              `json:"activeConnections"`
	Services          []ServiceTraffic `json:"services"`
}

// ServiceTraffic 是单个穿透服务的流量统计。
type ServiceTraffic struct {
	Key               string `json:"key"`
	DeviceName        string `json:"deviceName"`
	ServiceName       string `json:"serviceName"`
	Protocol          string `json:"protocol"`
	InBytes           uint64 `json:"inBytes"`
	OutBytes          uint64 `json:"outBytes"`
	InBytesPerSecond  uint64 `json:"inBytesPerSecond"`
	OutBytesPerSecond uint64 `json:"outBytesPerSecond"`
	ActiveConnections int    `json:"activeConnections"`
}
