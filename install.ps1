# FocusFlow Universal Windows Installer (amd64 & arm64)
# Usage:
#   irm https://raw.githubusercontent.com/aloneangel45/FocusFlow/main/install.ps1 | iex
# Optional environment variables:
#   $env:FOCUSFLOW_VERSION        Release tag to install (e.g. "v2.2.1" or "latest", default: "latest")
#   $env:FOCUSFLOW_INSTALL_DIR    Target installation directory (default: "$env:LOCALAPPDATA\Programs\FocusFlow")
#   $env:FOCUSFLOW_REPO           GitHub owner/repo (default: "aloneangel45/FocusFlow")
#   $env:FOCUSFLOW_BASE_URL       Override release download base URL (for mirrors/testing)
#   $env:FOCUSFLOW_NO_PATH_UPDATE Set to "1" to skip updating User PATH in registry

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Write-Info([string]$Message) {
    Write-Host "[INFO] $Message"
}

function Write-Ok([string]$Message) {
    Write-Host "[OK] $Message"
}

function Get-FocusFlowArch {
    $candidates = @()
    try {
        $osArch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
        if ($osArch) { $candidates += $osArch }
    } catch {
        # Fallback on older .NET Framework runtimes
    }
    if ($env:PROCESSOR_ARCHITEW6432) { $candidates += $env:PROCESSOR_ARCHITEW6432 }
    if ($env:PROCESSOR_ARCHITECTURE) { $candidates += $env:PROCESSOR_ARCHITECTURE }

    foreach ($raw in $candidates) {
        switch ($raw.Trim().ToUpperInvariant()) {
            'ARM64' { return 'arm64' }
            'AMD64' { return 'amd64' }
            'X64'   { return 'amd64' }
        }
    }
    throw "[ERR] Unsupported Windows CPU architecture: $($candidates -join ', ') (expected amd64 or arm64)."
}

function Get-FocusFlowBaseUrl([string]$Repo, [string]$Version) {
    if ($env:FOCUSFLOW_BASE_URL -and $env:FOCUSFLOW_BASE_URL.Trim() -ne '') {
        return $env:FOCUSFLOW_BASE_URL.Trim().TrimEnd('/')
    }
    if (-not $Version -or $Version.Trim().ToLowerInvariant() -eq 'latest') {
        return "https://github.com/$Repo/releases/latest/download"
    }
    $cleanVer = $Version.Trim().TrimStart('v', 'V')
    return "https://github.com/$Repo/releases/download/v$cleanVer"
}

function Get-ExpectedSha256([string]$ChecksumsContent, [string]$AssetName) {
    $cleanContent = $ChecksumsContent.TrimStart([char]0xFEFF)
    foreach ($line in ($cleanContent -split "`r?`n")) {
        $trimmed = $line.Trim().TrimStart([char]0xFEFF)
        if (-not $trimmed) { continue }
        $parts = $trimmed -split '\s+', 2
        if ($parts.Count -lt 2) { continue }
        $hashCandidate = $parts[0].Trim().TrimStart([char]0xFEFF).ToLowerInvariant()
        $fileCandidate = $parts[1].Trim().TrimStart('*')
        if ($fileCandidate -eq $AssetName) {
            if ($hashCandidate -notmatch '^[0-9a-f]{64}$') {
                throw "[ERR] Malformed SHA-256 digest for $AssetName in checksums.txt: '$hashCandidate'"
            }
            return $hashCandidate
        }
    }
    throw "[ERR] SHA-256 entry for '$AssetName' was not found in checksums.txt."
}

