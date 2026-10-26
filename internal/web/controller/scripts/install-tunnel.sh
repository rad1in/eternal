#!/bin/bash
# Sets up one end of an Eternal tunnel (WatTunnel, MIT) on a server that has no panel.
#
#   Iran end:    curl -fsSL PANEL/install-tunnel.sh | bash -s -- --token wat1:... --map 443:443,53/udp
#   Kharej end:  curl -fsSL PANEL/install-tunnel.sh | bash -s -- --role kharej --addr 203.0.113.7 --ports 443
#
# It installs the wattun binary, writes the config under /etc/wattun, starts a systemd
# service and waits for the link. A Kharej end prints a TOKEN= line to paste into the panel.

set -u

PANEL_URL="__PANEL_URL__"
RELEASE_REPO="rad1in/eternal"

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'
say() { printf '%b\n' "$*"; }
fail() { say "${red}$*${plain}" >&2; exit 1; }

role="" token="" map="" name="" ports="" addr="" carrier="tcp" listen="" version="" binary="" no_service=0
conf_dir="${WATTUN_CONF:-/etc/wattun}"
bin_path="${WATTUN_BIN:-/usr/local/bin/wattun}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --role) role="${2:-}"; shift 2 ;;
        --token) token="${2:-}"; shift 2 ;;
        --map) map="${2:-}"; shift 2 ;;
        --name) name="${2:-}"; shift 2 ;;
        --ports) ports="${2:-}"; shift 2 ;;
        --addr) addr="${2:-}"; shift 2 ;;
        --carrier) carrier="${2:-}"; shift 2 ;;
        --listen) listen="${2:-}"; shift 2 ;;
        --version) version="${2:-}"; shift 2 ;;
        --binary) binary="${2:-}"; shift 2 ;;
        --dir) conf_dir="${2:-}"; shift 2 ;;
        --no-service) no_service=1; shift ;;
        -h | --help) sed -n '2,10p' "$0" 2>/dev/null; exit 0 ;;
        *) fail "Unknown option: $1" ;;
    esac
done

[[ -n "$role" ]] || { [[ -n "$token" ]] && role=iran || role=kharej; }
case "$role" in
    iran) [[ "$token" == wat1:* ]] || fail "--token wat1:... is required for the Iran end."; [[ -n "$map" ]] || fail "--map listen:target[,...] is required (the ports users connect to)." ;;
    kharej) [[ -n "$addr" ]] || fail "--addr (this server's public IP or domain) is required for the Kharej end." ;;
    *) fail "--role must be iran or kharej." ;;
esac
[[ -n "$name" ]] || name="$role"
name=$(printf '%s' "$name" | tr -c 'A-Za-z0-9_-' '-' | cut -c1-32)
[[ -n "$name" ]] || name="$role"

[[ $EUID -eq 0 || $no_service -eq 1 ]] || fail "Please run this script as root."
command -v curl >/dev/null 2>&1 || fail "curl is required."

go_arch() {
    case "$(uname -m)" in
        x86_64 | x64 | amd64) echo amd64 ;;
        i*86 | x86) echo 386 ;;
        armv8* | aarch64 | arm64) echo arm64 ;;
        armv7* | armv7 | arm) echo arm ;;
        armv6* | armv5*) echo arm ;;
        s390x) echo s390x ;;
        *) fail "Unsupported CPU architecture: $(uname -m)" ;;
    esac
}
release_arch() {
    case "$(uname -m)" in
        armv7* | arm) echo armv7 ;;
        armv6*) echo armv6 ;;
        armv5*) echo armv5 ;;
        *) go_arch ;;
    esac
}

verify_sha() { # file url
    local sums expected actual
    sums=$(mktemp) || return 1
    if ! curl -fsSL --retry 3 --connect-timeout 15 -o "$sums" "$2.sha256" 2>/dev/null; then rm -f "$sums"; return 1; fi
    expected=$(awk '{print $1; exit}' "$sums"); rm -f "$sums"
    actual=$(sha256sum "$1" | awk '{print $1}')
    [[ -n "$expected" && "$expected" == "$actual" ]]
}

fetch_binary() { # dest
    local dest="$1" tmp tag url arch rarch
    arch=$(go_arch); rarch=$(release_arch)
    if [[ "$PANEL_URL" == http* ]]; then
        say "Downloading wattun from the panel"
        if curl -fsSL --retry 2 --connect-timeout 10 -k -o "$dest" "$PANEL_URL/install-tunnel/wattun/$arch" 2>/dev/null && [[ -s "$dest" ]]; then return 0; fi
    fi
    tag="$version"
    if [[ -z "$tag" ]]; then
        tag=$(curl -sSLI -o /dev/null -w '%{url_effective}' --connect-timeout 15 "https://github.com/$RELEASE_REPO/releases/latest" 2>/dev/null | sed -n 's#.*/releases/tag/##p')
    fi
    [[ -n "$tag" ]] || fail "Could not find the latest release, and the panel does not carry a wattun for this CPU. Use --binary PATH."
    url="https://github.com/$RELEASE_REPO/releases/download/$tag/wattun-linux-$rarch"
    say "Downloading wattun $tag from GitHub"
    if curl -fsSL --retry 3 --connect-timeout 15 -o "$dest" "$url" 2>/dev/null && verify_sha "$dest" "$url"; then return 0; fi
    url="https://github.com/$RELEASE_REPO/releases/download/$tag/eternal-linux-$rarch.tar.gz"
    tmp=$(mktemp -d) || return 1
    if curl -fsSL --retry 3 --connect-timeout 15 -o "$tmp/e.tar.gz" "$url" && verify_sha "$tmp/e.tar.gz" "$url" \
        && tar -xzf "$tmp/e.tar.gz" -C "$tmp" "eternal/bin/wattun-linux-$rarch" 2>/dev/null; then
        mv "$tmp/eternal/bin/wattun-linux-$rarch" "$dest"; rm -rf "$tmp"; return 0
    fi
    rm -rf "$tmp"
    return 1
}

