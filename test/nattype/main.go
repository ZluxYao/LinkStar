// nat_diagnosis.go
//
// 用法: go run nat_diagnosis.go
//
// 这是一个零依赖(纯标准库)的单文件NAT检测测试程序，用于验证 LinkStar 的检测逻辑：
//   1. STUN Mapping行为检测（换两台不同STUN服务器，对比映射端口）—— 可靠，任何标准STUN服务器都支持
//   2. STUN Filtering行为检测（CHANGE-REQUEST）—— 仅供参考，绝大多数公网STUN服务器
//      （Google/Cloudflare/小米/腾讯等）不支持RFC5780扩展，实测大概率会落到"未知"，
//      这不是bug，是公网STUN基础设施的真实局限，想要可靠结果需要自建支持双IP的RFC5780服务器
//   3. traceroute链路层拓扑检测 —— 判断中间有几层NAT、CGNAT出现在第几跳
//   4. 把上面三路信号折叠成经典 NAT1-4 标签 + 给出 可直接使用/本地可优化/运营商限制 三态建议
//
// 依赖: 系统需要有 traceroute（Linux/Mac）或 tracepath（Linux兜底）或 tracert（Windows）
// 网络: 需要真实UDP出网能力，不能在做了UDP出站限制的沙箱/容器网络里跑

package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"time"
)

// ============================================================
// STUN 协议层 (RFC 5389 基础 + RFC 3489/5780 CHANGE-REQUEST 扩展)
// ============================================================

const (
	stunMagicCookie uint32 = 0x2112A442

	msgTypeBindingRequest  uint16 = 0x0001
	msgTypeBindingResponse uint16 = 0x0101

	attrMappedAddress    uint16 = 0x0001
	attrChangeRequest    uint16 = 0x0003
	attrXorMappedAddress uint16 = 0x0020

	flagChangeIP   byte = 0x04
	flagChangePort byte = 0x02

	familyIPv4 byte = 0x01
)

func buildTransactionID() [12]byte {
	var tid [12]byte
	rand.Read(tid[:])
	return tid
}

func writeAttr(buf *bytes.Buffer, attrType uint16, value []byte) {
	binary.Write(buf, binary.BigEndian, attrType)
	binary.Write(buf, binary.BigEndian, uint16(len(value)))
	buf.Write(value)
	if pad := (4 - len(value)%4) % 4; pad > 0 {
		buf.Write(make([]byte, pad))
	}
}

func buildBindingRequest(tid [12]byte, changeIP, changePort bool) []byte {
	var attrs bytes.Buffer
	if changeIP || changePort {
		var flags byte
		if changeIP {
			flags |= flagChangeIP
		}
		if changePort {
			flags |= flagChangePort
		}
		writeAttr(&attrs, attrChangeRequest, []byte{0, 0, 0, flags})
	}

	var msg bytes.Buffer
	binary.Write(&msg, binary.BigEndian, msgTypeBindingRequest)
	binary.Write(&msg, binary.BigEndian, uint16(attrs.Len()))
	binary.Write(&msg, binary.BigEndian, stunMagicCookie)
	msg.Write(tid[:])
	msg.Write(attrs.Bytes())
	return msg.Bytes()
}

type stunAttr struct {
	Type  uint16
	Value []byte
}

func parseStunMessage(data []byte) (msgType uint16, tid [12]byte, attrs []stunAttr, err error) {
	if len(data) < 20 {
		return 0, tid, nil, errors.New("响应太短")
	}
	msgType = binary.BigEndian.Uint16(data[0:2])
	msgLen := binary.BigEndian.Uint16(data[2:4])
	copy(tid[:], data[8:20])

	if len(data) < 20+int(msgLen) {
		return 0, tid, nil, errors.New("响应长度不匹配")
	}

	body := data[20 : 20+int(msgLen)]
	for len(body) >= 4 {
		aType := binary.BigEndian.Uint16(body[0:2])
		aLen := binary.BigEndian.Uint16(body[2:4])
		if len(body) < int(4+aLen) {
			break
		}
		val := body[4 : 4+aLen]
		attrs = append(attrs, stunAttr{Type: aType, Value: val})
		pad := (4 - int(aLen)%4) % 4
		advance := 4 + int(aLen) + pad
		if advance > len(body) {
			break
		}
		body = body[advance:]
	}
	return msgType, tid, attrs, nil
}

