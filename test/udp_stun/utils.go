package main

import (
	"fmt"
	"net"
	"time"

	"github.com/pion/stun/v2"
)

// UDP STUN 握手 (用在 ListenUDP 创建的未连接 socket 上)
// 通过 WriteToUDP/ReadFromUDP 发 binding request, 解出 XORMappedAddress
func StunHandshakeUDP(conn *net.UDPConn, stunAddr *net.UDPAddr) (string, int, error) {
	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err := conn.WriteToUDP(msg.Raw, stunAddr); err != nil {
		return "", 0, fmt.Errorf("send stun request: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	buf := make([]byte, 1500)
	for {
		n, raddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return "", 0, fmt.Errorf("read stun response: %w", err)
		}
		// 只认 STUN 服务器回包, 其他丢
		if !raddr.IP.Equal(stunAddr.IP) || raddr.Port != stunAddr.Port {
			continue
		}
		var resp stun.Message
		resp.Raw = buf[:n]
		if err = resp.Decode(); err != nil {
			return "", 0, fmt.Errorf("decode stun response: %w", err)
		}
		var xorAddr stun.XORMappedAddress
		if err = xorAddr.GetFrom(&resp); err != nil {
			return "", 0, fmt.Errorf("get xor mapped address: %w", err)
		}
		return xorAddr.IP.String(), xorAddr.Port, nil
	}
}

// 旧版: net.Conn 形态 (DialUDP 后用得上). 这里没用到,留着给参考
func DoStunHandshake(conn net.Conn) (string, int, error) {
	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err := conn.Write(msg.Raw); err != nil {
		return "", 0, fmt.Errorf("send stun request: %w", err)
	}

	conn.SetDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetDeadline(time.Time{})

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return "", 0, fmt.Errorf("read stun response: %w", err)
	}

	var response stun.Message
	response.Raw = buf[:n]
	if err = response.Decode(); err != nil {
		return "", 0, fmt.Errorf("decode stun response: %w", err)
	}

	var xorAddr stun.XORMappedAddress
	if err = xorAddr.GetFrom(&response); err != nil {
		return "", 0, fmt.Errorf("get mapped address: %w", err)
	}

	return xorAddr.IP.String(), xorAddr.Port, nil
}
