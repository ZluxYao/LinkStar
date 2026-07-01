# LinkStar

轻量级家庭服务器 / NAS / 软路由网络入口管理工具，把导航主页、内网穿透（STUN/UPnP）、DDNS 动态解析和 Webhook 通知整合进一个 Go 单二进制里。

> 项目仍在快速迭代中，适合个人实验、自用部署和二次开发。

## 界面预览

### Home 导航页

![LinkStar Home](docs/img/home.png)

### STUN 管理页

![LinkStar STUN](docs/img/stun.png)

## 目录

- [界面预览](#界面预览)
- [功能特性](#功能特性)
- [使用指南](#使用指南)
  - [快速开始](#快速开始)
  - [界面入口](#界面入口)
  - [数据目录](#数据目录)
  - [DDNS 配置说明](#ddns-配置说明)
  - [Webhook 变量](#webhook-变量)
- [开发者指南](#开发者指南)
  - [技术栈](#技术栈)
  - [从源码构建](#从源码构建)
  - [本地开发](#本地开发)
  - [项目结构](#项目结构)
- [路线图](#路线图)
- [注意事项](#注意事项)
- [License](#license)

## 功能特性

- **Home 导航页**：应用快捷入口、分类、拖拽排序、搜索引擎管理、Bing 壁纸、图标上传/抓取。
- **STUN / UPnP 服务映射**：探测本机、公网 IP 与 NAT 路由链路，为内网服务维护外部访问地址。
- **服务管理**：按设备维护 TCP/UDP 服务，可设置内部端口、UPnP 映射端口、HTTPS 标记和是否展示到首页。
- **DDNS 动态解析**：支持 A / AAAA 记录，定时同步公网 IP 到 DNS 服务商。
- **DNS 服务商适配**：Cloudflare、阿里云 DNS、腾讯云 DNSPod、百度云、华为云、NameCheap、NameSilo。
- **Webhook 通知**：服务地址变化时推送 HTTP 请求，内置通用 JSON、Cloudflare SRV、Cloudflare 重定向规则模板。
- **单二进制部署**：Go `embed` 打包前端静态资源，运行后同时提供首页和管理后台。

---

## 使用指南

面向只想部署和使用 LinkStar 的用户。

### 快速开始

已经拿到编译好的二进制文件：

```bash
./linkstar
```

Windows：

```powershell
.\linkstar.exe
```

启动后打开 `http://localhost:3333/`。首次运行会自动创建 `config/`、`data/`、`logs/` 等运行目录。

### 界面入口

默认监听地址：

| 入口 | 地址 |
| --- | --- |
| 首页导航 | `http://localhost:3333/` |
| 管理后台 | `http://localhost:3333/linkstar/` |
| pprof 调试 | `http://localhost:3334/debug/pprof/` |

### 数据目录

LinkStar 默认使用本地 JSON 文件持久化配置：

| 路径 | 说明 |
| --- | --- |
| `config/homeConfig.json` | 首页导航、搜索、分类、布局、壁纸配置 |
| `config/stunConfig.json` | STUN 服务器列表、设备与服务配置 |
| `config/ddnsConfig.json` | DDNS 服务商、解析记录与同步间隔 |
| `config/webhookConfig.json` | Webhook 模板配置 |
| `data/icon/` | 用户上传或抓取的网站图标 |
| `logs/YYYY-MM-DD/` | 运行日志与错误日志 |

> 这些目录通常包含本机状态或敏感凭据，默认不会提交到 Git。

### DDNS 配置说明

DDNS 记录支持以下 IP 来源：

- `stun`：使用 STUN 模块探测到的公网 IP。
- `web`：从公网 IP 查询接口获取，未填写 URL 时使用内置 IPv4/IPv6 查询源。
- `dns` / `interface`：类型已预留，当前实现仍在完善中。

Cloudflare 支持 `proxied` 开关；NameCheap 当前仅适合 IPv4 A 记录场景。

### Webhook 变量

Webhook 请求体和 URL 中可使用服务运行时变量，例如：

```json
{
  "service": "#{service_name}",
  "device": "#{device_name}",
  "address": "#{address}",
  "ip": "#{external_ip}",
  "port": #{port},
  "protocol": "#{protocol}",
  "phase": "#{phase}",
  "time": "#{updated_at}"
}
```

适合在端口变化、服务重启或地址更新后同步到外部系统。

---

## 开发者指南

面向想要从源码构建、参与开发或二次开发的用户。

### 技术栈

- 后端：Go、Gin、logrus、pion/stun、goupnp
- 前端：React、TypeScript、Vite、Tailwind CSS、lucide-react
- 存储：本地 JSON 配置文件

### 从源码构建

环境要求：

- Go 1.25+
- Node.js 20+（用于构建前端）
- npm

构建前端：

```bash
cd web/home
npm install
npm run build

cd ../admin
npm install
npm run build
```

回到项目根目录构建后端：

```bash
cd ../..
go build -o linkstar .
```

运行：

```bash
./linkstar
```

> 后端会嵌入 `web/home/dist` 和 `web/admin/dist`，如果修改了前端代码，请先重新执行对应前端的 `npm run build`。

### 本地开发

后端：

```bash
go run .
```

Home 前端：

```bash
cd web/home
npm install
npm run dev
```

Admin 前端：

```bash
cd web/admin
npm install
npm run dev
```

前端接口默认请求同源 `/api/...`。联调时可按需要在 Vite 开发服务器中配置代理，或直接使用后端嵌入后的静态页面进行测试。

### 项目结构

```text
.
├── api/              # HTTP API 处理层
├── core/             # 日志、退出保存等基础能力
├── modules/          # home / stun / ddns / webhook 核心模块
├── routers/          # Gin 路由注册
├── utils/            # 通用工具
├── web/home/         # 首页导航前端
├── web/admin/        # 管理后台前端
└── main.go           # 程序入口，嵌入前端资源并启动服务
```

---

## 路线图

- 反向代理管理
- 证书管理
- 用户与权限
- 审计日志与通知中心
- Docker / systemd 部署示例

## 注意事项

- UPnP 映射依赖网关支持并开启 UPnP。
- 请妥善保护 `config/` 中的 DNS 服务商凭据。
- 将服务暴露到公网前，请确认服务本身的认证、访问控制和防火墙策略。

## License

本项目采用 GPL-3.0-or-later 开源许可证，详见 [LICENSE](LICENSE)。