func decodeXorMappedAddress(value []byte) (*net.UDPAddr, error) {
	if len(value) < 8 {
		return nil, errors.New("XOR-MAPPED-ADDRESS长度不足")
	}
	family := value[1]
	if family != familyIPv4 {
		return nil, errors.New("暂不支持IPv6解析")
	}
	xport := binary.BigEndian.Uint16(value[2:4])
	port := xport ^ uint16(stunMagicCookie>>16)

	var cookieBytes [4]byte
	binary.BigEndian.PutUint32(cookieBytes[:], stunMagicCookie)

	ipBytes := make([]byte, 4)
	for i := 0; i < 4; i++ {
		ipBytes[i] = value[4+i] ^ cookieBytes[i]
	}
	return &net.UDPAddr{IP: net.IP(ipBytes), Port: int(port)}, nil
}

func decodeMappedAddress(value []byte) (*net.UDPAddr, error) {
	if len(value) < 8 {
		return nil, errors.New("MAPPED-ADDRESS长度不足")
	}
	if value[1] != familyIPv4 {
		return nil, errors.New("暂不支持IPv6解析")
	}
	port := binary.BigEndian.Uint16(value[2:4])
	ip := net.IP(value[4:8])
	return &net.UDPAddr{IP: ip, Port: int(port)}, nil
}

type stunResult struct {
	MappedAddr *net.UDPAddr
	RespFrom   *net.UDPAddr
}

// stunQuery 发送一次Binding Request，可选携带CHANGE-REQUEST。
// 会在deadline内循环读取，丢弃事务ID不匹配的历史/串扰响应包。
func stunQuery(conn *net.UDPConn, server *net.UDPAddr, changeIP, changePort bool, timeout time.Duration) (*stunResult, error) {
	tid := buildTransactionID()
	req := buildBindingRequest(tid, changeIP, changePort)

	deadline := time.Now().Add(timeout)
	conn.SetDeadline(deadline)
	defer conn.SetDeadline(time.Time{})

	if _, err := conn.WriteToUDP(req, server); err != nil {
		return nil, fmt.Errorf("发送失败: %w", err)
	}

	buf := make([]byte, 1500)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			return nil, errors.New("等待响应超时")
		}

		msgType, respTid, attrs, perr := parseStunMessage(buf[:n])
		if perr != nil || msgType != msgTypeBindingResponse || !bytes.Equal(respTid[:], tid[:]) {
			if time.Now().After(deadline) {
				return nil, errors.New("等待响应超时")
			}
			continue
		}

		result := &stunResult{RespFrom: from}
		for _, a := range attrs {
			switch a.Type {
			case attrXorMappedAddress:
				if addr, err := decodeXorMappedAddress(a.Value); err == nil {
					result.MappedAddr = addr
				}
			case attrMappedAddress:
				if result.MappedAddr == nil {
					if addr, err := decodeMappedAddress(a.Value); err == nil {
						result.MappedAddr = addr
					}
				}
			}
		}
		if result.MappedAddr == nil {
			return nil, errors.New("响应中没有MAPPED-ADDRESS属性")
		}
		return result, nil
	}
}

func addrEqual(a, b *net.UDPAddr) bool {
	if a == nil || b == nil {
		return false
	}
	return a.IP.Equal(b.IP) && a.Port == b.Port
}

