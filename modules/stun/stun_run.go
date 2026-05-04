package stun

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/libp2p/go-reuseport"
	"github.com/pion/stun"
	"github.com/sirupsen/logrus"
)

// STUN 内网穿透实现
func (STUNRunner) Run(ctx context.Context, req STUNRequest, onState func(STUNState)) error {

	// 验证环境，确保不缺东西
	protocol, err := formatProtocol(req.Protocol)
	if err != nil {
		return err
	}
	localIP := Runtime.Network.LocalIP
	if localIP == "" {
		return fmt.Errorf("get local IP failed")
	}
	stunServer, err := Runtime.STUNService.GetBestSTUNServer()
	if err != nil {
		stunServer, err = Runtime.STUNService.GetBackupSTUNServer()
		if err != nil {
			return err
		}
	}

	// 端口复用连接STUN服务器
	localAddr := fmt.Sprintf("%s:0", localIP)
	stunConn, err := reuseport.Dial(protocol, localAddr, stunServer)
	if err != nil {
		primaryErr := err
		primarySTUNServer := stunServer
		// 可能 best STUN 掉线了，使用备用的
		stunServer, err = Runtime.STUNService.GetBackupSTUNServer()
		if err != nil {
			return fmt.Errorf("connect stun server %s failed: %v; get backup stun server: %w", primarySTUNServer, primaryErr, err)
		}
		// 再次发起连接
		stunConn, err = reuseport.Dial(protocol, localAddr, stunServer)
		if err != nil {
			return fmt.Errorf("connect stun server %s failed: %v; connect backup stun server %s: %w", primarySTUNServer, primaryErr, stunServer, err)
		}
	}
	defer stunConn.Close()

	//获取本地端口
	var localPort uint16
	switch protocol {
	case "tcp":
		localPort = uint16(stunConn.LocalAddr().(*net.TCPAddr).Port)
	case "udp":
		localPort = uint16(stunConn.LocalAddr().(*net.UDPAddr).Port)
	}

	// 发送STUN 请求,获取外部端口
	publicIP, publicPort, err := doStunHandshake(stunConn)
	if err != nil {
		return fmt.Errorf("stun handshake: %w", err)
	}
	if protocol == "udp" {
		logrus.Infof("UDP:%s:%d", publicIP, publicPort)
	}

	// 在本地端口复用监听，接受穿透后的入站连接
	var tcpListener net.Listener
	var udpListener *net.UDPConn
	listenAddr := fmt.Sprintf("%s:%d", localIP, localPort)
	if protocol == "tcp" {

		tcpListener, err = reuseport.Listen(protocol, listenAddr)
		if err != nil {
			return fmt.Errorf("listen on %s failed: %w", listenAddr, err)
		}
		defer tcpListener.Close()
	} else {
		// UDP 没有 Listener 概念，用 ListenUDP 接收入站包
		var udpListenAddr *net.UDPAddr
		udpListenAddr, resolveErr := net.ResolveUDPAddr("udp", listenAddr)
		if resolveErr != nil {
			return fmt.Errorf("resolve udp addr %s failed: %w", listenAddr, resolveErr)
		}
		// ↓ 换成 reuseport.ListenPacket，和 stunConn 共用同一个端口
		packetConn, listenErr := reuseport.ListenPacket("udp", udpListenAddr.String())
		if listenErr != nil {
			return fmt.Errorf("listen udp on %s failed: %w", listenAddr, listenErr)
		}
		udpListener = packetConn.(*net.UDPConn)
		defer udpListener.Close()

	}

	// UPNP启动 映射 网关的localPort->localip:localPort
	if req.UseUPnP {
		upnpCtx, upnpCancel := context.WithTimeout(ctx, 20*time.Second)
		err := AddPortMappingByQueueWithLocalIP(
			upnpCtx,
			localPort,
			localPort,
			strings.ToUpper(protocol),
			fmt.Sprintf("EasyLink-%s", req.ServiceName),
			localIP,
		)
		if err != nil {
			upnpCancel()
			return err
		}
		// 释放资源
		upnpCancel()
		defer func() {
			go DeletePortMapping(localPort, strings.ToUpper(protocol))
		}()
	}

	// 把端口传出去
	if onState != nil {
		onState(STUNState{State: STUNMapped, ExternalPort: uint16(publicPort)}) //todo统一一下命名
	}

	// 保活
	innerCtx, innerCancel := context.WithCancel(ctx)
	errCh := make(chan error, 2)
	defer innerCancel()
	if protocol == "tcp" { //tcp 保活
		go func() {
			if err := tcpStunHealthKeepAlive(
				innerCtx,
				stunConn,
				publicIP,
				publicPort,
				localPort,
				localIP,
				func() {
					if onState != nil {
						onState(STUNState{State: STUNAlive, ExternalPort: uint16(publicPort)})
					}
				},
			); err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("tcp keepalive failed: %w", err)
			}
		}()
	} else if protocol == "udp" { //udp 保活
		go func() {
			if err := udpStunHealthKeepAlive(
				innerCtx,
				stunConn,
				// udpListener, // 用来收包
				publicIP,
				publicPort,
				localPort,
				localIP,
				func() {
					if onState != nil {
						onState(STUNState{State: STUNAlive, ExternalPort: uint16(publicPort)})
					}
				},
			); err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("udp keepalive failed: %w", err)
			}
		}()
	}

	// 接收外网连接，转发到对应内网服务
	go func() {
		targetAddr := fmt.Sprintf("%s:%d", req.TargetIP, req.InternalPort) // 构建目标ip+端口
		if protocol == "tcp" {
			for {
				clientConn, err := tcpListener.Accept()
				if err != nil {
					if ctx.Err() != nil {
						errCh <- nil
					} else {
						errCh <- fmt.Errorf("listener closed unexpectedly: %w", err)
					}
					return
				}
				go ForwardTCP(clientConn, targetAddr, protocol) //转发到对应的内网服务
			}
		} else {
			buf := make([]byte, 65535)
			for {
				n, remoteAddr, err := udpListener.ReadFromUDP(buf)
				if err != nil {
					if ctx.Err() != nil {
						errCh <- nil
					} else {
						errCh <- fmt.Errorf("udp listener closed: %w", err)
					}
					return
				}
				data := make([]byte, n)
				copy(data, buf[:n])
				go ForwardUDP(udpListener, remoteAddr, data, targetAddr)
			}
		}

	}()

	return <-errCh
}

