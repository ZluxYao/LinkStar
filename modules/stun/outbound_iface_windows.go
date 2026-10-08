//go:build windows

package stun

import (
	"encoding/binary"
	"fmt"
	"linkstar/modules/stun/model"
	"math/bits"
	"net"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// listIfaces 已连接、有 IPv4 的网卡。能当出口的（有 MAC + 有 IPv4 网关）标 candidate，
// 跃点数小的优先：
//
//   - 没有 MAC 的是代理 TUN（Wintun）、EasyTier、WireGuard 这类隧道；
//   - 没有网关的是 VMware / Hyper-V 的内部网卡；
//   - 有线和 Wi-Fi 都连着时，跃点数小的就是 Windows 实际走的那张。
//
// PPP 拨号（直接在电脑上拨 PPPoE）同样没有 MAC，只在一张正常网卡都没有时才当候选。
func listIfaces() ([]OutboundIface, error) {
	size := uint32(15000)
	var buf []byte
	var head *windows.IpAdapterAddresses
	for tries := 0; ; tries++ {
		buf = make([]byte, size)
		head = (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_INCLUDE_GATEWAYS, 0, head, &size)
		if err == nil {
			break
		}
		// 缓冲区不够时系统会把需要的大小写回 size
		if err != windows.ERROR_BUFFER_OVERFLOW || tries >= 3 {
			return nil, fmt.Errorf("读取网卡列表失败: %w", err)
		}
	}
	defer runtime.KeepAlive(buf) // 链表节点都在 buf 里

	var list []OutboundIface
	var ppp []int
	for a := head; a != nil; a = a.Next {
		if a.OperStatus != windows.IfOperStatusUp || a.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK {
			continue
		}
		o := OutboundIface{Name: windows.UTF16PtrToString(a.FriendlyName), Index: int(a.IfIndex), metric: a.Ipv4Metric}
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			if ip := u.Address.IP().To4(); ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				o.LocalIP, o.PrefixLen = ip.String(), int(u.OnLinkPrefixLength)
				break
			}
		}
		for g := a.FirstGatewayAddress; g != nil; g = g.Next {
			if ip := g.Address.IP().To4(); ip != nil && !ip.IsUnspecified() {
				o.Gateway = ip.String()
				break
			}
		}
		if o.LocalIP == "" {
			continue
		}

		isPPP := a.IfType == windows.IF_TYPE_PPP
		o.Tunnel = a.PhysicalAddressLength == 0 && !isPPP
		o.candidate = a.PhysicalAddressLength > 0 && o.Gateway != ""
		if isPPP {
			ppp = append(ppp, len(list))
		}
		list = append(list, o)
	}

	if _, err := pickAutoIface(list); err != nil {
		for _, i := range ppp {
			list[i].candidate = true
		}
	}
	return list, nil
}

// bindToIface IP_UNICAST_IF：指定单播从哪张网卡出去，值是网络字节序的网卡序号
func (o OutboundIface) bindToIface(fd uintptr) {
	const ipUnicastIF = 31
	idx := bits.ReverseBytes32(uint32(o.Index))
	_ = syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, ipUnicastIF, int(int32(idx)))
}

// ===================== 路由层级 =====================

var (
	iphlpapi            = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreateFile  = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpCloseHandle = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho2Ex = iphlpapi.NewProc("IcmpSendEcho2Ex")
)

// traceHops 每跳递增 TTL 发 ICMP echo，路由器回的「TTL 超时」里带着它自己的地址。
//
// 不用 tracert：它的 -S 只认 IPv6，IPv4 没法指定从哪张卡出去，开着 TUN 时
// 每一跳都是 *。IcmpSendEcho2Ex 能指定源地址，也不要管理员权限。
func (o OutboundIface) traceHops(target string, maxHops int) ([]model.NatRouterInfo, error) {
	src, dst := o.ip(), net.ParseIP(target).To4()
	if src == nil || dst == nil {
		return nil, fmt.Errorf("路由探测需要有效的 IPv4：源 %q，目标 %q", o.LocalIP, target)
	}
	if err := procIcmpSendEcho2Ex.Find(); err != nil {
		return nil, fmt.Errorf("加载 ICMP 接口失败: %w", err)
	}
	handle, _, err := procIcmpCreateFile.Call()
	if windows.Handle(handle) == windows.InvalidHandle {
		return nil, fmt.Errorf("创建 ICMP 句柄失败: %w", err)
	}
	defer procIcmpCloseHandle.Call(handle)

	return collectHops(target, maxHops, func(ttl int) (string, bool, error) {
		return icmpEcho(handle, src, dst, ttl)
	})
}

// icmpEcho 同步发一包，最多等 500ms
func icmpEcho(handle uintptr, src, dst net.IP, ttl int) (hop string, reached bool, err error) {
	const (
		ipSuccess        = 0
		ipReqTimedOut    = 11010
		ipTTLExpTransmit = 11013
	)
	// IP_OPTION_INFORMATION，只用到 TTL
	options := struct {
		TTL, TOS, Flags, OptionsSize byte
		OptionsData                  uintptr
	}{TTL: byte(ttl)}
	request := []byte("LinkStar")
	// 回包是 ICMP_ECHO_REPLY + 原样带回的数据，128 字节够 32/64 位两种布局
	var reply [128]byte

	n, _, callErr := procIcmpSendEcho2Ex.Call(
		handle, 0, 0, 0, // 不传 Event / APC：同步等结果
		uintptr(binary.LittleEndian.Uint32(src)), // IPAddr 是网络序字节按原样放进整数
		uintptr(binary.LittleEndian.Uint32(dst)),
		uintptr(unsafe.Pointer(&request[0])), uintptr(len(request)),
		uintptr(unsafe.Pointer(&options)),
		uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)),
		500,
	)
	if n == 0 {
		if callErr == windows.Errno(ipReqTimedOut) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("第 %d 跳探测失败: %w", ttl, callErr)
	}

	// ICMP_ECHO_REPLY 开头 8 字节两种布局一样：回包地址(4) + 状态(4)
	switch binary.LittleEndian.Uint32(reply[4:8]) {
	case ipSuccess:
		return net.IP(reply[:4]).String(), true, nil
	case ipTTLExpTransmit:
		return net.IP(reply[:4]).String(), false, nil
	default:
		return "", false, nil // 不可达、超时之类，这一跳当 * 处理
	}
}
