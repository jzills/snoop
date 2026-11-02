# Installs snoop on Windows — https://github.com/jzills/snoop
#
#   irm https://jzills.github.io/snoop/install.ps1 | iex
#
# Downloads the newest snoop for Windows (x64 or arm64), checks it against the
# release's SHA256SUMS, installs it to %LOCALAPPDATA%\Programs\snoop, and adds
# that folder to your PATH. It never needs administrator rights.
#
#   $env:SNOOP_INSTALL_DIR  where to install (default: %LOCALAPPDATA%\Programs\snoop)
#   $env:SNOOP_VERSION      install this release instead of the newest, e.g. v0.2.0
#
# Everything is inside Install-Snoop, called on the last line, so a download
# cut off partway through defines a function and runs nothing.

function Install-Snoop {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue' # Invoke-WebRequest is slow with it
    $releases = 'https://github.com/jzills/snoop/releases'

    function Fail([string]$msg) {
        throw "snoop install: $msg"
    }

    # The machine's architecture, not this process's: a 32-bit PowerShell on
    # 64-bit Windows still gets the 64-bit build.
    $arch = $env:PROCESSOR_ARCHITEW6432
    if (-not $arch) { $arch = $env:PROCESSOR_ARCHITECTURE }
    switch ($arch) {
        'AMD64' { $arch = 'amd64' }
        'ARM64' { $arch = 'arm64' }
        default { Fail "no snoop build for Windows on $arch. Builds exist for x64 and arm64 ($releases)." }
    }

    $base = if ($env:SNOOP_INSTALL_BASE_URL) { $env:SNOOP_INSTALL_BASE_URL } else { $releases }
    $version = $env:SNOOP_VERSION
    if ($version) {
        if (-not $version.StartsWith('v')) { $version = "v$version" }
        $archive = "snoop_${version}_windows_$arch.zip"
        $urlDir = "$base/download/$version"
        $label = "snoop $version"
    } else {
        $archive = "snoop_windows_$arch.zip"
        $urlDir = "$base/latest/download"
        $label = 'the newest snoop'
    }

    $dir = $env:SNOOP_INSTALL_DIR
    if (-not $dir) { $dir = Join-Path $env:LOCALAPPDATA 'Programs\snoop' }

    $tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("snoop-install-" + [System.Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading $label for windows/$arch"
        foreach ($name in @($archive, 'SHA256SUMS')) {
            try {
                Invoke-WebRequest -UseBasicParsing -Uri "$urlDir/$name" -OutFile (Join-Path $tmp $name)
            } catch {
                Fail "could not download $urlDir/$name"
            }
        }

        $expected = $null
        foreach ($line in Get-Content (Join-Path $tmp 'SHA256SUMS')) {
            # "<hash>  <name>", or "<hash> *<name>" from a sha256sum in binary mode.
            $fields = $line -split '\s+', 2
            if ($fields.Count -eq 2 -and $fields[1].TrimStart('*') -eq $archive) { $expected = $fields[0] }
        }
        if (-not $expected) { Fail "$archive is not listed in the release's SHA256SUMS; refusing to install it" }
        $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $archive)).Hash
        if ($actual -ne $expected.ToUpperInvariant()) { Fail "checksum mismatch for $archive; refusing to install it" }

        Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath (Join-Path $tmp 'x')
        $exe = Join-Path $tmp 'x\snoop\snoop.exe'
        if (-not (Test-Path $exe)) { Fail "$archive has no snoop\snoop.exe" }
        $reported = & $exe --version 2>&1 | Select-Object -First 1
        if ($LASTEXITCODE -ne 0) { Fail "the downloaded snoop doesn't run on this machine: $reported" }

        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        $target = Join-Path $dir 'snoop.exe'
        # A running snoop.exe can't be overwritten, but it can be renamed.
        if (Test-Path $target) {
            $old = "$target.old"
            Remove-Item -Force $old -ErrorAction SilentlyContinue
            Move-Item -Force $target $old
        }
        Copy-Item $exe $target
        Remove-Item -Force "$target.old" -ErrorAction SilentlyContinue
        Write-Host "Installed $reported to $target"

        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $parts = @($userPath -split ';' | Where-Object { $_ })
        if ($parts -notcontains $dir) {
            [Environment]::SetEnvironmentVariable('Path', (($parts + $dir) -join ';'), 'User')
            $env:Path = "$env:Path;$dir"
            Write-Host "Added $dir to your PATH. Open a new terminal, then run: snoop"
        } else {
            Write-Host 'Run: snoop'
        }
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
}

Install-Snoop
