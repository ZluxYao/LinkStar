package stun

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/libp2p/go-reuseport"
	"github.com/pion/stun/v3"
)

// detectTCPMapping 是 TCP 场景下对 RFC 5780 §4.3 思路的类比：
// 复用同一本地端口依次连接主地址、备用 IP+主端口、备用 IP+备用端口，
// 比较服务端看到的映射地址是否一致，从而区分 EIM/ADM/APDM。
// 不做 Filtering：CHANGE-REQUEST 依赖无连接的 UDP 让服务端换源地址回包，
// TCP 面向连接做不到这一点，所以只测 Mapping。
func detectTCPMapping(server *RFC5780Server) (MappingBehavior, error) {
	// Test I：主地址，本地端口留给系统分配（"0.0.0.0:0"），
	// 这条连接全程保持打开以占住端口，后面两个 Test 靠它复用本地端口。
	conn1, m1, err := tcpBindingRequest("0.0.0.0:0", server.Primary.String())
	if err != nil {
		return MappingUnknown, fmt.Errorf("Test I 失败: %w", err)
	}
	defer conn1.Close()
	localAddr := conn1.LocalAddr().String()

	// Test II：备用 IP + 主端口，复用同一本地端口
	target2 := net.JoinHostPort(server.Other.IP.String(), fmt.Sprint(server.Primary.Port))
	conn2, m2, err := tcpBindingRequest(localAddr, target2)
	if err != nil {
		return MappingUnknown, fmt.Errorf("Test II 失败: %w", err)
	}
	conn2.Close()
	if tcpAddrEqual(m1, m2) {
		return MappingEndpointIndependent, nil
	}

	// Test III：备用 IP + 备用端口
	conn3, m3, err := tcpBindingRequest(localAddr, server.Other.String())
	if err != nil {
		return MappingUnknown, fmt.Errorf("Test III 失败: %w", err)
	}
	conn3.Close()
	if tcpAddrEqual(m2, m3) {
		return MappingAddressDependent, nil
	}
	return MappingAddressPortDependent, nil
}

func tcpAddrEqual(a, b *net.TCPAddr) bool {
	return a != nil && b != nil && a.IP.Equal(b.IP) && a.Port == b.Port
}

// tcpBindingRequest 用 reuseport.Dial 建立一条 TCP 连接并发一次 STUN Binding 请求。
// localAddr 为 "0.0.0.0:0" 时由系统分配临时端口（第一条连接）；
// 非 0 时强制复用该本地地址，reuseport 内部已经处理好 SO_REUSEADDR/SO_REUSEPORT，
// 不需要自己写平台相关代码。
// 连接不在函数内关闭，交给调用方决定何时释放——第一条连接必须保持打开才能占住端口。
func tcpBindingRequest(localAddr, remoteAddr string) (net.Conn, *net.TCPAddr, error) {
	conn, err := reuseport.Dial("tcp4", localAddr, remoteAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("连接 %s 失败: %w", remoteAddr, err)
	}

	fail := func(err error) (net.Conn, *net.TCPAddr, error) {
		conn.Close()
		return nil, nil, err
	}

	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return fail(fmt.Errorf("设置 deadline 失败: %w", err))
	}

	request := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err := conn.Write(request.Raw); err != nil {
		return fail(fmt.Errorf("发送失败: %w", err))
	}

	// STUN over TCP 有 20 字节固定头，[2:4] 是 body 长度，得先读头再按长度读 body，
	// 不能像 UDP 那样一次 Read 就拿到整个报文。
	header := make([]byte, 20)
	if _, err := io.ReadFull(conn, header); err != nil {
		return fail(fmt.Errorf("读取响应头失败: %w", err))
	}
	bodyLen := int(binary.BigEndian.Uint16(header[2:4]))
	if bodyLen > 4096 {
		return fail(fmt.Errorf("响应体过大: %d", bodyLen))
	}
	raw := make([]byte, 20+bodyLen)
	copy(raw, header)
	if _, err := io.ReadFull(conn, raw[20:]); err != nil {
		return fail(fmt.Errorf("读取响应体失败: %w", err))
	}

	response := new(stun.Message)
	response.Raw = raw
	if err := response.Decode(); err != nil {
		return fail(fmt.Errorf("解码失败: %w", err))
	}
	if response.TransactionID != request.TransactionID {
		return fail(errors.New("事务 ID 不匹配"))
	}
	if response.Type != stun.BindingSuccess {
		return fail(fmt.Errorf("服务器返回非成功响应: %s", response.Type))
	}

	var mapped stun.XORMappedAddress
	if err := mapped.GetFrom(response); err != nil {
		return fail(fmt.Errorf("读取 XOR-MAPPED-ADDRESS 失败: %w", err))
	}

	return conn, &net.TCPAddr{IP: mapped.IP, Port: mapped.Port}, nil
}
