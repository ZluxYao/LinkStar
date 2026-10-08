package stun_api

import (
	"linkstar/middleware"
	"linkstar/modules/stun"
	"linkstar/modules/stun/model"
	"linkstar/utils/res"
	"strings"

	"github.com/gin-gonic/gin"
)

// StunNetworkView 网络设置页要的全部：当前设置、本机网卡、内置 DNS、现在实际用的出口
type StunNetworkView struct {
	Config     model.NetworkConfig  `json:"config"`
	Ifaces     []stun.OutboundIface `json:"ifaces"`
	DefaultDNS []string             `json:"defaultDns"`
	Current    stun.OutboundIface   `json:"current"`
}

// GetStunNetworkView 读网络设置和本机网卡列表
func (StunApi) GetStunNetworkView(c *gin.Context) {
	ifaces, err := stun.ListOutboundIfaces()
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	if ifaces == nil {
		ifaces = []stun.OutboundIface{} // nil 序列化成 null，前端 .map 直接炸
	}
	cfg := stun.Runtime.Config.Network
	if cfg.DNS == nil {
		cfg.DNS = []string{}
	}
	res.OkWithData(StunNetworkView{
		Config:     cfg,
		Ifaces:     ifaces,
		DefaultDNS: stun.DefaultDNSServers,
		Current:    stun.CurrentOutboundIface(),
	}, c)
}

type StunNetworkUpdateRequest struct {
	IfaceMode string   `json:"ifaceMode"`
	Iface     string   `json:"iface"`
	DNSMode   string   `json:"dnsMode"`
	DNS       []string `json:"dns"`
}

// StunNetworkUpdateView 保存网络设置，立刻生效：出口变了会重启所有打洞服务
func (StunApi) StunNetworkUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[StunNetworkUpdateRequest](c)

	cfg := model.NetworkConfig{IfaceMode: cr.IfaceMode, DNSMode: cr.DNSMode}
	switch cr.IfaceMode {
	case model.ModeAuto, model.ModeSystem:
	case model.ModeCustom:
		cfg.Iface = strings.TrimSpace(cr.Iface)
		if cfg.Iface == "" {
			res.FailWithMsg("选「指定网卡」时要选一张网卡", c)
			return
		}
	default:
		res.FailWithMsg("出口网卡的模式不对", c)
		return
	}

	switch cr.DNSMode {
	case model.ModeAuto, model.ModeSystem:
	case model.ModeCustom:
		for _, s := range cr.DNS {
			if strings.TrimSpace(s) == "" {
				continue
			}
			server, err := stun.NormalizeDNSServer(s)
			if err != nil {
				res.FailWithMsg(err.Error(), c)
				return
			}
			cfg.DNS = append(cfg.DNS, server)
		}
		if len(cfg.DNS) == 0 {
			res.FailWithMsg("选「自定义」时至少填一个 DNS 地址", c)
			return
		}
	default:
		res.FailWithMsg("DNS 的模式不对", c)
		return
	}

	stun.Runtime.Config.Network = cfg
	if err := stun.UpdateConfig(stun.Runtime.Config); err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}

	// 设置先落盘再生效：指定的网卡暂时没连上也照样存，连上后会自动切过去
	outbound, err := stun.ApplyNetworkConfig(cfg)
	if err != nil {
		res.OkWithMsg("已保存，但现在用不了："+err.Error()+"。打洞服务会停着，等它连上自动恢复", c)
		return
	}
	res.OkWithMsg("已保存，打洞改从 "+outbound.String()+" 出去", c)
}
