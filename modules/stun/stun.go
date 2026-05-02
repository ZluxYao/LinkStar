package stun

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/libp2p/go-reuseport"
	"github.com/pion/stun"
)

// stun内网穿透实现
func (STUNTunnelRunner) Run(ctx context.Context, req STUNRequest, onState func(STUNState)) error {
	emitLog := func(format string, args ...any) {
		if onState != nil {
			onState(STUNState{State: STUNLog, Log: fmt.Sprintf(format, args...)})
		}
	}

	// 验证环境
	protocol, err := normalizeProtocol(req.Protocol)
	if err != nil {
		return err
	}
	localIP := Runtime.Network.LocalIP
	if localIP == "" {
		return fmt.Errorf("local IP is not ready")
	}

	stunServer, err := currentSTUNServer()
	if err != nil {
		return err
	}
	emitLog("选择 STUN 服务器: %s", stunServer)

	// 端口复用
	localAddr := fmt.Sprintf("%s:0", localIP)
	stunConn, err := reuseport.Dial(protocol, localAddr, stunServer)
	if err != nil {
		return fmt.Errorf("connect stun server %s: %w", stunServer, err)
	}
	emitLog("已连接 STUN 服务器: %s，本地地址: %s", stunServer, stunConn.LocalAddr())

	var localPort uint16
	switch protocol {
	case "tcp":
		localPort = uint16(stunConn.LocalAddr().(*net.TCPAddr).Port)
	case "udp":
		localPort = uint16(stunConn.LocalAddr().(*net.UDPAddr).Port)
	}

	// 发送stun 请求
	var publicIP string
	var publicPort int
	if protocol == "tcp" {
		publicIP, publicPort, err = doTcpStunHandshake(stunConn)
	} else {
		udpConn := stunConn.(*net.UDPConn)
		stunServerAddr, resolveErr := net.ResolveUDPAddr("udp", stunServer)
		if resolveErr != nil {
			stunConn.Close()
			return fmt.Errorf("resolve stun server %s: %w", stunServer, resolveErr)
		}
		publicIP, publicPort, err = doUDPStunHandshake(udpConn, stunServerAddr)
	}
	if err != nil {
		stunConn.Close()
		return fmt.Errorf("stun handshake failed: %w", err)
	}
	emitLog("STUN 打洞成功: %s:%d -> 本地端口 %d/%s", publicIP, publicPort, localPort, strings.ToUpper(protocol))

	// 监听端口
	listenAddr := fmt.Sprintf("%s:%d", localIP, localPort)
	listener, err := reuseport.Listen(protocol, listenAddr)
	if err != nil {
		stunConn.Close()
		return fmt.Errorf("listen on %s failed: %w", listenAddr, err)
	}
	emitLog("开始监听隧道端口: %s", listenAddr)

	// upnp
	if req.UseUPnP {
		upnpCtx, upnpCancel := context.WithTimeout(ctx, 25*time.Second)
		if err := AddPortMappingQueueWithLocalIP(
			upnpCtx,
			localPort,
			localPort,
			strings.ToUpper(protocol),
			fmt.Sprintf("LinkStar-%s", req.ServiceName),
			localIP,
		); err != nil {
			emitLog("UPnP 端口映射失败: %v", err)
		} else {
			emitLog("UPnP 端口映射成功: %d/%s -> %s:%d", localPort, strings.ToUpper(protocol), localIP, localPort)
		}
		upnpCancel()
	}

	defer func() {
		stunConn.Close()
		listener.Close()
		if req.UseUPnP {
			go DeletePortMapping(localPort, strings.ToUpper(protocol))
		}
	}()

	innerCtx, innerCancel := context.WithCancel(ctx)
	defer innerCancel()

	go func() {
		<-ctx.Done()
		stunConn.Close()
		listener.Close()
	}()

	// 传出去端口
	if onState != nil {
		onState(STUNState{
			State:        STUNMapped,
			ExternalPort: uint16(publicPort),
			Log:          fmt.Sprintf("端口打通: 公网 %s:%d，本地 %s:%d/%s", publicIP, publicPort, localIP, localPort, strings.ToUpper(protocol)),
		})
	}

	errCh := make(chan error, 2)

	// 保活
	if protocol == "tcp" {
		go func() {
			if err := tcpStunHealthCheck(
				innerCtx,
				stunConn,
				publicIP,
				publicPort,
				localPort,
				localIP,
				stunServer,
				func() {
					if onState != nil {
						onState(STUNState{
							State:        STUNAlive,
							ExternalPort: uint16(publicPort),
							Log:          fmt.Sprintf("健康检查通过，公网端口可访问: %s:%d", publicIP, publicPort),
						})
					}
				},
			); err != nil && ctx.Err() == nil {
				emitLog("TCP 健康检查失败: %v", err)
				errCh <- fmt.Errorf("tcp health check failed: %w", err)
			}
		}()
	} else {
		go func() {
			udpConn := stunConn.(*net.UDPConn)
			stunServerAddr, resolveErr := net.ResolveUDPAddr("udp", stunServer)
			if resolveErr != nil {
				if ctx.Err() == nil {
					emitLog("解析 STUN UDP 地址失败: %v", resolveErr)
					errCh <- fmt.Errorf("resolve stun server %s: %w", stunServer, resolveErr)
				}
				return
			}
			if err := udpStunHealthCheck(
				innerCtx,
				udpConn,
				stunServerAddr,
				publicPort,
				localPort,
				localIP,
				func() {
					if onState != nil {
						onState(STUNState{
							State:        STUNAlive,
							ExternalPort: uint16(publicPort),
							Log:          fmt.Sprintf("健康检查通过，公网端口可访问: %s:%d", publicIP, publicPort),
						})
					}
				},
			); err != nil && ctx.Err() == nil {
				emitLog("UDP 健康检查失败: %v", err)
				errCh <- fmt.Errorf("udp health check failed: %w", err)
			}
		}()
	}

	go func() {
		targetAddr := fmt.Sprintf("%s:%d", req.TargetIP, req.InternalPort)
		for {
			clientConn, err := listener.Accept()
			if err != nil {
				if ctx.Err() != nil {
					errCh <- nil
				} else {
					emitLog("接收隧道连接失败: %v", err)
					errCh <- fmt.Errorf("accept tunnel connection: %w", err)
				}
				return
			}
			emitLog("收到隧道连接: %s -> %s", clientConn.RemoteAddr(), targetAddr)
			go Forward(clientConn, targetAddr, protocol)
		}
	}()

	return <-errCh
}

