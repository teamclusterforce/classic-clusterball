# tools/build_ui_pack.ps1
# Rebuilds the textures drawn in 2D (menus, HUD, text pictures; listed by TextureDump in textures\ui_textures.txt).
# Each is upscaled with both Real-ESRGAN models: x4plus-anime (drawn art: clean edges) and x4plus (photo model:
# keeps every pixel step). pickmodel box-filters both back to the original size and keeps anime unless it deviates
# more than -Factor times as much as x4plus - that happens where anime invents shapes (5-px HUD text such as
# SUPERBOOST) and on photos (pilot portraits, venue previews), which suit x4plus anyway. The 16x pack runs the chosen
# model a second time. Fully transparent texels first get their neighbours' colour (bleed) so the colour key does not
# bleed into edges or mipmaps. Pictures larger than -MaxSize stay with the normal pack.
# Writes up/up16 (PNG) and hd/hd16 (BC7 with mipmaps), replacing what build_texture_pack.ps1 made for them.
#
#   powershell -File tools\build_ui_pack.ps1 -GameDir C:\clusterball [-Hd16]
param([Parameter(Mandatory)][string]$GameDir, [int]$MaxSize = 256, [double]$Factor = 1.12, [switch]$Hd16)

$ErrorActionPreference = 'Continue'
. (Join-Path $PSScriptRoot 'packtools.ps1')
Require-Tool $Esrgan 'Real-ESRGAN'
Require-Tool $Texconv 'texconv'
$bleed = Get-GoTool 'bleed'
$pick = Get-GoTool 'pickmodel'
$tex = Join-Path $GameDir 'textures'
Add-Type -AssemblyName System.Drawing

$work = Join-Path $tex 'stage_ui'
Remove-Item $work -Recurse -Force -ErrorAction SilentlyContinue
$d = @{}
foreach ($n in 'in', 'a', 'b', 'inA', 'inB', 'up', 'up16') { $d[$n] = Join-Path $work $n; New-Item -ItemType Directory -Force $d[$n] | Out-Null }

$files = @()
$hashes = Get-Content (Join-Path $tex 'ui_textures.txt') | ForEach-Object { $_.Trim() } | Where-Object { $_ } | Sort-Object -Unique
foreach ($h in $hashes) {
    $f = Join-Path $tex "dump\$h.png"
    if (-not (Test-Path $f)) { continue }
    $img = [Drawing.Image]::FromFile($f)
    $ok = $img.Width -ge 16 -and $img.Height -ge 16 -and $img.Width -le $MaxSize -and $img.Height -le $MaxSize
    $img.Dispose()
    if ($ok) { $files += $f }
}
& $bleed $d.in @files
function Esr($in, $out, $model) {
    & $Esrgan -i $in -o $out -n $model -m $EsrganModels -f png 2>&1 | Where-Object { $_ -match 'failed' } | ForEach-Object { Write-Warning $_ }
}
Esr $d.in $d.a 'realesrgan-x4plus-anime'
Esr $d.in $d.b 'realesrgan-x4plus'
$choice = & $pick -orig $d.in -a $d.a -b $d.b -factor $Factor
$nb = 0
foreach ($line in $choice) {
    $p = $line -split ' '
    if ($p[1] -eq 'B') { Copy-Item (Join-Path $d.b $p[0]) $d.up; Copy-Item (Join-Path $d.b $p[0]) $d.inB; $nb++ }
    else { Copy-Item (Join-Path $d.a $p[0]) $d.up; Copy-Item (Join-Path $d.a $p[0]) $d.inA }
}
Write-Host "$($files.Count) of $($hashes.Count) listed 2D textures: $($files.Count - $nb) x4plus-anime, $nb x4plus"
# @(, ...) keeps a one-element list of pairs from being flattened into the pair itself.
$pairs = @(, @($d.up, 'up', 'hd'))
if ($Hd16) {
    if (Get-ChildItem $d.inA -Filter *.png) { Esr $d.inA $d.up16 'realesrgan-x4plus-anime' }
    if (Get-ChildItem $d.inB -Filter *.png) { Esr $d.inB $d.up16 'realesrgan-x4plus' }
    $pairs += , @($d.up16, 'up16', 'hd16')
}
foreach ($p in $pairs) {
    $pngs = Get-ChildItem $p[0] -Filter *.png
    New-Item -ItemType Directory -Force (Join-Path $tex $p[1]), (Join-Path $tex $p[2]) | Out-Null
    Copy-Item $pngs.FullName (Join-Path $tex $p[1]) -Force
    $list = Join-Path $work 'list.txt'
    $pngs.FullName | Set-Content -Encoding ascii $list
    # --ignore-srgb: the PNGs carry no colour space; without it texconv darkens them by a gamma conversion
    & $Texconv -nologo --ignore-srgb -f BC7_UNORM -m 0 -y -o (Join-Path $tex $p[2]) -flist $list 2>&1 |
        Where-Object { $_ -match 'FAILED|ERROR' } | ForEach-Object { Write-Warning $_ }
    Write-Host "$($pngs.Count) textures -> $($p[2])"
}
Remove-Item $work -Recurse -Force
Write-Host 'done'
