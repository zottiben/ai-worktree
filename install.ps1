# Install the latest `awt` release binary on Windows.
#   irm https://raw.githubusercontent.com/zottiben/ai-worktree/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$repo = "zottiben/ai-worktree"
$installDir = "$env:LOCALAPPDATA\awt"

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }

$release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest"
$version = $release.tag_name
if (-not $version) { throw "Could not determine the latest release." }
$versionNum = $version.TrimStart("v")

$filename = "awt-v$versionNum-windows-$arch.zip"
$url = "https://github.com/$repo/releases/download/$version/$filename"

$tmpDir = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory -Path $_ }

Write-Host "Downloading awt $version for windows/$arch..."
Invoke-WebRequest -Uri $url -OutFile "$tmpDir\$filename"
Expand-Archive -Path "$tmpDir\$filename" -DestinationPath $tmpDir -Force

New-Item -ItemType Directory -Path $installDir -Force | Out-Null
Move-Item -Path "$tmpDir\awt.exe" -Destination "$installDir\awt.exe" -Force

Remove-Item -Recurse -Force $tmpDir

# Add to the user PATH if it isn't already there.
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    Write-Host "Added $installDir to your user PATH. Restart your terminal for it to take effect."
}

Write-Host "Installed awt $version to $installDir\awt.exe"
