param(
    [ValidateSet("debian", "arch", "both")]
    [string]$Distro = "debian",
    [switch]$NoBuild,
    [switch]$SkipInstall
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root

function Invoke-DockerCompose {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments
    )
    # Native stderr must not become a terminating error under
    # $ErrorActionPreference = "Stop" (Docker writes progress to stderr).
    $previous = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & docker compose @Arguments | Out-Host
    }
    finally {
        $ErrorActionPreference = $previous
    }
    return $LASTEXITCODE
}

try {
    $services = if ($Distro -eq "both") { @("debian", "arch") } else { @($Distro) }

    if (-not $NoBuild) {
        foreach ($service in $services) {
            Write-Host "==> docker compose build $service" -ForegroundColor Cyan
            $code = Invoke-DockerCompose @("build", $service)
            if ($code -ne 0) { exit $code }
        }
    }

    $smoke = if ($SkipInstall) { "0" } else { "1" }
    foreach ($service in $services) {
        Write-Host "==> docker compose run --rm $service" -ForegroundColor Cyan
        $code = Invoke-DockerCompose @("run", "--rm", "-e", "SMOKE_APPLY=$smoke", $service)
        if ($code -ne 0) { exit $code }
    }
}
finally {
    Pop-Location
}
