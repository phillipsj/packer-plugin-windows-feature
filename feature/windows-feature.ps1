param(
    [string[]]$Features,
    [string[]]$Capabilities,
    [switch]$OnlyCheckForRebootRequired = $false
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

function ExitWithCode($exitCode) {
    $host.SetShouldExit($exitCode)
    Exit
}

trap {
    Write-Output "ERROR: $_"
    Write-Output (($_.ScriptStackTrace -split '\r?\n') -replace '^(.*)$', 'ERROR: $1')
    Write-Output (($_.Exception.ToString() -split '\r?\n') -replace '^(.*)$', 'ERROR EXCEPTION: $1')
    ExitWithCode 1
}

Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class Windows
{
    [DllImport("kernel32", SetLastError=true)]
    public static extern UInt64 GetTickCount64();
    public static TimeSpan GetUptime()
    {
        return TimeSpan.FromMilliseconds(GetTickCount64());
    }
}
'@

function Wait-Condition {
    param(
        [scriptblock]$Condition,
        [int]$DebounceSeconds = 15
    )
    process {
        $begin = [Windows]::GetUptime()
        do {
            Start-Sleep -Seconds 1
            try {
                $result = &$Condition
            }
            catch {
                $result = $false
            }
            if (-not $result) {
                $begin = [Windows]::GetUptime()
                continue
            }
        } while ((([Windows]::GetUptime()) - $begin).TotalSeconds -lt $DebounceSeconds)
    }
}

function ExitWhenRebootRequired($rebootRequired = $false) {
    # check for pending Windows Updates.
    if (!$rebootRequired) {
        $systemInformation = New-Object -ComObject 'Microsoft.Update.SystemInfo'
        $rebootRequired = $systemInformation.RebootRequired
    }

    # check for pending Windows Features.
    if (!$rebootRequired) {
        $pendingPackagesKey = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\PackagesPending'
        $pendingPackagesCount = (Get-ChildItem -ErrorAction SilentlyContinue $pendingPackagesKey | Measure-Object).Count
        $rebootRequired = $pendingPackagesCount -gt 0
    }

    if ($rebootRequired) {
        Write-Output 'Waiting for the Windows Modules Installer to exit...'
        Wait-Condition { (Get-Process -ErrorAction SilentlyContinue TiWorker | Measure-Object).Count -eq 0 }
        ExitWithCode 101
    }
}

function Install-WindowsFeatures {
    [CmdletBinding()]
    param (
        [Parameter(Mandatory = $true)]
        [String[]]
        $RequiredFeatures
    )

    $restartNeeded = $false

    foreach ($feature in $RequiredFeatures) {
        $f = Get-WindowsFeature -Name $feature
        if (-not $f.Installed) {
            $result = Install-WindowsFeature -Name $feature
            if ($result.RestartNeeded -eq "Yes") {
                $restartNeeded = $true
            }
        }
        else {
            Write-Output "Windows feature: '$feature' is already installed."
        }
    }

    return $restartNeeded
}

function Install-WindowsCapabilities {
    [CmdletBinding()]
    param (
        [Parameter(Mandatory = $true)]
        [String[]]
        $RequiredCapabilities
    )

    $restartNeeded = $false

    foreach ($capability in $RequiredCapabilities) {
        $c = Get-WindowsCapability -Name $capability
        if ($c.State -ne "Installed") {
            $result = Add-WindowsCapability -Online -Name $capability
            if ($result.RestartNeeded) {
                $restartNeeded = $true
            }
        }
        else {
            Write-Output "Windows capability: '$capability' is already installed."
        }
    }

    return $restartNeeded
}

ExitWhenRebootRequired

if ($OnlyCheckForRebootRequired) {
    Write-Output "$env:COMPUTERNAME restarted."
    ExitWithCode 0
}

$featureRestart = Install-WindowsFeature -RequiredFeatures $Features
$capabilityRestart = Install-WindowsCapabilities -RequiredCapabilities $Capabilities

ExitWhenRebootRequired ($featureRestart -or $capabilityRestart)