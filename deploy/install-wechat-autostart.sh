#!/usr/bin/env bash
set -euo pipefail

if [ "${EUID}" -ne 0 ]; then
  printf 'Run as root: sudo %s <desktop-user>\n' "$0" >&2
  exit 1
fi

target_user=${1:-hominal}
user_record=$(getent passwd "$target_user")
if [ -z "$user_record" ]; then
  printf 'Unknown desktop user: %s\n' "$target_user" >&2
  exit 1
fi

user_home=$(printf '%s\n' "$user_record" | cut -d: -f6)
user_group=$(id -gn "$target_user")
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

for command_name in wechat wmctrl xdotool flock desktop-file-validate; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "$command_name" >&2
    exit 1
  fi
done

if ! mountpoint -q /agent; then
  printf '/agent is not mounted\n' >&2
  exit 1
fi

install -d -m 0755 -o "$target_user" -g "$user_group" \
  "$user_home/.local/bin" "$user_home/.config/autostart"
install -d -m 0750 -o "$target_user" -g "$user_group" /agent/state/logs

install -m 0755 -o "$target_user" -g "$user_group" \
  "$script_dir/desktop/wechat-autologin" \
  "$user_home/.local/bin/wechat-autologin"

desktop_tmp=$(mktemp)
lightdm_tmp=$(mktemp)
trap 'unlink "$desktop_tmp"; unlink "$lightdm_tmp"' EXIT

sed "s|@HOME@|$user_home|g" \
  "$script_dir/desktop/wechat-autostart.desktop.in" >"$desktop_tmp"
install -m 0644 -o "$target_user" -g "$user_group" \
  "$desktop_tmp" "$user_home/.config/autostart/wechat.desktop"

sed "s|@USER@|$target_user|g" \
  "$script_dir/desktop/lightdm-autologin.conf.in" >"$lightdm_tmp"
install -d -m 0755 /etc/lightdm/lightdm.conf.d
install -m 0644 "$lightdm_tmp" \
  /etc/lightdm/lightdm.conf.d/50-hominal-autologin.conf

bash -n "$user_home/.local/bin/wechat-autologin"
desktop-file-validate "$user_home/.config/autostart/wechat.desktop"

printf 'Installed persistent WeChat startup for %s\n' "$target_user"
printf 'Phone confirmation remains part of the official WeChat login flow.\n'