func getPreferredOutboundAddr() (*net.UDPAddr, error) {
	conn, err := net.Dial("udp", "114.114.114.114:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr), nil
}

// ============================================================
// 链路拓扑层 (traceroute)
// ============================================================

var (
	_, private10, _  = net.ParseCIDR("10.0.0.0/8")
	_, private172, _ = net.ParseCIDR("172.16.0.0/12")
	_, private192, _ = net.ParseCIDR("192.168.0.0/16")
	_, cgnRange, _   = net.ParseCIDR("100.64.0.0/10")

	ipRegex            = regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
	tracepathHopRegex  = regexp.MustCompile(`^\s*(\d+)\??:`)
	tracerouteHopRegex = regexp.MustCompile(`^\s*(\d+)\s+\d`)
)

type IPType string

const (
	IPTypePrivate IPType = "私网"
	IPTypeCGN     IPType = "运营商CGNAT"
	IPTypePublic  IPType = "公网"
)

type NatHop struct {
	Level int    `json:"level"`
	IP    string `json:"ip"`
	Type  IPType `json:"type"`
}

func classifyIP(ipStr string) IPType {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return IPTypePrivate
	}
	if cgnRange.Contains(ip) {
		return IPTypeCGN
	}
	if private10.Contains(ip) || private172.Contains(ip) || private192.Contains(ip) {
		return IPTypePrivate
	}
	return IPTypePublic
}

func buildTracerouteCmd(target string) (*exec.Cmd, bool) {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("tracert", "-d", "-h", "10", "-w", "300", target), false
	case "linux":
		if _, err := exec.LookPath("traceroute"); err == nil {
			return exec.Command("traceroute", "-n", "-m", "10", "-w", "1", "-q", "1", target), false
		}
		return exec.Command("tracepath", "-n", "-m", "8", target), true
	default: // darwin
		return exec.Command("traceroute", "-n", "-m", "10", "-w", "1", "-q", "1", target), false
	}
}

func scanNATChain(target string) ([]NatHop, error) {
	cmd, isTracepath := buildTracerouteCmd(target)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("创建管道失败: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动traceroute失败(可能未安装该命令): %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
	}()

	var chain []NatHop
	scanner := bufio.NewScanner(stdout)
	level := 0
	lastHopNum := -1

	for scanner.Scan() {
		line := scanner.Text()

		if runtime.GOOS == "linux" {
			if isTracepath {
				m := tracepathHopRegex.FindStringSubmatch(line)
				if m == nil {
					continue
				}
				var hopNum int
				fmt.Sscanf(m[1], "%d", &hopNum)
				if hopNum == lastHopNum {
					continue
				}
				lastHopNum = hopNum
			} else {
				if tracerouteHopRegex.FindString(line) == "" {
					continue
				}
			}
		}

		ips := ipRegex.FindAllString(line, -1)
		if len(ips) == 0 {
			continue
		}
		ip := ips[0]
		if ip == target {
			continue
		}

		t := classifyIP(ip)
		if t != IPTypePublic {
			level++
			chain = append(chain, NatHop{Level: level, IP: ip, Type: t})
			if t == IPTypeCGN {
				break
			}
		} else {
			break
		}
	}
	return chain, nil
}

// ============================================================
// 折叠 + 诊断结论
// ============================================================

type MappingBehavior string

const (
	MappingEIM     MappingBehavior = "EIM(地址无关映射)"
	MappingAPDM    MappingBehavior = "APDM(地址端口双依赖映射)"
	MappingUnknown MappingBehavior = "未知(交叉验证失败)"
)

type FilteringBehavior string

const (
	FilteringEIF     FilteringBehavior = "EIF(地址无关过滤)"
	FilteringADF     FilteringBehavior = "ADF(地址依赖过滤)"
	FilteringUnknown FilteringBehavior = "未知(公网STUN服务器多不支持CHANGE-REQUEST，无法验证)"
)

type ClassicLabel string

const (
	LabelUDPBlocked     ClassicLabel = "UDP被阻断"
	LabelOpenInternet   ClassicLabel = "公网直连(Open Internet)"
	LabelSymmetricFW    ClassicLabel = "公网IP+对称防火墙"
	LabelFullCone       ClassicLabel = "NAT1 - 完全圆锥型(Full Cone)"
	LabelRestrictedCone ClassicLabel = "NAT2 - 受限圆锥型(Restricted Cone)"
	LabelPortRestricted ClassicLabel = "NAT3 - 端口受限圆锥型(Port Restricted Cone)"
	LabelSymmetric      ClassicLabel = "NAT4 - 对称型(Symmetric)"
	LabelPooledCGNAT    ClassicLabel = "运营商池化NAT(出口IP不固定)"
	LabelUncertain      ClassicLabel = "无法完全确定(按保守估计处理)"
)

