#!/usr/bin/env bash
# Host layer for the ThinkPad demo box: the laptop is the server, the network
# and the screen. The stack itself is deploy/edge (install.sh), unchanged.
#
#   sudo deploy/edge/laptop/setup-host.sh [step...]
#
# Steps (default: all, in this order): packages apparmor tailscale battery
# bios logind journald uplinks demo-lan kiosk. Every step is idempotent, so re-running is safe.
#
# The ordering rule is the whole design: Tailscale is the break-glass path, so
# nothing here may leave the box without an uplink. demo-lan takes the
# Ethernet port away from uplink duty, and it refuses to run unless the Wi-Fi
# uplink already holds the default route.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
FILES="$HERE/files"
STACK_USER="${TRAKRF_USER:-mike}"        # owns the rootless stack
KIOSK_USER="${KIOSK_USER:-kiosk}"        # autologin screen user; no sudo
UPLINK_IF="${UPLINK_IF:-wlp3s0}"         # built-in card: client only
WIRED_IF="${WIRED_IF:-enp2s0f0}"         # demo LAN: CS463 via PoE switch
AP_DRIVER="${AP_DRIVER:-rtw88_8822bu}"   # USB dongle: access point
UPLINKS_FILE="${UPLINKS_FILE:-/etc/trakrf/uplinks.conf}"

DEMO_ADDR=192.168.8.10/24
DEMO_POOL=192.168.8.100,192.168.8.199
DEMO_SSID="${DEMO_SSID:-TrakRF}"         # same SSID/PSK as the Slate it replaces,
DEMO_PSK="${DEMO_PSK:-trakrf.id}"        # so devices re-join untouched
KIOSK_URL="${KIOSK_URL:-http://localhost:8080}"
DEMO_HOST="${DEMO_HOST:-app.demo-1.trakrf.id}"   # the EliteDesk keeps app.demo.trakrf.id

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run with sudo"
id "$STACK_USER" >/dev/null 2>&1 || die "stack user '$STACK_USER' does not exist"

# Copy a file only when it differs; print 1 if it changed.
install_file() { # src dest mode
  if cmp -s "$1" "$2" 2>/dev/null; then echo 0; return; fi
  install -D -m "$3" "$1" "$2"; echo 1
}

con_exists() { nmcli -t -f NAME con show | grep -qxF "$1"; }

step_packages() {
  log "packages"
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq curl git podman mosquitto-clients postgresql-client \
    rsync iw dnsmasq-base openssh-server apache2-utils >/dev/null
  loginctl enable-linger "$STACK_USER"
  echo 'net.ipv4.ip_unprivileged_port_start=443' > /etc/sysctl.d/99-trakrf-rootless-ports.conf
  sysctl -q --system
}

step_apparmor() {
  log "apparmor: let the containerized mosquitto read its config"
  [ -f /etc/apparmor.d/mosquitto ] || { log "  no host mosquitto profile; nothing to do"; return; }
  if [ "$(install_file "$FILES/apparmor-local-mosquitto" /etc/apparmor.d/local/mosquitto 644)" = 1 ]; then
    apparmor_parser -r /etc/apparmor.d/mosquitto
  fi
}

step_tailscale() {
  log "tailscale"
  command -v tailscale >/dev/null || curl -fsSL https://tailscale.com/install.sh | sh
  systemctl enable --now tailscaled
  # Login is interactive by design (a URL Mike approves); never automated here.
  if tailscale status >/dev/null 2>&1; then
    tailscale set --ssh
  else
    log "  not logged in: run 'sudo tailscale up --ssh --hostname=$(hostname)'"
    log "  then disable key expiry for this machine in the admin console"
  fi
  # OpenSSH stays on as the headless path: a Tailscale SSH 'check' policy
  # demands a browser re-auth, which no script can satisfy.
  # Ubuntu socket-activates sshd; enable whichever unit the release ships,
  # not both.
  if systemctl cat ssh.socket >/dev/null 2>&1; then
    systemctl enable --now ssh.socket 2>/dev/null
  else
    systemctl enable --now ssh.service 2>/dev/null
  fi
}

step_battery() {
  log "battery cap 75-80%"
  install_file "$FILES/trakrf-battery-cap.service" /etc/systemd/system/trakrf-battery-cap.service 644 >/dev/null
  systemctl daemon-reload
  systemctl enable trakrf-battery-cap.service >/dev/null
  systemctl restart trakrf-battery-cap.service
}

