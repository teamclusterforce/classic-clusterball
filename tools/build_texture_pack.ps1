# tools/build_texture_pack.ps1
# Builds the high-resolution texture packs from <GameDir>\textures\dump (written by the game with TextureDump=1,
# see dump_all_menus.ps1): Real-ESRGAN 4x (pack "hd") and optionally a second 4x pass (pack "hd16"), then BC7 with
# mipmaps via texconv. Only textures without a result yet are processed, so the script can be re-run after new dumps.
# Afterwards run build_ui_pack.ps1 for the textures drawn in 2D (menus, HUD).
#
#   powershell -File tools\build_texture_pack.ps1 -GameDir C:\clusterball [-Hd16]
param([Parameter(Mandatory)][string]$GameDir, [switch]$Hd16)

# Not 'Stop': Windows PowerShell 5.1 turns any stderr line of a native tool (Real-ESRGAN prints its GPU info there)
# into a terminating error. Failures are reported by the tools' own output instead.
$ErrorActionPreference = 'Continue'
. (Join-Path $PSScriptRoot 'packtools.ps1')
Require-Tool $Esrgan 'Real-ESRGAN'
Require-Tool $Texconv 'texconv'
$tex = Join-Path $GameDir 'textures'
Add-Type -AssemblyName System.Drawing

function Upscale([string]$in, [string]$out) {
    # Only files without a result yet; Real-ESRGAN loses the GPU on images below 16 px, so those stay original.
    $stage = Join-Path $tex ('stage_' + (Split-Path -Leaf $out))
    New-Item -ItemType Directory -Force $stage, $out | Out-Null
    Get-ChildItem $stage -Filter *.png | Remove-Item
    $n = 0
    foreach ($f in Get-ChildItem $in -Filter *.png) {
        if (Test-Path (Join-Path $out $f.Name)) { continue }
        $img = [Drawing.Image]::FromFile($f.FullName)
        $ok = $img.Width -ge 16 -and $img.Height -ge 16
        $img.Dispose()
        if ($ok) { Copy-Item $f.FullName $stage; $n++ }
    }
    Write-Host "$n new textures for $(Split-Path -Leaf $out)"
    if ($n -gt 0) {
        & $Esrgan -i $stage -o $out -n realesrgan-x4plus -m $EsrganModels -f png 2>&1 |
            Where-Object { $_ -match 'failed' } | ForEach-Object { Write-Warning $_ }
    }
    Remove-Item $stage -Recurse -Force
}

function Compress([string]$in, [string]$out) {
    New-Item -ItemType Directory -Force $out | Out-Null
    $list = Join-Path $tex 'texconv_list.txt'
    $todo = @(Get-ChildItem $in -Filter *.png | Where-Object { -not (Test-Path (Join-Path $out ($_.BaseName + '.dds'))) })
    Write-Host "$($todo.Count) textures to compress into $(Split-Path -Leaf $out)"
    if ($todo.Count -eq 0) { return }
    # Batches of 100: one texconv process over thousands of 8k images runs out of memory.
    for ($i = 0; $i -lt $todo.Count; $i += 100) {
        $todo[$i..([Math]::Min($i + 99, $todo.Count - 1))].FullName | Set-Content -Encoding ascii $list
        # --ignore-srgb: the PNGs carry no colour space; without it texconv darkens them by a gamma conversion
        & $Texconv -nologo --ignore-srgb -f BC7_UNORM -m 0 -y -o $out -flist $list 2>&1 |
            Where-Object { $_ -match 'FAILED|ERROR' } | ForEach-Object { Write-Warning $_ }
        Write-Host "  $([Math]::Min($i + 100, $todo.Count))/$($todo.Count)"
    }
    Remove-Item $list
}

Upscale (Join-Path $tex 'dump') (Join-Path $tex 'up')
Compress (Join-Path $tex 'up') (Join-Path $tex 'hd')
if ($Hd16) {
    Upscale (Join-Path $tex 'up') (Join-Path $tex 'up16')
    Compress (Join-Path $tex 'up16') (Join-Path $tex 'hd16')
}
Write-Host 'done'
