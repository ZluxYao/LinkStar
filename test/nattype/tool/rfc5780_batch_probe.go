// rfc5780_batch_probe.go
//
// 批量并发探测STUN服务器列表，判断每台是否真实支持RFC5780的CHANGE-REQUEST机制
//
// 用法:
//
//	go run rfc5780_batch_probe.go                 使用内置默认列表
//	go run rfc5780_batch_probe.go servers.json     使用自定义列表
//	  servers.json 格式: {"stunServerList": ["host:port", "host:port", ...]}
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"sync"
	"text/tabwriter"
	"time"
)

const (
	stunMagicCookie = 0x2112A442

	bindingRequest     = 0x0001
	attrChangeRequest  = 0x0003
	attrChangedAddress = 0x0005 // RFC3489旧属性
	attrXorMapped      = 0x0020
	attrOtherAddress   = 0x802C // RFC5780新属性

	probeTimeout = 3 * time.Second
	concurrency  = 8
)

var defaultServerList = []string{
	"stun.annatel.net:3478",
	"stun.antisip.com:3478",
	"stun.commpeak.com:3478",
	"stun.dcalling.de:3478",
	"stun.freeswitch.org:3478",
	"stun.ipfire.org:3478",
	"stun.sip.us:3478",
	"stun.siplogin.de:3478",
	"stun.sonetel.net:3478",
	"stun.voip.blackberry.com:3478",
	"stun.nextcloud.com:443",
	"stun.flashdance.cx:3478",
	"fwa.lifesizecloud.com:3478",
	"stun.nextcloud.com:3478",
	"stun.radiojar.com:3478",
	"stun.sonetel.com:3478",
	"stun.voipgate.com:3478",
}

type stunAttr struct {
	Type  uint16
	Value []byte
}

func buildBindingRequest(txID [12]byte, changeIP, changePort bool) []byte {
	var attrs []stunAttr
	if changeIP || changePort {
		var flags uint32
		if changeIP {
			flags |= 0x04 // A位: change-IP
		}
		if changePort {
			flags |= 0x02 // B位: change-port
		}
		val := make([]byte, 4)
		binary.BigEndian.PutUint32(val, flags)
		attrs = append(attrs, stunAttr{Type: attrChangeRequest, Value: val})
	}
	var body []byte
	for _, a := range attrs {
		body = append(body, encodeAttr(a)...)
	}
	header := make([]byte, 20)
	binary.BigEndian.PutUint16(header[0:2], bindingRequest)
	binary.BigEndian.PutUint16(header[2:4], uint16(len(body)))
	binary.BigEndian.PutUint32(header[4:8], stunMagicCookie)
	copy(header[8:20], txID[:])
	return append(header, body...)
}

func encodeAttr(a stunAttr) []byte {
	l := len(a.Value)
	padded := (l + 3) / 4 * 4
	buf := make([]byte, 4+padded)
	binary.BigEndian.PutUint16(buf[0:2], a.Type)
	binary.BigEndian.PutUint16(buf[2:4], uint16(l))
	copy(buf[4:4+l], a.Value)
	return buf
}

func parseAttrs(packet []byte) (map[uint16][]byte, error) {
	if len(packet) < 20 {
		return nil, fmt.Errorf("响应包长度不足")
	}
	msgLen := binary.BigEndian.Uint16(packet[2:4])
	if len(packet) < 20+int(msgLen) {
		return nil, fmt.Errorf("响应包被截断")
	}
	attrs := make(map[uint16][]byte)
	body := packet[20 : 20+int(msgLen)]
	i := 0
	for i+4 <= len(body) {
		t := binary.BigEndian.Uint16(body[i : i+2])
		l := binary.BigEndian.Uint16(body[i+2 : i+4])
		i += 4
		if i+int(l) > len(body) {
			break
		}
		attrs[t] = body[i : i+int(l)]
		padded := (int(l) + 3) / 4 * 4
		i += padded
	}
	return attrs, nil
}

func parseXorMappedAddress(val []byte) (string, error) {
	if len(val) < 8 || val[1] != 0x01 {
		return "", fmt.Errorf("不支持或长度不足")
	}
	xport := binary.BigEndian.Uint16(val[2:4])
	port := xport ^ uint16(stunMagicCookie>>16)
	xip := binary.BigEndian.Uint32(val[4:8])
	ip := xip ^ stunMagicCookie
	ipBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(ipBytes, ip)
	return fmt.Sprintf("%s:%d", net.IP(ipBytes).String(), port), nil
}

func parseAddress(val []byte) (string, error) {
	if len(val) < 8 || val[1] != 0x01 {
		return "", fmt.Errorf("不支持或长度不足")
	}
	port := binary.BigEndian.Uint16(val[2:4])
	ip := net.IP(val[4:8])
	return fmt.Sprintf("%s:%d", ip.String(), port), nil
}

func newTxID() [12]byte {
	var id [12]byte
	now := time.Now().UnixNano()
	binary.BigEndian.PutUint64(id[0:8], uint64(now))
	binary.BigEndian.PutUint32(id[8:12], uint32(os.Getpid()))
	return id
}

type probeResult struct {
	Server       string
	Reachable    bool
	MappedAddr   string
	HasOtherAddr bool
	OtherSource  string
	OtherAddr    string
	Step2OK      bool
	Verdict      string
	ErrMsg       string
}

