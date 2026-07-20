# Verification Record

## 2026-07-20

Environment: Windows amd64, Go 1.26.5. The module declares Go 1.25.

- `go test ./...`: passed, including the real Windows named-pipe integration
  tests and standard-output byte-integrity contract.
- `go vet ./...`: passed on the native Windows build.
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/pipeferry`: passed.
- Linux test packages for `unixsocket`, `execbridge`, and `cli`: compiled
  successfully with `go test -c`.
- `GOOS=linux GOARCH=amd64 go vet ./...`: passed.
- `govulncheck ./...` using `golang.org/x/vuln` v1.6.0: no called
  vulnerabilities found. The scanner reported one vulnerability in imported
  packages that is not reached by Pipeferry.
- A native Windows `go test -race ./...` attempt was not runnable because the
  local Go environment has CGO disabled. The required Linux race job remains in
  CI.

That verification was performed before WSL became available. The following
WSL2 verification supersedes its WSL-specific limitations.

### WSL2 follow-up

- The published Linux and Windows `v0.1.0` binaries reported the same version,
  commit, and build timestamp.
- The latest `main` CI runs passed Linux race tests, Windows tests, vet, builds,
  and installer syntax checks.
- A Windows test Named Pipe echoed a deterministic 1,048,583-byte payload
  through the WSL Unix socket without modification.
- `ssh-add -l`, `ssh-add -L`, and SSH Agent signing succeeded through the
  `openssh-ssh-agent` pipe.
- A missing Named Pipe failed without stopping the Linux listener.
- 32 concurrent and 100 sequential real SSH Agent connections completed with
  zero remaining Windows Pipeferry processes.
- SIGINT and SIGTERM regression tests passed. A direct SIGTERM E2E removed the
  socket and lock and left zero Windows Pipeferry processes.
- Across 100 samples, Windows process startup was p50 53.340 ms, p95 58.814 ms,
  and p99 59.729 ms. SSH Agent connection plus one identities round trip was
  p50 54.451 ms, p95 60.396 ms, and p99 64.067 ms.
- An unsafe existing socket parent such as `/tmp` is now rejected without
  changing its permissions; a dedicated mode `0700` directory is required.
- Temporary-key SSH authentication and a live compatible-agent restart remain
  outstanding.
