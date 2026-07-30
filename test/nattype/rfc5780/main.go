// RFC 5780 NAT 行为探测示例。
//
// UDP 严格按 RFC 5780 第 4.3、4.4 节探测 Mapping 和 Filtering。
// TCP 复用相同本地端口比较不同目标的映射；RFC 5780 的 CHANGE-REQUEST
// 只适用于无连接的 UDP，因此  TCP 不做 Filtering 分类。
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/pion/stun"
)

const probeTimeout = 3 * time.Second

// stunServers 同时提供 UDP/TCP STUN；其中 RFC 5780 的 CHANGE-REQUEST
// 能力会在运行时实测，因为同一域名的不同后端可能配置不一致。
var stunServers = []string{
	"stun.dcalling.de:3478",
	"stun.freeswitch.org:3478",
	"stun.sip.us:3478",
	"stun.sonetel.net:3478",
	"stun.radiojar.com:3478",
	"stun.sonetel.com:3478",
}

type response struct {
	mapped         *net.UDPAddr
	other          *net.UDPAddr
	responseOrigin *net.UDPAddr
	from           *net.UDPAddr
}

type report struct {
	Server       string   `json:"server"`
	UDPMapping   string   `json:"udp_mapping"`
	UDPFiltering string   `json:"udp_filtering"`
	TCPMapping   string   `json:"tcp_mapping"`
	Notes        []string `json:"notes"`
}

func main() {
	serverFlag := flag.String("server", "", "指定 RFC 5780 STUN 服务；留空时自动选择")
	flag.Parse()

	primary, err := selectRFC5780Server(*serverFlag)
	if err != nil {
		fmt.Printf("选择 RFC 5780 STUN 服务器失败: %v\n", err)
		return
	}
	fmt.Printf("已选择 RFC 5780 服务端: %s\n", primary)

	result := report{Server: primary.String(), TCPMapping: "未知"}
	other, mapping, err := detectUDPMapping(primary)
	if err != nil {
		result.UDPMapping = "无法确定"
		result.Notes = append(result.Notes, err.Error())
	} else {
		result.UDPMapping = mapping
	}

	filtering, err := detectUDPFiltering(primary)
	if err != nil {
		result.UDPFiltering = "无法确定"
		result.Notes = append(result.Notes, err.Error())
	} else {
		result.UDPFiltering = filtering
	}

	result.TCPMapping, err = detectTCPMapping(primary, other)
	if err != nil {
		result.Notes = append(result.Notes, err.Error())
	}
	result.Notes = append(result.Notes, "TCP 没有 RFC 5780 Filtering：响应必须沿已经建立的 TCP 连接返回")

	fmt.Println()
	fmt.Println("========== RFC 5780 检测报告 ==========")
	fmt.Printf("服务端: %s\n", result.Server)
	fmt.Printf("UDP Mapping: %s\n", result.UDPMapping)
	fmt.Printf("UDP Filtering: %s\n", result.UDPFiltering)
	fmt.Printf("TCP Mapping: %s\n", result.TCPMapping)
	for _, note := range result.Notes {
		fmt.Printf("提示: %s\n", note)
	}
}

// selectRFC5780Server 不只检查 Binding 是否有响应，还要求 OTHER-ADDRESS
// 同时提供不同的 IP 和端口，避免把单 IP 服务误当成完整 RFC 5780 服务。
func selectRFC5780Server(explicit string) (*net.UDPAddr, error) {
	candidates := stunServers
	if explicit != "" {
		candidates = []string{explicit}
	}

	var lastErr error
	for _, candidate := range candidates {
		addr, err := net.ResolveUDPAddr("udp4", candidate)
		if err != nil {
			lastErr = err
			continue
		}
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
		if err != nil {
			return nil, err
		}
		result, probeErr := udpRoundTrip(conn, addr, false, false)
		conn.Close()
		if probeErr != nil {
			lastErr = fmt.Errorf("%s: %w", candidate, probeErr)
			continue
		}
		if !validOtherAddress(addr, result.other) {
			lastErr = fmt.Errorf("%s 没有有效的双 IP/双端口 OTHER-ADDRESS", candidate)
			continue
		}
		return addr, nil
	}
	if lastErr == nil {
		lastErr = errors.New("候选列表为空")
	}
	return nil, lastErr
}

