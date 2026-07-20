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

## Build

Pipeferry requires Go 1.25 or newer.

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -o pipeferry ./cmd/pipeferry

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -o pipeferry.exe ./cmd/pipeferry
```

Tagged releases produce archives and SHA-256 checksum files for both targets.

## Use Windows OpenSSH Agent from WSL

Make both binaries available from WSL, then start the listener:

```bash
pipeferry unix-listen \
  --socket "${XDG_RUNTIME_DIR:-$HOME/.local/run}/pipeferry/ssh-agent.sock" \
  -- \
  pipeferry.exe npipe-connect \
    --pipe openssh-ssh-agent \
    --connect-timeout 5s
```

In another shell:

```bash
export SSH_AUTH_SOCK="${XDG_RUNTIME_DIR:-$HOME/.local/run}/pipeferry/ssh-agent.sock"
ssh-add -l
ssh-add -L
```

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

Pipeferry creates the socket's parent directory with mode `0700`, uses a lock to
prevent duplicate listeners, and removes only a stale Unix socket owned by the
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
pipeferry status --socket "$SSH_AUTH_SOCK"
pipeferry status --socket "$SSH_AUTH_SOCK" --json
```

The result distinguishes a missing path, non-socket path, live socket, and stale
socket. `locked` reports whether another listener holds the path lock.

### `doctor`

```bash
pipeferry doctor --json -- \
  pipeferry.exe npipe-connect --pipe openssh-ssh-agent
```

Doctor checks the WSL kernel, interop handler, child executable, and the child's
`--check` connection. Each check runs independently; any failure produces exit
code 9. Add `--ssh-agent` to make a separate SSH Agent identities request through
the bridge.

### `version`

```bash
pipeferry version
pipeferry --version
```

Release builds report the version, commit, build timestamp, operating system,
and architecture.

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
| 7 | listener already running |
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
- The initial release does not provide `ensure`, `PIPEFERRY_EXEC`, services,
  installers, auto-update, TCP, or connection multiplexing.

See [troubleshooting](docs/troubleshooting.md) for WSL interop, PATH, named-pipe,
and stale-socket diagnostics. The normative initial requirements are in
[docs/requirements/260720-pipeferry-requirements.md](docs/requirements/260720-pipeferry-requirements.md).

## Development and verification

```bash
go test ./...
go vet ./...
go test -race ./...
```

Windows tests include a real named-pipe echo integration test. Linux tests cover
Unix socket modes, locking, stale recovery, and regular-file safety. Full
Windows-to-WSL validation, OpenSSH Agent signing, load, and process-leak checks
must be completed on a Windows 11 plus WSL2 machine using
[docs/e2e-checklist.md](docs/e2e-checklist.md).

## License

[MIT](LICENSE)
