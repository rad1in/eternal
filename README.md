[English](/README.md) | [فارسی](/README.fa_IR.md) | [العربية](/README.ar_EG.md) | [中文](/README.zh_CN.md) | [Español](/README.es_ES.md) | [Русский](/README.ru_RU.md) | [Türkçe](/README.tr_TR.md)

<p align="center">
  <img src="media/readme/banner.svg" alt="Eternal: the self-hosted panel for proxy and VPN servers" width="100%">
</p>

<p align="center">
  <a href="https://github.com/rad1in/eternal/releases/latest"><img src="https://img.shields.io/github/v/release/rad1in/eternal?style=flat-square&color=6d5ff0&label=release" alt="Latest release"></a>
  <a href="https://github.com/rad1in/eternal/releases"><img src="https://img.shields.io/github/downloads/rad1in/eternal/total?style=flat-square&color=6d5ff0" alt="Downloads"></a>
  <a href="https://www.gnu.org/licenses/gpl-3.0.en.html"><img src="https://img.shields.io/badge/license-GPL--3.0-6d5ff0?style=flat-square" alt="License"></a>
  <img src="https://img.shields.io/badge/Go-1.27-6d5ff0?style=flat-square&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/React-19-6d5ff0?style=flat-square&logo=react&logoColor=white" alt="React">
  <img src="https://img.shields.io/badge/platforms-linux%20%C2%B7%20windows-6d5ff0?style=flat-square" alt="Platforms">
</p>

<p align="center">
  <b>Eternal</b> runs your proxy and VPN servers from one place: <b>Xray-core, OpenVPN, IKEv2, L2TP, SSTP, OpenConnect, SSH, WireGuard and more</b>,
  with clients, quotas, nodes, encrypted tunnels, a shop and a Telegram bot built in.<br>
  One binary. SQLite by default. Set up in a browser in under a minute.
</p>

<p align="center">
  <a href="#-quick-start">Quick start</a> ·
  <a href="#-see-it">Screenshots</a> ·
  <a href="#-what-you-get">Features</a> ·
  <a href="#-more-servers-in-one-command">Nodes</a> ·
  <a href="#-tunnels">Tunnels</a> ·
  <a href="#-sell-it">Shop</a> ·
  <a href="#-comparison-with-3x-ui">Comparison</a> ·
  <a href="#-documentation">Docs</a>
</p>

