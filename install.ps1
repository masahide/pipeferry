$ErrorActionPreference = 'Stop'

$Repository = if ($env:PIPEFERRY_REPOSITORY) {
    $env:PIPEFERRY_REPOSITORY
} else {
    'masahide/pipeferry'
}
$Version = if ($env:PIPEFERRY_VERSION) {
    $env:PIPEFERRY_VERSION
} else {
    'latest'
}
$InstallDir = if ($env:PIPEFERRY_WINDOWS_INSTALL_DIR) {
    $env:PIPEFERRY_WINDOWS_INSTALL_DIR
} else {
    Join-Path $env:LOCALAPPDATA 'Programs\pipeferry'
}
$Asset = 'pipeferry-windows-amd64.zip'

if (-not [Environment]::Is64BitOperatingSystem) {
    throw 'pipeferry: only 64-bit Windows is supported'
}

if ($Version -eq 'latest') {
    $ReleaseBase = "https://github.com/$Repository/releases/latest/download"
} else {
    $ReleaseBase = "https://github.com/$Repository/releases/download/$Version"
}

$TempDir = Join-Path ([IO.Path]::GetTempPath()) ("pipeferry-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $TempDir | Out-Null

try {
    $Archive = Join-Path $TempDir $Asset
    $ChecksumFile = "$Archive.sha256"
    Invoke-WebRequest -UseBasicParsing -Uri "$ReleaseBase/$Asset" -OutFile $Archive
    Invoke-WebRequest -UseBasicParsing -Uri "$ReleaseBase/$Asset.sha256" -OutFile $ChecksumFile

    $ExpectedHash = ((Get-Content -Raw $ChecksumFile).Trim() -split '\s+')[0]
    $ActualHash = (Get-FileHash -Algorithm SHA256 $Archive).Hash
    if ($ActualHash -ine $ExpectedHash) {
        throw "pipeferry: checksum mismatch for $Asset"
    }

    $ExtractDir = Join-Path $TempDir 'extract'
    Expand-Archive -Path $Archive -DestinationPath $ExtractDir
    $Binary = Get-ChildItem -Path $ExtractDir -Filter 'pipeferry.exe' -Recurse |
        Select-Object -First 1
    if (-not $Binary) {
        throw 'pipeferry: Windows binary was not found in the release archive'
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Copy-Item -Force $Binary.FullName (Join-Path $InstallDir 'pipeferry.exe')

    $UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $PathEntries = @($UserPath -split ';' | Where-Object { $_ })
    if ($PathEntries -notcontains $InstallDir) {
        $NewUserPath = (($PathEntries + $InstallDir) -join ';')
        [Environment]::SetEnvironmentVariable('Path', $NewUserPath, 'User')
        $env:Path = "$env:Path;$InstallDir"
        Write-Host "Added to the user PATH: $InstallDir"
    }

    Write-Host "Installed Windows binary: $InstallDir\pipeferry.exe"
} finally {
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $TempDir
}
