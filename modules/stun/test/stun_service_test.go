package test

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"sync"
	"testing"
	"time"

	"linkstar/modules/stun"

	"github.com/libp2p/go-reuseport"
	pionstun "github.com/pion/stun"
)

type stunResult struct {
	tcpIP string
	udpIP string
}

func TestReuseportUDPWrite(t *testing.T) {
	srv := "stun.miwifi.com:3478"
	localIP := "0.0.0.0"
	localAddr := fmt.Sprintf("%s:0", localIP)

	conn, err := reuseport.Dial("udp", localAddr, srv)
	if err != nil {
		t.Skipf("网络不可达，跳过: %v", err)
	}
	defer conn.Close()

	t.Logf("conn 1类型: %T", conn) // 看看实际类型

	ip, port, err := stun.DoStunHandshake(conn)
	if err != nil {
		t.Fatalf("reuseport UDP STUN 失败: %v", err)
	}
	t.Logf("[reuseport UDP] 公网地址: %s:%d", ip, port)
}

func TestAllSTUNServers(t *testing.T) {
	cfg, err := stun.ReadConfig()
	fmt.Println(cfg)
	if err != nil {
		t.Fatalf("1read 1stun config: %v", err)
	}

	var mu sync.Mutex
	results := make(map[string]*stunResult, len(cfg.StunServerList))
	for _, srv := range cfg.StunServerList {
		results[srv] = &stunResult{}
	}

	for _, proto := range []string{"tcp", "udp"} {
		for _, srv := range cfg.StunServerList {
			proto, srv := proto, srv
			t.Run(proto+"/"+srv, func(t *testing.T) {
				t.Parallel()
				ip, port, delay, err := queryStun(proto, srv)
				if err != nil {
					t.Logf("[FAIL] %s %s err=%v", proto, srv, err)
					return
				}
				t.Logf("[ OK ] %s %s -> %s:%d  %v", proto, srv, ip, port, delay.Round(time.Millisecond))
				mu.Lock()
				if proto == "tcp" {
					results[srv].tcpIP = ip
				} else {
					results[srv].udpIP = ip
				}
				mu.Unlock()
			})
		}
	}

	t.Cleanup(func() {
		type badEntry struct {
			srv    string
			reason string
		}
		var good []string
		var bads []badEntry
		for _, srv := range cfg.StunServerList {
			r := results[srv]
			switch {
			case r.tcpIP == "" && r.udpIP == "":
				bads = append(bads, badEntry{srv, "tcp+udp 均失败"})
			case r.tcpIP == "":
				bads = append(bads, badEntry{srv, "tcp 失败 udp=" + r.udpIP})
			case r.udpIP == "":
				bads = append(bads, badEntry{srv, "udp 失败 tcp=" + r.tcpIP})
			case r.tcpIP != r.udpIP:
				bads = append(bads, badEntry{srv, fmt.Sprintf("ip 不一致 tcp=%s udp=%s", r.tcpIP, r.udpIP)})
			default:
				good = append(good, srv)
			}
		}

		fmt.Println("\n====== TCP/UDP 均通过且 IP 一致 ======")
		out, _ := json.MarshalIndent(map[string][]string{"stunServerList": good}, "", "  ")
		fmt.Println(string(out))

		fmt.Println("\n====== 其他服务器 ======")
		sort.Slice(bads, func(i, j int) bool { return bads[i].srv < bads[j].srv })
		for _, b := range bads {
			fmt.Printf("  %-35s %s\n", b.srv, b.reason)
		}
	})
}

func queryStun(protocol, server string) (string, int, time.Duration, error) {
	start := time.Now()
	conn, err := net.DialTimeout(protocol, server, 3*time.Second)
	if err != nil {
		return "", 0, 0, err
	}
	defer conn.Close()

	msg := pionstun.MustBuild(pionstun.TransactionID, pionstun.BindingRequest)
	if _, err := conn.Write(msg.Raw); err != nil {
		return "", 0, 0, err
	}
	conn.SetDeadline(time.Now().Add(3 * time.Second))

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return "", 0, 0, err
	}

	var resp pionstun.Message
	resp.Raw = buf[:n]
	if err := resp.Decode(); err != nil {
		return "", 0, 0, err
	}

	var xorAddr pionstun.XORMappedAddress
	if err := xorAddr.GetFrom(&resp); err != nil {
		return "", 0, 0, err
	}
	return xorAddr.IP.String(), xorAddr.Port, time.Since(start), nil
}
