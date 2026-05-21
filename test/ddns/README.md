# DDNS 学习 Demo — webhook 工具 + cf 业务

这是 `modules/webhook` 和 CF DDNS 业务的**前期学习版**：分两个独立子包，跑通最小可用，搞清楚分层后再决定主项目里怎么整合。

---

## 当前 scope

✅ **做了**：
- `webhook.Do(method, url, headers, body)` — 极简 HTTP 工具函数
- `cf.UpdateRecord(token, zoneID, name, ip)` — CF DDNS 业务（typed struct，不用 JSONPath）
- 配置走 JSON 文件（`config/ddnsConfig.json`），首次启动自动建模板，参考 `modules/stun/stun_config.go` 模式
- `main.go` — 演示两者协同

⏳ **下一步再做**（本 demo 不涉及）：
- webhook 模块的调度器（订阅 stun 事件、定时器）
- 用户配置驱动的通知 webhook（`webhook.Send(cfg, vars)` + 模板渲染）
- 多 webhook 列表 + CRUD

---

## 文件结构

```
test/ddns/
├── go.mod
├── main.go                  # 演示入口
├── config.go                # Config struct + 读写 + 校验
├── config/
│   └── ddnsConfig.json      # 首次启动自动生成的配置（已 gitignore）
│
├── webhook/                 # HTTP 工具函数
│   └── webhook.go           # webhook.Do(method, url, headers, body) → (body, status, err)
│
└── cf/                      # CF DDNS 业务模块
    ├── types.go             # CloudFlare API 的 typed struct
    └── cf.go                # UpdateRecord：查 → 比较 → 改/建
                             # FindZoneID：自动从域名反查 zone_id
```

---

## 三层分工

```
┌─────────────────────────────────────┐
│  main.go                            │  ← 演示协调
└──────────┬────────────────────┬─────┘
           │                    │
           ↓                    ↓
   ┌──────────────┐    ┌──────────────┐
   │  cf 模块      │    │  其它业务...   │
   │  typed 业务  │    │  (未来扩展)    │
   └──────┬───────┘    └──────┬───────┘
          │                   │
          └────────┬──────────┘
                   ↓
          ┌──────────────────┐
          │  webhook.Do      │  ← 所有人共用的 HTTP 工具
          │  共享 http.Client │     超时/代理/连接池统一
          └──────────────────┘
```

---

## 怎么跑

### 第一次跑：自动生成配置模板

```bash
cd test/ddns
go run . -ip 1.2.3.4
```

会自动建出 `config/ddnsConfig.json`，内容长这样：
```json
{
  "createdAt": "...",
  "updatedAt": "...",
  "cf": {
    "token": "把你的 CF API Token 填这里",
    "zoneID": "",
    "recordName": "home.example.com"
  }
}
```

然后会因为 token 是占位符而拒绝运行。

### 第二步：填好配置后再跑

打开 `config/ddnsConfig.json` 修改：
- `cf.token` 填真实 Token（去 [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens) 建一个，权限 `Zone → DNS → Edit` + `Zone → Zone → Read`）
- `cf.recordName` 填你的完整域名（如 `home.example.com`）
- `cf.zoneID` 留空就好（会自动反查）
- 想要通知就再加个 `"notify": "https://your-webhook"`

```bash
go run . -ip 你的真实公网IP
```

### CLI 参数

```
-ip      要写入的 IPv4（必填，模拟 STUN 输出）
-config  配置文件路径（默认 config/ddnsConfig.json）
```

CF token、zone、域名、notify URL **全部走 JSON 配置**，不再通过 CLI 传。

---

## 三种典型输出

**首次跑（CF 上没这条记录）**：
```
☁️  步骤 1: 调 cf.UpdateRecord 更新 A 记录
🆕 新建成功：1.2.3.4
   record_id: abc123...
```

**改 IP（CF 上已有记录但 IP 不同）**：
```
✅ 更新成功：1.2.3.4 → 5.6.7.8
   record_id: abc123...
```

**重复跑同样 IP**：
```
⏭️  跳过（IP 未变化）：5.6.7.8
```

---

## 关键设计点

### 1. webhook.Do 智能处理 body

```go
webhook.Do("GET",  url, headers, nil)              // 不带 body
webhook.Do("POST", url, headers, []byte("raw"))    // 原始字节
webhook.Do("POST", url, headers, "raw string")     // 原始字符串
webhook.Do("POST", url, headers, struct{...}{})    // 自动 json.Marshal + Content-Type
webhook.Do("POST", url, headers, map[string]any{   // map 也行
    "ip": "1.2.3.4",
})
```

### 2. cf 业务用 typed struct，**不用 JSONPath**

CF 响应结构编译期就知道，所以直接定义 struct + `json.Unmarshal`，编译期类型安全、IDE 自动补全。看 [cf/types.go](cf/types.go)。

### 3. cf 用 webhook.Do 当 HTTP 工具

```go
body, status, err := webhook.Do("GET", url, authHeaders(token), nil)
// ... typed struct 解析
var resp listResp
json.Unmarshal(body, &resp)
```

省去每次写 `http.NewRequest + Header.Set + Do + ReadAll` 的 boilerplate。
**但响应解析归 cf 模块自己管**（typed struct），webhook 模块不掺和。

---

## 集成进 modules/ 时的下一步

按当前分层直接对应到主项目：

```
modules/
├── webhook/
│   ├── do.go          # 从 test/ddns/webhook/webhook.go 抄过去
│   ├── scheduler.go   # 新加：订阅 stun.OnIPChange + 定时器
│   ├── send.go        # 新加：Send(cfg, vars) 配置驱动的通知
│   ├── config.go      # 新加：用户 webhook CRUD
│   └── template.go    # 新加：模板渲染
│
└── cf/
    ├── types.go       # 从 test/ddns/cf/types.go 抄过去
    └── cf.go          # 从 test/ddns/cf/cf.go 抄过去
                       # main.go 里：stun.RegisterOnIPChange → cf.UpdateRecord
```

webhook 模块复杂度逐步加，cf 模块基本上现在就是终态。
