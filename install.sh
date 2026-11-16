#!/bin/sh
set -eu

VERSION="${REBOOT_AGENT_VERSION:-v0.1.4}"
REPO="${REBOOT_AGENT_REPO:-w243420707/reboot-agent}"
INSTALL_DIR="${REBOOT_AGENT_INSTALL_DIR:-/usr/local/bin}"
AGENT_BIN="$INSTALL_DIR/reboot-agent"
CONFIG_DIR="${REBOOT_AGENT_CONFIG_DIR:-/etc/reboot-agent}"
CONFIG_FILE="$CONFIG_DIR/config"
TMP_DIR="${TMPDIR:-/tmp}/reboot-agent-install.$$"

# 默认配置：hours=0 表示不做定时重启，只看内存占用。
DEFAULT_HOURS=0
DEFAULT_MEMORY=80
CONFIG_MODE=default

cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup 0 1 2 15

fail() {
    printf 'reboot-agent installer: %s\n' "$*" >&2
    exit 1
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

download() {
    url=$1
    output=$2
    if command_exists curl; then
        curl -fsSL --retry 3 "$url" -o "$output"
    elif command_exists wget; then
        wget -q -O "$output" "$url"
    else
        fail "需要 curl 或 wget 才能下载文件。"
    fi
}

write_file() {
    target=$1
    mode=$2
    temp="${target}.tmp.$$"
    umask 022
    cat >"$temp"
    chmod "$mode" "$temp"
    mv "$temp" "$target"
}

write_descriptor() {
    type=$1
    path=$2
    mkdir -p "$CONFIG_DIR"
    write_file "$CONFIG_DIR/startup" 0600 <<EOF
type=$type
path=$path
EOF
}

prompt_choice() {
    if [ ! -r /dev/tty ]; then
        printf '未检测到交互终端，使用默认配置。\n'
        CONFIG_MODE=default
        return 0
    fi
    printf '\n请选择配置方式：\n'
    printf '  1) 默认配置：不设定时重启，内存占用超过 %s%% 自动重启（直接回车）\n' "$DEFAULT_MEMORY"
    printf '  2) 手动配置：自定义定时重启间隔与内存阈值\n'
    while :; do
        printf '请选择 [1]: '
        answer=
        IFS= read -r answer </dev/tty || answer=
        case "$(printf '%s' "$answer" | tr -d '[:space:]')" in
            ''|1)
                CONFIG_MODE=default
                return 0
                ;;
            2)
                CONFIG_MODE=manual
                return 0
                ;;
            *)
                printf '请输入 1 或 2。\n'
                ;;
        esac
    done
}

write_default_config() {
    mkdir -p "$CONFIG_DIR"
    write_file "$CONFIG_FILE" 0600 <<EOF
hours=$DEFAULT_HOURS
memory_threshold=$DEFAULT_MEMORY
enabled=true
EOF
    if ! REBOOT_AGENT_CONFIG_DIR="$CONFIG_DIR" "$AGENT_BIN" status >/dev/null 2>&1; then
        rm -f "$CONFIG_FILE"
        fail "当前版本不支持默认配置，请升级到最新版本，或重新运行并选择 2 手动配置。"
    fi
    printf '已启用默认配置：不设定时重启，内存超过 %s%% 自动重启。\n' "$DEFAULT_MEMORY"
}

install_systemd() {
    service_path="/etc/systemd/system/reboot-agent.service"
    write_file "$service_path" 0644 <<EOF
[Unit]
Description=reboot-agent automatic VPS reboot agent
After=local-fs.target

[Service]
Type=simple
ExecStart=$AGENT_BIN daemon
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable reboot-agent.service
    systemctl restart reboot-agent.service 2>/dev/null || systemctl start reboot-agent.service
    write_descriptor systemd "$service_path"
}

install_openwrt() {
    service_path="/etc/init.d/reboot-agent"
    write_file "$service_path" 0755 <<EOF
#!/bin/sh /etc/rc.common
START=99
USE_PROCD=1
PROG="$AGENT_BIN"

start_service() {
    procd_open_instance
    procd_set_param command "\$PROG" daemon
    procd_set_param respawn 3600 5 5
    procd_close_instance
}
EOF
    "$service_path" enable
    "$service_path" restart 2>/dev/null || "$service_path" start
    write_descriptor openwrt "$service_path"
}

install_openrc() {
    service_path="/etc/init.d/reboot-agent"
    write_file "$service_path" 0755 <<EOF
#!/sbin/openrc-run
name="reboot-agent"

depend() {
    need localmount
    after bootmisc
}

start() {
    ebegin "Starting reboot-agent"
    "$AGENT_BIN" start
    eend \$?
}

stop() {
    ebegin "Stopping reboot-agent"
    "$AGENT_BIN" shutdown
    eend \$?
}

status() {
    "$AGENT_BIN" status
}
EOF
    rc-update add reboot-agent default
    rc-service reboot-agent restart 2>/dev/null || rc-service reboot-agent start
    write_descriptor openrc "$service_path"
}