func currentSTUNServer() (string, error) {
	if Runtime.STUNService != nil {
		stunServer, err := Runtime.STUNService.GetBackupSTUNServer()
		if err == nil && stunServer != "" {
			return stunServer, nil
		}
	}

	if Runtime.Config.BestSTUN != "" {
		return Runtime.Config.BestSTUN, nil
	}

	return "", fmt.Errorf("best STUN server is not ready")
}

func normalizeProtocol(protocol string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(protocol))
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

func doTcpStunHandshake(conn net.Conn) (string, int, error) {
	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err := conn.Write(msg.Raw); err != nil {
		return "", 0, fmt.Errorf("send stun request failed: %w", err)
	}

	conn.SetDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetDeadline(time.Time{})

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return "", 0, fmt.Errorf("read stun response failed: %w", err)
	}

	var response stun.Message
	response.Raw = buf[:n]
	if err = response.Decode(); err != nil {
		return "", 0, fmt.Errorf("decode stun response failed: %w", err)
	}

	var xorAddr stun.XORMappedAddress
	if err = xorAddr.GetFrom(&response); err != nil {
		return "", 0, fmt.Errorf("read mapped address failed: %w", err)
	}

	return xorAddr.IP.String(), xorAddr.Port, nil
}

func doUDPStunHandshake(conn *net.UDPConn, stunServerAddr *net.UDPAddr) (string, int, error) {
	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err := conn.WriteToUDP(msg.Raw, stunServerAddr); err != nil {
		return "", 0, fmt.Errorf("send udp stun request failed: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return "", 0, fmt.Errorf("read udp stun response failed: %w", err)
	}

	var response stun.Message
	response.Raw = buf[:n]
	if err = response.Decode(); err != nil {
		return "", 0, fmt.Errorf("decode udp stun response failed: %w", err)
	}

	var xorAddr stun.XORMappedAddress
	if err = xorAddr.GetFrom(&response); err != nil {
		return "", 0, fmt.Errorf("read udp mapped address failed: %w", err)
	}

	return xorAddr.IP.String(), xorAddr.Port, nil
}

func firstTcpHealthKeep(ctx context.Context, publicIP string, expectedPublicPort int) bool {
	sleepTime := 2 * time.Second
	for i := 0; i < 3; i++ {
		if !sleepWithCtx(ctx, sleepTime) {
			return false
		}
		if tcpConnectCheck(publicIP, expectedPublicPort, 3*time.Second) {
			return true
		}
		sleepTime *= 2
	}
	return false
}

func tcpStunHealthCheck(
	ctx context.Context,
	stunConn net.Conn,
	publicIP string,
	expectedPublicPort int,
	localPort uint16,
	localIP string,
	stunServer string,
	onAlive func(),
) error {
	if !firstTcpHealthKeep(ctx, publicIP, expectedPublicPort) {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("initial tcp keepalive failed")
	}

	if onAlive != nil {
		onAlive()
	}

	healthTicker := time.NewTicker(28 * time.Second)
	defer healthTicker.Stop()

	maxFailures := 3
	failureCount := 0
	currentStunConn := stunConn

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-healthTicker.C:
			if tcpConnectCheck(publicIP, expectedPublicPort, 3*time.Second) {
				failureCount = 0
				continue
			}

			failureCount++
			if failureCount == 1 && tcpConnectCheck(publicIP, expectedPublicPort, 3*time.Second) {
				failureCount = 0
				continue
			}

			_, port, err := doTcpStunHandshake(currentStunConn)
			if err != nil {
				currentStunConn.Close()

				localAddr := fmt.Sprintf("%s:%d", localIP, localPort)
				newConn, dialErr := reuseport.Dial("tcp", localAddr, stunServer)
				if dialErr != nil {
					return fmt.Errorf("reconnect stun failed: %w", dialErr)
				}

				_, newPort, handshakeErr := doTcpStunHandshake(newConn)
				if handshakeErr != nil {
					newConn.Close()
					return fmt.Errorf("verify reconnected stun failed: %w", handshakeErr)
				}

				if newPort != expectedPublicPort {
					newConn.Close()
					return fmt.Errorf("public port changed from %d to %d", expectedPublicPort, newPort)
				}

				currentStunConn = newConn
				continue
			}

			if port != expectedPublicPort {
				return fmt.Errorf("public port changed from %d to %d", expectedPublicPort, port)
			}

			if failureCount >= maxFailures {
				return fmt.Errorf("tcp connectivity failed %d times", maxFailures)
			}
		}
	}
}

