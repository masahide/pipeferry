# Pipeferry

Pipeferry is a lightweight cross-platform stream bridge for connecting Unix domain sockets to Windows named pipes through standard input and output.

Its primary use case is exposing a Windows named-pipe service, such as an SSH agent, to applications running inside WSL. Pipeferry is intentionally protocol-agnostic and can also be used as a general-purpose byte-stream bridge.

> Pipeferry is currently in the design and initial implementation phase. Command names and behavior described in this document may change before the first stable release.

## Overview

Windows applications commonly expose local services through named pipes, while Linux applications use Unix domain sockets.

WSL can launch Windows executables and communicate with them through standard input and output. Pipeferry uses this interoperability model to connect the two environments without requiring PowerShell, `socat`, or an SSH-specific proxy protocol.

```text
Linux application
        │
        │ Unix domain socket
        ▼
pipeferry unix-listen
        │
        │ standard input and output
        ▼
pipeferry.exe npipe-connect
        │
        │ Windows named pipe
        ▼
Windows service
```

For every accepted Unix socket connection, the Linux-side process starts a Windows-side Pipeferry process. The two processes forward the byte stream without parsing, modifying, or framing the application protocol.

## Goals

Pipeferry is designed around the following principles:

* Remain independent of SSH and OmniSSHAgent
* Forward arbitrary binary streams without protocol awareness
* Use a simple one-connection-per-process model
* Avoid custom multiplexing protocols in the initial implementation
* Avoid PowerShell at runtime
* Avoid external runtime dependencies
* Provide predictable shutdown and error handling
* Keep logs separate from forwarded data
* Support both interactive use and background operation
* Be easy to test with ordinary streams and mock services

## Non-goals

The initial version will not provide:

* Encryption or authentication between the Linux and Windows processes
* Network transport between different computers
* Application-level protocol parsing
* Connection multiplexing over a single Windows process
* Automatic SSH key management
* A graphical user interface
* Windows service management
* General TCP forwarding

These features may be considered later where they fit the project scope.

## Planned commands

Pipeferry will use a single executable with platform-specific subcommands.

### Linux

```text
pipeferry unix-listen
```

This command will:

* Create and listen on a Unix domain socket
* Accept multiple client connections
* Start one configured child process per connection
* Connect the socket to the child process standard input and output
* Remove stale socket files safely
* Shut down child processes when their corresponding connection closes
* Handle termination signals cleanly

### Windows

```text
pipeferry.exe npipe-connect
```

This command will:

* Connect to a Windows named pipe
* Forward standard input to the named pipe
* Forward named-pipe responses to standard output
* Write diagnostics only to standard error
* Exit when either side closes or an unrecoverable error occurs

## Proposed usage

The following example exposes the Windows OpenSSH agent inside WSL.

### Start the Unix socket listener

```bash
pipeferry unix-listen \
  --socket "${XDG_RUNTIME_DIR:-$HOME/.local/run}/pipeferry/ssh-agent.sock" \
  --exec "pipeferry.exe npipe-connect --pipe '\\.\pipe\openssh-ssh-agent'"
```

### Configure SSH clients

```bash
export SSH_AUTH_SOCK="${XDG_RUNTIME_DIR:-$HOME/.local/run}/pipeferry/ssh-agent.sock"
```

Applications using `SSH_AUTH_SOCK` can then communicate with the Windows SSH agent through Pipeferry.

The same approach can be used with another Windows named-pipe service:

```bash
pipeferry unix-listen \
  --socket "$HOME/.local/run/example/service.sock" \
  --exec "pipeferry.exe npipe-connect --pipe '\\.\pipe\example-service'"
```

## Proposed command-line interface

### `unix-listen`

```text
pipeferry unix-listen [options]
```

Planned options:

```text
--socket PATH
    Unix domain socket path to create.

--exec COMMAND
    Command started for each accepted connection.

--socket-mode MODE
    File permissions applied to the Unix socket.

--connect-timeout DURATION
    Maximum time allowed for the child process to establish its upstream
    connection.

--shutdown-timeout DURATION
    Maximum graceful shutdown period before a child process is terminated.

--max-connections NUMBER
    Maximum number of concurrent accepted connections.

--log-level LEVEL
    Logging level such as error, warn, info, or debug.
```

### `npipe-connect`

```text
pipeferry.exe npipe-connect [options]
```

Planned options:

```text
--pipe NAME
    Windows named-pipe path or name.

--connect-timeout DURATION
    Maximum time to wait for the named pipe to become available.

--log-level LEVEL
    Logging level such as error, warn, info, or debug.
```

## Stream behavior

Pipeferry treats all transferred data as an opaque byte stream.

It must not:

* Interpret SSH agent messages
* Add message boundaries
* Rewrite line endings
* Perform character encoding conversion
* Buffer an entire request before forwarding it
* Write logs or status messages to standard output

For the Windows-side process, standard output is reserved exclusively for data received from the named pipe.

All diagnostics must be written to standard error.

## Connection lifecycle

The initial implementation uses a one-to-one connection model.

```text
Unix connection 1
    └── Windows bridge process 1
        └── Named-pipe connection 1

Unix connection 2
    └── Windows bridge process 2
        └── Named-pipe connection 2
```

This design avoids the complexity of assigning channel identifiers, framing messages, synchronizing multiple writers, and recovering multiplexed sessions after a bridge process failure.

