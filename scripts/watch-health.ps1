# Watchdog ngoài cho AI Creator OS (Đợt X, tự hồi phục từ Đợt AK) — chạy định
# kỳ bằng Task Scheduler, vd: mỗi 5 phút.
#
# Vì sao cần: Task Scheduler "restart on failure" chỉ chạy lại app khi process
# *thoát*. Process bị deadlock/treo mà chưa chết thì tưởng vẫn khỏe.
# Script này gọi GET /healthz và coi là LỖI khi: không kết nối được,
# db_ok=false, hoặc needs_restart=true (có tác vụ nền kẹt).
#
# Lỗi 2 lần LIÊN TIẾP (tránh restart nhầm lúc app đang khởi động) → tự
# khởi động lại scheduled task "aicos" (Stop + Start). Không có task đó →
# chỉ ghi log + exit 1.
#
# Dùng với Task Scheduler (xem deploy/README.md):
#   - Trigger: mỗi 5 phút, lặp vô thời hạn
#   - Action: powershell.exe -NoProfile -ExecutionPolicy Bypass -File "C:\path\to\watch-health.ps1"
#
# Biến môi trường:
#   AICOS_HEALTH_URL         URL health (mặc định http://127.0.0.1:8080/healthz)
#   AICOS_TASK_NAME          tên scheduled task của app (mặc định aicos)
#   AICOS_WATCHDOG_RESTART   =0 → chỉ ghi log, không tự khởi động lại
#   WATCHDOG_VERBOSE         =1 → ghi 1 dòng cả khi khỏe (mặc định im lặng khi khỏe)

$ErrorActionPreference = "Stop"

$url = $env:AICOS_HEALTH_URL
if ([string]::IsNullOrWhiteSpace($url)) { $url = "http://127.0.0.1:8080/healthz" }
$task = $env:AICOS_TASK_NAME
if ([string]::IsNullOrWhiteSpace($task)) { $task = "aicos" }
$verbose = $env:WATCHDOG_VERBOSE -eq "1"
$state = Join-Path $env:TEMP "aicos-watchdog.fails"

function Stamp { Get-Date -Format "yyyy-MM-dd HH:mm:ss" }

$problem = $null
try {
    $resp = Invoke-RestMethod -Uri $url -Method Get -TimeoutSec 10
    if (-not $resp.db_ok) {
        $problem = "app trả lời nhưng DB không khỏe: $($resp | ConvertTo-Json -Compress)"
    } elseif ($resp.needs_restart) {
        $problem = "có tác vụ nền kẹt: $($resp.stale_ticks -join ', ')"
    }
} catch {
    $problem = "không kết nối được $url ($($_.Exception.Message))"
}

if (-not $problem) {
    Remove-Item -Path $state -ErrorAction SilentlyContinue
    if ($verbose) { Write-Output "$(Stamp) WATCHDOG: OK (uptime_sec=$($resp.uptime_sec))" }
    exit 0
}

$fails = 1
if (Test-Path $state) { $fails = [int](Get-Content $state) + 1 }
Set-Content -Path $state -Value $fails
Write-Output "$(Stamp) WATCHDOG: $problem (lần $fails liên tiếp)"

if ($fails -ge 2 -and $env:AICOS_WATCHDOG_RESTART -ne "0") {
    $t = Get-ScheduledTask -TaskName $task -ErrorAction SilentlyContinue
    if ($t) {
        Write-Output "$(Stamp) WATCHDOG: tự khởi động lại task $task"
        try {
            Stop-ScheduledTask -TaskName $task
            Start-Sleep -Seconds 3
            Start-ScheduledTask -TaskName $task
            Remove-Item -Path $state -ErrorAction SilentlyContinue
        } catch {
            Write-Output "$(Stamp) WATCHDOG: khởi động lại thất bại: $($_.Exception.Message)"
        }
    }
}
exit 1
