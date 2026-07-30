package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/pion/stun"
)

const tcpTimeout = 3 * time.Second

// stunResult 同时保留公网映射和响应源地址。
// 响应源地址用于确认 RFC 5780 CHANGE-REQUEST 是否真的让服务器换了出口。
type stunResult struct {
	MappedAddr *net.UDPAddr
	RespFrom   *net.UDPAddr
}

// buildBindingRequest 使用 pion/stun 构造标准 Binding Request。
// CHANGE-REQUEST 的 bit 2 表示换 IP，bit 1 表示换端口。
func buildBindingRequest(changeIP, changePort bool) *stun.Message {
	request := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if !changeIP && !changePort {
		return request
	}

	var flags byte
	if changeIP {
		flags |= 0x04
	}
	if changePort {
		flags |= 0x02
	}
	request.Add(stun.AttrChangeRequest, []byte{0, 0, 0, flags})
	return request
}

// mappedAddress 优先读取 RFC 5389 的 XOR-MAPPED-ADDRESS，
// 同时兼容只返回 RFC 3489 MAPPED-ADDRESS 的旧 STUN 服务端。
func mappedAddress(message *stun.Message) (*net.UDPAddr, error) {
	var xorAddr stun.XORMappedAddress
	if err := xorAddr.GetFrom(message); err == nil {
		return &net.UDPAddr{IP: xorAddr.IP, Port: xorAddr.Port}, nil
	}

	var plainAddr stun.MappedAddress
	if err := plainAddr.GetFrom(message); err == nil {
		return &net.UDPAddr{IP: plainAddr.IP, Port: plainAddr.Port}, nil
	}
	return nil, errors.New("响应中没有 MAPPED-ADDRESS 属性")
}

// stunQuery 在同一个 UDP socket 上完成一次请求响应。
// 复用 socket 是 Mapping 交叉验证成立的前提；事务 ID 校验用于过滤迟到响应。
func stunQuery(conn *net.UDPConn, server *net.UDPAddr, changeIP, changePort bool, timeout time.Duration) (*stunResult, error) {
	request := buildBindingRequest(changeIP, changePort)
	deadline := time.Now().Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("设置 UDP 超时: %w", err)
	}
	defer conn.SetDeadline(time.Time{})

	if _, err := conn.WriteToUDP(request.Raw, server); err != nil {
		return nil, fmt.Errorf("发送 Binding Request: %w", err)
	}

	buffer := make([]byte, 1500)
	for {
		n, from, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return nil, fmt.Errorf("等待 Binding Response: %w", err)
		}
		if !stun.IsMessage(buffer[:n]) {
			continue
		}

		response := &stun.Message{Raw: buffer[:n]}
		if err = response.Decode(); err != nil || response.Type != stun.BindingSuccess {
			continue
		}
		if !bytes.Equal(response.TransactionID[:], request.TransactionID[:]) {
			continue
		}

		mapped, err := mappedAddress(response)
		if err != nil {
			return nil, err
		}
		return &stunResult{MappedAddr: mapped, RespFrom: from}, nil
	}
}

// stunQueryRetry 容忍 UDP 的偶发丢包；每次请求都会由 pion 生成新的事务 ID。
func stunQueryRetry(conn *net.UDPConn, server *net.UDPAddr, changeIP, changePort bool, timeout time.Duration, attempts int) (result *stunResult, err error) {
	for range attempts {
		result, err = stunQuery(conn, server, changeIP, changePort, timeout)
		if err == nil {
			return result, nil
		}
	}
	return nil, err
}

func addrEqual(a, b *net.UDPAddr) bool {
	return a != nil && b != nil && a.IP.Equal(b.IP) && a.Port == b.Port
}

