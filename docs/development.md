# Development

This document contains build, test, and release information for Pipeferry
contributors. End-user installation and usage are documented in the project
README.

## Requirements

- Go 1.25 or newer
- Linux amd64 or WSL2 Ubuntu amd64 for Linux development
- Windows 11 amd64 for native Windows integration tests

## Build

Build the Linux binary:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -o pipeferry ./cmd/pipeferry
```

Build the Windows binary:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -o pipeferry.exe ./cmd/pipeferry
```

Release builds inject the version, commit, and build timestamp through linker
flags configured in `.github/workflows/release.yml`.

## Test and verification

```bash
go test ./...
go vet ./...
go test -race ./...
```

CI also builds Linux and Windows amd64 binaries and validates the shell and
PowerShell installer syntax.

Windows tests include a real named-pipe echo integration test. Linux tests cover
Unix socket modes, locking, stale recovery, and regular-file safety. Full
Windows-to-WSL validation, OpenSSH Agent signing, load, and process-leak checks
must be completed on a Windows 11 plus WSL2 machine using
[the E2E checklist](e2e-checklist.md).

## Release

Push a semantic-version tag beginning with `v`:

```bash
git tag -a v1.2.3 -m "pipeferry v1.2.3"
git push origin v1.2.3
```

The Release workflow builds Linux and Windows amd64 archives and publishes the
following stable asset names with SHA-256 checksum files:

- `pipeferry-linux-amd64.tar.gz`
- `pipeferry-linux-amd64.tar.gz.sha256`
- `pipeferry-windows-amd64.zip`
- `pipeferry-windows-amd64.zip.sha256`
- `install.sh`
- `install.ps1`
- `uninstall.sh`
- `uninstall.ps1`

The stable archive names are part of the installer contract. Do not rename them
without updating both installers.

On WSL, `install.sh` records the installed Windows executable as an absolute WSL
path in `${XDG_CONFIG_HOME:-$HOME/.config}/pipeferry/windows-executable`.
`unix-listen` and `doctor` resolve the exact child name `pipeferry.exe` through
this setting when it is not available on `PATH`. The Windows installer must not
add its installation directory to the Windows user `PATH`.
