package stun

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 局域网扫描。
//
// 「添加设备」现在要求用户自己知道那台机器的内网 IP——得去路由器后台翻 DHCP 列表，
// 或者跑去那台机器上敲一行命令。卡在这一步的人，后面打洞的事根本轮不到。
// 这里把「这段网里有哪些机器、各自开着什么端口」直接扫出来摆着选。

const (
	// lanScanMaxHosts 一次最多试多少个地址。
	//
	// 家里的网基本都是 /24，254 个地址。真碰上 /16 的（65534 个）扫完要几分钟，
	// 界面只能一直转圈，所以挑网段那一步就已经收窄成 /24，这里只是兜底。
	lanScanMaxHosts = 1024

	// lanScanWorkers 同时开多少个连接。
	//
	// 往上加能更快，但家用路由器的会话表就那么大，几千条并发短连接
	// 能把便宜路由器的 NAT 表顶满，表现是全家断网几秒——这比慢两秒严重得多。
	lanScanWorkers = 256

	// 局域网里一次往返一般不到 5 毫秒，这里留几百毫秒是给无线丢包和
	// 省电模式下慢半拍的设备。
	//
	// 整趟扫描的耗时基本等于「探活端口数 × lanAliveTimeout」——在线的机器几毫秒就回话了，
	// 时间全花在等那两百多个空地址上。所以这两个数字往上调一点，用户就要多等一整秒。
	lanAliveTimeout = 450 * time.Millisecond
	lanPortTimeout  = 400 * time.Millisecond
	lanNameTimeout  = 800 * time.Millisecond
)

// lanAliveProbePorts 先用这几个端口判断「这个地址上有没有机器」。
//
// 不挨个 ping：Windows 上发 ICMP 要管理员权限，而 LinkStar 是双击就能跑的。
// 改成 TCP 连过去，只要对方回了话就算数——连上了说明端口开着，
// 被拒了说明端口关着，两种都证明那台机器在。
//
// 扫不到的是防火墙设成「什么都不回」的机器（Windows 默认就这样）。
// 这是这种做法的边界，界面上得说出来，不然用户会以为设备不在网里。
var lanAliveProbePorts = []uint16{80, 443, 22, 445, 8080}

// lanPorts 确认机器活着之后再挨个试的端口，附带认得出来的名字。
//
// 加设备的下一步就是挑一个端口打洞。端口号在这儿直接看见，
// 就省了再跑去那台机器上翻一遍的功夫。
var lanPorts = []struct {
	Port uint16
	Name string
}{
	{22, "SSH"},
	{53, "DNS"},
	{80, "网页"},
	{139, "Windows 共享"},
	{443, "网页 HTTPS"},
	{445, "Windows 共享"},
	{548, "苹果共享"},
	{631, "打印机"},
	{873, "rsync"},
	{1883, "MQTT"},
	{2049, "NFS"},
	{3000, "网页应用"},
	{3389, "远程桌面"},
	{5000, "群晖 DSM"},
	{5001, "群晖 DSM"},
	{5244, "Alist"},
	{6690, "群晖 Drive"},
	{8006, "PVE"},
	{8080, "网页"},
	{8096, "Jellyfin"},
	{8123, "Home Assistant"},
	{8443, "网页 HTTPS"},
	{9000, "Portainer"},
	{9090, "Cockpit"},
	{32400, "Plex"},
}

// LanSubnet 本机接在哪个局域网上，也就是能扫哪一段。
type LanSubnet struct {
	CIDR string `json:"cidr"`
	// Iface 网卡名。同一段可能挂在好几块网卡上（无线 + 有线），只留第一块
	Iface string `json:"iface"`
	// LocalIP 本机在这一段上的地址，用来在结果里标出「这台就是我自己」
	LocalIP string `json:"localIP"`
	// Hosts 这一段要试多少个地址，用户点之前能估出要等多久
	Hosts int `json:"hosts"`
}

// LanPort 一台机器上开着的一个端口
type LanPort struct {
	Port uint16 `json:"port"`
	// Name 认得出来的名字，如「SSH」「群晖 DSM」；认不出来就是空
	Name string `json:"name"`
}

// LanHost 扫出来的一台机器
type LanHost struct {
	IP string `json:"ip"`
	// Name 反查到的机器名。家用路由器一般会把 DHCP 里登记的名字告诉你，
	// 查不到就是空——那是路由器不提供，不代表机器有问题
	Name  string    `json:"name"`
	Ports []LanPort `json:"ports"`
	// Self 就是跑着 LinkStar 的这台
	Self bool `json:"self"`
	// Added 设备列表里已经有这个 IP 了，别让人重复添加
	Added bool `json:"added"`
	// AddedName 已经添加过的话，当时起的名字
	AddedName string `json:"addedName"`
}

