# LinkStar 代码审查与后续规划

> 生成于 2026-07-26，基于当时 main 分支（3a13338）的代码状态。
> 所有条目均附实际代码位置，最严重的几条已人工复核。

## 目录

- [总体判断](#总体判断)
- [第一部分：代码审查](#第一部分代码审查)
  - [一、会导致进程崩溃的问题](#一会导致进程崩溃的问题)
  - [二、会丢数据或造成安全风险的问题](#二会丢数据或造成安全风险的问题)
  - [三、功能不正确但不崩溃的问题](#三功能不正确但不崩溃的问题)
  - [四、工程化问题](#四工程化问题)
  - [五、确认无问题的部分](#五确认无问题的部分避免误改)
  - [修复优先级](#修复优先级)
- [第二部分：后续模块规划](#第二部分后续模块规划)
  - [现状盘点](#现状盘点三处写了一半的东西)
  - [为什么先做 portmap](#为什么先做-portmap-而不是新功能)
  - [建议的模块顺序](#建议的模块顺序)
  - [前置提醒](#前置提醒)

---

## 总体判断

架构和安全选型是对的：argon2id 密码哈希（低内存机降级 bcrypt）、JWT 限定 HS256、常量时间比较、home/auth/ddns/webhook 四个模块的 `WithLock` + `Snapshot` 快照模式都写得很规范。

问题集中在 **STUN 模块**和**基础设施层**——它们明显落后于其他模块一个重构身位。典型证据是同一个 `os.Stat` nil 检查在 home/auth/webhook 三处已修，stun/ddns 两处漏了；以及 `utils/utilsFile/utils_json.go` 里有 44 行被注释掉的泛型抽象，被注释后在五个模块里各复制了一遍。

---

# 第一部分：代码审查

## 一、会导致进程崩溃的问题

### 1. webhook 发送时的 nil 指针 panic（高）

`modules/webhook/sender.go:35`

```go
client, err := buildClient(cfg.Proxy)   // err 没有检查

for i := 0; i < 3; i++ {
    respBody, err = sendOnce(client, method, rawURL, body, cfg, replacer)
```

`buildClient` 在代理地址解析失败时返回 `(nil, err)`，此处 err 被下一行循环直接覆盖，`client` 为 nil 传进 `sendOnce`，`client.Do(req)` 空指针解引用。

**而这个函数是在 `modules/stun/scheduler.go:599` 的裸 goroutine 里调用的，panic 无法 recover，整个进程崩溃。** 用户只要在服务里填一个格式不对的代理地址就能触发。

修复：`if err != nil { return "", err }`；同时给 `sendServiceWebhook` 的 goroutine 加 `defer recover()` 兜底。

### 2. STUN 的 Runtime 完全没有锁（高）

`modules/stun/enter.go:5-13` 是个裸结构体，没有任何 mutex。对比 `home.HomeRuntime`、`auth.AuthRuntime`、`ddns.DDNSRuntime`、`webhook.WebhookRuntime` 全都带 `sync.RWMutex`——只有 stun 漏了，这是全项目最大的一处不一致。

并发读写方：

| 角色 | 位置 |
|---|---|
| 写 | `api/stun_api/stun_service_add.go:71`、`stun_service_update.go:61-71`、`stun_service_delete.go:52`、`stun_device_add.go:43`、`stun_device_delete.go:41`（HTTP handler goroutine） |
| 写 | `modules/stun/network.go:85-86,102`（5s ticker 后台 goroutine） |
| 写 | `modules/stun/init_stun.go:38,53,62`（errgroup 并发 goroutine） |
| 读 | `modules/home/hydrate.go:86,110-111`（**公开无需登录**的 `GET /api/home/config`） |
| 读 | `modules/ddns/ddns_syncrecord.go:145`、`modules/stun/upnp.go:225,261`、`api/stun_api/get_stun_config.go:22-26` |

`stun_service_add.go:71` 对 `Services` 做 append（可能重新分配底层数组），而 `hydrate.go:115-118` 正持有 `&device.Services[j]` 指针在读。轻则读到撕裂数据，重则遍历中越界 panic。

顺带：设备/服务 ID 用「遍历取 max+1」生成（`stun_device_add.go:27-32`、`stun_service_add.go:48-53`），无锁下并发添加会分到相同 ID。

修复：给 `STUNRuntime` 加 `sync.RWMutex`，仿照 `modules/home/store.go:9-19` 的 `WithLock`/`Snapshot` 模式提供 `Update(fn)` / `Read(fn)`，禁止 API 层直接触碰 `Runtime.Config`；`hydrate.go` 改为返回值拷贝而非内部指针。

### 3. `/api/stun/status` 在启动窗口内会 panic（高）

`api/stun_api/stun_status.go:12,17` 直接 `stun.Runtime.Scheduler.Snapshot()`。

`app.go:48` 先 `go routers.Run(webFS)` 起 HTTP，`waitForBackend()` 一探测到端口通就返回，`app.go:54` 才 `startModulesInBackground()`。而 `Runtime.Scheduler` 在 `init_stun.go:23` 才赋值，且 `ReadConfig` 失败会直接 return 使其永远为 nil。

有意思的是 `modules/home/hydrate.go:96,127` 反而做了 `if stun.Runtime.Scheduler != nil` 判空，API 层没做。

修复：API 入口统一判空返回 503，或让 `Runtime.Scheduler` 在包 init 时就有空实例。

### 4. `os.Stat` 的 nil 检查只修了一半（高）

`modules/stun/stun_config.go:19`、`modules/ddns/ddns_config.go:20`

```go
if fileInfo, err := os.Stat(ConfigPath); os.IsNotExist(err) || fileInfo.Size() == 0 {
```

`os.Stat` 因权限拒绝、路径是目录、Windows 文件被独占锁定（`ERROR_SHARING_VIOLATION`）等非 NotExist 原因失败时，`os.IsNotExist(err)` 返回 false，短路失效，`fileInfo` 为 nil → panic。

`modules/home/home_config.go:15`、`modules/webhook/config.go:19`、`modules/auth/auth_config.go:21` 已经写成 `(fi != nil && fi.Size() == 0)`，这两处漏了。

---

## 二、会丢数据或造成安全风险的问题

### 5. 配置写入非原子（高）

`utils/utilsFile/utils_json.go:73-86` 用 `os.WriteFile`，内部是 `O_WRONLY|O_CREATE|O_TRUNC`——**先把原文件截断为 0，再写新内容**。这是全项目唯一的写入路径（6 个调用点）。也没有 `f.Sync()`。

写入频率并不低：

- `modules/ddns/ddns_store.go:36-55` `commitRecord`：每同步完一条 DDNS 记录就全量重写一次 `ddnsConfig.json`，扫描间隔 15 秒。
- `api/home_api/search_history.go:32-47`：用户每敲一次搜索就把整个 `homeConfig.json`（含全部 apps、categories、search engines）重写一遍。

修复：写临时文件（同目录）→ `f.Sync()` → `os.Rename` 原子替换。Windows 上 `os.Rename` 底层是 `MoveFileEx + MOVEFILE_REPLACE_EXISTING`，跨平台可行。可选再加 `.bak` 备份供恢复。

### 6. 读失败后的连锁反应（高）

五个模块的 `ReadConfig` 只处理了「文件不存在」和「size 为 0」，**JSON 语法错误（也就是上面写坏的半截文件）走 `return cfg, err` 分支**，模块初始化直接失败。

而 `app.go:104-113` 里初始化失败只打一行 `logrus.Errorf` 就 return，进程照常运行：

```go
initModule := func(name string, fn func() error) {
    go func() {
        if err := fn(); err != nil {
            logrus.Errorf("%s 模块初始化失败：%v", name, err)
            return
        }
    }()
}
```

此时 `home.Runtime.Config` 是零值，用户随便点一下搜索触发 `WithLock`，就把零值全量写回 `homeConfig.json`——apps 和分类彻底没了。这是 #5 + #6 组合放大后最严重的后果。

修复：`ReadJsonFile` 失败时按 `.bak` → `createConfig()` 顺序兜底；初始化失败的模块设 `initFailed` 标记，禁止其 `WithLock`/`OnShutdown` 落盘（宁可不存也不能存错），并把失败原因通过 API 暴露给前端。

### 7. 未初始化时 setup 接口敞开（高）

`POST /api/auth/setup` 是公开路由（`routers/auth_routers.go:18-19`），`Setup()` 检查的是内存里的 `cfg.Initialized`。

两条触发路径：

1. HTTP 已服务但 `InitAuth` 还没跑完的窗口内（`app.go:48` vs `app.go:115`，且 initModule 本身是 `go func`）。
2. `authConfig.json` 损坏导致 `InitAuth` 永久失败——setup 端点将**长期敞开**。

任何人都能调用它重置管理员密码，并用零值 Config 整体覆写 `config/authConfig.json`，原密码与原 JwtSecret 被抹掉。

`modules/auth/store.go:57` 有一句注释提到「启动竞态」，说明已察觉到这个窗口，但只补了 JwtSecret 的兜底，没堵住覆写本身。

修复：`auth.InitAuth()` 改为在 `routers.Run` 之前同步执行；给 `AuthRuntime` 加 `ready` 标记，未 ready 时 Setup/Login 一律拒绝；Setup 落盘前再读一次磁盘做二次确认（compare-and-swap 语义）。

### 8. 登录接口零暴力破解防护（高）

`api/auth_api/login.go:16-24` 直接调 `auth.Runtime.Login`。无失败计数、无延迟、无 IP 锁定、无验证码。全仓 grep `ratelimit|限流|attempt|lockout` 在业务代码中零命中。

配合服务硬编码监听 `0.0.0.0:3333`（`routers/enter.go:88`，桌面版也一样）和密码强度零要求（`store.go:45-47` 唯一校验是非空，`"1"` 是合法密码），风险叠加。

### 9. 图标抓取存在 SSRF（高）

`modules/home/get_icon.go:51-85` `FetchIconFromURL` 只做了「补 https 前缀 + url.Parse + host 非空」三项检查：

- 不校验 scheme（仅前缀判断，`u.Scheme` 未白名单化）
- 不解析目标 IP、不拒绝私网/回环/链路本地
- 用默认 `http.Client`，**默认跟随最多 10 次重定向**，入口检查会被 302 到 `127.0.0.1` 绕过
- `downloadIcon` 把响应体原样写入 `data/icon/<ts>.<ext>`，而该目录由 `routers/enter.go:42` 的 `r.Static` 公开直出

这构成一个完整的 SSRF **读取**原语：可拉取 `http://127.0.0.1:xxxx/`、`http://169.254.169.254/...` 并通过 URL 取回。

附带端口扫描 oracle：`api/home_api/icon_fetch.go:20` 把 `err.Error()` 原样返回前端，`connection refused` vs `HTTP 401` 可区分。

修复：白名单 http/https；自定义 `Transport.DialContext` 对每次实际连接的 IP 做 `IsLoopback/IsPrivate/IsLinkLocal/IsUnspecified` 拦截（同时挡住 DNS rebinding）；`CheckRedirect` 限跳数并每跳复检；错误统一为「抓取图标失败」，细节只进日志。可复用 `stun_server.go:189` 已有的 `isNonPublicIP`。

### 10. SVG 上传导致存储型 XSS（中）

`api/home_api/icon_upload.go:15-17` 的 `allowedIconExt` 含 `.svg`，校验只看 `filepath.Ext(file.Filename)`，不读 magic bytes、不查 Content-Type。文件落到 `data/icon/`，由 `r.Static` 以 `image/svg+xml` **同源**直出。访问该 SVG 时内嵌 `<script>` 会执行，可读取存 token 的 localStorage。

`icon_fetch` 走 `get_icon.go:40` 的 `iconExtByMime["image/svg+xml"]` 是同一入口。

修复：去掉 `.svg` 支持，或服务 icon 时加 `Content-Disposition: attachment` + `CSP: default-src 'none'; sandbox` + `X-Content-Type-Options: nosniff`；并用 `http.DetectContentType` 校验真实类型与扩展名一致。

（路径穿越已正确防住：文件名由 `time.Now().UnixNano()` 重新生成，无用户可控成分。）

### 11. UPnP 映射永不删除 + 优雅退出形同虚设（高）

`modules/stun/upnp.go:288` 租期写死 `uint32(0)`（永久）。而 `core/shutdown.go` 只注册了三个「存 JSON」的回调（`stun/init_stun.go:26`、`ddns/ddns_init.go:21`、`webhook/init.go:21`）。

未清理项：

| 项 | 证据 |
|---|---|
| `Scheduler.Close()` | 全仓无调用点，`scheduler.go:223` 是死代码 |
| STUN 隧道 goroutine / conn | `StopAll` 不执行 |
| **UPnP 端口映射** | 永久租期，`stun.go:119-121` 的删除 defer 只在 Run 正常返回时跑 |
| `upnpQueue` worker | `upnp.go:209 stop()` 无调用者 |
| DDNS `Scheduler.Stop()` | 只在 `RebuildScheduler` 里调，退出时不调 |
| HTTP server | `routers/enter.go:90-97` 创建了 srv 但无 `srv.Shutdown(ctx)` |

更要命的是 `core/shutdown.go:47` 只监听 SIGINT/SIGTERM——**Windows 上关闭控制台窗口、任务管理器结束进程、系统关机都不走这两个信号**（`CTRL_CLOSE_EVENT`/`CTRL_SHUTDOWN_EVENT` 不映射为 Go 的 SIGTERM）。而 STUN 的配置恰恰是**唯一一个只在退出时才保存**的（其余四个模块都是变更时立即落盘）。

另外 `logrus.Fatal`（`main_cli.go:12`、`routers/enter.go:96`）内部是 `os.Exit(1)`，绕过所有 defer 和 `RunShutdown`。`routers/enter.go:96` 尤其危险：它在子 goroutine 里，端口被占用时直接 exit，此时其他模块可能正在写配置文件——配合 #5 就是真实的「进程在写盘中途被杀」路径。

结果：每次非正常退出都在路由器上留一条僵尸映射，长期运行耗尽映射表；同时丢掉运行期的配置变更。

修复：`RunShutdown` 加整体超时与 recover；各模块注册真正的清理函数；STUN 改为变更时即落盘，不押在 shutdown 上；`routers.Run` 的 Fatal 改为 channel 回传 error。

### 12. 停用服务后 TCP 连接仍然通（高，同时是安全问题）

`modules/stun/forward.go:13` `ForwardTCP` 不接受 ctx，也没有任何全局登记。`Run` 返回只 close 了 listener，**已建立的转发连接会一直存活到对端主动断开**。用户在面板上点了「停用」，外网仍能通过已有连接访问内网服务。

修复：传 ctx 进来，或用 `sync.WaitGroup` + conn 集合，在 Run 的 defer 里统一 Close。

### 13. 其余安全项（中/低）

- **敏感配置 0644 落盘**（`utils_json.go:81`）：同机任意本地用户可读 `authConfig.json` 里的 jwtSecret（可直接伪造 token）与 passwordHash，以及 DDNS 服务商凭证。应改 0600 / 目录 0700。
- **改密码不使已签发 token 失效**（`modules/auth/store.go:98-120`）：不轮换 JwtSecret，Claims 无 jti/版本号/黑名单，TTL 默认 7 天。密码泄露后改密，旧 token 仍可用 7 天，也没有「退出登录」能力。
- **桌面免登录 secret 走 URL query**（`main_desktop.go:42-47`）：`gin.Default()` 的 Logger 会把 RawQuery 原样输出，再经 `core/logrus.go` 的 hook 落盘到 `logs/`——等于把一枚永久免登录凭证明文写进日志文件。
- **公开路由过度暴露**（`routers/home_routers.go:15-17`）：`home/search-history` 任意访客可读管理员搜索历史（而写入端是受保护的，读写权限不对称）；`home/config` 经 `hydrate.go:82,92` 会拼出内网 IP:端口 与 公网IP:端口，向未认证访客泄露完整内网拓扑。
- **文件上传无请求体总大小限制**（`icon_upload.go:23-32`）：未设 `r.MaxMultipartMemory`，`c.FormFile` 那一行已经把整个 multipart body 解析完了，之后才检查 5MB。传 2GB 会先吃掉磁盘再报「超过 5MB」。应加 `http.MaxBytesReader`。
- **HTTP Server 缺 ReadHeaderTimeout**（`routers/enter.go:90-94`）：只设了 IdleTimeout，对 slowloris 无防护。注意 WriteTimeout 会掐断 SSE，需保持 0 或单独处理。
- **`_ "net/http/pprof"` 空导入**（`routers/enter.go:6`）：当前不构成暴露（DefaultServeMux 未被任何 listener 服务），但只要将来有人写 `http.ListenAndServe(addr, nil)`，`/debug/pprof/*` 就会无鉴权全暴露（heap dump 含 jwtSecret 和 DDNS token）。这个 import 现在唯一的作用就是埋雷。
- **`JwtSecret` 生成的兜底路径**（`auth_config.go:73`）：`crypto/rand` 失败时用时间戳兜底，此路径下 secret 可预测。应直接 panic。

---

## 三、功能不正确但不崩溃的问题

### 14. `runService` 主循环漏了 return（高）

`modules/stun/scheduler.go:441-443`

```go
for {
    if ctx.Err() != nil {
        s.transition(entry, key, PhaseStopped, 0, "")
    }          // ← 缺 return
    err := s.runner.Run(ctx, req, func(state STUNState) {
```

ctx 已取消时只写了状态没有退出，会白跑一整轮完整的 `Run`（含 STUN 拨号、UPnP 提交）。下方 461 行的第二道判断能兜住，但代价是每次都多跑一轮。

### 15. `everAlive` 跨 goroutine 无同步（高）

`modules/stun/scheduler.go:450` 写、`:470` 读。`onState` 回调实际是在 `modules/stun/stun.go:136-155`（TCP）/ `:157-174`（UDP）的独立 goroutine 里被调用的，不是 Run 的同步调用栈。

后果不只是 race detector 报警：读到旧值会把「已成功穿透过」的服务误判为「从未活过」，走探针耗尽路径直接标 `PhaseFailed`。应改 `atomic.Bool`。

### 16. webhook 失败后永不重试（中）

`modules/stun/scheduler.go:610-618` 的 `shouldSendWebhook` 在**实际发送之前**就更新了 `lastWebhook`（`:578` 早于 `:599` 的 goroutine）。一旦发送失败，内置模板默认 `OnlyWhenChanged: true`，下次同地址走 `return phaseChanged && !onlyWhenChanged` → false，**永久不再重试**，直到公网端口变化。DDNS/重定向规则会一直停留在旧地址。

修复：发送成功后再更新 `lastWebhook`。

### 17. `Backoff.Reset()` 从未被调用（中）

`modules/stun/scheduler.go:531-543` 定义了但全仓无调用点。一条长期运行的服务偶发抖动几次后，`RestartingBackoff` 的 index 就永久停在最后一档（1 分钟），即使之后稳定运行数天，下一次抖动仍要等满 1 分钟才重连。`probeFailures` 同理不会归零。

### 18. UDP 转发 session 串台（高）

`modules/stun/forward.go:34` 的 `udpSessions` 是跨所有服务共享的全局 map，**key 只有客户端地址，不含服务标识/localPort**。同一客户端同时访问两个 UDP 服务时会命中同一条 session，包被转发到错误的内网目标。

且 `Run` 返回时 `localConn` 已被关闭，但 session 条目还在 map 里（要等 30s 读超时才 Delete），服务重启后新连接复用旧 entry，回包写进已关闭的 conn，而 `localConn.WriteToUDP` 的错误被完全丢弃。

修复：session map 下沉到每个 Run 实例内（或 key 加上 localPort），Run 退出时遍历关闭。

### 19. DDNS 的 IP 未变短路判断被注释掉了（中）

`modules/ddns/ddns_syncrecord.go:110-114`

```go
// // 2. IP 没变，跳过
// if ip == r.LastIP {
// 	r.LastStatus = model.DDNSRecordStatusSkipped
// 	return
// }
```

现在每个周期（默认 300s）每条记录都要走完整的「查 zone → 查记录 → 比对」。各 provider 内部虽有 `record.Content == ipAddr` 的跳过，但查询请求已经发出去了。10 条记录 = 每 5 分钟 20+ 次 API 调用，容易撞 Cloudflare 限流。`DDNSRecordStatusSkipped` 因此成了永远不会被赋值的死状态。

修复：恢复短路判断，并加一个「强制刷新」周期（如每小时无条件同步一次）防止远端被外部改动后不同步。

### 20. `GetPublicIPInfo` 恒返回 nil error（中）

`modules/stun/network_local.go:19-37` 内部两处失败都用 `fmt.Printf` 吞掉（还不是 logrus，日志不落盘），最后 `return info, nil`。

调用方 `init_stun.go:72-75` 写着 `if err != nil { return fmt.Errorf(...) }`——**这个分支永远不会进**。结果是 `Runtime.Network.LocalIP` 可能为空串，而 `InitSTUN` 报告「初始化完成」。

### 21. 其余正确性问题

- **STUN 服务器只用 TCP 探测却给 UDP 服务用**（`stun_server.go:146`）：`getSTUNServerDelay` 全程走 TCP，但 `stun.go:37` 的 UDP 服务用同一个 stunServer 做 UDP 拨号。默认列表里 `stun.nextcloud.com:443` 明显是 TCP，`stun.freeswitch.org:3478` 通常只有 UDP。应按协议分别维护可用列表。
- **`BestSTUNServer`/`AvailableSTUNServers` 无锁写**（`stun_server.go:137-138`）：`updating sync.Mutex` 只保证不并发执行 Update，不保护读者，而读者遍布 8 处。切片赋值非原子，可能读到撕裂的 slice header。
- **UPnP「永久租期 + 3 小时续约」自相矛盾**（`stun.go:288,305-307`）：既然永久，续约无意义；若路由器把 0 强制转成自己的默认租期（很多 IGD 实现会），3 小时又可能太晚。且 UPnP 续约失败会 `return error` **直接把整条 STUN 隧道拆掉**——UPnP 只是可选辅助，不该杀掉已打通的通道。
- **5 秒轮询公共 STUN 服务器**（`network.go:13`）：一天约 17000 次，对公益服务器不友好且易被限流，而公网 IP 变化频率是小时/天级。
- **`updateNatRouter` 无节流**（`network.go:89`）：每次 IP 变化就 `go` 一次，内部拉起 tracert/traceroute 子进程，IP 抖动时并发拉起多个且同时写同一全局切片。子进程还用的 `exec.Command` 而非 `CommandContext`，Linux 的 `tracepath` 分支没有单跳超时。
- **FAILED 条目永久泄漏**（`scheduler.go:193-204,254-264`）：注释提到的 "failedTTL goroutine" 在代码里不存在，`s.ctx` 除了 Close 时 cancel 外无使用者。PhaseFailed 的 entry 永远留在 map 里，每条附带最多 30 条日志。
- **`waitEntry` 超时后服务静默不启动**（`scheduler.go:244-252,383-389`）：旧 goroutine 卡超过 12s 时，StartService 二次检查发现 key 已存在就直接放弃，**无日志无错误返回**，用户点了「启用」但服务永远不起。
- **SSE 订阅者满了直接丢事件**（`scheduler.go:334-350`）：缓冲 16，客户端稍慢就永久丢失阶段变更，前端状态与后端不一致且无法自愈。
- **`ipPrefix` 前缀匹配误判**（`upnp.go:155-161`）：本机 `192.168.1.x` 时 `localPrefix = "192.168.1"`，会错误匹配上 `192.168.10.1`、`192.168.100.1`。
- **`RebuildScheduler`/`SyncRecordNow` 阻塞 HTTP 请求 30s+**（`ddns/enter.go:17-29`）：`Stop()` → `wg.Wait()` 要等 provider 的 30s client timeout 跑完，而它被 Add/Update/DeleteProvider 在 handler 里同步调用。且 `SyncRecordNow` 不参与 inflight 标记，可与 worker 并发同步同一条记录。
- **`go vet` 报 IPv6 地址拼接错误**（`stun.go:430`）：`fmt.Sprintf("%s:%d", ip, port)` 传给 `net.Dial`，IPv6 会解析错。同文件 `scheduler.go:577` 用的是正确的 `net.JoinHostPort`。todo 里有 IPv6，届时这行会直接失败。
- **格式化字符串写反**（`stun_server.go:164`）：`fmt.Errorf("%s read failed:", err)` 输出成 `<错误内容> read failed:`。
- **生产代码残留 Debug printf**（`modules/ddns/dns/cloudflare.go:89,110`）：第二条会把整个 DNS 记录列表打到 stdout。
- **配置路径全是相对路径**：`config/xxxConfig.json`、`data/icon`、`logs` 全项目无 `os.Executable()`/`filepath.Abs`。桌面版从开始菜单启动时 CWD 由启动器决定，会在另一个目录重新建一套空配置，用户看到数据「凭空消失」。系统服务/开机自启同理。

---

## 四、工程化问题

### 22. `task build` 不构建前端（高）

`Taskfile.yml:13-18` 的 build 只调 `build:desktop`、`build:cli`、`build:linux`，**不依赖 `build:frontend`**。而 `//go:embed web/admin/dist web/home/dist` 直接吃磁盘上已存在的 dist。

改完前端跑 `task build`，打进二进制的是**旧的前端资源，且完全静默无报错**。BUILD.md 里用文字警告了这件事，说明踩过坑但没在工具链层面修复。

修复：给 build 加 `- task: build:frontend` 作为第一步，或用 Task 的 `sources:`/`generates:` 做增量指纹（未改前端时可跳过 npm build）。

### 23. 管理后台首页是假数据（高）

`web/admin/src/pages/Dashboard.tsx:26-36` 整页 476 行渲染的全是 `web/admin/src/mock/dashboard.ts`（172 行）里的静态假数据，包括写死的 `{ name: '本机', ip: '127.0.0.1' }`、设备数、证书、审计日志。这是打进发布二进制的默认首页。

（顺带：mock 里的五个统计卡片 key 是 `device / service / ddns / cert / rule`——UI 已经替证书管理和反代规则占好位了。）

### 24. 没有真正的测试（高）

主模块 136 个 Go 文件只有 4 个 `_test.go`，共 359 行，全部依赖真实网络和外部凭证：

| 文件 | 断言数 | 性质 |
|---|---|---|
| `modules/stun/test/stun_service_test.go` | 2 | 打真实 STUN 服务器 |
| `modules/ddns/ddns_test.go` | **0** | 需 CF_ApiToken，无则 Skip，**零断言恒 PASS** |
| `modules/ddns/dns/cloudflare_test.go` | 3 | 打真实 Cloudflare API |
| `modules/webhook/sender_test.go` | 1 | **真实 PUT 修改线上 SRV 记录** |

CI 里跑 `go test ./...` 等于什么都没测。

建议：用 `httptest.Server` 替代真实端点；有副作用的联调脚本用 `//go:build integration` 隔离；优先给纯逻辑补测试（配置合并、middleware 鉴权、api 请求校验）。

### 25. `test/` 目录是实验脚本不是测试

10 个子目录中 9 个有独立 `go.mod`，是 `package main` 可执行程序，不参与 `go test ./...`。

**`test/nattype` 缺 go.mod 是个隐患**：它没有独立 module，`main.go` 声明 `package main`，会被主模块的 `go build ./...` / `go vet ./...` 扫到。

建议：`test/` 改名 `experiments/` 或 `scratch/` 避免与 Go 测试惯例混淆，给 `test/nattype` 补 go.mod 隔离。

### 26. 前端代码问题

技术栈：两个独立的 Vite + React 19 + TS + Tailwind v4 应用（`web/home` base `/`、`web/admin` base `/linkstar/`），源码共约 6900 行。

- **`web/home/src/App.tsx` 2459 行 / 97KB**（高）：整个导航主页塞在一个文件里，单个 `Home()` 组件声明了 30+ 个 useState、十余个 useEffect。至少该拆出 Clock / SearchBar / AppGrid / SettingsPanel / usePagedDrag / useWallpaper。
- `Stun.tsx` 1337 行、`Ddns.tsx` 1082 行（中）：每个页面内联定义 3 个 Modal。
- **重复代码**（中）：`useToast` 在两个页面各写一遍（超时 2200ms vs 2000ms，很可能是无意的不一致）；Modal 遮罩层 className 复制 7 次且样式已漂移；两套 `api.ts` 的鉴权逻辑（TOKEN_KEY、captureDesktopSecret、ApiResponse）完全重复。
- **全局 15 秒轮询导致整页重渲染**（中）：`setConfig(data)` 每 15s 替换整个对象引用，依赖它的所有 useMemo 全部失效重算。且用户正在拖拽/编辑时轮询照样触发，可能覆盖本地乐观更新。Stun 页已有 SSE，home 也可考虑改用。
- **pointermove 无节流**（中）：`App.tsx:1462-1486` 每个事件都执行 `document.elementFromPoint()` + `closest()`（强制样式重算），120Hz 触控板下每秒 120+ 次布局查询。浮动图标的 transform 更新已经绕过 React 了（写得不错），命中检测同样应该降频。
- **无障碍全缺**（中）：全项目 0 个 aria 属性、0 个 role/tabIndex。48 处 onClick 含大量非语义元素，所有 Modal 无 `role="dialog"`、无焦点陷阱。（正面：8 处 `<img>` 全部有 alt。）
- **移动端断点覆盖不足**（中）：home 页 `sm:/md:/lg:` 仅 1 处，网格固定 `pageCols=8 × pageRows=5` 硬编码，手机竖屏下 8 列图标被压得极小；反而用了 `min-[2000px]:` 这种超宽屏断点，说明只针对大屏调过。
- **`@dnd-kit` 已声明但完全未使用**（低）：`web/home/package.json` 里有 `@dnd-kit/core`、`@dnd-kit/sortable`，src 中零命中（拖拽是手写 pointer 事件）。
- 混用原生 `alert()`/`confirm()`（8 处），项目已有完整 Toast 系统。

### 27. 其余工程化

- **全部 Go 文件是 CRLF**，`gofmt -l` 报告几乎全不合规。任何人跑 `gofmt -w` 或用 format-on-save 都会产生全文件 diff，冲掉 git blame。建议加 `.gitattributes`（`*.go text eol=lf`）+ 一次性 gofmt 独立 commit。
- **日志无轮转无清理**（`core/logrus.go:102-139`）：只按天分目录，无按大小切分、无保留期。`logs/2026-07-02/info.log` 单日已 295KB。且 `InitLogger:54` 生产构建也开 `DebugLevel`，配合 `network.go:73` 每 5 秒一条 Info 日志 = 每天 17280 行。面向 NAS/路由器部署，磁盘是硬约束。
- **日志文件里写入 ANSI 颜色码**（`core/logrus.go:46,48`）：hook 用 `entry.String()` 走同一个 Formatter，转义码被原样写进 info.log。grep/less 读到乱码，每条还白多约 20 字节。
- **git 里躺着约 36MB 二进制垃圾**：`test/easyTier_stun/stun_test.exe` 10.5MB、`test/stun/stun.exe` 10.4MB、`test/stun/stun` 10.1MB、`test/udp_stun/udp_stun.exe` 5.0MB、`docs/img/home.png` 5.4MB、`lICON.PNG` 923KB、`.DS_Store` ×4。根 `.gitignore` **没有 `.DS_Store` 规则**。（正面：`linkstar`/`linkstar.exe` 31MB 已被正确忽略。）
- **死文件**：`web/dist/`（含内容为 `222` 的 `a.html`）和 `web/index.html` 是被 React 重写前的遗留，无任何 Go/前端引用；`config/Config.json` 是没人读的孤儿（内容与 stunConfig.json 同构，时间戳停在 2026-05-01），用户可能误以为是主配置去编辑。
- **端口 3333 硬编码 5 处**，`flags` 包是空的（显然原本就留给命令行参数）。
- **就绪探测靠抓 HTML 内容**（`app.go:77-100`）：GET `/linkstar/` 后 `strings.Contains(body, "LinkStar")`。前端改个 title 或换打包器就失效，表现为 `waitForBackend` 超时后 CLI 版直接 Fatal。应加 `/api/health` 端点。
- **四个空包**：`core/enter.go`、`modules/srv/enter.go`、`global/enter.go`、`flags/enter.go` 都只有一行 package 声明。
- **产物命名不一致**：Taskfile 产出 `linkstar-cli.exe`/`linkstar`/`linkstar-desktop.exe`，BUILD.md 写的是 `linkstar.exe`/`linkstar-linux`，三处对不上。
- **Taskfile 强依赖 PowerShell**（`Taskfile.yml:34,45`）：`build:linux` 这个交叉编译任务本身用 `powershell` 创建目录，在 Linux/macOS 主机上直接失败。而 `build/windows/Taskfile.yml:12-15` 里对 `Remove-Item` 就正确用了 `platforms:`，说明知道这个机制只是主 Taskfile 没用。

### 28. 值得抽公共层的重复代码

| 模式 | 重复次数 | 建议 |
|---|---|---|
| 「业务错误→文案 / 其他错误→保存失败」三段式 | 18 处 | `res.HandleErr(c, err, map[error]string{...})` |
| 按 ID 线性查找循环 | 20+ 处 | `cfg.FindCategory(id)` / `cfg.FindDeviceService(devID, svcID)`，stun 侧六处各能砍 20 行 |
| 「找最大值 +1」生成 ID/Order | 6 处 | 泛型 `nextOrder[T](items, get)` |
| 手写冒泡/插入排序 | 3 处 | `slices.SortStableFunc`（Go 1.25 标准库）。注意 `get_icon.go:128` 那个冒泡是**不稳定**的，会打乱同 priority 候选的 HTML 出现顺序 |
| Add/Update 的 Request 结构体除 ID 外字段完全一致 | 3 对 | 共享 payload 结构体嵌入 |
| 「只有一个 ID 字段」的 Request | 8 个 | `IDRequest` / `UintIDRequest` 两个共享类型 |
| 五个模块各写一遍 ReadConfig/createConfig/SaveConfig | 5 处 | 见下方「一次消掉四条」 |

**一次消掉四条问题的机会**：`utils/utilsFile/utils_json.go:8-51` 有 44 行**被注释掉的泛型 `ReadConfig`/`CreateConfig`/`UpdateConfig`**——抽象曾经存在过，被注释后在五个模块里各复制了一遍，直接导致了 #4（nil 检查只修 3/5）和「`stun_config.go:79` 用局部常量遮蔽同名包级常量」这类修一处漏一处的问题。把它实现出来（含 #5 的原子写、#6 的备份恢复、#4 的 nil 检查），五个模块一起受益。

另外 `api/` 下 `grep 'binding:"'` **零命中**——`ShouldBind` 实际只做 JSON 反序列化不做校验，所有校验都是手写 if，且缺口不少（Protocol 不校验、InternalPort 允许 0、设备 IP 不校验合法性会被拼进对外 URL）。加 binding tag 能一次性删掉几十行手写 if。

---

## 五、确认无问题的部分（避免误改）

- `icon_upload` 的**路径穿越已正确防住**：文件名由 `time.Now().UnixNano()` 重新生成，扩展名走白名单，`filepath.Join` 无用户可控成分。
- `MatchDesktopSecret` 用了 `subtle.ConstantTimeCompare` 且对空 secret 显式返回 false，CLI 构建下该通道确实永不放行。
- `jwt.ParseToken` 正确使用 `jwt.WithValidMethods([]string{"HS256"})`，**不存在 alg=none / RS256 混淆漏洞**。
- 密码哈希用 argon2id（低内存机降级 bcrypt），参数取 OWASP 推荐值，验证走常量时间比较——算法选型正确。
- `JwtSecret` 是 `crypto/rand` 32 字节随机生成，非硬编码。
- home/auth/ddns/webhook 的 `WithLock` 模式（深拷贝切片、mutator 出错不落盘、快照后解锁再写文件）设计干净且一致。
- `hooks.go` 的先拷贝再脱锁回调，避免了持锁回调导致的死锁。
- `rotateFiles:130-135` 在 err.log 打开失败时回头关掉已打开的 info.log，避免 fd 泄漏——细节处理得好。
- `exec_windows.go` / `exec_other.go` 的 `//go:build` 双向覆盖是标准写法，签名一致无遗漏平台。
- `main_cli.go` 与 `main_desktop.go` 重复度很低，公共逻辑已正确抽到 `app.go` 的 `initRuntime()`/`startBackend()`，构建标签互斥完备。这个结构合理，不建议改动。
- embed 资源体积合理：admin dist 377KB + home dist 340KB ≈ 717KB，占 31MB 二进制不到 2.5%。

---

## 修复优先级

**如果只做三件事：**

1. **修 webhook 的 nil panic**（#1，一行代码）—— 最容易被用户触发的崩溃
2. **给 STUN Runtime 加锁**（#2，照搬 home 的 WithLock/Snapshot，约半天）—— 唯一确凿的 data race
3. **`os.WriteFile` 换成临时文件 + rename**（#5）—— 所有数据丢失场景的共同地基

**完整排序：**

| 优先级 | 条目 | 理由 |
|---|---|---|
| P0 | #1 #2 #3 #4 | 会崩溃，且 #1 #3 #4 都是几行的修改 |
| P0 | #5 + #28 泛型 helper | 一处改动五模块受益，同时消掉 #4 和常量遮蔽 |
| P0 | #6 #7 | 「读失败→零值→全量覆写」是最容易造成不可逆数据丢失的组合 |
| P1 | #11 #12 | UPnP 僵尸映射 + 停用后仍可访问，Windows 下复现门槛极低 |
| P1 | #8 #9 #10 | 安全，尤其考虑到本工具的用途就是把服务暴露到公网 |
| P1 | #22 #23 | 会静默产出错误二进制 / 发布版给用户看假信息 |
| P2 | #14~#21 | 功能不正确但不崩溃 |
| P2 | 配置路径基准（相对路径问题） | 桌面版从快捷方式启动即「丢数据」，用户感知最强烈 |
| P2 | 日志轮转与去色 | 面向 NAS 部署，磁盘是硬约束 |
| P3 | #24 测试策略、#26 前端拆分 | 工程质量 |
| P3 | CRLF/gofmt、删 pprof import、删死文件、清理 git 二进制 | 一次性清理 |

---

# 第二部分：后续模块规划

## 现状盘点：三处「写了一半」的东西

**`test/nattype/main.go`（40KB）是个完整的 NAT 检测程序**，文件头注释写得比很多开源项目的设计文档都清楚——STUN Mapping 行为检测、RFC5780 Filtering 在公网 STUN 基础设施上的现实局限、traceroute 判 CGNAT 在第几跳、TCP 单独测 Mapping 并解释了为什么 TCP 不测 Filtering（CHANGE-REQUEST 要求服务器换源地址回包，该机制对 TCP 协议层面就不成立）。

这已经不是实验脚本了，是一个成品模块的核心逻辑躺在 test 目录里。**todo 第一条「nat 家庭探测」实际完成度很高，缺的只是搬进 `modules/` 并接上 API 和前端。**

**`modules/srv/enter.go` 是只有一行 `package srv` 的空包**，而 `test/srv-panel/main.go` 里已经有 SRV 记录查询的原型。开了头没落地。

**`web/admin/src/mock/dashboard.ts` 的五个统计卡片是 `device / service / ddns / cert / rule`**——UI 已经替证书管理和反向代理规则占好位了。

## 为什么先做 portmap 而不是新功能

现在 UPnP 的调用**焊死在 `modules/stun/stun.go:288` 的隧道主循环里**，租期写死 0，删除走 `stun.go:120` 的裸 goroutine 且绕过了队列（破坏了「UPnP 操作串行化」的设计前提——很多路由器的 IGD 实现并发调用会返回 500）。

todo 上的三条——**NAT-PMP/PCP、UDP 也要做 UPnP 映射、IPv6**——本质上是同一件事：**当前只有一种打洞方式，而且它和 STUN 隧道逻辑长在一起了。**

所以建议抽一个 `modules/portmap`：

```go
type Mapper interface {
    Name() string
    Available(ctx context.Context) bool
    Add(ctx context.Context, proto string, internal, external uint16, lease time.Duration) (uint16, error)
    Delete(ctx context.Context, proto string, external uint16) error
    Renew(ctx context.Context, proto string, external uint16, lease time.Duration) error
}
```

把现在的 UPnP 实现搬进去当第一个 Mapper，NAT-PMP/PCP 是第二个，IPv6 场景下的 PCP pinhole（直接放行防火墙）是第三个。上层按 `[]Mapper` 依次尝试。

收益是复合的：

- todo 里三条一次性有了落点
- 租期和续约变成接口的一部分，能顺手修掉「永久租期 + 3 小时续约」的自相矛盾
- 映射的生命周期集中管理，退出时统一清理，僵尸映射问题（审查 #11）一起解决
- UPnP 失败不再拆掉整条 STUN 隧道，降级为 warn + 继续保活

反过来，如果先做反向代理或证书管理，是在一个「穿透本身还不够可靠」的底座上加第二层楼。而且**反代和证书管理，Nginx Proxy Manager / Caddy / Traefik 已经做得很好了，这不是 LinkStar 的护城河**。能不能在各种家宽 NAT 环境下把洞打通、打不通时诚实地告诉用户为什么、并自动选择次优路径——这才是。

## 建议的模块顺序

### 1. `modules/portmap` + `modules/nattype`（最高优先）

这两个是一对：**nattype 告诉你「这个网络环境能做什么」，portmap 提供「能做的手段」。**

nattype 的检测结果（NAT1-4 标签、TCP 直连可行性、CGNAT 在第几跳）应该直接驱动 UI 上的诊断页。用户配了服务连不上的时候，能看到「你在 CGNAT 后面，UPnP 对你无效，需要中继」——这个体验价值极高，而且**代码你基本写完了**。

工作量：nattype 主要是搬运 + 接 API/前端；portmap 是抽象 + 搬运现有 UPnP + 新写 NAT-PMP/PCP。

### 2. `modules/relay`（中继兜底）

todo 里唯一一个真正的新东西，也是对称 NAT / CGNAT 用户的唯一出路。但它依赖 nattype 判断「什么时候该降级到中继」，所以排在后面。

需要决策的点：自建中继服务端还是接现成协议（复用 frp？或者走 QUIC——`test/quic_web/` 里已经试过 QUIC 了）；要不要做流量计费/限速。

工作量：大概是前面两个加起来的两倍。

### 3. 填上 `modules/srv`

SRV 记录本质是 DDNS 的延伸——`modules/ddns/dns/` 已经有 8 家服务商的适配（alidns、baidu、cloudflare、huawei、namecheap、namesilo、tencent_cloud），能让「域名 + 非标准端口」的服务被自动发现。

工作量小，和现有 DDNS 模块共用 provider 层，性价比高。而且 `modules/webhook/sender_test.go` 里那个真实修改 SRV 记录的测试说明已经在这条路上走过一段了。

### 4. 通知中心

现在 webhook 只是 STUN 状态变更的附属品。把它提升成**独立的通知总线**，事件源包括：STUN 阶段变更、DDNS 同步成功/失败、证书到期、服务掉线。加上 Bark / Telegram / 企业微信这类常用渠道的内置模板。

能复用现有的 webhook 发送层，增量不大但用户感知强。（做之前先修审查 #16 的「失败后永不重试」和 #1 的 nil panic。）

### 5. 证书管理、反向代理

放到最后。理由见上——这块有成熟替代品，不是差异化优势所在。

### 建议从路线图删掉：用户与权限

这是个单机 NAS 工具，多用户权限带来的复杂度（角色模型、资源隔离、审计粒度）和收益完全不成比例。现在的单密码 + JWT 就够了。真要加，也应该是「只读访客模式」这种轻量方案，而不是完整的 RBAC。

## 前置提醒

开新模块之前，建议先把审查里 STUN 模块那几条修掉，**特别是 #2 Runtime 加锁和 #11 UPnP 生命周期**。

原因不是洁癖：`portmap` 抽出来之后，新模块必然要和 `stun.Runtime` 交互；如果现在这个无锁的裸结构体不改，**新模块只会复制同一套坏模式，而且届时并发面更大、更难查**。

home / auth / ddns / webhook 的 `WithLock` + `Snapshot` 写法是现成的模板，照搬到 stun 上大概半天工作量。
