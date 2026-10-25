#!/usr/bin/env bash
# Eternal node installer, served by the master panel at <panel>/install-node.sh.
#   curl -fsSL <panel>/install-node.sh | bash -s -- --panel <panel> --token <token>
# Installs Eternal on this server without prompts, then registers it on the master
# so it shows up in Nodes. Run as root on the NEW server.

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
bold='\033[1m'
dim='\033[2m'
plain='\033[0m'

DEFAULT_VERSION="@@VERSION@@"
REPO="rad1in/eternal"
FOLDER="${XUI_FOLDER:-/usr/local/eternal}"

panel=""
token=""
node_name=""
node_port=""
version=""
installer_url=""
pin=""
insecure=0
node_tls="verify"
mode="install"
assume_yes=0

ok() { echo -e "  ${green}✔${plain} $*"; }
note() { echo -e "  ${yellow}!${plain} $*"; }
fail() {
    echo -e "  ${red}✘ $*${plain}" >&2
    exit 1
}

usage() {
    cat << 'EOF'
Usage: install-node.sh --panel URL --token TOKEN [options]

  --panel URL          the master panel address, with its secret path (as the panel shows it)
  --token TOKEN        the one-time token from Nodes > Add node
  --name NAME          the node's name in the panel (default: this host's name)
  --port PORT          the node panel's port (default: random)
  --version TAG        Eternal release to install (default: the master's version)
  --installer-url URL  use this install.sh instead of the one on GitHub
  --pin sha256//KEY    accept the master's TLS key (for a self-signed master)
  --insecure           do not check the master's TLS certificate
  --node-tls MODE      how the master verifies this node: verify, skip or pin (default: verify)
  --detect             print what is on this server and change nothing
  --uninstall          remove Eternal from this server (asks nothing; add --yes to confirm)
  --yes                confirm --uninstall
  -h, --help           this text
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --panel) panel="${2:-}"; shift 2 ;;
        --token) token="${2:-}"; shift 2 ;;
        --name) node_name="${2:-}"; shift 2 ;;
        --port) node_port="${2:-}"; shift 2 ;;
        --version) version="${2:-}"; shift 2 ;;
        --installer-url) installer_url="${2:-}"; shift 2 ;;
        --pin) pin="${2:-}"; shift 2 ;;
        --insecure) insecure=1; shift ;;
        --node-tls) node_tls="${2:-}"; shift 2 ;;
        --detect) mode="detect"; shift ;;
        --uninstall) mode="uninstall"; shift ;;
        --yes) assume_yes=1; shift ;;
        -h | --help) usage; exit 0 ;;
        *) usage >&2; fail "Unknown option: $1" ;;
    esac
done

detect_arch() {
    case "$(uname -m)" in
        x86_64 | x64 | amd64) echo amd64 ;;
        i*86 | x86) echo 386 ;;
        aarch64 | arm64) echo arm64 ;;
        armv7* | armv7 | arm) echo armv7 ;;
        armv6*) echo armv6 ;;
        armv5*) echo armv5 ;;
        s390x) echo s390x ;;
        *) echo "unknown" ;;
    esac
}

installed_version() {
    [[ -x "${FOLDER}/eternal" ]] || return 1
    "${FOLDER}/eternal" -v 2> /dev/null | head -n1
}

run_detect() {
    echo -e "${bold}This server${plain}"
    local os="unknown"
    [[ -r /etc/os-release ]] && os=$(. /etc/os-release && echo "${PRETTY_NAME:-$ID}")
    echo "  system        ${os}"
    echo "  architecture  $(uname -m) ($(detect_arch))"
    echo "  root          $([[ $EUID -eq 0 ]] && echo yes || echo no)"
    echo "  systemd       $(command -v systemctl > /dev/null 2>&1 && echo yes || echo no)"
    echo "  curl          $(command -v curl > /dev/null 2>&1 && echo yes || echo no)"
    local v
    if v=$(installed_version); then
        echo "  eternal       installed (${v})"
    else
        echo "  eternal       not installed"
    fi
    if [[ -r /etc/eternal/install-result.env ]]; then
        echo "  install result  /etc/eternal/install-result.env"
    fi
}

run_uninstall() {
    [[ $EUID -eq 0 ]] || fail "Run this as root."
    installed_version > /dev/null || fail "Eternal is not installed on this server."
    [[ $assume_yes -eq 1 ]] || fail "This removes Eternal, its database and Xray from this server. Run again with --yes to confirm."
    command -v eternal > /dev/null 2>&1 || fail "The eternal command is missing; remove ${FOLDER} and /etc/eternal by hand."
    printf 'y\n' | eternal uninstall
    ok "Eternal removed. Delete the node in the master panel too."
}

if [[ "$mode" == "detect" ]]; then
    run_detect
    exit 0
fi
if [[ "$mode" == "uninstall" ]]; then
    run_uninstall
    exit 0
fi

