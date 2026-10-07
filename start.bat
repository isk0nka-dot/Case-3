@echo off
echo ==============================================
echo  Argus AI - Local Development Startup
echo ==============================================
echo.
echo Checking for Docker...
docker --version >nul 2>&1
IF %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Docker is not installed or not running!
    echo Please install and start Docker Desktop: https://www.docker.com/products/docker-desktop/
    pause
    exit /b
)

echo Starting Docker containers (this may take a while for the first build)...
docker compose up -d --build

echo.
echo ==============================================
echo  Services are starting!
echo  Dashboard: http://localhost:3000
echo  API:       http://localhost:8080
echo ==============================================
echo.
echo To view logs, run: docker compose logs -f
pause
