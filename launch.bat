@echo off

set CLIENTS=3

echo Select launch mode:
echo   1) Normal
echo   2) Debug (server runs under Delve on :2345, attach GoLand Go Remote)
echo.
set /p MODE_CHOICE="Enter 1 or 2: "

if "%MODE_CHOICE%"=="2" goto debug
if "%MODE_CHOICE%"=="1" goto normal
echo Invalid choice. Exiting.
pause
exit /b 1

:normal
echo Building...
go build -o chat.exe .
if %errorlevel% neq 0 (
    echo Build failed
    pause
    exit /b 1
)

echo Launching server...
start "Server" cmd /k chat.exe -mode server

timeout /t 1 /nobreak >nul

for /l %%i in (1,1,%CLIENTS%) do (
    echo Launching client %%i...
    start "Client %%i" cmd /k chat.exe -mode client
)

echo Done. Server + %CLIENTS% clients launched.
goto end

:debug
echo Building with debug info...
go build -gcflags="all=-N -l" -o chat.exe .
if %errorlevel% neq 0 (
    echo Build failed
    pause
    exit /b 1
)

echo Launching server under Delve (listening on :2345)...
echo Attach GoLand via Run ^> Edit Configurations ^> Go Remote ^> localhost:2345
start "Server [DEBUG]" cmd /k dlv exec chat.exe --headless --listen=:2345 --api-version=2 -- -mode server

timeout /t 1 /nobreak >nul

for /l %%i in (1,1,%CLIENTS%) do (
    echo Launching client %%i...
    start "Client %%i" cmd /k chat.exe -mode client
)

echo Done. Server waiting for debugger on :2345. Clients running normally.

:end
