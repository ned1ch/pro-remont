# Локальный учебный кластер PostgreSQL для проекта pro-remont.
# Запускать из обычного (НЕ elevated) терминала: initdb отказывается работать
# от имени пользователя, входящего в группу администраторов.
[CmdletBinding()]
param(
    [int]$Port = 55432,
    [string]$DataDir = "",
    [string]$DbName = "pro_remont",
    [string]$PgBin = ""
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot

if (-not $DataDir) { $DataDir = Join-Path $ProjectRoot ".pgdata" }
if ($DbName -notmatch '^[A-Za-z0-9_]+$') { throw "Недопустимое имя базы: $DbName" }

if (-not $PgBin) {
    $cmd = Get-Command pg_ctl.exe -ErrorAction SilentlyContinue
    if ($cmd) {
        $PgBin = Split-Path $cmd.Source -Parent
    } else {
        $roots = @("C:\Program Files\PostgreSQL", "C:\Program Files (x86)\PostgreSQL")
        foreach ($root in $roots) {
            if (-not (Test-Path $root)) { continue }
            $versions = Get-ChildItem $root -Directory | Sort-Object Name -Descending
            foreach ($v in $versions) {
                $candidate = Join-Path $v.FullName "bin"
                if (Test-Path (Join-Path $candidate "pg_ctl.exe")) {
                    $PgBin = $candidate
                    break
                }
            }
            if ($PgBin) { break }
        }
    }
}

if (-not $PgBin -or -not (Test-Path (Join-Path $PgBin "pg_ctl.exe"))) {
    throw "PostgreSQL не найден. Установите PostgreSQL 14+ или укажите -PgBin 'C:\Program Files\PostgreSQL\18\bin'."
}

$pgCtl = Join-Path $PgBin "pg_ctl.exe"
$initdb = Join-Path $PgBin "initdb.exe"
$createdb = Join-Path $PgBin "createdb.exe"
$psql = Join-Path $PgBin "psql.exe"
$pgIsReady = Join-Path $PgBin "pg_isready.exe"
$logFile = Join-Path $ProjectRoot ".pg.log"

if (-not (Test-Path (Join-Path $DataDir "PG_VERSION"))) {
    Write-Host "Инициализация кластера PostgreSQL в $DataDir ..."
    New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
    & $initdb -D $DataDir -U postgres -A trust -E UTF8 --locale=C
    if ($LASTEXITCODE -ne 0) {
        throw "initdb завершился с ошибкой. Запустите скрипт из терминала без прав администратора."
    }
}

& $pgIsReady -h 127.0.0.1 -p $Port -U postgres *> $null
if ($LASTEXITCODE -ne 0) {
    Write-Host "Запуск PostgreSQL на порту $Port ..."
    & $pgCtl -D $DataDir -l $logFile -o "-p $Port -c listen_addresses=127.0.0.1" start
    if ($LASTEXITCODE -ne 0) {
        if (Test-Path $logFile) { Get-Content $logFile -Tail 20 }
        throw "Не удалось запустить PostgreSQL. Проверьте, не занят ли порт $Port."
    }
}

$ready = $false
for ($i = 0; $i -lt 30; $i++) {
    & $pgIsReady -h 127.0.0.1 -p $Port -U postgres *> $null
    if ($LASTEXITCODE -eq 0) { $ready = $true; break }
    Start-Sleep -Milliseconds 500
}
if (-not $ready) { throw "PostgreSQL не отвечает на 127.0.0.1:$Port" }

$exists = [string](& $psql -h 127.0.0.1 -p $Port -U postgres -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = '$DbName'")
$exists = $exists.Trim()
if ($exists -ne "1") {
    Write-Host "Создание базы $DbName ..."
    & $createdb -h 127.0.0.1 -p $Port -U postgres $DbName
    if ($LASTEXITCODE -ne 0) { throw "Не удалось создать базу $DbName" }
}

Write-Host ""
Write-Host "PostgreSQL готов." -ForegroundColor Green
Write-Host "DATABASE_URL=postgres://postgres@127.0.0.1:$Port/${DbName}?sslmode=disable"
Write-Host "Лог: $logFile"