// detectUDPMapping 实现 RFC 5780 4.3：
// Test I 访问主地址；Test II 访问备用 IP + 主端口；
// Test III 访问备用 IP + 备用端口，从而区分 EIM、ADM、APDM。
func detectUDPMapping(primary *net.UDPAddr) (*net.UDPAddr, string, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return nil, "", fmt.Errorf("UDP Mapping 创建 socket: %w", err)
	}
	defer conn.Close()

	fmt.Println("[UDP Mapping I] 请求主 STUN 地址")
	first, err := udpRoundTrip(conn, primary, false, false)
	if err != nil {
		return nil, "", fmt.Errorf("UDP Mapping Test I: %w", err)
	}
	if first.other == nil {
		return nil, "", errors.New("服务端未返回 OTHER-ADDRESS，不支持完整 RFC 5780")
	}
	if !validOtherAddress(primary, first.other) {
		return first.other, "", fmt.Errorf(
			"OTHER-ADDRESS=%s 未同时更换 IP 和端口，无法完成 RFC 5780 Mapping 分类", first.other,
		)
	}
	fmt.Printf("  映射=%s, OTHER-ADDRESS=%s\n", first.mapped, first.other)

	localIP := preferredLocalIP(primary)
	localPort := conn.LocalAddr().(*net.UDPAddr).Port
	if localIP != nil && localIP.Equal(first.mapped.IP) && localPort == first.mapped.Port {
		return first.other, "Endpoint-Independent Mapping (无 NAT)", nil
	}

	secondTarget := &net.UDPAddr{IP: first.other.IP, Port: primary.Port}
	fmt.Printf("[UDP Mapping II] 请求备用 IP + 主端口: %s\n", secondTarget)
	second, err := udpRoundTrip(conn, secondTarget, false, false)
	if err != nil {
		return first.other, "", fmt.Errorf("UDP Mapping Test II: %w", err)
	}
	fmt.Printf("  映射=%s\n", second.mapped)
	if sameAddr(first.mapped, second.mapped) {
		return first.other, "Endpoint-Independent Mapping (EIM)", nil
	}

	fmt.Printf("[UDP Mapping III] 请求备用 IP + 备用端口: %s\n", first.other)
	third, err := udpRoundTrip(conn, first.other, false, false)
	if err != nil {
		return first.other, "", fmt.Errorf("UDP Mapping Test III: %w", err)
	}
	fmt.Printf("  映射=%s\n", third.mapped)
	if sameAddr(second.mapped, third.mapped) {
		return first.other, "Address-Dependent Mapping (ADM)", nil
	}
	return first.other, "Address-and-Port-Dependent Mapping (APDM)", nil
}

// detectUDPFiltering 使用独立 socket，避免 Mapping 测试访问备用地址后污染过滤状态。
// Test II 要求服务端换 IP 和端口；失败后 Test III 只要求换端口。
func detectUDPFiltering(primary *net.UDPAddr) (string, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return "", fmt.Errorf("UDP Filtering 创建 socket: %w", err)
	}
	defer conn.Close()

	fmt.Println("[UDP Filtering I] 确认服务端支持 OTHER-ADDRESS")
	first, err := udpRoundTrip(conn, primary, false, false)
	if err != nil {
		return "", fmt.Errorf("UDP Filtering Test I: %w", err)
	}
	if first.other == nil {
		return "", errors.New("服务端未返回 OTHER-ADDRESS，不支持完整 RFC 5780")
	}
	if !validOtherAddress(primary, first.other) {
		return "", fmt.Errorf(
			"OTHER-ADDRESS=%s 未同时更换 IP 和端口，无法完成 RFC 5780 Filtering 分类", first.other,
		)
	}

	fmt.Println("[UDP Filtering II] CHANGE-REQUEST: 换 IP + 换端口")
	second, secondErr := udpRoundTrip(conn, primary, true, true)
	if secondErr == nil && responseChangedSource(second, primary) {
		return "Endpoint-Independent Filtering (EIF)", nil
	}

	fmt.Println("[UDP Filtering III] CHANGE-REQUEST: 仅换端口")
	third, thirdErr := udpRoundTrip(conn, primary, false, true)
	if thirdErr == nil && responseChangedPort(third, primary) {
		return "Address-Dependent Filtering (ADF)", nil
	}
	if thirdErr != nil {
		return "Address-and-Port-Dependent Filtering (APDF)", nil
	}
	return "", errors.New("服务端忽略 CHANGE-REQUEST，无法可靠判断 Filtering")
}

