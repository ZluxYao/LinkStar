# LinkStar 首页开发提示词

## 项目背景

LinkStar 是一个 Go 编写的 NAT 穿透工具，后端已实现 STUN/UPnP 内网穿透。
现在需要开发一个浏览器首页导航页（类似 Sun-Panel），把穿透出去的服务以图标形式展示，
用户可以像手机桌面一样点击打开服务。

---

## 技术栈

- React 18 + TypeScript
- Vite 构建
- Tailwind CSS（样式）
- shadcn/ui（UI 组件库，仅用于设置面板/弹窗）
- 项目目录：`web/`，构建输出 `web/dist/`（Go embed 加载）

---

## 视觉风格

参考 Sun-Panel：
- 全屏壁纸背景（支持图片/视频）
- 顶部中央：时钟 + 日期
- 主体：图标网格（圆角方形图标 + 名称标签）
- 支持分类分组，分组名称作为小标题
- 整体简洁，无多余边框/卡片感
- 右下角：网络切换按钮 + 设置齿轮图标

---

## 页面结构

```
/          首页（本文件描述的导航页）
/admin     后台管理（已有 Vue，暂不动）
```

---

## 功能需求

### 1. 图标网格

- 图标样式：圆角方形（iOS App 风格），大小 80x80px
- 图标下方显示服务名称
- 图标来源优先级：
  1. 用户手动上传的图标（存 `/data/icons/`）
  2. 自动抓取 favicon：`https://icon.horse/icon/{域名}`  这个可用后端写api抓？也放到打他/icos
  3. 降级：取服务名首字母，生成彩色字母头像
- 点击图标：默认在小窗（iframe）内打开，可在设置中改为新标签页

### 2. 数据来源

**自动服务**（从后端 STUN 映射拉取）：

```
GET /api/stun/status
```

返回已穿透的服务列表，字段参考：
```json
{
  "services": [
    {
      "id": "xxx",
      "name": "NAS 管理",
      "internalPort": 5000,
      "externalPort": 34521,
      "protocol": "tcp",
      "status": "alive",
      "device": { "ip": "192.168.1.100" }
    }
  ]
}
```

根据网络模式拼接 URL：
- 公网模式：`http://{公网IP}:{externalPort}`
- 内网模式：`http://{device.ip}:{internalPort}`

公网 IP 从 `GET /api/stun/network` 获取。

**自定义链接**（用户手动添加，存 homeConfig.json）：
```json
{
  "name": "GitHub",
  "url": "https://github.com",
  "icon": "/data/icons/github.png",
  "openMode": "window"
}
```

### 3. 分类分组

- 第一个分组"我的服务"固定，内容来自 STUN 自动映射
- 其他分组用户自定义，可增删改排序
- 分组标题小字显示，图标横向排列，自动换行
- 空分组不显示

### 4. 小窗（Mini Window）

- 点击图标弹出居中浮层（宽 900px，高 600px，可拖拽）
- 内部是 `<iframe>` 加载目标 URL
- 右上角关闭按钮
- 标题栏显示服务名
- 支持同时打开多个小窗（层叠显示，点击置顶）
- 如果目标 URL 不支持 iframe（X-Frame-Options 拦截），显示提示并提供"在新标签页打开"按钮

### 5. 网络切换（右下角）

```
[🌐 公网]  ←→  [🏠 内网]
```

- 切换后，所有服务图标的 URL 跟随变化
- 状态存 localStorage，刷新保留
- 显示当前公网 IP（从接口获取）

### 6. 顶部时钟

- 居中显示当前时间（HH:MM:SS）和日期（M月D日 星期X）
- 字体白色，阴影，实时更新

### 7. 壁纸

- 全屏背景，支持：
  - 图片（CSS background-image）
  - 视频（`<video autoplay loop muted>`）
- 壁纸 URL 从 homeConfig 读取
- 壁纸上可叠加模糊/亮度调整（CSS filter）

### 8. 设置面板（右下角齿轮）

点击弹出设置抽屉，包含：

- **壁纸设置**：上传文件 or 填 URL，blur/brightness 滑块
- **搜索引擎**：Google / Baidu / Bing / 自定义
- **图标打开方式**：小窗 / 新标签页
- **自定义分组管理**：增删改分组，拖拽排序图标
- **自定义 CSS**：textarea，注入 `<style>` 标签
- **自定义 HTML**：textarea，注入到 body 末尾

---

## 后端接口（需配合 Go 后端新增）

```
GET  /api/home/config          读取 homeConfig.json
POST /api/home/config          保存 homeConfig.json
POST /api/home/upload          上传图标/壁纸文件，返回访问路径
GET  /api/stun/status          获取 STUN 服务状态列表（已有）
GET  /api/stun/network         获取公网 IP 信息（已有或新增）
```

homeConfig.json 结构：
```json
{
  "wallpaper": {
    "type": "image",
    "url": "/data/wallpapers/bg.jpg",
    "blur": 4,
    "brightness": 0.7
  },
  "search": {
    "engine": "google",
    "customUrl": ""
  },
  "openMode": "window",
  "groups": [
    {
      "id": "custom-1",
      "name": "常用网站",
      "items": [
        {
          "id": "item-1",
          "name": "GitHub",
          "url": "https://github.com",
          "icon": "",
          "openMode": "tab"
        }
      ]
    }
  ],
  "customCSS": "",
  "customHTML": ""
}
```

---

## 文件结构

```
web/
├── src/
│   ├── main.tsx
│   ├── App.tsx
│   ├── pages/
│   │   └── Home.tsx              主页
│   ├── components/
│   │   ├── Clock.tsx             时钟组件
│   │   ├── AppGrid.tsx           图标网格
│   │   ├── AppIcon.tsx           单个图标
│   │   ├── AppWindow.tsx         小窗（iframe 浮层）
│   │   ├── WindowManager.tsx     多窗口管理
│   │   ├── SearchBar.tsx         搜索框
│   │   ├── GroupSection.tsx      分类分组
│   │   ├── NetworkToggle.tsx     公网/内网切换
│   │   └── SettingsPanel.tsx     设置面板
│   ├── hooks/
│   │   ├── useHomeConfig.ts      读写 homeConfig
│   │   ├── useStunServices.ts    拉取 STUN 服务列表
│   │   └── useNetwork.ts         网络模式状态
│   ├── types/
│   │   └── index.ts              TypeScript 类型定义
│   └── utils/
│       └── favicon.ts            favicon 抓取逻辑
├── index.html
├── vite.config.ts
├── tailwind.config.ts
└── tsconfig.json
```

---

## 注意事项

1. 首页不需要登录鉴权（自托管，局域网内访问）
2. iframe 加载失败要优雅降级，不能白屏
3. 图标网格响应式，移动端也能用
4. 所有接口请求加 loading 状态，避免空白闪烁
5. 拖拽排序图标用 `@dnd-kit/core`
6. 构建后输出到 `web/dist/`，Go 通过 `//go:embed web/dist` 加载

---

## 开发启动

```bash
cd web
npm create vite@latest . -- --template react-ts
npm install
npm install -D tailwindcss @tailwindcss/vite
npm install @dnd-kit/core @dnd-kit/sortable
npm install lucide-react        # 图标库
npx shadcn@latest init          # UI 组件

npm run dev    # 开发服务器 localhost:5173
npm run build  # 构建到 dist/
```

vite.config.ts 配置代理（开发时转发到 Go 后端）：
```ts
server: {
  proxy: {
    '/api': 'http://localhost:3333',
    '/data': 'http://localhost:3333',
  }
}
```
