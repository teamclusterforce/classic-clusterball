# Development tools

What was used to get Clusterball running on Direct3D 11 and to tune it.
Nothing here is needed to play or to build the Command Center.

## gltrace/

The first step: an `opengl32.dll` proxy that logs every OpenGL call of
`Clusterball.exe` and forwards it to the system OpenGL (`trace.c`), with
`scale.c` rendering the game's fixed 640 × 480 OpenGL mode at any window
size. The call log showed which OpenGL subset the renderer in
`modern-renderer/` has to implement. It shares `compat.c` with the renderer:

    cd tools\gltrace
    zig cc -target x86-windows-gnu -shared -O2 -o opengl32.dll trace.c scale.c pass.c ..\..\modern-renderer\compat.c ..\..\modern-renderer\appicon.c -lgdi32 -luser32

Settings: `CB_GLTRACE=1` writes `gltrace.log`, `CB_SHOTS=3,6,10` writes
framebuffer captures to `shots\`.

## glidestub/

A `glide2x.dll` whose 120 exports all return 0 ("no 3dfx hardware"), which
made the game fall back to OpenGL. No longer needed: `compat.c` now makes
`LoadLibraryA("glide2x.dll")` fail, which leads to the same path.
`gen.go` writes `stub.c` from `names.txt`.

## cmd/

Small analysis programs (one Go module, `go run ./cmd/<name> ...`); the
first line of each `main.go` gives its arguments.

| Tool | Purpose |
| ---- | ------- |
| peinfo, iatlookup, glexports | imports, IAT slots and OpenGL exports of `Clusterball.exe` and a proxy DLL |
| disasm, callers | disassemble an address range of `Clusterball.exe`; list the call sites of a function or the users of an IAT slot |
| mrgls | list the entries of a `.mrg` archive |
| bmp2png, crop, contactgrid, sidebyside | turn the renderer's screenshots into comparisons |
| pixdiff, pixdump, colmean | measure differences, pixel values and edge seams |
| atlasfind, cellart, celltab, alphasheet | find and read the game's 8 × 8 font atlas |
| fontgen | development version of the sharp font generator (`launcher/fontatlas.go`) |
| pngmean, pngsize | compare and sort texture pack passes |
| pixscale | Scale2x upscaler for HUD sprites - tried and rejected, kept for comparison |

## scripts/

Test scripts for the renderer's script mode (`CB_SCRIPT=<file>`): one
command per line, `<ms> move dx dy | click | keydown DIK | keyup DIK |
shot name | set key value | quit`. Mouse moves are in game pixels (852 × 480);
from the top-left corner (`move -6000 -6000`) the main menu entries lie at
x 426, y 127 (Training) to 287 (Ende), 32 apart. `dump_venues.ps1` and
`dump_all_worlds.ps1` load venues for texture dumps; they contain the paths
of the development machine. The complete dump run is
`assets\dump_all_menus.ps1`.

In script mode the renderer also watches for hangs: when no frame has been
presented for 5 s, `compat.log` gets the module the game's main thread is in
and the `Clusterball.exe` code addresses on its stack (`hang: ...`); `disasm`
turns those into the code that waits.