// ===================== 内部 函数 ===============

// 格式化 Protocol 输入
func formatProtocol(protocol string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(protocol)) // 去除空格转小写
	if value == "" {
		value = "tcp"
	}
	switch value {
	case "tcp", "udp":
		return value, nil
	default:
		return "", fmt.Errorf("unsupported protocol: %s", protocol)
	}

}

// 发送 STUN 请求
func doStunHandshake(conn net.Conn) (string, int, error) {
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

// ================= TCP 保活 ========================

// STUN TCP 保活
func tcpStunHealthKeepAlive(
	ctx context.Context,
	currentStunConn net.Conn,
	publicIP string,
	publicPort int,
	localPort uint16,
	localIP string,
	onAlive func(),
) error {

	// 进行第一次健康检查，同时保活
	if !tcpStunHealthCheck(ctx, publicIP, publicPort) {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("tcp health check failed")
	}

	// 告诉外面TCP 检查ok
	if onAlive != nil {
		onAlive()
	}

	// 开启28s 的检查
	healthTicker := time.NewTicker(28 * time.Second)
	defer healthTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-healthTicker.C:

			// 梯度健康检查
			if tcpStunHealthCheck(ctx, publicIP, publicPort) {
				continue
			}

			// 梯度健康检查失败，先尝试用现有 stunConn 重新握手确认端口
			_, port, err := doStunHandshake(currentStunConn)
			if err == nil {
				// 握手成功，但端口变了，说明 NAT 映射已变，无法恢复
				if port != publicPort {
					return fmt.Errorf("public port changed from %d to %d", publicPort, port)
				}
				// 端口未变，可能是网络抖动，继续等待
				continue
			}

			// stunConn 本身也断了，尝试重新建连
			currentStunConn.Close()

			stunServer, err := Runtime.STUNService.GetBestSTUNServer()
			if err != nil {
				stunServer, err = Runtime.STUNService.GetBackupSTUNServer()
				if err != nil {
					return fmt.Errorf("get stun server for reconnect: %w", err)
				}
			}

			localAddr := fmt.Sprintf("%s:%d", localIP, localPort)
			newConn, dialErr := reuseport.Dial("tcp", localAddr, stunServer)
			if dialErr != nil {
				return fmt.Errorf("reconnect stun server %s failed: %w", stunServer, dialErr)
			}

			_, newPort, handshakeErr := doStunHandshake(newConn)
			if handshakeErr != nil {
				newConn.Close()
				return fmt.Errorf("stun handshake after reconnect failed: %w", handshakeErr)
			}

			if newPort != publicPort {
				newConn.Close()
				return fmt.Errorf("public port changed from %d to %d", publicPort, newPort)
			}

			// 重连成功，端口一致，替换连接继续保活
			currentStunConn = newConn
		}
	}

}

// 类似 Time.Sleep 但是开被ctx 终终端
func sleepWithCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