install_sysv() {
    service_path="/etc/init.d/reboot-agent"
    write_file "$service_path" 0755 <<EOF
#!/bin/sh
### BEGIN INIT INFO
# Provides:          reboot-agent
# Required-Start:    \$remote_fs
# Required-Stop:     \$remote_fs
# Should-Start:      \$network
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: reboot-agent automatic VPS reboot agent
### END INIT INFO

case "\${1:-start}" in
start) "$AGENT_BIN" start ;;
stop) "$AGENT_BIN" shutdown ;;
restart) "$AGENT_BIN" shutdown || true; "$AGENT_BIN" start ;;
status) "$AGENT_BIN" status ;;
*) echo "Usage: \$0 {start|stop|restart|status}" >&2; exit 2 ;;
esac
EOF
    if command_exists update-rc.d; then
        update-rc.d reboot-agent defaults
    elif command_exists chkconfig; then
        chkconfig --add reboot-agent
    fi
    "$service_path" start
    write_descriptor sysv "$service_path"
}

install_rc_local() {
    service_path=/etc/rc.local
    if [ ! -e "$service_path" ]; then
        write_file "$service_path" 0755 <<'EOF'
#!/bin/sh
exit 0
EOF
    fi
    if ! grep -q '# reboot-agent begin' "$service_path" 2>/dev/null; then
        temp="${service_path}.tmp.$$"
        if grep -q '^[[:space:]]*exit[[:space:]][[:space:]]*0[[:space:]]*$' "$service_path"; then
            awk -v agent="$AGENT_BIN" '
                /^[[:space:]]*exit[[:space:]][[:space:]]*0[[:space:]]*$/ {
                    print "# reboot-agent begin"
                    print agent " start"
                    print "# reboot-agent end"
                }
                { print }
            ' "$service_path" >"$temp"
        else
            cat "$service_path" >"$temp"
            printf '\n# reboot-agent begin\n%s start\n# reboot-agent end\n' "$AGENT_BIN" >>"$temp"
        fi
        chmod 0755 "$temp"
        mv "$temp" "$service_path"
    fi
    "$AGENT_BIN" start
    write_descriptor rc-local "$service_path"
}

install_inittab() {
    service_path=/etc/inittab
    line="ra:2345:respawn:$AGENT_BIN daemon"
    if ! grep -F -q "$line" "$service_path" 2>/dev/null; then
        printf '%s\n' "$line" >>"$service_path"
    fi
    "$AGENT_BIN" start
    write_descriptor inittab "$service_path"
}

detect_and_install_startup() {
    if command_exists systemctl && [ -d /run/systemd/system ]; then
        install_systemd
        return
    fi
    if [ -f /etc/rc.common ] && [ -d /etc/init.d ]; then
        install_openwrt
        return
    fi
    if command_exists rc-update && command_exists rc-service && [ -d /etc/init.d ]; then
        install_openrc
        return
    fi
    if [ -x /etc/init.d/rcS ] || command_exists update-rc.d || command_exists chkconfig; then
        install_sysv
        return
    fi
    if [ -f /etc/inittab ] && command_exists init; then
        install_inittab
        return
    fi
    install_rc_local
}

sha256() {
    file=$1
    if command_exists sha256sum; then
        sha256sum "$file" | awk '{print $1}'
    elif command_exists shasum; then
        shasum -a 256 "$file" | awk '{print $1}'
    else
        fail "需要 sha256sum 或 shasum 才能校验下载文件。"
    fi
}

detect_arch() {
    machine=$(uname -m)
    case "$machine" in
        x86_64|amd64) printf 'amd64\n' ;;
        aarch64|arm64) printf 'arm64\n' ;;
        armv7l|armv7) printf 'armv7\n' ;;
        i386|i686) printf '386\n' ;;
        mips) printf 'mips\n' ;;
        mipsel|mipsle) printf 'mipsle\n' ;;
        riscv64) printf 'riscv64\n' ;;
        *) fail "暂不支持 CPU 架构：$machine" ;;
    esac
}

require_root() {
    if [ "$(id -u)" -ne 0 ]; then
        fail "请使用 root 运行安装脚本。"
    fi
}

require_root
ARCH=$(detect_arch)
ASSET="reboot-agent-linux-$ARCH"
BASE_URL="https://github.com/$REPO/releases/download/$VERSION"
mkdir -p "$TMP_DIR" "$INSTALL_DIR"

printf '下载 reboot-agent %s（linux/%s）...\n' "$VERSION" "$ARCH"
download "$BASE_URL/$ASSET" "$TMP_DIR/$ASSET"
download "$BASE_URL/$ASSET.sha256" "$TMP_DIR/$ASSET.sha256"

EXPECTED=$(awk 'NF { print $1; exit }' "$TMP_DIR/$ASSET.sha256")
ACTUAL=$(sha256 "$TMP_DIR/$ASSET")
[ "$EXPECTED" = "$ACTUAL" ] || fail "下载文件校验失败。"

chmod 0755 "$TMP_DIR/$ASSET"
had_config=0
if [ -f /etc/reboot-agent/config ]; then
    had_config=1
fi
cp "$TMP_DIR/$ASSET" "$AGENT_BIN"
chmod 0755 "$AGENT_BIN"

if [ "$had_config" -eq 1 ]; then
    printf '检测到现有配置，已保留原来的定时和内存阈值。\n'
    printf '如需修改，请运行：%s configure\n' "$AGENT_BIN"
else
    prompt_choice
    if [ "$CONFIG_MODE" = "manual" ]; then
        printf '手动配置：\n'
        "$AGENT_BIN" configure </dev/tty
    else
        write_default_config
    fi
fi
detect_and_install_startup
REBOOT_AGENT_CONFIG_DIR="$CONFIG_DIR" "$AGENT_BIN" status || true
printf '开机自启已配置，reboot-agent 正在运行。\n'
