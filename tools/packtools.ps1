# tools/packtools.ps1
# Shared paths for the texture pack scripts. Third-party programs live in tools\bin (not in the repository):
#   bin\esrgan\realesrgan-ncnn-vulkan.exe + bin\esrgan\models\  (Real-ESRGAN ncnn Vulkan, release zip)
#   bin\texconv.exe                                            (DirectXTex texconv)
# bleed.exe and pickmodel.exe are built from tools\cmd on first use.

$ToolsBin = Join-Path $PSScriptRoot 'bin'
$Esrgan = Join-Path $ToolsBin 'esrgan\realesrgan-ncnn-vulkan.exe'
$EsrganModels = Join-Path $ToolsBin 'esrgan\models'
$Texconv = Join-Path $ToolsBin 'texconv.exe'

function Require-Tool([string]$path, [string]$what) {
    if (-not (Test-Path $path)) { throw "$what not found: $path (see README, section Texture packs)" }
}

# Builds tools\bin\<name>.exe from tools\cmd\<name> and returns its path. Always built (Go's cache makes that
# quick): a tool left over from an older checkout would silently do what the old source did.
function Get-GoTool([string]$name) {
    $exe = Join-Path $ToolsBin "$name.exe"
    New-Item -ItemType Directory -Force $ToolsBin | Out-Null
    Push-Location $PSScriptRoot
    try { go build -o $exe "./cmd/$name" } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $exe)) { throw "building $name failed" }
    return $exe
}
