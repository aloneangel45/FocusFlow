# FocusFlow Universal Windows Installer (amd64 & arm64)
# Usage:
#   irm https://raw.githubusercontent.com/aloneangel45/FocusFlow/main/install.ps1 | iex
# Optional environment variables:
#   $env:FOCUSFLOW_VERSION        Release tag to install (e.g. "v2.2.2" or "latest", default: "latest")
#   $env:FOCUSFLOW_INSTALL_DIR    Target installation directory (default: "$env:LOCALAPPDATA\Programs\FocusFlow")
#   $env:FOCUSFLOW_REPO           GitHub owner/repo (default: "aloneangel45/FocusFlow")
#   $env:FOCUSFLOW_BASE_URL       Override release download base URL (for mirrors/testing)
#   $env:FOCUSFLOW_NO_PATH_UPDATE Set to "1" to skip updating User PATH in registry

# Sanitize PSModulePath on Windows PowerShell 5.1 (Desktop) BEFORE invoking any cmdlets
# so PowerShell\7 (.NET Core) module paths never break module autoloading.
if ($PSVersionTable -and $PSVersionTable.PSEdition -eq 'Desktop' -and $env:PSModulePath) {
    $cleanModulePaths = New-Object System.Collections.Generic.List[string]
    foreach ($modEntry in $env:PSModulePath.Split(';')) {
        $trimmedMod = $modEntry.Trim()
        if (-not $trimmedMod) { continue }
        if ($trimmedMod -match '(?i)[\\/]PowerShell[\\/]([67]([^\\/]*)?|Modules)([\\/]|$)') {
            continue
        }
        $cleanModulePaths.Add($trimmedMod)
    }
    $env:PSModulePath = [string]::Join(';', $cleanModulePaths.ToArray())
}

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Write-Info([string]$Message) {
    [Console]::WriteLine("[INFO] $Message")
}

function Write-Ok([string]$Message) {
    [Console]::WriteLine("[OK] $Message")
}

function Clear-FocusFlowReadOnly([string]$FilePath) {
    if ([System.IO.File]::Exists($FilePath)) {
        $attrs = [System.IO.File]::GetAttributes($FilePath)
        if (($attrs -band [System.IO.FileAttributes]::ReadOnly) -ne 0) {
            [System.IO.File]::SetAttributes($FilePath, ($attrs -band (-bnot [System.IO.FileAttributes]::ReadOnly)))
        }
    }
}

function Remove-FocusFlowDirectorySafe([string]$DirPath) {
    if (-not [System.IO.Directory]::Exists($DirPath)) {
        return
    }
    try {
        foreach ($file in [System.IO.Directory]::GetFiles($DirPath, '*', [System.IO.SearchOption]::AllDirectories)) {
            Clear-FocusFlowReadOnly -FilePath $file
        }
    } catch {
        # Best-effort attribute normalization prior to recursive delete
    }
    try {
        [System.IO.Directory]::Delete($DirPath, $true)
    } catch {
        # Never fail cleanup under transient AV scan locks
    }
}

