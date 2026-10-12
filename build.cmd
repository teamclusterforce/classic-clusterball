@echo off
rem build.cmd [output directory]
rem Builds the Direct3D layer (layer\ -> commandcenter\layer\opengl32.dll, needs Zig) and the
rem Command Center, which embeds the layer. Without Zig the committed opengl32.dll is used.
rem Default output directory: _release
setlocal

set "ROOT=%~dp0"
set "OUT=%~1"
if "%OUT%"=="" set "OUT=%ROOT%_release"

if not exist "%OUT%" mkdir "%OUT%"

where zig >nul 2>nul
if errorlevel 1 (
    echo Zig not found - using the committed commandcenter\layer\opengl32.dll.
) else (
    if not exist "%ROOT%layer\_build" mkdir "%ROOT%layer\_build"
    cd /d "%ROOT%layer"
    zig cc -target x86-windows-gnu -shared -O2 -o _build\opengl32.dll gld3d.c pass.c compat.c -ld3d11 -ldxgi -ldxguid -lgdi32 -luser32
    if errorlevel 1 (
        echo Layer build failed.
        exit /b 1
    )
    copy /y _build\opengl32.dll "%ROOT%commandcenter\layer\opengl32.dll" >nul
    echo Layer built: commandcenter\layer\opengl32.dll
)

cd /d "%ROOT%commandcenter"
go build -trimpath -ldflags="-H windowsgui -s -w" -o "%OUT%\ClusterballCC.exe"
if errorlevel 1 (
    echo Build failed.
    exit /b 1
)

echo Build complete: %OUT%\ClusterballCC.exe