# Lenovo BIOS settings through think-lmi (no BIOS admin password set, so no
# auth needed). Applied at the next reboot.
#  OnByAcAttach: plugging in power boots the box, the laptop's version of
#    "restore on power loss" and the whole of Tim's start-up procedure.
#  AbsolutePersistenceModuleActivation=Disable: an off-lease unit may still
#    carry the previous owner's Computrace enrolment. Disable is reversible;
#    PermanentlyDisable is not, and is left as a deliberate manual choice.
step_bios() {
  log "bios (think-lmi)"
  local T=/sys/class/firmware-attributes/thinklmi/attributes k v
  [ -d "$T" ] || { log "  no think-lmi; set these in BIOS setup by hand"; return; }
  for kv in OnByAcAttach=Enable AbsolutePersistenceModuleActivation=Disable; do
    k="${kv%%=*}"; v="${kv#*=}"
    [ -d "$T/$k" ] || { log "  $k: not on this model"; continue; }
    if [ "$(cat "$T/$k/current_value")" != "$v" ]; then
      echo "$v" > "$T/$k/current_value"
      log "  $k -> $v (takes effect at next reboot)"
    fi
  done
}

step_logind() {
  log "logind: power key + lid"
  # Applied on next boot. Restarting logind live would end the graphical session.
  install_file "$FILES/logind-trakrf.conf" /etc/systemd/logind.conf.d/90-trakrf.conf 644 >/dev/null
}

step_journald() {
  log "journald cap"
  if [ "$(install_file "$FILES/journald-trakrf.conf" /etc/systemd/journald.conf.d/90-trakrf.conf 644)" = 1 ]; then
    systemctl restart systemd-journald
  fi
}

# uplinks.conf: one per line, 'name|ssid|psk|priority'. Root-only and never in
# git: it holds Tim's house and hotspot PSKs. Higher priority is tried first.
step_uplinks() {
  log "uplinks on $UPLINK_IF"
  [ -f "$UPLINKS_FILE" ] || { log "  $UPLINKS_FILE missing; nothing to save"; return; }
  chmod 600 "$UPLINKS_FILE"
  local name ssid psk prio con
  while IFS='|' read -r name ssid psk prio; do
    case "$name" in ''|'#'*) continue ;; esac
    con="uplink-$name"
    if con_exists "$con"; then
      nmcli con modify "$con" 802-11-wireless.ssid "$ssid" wifi-sec.psk "$psk" \
        connection.autoconnect-priority "$prio"
    else
      nmcli con add type wifi ifname "$UPLINK_IF" con-name "$con" ssid "$ssid" \
        wifi-sec.key-mgmt wpa-psk wifi-sec.psk "$psk" \
        connection.autoconnect yes connection.autoconnect-priority "$prio" \
        ipv4.route-metric 50 ipv6.route-metric 50 >/dev/null
    fi
    log "  $con ($ssid) priority $prio"
  done < "$UPLINKS_FILE"
}