# Opens what the setup printed (carrier ports) and, on the Iran end, the published ports.
open_firewall() {
    local item port proto line items=()
    while IFS= read -r line; do [[ -n "$line" ]] && items+=("$line"); done < <(printf '%s\n' "$out" | sed -n 's/^LISTEN=//p')
    if [[ "$role" == iran ]]; then
        for item in $(printf '%s' "$map" | tr ',' ' '); do
            port="${item%%[:/]*}"; proto=tcp
            [[ "$item" == */udp ]] && proto=udp
            items+=("$port/$proto")
        done
    fi
    for item in "${items[@]}"; do
        port="${item%/*}"; proto="${item#*/}"
        [[ "$port" =~ ^[0-9]+$ ]] || continue
        if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q 'Status: active'; then
            ufw allow "$port/$proto" >/dev/null 2>&1
        elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
            firewall-cmd -q --permanent --add-port="$port/$proto" 2>/dev/null && firewall-cmd -q --reload
        fi
    done
}

mkdir -p "$(dirname "$bin_path")" "$conf_dir" || fail "Cannot create $conf_dir."
chmod 700 "$conf_dir"

if [[ -n "$binary" ]]; then
    [[ -x "$binary" ]] || fail "$binary is not an executable file."
    bin_path="$binary"
else
    tmp_bin=$(mktemp "$(dirname "$bin_path")/.wattun.XXXXXX") || fail "Cannot write to $(dirname "$bin_path")."
    fetch_binary "$tmp_bin" || { rm -f "$tmp_bin"; fail "Could not download wattun."; }
    chmod 755 "$tmp_bin"
    "$tmp_bin" -version >/dev/null 2>&1 || { rm -f "$tmp_bin"; fail "The downloaded wattun does not run on this server."; }
    mv -f "$tmp_bin" "$bin_path"
fi
say "wattun: $("$bin_path" -version 2>/dev/null)"

if [[ "$role" == iran ]]; then
    out=$("$bin_path" setup -dir "$conf_dir" -name "$name" -token "$token" -map "$map") || fail "Setup failed."
else
    args=(setup -dir "$conf_dir" -name "$name" -role kharej -carrier "$carrier" -addr "$addr" -ports "$ports")
    [[ -n "$listen" ]] && args+=(-listen "$listen")
    out=$("$bin_path" "${args[@]}") || fail "Setup failed."
fi
config=$(printf '%s\n' "$out" | sed -n 's/^CONFIG=//p')
token_out=$(printf '%s\n' "$out" | sed -n 's/^TOKEN=//p')
[[ -f "$config" ]] || fail "Setup did not write a config."
metrics=$(sed -n 's/^metrics: *127\.0\.0\.1:\([0-9]*\).*/\1/p' "$config")

if [[ $no_service -eq 1 ]]; then
    say "${green}Config written: $config${plain}"
    say "Run it with: $bin_path -config $config"
else
    command -v systemctl >/dev/null 2>&1 || fail "systemd is required. Run it by hand with: $bin_path -config $config"
    unit="wattun-$name"
    cat >"/etc/systemd/system/$unit.service" <<UNIT
[Unit]
Description=Eternal tunnel $name (WatTunnel)
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
Type=simple
ExecStart=$bin_path -config $config
Restart=always
RestartSec=2
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
UNIT
    systemctl daemon-reload
    systemctl enable "$unit" >/dev/null 2>&1
    systemctl restart "$unit" || fail "The service did not start: journalctl -u $unit"
    open_firewall
    say "Waiting for the link (up to 25 s)"
    ok=0
    for _ in $(seq 1 25); do
        if curl -fs --max-time 2 "http://127.0.0.1:$metrics/stats" 2>/dev/null | grep -q '"alive":true'; then ok=1; break; fi
        systemctl is-active -q "$unit" || break
        sleep 1
    done
    if [[ $ok -eq 1 ]]; then
        say "${green}Connected.${plain}"
    elif [[ "$role" == kharej ]]; then
        say "${yellow}Started. The link comes up once the Iran end connects.${plain}"
    else
        say "${yellow}Started, but not connected yet. Check the Kharej firewall and run: journalctl -u $unit -n 30${plain}"
    fi
fi

if [[ "$role" == kharej ]]; then
    say ""
    say "Paste this token into the panel (Tunnels > Add > this server is the external Kharej end):"
    say "${green}${token_out}${plain}"
    printf 'TOKEN=%s\n' "$token_out"
    say "It holds the tunnel key; treat it like a password."
fi
