# assets/build_scaled_pack.ps1
# Builds a texture pack for a list of textures at another size than the 4x pack: each texture at -Factor x its
# original, no edge above -Cap, resampled from the 4x and 16x upscales (assets\cmd\scalepack), then BC7 with mipmaps.
# Used for the ClusterForce base pack of the ported levels (8x, 2048 px). Textures without a 16x upscale need one
# first (build_texture_pack.ps1 -Hd16, or Real-ESRGAN on their up\ file).
#
#   powershell -File assets\build_scaled_pack.ps1 -GameDir C:\temp\cfleg\run -List hashes.txt -Factor 8 -Cap 2048 -Name hd8
param([Parameter(Mandatory)][string]$GameDir, [Parameter(Mandatory)][string]$List, [double]$Factor = 8,
      [int]$Cap = 2048, [Parameter(Mandatory)][string]$Name)

$ErrorActionPreference = 'Continue'
. (Join-Path $PSScriptRoot 'packtools.ps1')
Require-Tool $Texconv 'texconv'
$tex = Join-Path $GameDir 'textures'
$png = Join-Path $tex "stage_$Name"
$out = Join-Path $tex $Name
$scale = Get-GoTool 'scalepack'
& $scale -tex $tex -list $List -factor $Factor -cap $Cap -out $png
New-Item -ItemType Directory -Force $out | Out-Null
$todo = @(Get-ChildItem $png -Filter *.png)
$flist = Join-Path $tex 'texconv_list.txt'
for ($i = 0; $i -lt $todo.Count; $i += 100) {
    $todo[$i..([Math]::Min($i + 99, $todo.Count - 1))].FullName | Set-Content -Encoding ascii $flist
    # --ignore-srgb: the PNGs carry no colour space; without it texconv darkens them by a gamma conversion
    & $Texconv -nologo --ignore-srgb -f BC7_UNORM -m 0 -y -o $out -flist $flist 2>&1 |
        Where-Object { $_ -match 'FAILED|ERROR' } | ForEach-Object { Write-Warning $_ }
}
Remove-Item $flist -ErrorAction SilentlyContinue
Remove-Item $png -Recurse -Force
Write-Host "$($todo.Count) textures -> $out"
