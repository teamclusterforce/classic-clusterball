@echo off
setlocal

set "ROOT=%~dp0"
set "OUT=%ROOT%_release"

if not exist "%OUT%" mkdir "%OUT%"

cd /d "%ROOT%commandcenter"
go build -trimpath -ldflags="-H windowsgui -s -w" -o "%OUT%\ClusterballCC.exe"
if errorlevel 1 (
    echo Build failed.
    exit /b 1
)

echo Build complete: _release\ClusterballCC.exe