[[ -n "$panel" ]] || fail "--panel is required. Copy the whole command from the master panel (Nodes > Add node)."
[[ -n "$token" ]] || fail "--token is required. Copy the whole command from the master panel (Nodes > Add node)."
[[ "$panel" =~ ^https?://[^/]+ ]] || fail "--panel must start with http:// or https:// (got '${panel}')."
panel="${panel%/}"
[[ "$node_tls" == "verify" || "$node_tls" == "skip" || "$node_tls" == "pin" ]] || fail "--node-tls must be verify, skip or pin."
[[ -z "$node_port" || "$node_port" =~ ^[0-9]+$ ]] || fail "--port must be a number."
# XUI_FOLDER, XUI_RESULT_FILE, XUI_SKIP_INSTALL and XUI_ALLOW_NONROOT exist for the tests.
[[ $EUID -eq 0 || "${XUI_ALLOW_NONROOT:-0}" == "1" ]] || fail "Run this as root (use sudo bash)."
command -v curl > /dev/null 2>&1 || fail "curl is required."

[[ -n "$node_name" ]] || node_name="$(hostname)"
[[ -n "$version" ]] || version="$DEFAULT_VERSION"
[[ "$version" == v* || "$version" == "dev" || -z "$version" ]] || version="v${version}"
join_url="${panel}/join/node"
result_file="${XUI_RESULT_FILE:-/etc/eternal/install-result.env}"

# Talking to the master: -k only with --insecure or --pin, and --pin still refuses any other key.
master_curl=(-sS -m 40)
if [[ -n "$pin" ]]; then
    master_curl+=(-k --pinnedpubkey "$pin")
elif [[ $insecure -eq 1 ]]; then
    master_curl+=(-k)
fi

json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

echo -e "${bold}Eternal node installer${plain}  ${dim}→ ${panel}${plain}"
echo ""

reply=$(curl "${master_curl[@]}" -o /dev/null -w '%{http_code}' "${panel}/install-node.sh" 2>&1) || true
[[ "$reply" == "200" ]] || fail "Cannot reach the master at ${panel} (answer: ${reply:-none}). Check the address and that this server can open it."
ok "master reachable"

if [[ "${XUI_SKIP_INSTALL:-0}" == "1" ]]; then
    :
elif installed_version > /dev/null; then
    [[ -r "$result_file" ]] || fail "Eternal is already installed here but ${result_file} is missing. Use --uninstall --yes first, or add this node by hand."
    note "Eternal is already installed ($(installed_version)); only registering it on the master."
else
    arch=$(detect_arch)
    [[ "$arch" != "unknown" ]] || fail "Unsupported CPU architecture: $(uname -m)."
    if [[ -z "$installer_url" ]]; then
        tag="$version"
        if [[ -n "$tag" && "$tag" != "dev" ]] && ! curl -fsSI -o /dev/null "https://github.com/${REPO}/releases/tag/${tag}"; then
            note "Release ${tag} is not published; installing the latest release."
            tag=""
        fi
        if [[ -n "$tag" ]]; then
            installer_url="https://raw.githubusercontent.com/${REPO}/${tag}/install.sh"
        else
            installer_url="https://raw.githubusercontent.com/${REPO}/main/install.sh"
        fi
    fi
    installer="$(mktemp)" || fail "Cannot create a temporary file."
    trap 'rm -f "$installer"' EXIT
    curl -fsSL --retry 3 "$installer_url" -o "$installer" || fail "Could not download the installer from ${installer_url}."
    ok "installer downloaded"
    echo -e "  ${dim}installing Eternal, this takes a minute...${plain}"
    export XUI_NONINTERACTIVE=1
    [[ -n "$node_port" ]] && export XUI_PANEL_PORT="$node_port"
    if [[ -n "${tag:-}" ]]; then
        bash "$installer" "$tag" < /dev/null || fail "The installer failed; the server was not registered."
    else
        bash "$installer" < /dev/null || fail "The installer failed; the server was not registered."
    fi
    ok "Eternal installed"
fi

[[ -r "$result_file" ]] || fail "No install result at ${result_file}; cannot tell the master how to reach this node."
# shellcheck disable=SC1090
. "$result_file"
[[ -n "${XUI_API_TOKEN:-}" ]] || fail "The install result has no API token."

re='^(https?)://(\[[^]]+\]|[^:/]+):([0-9]+)/(.*)$'
[[ "${XUI_ACCESS_URL:-}" =~ $re ]] || fail "Cannot read the panel address from '${XUI_ACCESS_URL:-}'."
scheme="${BASH_REMATCH[1]}"
host="${BASH_REMATCH[2]}"
port="${BASH_REMATCH[3]}"
base="/${BASH_REMATCH[4]#/}"
[[ "$base" == */ ]] || base="${base}/"

# An address the installer could not detect is left for the master to fill in
# from the connection this request arrives on.
address=""
if [[ "$host" != SERVER_IP_UNKNOWN ]]; then
    address="${host#[}"
    address="${address%]}"
fi

payload=$(printf '{"token":"%s","name":"%s","scheme":"%s","address":"%s","port":%s,"basePath":"%s","apiToken":"%s","tlsVerifyMode":"%s"}' \
    "$(json_escape "$token")" "$(json_escape "$node_name")" "$scheme" "$(json_escape "$address")" \
    "$port" "$(json_escape "$base")" "$(json_escape "$XUI_API_TOKEN")" "$(json_escape "$node_tls")")

attempts="${XUI_JOIN_ATTEMPTS:-12}"
for ((i = 1; i <= attempts; i++)); do
    reply=$(curl "${master_curl[@]}" -H 'Content-Type: application/json' -d "$payload" "$join_url" 2>&1)
    if [[ "$reply" == *'"success":true'* ]]; then
        ok "${node_name} is now a node of ${panel}"
        echo ""
        echo -e "  Open ${bold}Nodes${plain} in the panel: it is listed there. Give it an inbound to start serving users."
        exit 0
    fi
    note "attempt ${i}/${attempts}: ${reply}"
    # A bad token never gets better; an unreachable node may once its panel is up.
    [[ "$reply" == *"invalid or expired"* ]] && fail "The token was refused. Issue a new command on the master (Nodes > Add node)."
    sleep 5
done
fail "The master could not add this node. Check that the master can reach ${scheme}://${address:-this server}:${port}${base} (open the port in the firewall) and run the command again."
