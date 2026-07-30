// 独立的完整 NAT 诊断示例。
//
// STUN 报文由 github.com/pion/stun 编解码；诊断功能包括：
//   1. UDP Mapping 与 Filtering 行为探测
//   2. TCP Mapping 与本地端口复用探测
//   3. traceroute/tracert 网关及 CGNAT 链路探测
//   4. NAT1-4 分类、建议与 JSON 报告
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"time"
)

// getPreferredOutboundAddr 获取本机默认出网网卡的IP。
// UDP的Dial不会真的发包，只是让内核做一次路由决策，
// 从LocalAddr里就能拿到系统选中的出口网卡地址。
// 注意：返回的Port是这个临时socket的随机端口，和检测用的conn无关，
// 上层只能用它的IP，不能用它的Port(见Open Internet判定处的说明)。
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
	_, cgnRange, _   = net.ParseCIDR("100.64.0.0/10") // RFC6598运营商级NAT专用段，出现即可确认CGNAT

	ipRegex            = regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
	tracepathHopRegex  = regexp.MustCompile(`^\s*(\d+)\??:`) // tracepath行首格式 "1:" 或 "1?:"
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

// buildTracerouteCmd 按平台选择可用的路由追踪命令。
// 参数统一按"快"调：限制最大跳数、缩短单跳超时、单探测包，
// 因为我们只关心公网出口前的几跳，不需要完整链路。
// 第二个返回值表示是否为tracepath(它的输出格式需要单独的解析逻辑)。
func buildTracerouteCmd(target string) (*exec.Cmd, bool) {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("tracert", "-d", "-h", "10", "-w", "300", target), false
	case "linux":
		if _, err := exec.LookPath("traceroute"); err == nil {
			return exec.Command("traceroute", "-n", "-m", "10", "-w", "1", "-q", "1", target), false
		}
		// 部分发行版默认没有traceroute但自带tracepath(iputils)
		return exec.Command("tracepath", "-n", "-m", "8", target), true
	default: // darwin
		return exec.Command("traceroute", "-n", "-m", "10", "-w", "1", "-q", "1", target), false
	}
}

// scanNATChain 通过traceroute扫描本机到公网之间的NAT层级。
// 原理：从第一跳开始逐跳看IP，私网IP=一层NAT设备，100.64/10=运营商CGNAT，
// 一旦碰到公网IP或CGNAT就停止——我们只关心"出公网之前经过了什么"。
// 边流式读边解析，命中终止条件后直接kill子进程，不等完整traceroute跑完。
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
		// 提前break时进程还在跑，必须kill+Wait回收，否则泄漏子进程
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
				// tracepath对同一跳可能输出多行(不同度量)，按跳号去重只取第一行
				m := tracepathHopRegex.FindStringSubmatch(line)
				if m == nil {
					continue
				}
				hopNum, _ := strconv.Atoi(m[1])
				if hopNum == lastHopNum {
					continue
				}
				lastHopNum = hopNum
			} else {
				// 过滤掉traceroute的表头行，只保留"跳号开头"的数据行
				if tracerouteHopRegex.FindString(line) == "" {
					continue
				}
			}
		}

		ips := ipRegex.FindAllString(line, -1)
		if len(ips) == 0 {
			continue // 超时跳(*)或纯文本行
		}
		ip := ips[0]
		if ip == target {
			// 跳过表头里的目标IP(如tracert的"Tracing route to ..."行)，
			// 以及到达目标本身的最后一跳
			continue
		}

		t := classifyIP(ip)
		if t != IPTypePublic {
			level++
			chain = append(chain, NatHop{Level: level, IP: ip, Type: t})
			if t == IPTypeCGN {
				break // CGNAT已是运营商侧，再往后没有诊断价值
			}
		} else {
			break // 到公网了，NAT链扫描结束
		}
	}
	if err := scanner.Err(); err != nil {
		// 读输出中途出错时已解析的跳照常返回，上层把它当部分数据用
		return chain, fmt.Errorf("读取traceroute输出出错: %w", err)
	}
	return chain, nil
}

// ============================================================
// 折叠 + 诊断结论
// ============================================================

// MappingBehavior 描述NAT对"同一内网端口访问不同目的地址"如何分配映射。
// EIM: 不管目的是谁，映射不变 —— 打洞的前提条件
// APDM: 换个目的地址就换映射 —— 即对称型NAT，无法预测端口
// 注: 严格的RFC4787还有ADM(仅地址依赖)一档，两台服务器IP不同即可触发映射变化，
// 与APDM在本检测中不可区分，也无需区分——对打洞的影响是一样的。
type MappingBehavior string

const (
	MappingEIM     MappingBehavior = "EIM(地址无关映射)"
	MappingAPDM    MappingBehavior = "APDM(地址端口双依赖映射)"
	MappingUnknown MappingBehavior = "未知(交叉验证失败)"
)

