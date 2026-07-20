# Pipeferry Initial Release Requirements

## Scope

Pipeferry bridges one Unix domain socket connection in WSL2 to one Windows named
pipe connection. The Linux process launches one child process for every accepted
connection; the Windows process forwards standard input and output to a named
pipe. Payload bytes are opaque and are never logged.

The supported targets are Windows 11 x86-64 and Ubuntu x86-64 on WSL2. The Linux
binary must build with `CGO_ENABLED=0`. PowerShell, Socat, Python, and Node.js are
not runtime dependencies.

## CLI contract

```text
pipeferry unix-listen [options] -- executable [arguments...]
pipeferry.exe npipe-connect --pipe NAME [options]
pipeferry status [--socket PATH] [--json]
pipeferry doctor [--json] [--ssh-agent] -- executable [arguments...]
pipeferry version
```

The child command is an argv array after `--`; it is never evaluated by a shell.
`ensure`, `--exec`, and `PIPEFERRY_EXEC` are outside the initial release scope.

`unix-listen` defaults to a private socket below `$XDG_RUNTIME_DIR/pipeferry`, or
`$HOME/.local/run/pipeferry` when XDG_RUNTIME_DIR is unavailable. A missing
parent is created with mode `0700`; an existing parent must already be owned by
the current user with mode `0700` and is never chmodded. The socket defaults to
`0600`. A lock file prevents two listeners from managing the same path. Only a
stale Unix socket owned by the current user may be removed; regular files and
directories are never removed.

`npipe-connect` accepts a short name such as `openssh-ssh-agent` or a full path
such as `\\.\pipe\openssh-ssh-agent`. It waits up to five seconds by default.
`--check` connects and closes without transferring payload.

`doctor --ssh-agent` performs an SSH Agent identities request only when the
option is explicitly provided; the default doctor path remains protocol
independent.

## Stream and shutdown contract

Standard output of `npipe-connect` is reserved exclusively for bytes read from
the named pipe. Diagnostics go to standard error.

EOF or a broken pipe in either direction starts shutdown of the whole
connection. Normal EOF, closed connection, and broken pipe conditions are not
reported as internal failures. Context cancellation closes both streams.
`unix-listen` stops accepting on SIGINT or SIGTERM, cancels active children,
waits for the configured shutdown timeout, and then reports timeout if cleanup
did not finish. Socket and lock cleanup is idempotent.

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | success |
| 1 | internal error |
| 2 | usage or unsupported command |
| 3 | child executable not found |
| 4 | Unix socket failure |
| 5 | named pipe connection failure |
| 6 | stream transfer failure |
| 7 | another listener is running |
| 8 | connection or shutdown timeout |
| 9 | one or more doctor checks failed |

## Acceptance criteria

- Arbitrary binary bytes traverse both directions unchanged.
- Windows OpenSSH Agent requests work through `SSH_AUTH_SOCK`.
- A named pipe connection failure affects only its Unix client.
- SIGTERM reaps children and removes the socket and lock.
- 32 concurrent and 100 sequential connections do not cause continued resource
  growth.
- No diagnostics enter the Windows-side standard-output payload.
- The distributed executables require no helper runtime listed above.

The OpenSSH Agent, full WSL boundary, load, resource, and signal criteria require
validation on a Windows 11 plus WSL2 test machine before release.
