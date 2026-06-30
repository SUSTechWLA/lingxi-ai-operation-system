<#
.SYNOPSIS
    一键启动躺营 AIOS 所有服务（云后端 + 标书工具 + 前端）
.DESCRIPTION
    按顺序启动：
      1. Docker Compose（PostgreSQL / Redis / Kafka / MinIO）
      2. 编译并启动云后端 (port 8080)
      3. 安装依赖并启动标书工具 (port 9001)
      4. 安装依赖并启动前端 (port 3000)
      5. 注册标书外部工具到云后端
#>
param(
    [switch]$SkipDocker,
    [switch]$SkipBiaoshu,
    [switch]$SkipFrontend,
    [string]$ApiKey = $env:OPENAI_API_KEY,
    [string]$BaseUrl = $env:OPENAI_BASE_URL,
    [string]$Model    = $env:OPENAI_MODEL
)

$ErrorActionPreference = "Stop"
$ROOT = Split-Path -Parent $PSCommandPath

# ─── 环境变量默认值 ──────────────────────────────────
if (-not $ApiKey)    { $ApiKey  = "sk-your-key-here" }
if (-not $BaseUrl)   { $BaseUrl = "https://api.deepseek.com" }
if (-not $Model)     { $Model   = "deepseek-v4-pro" }

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  躺营 AIOS 一键启动"                     -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

# ─── 1. Docker Compose ────────────────────────────────
if (-not $SkipDocker) {
    Write-Host ""
    Write-Host "[1/5] Starting Docker Compose..." -ForegroundColor Yellow
    Push-Location "$ROOT\cloud-backend"
    try {
        if (-not (Test-Path .env)) {
            Copy-Item .env.example .env
            Write-Host "  created .env from .env.example" -ForegroundColor DarkGray
        }
        docker compose up -d 2>&1 | Out-Null
        if ($LASTEXITCODE -ne 0) {
            Write-Host "  WARNING: docker compose failed - is Docker Desktop running?" -ForegroundColor Red
            Write-Host "  Use -SkipDocker if you do not need it." -ForegroundColor DarkGray
        } else {
            Write-Host "  docker compose up -d  done" -ForegroundColor Green
        }
    } finally {
        Pop-Location
    }
}

# ─── 2. Cloud Backend ────────────────────────────────
Write-Host ""
Write-Host "[2/5] Building cloud-backend..." -ForegroundColor Yellow
Push-Location "$ROOT\cloud-backend"
try {
    go build -o build\tangying-ai-os.exe .\cmd\tangying-ai-os\
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    Write-Host "  build done" -ForegroundColor Green
} finally {
    Pop-Location
}

Write-Host "  Starting cloud-backend on :8080 ..." -ForegroundColor Yellow
$env:AGENT_PLANNER_MODE = "llm"
$cloudProc = Start-Process -FilePath "$ROOT\cloud-backend\build\tangying-ai-os.exe" `
    -WorkingDirectory "$ROOT\cloud-backend" `
    -WindowStyle Minimized `
    -PassThru

Write-Host "  cloud-backend PID=$($cloudProc.Id)" -ForegroundColor Green

# ─── 3. Biaoshu Tools ─────────────────────────────────
if (-not $SkipBiaoshu) {
    Write-Host ""
    Write-Host "[3/5] Installing biaoshu-tools dependencies..." -ForegroundColor Yellow
    Push-Location "$ROOT\biaoshu-tools"
    try {
        pip install -r requirements.txt -q 2>&1 | Out-Null
        Write-Host "  pip install done" -ForegroundColor Green
    } finally {
        Pop-Location
    }

    Write-Host "  Starting biaoshu-tools on :9001 ..." -ForegroundColor Yellow

    $env:OPENAI_API_KEY  = $ApiKey
    $env:OPENAI_BASE_URL = $BaseUrl
    $env:OPENAI_MODEL    = $Model

    $biaoshuProc = Start-Process -FilePath "python" `
        -ArgumentList "app.py","--port","9001" `
        -WorkingDirectory "$ROOT\biaoshu-tools" `
        -WindowStyle Minimized `
        -PassThru

    Write-Host "  biaoshu-tools PID=$($biaoshuProc.Id)" -ForegroundColor Green
}

# ─── 4. Frontend ──────────────────────────────────────
if (-not $SkipFrontend) {
    Write-Host ""
    Write-Host "[4/5] Installing frontend dependencies..." -ForegroundColor Yellow
    Push-Location "$ROOT\frontend"
    try {
        if (-not (Test-Path node_modules)) {
            npm install 2>&1 | Out-Null
        }
        Write-Host "  npm install done" -ForegroundColor Green
    } finally {
        Pop-Location
    }

    Write-Host "  Starting frontend on :3000 ..." -ForegroundColor Yellow
    $frontendProc = Start-Process -FilePath "npm" `
        -ArgumentList "run","dev" `
        -WorkingDirectory "$ROOT\frontend" `
        -WindowStyle Minimized `
        -PassThru

    Write-Host "  frontend PID=$($frontendProc.Id)" -ForegroundColor Green
}

# ─── 5. Register Tools ───────────────────────────────
Write-Host ""
Write-Host "[5/5] Waiting for cloud-backend to be ready..." -ForegroundColor Yellow
$maxWait = 30
for ($i = 0; $i -lt $maxWait; $i++) {
    try {
        $null = Invoke-WebRequest -Uri "http://localhost:8080/api/health" -TimeoutSec 1 -ErrorAction Stop
        Write-Host "  cloud-backend is ready" -ForegroundColor Green
        break
    } catch {
        Start-Sleep -Seconds 1
    }
}

Write-Host "  Registering biaoshu tools..." -ForegroundColor Yellow
Push-Location "$ROOT\biaoshu-tools"
try {
    & .\register-tools.ps1
} finally {
    Pop-Location
}

# ─── Done ────────────────────────────────────────────
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  All services started!"                -ForegroundColor Cyan
Write-Host "  Cloud Backend : http://localhost:8080" -ForegroundColor Green
Write-Host "  Biaoshu Tools : http://localhost:9001" -ForegroundColor Green
Write-Host "  Frontend      : http://localhost:3000" -ForegroundColor Green
Write-Host "  API Docs      : http://localhost:8080/docs" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "To stop all services, close the terminal windows or run:" -ForegroundColor DarkGray
Write-Host "  Stop-Process -Name tangying-ai-os,python,node -Force" -ForegroundColor DarkGray
Write-Host '  docker compose -f cloud-backend\docker-compose.yml down' -ForegroundColor DarkGray
