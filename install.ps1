# WireGo Installer Script for Windows (PowerShell)
# Run: irm https://raw.githubusercontent.com/HappyRish01/wirego/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$Repo = "HappyRish01/wirego"
$InstallDir = "$env:LOCALAPPDATA\wirego"

Write-Host ""
Write-Host "WireGo Installer" -ForegroundColor Cyan
Write-Host ("-" * 40)

# Detect architecture
$Arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
Write-Host "Architecture: $Arch"

# Get latest release
Write-Host "Fetching latest release..."
try {
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $Tag = $Release.tag_name
} catch {
    Write-Host "error: Could not fetch latest release" -ForegroundColor Red
    exit 1
}

Write-Host "Version: $Tag"

# Download
$Url = "https://github.com/$Repo/releases/download/$Tag/wirego_windows_$Arch.zip"
Write-Host "Downloading..."

$TmpZip = "$env:TEMP\wirego_$Tag.zip"
$TmpDir = "$env:TEMP\wirego_extract"

try {
    Invoke-WebRequest -Uri $Url -OutFile $TmpZip

    # Extract
    if (Test-Path $TmpDir) { Remove-Item -Recurse -Force $TmpDir }
    Expand-Archive -Path $TmpZip -DestinationPath $TmpDir -Force

    # Create install directory
    if (!(Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    # Copy binary
    Copy-Item "$TmpDir\wirego.exe" -Destination "$InstallDir\wirego.exe" -Force

    # Add to PATH if not already there
    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($UserPath -notlike "*$InstallDir*") {
        Write-Host "Adding to PATH..."
        [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
        $env:Path = "$env:Path;$InstallDir"
    }

    Write-Host ""
    Write-Host ("-" * 40)
    Write-Host "WireGo installed successfully!" -ForegroundColor Green
    Write-Host ""
    Write-Host "Usage:"
    Write-Host "  wirego send <file-or-directory>"
    Write-Host "  wirego receive <code> <folder-name>"
    Write-Host ("-" * 40)
    Write-Host ""
    Write-Host "Restart your terminal for PATH changes to take effect." -ForegroundColor Yellow

} finally {
    # Cleanup
    Remove-Item -Force $TmpZip -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}