// udpRoundTrip 构造请求、校验事务 ID，并解析 RFC 5780 地址属性。
func udpRoundTrip(conn *net.UDPConn, target *net.UDPAddr, changeIP, changePort bool) (*response, error) {
	request := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if changeIP || changePort {
		var flags byte
		if changeIP {
			flags |= 0x04
		}
		if changePort {
			flags |= 0x02
		}
		request.Add(stun.AttrChangeRequest, []byte{0, 0, 0, flags})
	}

	if err := conn.SetDeadline(time.Now().Add(probeTimeout)); err != nil {
		return nil, err
	}
	defer conn.SetDeadline(time.Time{})
	if _, err := conn.WriteToUDP(request.Raw, target); err != nil {
		return nil, fmt.Errorf("发送到 %s: %w", target, err)
	}

	buffer := make([]byte, 1500)
	for {
		n, from, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return nil, fmt.Errorf("等待 %s 响应: %w", target, err)
		}
		if !stun.IsMessage(buffer[:n]) {
			continue
		}
		message := &stun.Message{Raw: buffer[:n]}
		if message.Decode() != nil || message.Type != stun.BindingSuccess ||
			!bytes.Equal(message.TransactionID[:], request.TransactionID[:]) {
			continue
		}

		result := &response{from: from}
		var mapped stun.XORMappedAddress
		if err := mapped.GetFrom(message); err != nil {
			return nil, fmt.Errorf("读取 XOR-MAPPED-ADDRESS: %w", err)
		}
		result.mapped = &net.UDPAddr{IP: mapped.IP, Port: mapped.Port}

		var other stun.OtherAddress
		if other.GetFrom(message) == nil {
			result.other = &net.UDPAddr{IP: other.IP, Port: other.Port}
		}
		var origin stun.ResponseOrigin
		if origin.GetFrom(message) == nil {
			result.responseOrigin = &net.UDPAddr{IP: origin.IP, Port: origin.Port}
		}
		return result, nil
	}
}

func responseChangedSource(value *response, primary *net.UDPAddr) bool {
	actual := value.from
	if value.responseOrigin != nil {
		actual = value.responseOrigin
	}
	return actual != nil && !sameAddr(actual, primary)
}

func responseChangedPort(value *response, primary *net.UDPAddr) bool {
	actual := value.from
	if value.responseOrigin != nil {
		actual = value.responseOrigin
	}
	return actual != nil && actual.IP.Equal(primary.IP) && actual.Port != primary.Port
}

// detectTCPMapping 优先比较 RFC 5780 主/备用地址；备用地址未开放 TCP 时，
// 再尝试候选 STUN 服务。两条连接保持相同本地端口，第一条在比较期间保持打开。
func detectTCPMapping(primary, other *net.UDPAddr) (string, error) {
	targets := uniqueTCPAddresses(primary, other)
	var first *net.UDPAddr
	var firstMapped *net.TCPAddr
	var firstConn net.Conn
	var localPort, firstIndex int

	fmt.Println("[TCP Mapping] 使用同一本地端口探测不同目标")
	for i, target := range targets {
		mapped, conn, port, err := tcpBinding(0, target)
		if err != nil {
			fmt.Printf("  %s 不可用: %v\n", target, err)
			continue
		}
		first, firstMapped, firstConn, localPort, firstIndex = target, mapped, conn, port, i
		break
	}
	if firstConn == nil {
		return "未知", errors.New("没有 TCP 可达的 STUN 服务")
	}
	defer firstConn.Close()
	fmt.Printf("  %s -> %s (本地端口 %d)\n", first, firstMapped, localPort)

	for _, target := range targets[firstIndex+1:] {
		mapped, conn, _, err := tcpBinding(localPort, target)
		if err != nil {
			fmt.Printf("  %s 不可用: %v\n", target, err)
			continue
		}
		conn.Close()
		fmt.Printf("  %s -> %s\n", target, mapped)
		if firstMapped.IP.Equal(mapped.IP) && firstMapped.Port == mapped.Port {
			return "Endpoint-Independent Mapping (EIM)", nil
		}
		return "Destination-Dependent Mapping", nil
	}
	return "未知", errors.New("找不到第二个 TCP 目标，或本地端口复用失败")
}