// scannableIP 这个地址能不能扫。
//
// 只认 RFC1918 那三段私有地址。运营商给的 100.64/10（CGNAT）虽然也连得通，
// 但那一段里住的是同一个片区的其他宽带用户，不是自己家的设备——
// 扫它既扫不出有用的东西，又是在敲别人家的门。
func scannableIP(ip net.IP) bool {
	ip4 := ip.To4()
	return ip4 != nil && ip4.IsPrivate()
}

// narrowTo24 网段比 /24 还大时，只取本机所在的那个 /24。
//
// 家里少见，但 10.0.0.0/8 这种配法是存在的，照原样扫是 1600 多万个地址。
func narrowTo24(ip net.IP, mask net.IPMask) *net.IPNet {
	ones, bits := mask.Size()
	if bits != 32 {
		ones = 24
	} else if ones < 24 {
		ones = 24
	}
	m := net.CIDRMask(ones, 32)
	return &net.IPNet{IP: ip.Mask(m), Mask: m}
}

// subnetHostCount 这一段里要挨个试的地址个数（去掉网络号和广播地址）
func subnetHostCount(n *net.IPNet) int {
	ones, bits := n.Mask.Size()
	if bits != 32 || ones > 30 {
		return 0
	}
	return 1<<(32-ones) - 2
}

// ListLanSubnets 本机接在哪几个局域网上。
//
// 给扫描前的下拉框用：有线、无线、Docker 虚拟网桥可能同时在，
// 让人照着地址挑，而不是背网卡名。
func ListLanSubnets() []LanSubnet {
	ifaces, err := net.Interfaces()
	if err != nil {
		return []LanSubnet{}
	}

	out := []LanSubnet{}
	seen := map[string]bool{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || !scannableIP(ipNet.IP) {
				continue
			}
			ip4 := ipNet.IP.To4()
			n := narrowTo24(ip4, ipNet.Mask)
			cidr := n.String()
			if seen[cidr] {
				continue // 同一段挂在两块网卡上，扫一遍就够
			}
			seen[cidr] = true
			out = append(out, LanSubnet{
				CIDR:    cidr,
				Iface:   iface.Name,
				LocalIP: ip4.String(),
				Hosts:   subnetHostCount(n),
			})
		}
	}

	// 本机主 IP 所在的那段排最前：绝大多数时候要扫的就是它
	local := strings.TrimSpace(Runtime.Network.LocalIP)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LocalIP == local && out[j].LocalIP != local
	})
	return out
}

// localIPv4s 本机自己占着的所有内网地址。
//
// 不能只认 Runtime.Network.LocalIP：装了 VMware / Hyper-V / WSL 的机器上，
// 每块虚拟网卡都给本机分了一个地址。扫到那段虚拟网时，本机的那个地址
// 会以一台普通设备的样子冒出来，用户照着加进设备列表，其实加的是自己。
func localIPv4s() map[string]bool {
	out := map[string]bool{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && scannableIP(ipNet.IP) {
				out[ipNet.IP.To4().String()] = true
			}
		}
	}
	if ip := strings.TrimSpace(Runtime.Network.LocalIP); ip != "" {
		out[ip] = true
	}
	return out
}

// isAliveErr 连接失败，但失败的方式证明那台机器在。
//
// 被拒和被重置都是对方主动回的话；超时、「主机不可达」则是没人应。
// 错误码按平台分开判（见 lan_scan_windows.go / lan_scan_other.go），
// 不能靠错误信息里的字：Windows 的系统报错是按系统语言翻译过的，
// 中文系统上是「由于目标计算机积极拒绝」，匹配 refused 一个都对不上。

// probePort 试一个端口：开着没有，以及这台机器在不在
func probePort(ctx context.Context, ip string, port uint16, timeout time.Duration) (open, alive bool) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp4", net.JoinHostPort(ip, strconv.Itoa(int(port))))
	if err == nil {
		_ = conn.Close()
		return true, true
	}
	return false, isAliveErr(err)
}

// runJobs 开固定数量的 worker 跑完一批活
func runJobs(n int, jobs []func()) {
	if len(jobs) == 0 {
		return
	}
	ch := make(chan func())
	var wg sync.WaitGroup
	if n > len(jobs) {
		n = len(jobs)
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range ch {
				f()
			}
		}()
	}
	for _, f := range jobs {
		ch <- f
	}
	close(ch)
	wg.Wait()
}

// subnetIPs 这一段里要挨个试的地址
func subnetIPs(n *net.IPNet) []string {
	ones, bits := n.Mask.Size()
	if bits != 32 {
		return nil
	}
	base := n.IP.Mask(n.Mask).To4()
	if base == nil {
		return nil
	}
	total := 1 << (32 - ones)
	start, end := 1, total-1 // 掐掉网络号和广播地址，那两个上面没有机器
	if total <= 2 {
		start, end = 0, total
	}

	out := make([]string, 0, end-start)
	v := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
	for i := start; i < end; i++ {
		x := v + uint32(i)
		out = append(out, fmt.Sprintf("%d.%d.%d.%d", byte(x>>24), byte(x>>16), byte(x>>8), byte(x)))
	}
	return out
}

