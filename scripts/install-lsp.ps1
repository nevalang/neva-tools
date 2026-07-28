param(
    [string]$Version,
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "Neva\\bin")
)

$ErrorActionPreference = "Stop"
$repository = "nevalang/neva-tools"

if ([string]::IsNullOrWhiteSpace($Version)) {
    $releases = Invoke-RestMethod "https://api.github.com/repos/$repository/releases?per_page=100"
    $releaseTag = ($releases | Where-Object { $_.tag_name -like "lsp/v*" } | Select-Object -First 1).tag_name
} else {
    $normalizedVersion = $Version -replace "^lsp/", "" -replace "^v", ""
    $releaseTag = "lsp/v$normalizedVersion"
}

if ([string]::IsNullOrWhiteSpace($releaseTag)) {
    throw "Could not find an LSP component release in $repository"
}

$architecture = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { throw "Unsupported architecture" }
if ($env:PROCESSOR_ARCHITECTURE -match "ARM64") { $architecture = "arm64" }
$asset = "neva-lsp-windows-$architecture.exe"
$releaseBase = "https://github.com/$repository/releases/download/$releaseTag"
$temporaryDir = Join-Path ([System.IO.Path]::GetTempPath()) ("neva-lsp-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $temporaryDir | Out-Null

try {
    Write-Host "Installing Neva LSP $($releaseTag.Replace('lsp/', '')) for windows/$architecture..."
    $checksumsPath = Join-Path $temporaryDir "SHA256SUMS"
    $assetPath = Join-Path $temporaryDir $asset
    Invoke-WebRequest "$releaseBase/SHA256SUMS" -OutFile $checksumsPath
    Invoke-WebRequest "$releaseBase/$asset" -OutFile $assetPath

    $expectedChecksum = ((Get-Content $checksumsPath) | Where-Object { $_ -match ("\\s" + [regex]::Escape($asset) + "$" ) } | Select-Object -First 1).Split()[0]
    if ([string]::IsNullOrWhiteSpace($expectedChecksum)) { throw "Release checksum is missing for $asset" }
    $actualChecksum = (Get-FileHash $assetPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualChecksum -ne $expectedChecksum.ToLowerInvariant()) { throw "Checksum verification failed for $asset" }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Copy-Item $assetPath (Join-Path $InstallDir "neva-lsp.exe") -Force
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (($userPath -split ";") -notcontains $InstallDir) {
        [Environment]::SetEnvironmentVariable("Path", (($userPath.TrimEnd(";") + ";" + $InstallDir).TrimStart(";")), "User")
        Write-Host "Added $InstallDir to your User PATH. Open a new terminal before running neva tool lsp."
    }
    Write-Host "Installed neva-lsp to $(Join-Path $InstallDir 'neva-lsp.exe')"
} finally {
    Remove-Item -Recurse -Force $temporaryDir -ErrorAction SilentlyContinue
}
