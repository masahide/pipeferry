$ErrorActionPreference = 'Stop'

$InstallDir = if ($env:PIPEFERRY_WINDOWS_INSTALL_DIR) {
    $env:PIPEFERRY_WINDOWS_INSTALL_DIR
} else {
    Join-Path $env:LOCALAPPDATA 'Programs\pipeferry'
}
$Binary = Join-Path $InstallDir 'pipeferry.exe'

if (Test-Path -LiteralPath $Binary) {
    Remove-Item -Force -LiteralPath $Binary
    Write-Host "Removed Windows binary: $Binary"
} else {
    Write-Host "Windows binary is not installed: $Binary"
}

$UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$PathEntries = @($UserPath -split ';' | Where-Object {
    $_ -and $_.TrimEnd('\') -ine $InstallDir.TrimEnd('\')
})
$NewUserPath = $PathEntries -join ';'
if ($NewUserPath -ne $UserPath) {
    [Environment]::SetEnvironmentVariable('Path', $NewUserPath, 'User')
    Write-Host "Removed from the user PATH: $InstallDir"
}

if ((Test-Path -LiteralPath $InstallDir) -and
    -not (Get-ChildItem -Force -LiteralPath $InstallDir | Select-Object -First 1)) {
    Remove-Item -Force -LiteralPath $InstallDir
    Write-Host "Removed empty install directory: $InstallDir"
}
