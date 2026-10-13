# build_cf_audio.ps1 -CF <ClusterForce workspace>
# Builds the launcher's ClusterForce audio from the ClusterForce masters:
#  - launcher\audio\music\<Venue>.mp3, Outro.mp3: a 30-second excerpt of the matching ClusterForce track, looping
#    seamlessly (the last two seconds are faded into the first two), loudness-normalised, MP3 128 kbit/s, 44.1 kHz
#    stereo like the game's own music. The game plays music only in the menus and while a venue loads, so 30 seconds
#    are enough; only these shortened versions go into the repository, never the masters.
#  - launcher\audio\music\Intro.mp3: the menu music, the whole song of the ClusterForce teaser, MP3 128 kbit/s.
#  - launcher\audio\ball\<variant>.wav: the three ball pickup sounds ClusterForce offers, in the format of the game's
#    own announcer clips such as Ball.wav: 22050 Hz mono 16-bit PCM with a plain 44-byte header.
# Needs ffmpeg on the PATH.
param([string]$CF = (Join-Path ([Environment]::GetFolderPath('Desktop')) 'CF'))
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$masters = Join-Path $CF 'ideas\Music'
$sounds = Join-Path $CF 'clusterforce-client\client\assets\sounds'
$musicOut = Join-Path $root 'launcher\audio\music'
$ballOut = Join-Path $root 'launcher\audio\ball'
New-Item -ItemType Directory -Force $musicOut, $ballOut | Out-Null

# Game file (without .mp3) -> ClusterForce master. The Outro starts at the beginning of its track, the venues a
# quarter into it, past the build-up. The Intro is not cut, see below.
$music = [ordered]@{
    'Outro'           = 'Outro'
    'Antarctica'      = 'Whiteout'
    'Bora Bora'       = 'Ocean Boulevard'
    'China'           = 'Light Dress'
    'Easter Island'   = 'Megalith Hypotheses'
    'Egal'            = 'Energetic Heaven'
    'Egypt'           = 'Sphinx'
    'Electronica'     = 'Electricity'
    'Gobi'            = 'Agitated Cargo'
    'Green Garden V2' = 'Green Garden Synthesis'
    'Hangarball'      = 'Hangarball'
    'HelterSkelter'   = 'HelterSkelter'
    'Lunar'           = 'Trabant'
    'Matterhorn'      = 'Matterhorn'
    'Metropolis'      = 'Metropolis'
    'Pole Position'   = 'Ballrace1'
    'Ruhrmansk'       = 'Industrial Wreckfest'
    'ShootLoop'       = 'ShootLoop'
    'Stonehenge'      = 'Regenstein Atmosphere'
    'Taj Mahal'       = 'Crescent Contrast'
    'Tuanake'         = 'Tuanake'
    'Tutorial'        = 'Giza Beatz'
    'Yucatan'         = 'Sensitive Target'
}
$len = 30   # seconds in the file
$xf = 2     # crossfade at the loop point

foreach ($game in $music.Keys) {
    $src = Join-Path $masters ($music[$game] + '.wav')
    if (-not (Test-Path $src)) { throw "missing master: $src" }
    $dur = [double](& ffprobe -v error -show_entries format=duration -of csv=p=0 $src)
    $start = if ($game -eq 'Outro') { 0 } else { [math]::Round($dur * 0.25, 1) }
    $total = $len + $xf
    $graph = "[0:a]asplit=3[a][b][c];" +
        "[a]atrim=0:$xf,asetpts=PTS-STARTPTS,afade=t=in:d=$xf[head];" +
        "[b]atrim=${len}:${total},asetpts=PTS-STARTPTS,afade=t=out:d=$xf[tail];" +
        "[c]atrim=${xf}:${len},asetpts=PTS-STARTPTS[body];" +
        "[head][tail]amix=inputs=2:normalize=0[x];" +
        "[x][body]concat=n=2:v=0:a=1,loudnorm=I=-16:TP=-1.5,aresample=44100,asetpts=N/SR/TB[out]"
    $dst = Join-Path $musicOut "$game.mp3"
    & ffmpeg -hide_banner -loglevel error -y -ss $start -t $total -i $src -filter_complex $graph -map '[out]' `
        -map_metadata -1 -c:a libmp3lame -b:a 128k -ar 44100 -ac 2 $dst
    if ($LASTEXITCODE) { throw "ffmpeg failed for $game" }
    "{0,-16} <- {1} (from {2} s)" -f $game, $music[$game], $start
}


# The menu music: the whole song of the ClusterForce teaser (trailer-cut.cmd), not cut, loudness-normalised like the
# rest, MP3 128 kbit/s.
$teaser = Join-Path $CF '_Import\clusterforce (1).wav'
if (-not (Test-Path $teaser)) { throw "missing teaser song: $teaser" }
& ffmpeg -hide_banner -loglevel error -y -i $teaser -af 'loudnorm=I=-16:TP=-1.5,aresample=44100,asetpts=N/SR/TB' `
    -map_metadata -1 -c:a libmp3lame -b:a 128k -ar 44100 -ac 2 (Join-Path $musicOut 'Intro.mp3')
if ($LASTEXITCODE) { throw 'ffmpeg failed for Intro' }
'Intro            <- teaser song (whole)'
# Ball pickup sounds: ClusterForce's three sets (CF-2193). The announcer call exists from two speakers.
$balls = [ordered]@{
    'standard'   = 'balls\ball_ground.wav'
    'computer'   = 'balls\pickup_01.wav'
    'announcer1' = 'announcer\1\en\ball_1.wav'
    'announcer2' = 'announcer\2\en\ball_1.wav'
}
foreach ($name in $balls.Keys) {
    $src = Join-Path $sounds $balls[$name]
    $dst = Join-Path $ballOut "$name.wav"
    & ffmpeg -hide_banner -loglevel error -y -i $src -map_metadata -1 -fflags +bitexact -c:a pcm_s16le -ar 22050 -ac 1 $dst
    if ($LASTEXITCODE) { throw "ffmpeg failed for $name" }
    "{0,-16} <- {1}" -f $name, $balls[$name]
}