// FilteringBehavior 描述NAT对"从没发过包的地址来的入站包"放不放行。
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

	// TCP侧独立于UDP结论：NAT1-4主结论由UDP信号驱动(打洞主路径)，
	// 这里只回答一个问题——"TCP直连(同时打开)值不值得尝试"
	TCP struct {
		Tested            bool            `json:"tested"`
		LocalPort         int             `json:"local_port"`
		MappedAddrServer1 string          `json:"mapped_addr_server1"`
		MappedAddrServer2 string          `json:"mapped_addr_server2"`
		Mapping           MappingBehavior `json:"mapping_behavior"`
		PortReuseOK       bool            `json:"port_reuse_ok"`
		FilteringNote     string          `json:"filtering_note"`
		DirectFeasibility string          `json:"direct_feasibility"`
	} `json:"tcp"`

	Verdict     string   `json:"verdict"`
	Suggestions []string `json:"suggestions"`
}

// stunServers STUN服务器候选列表。
// 注意事项：
//   - Mapping交叉验证的前提是前两台可用服务器解析到"不同的公网IP"，
//     否则对称型NAT也会返回相同映射，NAT4会被误判成NAT2/3，
//     resolveServers里按解析出的IP做了去重来保证这一点
//   - voipstunt/voipbuster同属Betamax，有解析到同一后端的可能，所以排在后面做兜底
//   - TCP 3478可达性实测(2026-07,国内网络)：hot-chilli/threema可用；
//     miwifi和Betamax两台仅UDP，stunprotocol.org/sipgate/cloudflare等TCP均不通。
//     TCP交叉验证至少需要两台不同IP的TCP可达服务器，检测时逐台实测、不可达自动跳过
var stunServers = []string{
	// "stun.miwifi.com:3478",
	// "stun.hot-chilli.net:3478",
	// "stun.threema.ch:3478",
	// "stun.voipstunt.com:3478",
	// "stun.voipbuster.com:3478",
	"stun.dcalling.de:3478",
	"stun.freeswitch.org:3478",
	"stun.sip.us:3478",
	"stun.sonetel.net:3478",
	"stun.radiojar.com:3478",
	"stun.sonetel.com:3478",

	"stun.nextcloud.com:3478",
	"stun.voipgate.com:3478 ",
}

const stunTimeout = 2 * time.Second

// stunRetries 每台服务器的重试次数(含首次)，见stunQueryRetry的说明
const stunRetries = 2

// resolveServers 解析STUN服务器域名，并按解析出的IP去重。
// 去重是Mapping交叉验证正确性的前提：如果两个域名解析到同一个目的IP:Port，
// 对称型NAT对"同一目的地址"也会复用同一个映射，
// 交叉验证就会把NAT4误判成EIM(进而误判成NAT2/3)。
func resolveServers(addrs []string) []*net.UDPAddr {
	seen := make(map[string]bool)
	var out []*net.UDPAddr
	for _, a := range addrs {
		udpAddr, err := net.ResolveUDPAddr("udp4", a)
		if err != nil {
			continue // 单个域名解析失败不致命，跳过即可
		}
		if seen[udpAddr.IP.String()] {
			continue
		}
		seen[udpAddr.IP.String()] = true
		out = append(out, udpAddr)
	}
	return out
}

