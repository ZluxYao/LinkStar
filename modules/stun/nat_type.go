package stun

import (
	"errors"
	"fmt"
	"linkstar/modules/stun/model"
	"sync"

	"github.com/sirupsen/logrus"
)

type NatType string

var natDetectionMu sync.Mutex

// GetNatDetectionResults 返回最近一次检测结果的副本。
// 检测进行中时会等待本轮结束，避免 API 读取到一半更新的状态。
func GetNatDetectionResults() (udp, tcp *model.NatDetectResult) {
	natDetectionMu.Lock()
	defer natDetectionMu.Unlock()

	if Runtime.Network.NatStatuUDP != nil {
		value := *Runtime.Network.NatStatuUDP
		udp = &value
	}
	if Runtime.Network.NatStatuTCP != nil {
		value := *Runtime.Network.NatStatuTCP
		tcp = &value
	}
	return udp, tcp
}

const (
	OpenInternet   NatType = "公网直连(Open Internet)"
	SymmetricFW    NatType = "公网IP+对称防火墙"
	FullCone       NatType = "NAT1 - 完全圆锥型(Full Cone)"
	RestrictedCone NatType = "NAT2 - 受限圆锥型(Restricted Cone)"
	PortRestricted NatType = "NAT3 - 端口受限圆锥型(Port Restricted Cone)"
	Symmetric      NatType = "NAT4 - 对称型(Symmetric)"
	Blocked        NatType = "探测被阻断(无响应，可能防火墙拦截UDP/TCP)"
	Uncertain      NatType = "无法完全确定"
)

// DetectAndClassifyNat 探测并分类UDP/TCP的NAT类型，同步写回Runtime.Network，
// 并直接把结果返回给调用方，不用调用方再回头去读Runtime。
//
// UDP/TCP两边独立探测、独立出错，一个失败不影响另一个继续跑。
// 顶层err只在两者都失败、或基础网络数据都拿不到时非nil；
// 单边失败会体现在对应NatDetectResult里，而不是掩盖成旧数据。
func DetectAndClassifyNat() (udpResult, tcpResult *model.NatDetectResult, err error) {
	natDetectionMu.Lock()
	defer natDetectionMu.Unlock()

	localIP, publicIP, e := getNetworkAddressData()
	if e != nil {
		return nil, nil, fmt.Errorf("网络基础数据未就绪: %w", e)
	}

	var errs []error

	udpStatus, udpErr := DetectNatUDP()
	if udpErr != nil {
		logrus.Warnf("UDP NAT探测失败: %v", udpErr)
		udpResult = &model.NatDetectResult{
			NatType:    string(Blocked),
			MappingErr: udpErr.Error(),
		}
		errs = append(errs, fmt.Errorf("UDP探测: %w", udpErr))
	} else {
		udpResult = toNatDetectResult(udpStatus, ClassifyUDPNatType(udpStatus, localIP, publicIP))
	}
	Runtime.Network.NatStatuUDP = udpResult

	tcpStatus, tcpErr := DetectNatTCP()
	if tcpErr != nil {
		logrus.Warnf("TCP NAT探测失败: %v", tcpErr)
		tcpResult = &model.NatDetectResult{
			NatType:    string(Blocked),
			MappingErr: tcpErr.Error(),
		}
		errs = append(errs, fmt.Errorf("TCP探测: %w", tcpErr))
	} else {
		tcpResult = toNatDetectResult(tcpStatus, ClassifyTCPNatType(tcpStatus, localIP, publicIP))
	}
	Runtime.Network.NatStatuTCP = tcpResult

	if len(errs) > 0 {
		err = errors.Join(errs...)
	}
	return udpResult, tcpResult, err
}

// ClassifyUDPNatType 根据UDP Mapping/Filtering探测结果，结合本地IP和公网IP
// 判定NAT类型。
func ClassifyUDPNatType(status *NATStatus, localIP, publicIP string) NatType {
	if status == nil {
		return Uncertain
	}

	if status.MappingErr != nil && status.FilteringErr != nil {
		return Blocked
	}

	isPublicDirect := localIP != "" && publicIP != "" && localIP == publicIP

	switch status.Mapping {
	case MappingEndpointIndependent:
		if isPublicDirect {
			switch {
			case status.FilteringErr != nil:
				return Uncertain
			case status.Filtering == FilteringEndpointIndependent:
				return OpenInternet
			default:
				return SymmetricFW
			}
		}
		switch status.Filtering {
		case FilteringEndpointIndependent:
			return FullCone
		case FilteringAddressDependent:
			return RestrictedCone
		case FilteringAddressPortDependent:
			return PortRestricted
		default:
			return Uncertain
		}

	case MappingAddressDependent, MappingAddressPortDependent:
		return Symmetric

	default:
		return Uncertain
	}
}

// ClassifyTCPNatType TCP下没有Filtering探测(CHANGE-REQUEST不适用)，
// EIM无法继续区分圆锥型子类型，按最保守的端口受限圆锥型处理。
func ClassifyTCPNatType(status *NATStatus, localIP, publicIP string) NatType {
	if status == nil {
		return Uncertain
	}
	if status.MappingErr != nil {
		return Blocked
	}

	isPublicDirect := localIP != "" && publicIP != "" && localIP == publicIP

	switch status.Mapping {
	case MappingEndpointIndependent:
		if isPublicDirect {
			return OpenInternet
		}
		return PortRestricted

	case MappingAddressDependent, MappingAddressPortDependent:
		return Symmetric

	default:
		return Uncertain
	}
}

// getNetworkAddressData 返回分类所需的本机IP和公网IP。
// Runtime中已有完整数据时直接返回；数据缺失时主动获取并写回Runtime。
func getNetworkAddressData() (localIP, publicIP string, err error) {
	localIP = Runtime.Network.LocalIP
	publicIP = Runtime.Network.PublicIP
	if localIP != "" && publicIP != "" {
		return localIP, publicIP, nil
	}

	var stunServer string
	if Runtime.STUNService != nil {
		// 从运行时 STUN 服务获取当前最优服务器
		stunServer, err = Runtime.STUNService.GetBestSTUNServer()
		if err != nil || stunServer == "" {
			stunServer, err = Runtime.STUNService.GetBackupSTUNServer()
			if err != nil {
				return "", "", fmt.Errorf("获取STUN服务器失败: %w", err)
			}
		}
	} else {
		// 从RFC 5780候选列表选择STUN服务器
		server, selectErr := selectRFC5780Server(rfc5780StunServer)
		if selectErr != nil {
			return "", "", fmt.Errorf("从RFC 5780候选列表选择STUN服务器失败: %w", selectErr)
		}
		stunServer = server.Name
	}
	if !updateNetworkAddress(stunServer) {
		return "", "", fmt.Errorf("获取本机网络地址失败(server=%s)", stunServer)
	}

	localIP = Runtime.Network.LocalIP
	publicIP = Runtime.Network.PublicIP
	if localIP == "" || publicIP == "" {
		return "", "", fmt.Errorf("获取到的网络地址不完整(localIP=%q, publicIP=%q)", localIP, publicIP)
	}
	return localIP, publicIP, nil
}

func toNatDetectResult(s *NATStatus, natType NatType) *model.NatDetectResult {
	r := &model.NatDetectResult{
		Mapping:   string(s.Mapping),
		Filtering: string(s.Filtering),
		NatType:   string(natType),
	}
	if s.MappingErr != nil {
		r.MappingErr = s.MappingErr.Error()
	}
	if s.FilteringErr != nil {
		r.FilteringErr = s.FilteringErr.Error()
	}
	return r
}
