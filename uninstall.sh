#!/bin/sh
set -eu

REPOSITORY="${PIPEFERRY_REPOSITORY:-masahide/pipeferry}"
INSTALL_DIR="${PIPEFERRY_INSTALL_DIR:-$HOME/.local/bin}"
BINARY="$INSTALL_DIR/pipeferry"

if [ -e "$BINARY" ] || [ -L "$BINARY" ]; then
  rm -f "$BINARY"
  echo "Removed Linux binary: $BINARY"
else
  echo "Linux binary is not installed: $BINARY"
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
