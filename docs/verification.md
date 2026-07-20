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

WSL was not installed or available in this environment. Linux test execution,
the race detector, Windows-to-WSL transfer, OpenSSH Agent operations, signal and
load execution, process-leak observation, and latency measurement remain for the
release E2E checklist.
