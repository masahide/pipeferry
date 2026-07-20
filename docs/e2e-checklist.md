# Windows 11 and WSL2 E2E Checklist

Run this checklist from one clean commit before a release. Record the commit,
Windows and WSL versions, Pipeferry versions, and results.

- [x] Build Linux amd64 and Windows amd64 binaries from the same commit.
- [x] Run `pipeferry.exe --version` from WSL.
- [x] Transfer null bytes, non-ASCII bytes, and at least 1 MiB through a test
  named-pipe echo server.
- [x] Run `ssh-add -l` and `ssh-add -L` through Windows OpenSSH Agent.
- [ ] Authenticate with a temporary test key to exercise signing.
- [ ] Stop and restart the named-pipe service during and between connections;
  confirm the Unix listener remains available.
- [x] Complete 32 concurrent connections and 100 sequential connections.
- [x] Send SIGTERM during active transfers; confirm the socket, lock, and Windows
  child processes are gone.
- [x] Confirm no PowerShell, Socat, Python, or Node.js helper process was started.
- [x] Record child startup and round-trip p50, p95, and p99 latency.

## 2026-07-20 result

- Commit under test: `bdf57a9` plus the uncommitted E2E test harness and shutdown
  regression tests.
- Linux and Windows amd64 binaries were built from the same working tree.
- A deterministic 1,048,583-byte payload containing null and non-ASCII bytes
  completed a byte-for-byte Unix-socket-to-Windows-named-pipe round trip.
- `ssh-add -l`, `ssh-add -L`, and an SSH Agent signature operation succeeded
  through Pipeferry. The Windows `ssh-agent` service was disabled and stopped,
  so another compatible agent was providing `openssh-ssh-agent`.
- A missing Named Pipe failed two independent connections while `status --json`
  continued to report the Linux listener as running.
- 32 concurrent and 100 sequential SSH Agent connections completed. The Windows
  child-process count returned to zero.
- SIGTERM removed the Unix socket and lock and left no Windows Pipeferry child.
- Pipeferry launched only the explicitly configured Windows binary. PowerShell,
  Socat, Python, and Node.js are not runtime helpers.
- Across 100 samples, Windows process startup was p50 53.340 ms, p95 58.814 ms,
  and p99 59.729 ms. SSH Agent connection plus one identities round trip was
  p50 54.451 ms, p95 60.396 ms, and p99 64.067 ms.
- A temporary-key SSH authentication and live provider stop/restart remain
  outstanding.