func probeServer(serverStr string) probeResult {
	res := probeResult{Server: serverStr}

	serverAddr, err := net.ResolveUDPAddr("udp4", serverStr)
	if err != nil {
		res.Verdict = "地址解析失败"
		res.ErrMsg = err.Error()
		return res
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
	if err != nil {
		res.Verdict = "本地socket创建失败"
		res.ErrMsg = err.Error()
		return res
	}
	defer conn.Close()

	// ---- Step 1: 普通Binding Request，检查是否声明备用地址 ----
	req1 := buildBindingRequest(newTxID(), false, false)
	if _, err := conn.WriteToUDP(req1, serverAddr); err != nil {
		res.Verdict = "不可达"
		res.ErrMsg = err.Error()
		return res
	}
	conn.SetReadDeadline(time.Now().Add(probeTimeout))
	buf := make([]byte, 1500)
	n, from1, err := conn.ReadFromUDP(buf)
	if err != nil {
		res.Verdict = "不可达(超时)"
		res.ErrMsg = "step1无响应"
		return res
	}
	res.Reachable = true

	attrs1, err := parseAttrs(buf[:n])
	if err != nil {
		res.Verdict = "响应解析失败"
		res.ErrMsg = err.Error()
		return res
	}
	if mapped, ok := attrs1[attrXorMapped]; ok {
		if addr, err := parseXorMappedAddress(mapped); err == nil {
			res.MappedAddr = addr
		}
	}
	if other, ok := attrs1[attrOtherAddress]; ok {
		res.HasOtherAddr = true
		res.OtherSource = "OTHER-ADDRESS"
		if addr, err := parseAddress(other); err == nil {
			res.OtherAddr = addr
		}
	} else if changed, ok := attrs1[attrChangedAddress]; ok {
		res.HasOtherAddr = true
		res.OtherSource = "CHANGED-ADDRESS(旧)"
		if addr, err := parseAddress(changed); err == nil {
			res.OtherAddr = addr
		}
	}

	if !res.HasOtherAddr {
		res.Verdict = "不支持(单地址部署)"
		return res
	}

	// ---- Step 2: 实测CHANGE-REQUEST(仅换端口)是否真的生效 ----
	req2 := buildBindingRequest(newTxID(), false, true)
	if _, err := conn.WriteToUDP(req2, serverAddr); err != nil {
		res.Verdict = "声明支持但Step2发送失败"
		res.ErrMsg = err.Error()
		return res
	}
	conn.SetReadDeadline(time.Now().Add(probeTimeout))
	_, from2, err := conn.ReadFromUDP(buf)
	if err != nil {
		res.Verdict = "声明支持但实测无响应"
		res.ErrMsg = "step2超时"
		return res
	}

	if from2.Port == from1.Port {
		res.Verdict = "声明支持但忽略CHANGE-REQUEST"
		return res
	}

	res.Step2OK = true
	res.Verdict = "支持RFC5780"
	return res
}

func loadServerList() []string {
	if len(os.Args) < 2 {
		return defaultServerList
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Printf("读取文件失败: %v，使用内置默认列表\n", err)
		return defaultServerList
	}
	var parsed struct {
		StunServerList []string `json:"stunServerList"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || len(parsed.StunServerList) == 0 {
		fmt.Printf("解析JSON失败或列表为空: %v，使用内置默认列表\n", err)
		return defaultServerList
	}
	return parsed.StunServerList
}

func main() {
	servers := loadServerList()
	fmt.Printf("待测服务器数量: %d, 并发度: %d, 单次超时: %v\n\n", len(servers), concurrency, probeTimeout)

	results := make([]probeResult, len(servers))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, s := range servers {
		wg.Add(1)
		go func(idx int, server string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = probeServer(server)
		}(i, s)
	}
	wg.Wait()

	// 支持RFC5780的排最前，其次是声明但未验证成功的，最后是完全不可达的
	sort.SliceStable(results, func(i, j int) bool {
		rank := func(r probeResult) int {
			switch {
			case r.Verdict == "支持RFC5780":
				return 0
			case r.HasOtherAddr:
				return 1
			case r.Reachable:
				return 2
			default:
				return 3
			}
		}
		return rank(results[i]) < rank(results[j])
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "服务器\t可达\t备用地址\t判定结果")
	fmt.Fprintln(w, "----\t----\t----\t----")
	supportCount := 0
	for _, r := range results {
		reachable := "✗"
		if r.Reachable {
			reachable = "✓"
		}
		otherAddr := "-"
		if r.OtherAddr != "" {
			otherAddr = r.OtherAddr
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Server, reachable, otherAddr, r.Verdict)
		if r.Verdict == "支持RFC5780" {
			supportCount++
		}
	}
	w.Flush()

	fmt.Printf("\n========================================\n")
	fmt.Printf("真实支持RFC5780完整Filtering检测: %d/%d\n", supportCount, len(servers))
	if supportCount > 0 {
		fmt.Println("\n可用于Filtering Behavior测试的服务器:")
		for _, r := range results {
			if r.Verdict == "支持RFC5780" {
				fmt.Printf("  - %s  (备用地址: %s)\n", r.Server, r.OtherAddr)
			}
		}
	}
}
