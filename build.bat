@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"
title Typhon - сборка

echo.
echo  ==============================
echo   Typhon - сборка приложения
echo  ==============================
echo.

where go >nul 2>&1
if errorlevel 1 (
    echo  [ОШИБКА] Go не найден в PATH.
    echo  Установи Go: https://go.dev/dl/
    goto :fail
)

for /f "delims=" %%G in ('go env GOPATH') do set "GOPATH_BIN=%%G\bin"
set "PATH=%PATH%;%GOPATH_BIN%"

where npm >nul 2>&1
if errorlevel 1 (
    echo  [ОШИБКА] npm не найден в PATH.
    echo  Установи Node.js: https://nodejs.org/
    goto :fail
)

where wails3 >nul 2>&1
if errorlevel 1 (
    echo  wails3 не найден. Поставить его сейчас? Это займёт пару минут.
    choice /c YN /n /m "  [Y] да  [N] нет: "
    if errorlevel 2 goto :fail
    echo.
    echo  Ставлю wails3...
    go install github.com/wailsapp/wails/v3/cmd/wails3@latest
    if errorlevel 1 goto :fail
    where wails3 >nul 2>&1
    if errorlevel 1 (
        echo  [ОШИБКА] wails3 поставился, но не виден в PATH: %GOPATH_BIN%
        goto :fail
    )
)

tasklist /fi "imagename eq typhon.exe" 2>nul | find /i "typhon.exe" >nul
if not errorlevel 1 (
    echo  Лаунчер сейчас запущен, бинарь занят и не перезапишется.
    choice /c YN /n /m "  Закрыть его?  [Y] да  [N] отмена: "
    if errorlevel 2 goto :fail
    taskkill /im typhon.exe /f >nul 2>&1
    echo  Закрыл.
    echo.
)

where cat >nul 2>&1
if errorlevel 1 (
    for /f "delims=" %%G in ('where git 2^>nul') do call :add_unix_tools "%%~dpG"
)
where cat >nul 2>&1
if errorlevel 1 if exist "%ProgramFiles%\Git\usr\bin\cat.exe" set "PATH=%PATH%;%ProgramFiles%\Git\usr\bin"
where cat >nul 2>&1
if errorlevel 1 (
    echo  [ОШИБКА] Не найден cat.exe: сборка wails читает им файл VERSION.
    echo  Он входит в Git for Windows: https://git-scm.com/download/win
    goto :fail
)

echo  Собираю. Первый запуск дольше: тянутся зависимости фронта.
echo.
call wails3 task build
if errorlevel 1 (
    echo.
    echo  [ОШИБКА] Сборка упала. Смотри вывод выше.
    goto :fail
)

if not exist "bin\typhon.exe" (
    echo.
    echo  [ОШИБКА] Сборка прошла, но bin\typhon.exe не появился.
    goto :fail
)

for %%F in ("bin\typhon.exe") do set "EXE_SIZE=%%~zF"
set /a EXE_MB=%EXE_SIZE% / 1048576

echo.
echo  ==============================
echo   Готово: %CD%\bin\typhon.exe
echo   Размер: %EXE_MB% МБ
echo  ==============================
echo.
choice /c YN /n /m "  Запустить лаунчер?  [Y] да  [N] нет: "
if errorlevel 2 goto :done
start "" "bin\typhon.exe"

:done
echo.
pause
exit /b 0

:fail
echo.
pause
exit /b 1

:add_unix_tools
set "_gitdir=%~1"
set "_gitdir=%_gitdir:~0,-1%"
for %%P in ("%_gitdir%") do set "_gitroot=%%~dpP"
if exist "%_gitroot%usr\bin\cat.exe" set "PATH=%PATH%;%_gitroot%usr\bin"
exit /b 0
