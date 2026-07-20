# Pipeferry

Pipeferry is a small, protocol-independent bridge from a Unix domain socket in
WSL2 to a Windows named pipe. It uses WSL interoperability and standard streams,
without PowerShell, Socat, Python, Node.js, or a custom multiplexing protocol at
runtime.

```text
Linux application
  -> Unix domain socket
  -> pipeferry unix-listen
  -> child stdin/stdout
  -> pipeferry.exe npipe-connect
  -> Windows named pipe service
```

Each Unix connection gets its own Windows bridge process. Failures therefore stay
local to one connection, and Pipeferry never interprets or logs payload bytes.

The initial release targets Windows 11 x86-64 and Ubuntu x86-64 on WSL2.

## Install

To use the Windows OpenSSH Agent from WSL, follow the dedicated
[OpenSSH Agent installation manual](docs/openssh-agent.md). It provides a
separate one-line installer that installs Pipeferry, registers and starts the
systemd user service, and prepares the shell environment files.

For protocol-independent use, run the generic installer below.

Run this one-liner in WSL2:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/pipeferry/main/install.sh | sh
```

The installer verifies the Linux release archive with its published SHA-256
checksum and installs `pipeferry` to `~/.local/bin`. On WSL2 it then invokes the
Windows installer through PowerShell, verifies the Windows archive, installs
`pipeferry.exe` to `%LOCALAPPDATA%\Programs\pipeferry`, and records its WSL path
in `~/.config/pipeferry/windows-executable`. The Linux command uses this setting
to resolve `pipeferry.exe`; no Windows `PATH` change or WSL restart is required.

## Uninstall

Run this one-liner in WSL2:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/pipeferry/main/uninstall.sh | sh
```

The common uninstaller first stops and removes every Pipeferry-managed systemd
user service. It then removes integration environment files, both binaries, and
the recorded Windows executable setting. It also removes the Windows user
`PATH` entry created by Pipeferry versions before `v0.1.1`.

The child command is an argument array after `--`. It is not parsed by a shell.
A full pipe path is also accepted:

```bash
pipeferry unix-listen \
  --socket "$HOME/.local/run/pipeferry/example.sock" \
  -- \
  /mnt/c/Tools/pipeferry.exe npipe-connect \
    --pipe '\\.\pipe\example-service'
```

## Commands

### `unix-listen` (Linux)

```text
pipeferry unix-listen [options] -- executable [arguments...]
```

| Option | Default | Meaning |
|---|---|---|
| `--socket PATH` | XDG/private home path | Unix socket to create |
| `--socket-mode MODE` | `0600` | Octal socket permissions |
| `--shutdown-timeout DURATION` | `5s` | Active-connection shutdown limit |
| `--max-connections NUMBER` | `32` | Concurrent connection limit |
| `--log-level LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `--log-format FORMAT` | `text` | `text` or `json` |
| `--log-file PATH` | standard error | Append listener logs to a file |

Pipeferry creates a missing socket parent directory with mode `0700` and
requires an existing parent to already be owned by the current user with mode
`0700`. It never changes permissions on an existing parent. A lock prevents
duplicate listeners, and Pipeferry removes only a stale Unix socket owned by the
current user. It will not delete a regular file or directory at the socket path.
SIGINT and SIGTERM stop new accepts, cancel active children, wait up to the
shutdown timeout, and remove the socket and lock.

### `npipe-connect` (Windows)

```text
pipeferry.exe npipe-connect --pipe NAME [options]
```

| Option | Default | Meaning |
|---|---|---|
| `--pipe NAME` | required | Short name or full `\\.\pipe\...` path |
| `--connect-timeout DURATION` | `5s` | Named-pipe connection timeout |
| `--check` | false | Connect and close without transferring data |
| `--log-level LEVEL` | `info` | Validate the requested log threshold |

Standard output is exclusively the named-pipe payload. All diagnostics go to
standard error. EOF, a broken pipe, or cancellation closes the whole connection;
the initial release does not promise cross-platform half-close behavior.

### `status` (Linux)

```bash
pipeferry status --socket "$XDG_RUNTIME_DIR/pipeferry/example.sock"
pipeferry status --socket "$XDG_RUNTIME_DIR/pipeferry/example.sock" --json
```

The result distinguishes a missing path, non-socket path, live socket, and stale
socket. `locked` reports whether another listener holds the path lock.

### `doctor`

```bash
pipeferry doctor --json -- \
  pipeferry.exe npipe-connect --pipe example-service
```

Doctor checks the WSL kernel, interop handler, child executable, and the child's
`--check` connection. Each check runs independently; any failure produces exit
code 9.

### `version`

```bash
pipeferry version
pipeferry --version
```

Release builds report the version, commit, build timestamp, operating system,
and architecture.

### systemd user service (Linux/WSL)

Register a listener as a monitored, automatically restarted user service:

```bash
pipeferry service install \
  --name example \
  --socket-name example.sock \
  -- \
  pipeferry.exe npipe-connect \
    --pipe example-service \
    --connect-timeout 5s
```

The command enables and starts `pipeferry-example.service`, then prints the
socket and journal command. It does not modify shell startup files or configure
protocol-specific environment variables. Inspect or remove the service with:

```bash
pipeferry service status --name example
pipeferry service status --name example --json
pipeferry service uninstall --name example
```

Use `systemctl --user restart pipeferry-example.service` to restart it and
`journalctl --user --unit pipeferry-example.service --follow` to follow logs.
Re-running the same install is safe and restarts the service. A different
configuration requires `--force`.

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | success |
| 1 | internal error |
| 2 | invalid usage or unsupported command |
| 3 | child executable not found |
| 4 | Unix socket failure |
| 5 | named-pipe connection failure |
| 6 | transfer failure |
| 7 | listener already running or conflicting service configuration |
| 8 | connection or shutdown timeout |
| 9 | diagnostic failure |

Normal EOF, a closed connection, and a broken pipe begin normal per-connection
shutdown. A named-pipe failure never stops the Linux listener from accepting a
later connection.

## Security and limitations

- Put sockets below `XDG_RUNTIME_DIR` or another private directory.
- Pipeferry does not authenticate Unix clients or change named-pipe ACLs.
- Do not pass untrusted child commands or secrets in command arguments.
- Do not combine `npipe-connect` standard error with standard output.
- One Windows process is launched per Unix connection.
- The initial release does not provide `ensure`, `PIPEFERRY_EXEC`, system-level
  services, auto-update, TCP, or connection multiplexing.

See [troubleshooting](docs/troubleshooting.md) for WSL interop, PATH, named-pipe,
and stale-socket diagnostics. The normative initial requirements are in
[docs/requirements/260720-pipeferry-requirements.md](docs/requirements/260720-pipeferry-requirements.md).

## Development

Build, test, release, and contributor verification instructions are in
[docs/development.md](docs/development.md).

## License

[MIT](LICENSE)
