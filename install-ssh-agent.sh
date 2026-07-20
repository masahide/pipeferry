#!/bin/sh
set -eu

REPOSITORY="${PIPEFERRY_REPOSITORY:-masahide/pipeferry}"
INSTALL_DIR="${PIPEFERRY_INSTALL_DIR:-$HOME/.local/bin}"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/pipeferry"
VERSION="${PIPEFERRY_VERSION:-latest}"
GENERIC_INSTALLER_URL="${PIPEFERRY_GENERIC_INSTALLER_URL:-https://raw.githubusercontent.com/$REPOSITORY/main/install.sh}"
DISPLAY_CONFIG_DIR="$CONFIG_DIR"
if [ -z "${XDG_CONFIG_HOME:-}" ]; then
  DISPLAY_CONFIG_DIR="~/.config/pipeferry"
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "pipeferry: curl is required" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
shell_tmp=""
fish_tmp=""
cleanup() {
  rm -rf "$tmp_dir"
  if [ -n "$shell_tmp" ]; then
    rm -f "$shell_tmp"
  fi
  if [ -n "$fish_tmp" ]; then
    rm -f "$fish_tmp"
  fi
}
trap cleanup EXIT HUP INT TERM

echo "Installing Pipeferry..."
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
  --output "$tmp_dir/install.sh" "$GENERIC_INSTALLER_URL"

PIPEFERRY_REPOSITORY="$REPOSITORY" \
PIPEFERRY_INSTALL_DIR="$INSTALL_DIR" \
PIPEFERRY_VERSION="$VERSION" \
  sh "$tmp_dir/install.sh"

PIPEFERRY="$INSTALL_DIR/pipeferry"
if [ ! -x "$PIPEFERRY" ]; then
  echo "pipeferry: installed Linux binary is not executable: $PIPEFERRY" >&2
  exit 1
fi

echo "Installing Pipeferry OpenSSH Agent service..."
"$PIPEFERRY" service install \
  --name ssh-agent \
  --socket-name ssh-agent.sock \
  -- \
  pipeferry.exe npipe-connect \
    --pipe openssh-ssh-agent \
    --connect-timeout 5s

install -d -m 0700 "$CONFIG_DIR"
umask 077

shell_tmp="$(mktemp "$CONFIG_DIR/.ssh-agent.sh.XXXXXX")"
cat > "$shell_tmp" <<'EOF'
# Source this file from Bash or Zsh.
export SSH_AUTH_SOCK="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/pipeferry/ssh-agent.sock"
EOF

fish_tmp="$(mktemp "$CONFIG_DIR/.ssh-agent.fish.XXXXXX")"
cat > "$fish_tmp" <<'EOF'
# Source this file from Fish.
if set -q XDG_RUNTIME_DIR
    set -gx SSH_AUTH_SOCK "$XDG_RUNTIME_DIR/pipeferry/ssh-agent.sock"
else
    set -gx SSH_AUTH_SOCK "/run/user/"(id -u)"/pipeferry/ssh-agent.sock"
end
EOF

chmod 0600 "$shell_tmp" "$fish_tmp"
mv "$shell_tmp" "$CONFIG_DIR/ssh-agent.sh"
shell_tmp=""
mv "$fish_tmp" "$CONFIG_DIR/ssh-agent.fish"
fish_tmp=""

cat <<EOF

Pipeferry SSH Agent service is installed and running.

For Bash or Zsh, add this line to your shell configuration:

  source $DISPLAY_CONFIG_DIR/ssh-agent.sh

For Fish:

  source $DISPLAY_CONFIG_DIR/ssh-agent.fish
EOF