// TCP STUN 梯度健康检查
func tcpStunHealthCheck(ctx context.Context, publicIP string, expectedPublicPort int) bool {
	// 第一次检查
	if tcpConnectCheck(publicIP, expectedPublicPort, 3*time.Second) {
		return true
	}

	// 梯度检查1 3 5
	sleepTime := 1 * time.Second
	for i := 0; i < 3; i++ {
		if !sleepWithCtx(ctx, sleepTime) {
			return false
		}
		if tcpConnectCheck(publicIP, expectedPublicPort, 3*time.Second) {
			return true
		}
		sleepTime += 2 * time.Second
	}
	return false
}

// TCP检查是否存活
func tcpConnectCheck(host string, port int, timeout time.Duration) bool {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ================= UDP 保活 ========================

// UDP STUN 保活
func udpStunHealthKeepAlive(
	ctx context.Context,
	currentStunConn net.Conn,
	// udpListener *net.UDPConn,
	publicIP string,
	publicPort int,
	localPort uint16,
	localIP string,
	onAlive func(),
) error {
	// 先做一次外部连通性验证
	if !udpStunHealthCheck(ctx, publicIP, publicPort, localPort) {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("udp health check failed")
	}
	if onAlive != nil {
		onAlive()
	}

	// UDP NAT 映射超时通常 30s，用 20s 心跳保住映射
	keepAliveTicker := time.NewTicker(20 * time.Second)
	// 外部连通性检查周期可以长一些
	healthTicker := time.NewTicker(28 * time.Second)
	defer keepAliveTicker.Stop()
	defer healthTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-keepAliveTicker.C:
			// 向 STUN 服务器发心跳，维持 NAT 映射
			msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
			currentStunConn.SetWriteDeadline(time.Now().Add(3 * time.Second))
			currentStunConn.Write(msg.Raw)
			currentStunConn.SetWriteDeadline(time.Time{})

		case <-healthTicker.C:
			if udpStunHealthCheck(ctx, publicIP, publicPort, localPort) {
				continue
			}

			// 外部连通性失败，先用现有 conn 重新握手确认端口
			_, port, err := doStunHandshake(currentStunConn)
			if err == nil {
				if port != publicPort {
					return fmt.Errorf("public port changed from %d to %d", publicPort, port)
				}
				continue
			}

			// stunConn 断了，重新建连
			currentStunConn.Close()
			stunServer, err := Runtime.STUNService.GetBestSTUNServer()
			if err != nil {
				stunServer, err = Runtime.STUNService.GetBackupSTUNServer()
				if err != nil {
					return fmt.Errorf("get stun server for reconnect: %w", err)
				}
			}

			localAddr := fmt.Sprintf("%s:%d", localIP, localPort)
			newConn, dialErr := reuseport.Dial("udp", localAddr, stunServer)
			if dialErr != nil {
				return fmt.Errorf("reconnect stun server %s failed: %w", stunServer, dialErr)
			}

			_, newPort, handshakeErr := doStunHandshake(newConn)
			if handshakeErr != nil {
				newConn.Close()
				return fmt.Errorf("stun handshake after reconnect failed: %w", handshakeErr)
			}

			if newPort != publicPort {
				newConn.Close()
				return fmt.Errorf("public port changed from %d to %d", publicPort, newPort)
			}

			currentStunConn = newConn
		}
	}
}

// UDP 梯度健康检查
func udpStunHealthCheck(ctx context.Context, publicIP string, publicPort int, localPort uint16) bool {
	if udpConnectCheck(publicIP, publicPort, localPort, 3*time.Second) {
		return true
	}
	sleepTime := 1 * time.Second
	for i := 0; i < 3; i++ {
		if !sleepWithCtx(ctx, sleepTime) {
			return false
		}
		if udpConnectCheck(publicIP, publicPort, localPort, 3*time.Second) {
			return true
		}
		sleepTime += 2 * time.Second
	}
	return false
}

// 向 publicIP:publicPort 发一个探测包，用临时 socket 收回包，不干扰 udpListener
func udpConnectCheck(publicIP string, publicPort int, localPort uint16, timeout time.Duration) bool {
	// 发探测包
	dst := fmt.Sprintf("%s:%d", publicIP, publicPort)
	probe, err := net.Dial("udp", dst)
	if err != nil {
		return false
	}
	defer probe.Close()

	probe.SetDeadline(time.Now().Add(timeout))
	if _, err = probe.Write([]byte("ping")); err != nil {
		return false
	}

	// 单独开一个临时 socket 在同端口收回包，不碰 udpListener
	tempConn, err := reuseport.ListenPacket("udp", fmt.Sprintf(":%d", localPort))
	if err != nil {
		return false
	}
	defer tempConn.Close()

	tempConn.SetDeadline(time.Now().Add(timeout))
	buf := make([]byte, 64)
	_, _, err = tempConn.ReadFrom(buf)
	return err == nil
}
