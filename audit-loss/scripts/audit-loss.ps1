param(
    [Parameter(Position = 0)]
    [ValidateSet("happy", "main-unreachable", "malformed")]
    [string]$Scenario
)

$ErrorActionPreference = "Stop"
$Spammer = "http://localhost:18080"
$Toxic = "http://localhost:18081"
$Sink = "http://localhost:18082"

function Write-Step($Message) {
    Write-Host "==> $Message"
}

function Wait-Healthy($Url) {
    Write-Step "waiting for $Url"
    for ($i = 0; $i -lt 60; $i++) {
        try {
            $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2
            if ($resp.StatusCode -eq 200) {
                return
            }
        } catch {
            Write-Host "  not ready ($($_.Exception.Message))"
        }
        Start-Sleep -Seconds 1
    }
    throw "timed out waiting for $Url"
}

function Start-Run($Name, $Count) {
    $body = @{ count = $Count; threads = 1; batch_size = $Count; scenario = $Name } | ConvertTo-Json
    if ($Name -eq "malformed") {
        $body = @{ count = $Count; threads = 1; batch_size = $Count; scenario = $Name; fault = "malformed_json" } | ConvertTo-Json
    }
    Write-Step "POST /run $body"
    return Invoke-RestMethod -Method Post -Uri "$Spammer/run" -ContentType "application/json" -Body $body
}

function Get-RunStats($RunId) {
    $deadline = (Get-Date).AddSeconds(60)
    do {
        $stats = Invoke-RestMethod -Uri "$Sink/stats?run_id=$RunId"
        Write-Host ("  status={0} sent={1} received={2} main={3} secondary={4} rejected={5}" -f $stats.status, $stats.sent, $stats.received, $stats.by_collector.main, $stats.by_collector.secondary, $stats.rejected)
        if ($stats.status -eq "drained") {
            return $stats
        }
        Start-Sleep -Seconds 1
    } while ((Get-Date) -lt $deadline)
    throw "run $RunId did not drain"
}

function Assert-Happy($Stats) {
    if ($Stats.received -ne $Stats.sent -or $Stats.sent -le 0) { throw "happy: received $($Stats.received) sent $($Stats.sent)" }
    if ($Stats.by_collector.main -ne $Stats.sent) { throw "happy: main $($Stats.by_collector.main)" }
    if ($Stats.by_collector.secondary -ne 0) { throw "happy: secondary $($Stats.by_collector.secondary)" }
    $missing = @()
    if ($null -ne $Stats.missing) { $missing = @($Stats.missing) }
    if ($missing.Count -ne 0) { throw "happy: missing $($missing -join ',')" }
}

Wait-Healthy "$Spammer/health"
Wait-Healthy "$Toxic/health"
Wait-Healthy "$Sink/health"
Wait-Healthy "http://localhost:13133/"
Wait-Healthy "http://localhost:13134/"
Wait-Healthy "http://localhost:13135/"

switch ($Scenario) {
    "happy" {
        $run = Start-Run "happy" 10
        $stats = Get-RunStats $run.run_id
        Assert-Happy $stats
        Write-Step "happy passed run $($run.run_id)"
    }
    "main-unreachable" {
        Write-Step "blackhole to-main"
        Invoke-RestMethod -Method Post -Uri "$Toxic/faults/to-main" -ContentType "application/json" -Body '{"blackhole":true}' | Out-Host
        $run = Start-Run "main-unreachable" 10
        $stats = Get-RunStats $run.run_id
        if ($stats.received -ne $stats.sent -or $stats.by_collector.secondary -ne $stats.sent -or $stats.by_collector.main -ne 0) {
            throw "failover: sent $($stats.sent) received $($stats.received) main $($stats.by_collector.main) secondary $($stats.by_collector.secondary)"
        }
        Write-Step "clear to-main and wait 16s for failover to prefer main again"
        Invoke-WebRequest -Method Delete -Uri "$Toxic/faults/to-main" -UseBasicParsing | Out-Null
        Start-Sleep -Seconds 16
        $again = Start-Run "main-unreachable-recover" 10
        $recovered = Get-RunStats $again.run_id
        Assert-Happy $recovered
        Write-Step "main-unreachable passed"
    }
    "malformed" {
        $run = Start-Run "malformed" 10
        $stats = Get-RunStats $run.run_id
        if ($stats.received -le 0 -or $stats.received -ge $stats.sent -or $stats.rejected -le 0) {
            throw "malformed: sent $($stats.sent) received $($stats.received) rejected $($stats.rejected)"
        }
        Write-Step "malformed passed sent=$($stats.sent) received=$($stats.received) rejected=$($stats.rejected)"
    }
}
