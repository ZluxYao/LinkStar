# 编译命令

LinkStar 有两个编译目标,靠 Go build tag `desktop` 区分:

| 目标 | build tag | 入口文件 | 产物形态 |
|------|-----------|----------|----------|
| CLI 版 | 无 | `main_cli.go`(`//go:build !desktop`) | 命令行程序,前台跑,看日志 |
| 桌面版 | `desktop` | `main_desktop.go`(`//go:build desktop`) | 托盘 + 窗口的桌面 App(Wails v3) |

两者共用 `app.go`(后端启动、模块初始化)。前端资源通过 `go:embed` 打进二进制。

---

## 前置:先构建前端

Go 编译依赖 `web/admin/dist` 和 `web/home/dist`(embed 目标)。**改过前端后必须先重新构建**,否则打进去的是旧资源。

```powershell
# 分别构建
cd web/admin; npm run build; cd ../..
cd web/home;  npm run build; cd ../..
```

或用 Taskfile 一把梭:

```powershell
task build:frontend
```

---

## CLI 版

**开发/调试**(带日志输出):

```powershell
go build -o bin/linkstar.exe .
```

**发布**(去符号表,体积更小):

```powershell
go build -ldflags "-s -w" -o bin/linkstar.exe .
```

> CLI 版不要加 `-H windowsgui`,否则控制台日志看不到。

---

## 桌面版

### 方式一:裸 `go build`(快速验证)

```powershell
go build -tags desktop -ldflags "-s -w -H windowsgui" -o bin/linkstar-desktop.exe .
```

- `-tags desktop`:切到 `main_desktop.go` 入口
- `-H windowsgui`:双击运行时**不弹黑色控制台窗口**(桌面 App 必须加)

> 这种方式产物**没有内嵌图标/版本信息**(缺 `.syso` 资源),仅适合本地跑起来看效果。

### 方式二:Wails 官方打包(正式发布)

带图标、清单、版本信息的完整打包,走 Taskfile:

```powershell
task build
```

等价于 `build/windows/Taskfile.yml` 里的:

```powershell
# 1. 生成 Windows 资源文件(图标 + manifest + 版本信息)
wails3 generate syso -arch amd64 -icon build/windows/icon.ico -manifest build/windows/wails.exe.manifest -info build/windows/info.json -out wails_windows_amd64.syso

# 2. 编译(production + desktop 双 tag)
go build -tags "production desktop" -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o bin/linkstar.exe .

# 3. 清理临时 .syso
Remove-Item -LiteralPath wails_windows_amd64.syso -Force
```

需要先装 Wails v3 CLI:

```powershell
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
```

---

## 运行

```powershell
# CLI 版:前台运行,Ctrl+C 退出(自动保存配置)
bin/linkstar.exe

# 桌面版:双击或命令行启动,托盘「退出」保存配置
bin/linkstar-desktop.exe
```

两个版本都监听 `0.0.0.0:3333`:

- 管理后台:http://127.0.0.1:3333/linkstar/
- 导航主页:http://127.0.0.1:3333/

---

## 交叉编译(在 Windows 上编其他平台)

CLI 版可跨平台;桌面版依赖各平台的 Wails/WebView,交叉编译不保证可用,建议在目标平台本机编。

```powershell
# Linux amd64(CLI)
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -ldflags "-s -w" -o bin/linkstar-linux .

# macOS arm64(CLI)
$env:GOOS="darwin"; $env:GOARCH="arm64"; go build -ldflags "-s -w" -o bin/linkstar-mac .

# 编完记得清掉环境变量,免得影响后续本机编译
Remove-Item Env:GOOS; Remove-Item Env:GOARCH
```
