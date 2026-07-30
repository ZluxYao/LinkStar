// RFC 5780 NAT 行为探测
//
// UDP 严格按 RFC 5780 第 4.3、4.4 节探测 Mapping 和 Filtering。
// TCP 复用相同本地端口比较不同目标的映射；RFC 5780 的 CHANGE-REQUEST
// 只适用于无连接的 UDP，因此  TCP 不做 Filtering 分类。
package stun

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/pion/stun/v3"
)

// detectUDPMapping 实现 RFC 5780 4.3：
// Test I 访问主地址；Test II 访问备用 IP + 主端口；
// Test III 访问备用 IP + 备用端口，从而区分 EIM、ADM、APDM。
func detectUDPMapping(server *RFC5780Server) (MappingBehavior, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return MappingUnknown, fmt.Errorf("创建 UDP socket 失败: %w", err)
	}
	defer conn.Close()

	doTest := func(target *net.UDPAddr) (*net.UDPAddr, error) {
		if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
			return nil, fmt.Errorf("设置 deadline 失败: %w", err)
		}
		r, err := stunBindingRequest(conn, target)
		if err != nil {
			return nil, err
		}
		if r.Mapped == nil {
			return nil, errors.New("响应缺少 XOR-MAPPED-ADDRESS")
		}
		return r.Mapped, nil
	}

	// Test I：主地址
	m1, err := doTest(server.Primary)
	if err != nil {
		return MappingUnknown, fmt.Errorf("Test I 失败: %w", err)
	}

	// Test II：备用 IP + 主端口
	target2 := &net.UDPAddr{IP: server.Other.IP, Port: server.Primary.Port}
	m2, err := doTest(target2)
	if err != nil {
		return MappingUnknown, fmt.Errorf("Test II 失败: %w", err)
	}
	if udpAddrEqual(m1, m2) {
		return MappingEndpointIndependent, nil
	}

	// Test III：备用 IP + 备用端口
	m3, err := doTest(server.Other)
	if err != nil {
		return MappingUnknown, fmt.Errorf("Test III 失败: %w", err)
	}
	if udpAddrEqual(m2, m3) {
		return MappingAddressDependent, nil
	}
	return MappingAddressPortDependent, nil

}

func udpAddrEqual(a, b *net.UDPAddr) bool {
	return a != nil && b != nil && a.IP.Equal(b.IP) && a.Port == b.Port
}

// detectUDPFiltering 使用独立 socket，避免 Mapping 测试访问备用地址后污染过滤状态。
// Test II 要求服务端换 IP 和端口；失败后 Test III 只要求换端口。
func detectUDPFiltering(server *RFC5780Server) (FilteringBehavior, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return FilteringUnknown, fmt.Errorf("创建 UDP socket 失败: %w", err)
	}
	defer conn.Close()

	// Test I：先确认服务器本身可达，不可达就没必要往下测
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return FilteringUnknown, fmt.Errorf("设置 deadline 失败: %w", err)
	}
	if _, err := stunBindingRequest(conn, server.Primary); err != nil {
		return FilteringUnknown, fmt.Errorf("Test I 失败: %w", err)
	}

	// Test II：要求换 IP 又换端口
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return FilteringUnknown, fmt.Errorf("设置 deadline 失败: %w", err)
	}
	_, err = stunBindingRequest(conn, server.Primary, changeRequest{ChangeIP: true, ChangePort: true})
	if err == nil {
		return FilteringEndpointIndependent, nil
	}
	if !isTimeout(err) {
		return FilteringUnknown, fmt.Errorf("Test II 失败: %w", err)
	}

	// Test III：只换端口
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return FilteringUnknown, fmt.Errorf("设置 deadline 失败: %w", err)
	}
	_, err = stunBindingRequest(conn, server.Primary, changeRequest{ChangeIP: false, ChangePort: true})
	if err == nil {
		return FilteringAddressDependent, nil
	}
	if !isTimeout(err) {
		return FilteringUnknown, fmt.Errorf("Test III 失败: %w", err)
	}

	return FilteringAddressPortDependent, nil
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// selectRFC5780Server 不只检查 Binding 是否有响应，还要求 OTHER-ADDRESS, 返回UDP 测试最低延时的
// 同时提供不同的 IP 和端口，避免把单 IP 服务误当成完整 RFC 5780 服务。
func selectRFC5780Server(serverList []string) (*RFC5780Server, error) {
	candidates := normalizeCandidates(serverList)
	if len(candidates) == 0 {
		return nil, errors.New("RFC 5780 候选列表为空")
	}

	type probeResult struct {
		Candidate string
		Server    *RFC5780Server
		Err       error
	}
	results := make(chan probeResult, len(candidates))

	for _, candidate := range candidates {
		go func(server string) {
			srv, err := probeRFC5780StunServerUDP(server)
			results <- probeResult{Candidate: server, Server: srv, Err: err}
		}(candidate)
	}

	var errs []error
	for i := 0; i < len(candidates); i++ {
		r := <-results
		if r.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", r.Candidate, r.Err))
			continue
		}
		// 第一个成功的直接返回，不等剩下的候选（尤其是死掉的那些会拖到超时才出结果）。
		return r.Server, nil
	}

	if len(errs) == 0 {
		return nil, errors.New("没有有效的 RFC 5780 候选服务器")
	}
	return nil, fmt.Errorf("没有有效的 RFC 5780 候选服务器: %w", errors.Join(errs...))

}

