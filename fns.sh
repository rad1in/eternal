kv_value() {
    printf '%s\n' "$2" | sed -n "s/^$1=//p" | head -n1
}
prepare_setup_wizard() {
    local ip="$1" out port_arg=() code
    [[ -n "${XUI_PANEL_PORT:-}" ]] && port_arg=(-port "${XUI_PANEL_PORT}")
    out=$("${xui_folder}/eternal" setup-init "${port_arg[@]}" 2> /dev/null)
    code=$?
    [[ $code -eq 0 ]] || return 1
    SETUP_WIZARD_PORT=$(kv_value setup_port "$out")
    SETUP_WIZARD_TOKEN=$(kv_value setup_token "$out")
    SETUP_WIZARD_IP="$ip"
    [[ "$SETUP_WIZARD_PORT" =~ ^[0-9]+$ && -n "$SETUP_WIZARD_TOKEN" ]]
}
setup_wizard_hosts() {
    local seen=" " h
    for h in "$SETUP_WIZARD_IP" $(hostname -I 2> /dev/null); do
        [[ -n "$h" && "$h" != *:* && "$seen" != *" $h "* ]] || continue
        seen+="$h "
        echo "$h"
    done
}
print_setup_wizard_links() {
    local version="$1" bold='\033[1m' dim='\033[2m' cyan='\033[0;36m' h first=""
    local rule="──────────────────────────────────────────────────────────────"
    echo ""
    echo -e "${green}╭${rule}╮${plain}"
    local title="Eternal ${version} is installed"
    printf "${green}│${plain}  ${green}✔${plain} ${bold}%s${plain}%*s${green}│${plain}\n" "${title}" $((62 - 4 - ${#title})) ""
    echo -e "${green}╰${rule}╯${plain}"
    echo ""
    echo -e "  ${bold}Finish the setup in your browser${plain} — open the link that reaches"
    echo -e "  this server from where you are:"
    echo ""
    local label
    while read -r h; do
        [[ -n "$h" ]] || continue
        if [[ -z "$first" && "$h" == "$SETUP_WIZARD_IP" ]]; then
            label="as the internet sees this host"
        else
            label="private address"
        fi
        [[ -z "$first" ]] && first="$h"
        echo -e "    ${cyan}➜${plain}  ${blue}http://${h}:${SETUP_WIZARD_PORT}/setup?t=${SETUP_WIZARD_TOKEN}${plain}"
        echo -e "       ${dim}${label}${plain}"
    done < <(setup_wizard_hosts)
    echo ""
    echo -e "  ${yellow}!${plain} Port ${bold}${SETUP_WIZARD_PORT}${plain} was picked at random and is free on this host."
    echo -e "    Open it in the firewall if one is running. The wizard can move it"
    echo -e "    before you finish."
    echo -e "  ${yellow}!${plain} The link works once: it stops working when the wizard is done."
    echo -e "    Lost it? Run ${bold}eternal setup-link${plain} for a new one."
    echo ""
    echo -e "  ${dim}Service:${plain}  systemctl status eternal"
    echo -e "  ${dim}Logs:${plain}     journalctl -u eternal -f"
    echo ""
}
