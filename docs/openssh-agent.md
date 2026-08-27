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

First, open Windows PowerShell and install the Windows binary:

```powershell
irm https://raw.githubusercontent.com/masahide/pipeferry/main/install.ps1 | iex
```

Then run this one-liner in WSL:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/pipeferry/main/install-ssh-agent.sh | sh
```

The OpenSSH Agent installer performs these steps:

1. Runs the generic Pipeferry installer for the Linux binary.
2. Locates the preinstalled Windows binary and records its absolute WSL path.
3. Registers and starts `pipeferry-ssh-agent.service` as a systemd user service.
4. Writes the following shell environment files:
   - `~/.config/pipeferry/ssh-agent.sh` for Bash and Zsh
   - `~/.config/pipeferry/ssh-agent.fish` for Fish

It does not edit shell startup files automatically.

By default, it looks for the Windows binary under
`/mnt/*/Users/*/AppData/Local/Programs/pipeferry/pipeferry.exe`. For a custom
Windows install path, set `PIPEFERRY_WINDOWS_EXECUTABLE` to its absolute WSL
path.

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

Run the WSL uninstaller first. It stops and removes Pipeferry-managed systemd
user services, removes the OpenSSH Agent shell environment files, and removes
the Linux binary and its recorded Windows executable setting:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/pipeferry/main/uninstall.sh | sh
```

The WSL uninstaller does not remove the Windows binary. To remove that binary,
run this one-liner in Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/masahide/pipeferry/main/uninstall.ps1 | iex
```
