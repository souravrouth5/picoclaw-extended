@echo off
setlocal enabledelayedexpansion
echo ==> Installing RE dependencies for Windows

REM Check for winget
where winget >nul 2>&1
if %errorlevel% neq 0 (
    echo winget not found. Please install from Microsoft Store or update Windows.
    echo Manual installs needed:
    echo   Python 3:    https://www.python.org/downloads/
    echo   Java 17:     https://adoptium.net/
    echo   apktool:     https://apktool.org/
    echo   jadx:        https://github.com/skylot/jadx/releases
    echo   adb:         https://developer.android.com/tools/releases/platform-tools
    exit /b 1
)

echo ==> Installing Python 3...
winget install --id Python.Python.3.11 -e --silent --accept-package-agreements --accept-source-agreements

echo ==> Installing Java 17...
winget install --id EclipseAdoptium.Temurin.17.JDK -e --silent --accept-package-agreements --accept-source-agreements

echo ==> Installing Android Platform Tools (adb)...
winget install --id Google.PlatformTools -e --silent --accept-package-agreements --accept-source-agreements

echo ==> Installing pip packages...
python -m pip install --upgrade pip
python -m pip install mcp frida-tools

echo ==> Downloading apktool...
set APKTOOL_VER=2.9.3
set APKTOOL_DIR=%USERPROFILE%\.picoclaw\tools
if not exist "%APKTOOL_DIR%" mkdir "%APKTOOL_DIR%"
curl -L "https://github.com/iBotPeaches/Apktool/releases/download/v%APKTOOL_VER%/apktool_%APKTOOL_VER%.jar" ^
    -o "%APKTOOL_DIR%\apktool.jar"

REM Create apktool.bat wrapper
echo @echo off > "%APKTOOL_DIR%\apktool.bat"
echo java -jar "%APKTOOL_DIR%\apktool.jar" %%* >> "%APKTOOL_DIR%\apktool.bat"

echo ==> Downloading jadx...
set JADX_VER=1.5.0
curl -L "https://github.com/skylot/jadx/releases/download/v%JADX_VER%/jadx-%JADX_VER%.zip" ^
    -o "%TEMP%\jadx.zip"
powershell -Command "Expand-Archive -Path '%TEMP%\jadx.zip' -DestinationPath '%APKTOOL_DIR%\jadx' -Force"

echo.
echo ==> Dependencies installed.
echo     Add to PATH: %APKTOOL_DIR% and %APKTOOL_DIR%\jadx\bin
echo     Connect your Android device via USB with ADB debugging enabled.
echo     Push frida-server to device: see https://frida.re/docs/android/