// probeRFC5780StunServer
//  1. 发一次 Binding，看服务器活不活、延迟多少；
//  2. 响应里的 OTHER-ADDRESS 是不是双 IP/双端口（RFC 5780 服务器的硬性前提，
//     没有第二个公网 IP，后面的 Mapping/Filtering 测试根本做不了）。
//
// 不验证 targets 的可达性，那部分留给正式检测流程。
func probeRFC5780StunServerUDP(server string) (*RFC5780Server, error) {
	primary, err := net.ResolveUDPAddr("udp4", server)
	if err != nil {
		return nil, fmt.Errorf("解析失败: %w", err)
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return nil, fmt.Errorf("创建 UDP socket 失败: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return nil, fmt.Errorf("设置 deadline 失败: %w", err)
	}

	start := time.Now()
	result, err := stunBindingRequest(conn, primary)
	delay := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("stunBinding 失败: %w", err)
	}

	if result.Other == nil {
		return nil, errors.New("响应中缺少 OTHER-ADDRESS，服务器不支持 RFC 5780")
	}
	if !validOtherAddress(primary, result.Other) {
		return nil, errors.New("OTHER-ADDRESS 不是双 IP/双端口，不符合 RFC 5780 前提")
	}

	return &RFC5780Server{
		Name:    server,
		Primary: primary,
		Other:   result.Other,
		Delay:   delay,
	}, nil
}

type stunBindingResult struct {
	Mapped *net.UDPAddr // XOR-MAPPED-ADDRESS：服务器看到的客户端公网地址
	Other  *net.UDPAddr // OTHER-ADDRESS：RFC 5780 用于切换探测目标的备用端点
}

// stunBindingRequest 向 target 发送一次 Binding 请求并等待响应。
// 沿用调用方在 conn 上已设置好的 deadline，不在内部单独处理超时。
func stunBindingRequest(conn *net.UDPConn, target *net.UDPAddr, extra ...stun.Setter) (*stunBindingResult, error) {
	args := append([]stun.Setter{stun.TransactionID, stun.BindingRequest}, extra...)
	request := stun.MustBuild(args...)

	if _, err := conn.WriteToUDP(request.Raw, target); err != nil {
		return nil, fmt.Errorf("发送失败: %w", err)
	}

	buf := make([]byte, 1500)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			// deadline 到期会从这里返回，不会死循环。
			return nil, fmt.Errorf("读取响应失败: %w", err)
		}

		response := new(stun.Message)
		response.Raw = append(response.Raw[:0], buf[:n]...)
		if err := response.Decode(); err != nil {
			continue // 不是合法 STUN 报文，丢弃继续等
		}
		if response.TransactionID != request.TransactionID {
			continue // 同一 socket 上滞留的旧响应，丢弃继续等
		}
		if response.Type != stun.BindingSuccess {
			return nil, fmt.Errorf("服务器返回非成功响应: %s（来自 %s）", response.Type, from)
		}

		// 整理数据
		result := &stunBindingResult{}

		var mapped stun.XORMappedAddress
		if err := mapped.GetFrom(response); err == nil {
			result.Mapped = &net.UDPAddr{IP: mapped.IP, Port: mapped.Port}
		}

		var other stun.OtherAddress
		if err := other.GetFrom(response); err == nil {
			result.Other = &net.UDPAddr{IP: other.IP, Port: other.Port}
		}
		return result, nil
	}
}

// changeRequest 实现 CHANGE-REQUEST 属性（RFC 5780 §4.2，常量来自 RFC 3489 §12）。
// pion/stun 官方明确不打算内置这个属性的 helper struct，只给了 AttrChangeRequest 常量，
// 所以需要自己按 STUN attribute 的 TLV 格式手动编码。
type changeRequest struct {
	ChangeIP   bool
	ChangePort bool
}

// AddTo 实现 stun.Setter 接口。
func (c changeRequest) AddTo(m *stun.Message) error {
	value := make([]byte, 4) // CHANGE-REQUEST 固定 4 字节，只有第 29、30 位有意义
	if c.ChangeIP {
		value[3] |= 0x04
	}
	if c.ChangePort {
		value[3] |= 0x02
	}
	m.Add(stun.AttrChangeRequest, value)
	return nil
}

func validOtherAddress(primary, other *net.UDPAddr) bool {
	return primary != nil && other != nil &&
		!other.IP.Equal(primary.IP) && other.Port != primary.Port &&
		!other.IP.IsPrivate() && !other.IP.IsLoopback() && !other.IP.IsUnspecified()
}

func normalizeCandidates(serverList []string) []string {
	seen := make(map[string]struct{}, len(serverList))
	candidates := make([]string, 0, len(serverList))
	for _, c := range serverList {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		candidates = append(candidates, c)
	}
	return candidates
}
