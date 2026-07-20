# Troubleshooting

Run `pipeferry doctor --json -- pipeferry.exe npipe-connect --pipe
openssh-ssh-agent` first. Every check is independent, so later checks still run
after an earlier failure.

## WSL interoperability

`doctor` expects a Microsoft WSL kernel and
`/proc/sys/fs/binfmt_misc/WSLInterop`. If the handler is missing, confirm that
interop is enabled in `/etc/wsl.conf`, then restart the distribution with
`wsl.exe --shutdown` from Windows.

## Windows executable not found

The WSL installer records the Windows binary path in
`~/.config/pipeferry/windows-executable`; Windows `PATH` registration is not
required. Check that the file contains an absolute WSL path and that the target
exists:

```bash
cat "${XDG_CONFIG_HOME:-$HOME/.config}/pipeferry/windows-executable"
test -f "$(cat "${XDG_CONFIG_HOME:-$HOME/.config}/pipeferry/windows-executable")"
```

Re-run the installer to repair a missing or stale setting. An absolute
`/mnt/c/.../pipeferry.exe` path may still be passed after `--` as a manual
override.

## Named pipe unavailable

From WSL, run:

```bash
pipeferry.exe npipe-connect --pipe openssh-ssh-agent --check
```

Exit code 5 means the service or pipe is unavailable; exit code 8 means the
connection timed out. Confirm that the Windows service is running and that the
pipe name is correct. Pipeferry does not create or modify named-pipe ACLs.

## Socket path conflicts

Use `pipeferry status --socket PATH --json`. A running listener reports
`running: true`; a dead socket reports `stale: true`. Pipeferry automatically
removes only a stale Unix socket owned by the current user. It deliberately
refuses to delete a regular file, directory, or another user's socket.

If a lock remains after an unclean exit, starting Pipeferry safely reuses it once
the kernel lock is no longer held.

## Logs and payloads

Linux listener logs go to standard error unless `--log-file` is used. Windows
bridge diagnostics always go to standard error. Do not redirect Windows standard
error into standard output, because standard output is the payload stream.
