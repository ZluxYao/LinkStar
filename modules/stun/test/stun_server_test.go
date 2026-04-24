package test

import (
	"fmt"
	"net"
	"testing"
	"time"

	stunpkg "linkstar/modules/stun"

	"github.com/pion/stun"
)

// 公共测试用的 STUN 服务器列表
var testServers = []string{
	"stun.radiojar.com:3478",
	"stun.ringostat.com:3478",
	"stun.irishvoip.com:3478",
	"stun.voipgate.com:3478",
	"stun.tula.nu:3478",
	"stun.yesdates.com:3478",
	"stun.telnyx.com:3478",
	"stun.vavadating.com:3478",
	"stun.bau-ha.us:3478",
	"stun.bridesbay.com:3478",
	"stun.3wayint.com:3478",
	"stun.finsterwalder.com:3478",
	"stun.romaaeterna.nl:3478",
	"stun.fitauto.ru:3478",
	"stun.antisip.com:3478",
	"stun.heeds.eu:3478",
	"stun.hot-chilli.net:3478",
	"stun.eurosys.be:3478",
	"stun.vincross.com:3478",
	"stun.cibercloud.com.br:3478",
	"stun.siptrunk.com:3478",
}

// 测试 NewSTUNService 初始化和 GetBestSTUNServer
func TestNewSTUNService(t *testing.T) {
	svc := stunpkg.NewSTUNService(testServers)

	best, err := svc.GetBestSTUNServer()
	if err != nil {
		t.Logf("GetBestSTUNServer error: %v", err)
	} else {
		t.Logf("BestSTUNServer: %s", best)
	}

	t.Logf("AvailableSTUNServers: %v", svc.AvailableSTUNServers)
}

// 测试 GetBackupServer
func TestGetBackupServer(t *testing.T) {
	svc := stunpkg.NewSTUNService(testServers)

	backup, err := svc.GetBackupServer()
	if err != nil {
		t.Logf("GetBackupServer error: %v", err)
		return
	}
	t.Logf("BackupServer: %s", backup)
}

// 测试 UpdateSTUNService 并发安全（连续调用不应 panic）
func TestUpdateSTUNService_Concurrent(t *testing.T) {
	svc := stunpkg.NewSTUNService(testServers)

	done := make(chan struct{})
	for i := 0; i < 5; i++ {
		go func() {
			svc.UpdateSTUNService()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 5; i++ {
		<-done
	}
	t.Logf("并发 Update 后 BestSTUNServer: %s", svc.BestSTUNServer)
}

// 手动拨一个 STUN 服务，打印映射的公网地址
func TestRawSTUNBinding(t *testing.T) {
	srv := "stun.radiojar.com:3478"

	conn, err := net.DialTimeout("tcp", srv, 3*time.Second)
	if err != nil {
		t.Skipf("网络不可达，跳过: %v", err)
	}
	defer conn.Close()

	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if _, err = conn.Write(msg.Raw); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("read: %v", err)
	}

	// 解析响应
	resp := &stun.Message{Raw: buf[:n]}
	if err = resp.Decode(); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var xorAddr stun.XORMappedAddress
	if err = xorAddr.GetFrom(resp); err != nil {
		t.Logf("XORMappedAddress 不可用: %v", err)
	} else {
		t.Logf("公网地址: %s", xorAddr.String())
	}

	fmt.Printf("[TestRawSTUNBinding] 公网地址 = %s\n", xorAddr.String())
}
