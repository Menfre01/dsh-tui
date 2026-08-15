# dsh-tui Windows installer
# Usage: powershell -ExecutionPolicy Bypass -File install.ps1
#
# dsh-tui is a pure client — it requires a running dsh host (deepseek-harness
# "dsh web") to connect to.

param(
    [string]$InstallDir = "$env:USERPROFILE\.local\bin"
)

# Fail fast on any error instead of silently continuing with a broken install
$ErrorActionPreference = "Stop"

$Repo = "Menfre01/dsh-tui"
$Binary = "dsh-tui"

# PowerShell 5.1 on older Windows defaults to TLS 1.0/1.1, which GitHub rejects.
# Safe on PowerShell 7+ (property still exists, already defaults to TLS 1.2).
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

# Detect architecture
$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "ARM64" { "arm64" }
    "AMD64" { "amd64" }
    default { "amd64" }
}

$ZipName = "${Binary}_windows_$Arch.zip"
$Url = "https://github.com/$Repo/releases/latest/download/$ZipName"

Write-Host "-> Downloading dsh-tui (windows/$Arch)..."
Write-Host "   $Url"

$TmpDir = Join-Path $env:TEMP "dsh-tui-install"
New-Item -ItemType Directory -Force -Path $TmpDir | Out-Null

try {
    $ZipFile = Join-Path $TmpDir "dsh-tui.zip"
    # -UseBasicParsing: required on PowerShell 5.1 without IE engine (Server Core), no-op on 7+
    Invoke-WebRequest -Uri $Url -OutFile $ZipFile -UseBasicParsing

    # Verify SHA256 against the release checksums (skip if the entry is missing)
    $ChecksumsUrl = "https://github.com/$Repo/releases/latest/download/checksums.txt"
    try {
        $checksums = (Invoke-WebRequest -Uri $ChecksumsUrl -UseBasicParsing).Content
        $line = $checksums -split '\r?\n' | Where-Object { $_ -match [regex]::Escape($ZipName) } | Select-Object -First 1
        if ($line) {
            $expected = ($line -split '\s+')[0]
            $actual = (Get-FileHash -Algorithm SHA256 -Path $ZipFile).Hash.ToLower()
            if ($actual -ne $expected) {
                Write-Host "!  SHA256 mismatch for $ZipName"
                Write-Host "   expected: $expected"
                Write-Host "   actual:   $actual"
                exit 1
            }
            Write-Host "v  SHA256 verified"
        } else {
            Write-Host "!  checksums.txt has no entry for $ZipName; skipping verification"
        }
    } catch {
        Write-Host "!  Could not fetch checksums.txt; skipping verification"
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Expand-Archive -Path $ZipFile -DestinationPath $TmpDir -Force
    Move-Item -Force (Join-Path $TmpDir "$Binary.exe") (Join-Path $InstallDir "$Binary.exe")
    Write-Host ""
    Write-Host "v  dsh-tui installed to $InstallDir\$Binary.exe"
} finally {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}

# Add to User PATH (persists across sessions)
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$InstallDir*") {
    $newPath = if ($userPath) { "$InstallDir;$userPath" } else { $InstallDir }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "v  Added $InstallDir to your User PATH."
    Write-Host "   Open a new terminal to use dsh-tui."
} else {
    Write-Host "v  $InstallDir already on your User PATH."
}

Write-Host ""
Write-Host "Next steps:"
Write-Host "  1. Start the dsh host:          dsh web"
Write-Host "  2. Launch the TUI:              dsh-tui           # new session"
Write-Host "  3. Resume a session:            dsh-tui --resume <id>"
