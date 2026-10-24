param(
    [switch]$Check
)

$ErrorActionPreference = 'Stop'
[Console]::InputEncoding = [System.Text.UTF8Encoding]::new($false)
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)

$goBotExe = Join-Path $PSScriptRoot 'go_bot\wxbot.exe'
if (-not (Test-Path $goBotExe)) {
    Write-Host '正在编译 Go 机器人程序...'
    Push-Location (Join-Path $PSScriptRoot 'go_bot')
    try {
        & go build -o wxbot.exe main.go
        if ($LASTEXITCODE -ne 0) { throw 'Go 编译失败' }
    } finally {
        Pop-Location
    }
}

$rustDll = Join-Path $PSScriptRoot 'rust_bridge\target\release\rust_bridge.dll'
if (-not (Test-Path $rustDll)) {
    Write-Host '正在编译 Rust 注入模块...'
    Push-Location (Join-Path $PSScriptRoot 'rust_bridge')
    try {
        & cargo build --release
        if ($LASTEXITCODE -ne 0) { throw 'Cargo 编译失败' }
    } finally {
        Pop-Location
    }
}

if ($Check) {
    & $goBotExe -check -root $PSScriptRoot
    exit $LASTEXITCODE
}

& $goBotExe -root $PSScriptRoot
exit $LASTEXITCODE
