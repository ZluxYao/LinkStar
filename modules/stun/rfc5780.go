package stun

import (
	"fmt"
	"net"
	"time"
)

var rfc5780StunServer = []string{
	"stun.dcalling.de:3478",
	"stun.freeswitch.org:3478",
	"stun.sip.us:3478",
	"stun.sonetel.net:3478",
	"stun.radiojar.com:3478",
	"stun.sonetel.com:3478",

	// 国外
	"stun.nextcloud.com:3478",
	"stun.voipgate.com:3478",
}

type RFC5780Server struct {
	Name    string
	Primary *net.UDPAddr
	Other   *net.UDPAddr
	Delay   time.Duration
}

type MappingBehavior string

const (
	MappingUnknown              MappingBehavior = "unknown"
	MappingEndpointIndependent  MappingBehavior = "EIM"  // 端点无关映射
	MappingAddressDependent     MappingBehavior = "ADM"  // 地址相关映射
	MappingAddressPortDependent MappingBehavior = "APDM" // 地址端口相关映射
)

type FilteringBehavior string

const (
	FilteringUnknown              FilteringBehavior = "unknown"
	FilteringEndpointIndependent  FilteringBehavior = "EIF"
	FilteringAddressDependent     FilteringBehavior = "ADF"
	FilteringAddressPortDependent FilteringBehavior = "APDF"
)

// Protocol 标识一次 NAT 探测使用的传输层协议。
type Protocol string

const (
	ProtoUDP Protocol = "udp"
	ProtoTCP Protocol = "tcp"
)

// NATStatus 汇总一次 NAT 探测的结果，UDP/TCP 共用同一个结构体。
// TCP 场景下 CHANGE-REQUEST 机制不适用（见包注释），Filtering
// 固定为 FilteringUnknown、FilteringErr 固定为 nil —— 不是测试失败，
// 是这个字段在 TCP 下本来就没有意义，跟真正的探测失败要能区分开。
type NATStatus struct {
	Protocol     Protocol
	NatType      NatType
	Mapping      MappingBehavior
	Filtering    FilteringBehavior
	MappingErr   error
	FilteringErr error
}

// NatType nat类型
type NatType string

const (
	OpenInternet   NatType = "公网直连(Open Internet)"
	SymmetricFW    NatType = "公网IP+对称防火墙"
	FullCone       NatType = "NAT1 - 完全圆锥型(Full Cone)"
	RestrictedCone NatType = "NAT2 - 受限圆锥型(Restricted Cone)"
	PortRestricted NatType = "NAT3 - 端口受限圆锥型(Port Restricted Cone)"
	Symmetric      NatType = "NAT4 - 对称型(Symmetric)"
	PooledCGNAT    NatType = "运营商池化NAT(出口IP不固定)"
	Uncertain      NatType = "无法完全确定(按保守估计处理)"
)

// DetectNatUDP 探测本机 UDP 场景下的 NAT Mapping 和 Filtering 行为。
// 只有连服务器都选不出来时才返回顶层 error，
// Mapping/Filtering 各自的失败原因走 NATStatus.MappingErr/FilteringErr。
func DetectNatUDP() (*NATStatus, error) {
	server, err := selectRFC5780Server(rfc5780StunServer)
	if err != nil {
		return nil, fmt.Errorf("选择探测服务器失败: %w", err)
	}

	status := &NATStatus{
		Protocol:  ProtoUDP,
		Mapping:   MappingUnknown,
		Filtering: FilteringUnknown,
	}

	if mapping, err := detectUDPMapping(server); err != nil {
		status.MappingErr = err
	} else {
		status.Mapping = mapping
	}

	if filtering, err := detectUDPFiltering(server); err != nil {
		status.FilteringErr = err
	} else {
		status.Filtering = filtering
	}

	return status, nil
}

// DetectNatTCP 探测本机 TCP 场景下的 NAT Mapping 行为。
// Filtering 在 TCP 下不适用，固定返回 FilteringUnknown，不代表测试失败。
func DetectNatTCP() (*NATStatus, error) {
	server, err := selectRFC5780Server(rfc5780StunServer)
	if err != nil {
		return nil, fmt.Errorf("选择探测服务器失败: %w", err)
	}

	status := &NATStatus{
		Protocol:  ProtoTCP,
		Mapping:   MappingUnknown,
		Filtering: FilteringUnknown, // TCP 下恒为 Unknown，非探测失败
	}

	if mapping, err := detectTCPMapping(server); err != nil {
		status.MappingErr = err
	} else {
		status.Mapping = mapping
	}

	return status, nil
}
