# AGENTS.md

给在这个仓库里干活的 AI 编码助手看的约束。先读这份，再动代码。

## 项目是什么

LinkStar：家宽内网穿透 / 自托管入口工具。STUN 打洞、UPnP、DDNS、ACME 证书、内建反向代理、导航主页、Webhook 通知。

- 后端：Go 1.25 + gin + logrus，单一二进制，前端产物通过 `go:embed` 打进去
- 前端：两个独立的 React 19 + Vite + Tailwind 4 + TypeScript 项目
  - `web/admin` 管理后台，挂在 `/linkstar/`，hash 路由
  - `web/home` 导航主页，挂在 `/`
- 两个编译目标，靠 build tag 区分（详见 [BUILD.md](BUILD.md)）：
  - CLI 版：`main_cli.go`（`//go:build !desktop`）
  - 桌面版：`main_desktop.go`（`//go:build desktop`，Wails v3 beta）
- 监听固定 `0.0.0.0:3333`；配置在**工作目录**下的 `config/*.json`，用户数据在 `data/`

## 目录分层

```
main_cli.go / main_desktop.go   入口，只管启动方式
app.go                          启动后端、并发初始化各模块、跨模块依赖注入
routers/                        路由注册（xxx_routers.go），只做 URL → handler 的映射
api/<模块>_api/                 HTTP handler，一个接口一个文件；enter.go 定义空结构体
middleware/                     鉴权、请求绑定
modules/<模块>/                 业务逻辑与运行时状态，handler 不写业务
modules/<模块>/model/           配置/数据结构
core/                           日志、退出时保存
utils/                          通用工具（res 统一响应、utilsFile 读写 JSON 等）
web/admin, web/home             前端源码 + 已提交的 dist
test/                           手工实验用的独立小程序（多数有自己的 go.mod），不是单元测试
docs/                           面向用户的文档（中文）
```

依赖方向：`routers → api → modules → utils/core`。不要让 `modules` 反向 import `api` 或 `routers`。

## 后端约定

### 新增一个接口

1. `api/<模块>_api/` 下新建文件，定义 `XxxRequest` 结构体和 `func (XxxApi) XxxView(c *gin.Context)`
2. 在 `routers/<模块>_routers.go` 注册，请求体用 `middleware.BindJsonMiddleware[XxxRequest]`（query 用 `BindQueryMiddleware`），handler 里用 `middleware.GetBindRequest[XxxRequest](c)` 取
3. 响应一律走 `utils/res`：`res.OkWithData` / `res.OkWithMsg` / `res.FailWithMsg` 等。HTTP 状态码保持 200，业务状态看 `code`（0 成功，7 通用失败，401 未登录，428 未初始化，429 限流）
4. 需要登录的接口挂在 `routers/enter.go` 里的 `protected` 组下；只有鉴权自身、主页只读接口、系统信息才放公开组
5. 前端 `web/<端>/src/lib/api.ts`（或 `api.ts`）和 `types.ts` 同步加上调用与类型

### 模块运行时与持久化

- 每个模块有一个全局 `Runtime`（见 `modules/webhook/enter.go`），内部 `sync.RWMutex` 保护配置
- 读：通过返回**快照**的方法（拷贝切片），不要把内部切片/指针直接交出去
- 写：通过 `Runtime.Update(func(cfg *Config) error)` 这一类方法，在锁内修改、mutator 出错不落盘、解锁后再写文件
- 配置文件读写用 `utils/utilsFile.ReadJsonFile / WriteJsonFile`；文件不存在时生成默认配置
- 在 `InitXxx()` 里用 `core.OnShutdown` 注册退出保存
- 新模块要在 `app.go` 的 `startModulesInBackground` 里 `initModule(...)`，注意已有的顺序依赖（如 Cert 必须在 STUN 前）
- 配置结构加字段要考虑**老配置文件**：零值必须是合理默认，或在读取时补默认值，不能让老用户升级后行为突变

### 跨模块依赖：不准循环 import

`ddns → stun → cert` 已经是固定方向。下游模块需要上游能力时：

- 下游声明一个窄接口或回调类型 + `RegisterXxx(...)`（参考 `modules/stun/hooks.go`、`cert.RegisterDNSSolverFactory`）
- 由 `app.go` 在启动时注入实现
- 不要为了图省事把逻辑挪进 `api` 层去拼两个模块

### 并发

