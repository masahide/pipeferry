#!/bin/sh
set -eu

REPOSITORY="${PIPEFERRY_REPOSITORY:-masahide/pipeferry}"
INSTALL_DIR="${PIPEFERRY_INSTALL_DIR:-$HOME/.local/bin}"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/pipeferry"
BINARY="$INSTALL_DIR/pipeferry"
SYSTEMD_USER_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"

services_changed=0
for unit_file in "$SYSTEMD_USER_DIR"/pipeferry-*.service; do
  if [ ! -f "$unit_file" ]; then
    continue
  fi
  if ! grep -q '^# Pipeferry-SocketName=' "$unit_file"; then
    echo "Skipping unmanaged unit: $unit_file" >&2
    continue
  fi

  unit="$(basename "$unit_file")"
  name="${unit#pipeferry-}"
  name="${name%.service}"
  removed=0

  if [ -x "$BINARY" ]; then
    if "$BINARY" service uninstall --name "$name"; then
      removed=1
    else
      echo "pipeferry: normal service removal failed; attempting fallback removal for $unit" >&2
    fi
  fi

  if [ "$removed" -eq 0 ]; then
    if command -v systemctl >/dev/null 2>&1; then
      systemctl --user disable --now "$unit" >/dev/null 2>&1 || true
    fi
    rm -f "$unit_file"
    echo "Removed systemd user unit: $unit_file"
  fi
  services_changed=1
done

if [ "$services_changed" -eq 1 ] && command -v systemctl >/dev/null 2>&1; then
  systemctl --user daemon-reload >/dev/null 2>&1 || true
  systemctl --user reset-failed >/dev/null 2>&1 || true
fi

for shell_config in "$CONFIG_DIR/ssh-agent.sh" "$CONFIG_DIR/ssh-agent.fish"; do
  if [ -f "$shell_config" ] || [ -L "$shell_config" ]; then
    rm -f "$shell_config"
    echo "Removed shell environment file: $shell_config"
  fi
done

if [ -e "$BINARY" ] || [ -L "$BINARY" ]; then
  rm -f "$BINARY"
  echo "Removed Linux binary: $BINARY"
else
  echo "Linux binary is not installed: $BINARY"
fi

if [ -f "$CONFIG_DIR/windows-executable" ]; then
  rm -f "$CONFIG_DIR/windows-executable"
  echo "Removed Windows binary config: $CONFIG_DIR/windows-executable"
fi
if [ -d "$CONFIG_DIR" ]; then
  rmdir "$CONFIG_DIR" 2>/dev/null || true
fi

if [ "${PIPEFERRY_SKIP_WINDOWS_UNINSTALL:-0}" = "1" ]; then
  exit 0
fi

if grep -qi microsoft /proc/sys/kernel/osrelease 2>/dev/null; then
  if ! command -v powershell.exe >/dev/null 2>&1; then
    echo "pipeferry: WSL was detected, but powershell.exe is unavailable" >&2
    exit 1
  fi

  PIPEFERRY_PS_URL="${PIPEFERRY_POWERSHELL_UNINSTALLER_URL:-https://raw.githubusercontent.com/$REPOSITORY/main/uninstall.ps1}"
  case "$PIPEFERRY_PS_URL" in
    *"'"*)
      echo "pipeferry: invalid PowerShell uninstaller URL" >&2
      exit 1
      ;;
  esac

  echo "Uninstalling pipeferry.exe from Windows..."
  powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -Command \
    "irm '$PIPEFERRY_PS_URL' -UseBasicParsing | iex"
fi
