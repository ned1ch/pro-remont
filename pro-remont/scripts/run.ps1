# Запуск dev-окружения: локальная PostgreSQL + Go-сервер.
[CmdletBinding()]
param(
    [switch]$NoDb,
    [int]$Port = 55432,
    [string]$Addr = "127.0.0.1:8080"
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $ProjectRoot

try {
    if (-not (Test-Path ".env")) {
        if ($NoDb) {
            Copy-Item ".env.example" ".env"
            Write-Host "Создан .env из .env.example — проверьте DATABASE_URL." -ForegroundColor Yellow
        } else {
            $secret = -join ((48..57) + (65..90) + (97..122) | Get-Random -Count 48 | ForEach-Object { [char]$_ })
            $lines = @(
                "APP_ADDR=$Addr",
                "DATABASE_URL=postgres://postgres@127.0.0.1:$Port/pro_remont?sslmode=disable",
                "JWT_SECRET=$secret",
                "TOKEN_TTL=24h",
                "WEB_DIR=web",
                "SEED_DEMO=true",
                "LOG_LEVEL=info"
            )
            Set-Content -LiteralPath ".env" -Value ($lines -join "`r`n") -Encoding ASCII
            Write-Host "Создан .env для локальной базы." -ForegroundColor Green
        }
    }

    if (-not $NoDb) {
        & (Join-Path $PSScriptRoot "dev-db.ps1") -Port $Port
    }

    $serverExe = Join-Path $ProjectRoot "build\server.exe"
    $useBinary = $false
    if (Test-Path $serverExe) {
        $sources = Get-ChildItem -Path (Join-Path $ProjectRoot "cmd"), (Join-Path $ProjectRoot "internal") -Recurse -File -ErrorAction SilentlyContinue |
            Where-Object { $_.Extension -eq ".go" -or $_.Extension -eq ".sql" }
        $newest = $sources | Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1
        if (-not $newest -or (Get-Item $serverExe).LastWriteTimeUtc -ge $newest.LastWriteTimeUtc) {
            $useBinary = $true
        }
    }

    Write-Host ""
    Write-Host "Сервер: http://$Addr" -ForegroundColor Cyan
    Write-Host "Демо-вход: client@example.ru / Client123! (см. README)" -ForegroundColor Cyan
    Write-Host "Остановка — Ctrl+C" -ForegroundColor DarkGray
    Write-Host ""

    if ($useBinary) {
        Write-Host "Запуск собранного сервера build\server.exe ..." -ForegroundColor DarkGray
        & $serverExe
    } else {
        $go = Get-Command go -ErrorAction SilentlyContinue
        if (-not $go) { throw "Go не найден в PATH. Установите Go 1.26+ или соберите build\server.exe." }
        & go run ./cmd/server
    }
} finally {
    Pop-Location
}
