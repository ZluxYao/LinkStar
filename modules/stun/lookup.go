package stun

import (
	"fmt"
	"strings"

	"linkstar/modules/stun/model"
)

// PublicEndpoint 一个服务当前的对外访问点。
// 端口取自调度器的实时状态，因此 STUN 端口漂移后这里立刻是新值。
type PublicEndpoint struct {
	Scheme string // "http" / "https"
	Host   string // 服务配置的域名，留空则回落公网 IP
	Port   uint16 // 当前外部端口
}

// Valid 是否拿到了可用的访问点
func (e PublicEndpoint) Valid() bool {
	return e.Host != "" && e.Port != 0
}

// URL 拼成 https://fw.example.com:34521 形式（不带末尾斜杠）
func (e PublicEndpoint) URL() string {
	if !e.Valid() {
		return ""
	}
	return fmt.Sprintf("%s://%s:%d", e.Scheme, e.Host, e.Port)
}

// FindDeviceService 按设备 ID + 服务 ID 定位服务
func FindDeviceService(deviceID, serviceID uint) (*model.Device, *model.Service) {
	for i := range Runtime.Config.Devices {
		device := &Runtime.Config.Devices[i]
		if device.DeviceID != deviceID {
			continue
		}
		for j := range device.Services {
			if device.Services[j].ID == serviceID {
				return device, &device.Services[j]
			}
		}
		return device, nil
	}
	return nil, nil
}

// FindServiceByName 按服务名定位服务（不区分大小写），供 307 服务索引使用
func FindServiceByName(name string) (*model.Device, *model.Service) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	for i := range Runtime.Config.Devices {
		device := &Runtime.Config.Devices[i]
		for j := range device.Services {
			if strings.EqualFold(device.Services[j].Name, name) {
				return device, &device.Services[j]
			}
		}
	}
	return nil, nil
}

// LiveExternalPort 取服务当前的外部端口：优先调度器实时值，回落 UPnP 映射端口
func LiveExternalPort(deviceID uint, service *model.Service) uint16 {
	if service == nil {
		return 0
	}
	if Runtime.Scheduler != nil {
		if ev, ok := Runtime.Scheduler.Get(deviceID, service.ID); ok && ev.ExternalPort != 0 {
			return ev.ExternalPort
		}
	}
	return service.UPnPMappedPort
}

// ServiceEndpoint 计算服务当前的对外访问点
func ServiceEndpoint(deviceID uint, service *model.Service) PublicEndpoint {
	if service == nil {
		return PublicEndpoint{}
	}

	host := strings.TrimSpace(service.Domain)
	if host == "" {
		host = Runtime.Network.PublicIP
	}

	return PublicEndpoint{
		Scheme: PublicScheme(service),
		Host:   host,
		Port:   LiveExternalPort(deviceID, service),
	}
}

// PublicScheme 外网访问该服务时的协议。
//
// 区别只在「洞口那一层 TLS 是谁的」，三种情况：
//   - 终结 TLS：洞口自己出示证书，外面一定是 https，内网说不说 TLS 都无关
//   - 不终结、但勾了内网 HTTPS：洞口把外面进来的明文字节包进自己那条 TLS 再送进内网，
//     外面那一段反而是明文。此时按 https 给链接，浏览器只会得到 ERR_SSL_PROTOCOL_ERROR
//   - 两个都不勾：纯字节管道，外面看到的就是内网服务本身，只能按展示开关 Https 说
func PublicScheme(service *model.Service) string {
	if service == nil {
		return "http"
	}
	if service.TLSTerminate {
		return "https"
	}
	if !service.BackendHTTPS && service.Https {
		return "https"
	}
	return "http"
}

// InternalScheme 内网直连该服务时的协议，只取决于服务本身。
// Https 是老的展示开关，BackendHTTPS 才是「它真的说 TLS」，两者任一为真都按 https 展示。
func InternalScheme(service *model.Service) string {
	if service != nil && (service.BackendHTTPS || service.Https) {
		return "https"
	}
	return "http"
}

// ServiceOnline 服务当前是否处于运行阶段
func ServiceOnline(deviceID, serviceID uint) (online bool, known bool) {
	if Runtime.Scheduler == nil {
		return false, false
	}
	ev, ok := Runtime.Scheduler.Get(deviceID, serviceID)
	if !ok {
		return false, true
	}
	return ev.Phase == PhaseRunning, true
}
