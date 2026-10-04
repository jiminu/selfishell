$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
if ($request.operation -eq 'file-move') {
    $source = Get-Item -LiteralPath $request.source -Force
    if ($source.PSIsContainer) { [IO.Directory]::Move($request.source, $request.destination) }
    else { [IO.File]::Move($request.source, $request.destination) }
    '{}'
    exit 0
}
if ($request.operation -eq 'probe' -or $request.operation -eq 'font-status') {
    Add-Type -AssemblyName System.Drawing
    $fonts = [Drawing.Text.InstalledFontCollection]::new()
    try {
        $appData = [Environment]::GetFolderPath('LocalApplicationData')
        $settingsPaths = @()
        $wslProfileGuids = @()
        if ($request.operation -eq 'probe') {
            $settingsPaths = @(
                "$appData\Packages\Microsoft.WindowsTerminal_8wekyb3d8bbwe\LocalState\settings.json",
                "$appData\Packages\Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe\LocalState\settings.json",
                "$appData\Microsoft\Windows Terminal\settings.json"
            ) | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf }
            $fragmentDir = "$appData\Microsoft\Windows Terminal\Fragments\Microsoft.WSL"
            if (-not [string]::IsNullOrEmpty($request.distro) -and (Test-Path -LiteralPath $fragmentDir -PathType Container)) {
                foreach ($fragment in Get-ChildItem -LiteralPath $fragmentDir -Filter '*.json' -File) {
                    try {
                        $profiles = (Get-Content -LiteralPath $fragment.FullName -Raw -Encoding UTF8 | ConvertFrom-Json).profiles
                        foreach ($profile in $profiles) {
                            if ($profile.name -ceq $request.distro -and $profile.guid) { $wslProfileGuids += $profile.guid }
                        }
                    } catch { } # Unreadable fragments cannot identify a safe target.
                }
            }
        }
        $registrations = @{}
        if ($request.operation -eq 'font-status') {
            $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Software\Microsoft\Windows NT\CurrentVersion\Fonts')
            if ($null -ne $key) {
                try {
                    foreach ($name in $key.GetValueNames()) {
                        if ($name.StartsWith('Selfishell ')) { $registrations[$name] = $key.GetValue($name) }
                    }
                } finally { $key.Dispose() }
            }
        }
        [pscustomobject]@{
            registrations = $registrations
            appData = $appData
            terminalInstalled = ((Test-Path -LiteralPath "$appData\Microsoft\WindowsApps\wt.exe") -or (Test-Path -LiteralPath "$appData\Microsoft\Windows Terminal\settings.json"))
            fontInstalled = (($fonts.Families.Name -contains 'JetBrainsMonoNL Nerd Font Mono') -or ($fonts.Families.Name -contains 'JetBrainsMonoNL NFM'))
            settingsPaths = @($settingsPaths)
            wslProfileGuids = @($wslProfileGuids)
        } | ConvertTo-Json -Compress
    } finally { $fonts.Dispose() }
    exit 0
}
if ($request.operation -eq 'font-register') {
    $font = Get-Item -LiteralPath $request.path
    if ($font.PSIsContainer -or ($font.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Unsafe Windows font path' }
    if ((Get-FileHash -LiteralPath $font.FullName -Algorithm SHA256).Hash -ne $request.checksum) { throw 'Windows font checksum mismatch' }
    $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Software\Microsoft\Windows NT\CurrentVersion\Fonts')
    try {
        $name = 'Selfishell ' + $request.name + ' (TrueType)'
        $existing = $key.GetValue($name)
        if ($null -ne $existing -and $existing -ne $font.FullName -and ([string]::IsNullOrEmpty($request.previousPath) -or $existing -ne $request.previousPath) -and ([string]::IsNullOrEmpty($request.alternatePreviousPath) -or $existing -ne $request.alternatePreviousPath)) { throw 'Existing Windows font registration is user data' }
        Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class SelfishellFont {
    [DllImport("gdi32.dll", CharSet=CharSet.Unicode)]
    public static extern int AddFontResourceEx(string path, uint flags, IntPtr reserved);
    [DllImport("gdi32.dll", CharSet=CharSet.Unicode)]
    public static extern bool RemoveFontResourceEx(string path, uint flags, IntPtr reserved);
    [DllImport("user32.dll", CharSet=CharSet.Unicode, SetLastError=true)]
    public static extern IntPtr SendMessageTimeout(IntPtr window, uint message, UIntPtr wparam, IntPtr lparam, uint flags, uint timeout, out UIntPtr result);
}
'@
        # Repeated setup does not increase the global font resource reference count.
        if ($existing -eq $font.FullName) {
            Add-Type -AssemblyName System.Drawing
            $fonts = [Drawing.Text.InstalledFontCollection]::new()
            try {
                if (($fonts.Families.Name -contains 'JetBrainsMonoNL Nerd Font Mono') -or ($fonts.Families.Name -contains 'JetBrainsMonoNL NFM')) { '{}'; exit 0 }
            } finally { $fonts.Dispose() }
        }
        if ([SelfishellFont]::AddFontResourceEx($font.FullName, 0, [IntPtr]::Zero) -eq 0) { throw 'Could not load Windows font' }
        $key.SetValue($name, $font.FullName, [Microsoft.Win32.RegistryValueKind]::String)
        if ($null -ne $existing -and $existing -ne $font.FullName) {
            [void][SelfishellFont]::RemoveFontResourceEx($existing, 0, [IntPtr]::Zero)
        }
        $result = [UIntPtr]::Zero
        [void][SelfishellFont]::SendMessageTimeout([IntPtr]0xffff, 0x001d, [UIntPtr]::Zero, [IntPtr]::Zero, 2, 1000, [ref]$result)
        '{}'
    } finally { $key.Dispose() }
    exit 0
}
throw 'Unknown Windows integration operation'