A persistent multiplexed bridge may be considered later if process startup overhead becomes a measurable problem.

## Error handling

Pipeferry should fail explicitly and avoid silently discarding data.

Expected behavior includes:

* Rejecting invalid command-line arguments before listening
* Reporting missing Windows executables clearly
* Reporting named-pipe connection failures through standard error
* Removing incomplete Unix socket state after startup failure
* Closing both forwarding directions when one side fails
* Reaping child processes after every connection
* Avoiding goroutine and process leaks
* Returning non-zero exit codes for startup and runtime failures
* Distinguishing normal client disconnects from internal errors where possible

## Security considerations

Pipeferry does not authenticate clients by itself.

Users are responsible for selecting safe socket locations and permissions.

Recommended practices:

* Place Unix sockets under `XDG_RUNTIME_DIR` where available
* Use a private fallback directory with mode `0700`
* Create the Unix socket with mode `0600`
* Do not listen on TCP or externally reachable interfaces
* Do not expose privileged Windows named pipes to untrusted WSL users
* Avoid passing secrets directly in command-line arguments
* Do not log forwarded payloads
* Validate stale socket ownership before removing a socket file
* Use absolute paths where command resolution could be influenced by untrusted input

The child command should normally be configured by the local user or administrator rather than supplied by an untrusted remote client.

## WSL interoperability

Pipeferry relies on WSL interoperability to start the Windows executable from Linux.

For example:

```bash
pipeferry.exe --version
```

The Windows executable must be available through the WSL `PATH`, or its path must be specified explicitly.

An explicit command may look like this:

```bash
pipeferry unix-listen \
  --socket "$HOME/.local/run/pipeferry/service.sock" \
  --exec "/mnt/c/Users/example/AppData/Local/Programs/Pipeferry/pipeferry.exe npipe-connect --pipe '\\.\pipe\example-service'"
```

Environments that disable WSL interoperability cannot use the standard Pipeferry WSL bridge model.

## Project structure

The planned repository structure is:

```text
pipeferry
├── cmd
│   └── pipeferry
│       └── main.go
├── internal
│   ├── command
│   ├── execbridge
│   ├── namedpipe
│   ├── streamcopy
│   └── unixsocket
├── docs
├── scripts
│   ├── install.ps1
│   └── uninstall.ps1
├── go.mod
├── LICENSE
└── README.md
```

Platform-specific implementations will use Go build constraints where necessary.

## Development

The project requires a supported Go toolchain.

Build the Linux executable:

```bash
GOOS=linux GOARCH=amd64 go build -o dist/linux-amd64/pipeferry ./cmd/pipeferry
```

Build the Windows executable:

```bash
GOOS=windows GOARCH=amd64 go build -o dist/windows-amd64/pipeferry.exe ./cmd/pipeferry
```

Run tests:

```bash
go test ./...
```

Run tests with the race detector on supported platforms:

```bash
go test -race ./...
```

Windows named-pipe integration tests require a Windows environment. End-to-end WSL tests require both Windows and WSL interoperability.

## Testing strategy

The intended test coverage includes:

### Unit tests

* Bidirectional stream forwarding
* Partial reads and writes
* EOF propagation
* Context cancellation
* Timeout behavior
* Child-process cleanup
* Unix socket path validation
* Named-pipe name validation
* Exit-code handling

### Integration tests

* Unix socket to local child-process echo service
* Standard input and output to a mock Windows named pipe
* Multiple simultaneous Unix socket connections
* Upstream process startup failure
* Upstream named-pipe connection failure
* Client disconnect during an active transfer
* Parent shutdown with active child processes

### End-to-end tests

* WSL Unix socket to Windows named pipe
* Windows OpenSSH agent access through `SSH_AUTH_SOCK`
* Repeated SSH agent queries
* Concurrent SSH and Git operations
* WSL shutdown and restart
* Windows-side service restart
* Long-running idle connections

## Relationship to OmniSSHAgent

Pipeferry originated from the need to expose OmniSSHAgent named pipes to WSL applications.

The project is maintained as a separate, general-purpose utility so that it can also be used with:

* Windows OpenSSH Agent
* Password-manager SSH agents
* Development tools exposing named pipes
* Local IPC services
* Custom Windows applications

Pipeferry does not depend on OmniSSHAgent and does not contain OmniSSHAgent-specific configuration or key-management logic.

## Roadmap

Initial milestones:

* Implement the `npipe-connect` command on Windows
* Implement the `unix-listen` command on Linux
* Add safe bidirectional stream forwarding
* Add graceful shutdown and timeout handling
* Add concurrent connection support
* Add Windows and WSL integration tests
* Add release builds for Windows and Linux
* Add checksum generation
* Add a PowerShell installer
* Add installation and troubleshooting documentation

Possible future work:

* Additional Unix socket connection modes
* Named-pipe listener mode
* TCP transports
* Structured logging
* Health and diagnostic commands
* Persistent cross-platform bridge sessions
* Optional connection multiplexing

## Contributing

Issues and pull requests are welcome once the initial implementation and contribution guidelines are available.

When reporting a problem, include:

* Pipeferry version
* Windows version
* WSL version and distribution
* Exact command line
* Relevant standard-error output
* Whether WSL interoperability is enabled
* The type of named-pipe service being accessed

Do not include private keys, authentication payloads, or other sensitive stream contents.

## License

This project is expected to be released under the MIT License.
