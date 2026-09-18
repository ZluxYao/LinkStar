package main

import (
	"embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"linkstar/core"
	"linkstar/modules/auth"
	"linkstar/modules/cert"
	"linkstar/modules/ddns"
	"linkstar/modules/ddns/dns"
	"linkstar/modules/home"
	"linkstar/modules/proxy"
	"linkstar/modules/stun"
	"linkstar/modules/webhook"
	"linkstar/routers"

	"github.com/sirupsen/logrus"
)

const (
	appName    = "LinkStar"
	backendURL = "http://127.0.0.1:3333"
	adminURL   = backendURL + "/linkstar/"
	homeURL    = backendURL + "/"
)

//go:embed web/admin/dist web/home/dist
var webFS embed.FS

func initRuntime() {
	// 设置时区
	os.Setenv("TZ", "Asia/Shanghai")
	core.InitLogger()
	logrus.Info("LinkStar Run")
	core.ListenShutdown()
}

func startBackend(webFS embed.FS) (bool, error) {
	if isLinkStarBackendReady() {
		logrus.Infof("后端已在 %s 运行", backendURL)
		return false, nil
	}

	go routers.Run(webFS)

	if err := waitForBackend(); err != nil {
		return false, err
	}

	startModulesInBackground()
	return true, nil
}

// waitForBackend 轮询等待后端就绪，超时返回 error。
func waitForBackend() error {
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("后端在 %s 未能就绪", backendURL)
		case <-ticker.C:
			if isLinkStarBackendReady() {
				return nil
			}
		}
	}
}

// isLinkStarBackendReady 探测本机后端是否已经启动并返回 LinkStar 页面。
func isLinkStarBackendReady() bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:3333", 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()

	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(adminURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	if err != nil {
		return false
	}
	return strings.Contains(string(body), "LinkStar")
}

// startModulesInBackground 后台并发初始化各业务模块，互不阻塞。
func startModulesInBackground() {
	registerCertDNSSolver()

	initModule := func(name string, fn func() error) {
		go func() {
			logrus.Infof("%s 模块开始初始化", name)
			if err := fn(); err != nil {
				logrus.Errorf("%s 模块初始化失败：%v", name, err)
				return
			}
			logrus.Infof("%s 模块初始化完成", name)
		}()
	}

	initModule("Auth", auth.InitAuth)
	initModule("Home", home.InitHome)
	initModule("Webhook", webhook.InitWebhook)
	// Cert 排在 STUN 前面：洞口终结 TLS 时要用它的证书
	initModule("Cert", cert.InitCert)
	initModule("STUN", stun.InitSTUN)
	initModule("DDNS", ddns.DDNSInit)
	initModule("Proxy", proxy.InitProxy)
}

// registerCertDNSSolver 把 DDNS 的 TXT 写入能力注入证书模块。
//
// modules/ddns 已经 import 了 modules/stun，而 stun 要 import cert，
// 所以 cert 必须是叶子包、不能直接 import ddns。cert 只定义一个窄接口，
// 由这里（同时能看见两边）在启动时注入实现，依赖图保持无环。
//
// 工厂体内部到真正签发时才会调用 ddns，所以此处同步注册不依赖 DDNS 已初始化。
func registerCertDNSSolver() {
	cert.RegisterDNSSolverFactory(func(id uint) (cert.TXTWriter, error) {
		p, ok := ddns.Runtime.FindProvider(id)
		if !ok {
			return nil, fmt.Errorf("DNS 服务商 %d 不存在，请先在 DDNS 里配置", id)
		}
		w := dns.BuildACMEClient(p)
		if w == nil {
			return nil, fmt.Errorf("服务商 %s 暂不支持 DNS-01", p.Type)
		}
		return w, nil
	})
}
