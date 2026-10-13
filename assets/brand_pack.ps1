# assets/brand_pack.ps1
# Puts the Clusterball Re-Enhanced and Team ClusterForce marks into the texture packs: assets\cmd\brand draws them
# into the upscaled menu textures (<GameDir>\textures\up, up16 -> textures\brand\up, brand\up16), texconv
# compresses those into textures\hd and hd16. It also draws the arenas' billboards anew, at the size the pack has.
# Run it after build_texture_pack.ps1 / build_ui_pack.ps1.
#
#   powershell -File assets\brand_pack.ps1 -GameDir C:\clusterball
param([Parameter(Mandatory)][string]$GameDir)

$ErrorActionPreference = 'Continue'
. (Join-Path $PSScriptRoot 'packtools.ps1')
Require-Tool $Texconv 'texconv'
$brand = Get-GoTool 'brand'
& $brand -game $GameDir
$tex = Join-Path $GameDir 'textures'
foreach ($p in @(@('up', 'hd'), @('up16', 'hd16'))) {
    $pngs = @(Get-ChildItem (Join-Path $tex "brand\$($p[0])") -Filter *.png -ErrorAction SilentlyContinue)
    if ($pngs.Count -eq 0) { continue }
    $list = Join-Path $tex 'brand\list.txt'
    $pngs.FullName | Set-Content -Encoding ascii $list
    # --ignore-srgb: the PNGs carry no colour space; without it texconv darkens them by a gamma conversion
    & $Texconv -nologo --ignore-srgb -f BC7_UNORM -m 0 -y -o (Join-Path $tex $p[1]) -flist $list 2>&1 |
        Where-Object { $_ -match 'FAILED|ERROR' } | ForEach-Object { Write-Warning $_ }
    Remove-Item $list
    Write-Host "$($pngs.Count) branded textures -> $($p[1])"
}
