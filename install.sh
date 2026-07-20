#!/bin/sh
set -eu

REPOSITORY="${PIPEFERRY_REPOSITORY:-masahide/pipeferry}"
INSTALL_DIR="${PIPEFERRY_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${PIPEFERRY_VERSION:-latest}"

case "$VERSION" in
  *[!A-Za-z0-9._-]*)
    echo "pipeferry: invalid PIPEFERRY_VERSION: $VERSION" >&2
    exit 1
    ;;
esac

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64|Linux-amd64)
    asset="pipeferry-linux-amd64.tar.gz"
    ;;
  *)
    echo "pipeferry: unsupported platform: $(uname -s)/$(uname -m)" >&2
    exit 1
    ;;
esac

if ! command -v curl >/dev/null 2>&1; then
  echo "pipeferry: curl is required" >&2
  exit 1
fi

if [ "$VERSION" = "latest" ]; then
  release_base="https://github.com/$REPOSITORY/releases/latest/download"
else
  release_base="https://github.com/$REPOSITORY/releases/download/$VERSION"
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

echo "Installing pipeferry for Linux..."
curl --fail --location --proto '=https' --tlsv1.2 \
  --output "$tmp_dir/$asset" "$release_base/$asset"
curl --fail --location --proto '=https' --tlsv1.2 \
  --output "$tmp_dir/$asset.sha256" "$release_base/$asset.sha256"

(
  cd "$tmp_dir"
  sha256sum --check "$asset.sha256"
)

tar -xzf "$tmp_dir/$asset" -C "$tmp_dir"
binary="$(find "$tmp_dir" -type f -name pipeferry | head -n 1)"
if [ -z "$binary" ]; then
  echo "pipeferry: Linux binary was not found in the release archive" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
install -m 0755 "$binary" "$INSTALL_DIR/pipeferry"
echo "Installed Linux binary: $INSTALL_DIR/pipeferry"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo "Add $INSTALL_DIR to PATH if it is not already configured." >&2
    ;;
esac

if [ "${PIPEFERRY_SKIP_WINDOWS_INSTALL:-0}" = "1" ]; then
  exit 0
fi

if grep -qi microsoft /proc/sys/kernel/osrelease 2>/dev/null; then
  if ! command -v powershell.exe >/dev/null 2>&1; then
    echo "pipeferry: WSL was detected, but powershell.exe is unavailable" >&2
    exit 1
  fi

  echo "Installing pipeferry.exe on Windows..."
  PIPEFERRY_PS_URL="${PIPEFERRY_POWERSHELL_INSTALLER_URL:-https://raw.githubusercontent.com/$REPOSITORY/main/install.ps1}"
  case "$PIPEFERRY_PS_URL" in
    *"'"*)
      echo "pipeferry: invalid PowerShell installer URL" >&2
      exit 1
      ;;
  esac
  powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -Command \
    "\$env:PIPEFERRY_VERSION='$VERSION'; irm '$PIPEFERRY_PS_URL' -UseBasicParsing | iex"
fi
