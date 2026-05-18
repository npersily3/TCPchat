@echo off

set CLIENTS=3

echo Select launch mode:
echo   1) Normal
echo   2) Debug server only   (Delve on :2345)
echo   3) Debug client 1 only (Delve on :2346)
echo   4) Debug both          (server :2345, client 1 :2346)
echo.
set /p MODE_CHOICE="Enter 1, 2, 3, or 4: "

if "%MODE_CHOICE%"=="1" goto normal
if "%MODE_CHOICE%"=="2" goto debug_server
if "%MODE_CHOICE%"=="3" goto debug_client
if "%MODE_CHOICE%"=="4" goto debug_both
echo Invalid choice. Exiting.
pause
exit /b 1

:normal
echo Building...
go build -o chat.exe .
if %errorlevel% neq 0 ( echo Build failed & pause & exit /b 1 )

echo Launching server...
start "Server" cmd /k chat.exe -mode server
timeout /t 1 /nobreak >nul

for /l %%i in (1,1,%CLIENTS%) do (
    start "Client %%i" cmd /k chat.exe -mode client
)
echo Done. Server + %CLIENTS% clients launched.
goto end

:debug_server
echo Building with debug info...
go build -gcflags="all=-N -l" -o chat.exe .
if %errorlevel% neq 0 ( echo Build failed & pause & exit /b 1 )

echo Launching server under Delve on :2345...
start "Server [DEBUG :2345]" cmd /k dlv exec chat.exe --headless --listen=:2345 --api-version=2 -- -mode server
timeout /t 1 /nobreak >nul

for /l %%i in (1,1,%CLIENTS%) do (
    start "Client %%i" cmd /k chat.exe -mode client
)
echo Done. Attach GoLand Go Remote to localhost:2345 for server.
goto end

:debug_client
echo Building with debug info...
go build -gcflags="all=-N -l" -o chat.exe .
if %errorlevel% neq 0 ( echo Build failed & pause & exit /b 1 )

echo Launching server normally...
start "Server" cmd /k chat.exe -mode server
timeout /t 1 /nobreak >nul

echo Launching client 1 under Delve on :2346...
start "Client 1 [DEBUG :2346]" cmd /k dlv exec chat.exe --headless --listen=:2346 --api-version=2 -- -mode client

for /l %%i in (2,1,%CLIENTS%) do (
    start "Client %%i" cmd /k chat.exe -mode client
)
echo Done. Attach GoLand Go Remote to localhost:2346 for client 1.
goto end

:debug_both
echo Building with debug info...
go build -gcflags="all=-N -l" -o chat.exe .
if %errorlevel% neq 0 ( echo Build failed & pause & exit /b 1 )

echo Launching server under Delve on :2345...
start "Server [DEBUG :2345]" cmd /k dlv exec chat.exe --headless --listen=:2345 --api-version=2 -- -mode server
timeout /t 1 /nobreak >nul

echo Launching client 1 under Delve on :2346...
start "Client 1 [DEBUG :2346]" cmd /k dlv exec chat.exe --headless --listen=:2346 --api-version=2 -- -mode client

for /l %%i in (2,1,%CLIENTS%) do (
    start "Client %%i" cmd /k chat.exe -mode client
)
echo Done. Attach GoLand Go Remote to localhost:2345 (server) and localhost:2346 (client 1).

:end
