# dump_venues.ps1 - loads named training venues once with TextureDump=1 by setting training_venue in config.cfg
param([string[]]$Venues)
$run = 'C:\temp\cfleg\run'
$cfg = "$run\config.cfg"
$log = 'C:\temp\cfleg\scripts\dump_venues.log'
"start $(Get-Date)" | Out-File $log
$orig = Get-Content $cfg
foreach ($v in $Venues) {
    (Get-Content $cfg) -replace '^training_venue .*', "training_venue $v" | Set-Content -Encoding ascii $cfg
    $tag = $v -replace '\W', ''
    $lines = @('6000 move -6000 -6000', '6500 move 426 127', '7000 click', "9500 shot venue_$tag")
    $t = 9500
    for ($r = 0; $r -lt 3; $r++) {
        $t += 1000; $lines += "$t move -6000 -6000"
        $t += 1000; $lines += "$t move 637 397"
        $t += 1000; $lines += "$t click"
    }
    $t += 28000; $lines += "$t keydown 57"
    $t += 500; $lines += "$t keyup 57"
    $t += 6000; $lines += "$t shot venue_${tag}_ingame"
    $t += 500; $lines += "$t quit"
    $script = "C:\temp\cfleg\scripts\venue_$tag.txt"
    $lines | Set-Content -Encoding ascii $script
    $before = (Get-ChildItem "$run\textures\dump" -Filter *.png).Count
    $env:CB_TEXDUMP = '1'; $env:CB_SCRIPT = $script
    $p = Start-Process "$run\Clusterball.exe" -WorkingDirectory $run -PassThru
    $p.WaitForExit(($t + 30000)) | Out-Null
    if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force; $state = 'killed' } else { $state = "exit $($p.ExitCode)" }
    $after = (Get-ChildItem "$run\textures\dump" -Filter *.png).Count
    "venue $v : $state, new textures $($after - $before), total $after" | Out-File $log -Append
}
$orig | Set-Content -Encoding ascii $cfg
Remove-Item env:CB_TEXDUMP, env:CB_SCRIPT -ErrorAction SilentlyContinue
"done $(Get-Date)" | Out-File $log -Append