// ScanLan 扫一个网段，把在线的机器和它们开着的端口找出来。
//
// 分两步走而不是对每个地址都把端口表跑一遍：先用 5 个端口判断在不在（254 个地址
// 一秒出头），只有活着的那十来台才挨个试完整的端口表。全表硬扫要多花十几秒，
// 而扫不到的机器多试 20 个端口一样扫不到。
func ScanLan(ctx context.Context, cidr string) ([]LanHost, error) {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		subnets := ListLanSubnets()
		if len(subnets) == 0 {
			return nil, errors.New("这台机器没接在任何局域网上")
		}
		cidr = subnets[0].CIDR
	}

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("网段写得不对: %w", err)
	}
	if !scannableIP(ipNet.IP) {
		return nil, errors.New("只能扫自己家的内网（10.x / 172.16-31.x / 192.168.x）")
	}
	targets := subnetIPs(ipNet)
	if len(targets) == 0 {
		return nil, errors.New("这个网段里没有可扫的地址")
	}
	if len(targets) > lanScanMaxHosts {
		return nil, fmt.Errorf("这个网段有 %d 个地址，太大了，最多一次扫 %d 个——填成 /24 试试",
			len(targets), lanScanMaxHosts)
	}

	// 第一步：谁在
	var mu sync.Mutex
	alive := make([]string, 0, 32)
	probes := make([]func(), 0, len(targets))
	for _, ip := range targets {
		probes = append(probes, func() {
			for _, p := range lanAliveProbePorts {
				if ctx.Err() != nil {
					return
				}
				if _, ok := probePort(ctx, ip, p, lanAliveTimeout); ok {
					mu.Lock()
					alive = append(alive, ip)
					mu.Unlock()
					return
				}
			}
		})
	}
	runJobs(lanScanWorkers, probes)
	if err := ctx.Err(); err != nil {
		return nil, errors.New("扫描被取消")
	}

	// 本机自己不一定被上面扫出来（往自己身上连多半是拒绝，但也可能整台都不回），
	// 而「本机」恰恰是最常要添加的那一台
	selfIPs := localIPv4s()
	inAlive := make(map[string]bool, len(alive))
	for _, ip := range alive {
		inAlive[ip] = true
	}
	for ip := range selfIPs {
		if !inAlive[ip] && ipNet.Contains(net.ParseIP(ip)) {
			alive = append(alive, ip)
		}
	}

	// 第二步：活着的这几台各开着什么端口，叫什么名字
	hosts := make([]LanHost, len(alive))
	jobs := make([]func(), 0, len(alive)*(len(lanPorts)+1))
	for i, ip := range alive {
		hosts[i] = LanHost{IP: ip, Ports: []LanPort{}, Self: selfIPs[ip]}

		jobs = append(jobs, func() {
			nctx, cancel := context.WithTimeout(ctx, lanNameTimeout)
			defer cancel()
			names, err := net.DefaultResolver.LookupAddr(nctx, ip)
			if err != nil || len(names) == 0 {
				return
			}
			mu.Lock()
			hosts[i].Name = strings.TrimSuffix(names[0], ".")
			mu.Unlock()
		})

		for _, p := range lanPorts {
			jobs = append(jobs, func() {
				if open, _ := probePort(ctx, ip, p.Port, lanPortTimeout); open {
					mu.Lock()
					hosts[i].Ports = append(hosts[i].Ports, LanPort{Port: p.Port, Name: p.Name})
					mu.Unlock()
				}
			})
		}
	}
	runJobs(lanScanWorkers, jobs)

	// 已经在设备列表里的标出来：不标的话用户会照着扫描结果又加一遍，
	// 列表里就出现两个同 IP 的设备，而这个错要等服务加错地方才发现
	for i := range hosts {
		for _, d := range Runtime.Config.Devices {
			if strings.TrimSpace(d.IP) == hosts[i].IP {
				hosts[i].Added = true
				hosts[i].AddedName = d.Name
				break
			}
		}
		sort.Slice(hosts[i].Ports, func(a, b int) bool {
			return hosts[i].Ports[a].Port < hosts[i].Ports[b].Port
		})
	}
	sort.Slice(hosts, func(a, b int) bool {
		return ipLess(hosts[a].IP, hosts[b].IP)
	})
	return hosts, nil
}

// ipLess 按地址数值排，不按字符串排。
// 按字符串排的话 192.168.1.10 会跑到 192.168.1.9 前面，看着像乱的。
func ipLess(a, b string) bool {
	ia, ib := net.ParseIP(a).To4(), net.ParseIP(b).To4()
	if ia == nil || ib == nil {
		return a < b
	}
	for i := 0; i < 4; i++ {
		if ia[i] != ib[i] {
			return ia[i] < ib[i]
		}
	}
	return false
}
