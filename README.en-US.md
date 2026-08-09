

<div align="center">

<img src="docs/img/logo.png" alt="LinkStar Logo" width="140">

# LinkStar

**Pack a navigation homepage, STUN NAT traversal, DDNS, and Webhook notifications into a single Go binary.**

A network entry management tool for home servers, NAS, or soft routers—uses **STUN + UPnP** for NAT traversal, allowing you to expose internal services to the public internet stably without a public IP.

[![Release](https://img.shields.io/github/v/release/ZluxYao/LinkStar?label=Release&color=success)](https://github.com/ZluxYao/LinkStar/releases/latest)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](go.mod)
[![STUN](https://img.shields.io/badge/NAT-STUN%20%2B%20UPnP-orange)](#NAT-traversal)
[![License](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey)](#building-from-source)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](#developer-guide)

![LinkStar Home](docs/img/home.png)

</div>

---

LinkStar consolidates several previously separate tasks—building a beautiful navigation homepage, punching holes for internal services via STUN, updating DNS records as the public IP changes, and notifying external systems when addresses change—all into a single application. Frontend resources are embedded using Go's `embed`, so you only need to download one binary to run it. It provides two interfaces: a **Home Navigation** page and an **Admin Dashboard** (password-protected).

## Table of Contents

- [Why LinkStar](#why-linkstar)
- [Features](#features)
- [NAT Traversal](#NAT-traversal)
- [Interface Preview](#interface-preview)
- [Quick Start](#quick-start)
- [Usage Guide](#usage-guide)
  - [Interface Entries](#interface-entries)
  - [Password & Authentication](#password--authentication)
  - [Data Directories](#data-directories)
  - [DDNS Configuration](#DDNS-configuration)
  - [Webhook Variables](#Webhook-variables)
- [Developer Guide](#developer-guide)
- [Roadmap](#roadmap)
- [Notes & Caveats](#notes--caveats)
- [Community & Support](#community--support)
- [License](#license)

## Why LinkStar

- **All-in-One Binary**: No Docker required, no need to install multiple services. The frontend is bundled with the program, just run `./linkstar` directly.
- **NAT Traversal Without a Public IP**: STUN detects your public exit + UPnP automatically creates port mappings, allowing services behind home broadband NAT to be accessible externally.
- **Automatic Sync on Address Change**: Automatically updates DDNS records and triggers Webhooks when the public IP changes, eliminating the need for manual monitoring.
- **Dual Forms**: Can run as a persistent background service (CLI version) or a system-tray desktop app (based on Wails).

## Features

| Module | Capabilities |
| --- | --- |
| 🏠 Navigation Homepage | App shortcuts, categories with drag-and-drop sorting, search engine management, Bing daily wallpaper, icon upload & auto-scraping |
| 🌐 NAT Traversal | STUN detects local/public IP & NAT routing path, UPnP automatically creates port mappings, maintains external access URLs |
| 🔌 Service Management | Manage TCP/UDP services per device, configure internal ports, mapped ports, HTTPS tags, and visibility on the homepage |
| 🔁 DDNS Resolution | Supports A/AAAA records, periodically syncs public IP to DNS providers |
| 📡 Webhook | Pushes HTTP requests on service address changes, includes built-in templates for generic JSON, Cloudflare SRV, and Cloudflare redirect rules |
| ⚡ Real-time Status | Backend performs periodic health checks; Web UI pushes service status changes in real-time via SSE |
| 🔐 Password Protection | Guided setup for admin password on first use; admin dashboard & all API endpoints require login (JWT token); desktop app local windows bypass login |

**Supported DNS Providers**: Cloudflare, Aliyun DNS, Tencent DNSPod, Baidu Cloud, Huawei Cloud, NameCheap, NameSilo.

## NAT Traversal

LinkStar's NAT traversal is built on the standard **STUN** protocol (implemented via [pion/stun](https://github.com/pion/stun)):

1. **STUN Discovery**: Sends Binding requests to public STUN servers to obtain the public exit IP and port behind the NAT, and determines the NAT type.
2. **Port Reuse for Hole Punching**: Listens on the same local port (TCP/UDP) to maintain the NAT mapping established by the STUN session.
3. **Automatic UPnP Mapping**: Automatically creates port mappings when the gateway supports UPnP, directing public ports to internal services in TCP scenarios.
4. **Port Forwarding**: Forwards inbound connections from the internet to the internal port of the target device, enabling service exposure without a public IP.
5. **Heartbeat & Keep-Alive**: Periodic health checks and reconnections automatically detect public port changes and trigger DDNS/Webhook syncs.

> Ideal for scenarios like home broadband (CGNAT), ISP NAT, or soft routers without a dedicated public IP, serving as a lightweight self-hosted alternative to frp/ngrok.

## Interface Preview

See the header screenshot for the navigation homepage. The NAT traversal management interface in the admin dashboard looks like this:

<img src="docs/img/stun.png" alt="LinkStar NAT Traversal">

## Quick Start

Download the corresponding platform binary from [Releases](https://github.com/ZluxYao/LinkStar/releases/latest) and run it directly:

```bash
# Linux / macOS
./linkstar
```

```powershell
# Windows
.\linkstar.exe
```

After starting, open `http://localhost:3333/`. On first run, it will automatically create `config/`, `data/`, and `logs/` directories. The first time you access the admin dashboard, you'll be guided to set an admin password. After that, it's ready to use with no other prerequisites.

> For building from source or using the system-tray desktop version, see the [Developer Guide](#developer-guide).

## Usage Guide

### Interface Entries

| Entry | Address | Description |
| --- | --- | --- |
| Home Navigation | `http://localhost:3333/` | Publicly accessible, no login required |
| Admin Dashboard | `http://localhost:3333/linkstar/` | Requires password login |

> The service listens on `0.0.0.0:3333` (port is not currently configurable). Other devices on the LAN can access it via the local IP.

### Password & Authentication

- **Initial Setup**: Guides you to set an admin password upon first access to the dashboard. All admin endpoints are blocked until this is completed.
- **Session Duration**: Issues a JWT token upon login, valid for 7 days by default. Can be adjusted via `tokenTtlHours` in `config/authConfig.json`.
- **Change Password**: Verify the old password within the dashboard to set a new one.
- **Forgot Password**: Stop the program, delete `config/authConfig.json`, and restart. This will trigger the password setup process again (all existing login tokens will be invalidated).
- **Desktop Version**: Local windows bypass login via internal channels; browser access still requires a password.

### Data Directories

LinkStar persists configuration using local JSON files:

| Path | Description |
| --- | --- |
| `config/homeConfig.json` | Homepage navigation, search, categories, layout, wallpaper config |
| `config/stunConfig.json` | STUN server list, device & service config |
| `config/ddnsConfig.json` | DDNS provider, record config, sync interval |
| `config/webhookConfig.json` | Webhook template config |
| `config/authConfig.json` | Admin password hash, JWT secret, token TTL |
| `data/icon/` | User-uploaded or scraped website icons |
| `logs/YYYY-MM-DD/` | Runtime & error logs |

> These directories typically contain local state or sensitive credentials and are excluded from Git by default.

### DDNS Configuration

DDNS records support the following IP sources:

- `stun`: Public IP detected by the STUN module.
- `web`: Obtained from public IP lookup APIs. Uses built-in IPv4/IPv6 sources if no URL is provided.
- `dns` / `interface`: Types are reserved; current implementations are under development.

Cloudflare supports the `proxied` toggle; NameCheap currently only supports IPv4 A records.

### Webhook Variables

Service runtime variables can be used in the Webhook request body and URL, for example:

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

Useful for syncing to external systems after port changes, service restarts, or address updates.

## Developer Guide

For users who want to build from source, contribute to development, or create custom builds.

### Tech Stack

- **Backend**: Go, Gin, logrus, pion/stun, goupnp
- **Desktop Wrapper**: Wails v3 (optional, used for building the tray-based desktop app)
- **Frontend**: React, TypeScript, Vite, Tailwind CSS, lucide-react
- **Storage**: Local JSON configuration files

### Requirements

- Go 1.25+
- Node.js 20+ and npm (for building the frontend)

### Building from Source

First, build the frontend (the backend will embed `web/home/dist` and `web/admin/dist` via `embed`):

```bash
cd web/home && npm install && npm run build
cd ../admin && npm install && npm run build
```

Then, return to the project root to build the backend:

```bash
cd ../..
go build -o linkstar .     # CLI / Service version
./linkstar
```

> After modifying frontend code, you must re-run the corresponding frontend's `npm run build` for the embedded static assets to update.
>
> For more build details (release size optimization, Windows considerations, desktop packaging), see [BUILD.md](BUILD.md).

### Desktop Version (Optional)

The project includes a [Taskfile](Taskfile.yml) to build the Wails v3 system-tray desktop version via `task`:

```bash
task build:frontend   # Build Home / Admin frontends
task build            # Build desktop app for current platform
task run              # Run desktop app
```

The desktop app runs in the system tray, allowing quick access to the admin dashboard or homepage. Closing the window minimizes it to the tray.

### Local Development

Backend:

```bash
go run .
```

Home / Admin Frontends (in their respective directories):

```bash
cd web/home  && npm install && npm run dev
cd web/admin && npm install && npm run dev
```

The frontend API requests the same-origin `/api/...` by default. During integration testing, you can configure a proxy in the Vite dev server or directly test using the static pages embedded by the backend.

### Project Structure

```text
.
├── api/              # HTTP API handling layer
├── core/             # Logging, graceful shutdown, and other core utilities
├── modules/          # home / stun / ddns / webhook / auth core modules
├── routers/          # Gin route registration
├── utils/            # Common utilities
├── web/home/         # Homepage navigation frontend
├── web/admin/        # Admin dashboard frontend
├── app.go            # Backend startup, module initialization, frontend embedding
├── main_cli.go       # CLI / Service version entry point
└── main_desktop.go   # Wails desktop version entry point (build tag: desktop)
```

## Roadmap

- [ ] Reverse Proxy Management
- [ ] Certificate Management
- [ ] Users & Permissions
- [ ] Audit Logs & Notification Center
- [ ] Docker / systemd Deployment Examples

## Notes & Caveats

- The service listens on `0.0.0.0:3333` and is directly accessible on the LAN. The navigation homepage is public, while administrative actions are password-protected.
- UPnP mapping relies on your gateway supporting and having UPnP enabled.
- Protect the `config/` directory carefully: it contains DNS provider credentials and admin password configurations (`authConfig.json`). Do not commit it to Git or share it externally.
- Before exposing services to the public internet, ensure the exposed services themselves have proper authentication, access controls, and firewall rules. LinkStar only secures its own admin dashboard and does not add authentication to tunneled services.

## Community & Support

If you encounter issues, want to suggest features, or discuss ideas, join our community:

- **QQ Group**: `1053565441`
- **WeChat Group**: Scan the QR code below to join

<img src="docs/img/wx.jpg" alt="WeChat Group QR Code" width="240">

> If the group QR code expires or you can't join, add the author on WeChat `ZluxYao` with the remark "LinkStar".

## License

This project is licensed under the GPL-3.0-or-later license. See [LICENSE](LICENSE) for details.

---

<div align="center">

If LinkStar is helpful to you, feel free to leave a ⭐ Star to show your support.

</div>

<sub>**Keywords**: STUN, NAT traversal, port forwarding, UPnP, DDNS, Webhook, homelab, NAS, homepage dashboard, Go, self-hosted.</sub>