ap_iface() {
  local d
  for d in /sys/class/net/*/device/driver; do
    [ "$(basename "$(readlink -f "$d")")" = "$AP_DRIVER" ] && basename "$(dirname "$(dirname "$d")")" && return
  done
  return 1
}

step_demo_lan() {
  log "demo LAN br-demo $DEMO_ADDR (SSID $DEMO_SSID)"
  ip -4 route show default dev "$UPLINK_IF" | grep -q . \
    || die "$UPLINK_IF holds no default route; bring a Wi-Fi uplink up first (this step removes Ethernet from uplink duty)"

  local tmp; tmp="$(mktemp)"
  sed "s|@DEMO_HOST@|$DEMO_HOST|" "$FILES/dnsmasq-shared-trakrf.conf" > "$tmp"
  install_file "$tmp" /etc/NetworkManager/dnsmasq-shared.d/trakrf.conf 644 >/dev/null
  rm -f "$tmp"
  [ -f /etc/NetworkManager/dnsmasq-shared.d/dhcp-hosts-trakrf.conf ] \
    || printf '# dhcp-host=<mac>,192.168.8.20,cs463\n' > /etc/NetworkManager/dnsmasq-shared.d/dhcp-hosts-trakrf.conf

  con_exists br-demo || nmcli con add type bridge ifname br-demo con-name br-demo >/dev/null
  nmcli con modify br-demo bridge.stp no \
    ipv4.method shared ipv4.addresses "$DEMO_ADDR" ipv4.shared-dhcp-range "$DEMO_POOL" \
    ipv6.method disabled connection.autoconnect yes \
    connection.autoconnect-slaves yes

  # Ethernet joins the demo LAN; the installer's profile must stop claiming it.
  local p
  for p in $(nmcli -t -f NAME,DEVICE,TYPE con show | awk -F: -v i="$WIRED_IF" '$3=="802-3-ethernet" && $1!="br-demo-wired" {print $1}'); do
    nmcli con modify "$p" connection.autoconnect no
  done
  con_exists br-demo-wired || nmcli con add type ethernet ifname "$WIRED_IF" con-name br-demo-wired \
    master br-demo slave-type bridge >/dev/null
  nmcli con modify br-demo-wired connection.autoconnect yes

  local ap
  if ap="$(ap_iface)"; then
    # Bound by MAC so a renamed or replugged dongle is still matched.
    local mac; mac="$(cat "/sys/class/net/$ap/address")"
    con_exists br-demo-ap || nmcli con add type wifi ifname '*' con-name br-demo-ap \
      ssid "$DEMO_SSID" master br-demo slave-type bridge >/dev/null
    nmcli con modify br-demo-ap 802-11-wireless.mode ap 802-11-wireless.band bg \
      802-11-wireless.channel 6 802-11-wireless.mac-address "$mac" \
      802-11-wireless.ssid "$DEMO_SSID" wifi-sec.key-mgmt wpa-psk wifi-sec.psk "$DEMO_PSK" \
      wifi-sec.proto rsn wifi-sec.pairwise ccmp wifi-sec.group ccmp \
      connection.autoconnect yes
  else
    log "  WARNING: no $AP_DRIVER dongle found; bridge built without the access point"
  fi

  nmcli con up br-demo >/dev/null
  nmcli con up br-demo-wired >/dev/null 2>&1 || log "  $WIRED_IF: no carrier (fine until the PoE switch is plugged in)"
  con_exists br-demo-ap && { nmcli con up br-demo-ap >/dev/null || log "  WARNING: AP failed to start"; }
  ip -4 route show default dev "$UPLINK_IF" | grep -q . || die "uplink default route lost after demo-lan"
}

step_kiosk() {
  log "kiosk ($KIOSK_USER -> $KIOSK_URL)"
  if ! command -v google-chrome >/dev/null; then
    install -d -m 755 /etc/apt/keyrings
    curl -fsSL https://dl.google.com/linux/linux_signing_key.pub | gpg --dearmor --yes -o /etc/apt/keyrings/google-chrome.gpg
    echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/google-chrome.gpg] https://dl.google.com/linux/chrome/deb/ stable main" \
      > /etc/apt/sources.list.d/google-chrome.list
    apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq google-chrome-stable >/dev/null
  fi
  id "$KIOSK_USER" >/dev/null 2>&1 || useradd -m -s /bin/bash -c "TrakRF demo screen" "$KIOSK_USER"
  passwd -d "$KIOSK_USER" >/dev/null   # passwordless console login; no sudo, no ssh
  usermod -aG bluetooth "$KIOSK_USER" 2>/dev/null || true

  install_file "$FILES/trakrf-kiosk" /usr/local/bin/trakrf-kiosk 755 >/dev/null
  local home; home="$(getent passwd "$KIOSK_USER" | cut -d: -f6)"
  install -d -o "$KIOSK_USER" -g "$KIOSK_USER" "$home/.config" "$home/.config/autostart"
  sed "s|@KIOSK_URL@|$KIOSK_URL|" "$FILES/trakrf-kiosk.desktop" > "$home/.config/autostart/trakrf-kiosk.desktop"
  chown "$KIOSK_USER:$KIOSK_USER" "$home/.config/autostart/trakrf-kiosk.desktop"

  local gdm=/etc/gdm3/custom.conf
  [ -f "$gdm" ] || die "$gdm missing (GDM not installed?)"
  sed -i -E '/^\s*#?\s*AutomaticLogin(Enable)?\s*=/d' "$gdm"
  sed -i "/^\[daemon\]/a AutomaticLoginEnable=true\nAutomaticLogin=$KIOSK_USER" "$gdm"
}

ALL=(packages apparmor tailscale battery bios logind journald uplinks demo-lan kiosk)
[ $# -gt 0 ] || set -- "${ALL[@]}"
for s in "$@"; do
  case "$s" in
    packages|apparmor|tailscale|battery|bios|logind|journald|uplinks|kiosk) "step_$s" ;;
    demo-lan) step_demo_lan ;;
    *) die "unknown step '$s' (known: ${ALL[*]})" ;;
  esac
done
log "done: $*"