func RunDiagnosis() (*Report, error) {
	report := &Report{}

	// ---- 拓扑层 ----
	// 拓扑信号独立于STUN信号，即使traceroute失败也不影响后续检测，
	// 它的作用是在generateVerdict里区分"多层NAT叠加"和"单路由器行为"
	fmt.Println("[1/5] 扫描链路层级 (traceroute)...")
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
	// 只取IP用于后面的Open Internet判定，Port是临时socket的不能用
	localAddr, _ := getPreferredOutboundAddr()
	if localAddr != nil {
		report.Detail.LocalOutboundIP = localAddr.IP.String()
	}

	servers := resolveServers(stunServers)
	if len(servers) < 2 {
		return nil, errors.New("解析出的不同IP的STUN服务器不足2个，检查DNS/网络连通性")
	}

	// TCP检测与UDP结论互相独立，挂在defer上保证UDP侧任何一条早退路径
	// (UDP被阻断/公网直连/池化NAT)都不会漏掉它——尤其UDP被阻断时，
	// TCP是否可达反而是判断"阻断范围"的有价值旁证
	defer runTCPMappingCheck(report, servers)

	// 所有STUN探测必须复用这一个socket：NAT映射是按(本地IP,本地端口)建立的，
	// 换socket就是换映射，跨服务器对比就没有意义了
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
	if err != nil {
		return nil, fmt.Errorf("本地UDP监听失败: %w", err)
	}
	defer conn.Close()
	report.Detail.LocalPort = conn.LocalAddr().(*net.UDPAddr).Port

	// ---- 基础映射探测 ----
	fmt.Println("[2/5] 探测基础映射 (STUN Binding Request)...")
	res1, err1 := stunQueryRetry(conn, servers[0], false, false, stunTimeout, stunRetries)
	if err1 != nil {
		// 换一台服务器再试，两台(各含重试)都不通才判UDP被阻断，
		// 避免单台服务器故障/丢包造成误判
		res1, err1 = stunQueryRetry(conn, servers[1], false, false, stunTimeout, stunRetries)
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
	// 判定条件：STUN看到的映射地址 == 本机出口IP + conn自己的本地端口。
	// 端口必须用report.Detail.LocalPort(即conn的端口)来比——
	// localAddr.Port是getPreferredOutboundAddr里那个临时Dial socket的随机端口，
	// 和conn毫无关系，拿它比较会导致公网直连永远检测不出来
	if localAddr != nil && localAddr.IP.Equal(res1.MappedAddr.IP) && res1.MappedAddr.Port == report.Detail.LocalPort {
		fmt.Println("[3/5] 映射地址与本机出口地址完全一致，判定为公网直连，继续验证是否存在防火墙限制")
		// 用CHANGE-REQUEST让服务器从另一个地址回包：能收到说明入站无限制
		filterRes, ferr := stunQuery(conn, servers[0], true, true, stunTimeout)
		if ferr == nil && filterRes != nil && !addrEqual(filterRes.RespFrom, servers[0]) {
			report.Summary = LabelOpenInternet
			report.Detail.Filtering = FilteringEIF
			report.Verdict = "可直接使用"
			report.Suggestions = []string{"检测到公网IP且无防火墙限制，可直接对外提供服务，无需打洞"}
		} else {
			// 注意这里无法区分"防火墙拦了"和"服务器不支持CHANGE-REQUEST"，
			// 所以结论偏保守，建议里做了说明
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
	// 核心检测：同一个本地端口问两台不同IP的服务器，对比返回的映射。
	// 映射端口相同 => EIM(锥型)；不同 => 目的地址相关映射(对称型)
	fmt.Println("[3/5] 交叉验证Mapping行为 (更换STUN服务器对比映射端口)...")
	res2, err2 := stunQueryRetry(conn, servers[1], false, false, stunTimeout, stunRetries)
	if err2 != nil && len(servers) >= 3 {
		res2, err2 = stunQueryRetry(conn, servers[2], false, false, stunTimeout, stunRetries)
	}

	if err2 == nil && res2 != nil {
		report.Detail.MappedAddrServer2 = res2.MappedAddr.String()
		fmt.Printf("  服务器2映射地址: %s\n", res2.MappedAddr)

		if !res1.MappedAddr.IP.Equal(res2.MappedAddr.IP) {
			// 连出口IP都不一样，比对称型更糟：运营商在按流负载均衡出口，
			// 对端拿到的地址下一秒就可能失效，打洞没有意义
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
	// 两步测试：
	//   1. 请求换IP+换端口回包(CHANGE-REQUEST)，能收到 => EIF(完全圆锥的过滤特征)
	//   2. 退一步只请求换端口，能收到 => ADF(受限圆锥：认IP不认端口)
	// 两步都失败无法区分"NAT过滤了"和"服务器不支持RFC5780"，只能记未知。
	// 单次探测即可：这本来就是参考信号，不值得为它多花重试时间
	fmt.Println("[4/5] 探测Filtering行为 (CHANGE-REQUEST，多数公网服务器不支持，仅供参考)...")
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

// classify 把Mapping+Filtering两维信号折叠成经典NAT1-4标签。
// 折叠规则(RFC4787行为 -> RFC3489经典分类):
//   - APDM               => NAT4对称型(Filtering已无关紧要，对称型映射本身就断了打洞的路)
//   - EIM + EIF          => NAT1完全圆锥
//   - EIM + ADF          => NAT2受限圆锥
//   - EIM + Filtering未知 => 保守按NAT3处理，不要过度乐观
//     (NAT3是"EIM里最差"的情况，按它规划打洞策略不会翻车，实际可能更好)
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

// generateVerdict 结合NAT标签和链路拓扑信号，产出三态建议。
// 拓扑信号在这里的价值：同样是NAT3/NAT4，
// "多层NAT+CGNAT叠加"和"单台家用路由器"的可优化空间完全不同。
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
	fmt.Println("---- TCP直连能力 ----")
	if report.TCP.Tested {
		fmt.Printf("TCP映射地址(服务器1): %s (本地端口 %d)\n", report.TCP.MappedAddrServer1, report.TCP.LocalPort)
		if report.TCP.MappedAddrServer2 != "" {
			fmt.Printf("TCP映射地址(服务器2): %s\n", report.TCP.MappedAddrServer2)
		}
		fmt.Printf("TCP Mapping行为: %s\n", report.TCP.Mapping)
		fmt.Printf("本地端口复用(SO_REUSEADDR): %v\n", report.TCP.PortReuseOK)
		fmt.Printf("TCP Filtering: %s\n", report.TCP.FilteringNote)
	} else {
		fmt.Println("(所有服务器TCP均不可达，未能检测)")
	}
	fmt.Printf("TCP直连判定: %s\n", report.TCP.DirectFeasibility)

	fmt.Println()
	fmt.Println("---- JSON ----")
	jsonBytes, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(jsonBytes))
}
