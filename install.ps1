$ErrorActionPreference = "Stop"
$repo = "brunodasilvalenga/act"
$binary = "act"

# Get latest version
$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
$version = $release.tag_name -replace '^v', ''

# Detect arch
$arch = if ([Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
} else { "amd64" }

$archive = "${binary}_${version}_windows_${arch}.zip"
$url = "https://github.com/$repo/releases/download/v$version/$archive"
$checksumUrl = "https://github.com/$repo/releases/download/v$version/checksums.txt"

$tmp = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory $_ }

Write-Host "Downloading $binary v$version for windows/$arch..."
Invoke-WebRequest -Uri $url -OutFile "$tmp\$archive"
Invoke-WebRequest -Uri $checksumUrl -OutFile "$tmp\checksums.txt"

# Verify checksum
$expected = (Get-Content "$tmp\checksums.txt" | Select-String $archive).ToString().Split(" ")[0]
$actual = (Get-FileHash "$tmp\$archive" -Algorithm SHA256).Hash.ToLower()

if ($expected -ne $actual) {
    Write-Error "Checksum mismatch!"
    exit 1
}

# Extract
Expand-Archive "$tmp\$archive" -DestinationPath $tmp

# Install to user's local bin
$installDir = "$env:LOCALAPPDATA\Programs\act"
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
Copy-Item "$tmp\$binary.exe" "$installDir\$binary.exe" -Force

# Add to PATH if not already there
$userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$userPath;$installDir", "User")
    Write-Host "Added $installDir to PATH (restart terminal to take effect)"
}

Write-Host "Installed $binary v$version to $installDir\$binary.exe"

# Cleanup
Remove-Item -Recurse -Force $tmp
