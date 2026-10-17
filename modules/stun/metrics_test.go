package stun

import (
	"testing"
	"time"
)

func TestBytesPerSecond表格(t *testing.T) {
	tests := []struct {
		name    string
		delta   uint64
		elapsed time.Duration
		want    uint64
	}{
		{name: "没有新增流量", delta: 0, elapsed: time.Second, want: 0},
		{name: "一秒一千字节", delta: 1000, elapsed: time.Second, want: 1000},
		{name: "半秒两千字节", delta: 2000, elapsed: 500 * time.Millisecond, want: 4000},
		{name: "没有采样间隔", delta: 1000, elapsed: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bytesPerSecond(tt.delta, tt.elapsed); got != tt.want {
				t.Fatalf("bytesPerSecond(%d, %v) = %d，想要 %d", tt.delta, tt.elapsed, got, tt.want)
			}
		})
	}
}

func TestTrafficRegistry快照包含累计流量和活跃连接(t *testing.T) {
	registry := newTrafficRegistry()
	info := TrafficServiceInfo{
		Key:         "1-2",
		DeviceName:  "本机",
		ServiceName: "网页",
		Protocol:    "TCP",
	}
	meter := registry.meter(info)
	meter.addInbound(1024)
	meter.addOutbound(2048)
	meter.beginConnection()

	result := registry.snapshot([]TrafficServiceInfo{info})
	if result.InBytes != 1024 || result.OutBytes != 2048 {
		t.Fatalf("累计流量 = %d/%d，想要 1024/2048", result.InBytes, result.OutBytes)
	}
	if result.ActiveConnections != 1 || len(result.Services) != 1 {
		t.Fatalf("连接数或服务数不对：active=%d services=%d", result.ActiveConnections, len(result.Services))
	}
	if result.Services[0].ServiceName != "网页" {
		t.Fatalf("服务名称 = %q，想要 网页", result.Services[0].ServiceName)
	}

	meter.endConnection()
	result = registry.snapshot([]TrafficServiceInfo{info})
	if result.ActiveConnections != 0 {
		t.Fatalf("连接关闭后 active = %d，想要 0", result.ActiveConnections)
	}
}
