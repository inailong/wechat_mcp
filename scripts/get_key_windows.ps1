<#
.SYNOPSIS
    WeChat MCP Key Extraction & Auto-Setup Script for Windows (PowerShell)
.DESCRIPTION
    Checks Python environment, invokes get_key_windows.py or installs dependencies,
    and configures %APPDATA%\wechat_mcp\config.json automatically.
#>

$ErrorActionPreference = "Continue"

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "   WeChat MCP - Windows Key Extractor (PowerShell Helper)   " -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan

# 1. Check WeChat process
$wechatProc = Get-Process -Name "WeChat" -ErrorAction SilentlyContinue
if (-not $wechatProc) {
    Write-Host "[!] WeChat is not currently running." -ForegroundColor Yellow
    Write-Host "    Please start WeChat and log in first." -ForegroundColor Yellow
    $ans = Read-Host "    Press 'Y' if you have logged in now, or any other key to abort"
    if ($ans -ne 'Y' -and $ans -ne 'y') {
        Write-Host "[-] Aborted." -ForegroundColor Red
        exit 1
    }
}

# 2. Check Python
$pythonCmd = Get-Command python -ErrorAction SilentlyContinue
if (-not $pythonCmd) {
    $pythonCmd = Get-Command python3 -ErrorAction SilentlyContinue
}

if ($pythonCmd) {
    Write-Host "[+] Found Python: $($pythonCmd.Source)" -ForegroundColor Green
    $scriptPath = Join-Path $PSScriptRoot "get_key_windows.py"
    & $pythonCmd.Source $scriptPath
} else {
    Write-Host "[!] Python was not found in PATH." -ForegroundColor Yellow
    Write-Host "[*] Trying to run PyWxDump or guide alternative setup..." -ForegroundColor Yellow
    
    # Check if pywxdump exists standalone
    $wxdumpCmd = Get-Command pywxdump -ErrorAction SilentlyContinue
    if ($wxdumpCmd) {
        Write-Host "[+] Running pywxdump bias --auto..." -ForegroundColor Green
        & $wxdumpCmd.Source bias --auto
    } else {
        Write-Host "[*] You can install Python easily via winget:" -ForegroundColor Cyan
        Write-Host "    winget install Python.Python.3.11" -ForegroundColor White
        Write-Host "    pip install pywxdump" -ForegroundColor White
        Write-Host "    python scripts\get_key_windows.py" -ForegroundColor White
        
        Write-Host "`n[*] Or if you know your 64-character database key, enter it directly:" -ForegroundColor Cyan
        $manualKey = Read-Host "    Enter 64-char Hex Key"
        if ($manualKey -and $manualKey.Length -eq 64) {
            $appDataDir = Join-Path $env:APPDATA "wechat_mcp"
            if (-not (Test-Path $appDataDir)) {
                New-Item -ItemType Directory -Path $appDataDir -Force | Out-Null
            }
            $cfgFile = Join-Path $appDataDir "config.json"
            $cfgObj = @{ decryptKey = $manualKey.ToLower() }
            $cfgObj | ConvertTo-Json | Set-Content -Path $cfgFile -Encoding UTF8
            Write-Host "[+] Configuration saved to: $cfgFile" -ForegroundColor Green
            
            $exePath = Join-Path $PSScriptRoot "..\bin\wechat_mcp.exe"
            if (Test-Path $exePath) {
                Write-Host "[*] Running diagnostics..." -ForegroundColor Cyan
                & $exePath --doctor
            }
        }
    }
}