> [!NOTE]
> **Eternal is a fork of [3x-ui](https://github.com/MHSanaei/3x-ui)** by MHSanaei (itself a fork of X-UI). It keeps the 3x-ui core and adds a lot of its own, listed below.

> [!IMPORTANT]
> This project is intended for personal use. Please do not use it for illegal purposes.

---

## ✨ What is new in 1.4

| | |
| --- | --- |
| 🚀 **Setup in the browser** | The installer prints one link, opens it for you, and a four-step wizard creates your admin account, port and secret path. No credentials are typed in a terminal. |
| 🖥️ **Nodes in one command** | `curl … /install-node.sh \| bash -s -- --panel … --token …` installs a server and adds it to your panel. |
| 🔗 **Built-in tunnels** | [WatTunnel](https://github.com/JohnWatsoon/WatTunnel) (MIT) runs inside the panel: encrypted Iran ↔ Kharej forwarding between the panel, your nodes and plain servers, with live status. |
| 🤖 **A much stronger shop bot** | New menu, per-user language, calculator, add-on orders, card receipts with admin approval, forced channel join, Telegram Mini App and more. |

---

## 🚀 Quick start

```bash
bash <(curl -Ls https://raw.githubusercontent.com/rad1in/eternal/main/install.sh)
```

<p align="center">
  <img src="media/readme/terminal-install.png" alt="The installer prints a one-time setup link" width="760">
</p>

The installer prints a one-time link to a setup wizard (and opens it in your browser when the machine has one). The panel shows nothing else until you finish: every other address answers like an unknown site.

<table>
  <tr>
    <td width="50%"><img src="media/readme/setup-2-account-dark.png" alt="Step 2: create the admin account"></td>
    <td width="50%"><img src="media/readme/setup-3-address-dark.png" alt="Step 3: choose the port and the secret path"></td>
  </tr>
  <tr>
    <td align="center"><sub><b>Admin account</b> with a strength check</sub></td>
    <td align="center"><sub><b>Port and secret path</b>, checked live</sub></td>
  </tr>
</table>

When you press **Create my panel** the wizard replaces the default login, moves the panel to the address you chose and sends you there. The link stops working. Lost it? Run `eternal setup-link` on the server for a new one.

Run `eternal` any time for the management menu (start, stop, update, certificates, backups, reset credentials).

<details>
<summary><b>Unattended install, a specific version, or Docker</b></summary>

```bash
# Pre-set the credentials and skip the wizard (cloud-init, scripts)
XUI_NONINTERACTIVE=1 XUI_USERNAME=admin XUI_PASSWORD='a-long-password' \
  bash <(curl -Ls https://raw.githubusercontent.com/rad1in/eternal/main/install.sh)

# A specific release
bash <(curl -Ls https://raw.githubusercontent.com/rad1in/eternal/main/install.sh) v1.4.0

# Docker
docker compose up -d
```

Fail2ban inside the image bans with `iptables`; with plain `docker run` add `--cap-add=NET_ADMIN --cap-add=NET_RAW`. There is also a Helm chart in [`deploy/helm`](deploy/helm) and cloud-init templates in [`deploy/cloud-init`](deploy/cloud-init).

</details>

The installer and the update script download release archives from this repository's [releases](https://github.com/rad1in/eternal/releases). They support Debian/Ubuntu, RHEL-family, Arch and Alpine on amd64, arm64, armv5/v6/v7, 386 and s390x, plus a Windows build.

---

## 🖼 See it

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="media/readme/dashboard-dark.png">
    <img src="media/readme/dashboard-light.png" alt="Dashboard" width="100%">
  </picture>
</p>

<table>
  <tr>
    <td width="50%">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="media/readme/inbounds-dark.png">
        <img src="media/readme/inbounds-light.png" alt="Inbounds">
      </picture>
    </td>
    <td width="50%">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="media/readme/clients-dark.png">
        <img src="media/readme/clients-light.png" alt="Clients">
      </picture>
    </td>
  </tr>
  <tr>
    <td align="center"><sub><b>Inbounds</b> for every protocol</sub></td>
    <td align="center"><sub><b>Clients</b> with quota, expiry and live usage</sub></td>
  </tr>
  <tr>
    <td width="50%">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="media/readme/nodes-dark.png">
        <img src="media/readme/nodes-light.png" alt="Nodes">
      </picture>
    </td>
    <td width="50%">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="media/readme/tunnels-dark.png">
        <img src="media/readme/tunnels-light.png" alt="Tunnels">
      </picture>
    </td>
  </tr>
  <tr>
    <td align="center"><sub><b>Nodes</b>: status, load, cost and caps</sub></td>
    <td align="center"><sub><b>Tunnels</b> between the panel and its nodes</sub></td>
  </tr>
  <tr>
    <td width="50%">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="media/readme/sales-dark.png">
        <img src="media/readme/sales-light.png" alt="Sales">
      </picture>
    </td>
    <td width="50%">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="media/readme/shop-dark.png">
        <img src="media/readme/shop-light.png" alt="Public shop">
      </picture>
    </td>
  </tr>
  <tr>
    <td align="center"><sub><b>Sales</b>: plans, orders, discounts, tickets</sub></td>
    <td align="center"><sub>The <b>public shop</b> customers buy from</sub></td>
  </tr>
</table>

<sub>The screenshots show demo data on a test machine: two real node panels, one real tunnel, made-up clients and plans.</sub>

---

## 🧰 What you get

<table>
  <tr>
    <td width="33%" valign="top">

**Protocols**
- VLESS, VMess, Trojan, Shadowsocks, REALITY, XTLS
- Hysteria2, TUIC v5, WireGuard, AmneziaWG, MTProto
- **OpenVPN**, **IKEv2/IPsec**, **L2TP/IPsec**, **SSTP**, **OpenConnect**
- **SSH tunnel** accounts
- Per-client quota, expiry, IP and device limits, instant disconnect

    </td>
    <td width="33%" valign="top">

**Run it well**
- Roles and per-admin scoping, admin sessions, audit log
- Panel IP allow-list, login lockout, captcha, new-IP alerts
- Encrypted and scheduled backups to disk or S3
- Prometheus metrics and a Grafana dashboard
- SQLite, PostgreSQL or MySQL/MariaDB
- Decoy site for visitors who do not know the secret path

    </td>
    <td width="33%" valign="top">

**Keep users connected**
- Cloudflare clean-IP scanner, port and SNI rotation
- Automatic subscription failover to a backup node
- Subscriptions for Xray, sing-box, Clash, Outline, Surge, Quantumult X, Loon
- Import from Marzban, Marzneshin and Hiddify
- Webhooks, ntfy, Gotify, Slack, Bale, Telegram and Discord alerts

    </td>
  </tr>
</table>

### 🖥 More servers in one command

Open **Nodes → Install command**, copy the command, run it as root on a fresh server. That is all: the panel serves the installer, the server installs Eternal and appears in your node list.

<p align="center">
  <img src="media/readme/terminal-node.png" alt="One command adds a node" width="860">
</p>

- The token works once and expires in an hour.
- `--name`, `--port`, `--version`, `--pin` (for a self-signed master), `--detect` (print what is on the server and change nothing) and `--uninstall --yes` are available.
- Nodes report CPU, memory, latency and Xray state, can have a monthly traffic cap and cost, and take over for a node that goes down. Communication can use mutual TLS.

Details: [multi-node guide](docs/content/docs/en/operations/multi-node.mdx).

### 🔗 Tunnels

Eternal ships **WatTunnel** as a first-class feature. A tunnel is two ends: the **Kharej** server (outside the filter, where your services run) and the **Iran** server (where users connect). Each end is the panel itself, one of your nodes, or any server that does not run Eternal.

<table>
  <tr>
    <td width="50%"><img src="media/readme/wizard-ends-dark.png" alt="Pick the two ends"></td>
    <td width="50%"><img src="media/readme/tunnel-detail-dark.png" alt="A connected tunnel"></td>
  </tr>
  <tr>
    <td align="center"><sub>Pick the two ends: the panel, a node, or a server without a panel</sub></td>
    <td align="center"><sub>Live state of both ends, ports, log and speed test</sub></td>
  </tr>
</table>

- **Auto mode** tests every connection type (tcp, udp, tls, ws, wss, h2, grpc, icmp, …) and keeps the fastest one that works on your route.
- Add or remove forwarded ports **without dropping the tunnel**, with optional per-port rate and quota.
- A server without a panel joins with one line: `curl -fsSL https://PANEL/install-tunnel.sh | bash -s -- --token wat1:…`.
- Alerts when a tunnel goes down or comes back, through the same channels as node alerts.
- The anti-censorship kit (fake-SNI, TLS fingerprints, CDN, bonding) is behind an **Advanced** switch.

The tunnel is encrypted between its two ends only. Run an end-to-end encrypted protocol (VLESS + REALITY, TLS, WireGuard) inside it. Details: [tunnels guide](docs/content/docs/en/operations/tunnels.mdx).

### 🛒 Sell it

Eternal has a shop of its own, so you do not need a separate billing system.

- **Plans and orders** with a public purchase page that needs no panel login.
- **Payments:** card-to-card with admin approval, Zarinpal, IDPay, NOWPayments (crypto), Stripe and PayPal.
- **Wallet, discount codes, referral bonus, support tickets.**
- **Reseller panels:** separate, isolated panels with their own login, address and quotas.

**The Telegram shop bot** sells from inside Telegram:

| | |
| --- | --- |
| 🛍 **Buy and renew** | Plans grouped by category, a per-GB / per-day calculator, renewals and add-on orders that stack on an existing account. |
| 📦 **My services** | Usage bar, expiry, subscription link and QR code, with ownership checked on every tap. |
| 💳 **Card receipts** | The buyer sends a photo; admins approve or reject with one button, in the bot or in a receipts group. |
| 🌍 **Per-user language** | The bot talks to each customer in their own language (13 supported). |
| 🔒 **Safe by default** | Forced channel join, bans, rate limits, a cap on open orders, idempotent order handling. |
| 📱 **Mini App** | An "Open shop" button opens the public shop inside Telegram, signed in with verified `initData`. |
| 🔔 **Notifications** | Order confirmed, rejected, expired, unpaid reminder, ticket answered. |
| ✏️ **Your words** | Menu texts and link buttons are editable per language. |

---

## 📊 Comparison with 3x-ui

The first rows are shared with 3x-ui; the rest exist only in Eternal. The comparison is against the 3x-ui version this fork branched from and may be outdated if upstream has added features since.

| Feature | Eternal | 3x-ui |
| --- | :---: | :---: |
| VLESS, VMess, Trojan, Shadowsocks, REALITY, XTLS | ✅ | ✅ |
| Hysteria2, TUIC v5, WireGuard, AmneziaWG, MTProto | ✅ | ✅ |
| Per-client quota, expiry, IP and device limits | ✅ | ✅ |
| Multi-node management | ✅ | ✅ |
| Subscription server (raw, JSON, Clash) | ✅ | ✅ |
| Telegram and Discord bots | ✅ | ✅ |
| REST API with scoped tokens | ✅ | ✅ |
| Routing rules, WARP and outbound chaining | ✅ | ✅ |
| SQLite and PostgreSQL | ✅ | ✅ |
| Fail2ban IP limits | ✅ | ✅ |
| 13 languages, dark and light themes, PWA | ✅ | ✅ |
| Browser setup wizard on first install | ✅ | ❌ |
| OpenVPN inbounds | ✅ | ❌ |
| IKEv2/IPsec and L2TP/IPsec inbounds | ✅ | ❌ |
| SSTP (Windows built-in VPN) and OpenConnect/AnyConnect | ✅ | ❌ |
| SSH tunnel accounts with metered traffic | ✅ | ❌ |
| Built-in encrypted tunnels (WatTunnel) with live status | ✅ | ❌ |
| Built-in shop: plans, orders, public purchase page | ✅ | ❌ |
| Telegram shop bot with calculator, receipts and Mini App | ✅ | ❌ |
| Card-to-card, Zarinpal, IDPay, crypto (NOWPayments), Stripe and PayPal payments | ✅ | ❌ |
| Customer wallet, discount codes, referral bonus | ✅ | ❌ |
| Reseller panels: separate isolated panels with their own login, address and quotas | ✅ | ❌ |
| Roles and per-admin scoping (RBAC), admin sessions | ✅ | ❌ |
| Panel IP allow-list, login lockout, captcha, new-IP/country alerts | ✅ | ❌ |
| Telegram one-time-code login, signed subscription links | ✅ | ❌ |
| MySQL / MariaDB backend | ✅ | ❌ |
| Encrypted and scheduled backups to local folder or S3 | ✅ | ❌ |
| Prometheus metrics and Grafana dashboard | ✅ | ❌ |
| Cloudflare clean-IP scanner, port/SNI rotation, blocked-port alerts | ✅ | ❌ |
| One-command node install and automatic node failover | ✅ | ❌ |
| Node monthly cap, cost tracking and speed test | ✅ | ❌ |
| Client templates, daily usage chart, instant disconnect | ✅ | ❌ |
| Import from Marzban, Marzneshin and Hiddify | ✅ | ❌ |
| sing-box, Outline, Surge, Quantumult X and Loon subscriptions | ✅ | ❌ |
| Short subscription links, rate limiting, fetch statistics | ✅ | ❌ |
| Webhooks, ntfy, Gotify, Slack and Bale notifications | ✅ | ❌ |
| Editable bot texts and weekly summary report | ✅ | ❌ |
| Shared-account detection and maintenance mode | ✅ | ❌ |
| Panel health check, usage report export, keyboard shortcuts | ✅ | ❌ |
| Decoy site for unknown visitors, audit log, branding settings | ✅ | ❌ |
| `eternal client` command line | ✅ | ❌ |

---

## 🏗 How it fits together

```mermaid
flowchart LR
  U[Users] -->|VLESS, OpenVPN, IKEv2, ...| N1[Node A]
  U -->|...| N2[Node B]
  U -->|buy| S[Shop / Telegram bot]
  A[Admin browser] --> P
  S --> P
  subgraph P[Eternal panel]
    direction TB
    API[Panel + REST API] --- DB[(SQLite / PostgreSQL / MySQL)]
    API --- X[Xray-core and the VPN sidecars]
    API --- T[wattun tunnel ends]
  end
  P -- "mTLS / token" --> N1
  P -- "mTLS / token" --> N2
  T == "WatTunnel" ==> N1
```

Every state-changing action goes through one runtime layer that targets the local server or a node, so what works on the panel works on every node. The architecture and layering rules are in [`docs/architecture.md`](docs/architecture.md).

---

## ⚙️ Everyday commands

```bash
eternal                     # management menu
eternal setup-link          # a fresh setup link, before the wizard is finished
eternal setting -show true  # print the panel settings
eternal client list         # manage clients through the running panel's API
eternal migrate-db --dsn "postgres://user:pass@127.0.0.1:5432/db?sslmode=disable"
eternal update              # update the panel
```

### Build from source

Requirements: Go (see `go.mod`), a C compiler (the SQLite driver needs CGo), Node 26 for the frontend.

```bash
cd frontend && npm ci && npm run build && cd ..
go build -o eternal .
go build -o wattun ./cmd/wattun   # the tunnel daemon, optional
```

### Databases

SQLite (default), PostgreSQL and MySQL/MariaDB. Select with environment variables, which the installer writes to `/etc/default/eternal`:

```
XUI_DB_TYPE=postgres        # sqlite | postgres | mysql
XUI_DB_DSN=postgres://user:password@127.0.0.1:5432/db?sslmode=disable
```

`eternal migrate-db --dsn "<dsn>"` copies an existing SQLite database into PostgreSQL or MySQL.

---

## 💜 Sponsors

Sponsors are one JSON file in a GitHub repository that only the project owner
can edit; there is no sponsor setting, page or API in any panel. Every panel,
including the panels of hosted resellers, reads

```
https://raw.githubusercontent.com/rad1in/sponsors/main/sponsors.json
```

and the logos from `logos/` next to it. Panels cache the file for an hour
(10 minutes after a failed fetch, keeping the last good list), re-check each
entry's `from` and `until` on every request, and show no sponsors, without any
error, when the file is missing or unreachable.

<details>
<summary><b>The sponsors file format</b></summary>

To publish a sponsor, add an entry to that file and its logo to `logos/`. Every field is explained in the [sponsors reference](docs/content/docs/en/reference/sponsors.mdx):

```json
{
  "contact": "https://t.me/example",
  "sponsors": [
    {
      "id": "acme-2026-10",
      "name": "Acme VPS",
      "enable": true,
      "slots": ["dashboard", "page", "login"],
      "from": "2026-10-01T00:00:00Z",
      "until": "2026-11-01T00:00:00Z",
      "logo": "acme.png",
      "title": {
        "en": "Acme VPS: 20% off",
        "fa": "Acme VPS: ۲۰٪ تخفیف"
      },
      "text": {
        "en": "Fast servers for your panel. Use code ETERNAL at checkout.",
        "fa": "سرورهای سریع برای پنل شما. هنگام خرید از کد ETERNAL استفاده کنید."
      },
      "link": "https://acme.example/?utm_source=eternal"
    }
  ]
}
```

</details>

---

## 📚 Documentation

- Architecture and layering rules: [`docs/architecture.md`](docs/architecture.md)
- Multi-node and one-command install: [`docs/content/docs/en/operations/multi-node.mdx`](docs/content/docs/en/operations/multi-node.mdx)
- Tunnels: [`docs/content/docs/en/operations/tunnels.mdx`](docs/content/docs/en/operations/tunnels.mdx)
- Telegram bot: [`docs/content/docs/en/operations/telegram-bot.mdx`](docs/content/docs/en/operations/telegram-bot.mdx)
- Security hardening: [`docs/content/docs/en/operations/security.mdx`](docs/content/docs/en/operations/security.mdx)
- API reference: served inside the panel and generated into `docs/content/docs/en/reference/api/`
- Contributing guide: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Upstream documentation for the shared core: [docs.sanaei.dev](https://docs.sanaei.dev)

## 🌐 Languages

English · فارسی · العربية · 中文（简体） · 中文（繁體） · Español · Русский · Українська · Türkçe · Tiếng Việt · 日本語 · Bahasa Indonesia · Português (Brasil)

## 📜 License and credits

Eternal is free software under the **GNU GPL v3**. It is based on:

- [3x-ui](https://github.com/MHSanaei/3x-ui) by MHSanaei and contributors (GPL-3.0)
- [X-UI](https://github.com/vaxilu/x-ui) by vaxilu, the project 3x-ui was forked from (GPL-3.0)
- [Xray-core](https://github.com/XTLS/Xray-core) by the XTLS team (MPL-2.0)
- [WatTunnel](https://github.com/JohnWatsoon/WatTunnel) by Amirreza (MIT), vendored in `internal/wattunnel` and `cmd/wattun`; its license is in `third_party/wattunnel/LICENSE`

Routing rule sets used by the Iran and Russia templates come from [Iran v2ray rules](https://github.com/chocolate4u/Iran-v2ray-rules) and [Russia v2ray rules](https://github.com/runetfreedom/russia-v2ray-rules-dat) (GPL-3.0).