function Invoke-FocusFlowDownload([string]$Uri, [string]$DestinationPath) {
    $wc = New-Object System.Net.WebClient
    try {
        $wc.Headers.Add('User-Agent', 'FocusFlow-Installer/2.2.2')
        $wc.DownloadFile($Uri, $DestinationPath)
    } finally {
        $wc.Dispose()
    }
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
    try {
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    } catch {
        # System default TLS configuration on modern runtimes
    }

    $repo = if ($env:FOCUSFLOW_REPO) { $env:FOCUSFLOW_REPO.Trim() } else { 'aloneangel45/FocusFlow' }
    $version = if ($env:FOCUSFLOW_VERSION) { $env:FOCUSFLOW_VERSION.Trim() } else { 'latest' }
    $userHome = [Environment]::GetFolderPath('UserProfile')
    if (-not $userHome) { $userHome = $HOME }

    $installDir = if ($env:FOCUSFLOW_INSTALL_DIR -and $env:FOCUSFLOW_INSTALL_DIR.Trim() -ne '') {
        $env:FOCUSFLOW_INSTALL_DIR.Trim()
    } elseif ($env:LOCALAPPDATA -and $env:LOCALAPPDATA.Trim() -ne '') {
        [System.IO.Path]::Combine($env:LOCALAPPDATA.Trim(), 'Programs', 'FocusFlow')
    } else {
        [System.IO.Path]::Combine($userHome, '.focusflow', 'bin')
    }

    $arch = Get-FocusFlowArch
    $innerExe = "focusflow-windows-$arch.exe"
    $archiveName = "focusflow-windows-$arch.zip"
    $baseUrl = Get-FocusFlowBaseUrl -Repo $repo -Version $version

    $tmpDir = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), ("focusflow-install-" + [Guid]::NewGuid().ToString('N')))
    [void][System.IO.Directory]::CreateDirectory($tmpDir)

    try {
        $archivePath = [System.IO.Path]::Combine($tmpDir, $archiveName)
        $checksumsPath = [System.IO.Path]::Combine($tmpDir, 'checksums.txt')
        $extractDir = [System.IO.Path]::Combine($tmpDir, 'extracted')

        Write-Info "Detected platform: windows/$arch"
        Write-Info "Downloading $archiveName from $baseUrl..."
        Invoke-FocusFlowDownload -Uri "$baseUrl/$archiveName" -DestinationPath $archivePath

        Write-Info "Downloading checksums.txt for mandatory SHA-256 verification..."
        Invoke-FocusFlowDownload -Uri "$baseUrl/checksums.txt" -DestinationPath $checksumsPath

        $checksumsText = [System.IO.File]::ReadAllText($checksumsPath, [System.Text.Encoding]::UTF8)
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
        if ([System.IO.Directory]::Exists($extractDir)) {
            Remove-FocusFlowDirectorySafe -DirPath $extractDir
        }
        [System.IO.Compression.ZipFile]::ExtractToDirectory($archivePath, $extractDir)

        $extractedExe = [System.IO.Path]::Combine($extractDir, $innerExe)
        if (-not [System.IO.File]::Exists($extractedExe)) {
            throw "[ERR] Expected executable '$innerExe' not found inside $archiveName."
        }

        [void][System.IO.Directory]::CreateDirectory($installDir)
        $targetExe = [System.IO.Path]::Combine($installDir, 'focusflow.exe')
        Clear-FocusFlowReadOnly -FilePath $targetExe
        [System.IO.File]::Copy($extractedExe, $targetExe, $true)
        Clear-FocusFlowReadOnly -FilePath $targetExe

        if ($env:FOCUSFLOW_NO_PATH_UPDATE -ne '1') {
            $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
            $alreadyInUserPath = $false
            if ($userPath) {
                foreach ($entry in $userPath.Split(';')) {
                    $cleanEntry = $entry.Trim()
                    if (-not $cleanEntry) { continue }
                    if ($cleanEntry.TrimEnd('\').Equals($installDir.TrimEnd('\'), [System.StringComparison]::OrdinalIgnoreCase)) {
                        $alreadyInUserPath = $true
                        break
                    }
                }
            }
            if (-not $alreadyInUserPath) {
                $updatedUserPath = if ($userPath) { "$userPath;$installDir" } else { $installDir }
                [Environment]::SetEnvironmentVariable('Path', $updatedUserPath, 'User')
                Write-Info "Added '$installDir' to User PATH."
            }
        }

        $inSessionPath = $false
        if ($env:PATH) {
            foreach ($pEntry in $env:PATH.Split(';')) {
                if ($pEntry.Trim().TrimEnd('\').Equals($installDir.TrimEnd('\'), [System.StringComparison]::OrdinalIgnoreCase)) {
                    $inSessionPath = $true
                    break
                }
            }
        }
        if (-not $inSessionPath) {
            $env:PATH = if ($env:PATH) { "$installDir;$env:PATH" } else { $installDir }
        }

        Write-Ok "Installed FocusFlow to $targetExe"
        & $targetExe version
    } finally {
        Remove-FocusFlowDirectorySafe -DirPath $tmpDir
    }
}

Install-FocusFlow