function Install-FocusFlow {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $repo = if ($env:FOCUSFLOW_REPO) { $env:FOCUSFLOW_REPO.Trim() } else { 'aloneangel45/FocusFlow' }
    $version = if ($env:FOCUSFLOW_VERSION) { $env:FOCUSFLOW_VERSION.Trim() } else { 'latest' }
    $installDir = if ($env:FOCUSFLOW_INSTALL_DIR) {
        $env:FOCUSFLOW_INSTALL_DIR.Trim()
    } elseif ($env:LOCALAPPDATA) {
        Join-Path $env:LOCALAPPDATA 'Programs\FocusFlow'
    } else {
        Join-Path $HOME '.focusflow\bin'
    }

    $arch = Get-FocusFlowArch
    $innerExe = "focusflow-windows-$arch.exe"
    $archiveName = "focusflow-windows-$arch.zip"
    $baseUrl = Get-FocusFlowBaseUrl -Repo $repo -Version $version

    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("focusflow-install-" + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        $archivePath = Join-Path $tmpDir $archiveName
        $checksumsPath = Join-Path $tmpDir 'checksums.txt'
        $extractDir = Join-Path $tmpDir 'extracted'

        Write-Info "Detected platform: windows/$arch"
        Write-Info "Downloading $archiveName from $baseUrl..."
        Invoke-WebRequest -Uri "$baseUrl/$archiveName" -OutFile $archivePath -UseBasicParsing

        Write-Info "Downloading checksums.txt for mandatory SHA-256 verification..."
        Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile $checksumsPath -UseBasicParsing

        $checksumsText = Get-Content -Path $checksumsPath -Raw -Encoding UTF8
        $expectedHash = Get-ExpectedSha256 -ChecksumsContent $checksumsText -AssetName $archiveName
        $sha256Hasher = [System.Security.Cryptography.SHA256]::Create()
        $archiveStream = [System.IO.File]::OpenRead($archivePath)
        try {
            $actualHash = [System.BitConverter]::ToString($sha256Hasher.ComputeHash($archiveStream)).Replace('-', '').ToLowerInvariant()
        } finally {
            $archiveStream.Dispose()
            $sha256Hasher.Dispose()
        }

        if ($actualHash -ne $expectedHash) {
            throw "[ERR] SHA-256 checksum verification FAILED for $archiveName! Expected: $expectedHash, Actual: $actualHash"
        }
        Write-Ok "Verified SHA-256: $actualHash"

        Add-Type -AssemblyName System.IO.Compression.FileSystem
        if (Test-Path -LiteralPath $extractDir) {
            Remove-Item -LiteralPath $extractDir -Recurse -Force
        }
        [System.IO.Compression.ZipFile]::ExtractToDirectory($archivePath, $extractDir)

        $extractedExe = Join-Path $extractDir $innerExe
        if (-not (Test-Path -LiteralPath $extractedExe)) {
            throw "[ERR] Expected executable '$innerExe' not found inside $archiveName."
        }

        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        $targetExe = Join-Path $installDir 'focusflow.exe'
        Copy-Item -LiteralPath $extractedExe -Destination $targetExe -Force

        if ($env:FOCUSFLOW_NO_PATH_UPDATE -ne '1') {
            $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
            $pathEntries = @()
            if ($userPath) {
                $pathEntries = $userPath -split ';' | Where-Object { $_ -ne '' }
            }
            $alreadyInUserPath = $false
            foreach ($entry in $pathEntries) {
                if ($entry.TrimEnd('\').Equals($installDir.TrimEnd('\'), [System.StringComparison]::OrdinalIgnoreCase)) {
                    $alreadyInUserPath = $true
                    break
                }
            }
            if (-not $alreadyInUserPath) {
                $updatedUserPath = if ($userPath) { "$userPath;$installDir" } else { $installDir }
                [Environment]::SetEnvironmentVariable('Path', $updatedUserPath, 'User')
                Write-Info "Added '$installDir' to User PATH."
            }
        }

        if (-not (($env:PATH -split ';') | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') })) {
            $env:PATH = "$installDir;$env:PATH"
        }

        Write-Ok "Installed FocusFlow to $targetExe"
        & $targetExe version
    } finally {
        if (Test-Path -LiteralPath $tmpDir) {
            Remove-Item -LiteralPath $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

Install-FocusFlow