// stunQueryTCP 通过 TCP 执行 Binding Request。
// TCP 是字节流，因此先读取 20 字节 STUN 头，再根据头中的长度读取消息体。
// 成功时保持连接打开，避免 NAT 在第二次交叉探测前回收第一条映射。
func stunQueryTCP(localPort int, server *net.UDPAddr, timeout time.Duration) (result *stunResult, conn net.Conn, usedPort int, err error) {
	dialer := net.Dialer{Timeout: timeout, Control: setReuseAddr}
	if localPort != 0 {
		dialer.LocalAddr = &net.TCPAddr{Port: localPort}
	}

	conn, err = dialer.Dial("tcp4", net.JoinHostPort(server.IP.String(), strconv.Itoa(server.Port)))
	if err != nil {
		return nil, nil, 0, fmt.Errorf("TCP 连接失败: %w", err)
	}
	defer func() {
		if err != nil && conn != nil {
			conn.Close()
			conn = nil
		}
	}()

	usedPort = conn.LocalAddr().(*net.TCPAddr).Port
	if err = conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, nil, 0, fmt.Errorf("设置 TCP 超时: %w", err)
	}

	request := buildBindingRequest(false, false)
	if _, err = conn.Write(request.Raw); err != nil {
		return nil, nil, 0, fmt.Errorf("发送 Binding Request: %w", err)
	}

	header := make([]byte, 20)
	if _, err = io.ReadFull(conn, header); err != nil {
		return nil, nil, 0, fmt.Errorf("读取 STUN 响应头: %w", err)
	}
	bodyLength := int(binary.BigEndian.Uint16(header[2:4]))
	if bodyLength > 4096 {
		return nil, nil, 0, fmt.Errorf("STUN 响应体过大: %d", bodyLength)
	}

	raw := make([]byte, 20+bodyLength)
	copy(raw, header)
	if _, err = io.ReadFull(conn, raw[20:]); err != nil {
		return nil, nil, 0, fmt.Errorf("读取 STUN 响应体: %w", err)
	}

	response := &stun.Message{Raw: raw}
	if err = response.Decode(); err != nil {
		return nil, nil, 0, fmt.Errorf("解析 STUN 响应: %w", err)
	}
	if response.Type != stun.BindingSuccess || !bytes.Equal(response.TransactionID[:], request.TransactionID[:]) {
		return nil, nil, 0, errors.New("STUN 响应类型或事务 ID 不匹配")
	}

	mapped, err := mappedAddress(response)
	if err != nil {
		return nil, nil, 0, err
	}
	return &stunResult{MappedAddr: mapped}, conn, usedPort, nil
}

// runTCPMappingCheck 从同一个本地端口连接两台不同 IP 的 STUN 服务器。
// 映射不变表示 EIM，可尝试 TCP simultaneous open；变化则建议使用中继。
func runTCPMappingCheck(report *Report, servers []*net.UDPAddr) {
	fmt.Println("[5/5] 检测 TCP Mapping 行为 (STUN over TCP + 本地端口复用)...")
	report.TCP.Mapping = MappingUnknown
	report.TCP.FilteringNote = "无需检测(TCP 入站依赖 simultaneous open，成败主要取决于 Mapping 行为)"

	var first *stunResult
	var firstConn net.Conn
	var localPort, firstIndex int
	for i, server := range servers {
		result, currentConn, port, probeErr := stunQueryTCP(0, server, tcpTimeout)
		if probeErr != nil {
			fmt.Printf("  %s TCP 探测失败: %v\n", server, probeErr)
			continue
		}
		first, firstConn, localPort, firstIndex = result, currentConn, port, i
		break
	}
	if first == nil {
		report.TCP.DirectFeasibility = "未知(没有 TCP 可达的 STUN 服务器)"
		return
	}
	defer firstConn.Close()

	report.TCP.Tested = true
	report.TCP.LocalPort = localPort
	report.TCP.MappedAddrServer1 = first.MappedAddr.String()
	fmt.Printf("  服务器1 TCP 映射地址: %s (本地端口 %d)\n", first.MappedAddr, localPort)

	var second *stunResult
	for i := firstIndex + 1; i < len(servers); i++ {
		result, currentConn, _, probeErr := stunQueryTCP(localPort, servers[i], tcpTimeout)
		if probeErr != nil {
			fmt.Printf("  %s TCP 探测失败: %v\n", servers[i], probeErr)
			continue
		}
		currentConn.Close()
		second = result
		break
	}
	if second == nil {
		report.TCP.DirectFeasibility = "未知(第二台 TCP 服务器不可达，或本机 TCP 端口复用失败)"
		return
	}

	report.TCP.PortReuseOK = true
	report.TCP.MappedAddrServer2 = second.MappedAddr.String()
	fmt.Printf("  服务器2 TCP 映射地址: %s\n", second.MappedAddr)

	if !first.MappedAddr.IP.Equal(second.MappedAddr.IP) {
		report.TCP.DirectFeasibility = "不可行(TCP 出口 IP 不固定，建议走中继)"
		return
	}
	if first.MappedAddr.Port == second.MappedAddr.Port {
		report.TCP.Mapping = MappingEIM
		report.TCP.DirectFeasibility = "可尝试(TCP 映射为 EIM 且本地端口复用可用)"
		return
	}

	report.TCP.Mapping = MappingAPDM
	report.TCP.DirectFeasibility = "不可行(TCP 映射依赖目标，端口不可预测，建议走中继或 UDP 打洞)"
}