func udpStunHealthCheck(
	ctx context.Context,
	udpConn *net.UDPConn,
	stunServer *net.UDPAddr,
	expectedPublicPort int,
	localPort uint16,
	localIP string,
	onAlive func(),
) error {
	healthTicker := time.NewTicker(28 * time.Second)
	defer healthTicker.Stop()

	consecutiveFailures := 0
	maxFailures := 3
	currentConn := udpConn
	aliveReported := false

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-healthTicker.C:
			_, port, err := doUDPStunHandshake(currentConn, stunServer)
			if err != nil {
				consecutiveFailures++
				if consecutiveFailures < maxFailures {
					continue
				}

				currentConn.Close()

				localAddr := &net.UDPAddr{
					IP:   net.ParseIP(localIP),
					Port: int(localPort),
				}
				newConn, listenErr := net.ListenUDP("udp", localAddr)
				if listenErr != nil {
					return fmt.Errorf("rebuild udp connection failed: %w", listenErr)
				}

				_, newPort, handshakeErr := doUDPStunHandshake(newConn, stunServer)
				if handshakeErr != nil {
					newConn.Close()
					return fmt.Errorf("verify rebuilt udp connection failed: %w", handshakeErr)
				}

				if newPort != expectedPublicPort {
					newConn.Close()
					return fmt.Errorf("public port changed from %d to %d", expectedPublicPort, newPort)
				}

				currentConn = newConn
				consecutiveFailures = 0
				continue
			}

			if port != expectedPublicPort {
				return fmt.Errorf("public port changed from %d to %d", expectedPublicPort, port)
			}

			if !aliveReported {
				if onAlive != nil {
					onAlive()
				}
				aliveReported = true
			}

			consecutiveFailures = 0
		}
	}
}

func tcpConnectCheck(host string, port int, timeout time.Duration) bool {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
