package stun

import (
	"strings"
	"time"

	"linkstar/modules/stun/model"
)

// GetObservabilitySnapshot 返回管理后台需要的完整网络观测快照。
// 观测只读取运行时状态，不主动执行 NAT 探测或网络请求。
func GetObservabilitySnapshot() model.ObservabilitySnapshot {
	network := Runtime.NetworkSnapshot()
	udp, tcp := GetNatDetectionResults()

	bestSTUN := Runtime.Config.BestSTUN
	if Runtime.STUNService != nil && Runtime.STUNService.BestSTUNServer != "" {
		bestSTUN = Runtime.STUNService.BestSTUNServer
	}

	trafficInfos := make([]TrafficServiceInfo, 0)
	for _, device := range Runtime.Config.Devices {
		for _, service := range device.Services {
			key := serviceKey(device.DeviceID, service.ID)
			trafficInfos = append(trafficInfos, TrafficServiceInfo{
				Key:         key,
				DeviceName:  device.Name,
				ServiceName: service.Name,
				Protocol:    observationProtocol(service.Protocol),
			})
		}
	}
	traffic := trafficRuntime.snapshot(trafficInfos)

	trafficByKey := make(map[string]model.ServiceTraffic, len(traffic.Services))
	for _, service := range traffic.Services {
		trafficByKey[service.Key] = service
	}

	statusByKey := make(map[string]StateEvent)
	if Runtime.Scheduler != nil {
		for _, status := range Runtime.Scheduler.Snapshot() {
			statusByKey[status.Key] = status
		}
	}

	resources := make([]model.NatResource, 0, len(trafficInfos))
	for _, device := range Runtime.Config.Devices {
		for _, service := range device.Services {
			key := serviceKey(device.DeviceID, service.ID)
			status := statusByKey[key]
			trafficStatus := trafficByKey[key]
			protocol := observationProtocol(service.Protocol)
			source := "NAT 打洞"
			if service.UseUPnP && protocol == "TCP" {
				source = "UPnP + NAT 打洞"
			}
			resources = append(resources, model.NatResource{
				Key:               key,
				DeviceName:        device.Name,
				ServiceName:       service.Name,
				Protocol:          protocol,
				LocalIP:           device.IP,
				LocalPort:         service.InternalPort,
				ExternalIP:        network.PublicIP,
				ExternalPort:      status.ExternalPort,
				Source:            source,
				Status:            status.PhaseStr,
				ActiveConnections: trafficStatus.ActiveConnections,
				LastError:         status.LastError,
				UpdatedAt:         status.UpdatedAt,
			})
		}
	}

	return model.ObservabilitySnapshot{
		SampledAt: time.Now(),
		Network: model.NetworkObservation{
			LocalIP:  network.LocalIP,
			PublicIP: network.PublicIP,
			Iface:    network.Iface,
			Gateway:  network.Gateway,
			BestSTUN: bestSTUN,
		},
		NAT: model.NATObservation{
			Routers:         append([]model.NatRouterInfo(nil), network.NatRouterList...),
			UDP:             udp,
			TCP:             tcp,
			RouterUpdatedAt: network.NatRouterUpdatedAt,
			DetectedAt:      network.NatDetectedAt,
			Resources:       resources,
		},
		Traffic: traffic,
	}
}

func observationProtocol(protocol string) string {
	protocol = strings.ToUpper(strings.TrimSpace(protocol))
	if protocol == "" {
		return "TCP"
	}
	return protocol
}