type Report struct {
	Summary ClassicLabel `json:"summary"`

	Detail struct {
		LocalOutboundIP       string            `json:"local_outbound_ip"`
		LocalPort             int               `json:"local_port"`
		MappedAddrServer1     string            `json:"mapped_addr_server1"`
		MappedAddrServer2     string            `json:"mapped_addr_server2"`
		Mapping               MappingBehavior   `json:"mapping_behavior"`
		Filtering             FilteringBehavior `json:"filtering_behavior"`
		CrossServerIPMismatch bool              `json:"cross_server_ip_mismatch"`
	} `json:"detail"`

	Topology struct {
		Hops          []NatHop `json:"hops"`
		NatHopCount   int      `json:"nat_hop_count"`
		CGNATDetected bool     `json:"cgnat_detected"`
		CGNATHopIndex int      `json:"cgnat_hop_index"`
	} `json:"topology"`

	Verdict     string   `json:"verdict"`
	Suggestions []string `json:"suggestions"`
}

var stunServers = []string{
	"stun.hot-chilli.net:3478",
	"stun.voipstunt.com:3478",
	"stun.voipbuster.com:3478",
	"stun.voipstunt.com:3478",
}

const stunTimeout = 2 * time.Second

func resolveServers(addrs []string) []*net.UDPAddr {
	var out []*net.UDPAddr
	for _, a := range addrs {
		if udpAddr, err := net.ResolveUDPAddr("udp4", a); err == nil {
			out = append(out, udpAddr)
		}
	}
	return out
}

