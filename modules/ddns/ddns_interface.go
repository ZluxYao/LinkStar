package ddns

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// NetInterface 一块网卡，和它上面能写进 DNS 的地址。
//
// 给「本地网卡」这个 IP 来源的下拉框用：让人看着地址挑，而不是背网卡名。
// 名字打错了不会当场报错，要等到下一轮同步失败才看得见。
type NetInterface struct {
	Name string   `json:"name"`
	IPv4 []string `json:"ipv4"`
	IPv6 []string `json:"ipv6"`
	// HasPublic 这块网卡上有没有公网地址。
	// 只有私有地址（192.168 / fd00::）的网卡也列出来，但得让人知道：
	// 把这种地址写进公网 DNS，外面谁也连不上。
	HasPublic bool `json:"hasPublic"`
}

// usableIfaceIP 这个地址写进 DNS 有没有意义。
//
// 回环和链路本地（fe80::/10、169.254/16）出了本机就不可达，
// 写进公网 DNS 等于让所有人解析到一个连不上的地址。
func usableIfaceIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && ip.IsGlobalUnicast()
}

// ListInterfaces 列出这台机器上地址能用的网卡。
//
// 过滤口径和 resolveFromInterface 是同一个 usableIfaceIP：列表里能选到的，
// 同步时就一定挑得出来；不然用户选了个后端根本不认的网卡，界面上还看不出问题。
func ListInterfaces() ([]NetInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("读取网卡列表失败: %w", err)
	}

	list := make([]NetInterface, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // 没启用的网卡现在没地址，选了也白选
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		// 两个都给空切片：nil 会被序列化成 null，前端拿去 .length 直接炸
		item := NetInterface{Name: iface.Name, IPv4: []string{}, IPv6: []string{}}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || !usableIfaceIP(ipNet.IP) {
				continue
			}
			ip := ipNet.IP
			if !ip.IsPrivate() {
				item.HasPublic = true
			}
			if ip.To4() != nil {
				item.IPv4 = append(item.IPv4, ip.String())
			} else {
				item.IPv6 = append(item.IPv6, ip.String())
			}
		}
		if len(item.IPv4) == 0 && len(item.IPv6) == 0 {
			continue // 一个能用的地址都没有，列出来只会让人选错
		}
		list = append(list, item)
	}

	// 有公网地址的排前面：绝大多数情况下要挑的就是那一块
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].HasPublic != list[j].HasPublic {
			return list[i].HasPublic
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	return list, nil
}
