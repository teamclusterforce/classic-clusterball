# assets/dump_all_menus.ps1
# Plays through every menu screen, tab and hover state, a 1-minute LAN match and a 1-minute training with
# TextureDump=1, so all textures of the current language - including the text pictures the game composes at run
# time - land in <GameDir>\textures\dump, and everything drawn in 2D is listed in textures\ui_textures.txt.
# Input for build_texture_pack.ps1 and build_ui_pack.ps1. The game is driven by the Direct3D layer's test script
# (CB_SCRIPT: mouse moves in game pixels, clicks, keys as DirectInput codes) and takes about 7 minutes.
# -English: moves Local.mrg aside for the run, the game then uses the English base texts in Data.mrg.
# config.cfg and Local.mrg are restored afterwards.
#
#   powershell -File assets\dump_all_menus.ps1 -GameDir C:\clusterball [-English]
param([Parameter(Mandatory)][string]$GameDir, [switch]$English)

$tag = if ($English) { 'en' } else { 'de' }
$cfg = Join-Path $GameDir 'config.cfg'
$shots = Join-Path $GameDir 'shots'
New-Item -ItemType Directory -Force $shots | Out-Null
$orig = Get-Content $cfg

# A script is built step by step: W waits, A adds a command at the current time.
function New-Script { $script:l = @(); $script:t = 6000 }
function W($ms) { $script:t += $ms }
function A($s) { $script:l += "$($script:t) $s" }
# Moves start from the top-left corner; the game needs a moment before a click registers.
function Hover($x, $y) { A 'move -6000 -6000'; W 400; A "move $x $y"; W 600 }
function C($x, $y) { Hover $x $y; W 200; A 'click'; W 1500 }
# A screen change sometimes swallows a click, so menu entries are clicked twice.
function C2($x, $y) { C $x $y; C $x $y }
function Key($dik) { A "keydown $dik"; W 300; A "keyup $dik"; W 1500 } # 1 = Esc, 57 = Space
function Shot($n) { A "shot ${tag}_$n"; W 300 }

function Run-Game([string]$name) {
    A 'quit'
    $file = Join-Path $shots "dump_$name.txt"
    Set-Content -Encoding ascii $file $script:l
    $before = (Get-ChildItem (Join-Path $GameDir 'textures\dump') -Filter *.png -ErrorAction SilentlyContinue).Count
    $env:CB_SCRIPT = $file; $env:CB_TEXDUMP = '1'
    $p = Start-Process (Join-Path $GameDir 'Clusterball.exe') -WorkingDirectory $GameDir -PassThru
    if (-not $p.WaitForExit($script:t + 60000)) { Stop-Process -Id $p.Id -Force; $state = 'killed' } else { $state = "exit $($p.ExitCode)" }
    $env:CB_SCRIPT = $null; $env:CB_TEXDUMP = $null
    $after = (Get-ChildItem (Join-Path $GameDir 'textures\dump') -Filter *.png).Count
    Write-Host "$tag/$name : $state, $([int]($script:t / 1000)) s, new textures $($after - $before), total $after"
}

if ($English) { Rename-Item (Join-Path $GameDir 'Local.mrg') 'Local.mrg.off' }
try {
    # LAN host (an Internet host asks for a login), short matches
    (Get-Content $cfg) -replace '^host_type .*', 'host_type 1.000000' -replace '^host_timelimit .*', 'host_timelimit 1.000000' `
        -replace '^host_maxplayers .*', 'host_maxplayers 2.000000' -replace '^training_timelimit .*', 'training_timelimit 1.000000' |
        Set-Content -Encoding ascii $cfg

    # Game coordinates (852 x 480): main menu entries at x 426, y 127 (Training) to 287 (Ende), 32 apart.
    New-Script
    foreach ($y in 127, 159, 191, 223, 255, 287) { Hover 426 $y; W 400 }          # main menu hover states
    C2 426 127; C 468 110; C 382 110; Key 1                                       # training: Piloten, Setup tabs
    C2 426 191; C 337 110; C 426 110; C 511 110; Hover 426 379; W 400; Hover 637 397; W 400; Key 1  # join tabs
    C2 426 223; Hover 549 290; W 400; Hover 646 290; W 400; Key 1                 # saved games buttons
    C2 426 255; C 293 110; C 382 110; C 469 110; C 556 110; Hover 488 292; W 400; Key 1  # setup tabs
    C2 426 159; C 637 397; W 1000                                                 # host -> waiting room
    C 261 82; C 351 82; C 171 82; C2 706 233; W 8000                              # its tabs, start
    Key 57; W 5000; Key 1; Shot 'esc'; Key 1; W 70000; Shot 'matchend'; W 5000    # ESC menu, match end
    Shot 'board'; C 261 82; C 351 82                                              # score board tabs
    Run-Game 'menus'

    New-Script
    C2 426 127; C2 637 397; W 25000; Key 57; W 70000; Shot 'trainend'; W 5000; Shot 'trainboard'
    Run-Game 'training'
} finally {
    $orig | Set-Content -Encoding ascii $cfg
    if ($English) { Rename-Item (Join-Path $GameDir 'Local.mrg.off') 'Local.mrg' }
}
