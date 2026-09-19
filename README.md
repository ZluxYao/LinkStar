<div align="center">

<img src="docs/img/logo.png" alt="LinkStar Logo" width="140">

# LinkStar

**没有公网 IP，也能把家里的服务放到外面访问。一个 Go 单二进制，下载就能跑。**

面向家庭服务器 / NAS / 软路由的网络入口工具——**STUN + UPnP** 打洞、**DDNS** 跟着家宽 IP 走、**证书**和**反向代理**都在里面，再配一个导航主页。

[![Release](https://img.shields.io/github/v/release/ZluxYao/LinkStar?label=Release&color=success)](https://github.com/ZluxYao/LinkStar/releases/latest)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](go.mod)
[![STUN](https://img.shields.io/badge/NAT-STUN%20%2B%20UPnP-orange)](#内网穿透)
[![License](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey)](#从源码构建)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](#开发者指南)

**简体中文** · [English](README.en-US.md)

![LinkStar Home](docs/img/home.jpg)

<sub>导航主页</sub>

![LinkStar 穿透管理](docs/img/stun.png)

<sub>管理后台 · 穿透管理</sub>

</div>

---

家里的 NAS、软路由、Jellyfin，想在外面打开，通常卡在三件事上：运营商不给公网 IP、家宽 IP 天天变、浏览器一进去就是「不安全」。LinkStar 把这三件事和它们的配套——打洞、DDNS、证书、反向代理——收进同一个程序。前端随 Go `embed` 打包进去了，下载一个二进制运行即可，同时提供**导航主页**和**管理后台**（密码保护）两套界面。

## 目录

- [为什么用 LinkStar](#为什么用-linkstar)
- [功能特性](#功能特性)
- [内网穿透](#内网穿透)
- [快速开始](#快速开始)
- [使用指南](#使用指南)
  - [界面入口](#界面入口)
  - [密码与登录](#密码与登录)
  - [数据目录](#数据目录)
  - [证书](#证书)
  - [反向代理](#反向代理)
  - [DDNS 配置说明](#ddns-配置说明)
  - [入口重定向（Cloudflare）](#入口重定向cloudflare)
  - [Webhook 变量](#webhook-变量)
- [开发者指南](#开发者指南)
- [路线图](#路线图)
- [注意事项](#注意事项)
- [社区交流](#社区交流)
- [License](#license)

## 为什么用 LinkStar

- **一个二进制搞定全部**：不用 Docker、不用装一堆服务，前端已随程序打包，`./linkstar` 直接跑。
- **没有公网 IP 也能穿透**：STUN 探测公网出口 + UPnP 自动映射，家宽大内网、运营商 NAT 后面的服务也能对外访问。
- **地址一变就自动跟上**：公网 IP 或外网端口变了，自动更新 DNS 记录、改 Cloudflare 重定向规则、推 Webhook，不用人盯着。
- **证书和反向代理都在里面**：不用再单独装一套 nginx + certbot。证书能自动签发自动续期，反向代理干的就是 nginx 那些活。
- **不知道内网 IP 也能开始**：局域网扫一遍，在线的机器和它们开着的端口摆出来，点一下就建好服务。
- **两种形态**：既能作为后台服务常驻（CLI 版），也有系统托盘的桌面版（基于 Wails）。

## 功能特性

| 模块 | 能力 |
| --- | --- |
| 🏠 导航主页 | 应用快捷入口、分类与拖拽排序、搜索引擎管理、Bing 每日壁纸 / 自定义壁纸、图标上传与自动抓取 |
| 🌐 内网穿透 | STUN 探测本机 / 公网 IP 与 NAT 链路，UPnP 自动创建端口映射，心跳保活，外网地址变化实时可见 |
| 🧭 NAT 类型检测 | RFC 5780 探测，UDP / TCP 分别判定：公网直连、NAT1~NAT4，打不通时先看这里 |
| 🔌 服务管理 | 按设备维护 TCP / UDP 服务，复制服务、卡片上直接启停，`/go/{服务名}` 按名字进，端口漂了也能用 |
| 📡 局域网扫描 | 选一段网扫一遍，列出在线机器和开着的端口，常见端口带名字（DSM、PVE、Alist、Jellyfin……），点端口直接建服务 |
| 🔐 证书管理 | 上传 PEM、读本地路径、ACME DNS-01、ACME HTTP-01，另有自签；按 SNI 匹配，支持通配符，到期前自动续 |
| 🔁 反向代理 | 干 nginx 的活：按域名分流、HTTP / HTTPS 双入口、站点可独占端口、WebSocket / SSE 透传、访问日志 |
| 🌍 DDNS 解析 | A / AAAA 记录，五种 IP 来源，定时把地址同步到 DNS 服务商 |
| ↪️ 入口重定向 | 一个固定域名始终指向服务当前的外网地址，三个 Cloudflare ID 都不用填 |
| 🔔 Webhook | 地址变化时推 HTTP 请求，内置通用 JSON、Cloudflare SRV 等模板 |
| 📋 运行日志 | 后台直接看日志，按级别和关键字过滤 |
| ⚡ 实时状态 | 后端定时心跳检测，界面通过 SSE 实时推送服务状态变化 |
| 🔒 密码保护 | 首次使用引导设置管理密码（至少 8 位），管理接口需登录（JWT），桌面版本地窗口免登录 |
| 📱 手机端 | 后台和主页都适配了小屏 |

**已适配的 DNS 服务商**：Cloudflare、阿里云 DNS、腾讯云 DNSPod、百度云、华为云、NameCheap、NameSilo。

## 内网穿透

LinkStar 的 NAT 穿透（内网穿透 / NAT traversal）基于标准 **STUN** 协议（[pion/stun](https://github.com/pion/stun) 实现）：

1. **STUN 探测**：向公共 STUN 服务器发送 Binding 请求，拿到本机在 NAT 后的公网出口 IP 与端口，判断 NAT 类型。
2. **端口复用打洞**：在同一本地端口上复用监听（TCP/UDP），保持 STUN 会话打通的 NAT 映射。
3. **UPnP 自动映射**：网关支持 UPnP 时自动创建端口映射，TCP 场景下把公网端口指向内网服务。
4. **端口转发**：把外网入站连接转发到目标设备的内部端口，实现无公网 IP 的服务暴露。
5. **心跳保活**：定时健康检查与重连，公网端口变化时自动感知并触发 DDNS / 入口重定向 / Webhook 同步。

洞口上还可以直接挂证书：外面访问就是 `https://`，浏览器地址栏不再有感叹号，不需要额外再架一层。

> 适合家宽（家庭宽带）大内网、运营商 NAT、软路由等没有独立公网 IP 的场景，作为 frp / ngrok 之外的轻量自建选择。

## 快速开始

从 [Releases](https://github.com/ZluxYao/LinkStar/releases/latest) 下载对应平台的文件：

| 文件 | 平台 |
| --- | --- |
| `linkstar` | Linux x86_64 |
| `linkstar-linux-arm64` | Linux ARM64（树莓派 4/5、ARM 软路由、NAS） |
| `linkstar-linux-armv7` | Linux ARMv7（老树莓派、32 位 ARM 路由器） |
| `linkstar-linux-mipsle` | Linux MIPS 小端（OpenWrt 路由器） |
| `linkstar-cli.exe` | Windows x86_64，命令行版 |
| `linkstar-desktop.exe` | Windows x86_64，桌面版，带窗口和托盘 |
| `LinkStar-x.y.z-x86.fpk` | 飞牛 NAS（fnOS）安装包 |

macOS 目前需要在 Mac 上自行编译，见 [BUILD.md](BUILD.md)。

```bash
# Linux
chmod +x linkstar
./linkstar
```

```powershell
# Windows
.\linkstar-cli.exe
```

启动后打开 `http://localhost:3333/`。首次运行会自动创建 `config/`、`data/`、`logs/` 目录；第一次进管理后台会引导设置管理密码，之后就能用了，没有别的前置配置。

想装成开机自启的系统服务、或者从旧版升上来，见 [部署与升级](docs/部署与升级.md)。第一次用想知道从哪下手，见 [快速上手：把一个内网服务发布到外网](docs/快速上手.md)。

## 使用指南

### 界面入口

| 入口 | 地址 | 说明 |
| --- | --- | --- |
| 导航主页 | `http://localhost:3333/` | 公开访问，无需登录 |
| 管理后台 | `http://localhost:3333/linkstar/` | 需要密码登录 |
| 服务索引 | `http://localhost:3333/go/{服务名}` | 按名字跳到该服务当前的外网地址 |

> 服务监听 `0.0.0.0:3333`（端口暂不支持修改），局域网内其他设备可通过本机 IP 访问。

**服务索引**值得单独说一下。打洞拿到的外网端口会漂，而 DNS 的 A 记录里带不了端口——所以原本每个服务都得在 Cloudflare 上挂一条重定向规则（免费版上限 10 条），端口一变还要靠 Webhook 回写。

换个做法：**只把 LinkStar 自己这一个洞暴露出去，其余服务全部由它查表跳过去**，Cloudflare 规则就从 N 条降到 1 条。

```text
https://linkstar.example.com/fw        （Cloudflare 边缘，唯一一条重定向规则）
  → https://ls.example.com:21313/fw    （LinkStar 自己的洞）
  → 307 https://fw.example.com:34521/  （fw 的洞，LinkStar 在那儿终结 TLS）
```

两种写法都行：显式的 `/go/fw`，或者直接 `/fw`（不和已有路径撞车时）。服务名不区分大小写，后面的路径和查询参数原样带过去。用 307 而不是 301 是有意的——301 会让浏览器把一个迟早失效的端口永久记住，用户只能清浏览器数据才能恢复；307 还能保住 method 和 body。响应带 `Cache-Control: no-store`。

洞还没打通、或者服务被停用时，给的是一个写明原因的页面，而不是一个注定打不开的跳转。

### 密码与登录

- **首次设置**：第一次进管理后台时引导设置管理密码，**至少 8 位**；设置完成前所有管理接口拒绝访问。
- **登录有效期**：登录后签发 JWT token，默认 7 天，可在 `config/authConfig.json` 里用 `tokenTtlHours` 调整。
- **修改密码**：在「系统设置」里验证旧密码后修改。**改完之后其他设备要重新登录**——改密码会换掉签名密钥，已经发出去的 token 立刻作废。
- **忘记密码**：停掉程序，删除 `config/authConfig.json` 再启动，会重新进设置密码流程（所有已登录设备失效）。
- **桌面版**：本地窗口通过内部通道免登录，浏览器访问仍需密码。

### 数据目录

配置都是本地 JSON 文件，程序目录下：

| 路径 | 说明 |
| --- | --- |
| `config/homeConfig.json` | 导航主页：快捷入口、搜索、分类、布局、壁纸 |
| `config/stunConfig.json` | STUN 服务器列表、设备与服务、入口重定向配置 |
| `config/ddnsConfig.json` | DDNS 服务商、解析记录与同步间隔 |
| `config/certConfig.json` | 证书清单与 ACME 参数 |
| `config/proxyConfig.json` | 反向代理入口与站点 |
| `config/webhookConfig.json` | Webhook 模板 |
| `config/authConfig.json` | 管理密码哈希、JWT 签名密钥与有效期 |
| `data/cert/{证书ID}/` | 证书和私钥 PEM、ACME 账户密钥 |
| `data/icon/` | 上传或抓取来的网站图标 |
| `data/wallpaper/` | 自己上传的壁纸 |
| `logs/YYYY-MM-DD/` | 当天的 `info.log` 与 `err.log` |

> `config/` 里有 DNS 服务商的 API Token、管理密码哈希和证书私钥。**备份要带上它，但不要提交到 Git、不要发到群里**。截图发问题之前先把 `ddnsConfig.json` 里的 token 挡掉。

### 证书

四种来源，外加一种自签：

| 来源 | 什么时候用 | 需要什么 |
| --- | --- | --- |
| 上传 PEM | 已经有证书了，从别处买的或签好的 | 证书和私钥两段文本 |
| 读本地路径 | 机器上已经有 certbot / acme.sh 在续期 | 两个文件路径；文件变了会自动热重载 |
| ACME DNS-01 | 想自动签、想要通配符、80 端口进不来 | 域名在已适配的服务商，且 Token 有 DNS 编辑权限 |
| ACME HTTP-01 | 想自动签，公网 80 端口能进得来 | 域名解析到本机，80 端口入站通 |
| 自签 | 只想要加密、不在乎浏览器认不认（比如纯内网） | 什么都不用 |

**全新安装会自带一张自签证书**，并设为默认。这样「洞口挂 HTTPS」「反代走 HTTPS」这些开关勾上就能用，不会因为一张证书都没有而握手失败。自签浏览器会报不安全，这是正常的——真要给外网用就换成 ACME 签的。删掉之后重启不会再长回来。

ACME 两点提醒：**staging 环境签出来的证书浏览器不认**，验证通了再切正式；正式环境一小时内失败 5 次会被 Let's Encrypt 挡住，别反复试。

证书按 SNI 匹配，支持通配符，到期前自动续期。续期只换指针，**已经连着的连接不会断**。

### 反向代理

就是干 nginx 的活：一个端口收进来，按域名分给不同的内网服务。

- **默认入口**对应 nginx 的 `listen 80` / `listen 443 ssl`，站点不单独指定端口时就挂在这两个上面。首次安装端口预填 80 / 443 但**不启用**——你在页面上点开启之前，不会有任何端口被占上。
- 站点可以**单独占一个端口**，多个端口并存。
- 每个站点自己配域名（可以多个）、路径前缀、要不要 HTTPS、挂哪张证书。
- 自动补 `X-Forwarded-For`，并且把 `Host` 写回客户端原本发来的那个——否则 Jellyfin、Home Assistant 这类服务生成的跳转地址会变成内网 IP。
- SSE 不缓冲，WebSocket 原样透传。
- 后端勾错 http / https 时会自动换一次重试。
- 后端连不上时给一个能看懂的 502 页面，写明是哪个站点、哪个后端连不上。
- 访问日志可开关。

> 「后端走 HTTPS」这个勾只跟**内网那一段**有关：内网服务本身是明文 HTTP 就别勾，它是 https（自签也算）才勾。对外是不是 https，由站点自己的 HTTPS 开关和证书决定，两件事互不影响。

### DDNS 配置说明

解析记录的 IP 来源有五种：

| 来源 | 取哪儿的地址 |
| --- | --- |
| `stun` | STUN 模块探到的公网 IP |
| `web` | 请求一个返回 IP 的网址；留空用内置的 IPv4 / IPv6 查询源 |
| `dns` | 解析另一个域名，跟着它走 |
| `interface` | 读本机网卡。界面会把网卡列出来，看着地址挑，不用背网卡名 |
| `custom` | 固定就是填好的那个 IP，不去探测 |

默认每 5 分钟扫一轮。**没同步成的记录会先隔 30 秒再试，每失败一次翻一倍，最多退回到配置的间隔**——开机时打洞还没拿到公网 IP 这种很快就能补上，而 Token 填错这种不会变成一直去敲服务商的 API。另外公网 IP 一拿到就会立刻触发一次同步，不用等下一轮。

Cloudflare 支持 `proxied`（橙云）开关；NameCheap 目前只适合 IPv4 A 记录。

### 入口重定向（Cloudflare）

**要解决的问题**：打洞拿到的外网端口会变，域名却只能指向 IP、指不了端口。于是每次端口一变，外面存的那个地址就打不开了。

**做法**：让一个固定的入口域名（比如 `nas.example.com`）始终 307 跳到服务当前的真实地址。界面上只要填「入口域名 / 落地域名 / 保留路径」，zone / ruleset / rule 三个 ID 都不用去 Cloudflare 后台抄，后端自己查。服务商复用 DDNS 里已经配好的那个，Token 不用再贴一遍。

同步挂在保活心跳上，地址没变就不打服务商 API。保存服务时会顺手给落地域名补一条 DDNS 记录，家宽 IP 变了也跟得上。

两条解析记录的要求**正好相反**，界面上会把它们的现状摆出来对照：

- **入口域名要开橙云**——不开的话请求根本不经过 Cloudflare，重定向规则轮不到执行。
- **落地域名要关橙云**——开了的话 Cloudflare 的代理不转发打洞出来的高位端口。

配错了两边都不报错，只表现为「访问不了」，所以务必照着界面上那两行核对一遍。

> 删除服务时，自动加的 DNS 记录、Cloudflare 规则和入口域名占位记录会一起收走。三道保险：没打自动标记的不动、你自己改过的不动、还有别的服务在用同一个域名的不动。

### Webhook 变量

Webhook 请求体和 URL 中可以用服务的运行时变量：

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

> 如果你要做的只是「让一个固定域名指向这个服务」，用上面的[入口重定向](#入口重定向cloudflare)，不用写 Webhook。
>
> 「复制服务」出来的新服务，Webhook 默认是关着的。照抄的 URL 常常指着一条具体的记录或规则，两个洞轮流往同一条上写，两边都显示「发送成功」，可那个域名任何时刻只能通到其中一个。

## 开发者指南

面向想要从源码构建、参与开发或二次开发的用户。

### 技术栈

- **后端**：Go、Gin、logrus、pion/stun、goupnp、lego（ACME）
- **桌面壳**：Wails v3（可选，构建托盘桌面版时使用）
- **前端**：React、TypeScript、Vite、Tailwind CSS、lucide-react
- **存储**：本地 JSON 配置文件

### 环境要求

- Go 1.25+
- Node.js 20+ 与 npm（用于构建前端）

### 从源码构建

先构建前端（后端会通过 `embed` 嵌入 `web/home/dist` 与 `web/admin/dist`）：

```bash
cd web/home && npm install && npm run build
cd ../admin && npm install && npm run build
```

再回到项目根目录构建后端：

```bash
cd ../..
go build -o linkstar .     # CLI / 服务版
./linkstar
```

> 修改前端代码后，需重新执行对应前端的 `npm run build`，嵌入的静态资源才会更新。
>
> 更多构建细节（发布版压缩体积、交叉编译、桌面版打包）见 [BUILD.md](BUILD.md)。

### 桌面版（可选）

项目内置 [Taskfile](Taskfile.yml)，可构建基于 Wails v3 的系统托盘桌面版：

```bash
task build:frontend   # 构建 Home / Admin 两个前端
task build            # 构建当前平台的桌面应用
task run              # 运行桌面应用
```

桌面版在托盘常驻，可快速打开管理后台或导航主页，关闭窗口即最小化到托盘。

### 本地开发

后端：

```bash
go run .
```

Home / Admin 前端（分别在各自目录）：

```bash
cd web/home  && npm install && npm run dev
cd web/admin && npm install && npm run dev
```

前端接口默认请求同源 `/api/...`，联调时可在 Vite 开发服务器中配置代理，或直接使用后端嵌入后的静态页面测试。

### 项目结构

```text
.
├── api/              # HTTP API 处理层
├── core/             # 日志、退出保存等基础能力
├── modules/          # home / stun / ddns / cert / proxy / webhook / auth 核心模块
├── routers/          # Gin 路由注册
├── utils/            # 通用工具
├── web/home/         # 导航主页前端
├── web/admin/        # 管理后台前端
├── app.go            # 后端启动、模块初始化、前端资源嵌入
├── main_cli.go       # CLI / 服务版入口
└── main_desktop.go   # Wails 桌面版入口（build tag: desktop）
```

## 路线图

- [x] 反向代理管理
- [x] 证书管理
- [ ] 用户与权限
- [ ] 审计日志与通知中心
- [ ] Docker 镜像
- [ ] 管理端口可配置

## 注意事项

- 服务监听 `0.0.0.0:3333`，局域网内可直接访问；导航主页是公开页面，管理操作受密码保护。
- UPnP 映射依赖网关支持并开启 UPnP。
- 打洞能不能成，取决于运营商的 NAT 类型。对称 NAT（界面上会标出来）成功率低，这是协议层面的限制，不是配置问题。
- 请妥善保护 `config/` 目录：其中有 DNS 服务商凭据、证书私钥和管理密码相关配置，不要提交到 Git 或对外分享。
- **把服务暴露到公网前，先确认那个服务自己的认证、访问控制和防火墙策略**——LinkStar 只保护自己的管理后台，不会给被穿透的服务加认证。

## 社区交流

遇到问题、想反馈需求或一起讨论，欢迎加入交流群：

- **QQ 群**：`1053565441`
- **微信群**：扫描下方二维码加入

<img src="docs/img/wx.png" alt="微信群二维码" width="240">

> 如果群二维码失效或无法进群，可加作者微信 `ZluxYao`，备注 LinkStar。

## License

本项目采用 GPL-3.0-or-later 开源许可证，详见 [LICENSE](LICENSE)。

---

<div align="center">

如果 LinkStar 对你有帮助，欢迎点一个 ⭐ Star 支持一下。

</div>

<sub>**关键词 / Keywords**：STUN、NAT 穿透 / NAT traversal、内网穿透、端口映射 / port forwarding、UPnP、DDNS、动态域名解析、反向代理 / reverse proxy、ACME、Let's Encrypt、证书管理、Webhook、家庭服务器 / homelab、NAS、导航主页 / homepage dashboard、Go、self-hosted。</sub>
