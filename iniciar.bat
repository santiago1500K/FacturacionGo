@echo off
REM Inicia los 4 microservicios y el frontend, cada uno en su propia ventana.
cd /d "%~dp0"

start "Auth :8081"      cmd /k "cd auth && go run ."
start "Productos :8082" cmd /k "cd productos && go run ."
start "Clientes :8083"  cmd /k "cd clientes && go run ."
start "Facturas :8084"  cmd /k "cd facturas && go run ."
start "Frontend :8080"  cmd /k "cd frontend && go run ."

echo Esperando a que compilen los servicios...
timeout /t 8 /nobreak >nul
start http://localhost:8080