func RunDiagnosis() (*Report, error) {
	report := &Report{}

	// ---- 拓扑层 ----
	fmt.Println("[1/4] 扫描链路层级 (traceroute)...")
	hops, err := scanNATChain("114.114.114.114")
	if err != nil {
		fmt.Printf("  traceroute执行失败: %v (跳过拓扑层检测)\n", err)
	}
	report.Topology.Hops = hops
	report.Topology.NatHopCount = len(hops)
	for _, h := range hops {
		fmt.Printf("  第%d跳: %-15s [%s]\n", h.Level, h.IP, h.Type)
		if h.Type == IPTypeCGN {
			report.Topology.CGNATDetected = true
			report.Topology.CGNATHopIndex = h.Level
		}
	}
	if len(hops) == 0 {
		fmt.Println("  (未捕获到中间NAT跳，可能本机就在第一跳公网，或traceroute被防火墙拦截)")
	}

	// ---- 本机出口地址 ----
	localAddr, _ := getPreferredOutboundAddr()
	if localAddr != nil {
		report.Detail.LocalOutboundIP = localAddr.IP.String()
	}

	servers := resolveServers(stunServers)
	if len(servers) < 2 {
		return nil, errors.New("可用STUN服务器不足2个，检查DNS/网络连通性")
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
	if err != nil {
		return nil, fmt.Errorf("本地UDP监听失败: %w", err)
	}
	defer conn.Close()
	report.Detail.LocalPort = conn.LocalAddr().(*net.UDPAddr).Port

	// ---- 基础映射探测 ----
	fmt.Println("[2/4] 探测基础映射 (STUN Binding Request)...")
	res1, err1 := stunQuery(conn, servers[0], false, false, stunTimeout)
	if err1 != nil {
		res1, err1 = stunQuery(conn, servers[1], false, false, stunTimeout)
		if err1 != nil {
			report.Summary = LabelUDPBlocked
			report.Verdict = "本地可优化"
			report.Suggestions = []string{
				"连续两台STUN服务器均无响应，UDP出站可能被阻断",
				"检查本机/路由器防火墙的UDP出站规则",
				"确认光猫是否处于路由模式并拦截了UDP，尝试切换为桥接模式",
			}
			return report, nil
		}
	}
	report.Detail.MappedAddrServer1 = res1.MappedAddr.String()
	fmt.Printf("  服务器1映射地址: %s\n", res1.MappedAddr)

	// ---- Open Internet 判定 ----
	if localAddr != nil && localAddr.IP.Equal(res1.MappedAddr.IP) && localAddr.Port == res1.MappedAddr.Port {
		fmt.Println("[3/4] 映射地址与本机出口地址完全一致，判定为公网直连，继续验证是否存在防火墙限制")
		filterRes, ferr := stunQuery(conn, servers[0], true, true, stunTimeout)
		if ferr == nil && filterRes != nil && !addrEqual(filterRes.RespFrom, servers[0]) {
			report.Summary = LabelOpenInternet
			report.Detail.Filtering = FilteringEIF
			report.Verdict = "可直接使用"
			report.Suggestions = []string{"检测到公网IP且无防火墙限制，可直接对外提供服务，无需打洞"}
		} else {
			report.Summary = LabelSymmetricFW
			report.Detail.Filtering = FilteringUnknown
			report.Verdict = "本地可优化"
			report.Suggestions = []string{
				"检测到公网IP，但未能验证是否存在防火墙限制(该项测试依赖对方服务器支持RFC5780扩展，公网服务器大多不支持)",
				"若实际连接仍失败，检查本机/路由器防火墙的入站规则",
			}
		}
		return report, nil
	}

	// ---- Mapping行为交叉验证 ----
	fmt.Println("[3/4] 交叉验证Mapping行为 (更换STUN服务器对比映射端口)...")
	res2, err2 := stunQuery(conn, servers[1], false, false, stunTimeout)
	if err2 != nil && len(servers) >= 3 {
		res2, err2 = stunQuery(conn, servers[2], false, false, stunTimeout)
	}

	if err2 == nil && res2 != nil {
		report.Detail.MappedAddrServer2 = res2.MappedAddr.String()
		fmt.Printf("  服务器2映射地址: %s\n", res2.MappedAddr)

		if !res1.MappedAddr.IP.Equal(res2.MappedAddr.IP) {
			report.Detail.CrossServerIPMismatch = true
			report.Detail.Mapping = MappingUnknown
			report.Summary = LabelPooledCGNAT
			report.Verdict = "运营商限制"
			report.Suggestions = []string{
				"两台不同STUN服务器测出的公网出口IP不一致，说明运营商在做NAT池化/负载均衡，出口地址不固定",
				"端侧无法解决，不建议尝试打洞，直接走TURN中继",
			}
			return report, nil
		}

		if res1.MappedAddr.Port == res2.MappedAddr.Port {
			report.Detail.Mapping = MappingEIM
		} else {
			report.Detail.Mapping = MappingAPDM
		}
	} else {
		report.Detail.Mapping = MappingUnknown
		fmt.Println("  第二台服务器无响应，Mapping行为无法交叉验证")
	}

	// ---- Filtering行为探测 (尽力而为，公网环境大概率不可靠) ----
	fmt.Println("[4/4] 探测Filtering行为 (CHANGE-REQUEST，多数公网服务器不支持，仅供参考)...")
	report.Detail.Filtering = FilteringUnknown

	fcRes, fcErr := stunQuery(conn, servers[0], true, true, stunTimeout)
	if fcErr == nil && fcRes != nil && !addrEqual(fcRes.RespFrom, servers[0]) {
		report.Detail.Filtering = FilteringEIF
	} else {
		portRes, pErr := stunQuery(conn, servers[0], false, true, stunTimeout)
		if pErr == nil && portRes != nil &&
			portRes.RespFrom.IP.Equal(servers[0].IP) && portRes.RespFrom.Port != servers[0].Port {
			report.Detail.Filtering = FilteringADF
		}
	}

	classify(report)
	generateVerdict(report)
	return report, nil
}

func classify(r *Report) {
	if r.Detail.Mapping == MappingAPDM {
		r.Summary = LabelSymmetric
		return
	}
	if r.Detail.Mapping == MappingEIM {
		switch r.Detail.Filtering {
		case FilteringEIF:
			r.Summary = LabelFullCone
		case FilteringADF:
			r.Summary = LabelRestrictedCone
		default: // FilteringUnknown -> 保守按NAT3处理，不要过度乐观
			r.Summary = LabelPortRestricted
		}
		return
	}
	r.Summary = LabelUncertain
}

func generateVerdict(r *Report) {
	switch r.Summary {
	case LabelFullCone:
		r.Verdict = "可直接使用"
		r.Suggestions = []string{"NAT类型为完全圆锥型，打洞成功率非常高，可直接尝试P2P直连"}

	case LabelRestrictedCone, LabelPortRestricted:
		if r.Topology.CGNATDetected && r.Topology.NatHopCount >= 2 {
			r.Verdict = "本地可优化"
			r.Suggestions = []string{
				fmt.Sprintf("检测到%d层NAT(含运营商CGNAT)，当前行为可能是多层NAT叠加导致", r.Topology.NatHopCount),
				"若光猫处于路由模式(未桥接)，建议切换为桥接模式，把拨号交给主路由器，通常能改善NAT行为",
				"若已是桥接模式，此结果就是路由器本身行为，标准打洞流程仍可正常使用",
			}
		} else {
			r.Verdict = "可直接使用"
			r.Suggestions = []string{"标准打洞流程可用，无需额外配置"}
		}
		if r.Detail.Filtering == FilteringUnknown {
			r.Suggestions = append(r.Suggestions, "注：Filtering行为未能实测验证，此结论按较保守的NAT3处理，实际打洞成功率可能更好")
		}

	case LabelSymmetric:
		if r.Topology.CGNATDetected {
			r.Verdict = "运营商限制"
			r.Suggestions = []string{
				"检测到运营商级CGNAT且本端为对称型映射，端侧无法改善",
				"建议直接走TURN中继，不必在打洞重试上投入资源",
			}
		} else {
			r.Verdict = "本地可优化"
			r.Suggestions = []string{
				"对称型映射来自本地路由器，若路由器支持，可尝试开启UPnP或做静态端口转发/DMZ",
				"若无法调整路由器设置，仍建议走中继",
			}
		}

	case LabelUncertain:
		r.Verdict = "无法完全确定"
		r.Suggestions = []string{
			"Mapping行为交叉验证失败(可能是STUN服务器都无响应)，建议检查网络后重试",
			"若持续无法确定，建议直接尝试打洞作为最终验证手段",
		}
	}
}

// ============================================================
// main
// ============================================================

func main() {
	report, err := RunDiagnosis()
	if err != nil {
		fmt.Println("检测失败:", err)
		return
	}

	fmt.Println()
	fmt.Println("========== 检测报告 ==========")
	fmt.Printf("大结论: %s\n", report.Summary)
	fmt.Printf("处理建议: %s\n", report.Verdict)
	for _, s := range report.Suggestions {
		fmt.Println("  -", s)
	}

	fmt.Println()
	fmt.Println("---- 详细数据 ----")
	fmt.Printf("本机出口IP: %s (本地端口 %d)\n", report.Detail.LocalOutboundIP, report.Detail.LocalPort)
	fmt.Printf("STUN服务器1映射地址: %s\n", report.Detail.MappedAddrServer1)
	if report.Detail.MappedAddrServer2 != "" {
		fmt.Printf("STUN服务器2映射地址: %s\n", report.Detail.MappedAddrServer2)
	}
	fmt.Printf("Mapping行为: %s\n", report.Detail.Mapping)
	fmt.Printf("Filtering行为: %s\n", report.Detail.Filtering)
	fmt.Printf("跨服务器出口IP不一致: %v\n", report.Detail.CrossServerIPMismatch)

	fmt.Println()
	fmt.Println("---- 链路拓扑 ----")
	if len(report.Topology.Hops) == 0 {
		fmt.Println("(无中间NAT跳数据)")
	}
	for _, h := range report.Topology.Hops {
		fmt.Printf("  第%d跳: %-15s [%s]\n", h.Level, h.IP, h.Type)
	}
	fmt.Printf("NAT层数: %d, 检测到运营商CGNAT: %v\n", report.Topology.NatHopCount, report.Topology.CGNATDetected)

	fmt.Println()
	fmt.Println("---- JSON ----")
	jsonBytes, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(jsonBytes))
}
