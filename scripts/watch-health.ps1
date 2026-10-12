# Watchdog ngoài cho AI Creator OS (Đợt X) — chạy định kỳ bằng Task Scheduler,
# vd: mỗi 5 phút.
#
# Vì sao cần: Task Scheduler "restart on failure" chỉ chạy lại app khi process
# *thoát*. Process bị deadlock/treo mà chưa chết thì tưởng vẫn khỏe.
# Script này gọi GET /healthz: không kết nối được hoặc db_ok=false
# → ghi 1 dòng log + exit 1, để Task Scheduler ghi nhận.
#
# Dùng với Task Scheduler (xem deploy/README.md):
#   - Trigger: mỗi 5 phút, lặp vô thời hạn
#   - Action: powershell.exe -NoProfile -ExecutionPolicy Bypass -File "C:\path\to\watch-health.ps1"
#
# Biến môi trường:
#   AICOS_HEALTH_URL  URL health (mặc định http://127.0.0.1:8080/healthz)
#   WATCHDOG_VERBOSE  =1 → ghi 1 dòng cả khi khỏe (mặc định im lặng khi khỏe)

$ErrorActionPreference = "Stop"

$url = $env:AICOS_HEALTH_URL
if ([string]::IsNullOrWhiteSpace($url)) { $url = "http://127.0.0.1:8080/healthz" }
$verbose = $env:WATCHDOG_VERBOSE -eq "1"

function Stamp { Get-Date -Format "yyyy-MM-dd HH:mm:ss" }

try {
    $resp = Invoke-RestMethod -Uri $url -Method Get -TimeoutSec 10
} catch {
    Write-Output "$(Stamp) WATCHDOG: không kết nối được $url ($($_.Exception.Message))"
    exit 1
}

# db_ok=false → app sống nhưng DB có vấn đề.
if (-not $resp.db_ok) {
    Write-Output "$(Stamp) WATCHDOG: app trả lời nhưng DB không khỏe: $($resp | ConvertTo-Json -Compress)"
    exit 1
}

if ($verbose) {
    Write-Output "$(Stamp) WATCHDOG: OK (uptime_sec=$($resp.uptime_sec))"
}
exit 0
