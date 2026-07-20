# Use the Windows OpenSSH Agent from WSL

This integration is optional. Pipeferry itself is a protocol-independent bridge;
the installer on this page adds the OpenSSH Agent-specific service and shell
environment files on top of the generic Pipeferry installation.

## Requirements

- WSL2 on Windows 11 x86-64
- systemd enabled in WSL
- a Windows service providing the `openssh-ssh-agent` named pipe
- `curl`, `tar`, and `sha256sum` in WSL

If PID 1 is not systemd, add the following to `/etc/wsl.conf`:

```ini
[boot]
systemd=true
```

Then run `wsl --shutdown` from Windows and start the distribution again.

## Install

Run this one-liner in WSL:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/pipeferry/main/install-ssh-agent.sh | sh
```

The OpenSSH Agent installer performs these steps:

1. Runs the generic Pipeferry installer for the Linux and Windows binaries.
2. Registers and starts `pipeferry-ssh-agent.service` as a systemd user service.
3. Writes the following shell environment files:
   - `~/.config/pipeferry/ssh-agent.sh` for Bash and Zsh
   - `~/.config/pipeferry/ssh-agent.fish` for Fish

It does not edit shell startup files automatically.

For Bash, add this line to `~/.bashrc`; for Zsh, add it to `~/.zshrc`:

```bash
source ~/.config/pipeferry/ssh-agent.sh
```

For Fish, add this line to `~/.config/fish/config.fish`:

```fish
source ~/.config/pipeferry/ssh-agent.fish
```

Open a new shell or source the file, then verify the connection:

```bash
ssh-add -l
ssh-add -L
```

For a complete Pipeferry diagnostic, including an SSH Agent identities request:

```bash
pipeferry doctor --json --ssh-agent -- \
  pipeferry.exe npipe-connect --pipe openssh-ssh-agent
```

## Service management

```bash
pipeferry service status --name ssh-agent
systemctl --user restart pipeferry-ssh-agent.service
journalctl --user --unit pipeferry-ssh-agent.service --follow
```

## Uninstall

Pipeferry uses one common uninstaller. It stops and removes Pipeferry-managed
systemd user services, removes the OpenSSH Agent shell environment files, and
then removes both binaries:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/pipeferry/main/uninstall.sh | sh
```
