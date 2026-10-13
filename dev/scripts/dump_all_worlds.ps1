# dump_all_worlds.ps1 - loads every training world once with TextureDump=1 (test driver, one game process per world)
param([int]$From = 0, [int]$To = 23, [int[]]$Only)
$run = 'C:\temp\cfleg\run'
$log = 'C:\temp\cfleg\scripts\dump_all_worlds.log'
"start $(Get-Date)" | Out-File $log
$list = if ($Only) { $Only } else { $From..$To }
foreach ($k in $list) {
    # the main menu re-centres its cursor when it appears: wait for it, then corner, target, click (twice)
    $lines = @('6000 move -6000 -6000', '6500 move 426 127', '7000 click', '9500 move -6000 -6000', '10000 move 502 150')
    $t = 11000
    for ($i = 0; $i -lt $k; $i++) { $lines += "$t click"; $t += 1500 }
    $t += 1000; $lines += "$t shot world_$k"
    for ($r = 0; $r -lt 3; $r++) {
        $t += 1000; $lines += "$t move -6000 -6000"
        $t += 1000; $lines += "$t move 637 397"
        $t += 1000; $lines += "$t click"
    }
    $t += 28000; $lines += "$t keydown 57"
    $t += 500; $lines += "$t keyup 57"
    $t += 6000; $lines += "$t shot world_${k}_ingame"
    $t += 500; $lines += "$t quit"
    $script = "C:\temp\cfleg\scripts\world_$k.txt"
    $lines | Set-Content -Encoding ascii $script
    $before = (Get-ChildItem "$run\textures\dump" -Filter *.png).Count
    $env:CB_TEXDUMP = '1'; $env:CB_SCRIPT = $script
    $p = Start-Process "$run\Clusterball.exe" -WorkingDirectory $run -PassThru
    $p.WaitForExit(($t + 30000)) | Out-Null
    if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force; $state = 'killed' } else { $state = "exit $($p.ExitCode)" }
    $after = (Get-ChildItem "$run\textures\dump" -Filter *.png).Count
    "world $k : $state, new textures $($after - $before), total $after" | Out-File $log -Append
}
Remove-Item env:CB_TEXDUMP, env:CB_SCRIPT -ErrorAction SilentlyContinue
"done $(Get-Date)" | Out-File $log -Append