- 定时任务、打洞、同步都在 goroutine 里跑，修改共享状态必须持锁
- 对同一个外部资源做"读-改-写"（例如 Cloudflare 一个 zone 的 ruleset）要串行化，按资源 key 加锁
- 不要在持锁期间做网络请求或写文件

### 平台差异

- Windows / 非 Windows 的实现用文件名后缀 + build tag 分开（`xxx_windows.go` / `xxx_other.go`）
- 改了任意一边，确认另一边签名一致；至少交叉编一次 Linux：`GOOS=linux go build ./...`

### 日志与错误

- 日志用 `logrus`，中文；错误包一层上下文：`fmt.Errorf("读取 xx 配置失败: %w", err)`
- 返回给前端的错误信息是给用户看的：说清楚哪里不对、该怎么办，别直接甩底层英文错误
- **不要报假成功**：部分完成的操作要把"还差什么"告诉前端（`api.ts` 的 `requestWithMsg` 就是为此存在）

## 前端约定

- 两个前端各自独立的 `package.json`，改哪边就在哪边目录下操作
- 开发：`npm run dev`（admin 端口 3010，代理 `/api`、`/data` 到 3333）
- **`dist/` 是提交进仓库的**，Go 编译时 embed 它。改了前端源码，必须 `npm run build`（或 `task build:frontend`）并把新的 dist 一起提交，否则二进制里还是旧界面
- 构建前过一遍：`npm run lint`、`npm run build`（含 `tsc -b`）
- 要兼顾手机端宽度（弹窗、表格已经做过移动端适配，别改回去）
- 新增后端字段时同步更新 `types.ts`

## 构建与验证

改完至少跑：

```powershell
go build ./...
go vet ./...
go test ./api/... ./modules/... ./routers/... ./utils/...
```

- 不要直接 `go test ./...`：`test/` 下是手工实验程序，有的会真的去连外网 STUN 服务器
- 涉及桌面版的改动再加：`go build -tags desktop ./...`
- 涉及平台分支的改动再加：`$env:GOOS="linux"; go build ./...; Remove-Item Env:GOOS`
- 完整发布构建：`task build`（需要 Wails v3 CLI）

### 测试风格

- 表驱动，用例名用中文描述场景（参考 `modules/ddns/ddns_landing_test.go`）
- 外部 API（Cloudflare 等）用 `httptest` 假服务器；需要替换地址时把 const 改成包级 var，并在注释里说明只为测试
- 修 bug 先补一个能复现的测试

## 安全与隐私

- 示例域名一律用 `example.com`，公网 IP 用 RFC 5737 文档地址段（`203.0.113.x` 等），注释、测试、文档、截图都一样
- 不要提交 `config/`、`data/`、`bin/`、`logs/`、任何 token/密钥（已在 `.gitignore`，别绕过）
- 接口返回 DNS 服务商凭证时必须脱敏
- 鉴权相关（`modules/auth`、`middleware/auth.go`、桌面 secret）改动要格外谨慎：密码校验并发限制、登录失败次数限制不能被削弱

## 版本号

发版时以下几处必须一起改，单独一个提交，信息写 `版本号 x.y.z`：

- `api/system_api/version.go` 的 `Version`
- `build/windows/info.json`（4 处）
- `build/windows/wails.exe.manifest`（`x.y.z.0`）
- `web/admin/src/layout/Sidebar.tsx` 的默认版本号，然后重新构建 admin 的 dist

非发版任务不要动版本号。

## 代码风格

- Go：`gofmt`；导出符号都写中文注释，以符号名开头
- 注释写**为什么**，不复述代码在干嘛；踩过的坑、看起来多余但不能删的代码要写清楚原因（参考 `app.go` 里依赖注入的注释）
- 跟着周围代码的命名和写法走，不要顺手大面积重构、改格式、改无关文件
- 不要引入新依赖，除非确实必要并说明理由

## Git 提交

- 提交信息用中文
- 标题写**用户能感知到的现象或改动**，而不是技术动作，例如「开了两个以上入口重定向，它们互相把对方按回旧端口」
- 正文讲清楚：原因是什么、为什么这样修、有什么取舍、顺带改了什么
- 一个提交做一件事；前端 dist 跟对应的源码改动放在同一个提交
- 只在用户要求时提交/推送；不要改写已推送的历史







# 修改前

要和我对其改什么，尽量最小的修改，同时保持整个项目的优雅