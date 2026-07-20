# Windows 11 and WSL2 E2E Checklist

Run this checklist from one clean commit before a release. Record the commit,
Windows and WSL versions, Pipeferry versions, and results.

- [ ] Build Linux amd64 and Windows amd64 binaries from the same commit.
- [ ] Run `pipeferry.exe --version` from WSL.
- [ ] Transfer null bytes, non-ASCII bytes, and at least 1 MiB through a test
  named-pipe echo server.
- [ ] Run `ssh-add -l` and `ssh-add -L` through Windows OpenSSH Agent.
- [ ] Authenticate with a temporary test key to exercise signing.
- [ ] Stop and restart the named-pipe service during and between connections;
  confirm the Unix listener remains available.
- [ ] Complete 32 concurrent connections and 100 sequential connections.
- [ ] Send SIGTERM during active transfers; confirm the socket, lock, and Windows
  child processes are gone.
- [ ] Confirm no PowerShell, Socat, Python, or Node.js helper process was started.
- [ ] Record child startup and round-trip p50, p95, and p99 latency.