func uniqueTCPAddresses(primary, other *net.UDPAddr) []*net.UDPAddr {
	addresses := make([]*net.UDPAddr, 0, len(stunServers)+2)
	seen := make(map[string]bool)
	add := func(addr *net.UDPAddr) {
		if addr == nil || seen[addr.String()] {
			return
		}
		seen[addr.String()] = true
		addresses = append(addresses, addr)
	}
	add(primary)
	add(other)
	for _, server := range stunServers {
		addr, err := net.ResolveUDPAddr("udp4", server)
		if err == nil {
			add(addr)
		}
	}
	return addresses
}

func tcpBinding(localPort int, target *net.UDPAddr) (*net.TCPAddr, net.Conn, int, error) {
	dialer := net.Dialer{Timeout: probeTimeout, Control: setReuseAddr}
	if localPort != 0 {
		dialer.LocalAddr = &net.TCPAddr{Port: localPort}
	}
	conn, err := dialer.Dial("tcp4", net.JoinHostPort(target.IP.String(), strconv.Itoa(target.Port)))
	if err != nil {
		return nil, nil, 0, err
	}
	fail := func(err error) (*net.TCPAddr, net.Conn, int, error) {
		conn.Close()
		return nil, nil, 0, err
	}
	if err = conn.SetDeadline(time.Now().Add(probeTimeout)); err != nil {
		return fail(err)
	}

	request := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err = conn.Write(request.Raw); err != nil {
		return fail(err)
	}
	header := make([]byte, 20)
	if _, err = io.ReadFull(conn, header); err != nil {
		return fail(err)
	}
	bodyLength := int(binary.BigEndian.Uint16(header[2:4]))
	if bodyLength > 4096 {
		return fail(fmt.Errorf("响应体过大: %d", bodyLength))
	}
	raw := make([]byte, 20+bodyLength)
	copy(raw, header)
	if _, err = io.ReadFull(conn, raw[20:]); err != nil {
		return fail(err)
	}

	message := &stun.Message{Raw: raw}
	if err = message.Decode(); err != nil {
		return fail(err)
	}
	if message.Type != stun.BindingSuccess || !bytes.Equal(message.TransactionID[:], request.TransactionID[:]) {
		return fail(errors.New("响应类型或事务 ID 不匹配"))
	}
	var mapped stun.XORMappedAddress
	if err = mapped.GetFrom(message); err != nil {
		return fail(err)
	}
	port := conn.LocalAddr().(*net.TCPAddr).Port
	return &net.TCPAddr{IP: mapped.IP, Port: mapped.Port}, conn, port, nil
}

func sameAddr(a, b *net.UDPAddr) bool {
	return a != nil && b != nil && a.IP.Equal(b.IP) && a.Port == b.Port
}

func validOtherAddress(primary, other *net.UDPAddr) bool {
	return primary != nil && other != nil &&
		!other.IP.Equal(primary.IP) && other.Port != primary.Port &&
		!other.IP.IsPrivate() && !other.IP.IsLoopback() && !other.IP.IsUnspecified()
}

// preferredLocalIP 让系统路由表选择到目标地址的出口网卡，只使用选中的 IP。
func preferredLocalIP(target *net.UDPAddr) net.IP {
	conn, err := net.DialUDP("udp4", nil, target)
	if err != nil {
		return nil
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP
}
