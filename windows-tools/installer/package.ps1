<#
.SYNOPSIS
    Packaging and installation script for Windows Diego Cell.
.DESCRIPTION
    This script packages all the Windows Diego binaries into an self-extracting file.
    Upon self-extraction this script is run to unpack and install the Diego services.
.PARAMETER action
    This is the parameter that specifies what the script should do: package the binaries and create the installer, or install the services.
.PARAMETER binDir
    When the action is 'package', this parameter specifies where the Diego binaries are located. Not used otherwise.
.NOTES
    Author: Vlad Iovanov
    Date:   August 29, 2015
#>
param (
    [Parameter(Mandatory=$true)]
    [ValidateSet('package','install')]
    [string] $action,
    [string] $binDir
)

if (($pshome -like "*syswow64*") -and ((Get-WmiObject Win32_OperatingSystem).OSArchitecture -like "64*")) {
    Write-Warning "Restarting script under 64 bit powershell"
    
    $powershellLocation = join-path ($pshome -replace "syswow64", "sysnative") "powershell.exe"
    $scriptPath = $SCRIPT:MyInvocation.MyCommand.Path
    
    # relaunch this script under 64 bit shell
    $process = Start-Process -Wait -PassThru -NoNewWindow $powershellLocation "-nologo -file ${scriptPath} -action ${action} -binDir ${binDir}"
    
    # This will exit the original powershell process. This will only be done in case of an x86 process on a x64 OS.
    exit $process.ExitCode
}

# Entry point of the script when the action is "package"
function DoAction-Package($binDir)
{
    Write-Output "Packaging files from the ${binDir} dir ..."
    [Reflection.Assembly]::LoadWithPartialName( "System.IO.Compression.FileSystem" ) | out-null

    $destFile = Join-Path $(Get-Location) "binaries.zip"
    $compressionLevel = [System.IO.Compression.CompressionLevel]::Optimal
    $includeBaseDir = $false
    Remove-Item -Force -Path $destFile -ErrorAction SilentlyContinue

    Write-Output 'Creating zip ...'

    [System.IO.Compression.ZipFile]::CreateFromDirectory($binDir, $destFile, $compressionLevel, $includeBaseDir)

    Write-Output 'Creating the self extracting exe ...'

    $installerProcess = Start-Process -Wait -PassThru -NoNewWindow 'iexpress' "/N /Q diego-installer.sed"

    if ($installerProcess.ExitCode -ne 0)
    {
        Write-Error "There was an error building the installer."
        exit 1
    }
    
    Write-Output 'Removing artifacts ...'
    Remove-Item -Force -Path $destfile -ErrorAction SilentlyContinue
    
    Write-Output 'Done.'
}

# Entry point of the script when the action is "install"
function DoAction-Install()
{
	Write-Output 'Stopping any existing Diego Services'
	Stop-Service -Name "consul"
    Stop-Service -Name "converger"
    Stop-Service -Name "rep"
    Stop-Service -Name "auctioneer"
    Stop-Service -Name "garden-windows"

    Write-Output 'Installing Diego services ...'
    
    if ([string]::IsNullOrWhiteSpace($env:CONSUL_SERVER_IP -eq $null))
    {
        Write-Error 'Could not find environment variable CONSUL_SERVER_IP. Please set it to the IP address used by Consul and try again.'
        exit 1
    }

    if ([string]::IsNullOrWhiteSpace($env:REP_CELL_ID))
    {
        Write-Error 'Could not find environment variable REP_CELL_ID. Please set it (e.g. windows-cell-0) and then try again.'
        exit 1
    }

	if ([string]::IsNullOrWhiteSpace($env:DIEGO_NETADAPTER))
    {
       $env:DIEGO_NETADAPTER = "Ethernet"
    }
    $ipConfiguration = Get-NetIPConfiguration -InterfaceAlias $env:DIEGO_NETADAPTER
    $ipAddress = $ipConfiguration.IPv4Address.IPAddress
    $dnsAddresses = (($ipConfiguration.DNSServer | where {$_.AddressFamily -eq [System.Net.Sockets.AddressFamily]::InterNetwork}).ServerAddresses | where {$_ -ne '127.0.0.1'}) -Join ","


    if ([string]::IsNullOrWhiteSpace($env:GARDEN_CELL_IP))
    {
        $env:GARDEN_CELL_IP = $ipAddress
    }
    
    if ([string]::IsNullOrWhiteSpace($env:ETCD_CLUSTER))
    {
        $env:ETCD_CLUSTER = "http://etcd.service.dc1.consul:4001"
    }

    if ([string]::IsNullOrWhiteSpace($env:CONSUL_RECURSORS))
    {
        $env:CONSUL_RECURSORS = $dnsAddresses
    }

    if ([string]::IsNullOrWhiteSpace($env:CONSUL_CLUSTER))
    {
        $env:CONSUL_CLUSTER = "http://127.0.0.1:8500"
    }

    if ([string]::IsNullOrWhiteSpace($env:GARDEN_LISTEN_NETWORK))
    {
        $env:GARDEN_LISTEN_NETWORK = "tcp"
    }

    if ([string]::IsNullOrWhiteSpace($env:GARDEN_LISTEN_ADDRESS))
    {
        $env:GARDEN_LISTEN_ADDRESS = "127.0.0.1:58008"
    }

    if ([string]::IsNullOrWhiteSpace($env:GARDEN_LOG_LEVEL))
    {
        $env:GARDEN_LOG_LEVEL = "info"
    }

    if ([string]::IsNullOrWhiteSpace($env:REP_ZONE))
    {
        $env:REP_ZONE = "z1"
    }
    
    if ([string]::IsNullOrWhiteSpace($env:REP_MEMORY_MB))
    {
        $env:REP_MEMORY_MB = "auto"
    }
    
    if ([string]::IsNullOrWhiteSpace($env:REP_DISK_MB))
    {
        $env:REP_DISK_MB = "auto"
    }
    
    if ([string]::IsNullOrWhiteSpace($env:REP_LISTEN_ADDR))
    {
        $env:REP_LISTEN_ADDR = "0.0.0.0:1700"
    }
    
    if ([string]::IsNullOrWhiteSpace($env:REP_ROOT_FS_PROVIDER))
    {
        $env:REP_ROOT_FS_PROVIDER = "windowsservercore"
    }
    
    if ([string]::IsNullOrWhiteSpace($env:REP_CONTAINER_MAX_CPU_SHARES))
    {
        $env:REP_CONTAINER_MAX_CPU_SHARES = "1024"
    }
    
    if ([string]::IsNullOrWhiteSpace($env:REP_CONTAINER_INODE_LIMIT))
    {
        $env:REP_CONTAINER_INODE_LIMIT = "200000"
    }
    
    if ([string]::IsNullOrWhiteSpace($env:BBS_ADDRESS))
    {
        $env:BBS_ADDRESS = "http://bbs.service.dc1.consul:8889"    
    }

    if ([string]::IsNullOrWhiteSpace($env:DIEGO_INSTALL_DIR))
    {
        $env:DIEGO_INSTALL_DIR = "c:\diego"
    }
    
    Write-Output "Using GARDEN_CELL_IP $($env:GARDEN_CELL_IP)"
    Write-Output "Using ETCD_CLUSTER $($env:ETCD_CLUSTER)"
    Write-Output "Using CONSUL_RECURSORS $($env:CONSUL_RECURSORS)"
    Write-Output "Using CONSUL_CLUSTER $($env:CONSUL_CLUSTER)"
    Write-Output "Using GARDEN_LISTEN_NETWORK $($env:GARDEN_LISTEN_NETWORK)"
    Write-Output "Using GARDEN_LISTEN_ADDRESS $($env:GARDEN_LISTEN_ADDRESS)"
    Write-Output "Using GARDEN_LOG_LEVEL $($env:GARDEN_LOG_LEVEL)"
    Write-Output "Using REP_ZONE $($env:REP_ZONE)"
    Write-Output "Using REP_MEMORY_MB $($env:REP_MEMORY_MB)"
    Write-Output "Using REP_DISK_MB $($env:REP_DISK_MB)"
    Write-Output "Using REP_LISTEN_ADDR $($env:REP_LISTEN_ADDR)"
    Write-Output "Using REP_ROOT_FS_PROVIDER $($env:REP_ROOT_FS_PROVIDER)"
    Write-Output "Using REP_CONTAINER_MAX_CPU_SHARES $($env:REP_CONTAINER_MAX_CPU_SHARES)"
    Write-Output "Using REP_CONTAINER_INODE_LIMIT $($env:REP_CONTAINER_INODE_LIMIT)"
    Write-Output "Using BBS_ADDRESS $($env:BBS_ADDRESS)"

    $configuration = @{}
    
    $configuration["etcdCluster"] = $env:ETCD_CLUSTER
    $configuration["consulCluster"] = $env:CONSUL_CLUSTER
    $configuration["gardenListenNetwork"] = $env:GARDEN_LISTEN_NETWORK
    $configuration["gardenListenAddr"] = $env:GARDEN_LISTEN_ADDRESS
    $configuration["gardenLogLevel"] = $env:GARDEN_LOG_LEVEL
    $configuration["gardenCellIP"] = $env:GARDEN_CELL_IP
    $configuration["consulServerIp"] = $env:CONSUL_SERVER_IP
    $configuration["consulRecursors"] = $env:CONSUL_RECURSORS
    $configuration["repCellID"] = $env:REP_CELL_ID
    $configuration["repZone"] = $env:REP_ZONE
    $configuration["repMemoryMB"] = $env:REP_MEMORY_MB
    $configuration["repDiskMB"] = $env:REP_DISK_MB
    $configuration["repListenAddr"] = $env:REP_LISTEN_ADDR
    $configuration["repRootFSProvider"] = $env:REP_ROOT_FS_PROVIDER
    $configuration["repContainerMaxCpuShares"] = $env:REP_CONTAINER_MAX_CPU_SHARES
    $configuration["repContainerInodeLimit"] = $env:REP_CONTAINER_INODE_LIMIT
    $configuration["bbsAddress"] = $env:BBS_ADDRESS
    
    $destFolder = $env:DIEGO_INSTALL_DIR
    
    foreach ($dir in @($destFolder))
    {
        Write-Output "Cleaning up directory ${dir}"
        Remove-Item -Force -Recurse -Path $dir -ErrorVariable errors -ErrorAction SilentlyContinue

        if ($errs.Count -eq 0)
        {
            Write-Output "Successfully cleaned the directory ${dir}"
        }
        else
        {
            Write-Error "There was an error cleaning up the directory '${dir}'.`r`nPlease make sure the folder and any of its child items are not in use, then run the installer again."
            exit 1;
        }

        Write-Output "Setting up directory ${dir}"
        New-Item -path $dir -type directory -Force -ErrorAction SilentlyContinue
    }

    [Reflection.Assembly]::LoadWithPartialName( "System.IO.Compression.FileSystem" ) | out-null
    $srcFile = ".\binaries.zip"

    Write-Output 'Unpacking files ...'
    try
    {
        [System.IO.Compression.ZipFile]::ExtractToDirectory($srcFile, $destFolder)
    }
    catch
    {
        Write-Error "There was an error writing to the installation directory '${destFolder}'.`r`nPlease make sure the folder and any of its child items are not in use, then run the installer again."
        exit 1;
    }

    InstallDiego $destfolder $configuration $configFolder $logsFolder
}

# This function calls the nssm.exe binary to set a property
function SetNSSMParameter($serviceName, $parameterName, $parameterValue)
{
    Write-Output "Setting parameter '${parameterName}' for service '${serviceName}'"
    $nssmProcess = Start-Process -Wait -PassThru -NoNewWindow 'nssm' "set ${serviceName} ${parameterName} ${parameterValue}"

    if ($nssmProcess.ExitCode -ne 0)
    {
        Write-Error "There was an error setting the ${parameterName} NSSM parameter."
        exit 1
    }
}

# This function calls the nssm.exe binary to install a new  Windows Service
function InstallNSSMService($serviceName, $executable)
{
    Write-Output "Installing service '${serviceName}'"
    
    $nssmProcess = Start-Process -Wait -PassThru -NoNewWindow 'nssm' "remove ${serviceName} confirm"
   
    if (($nssmProcess.ExitCode -ne 0) -and ($nssmProcess.ExitCode -ne 3))
    {
        Write-Error "There was an error removing the '${serviceName}' service."
        exit 1
    }
    
    $nssmProcess = Start-Process -Wait -PassThru -NoNewWindow 'nssm' "install ${serviceName} ${executable}"

    if (($nssmProcess.ExitCode -ne 0) -and ($nssmProcess.ExitCode -ne 5))
    {
        Write-Error "There was an error installing the '${serviceName}' service."
        exit 1
    }
}

# This function sets up a Windows Service using the Non Sucking Service Manager
function SetupNSSMService($serviceName, $serviceDisplayName, $serviceDescription, $startupDirectory, $executable, $arguments, $stdoutLog, $stderrLog)
{
    InstallNSSMService $serviceName $executable
    SetNSSMParameter $serviceName "DisplayName" $serviceDisplayName
    SetNSSMParameter $serviceName "Description" $serviceDescription
    SetNSSMParameter $serviceName "AppDirectory" $startupDirectory
    SetNSSMParameter $serviceName "AppParameters" $arguments
    SetNSSMParameter $serviceName "AppStdout" $stdoutLog
    SetNSSMParameter $serviceName "AppStderr" $stderrLog
}


# This function does all the installation. Writes the config, installs services, sets up firewall 
function InstallDiego($destfolder, $configuration)
{
    Write-Output "Writing JSON configuration ..."
    $configFile = Join-Path $destFolder "config\windows-diego.json"
    $configuration | ConvertTo-Json | Out-File -Encoding ascii -FilePath $configFile

    Write-Output "Installing nssm services ..."

    $serviceConfigs = @{
        "converger" = @{
            "serviceDisplayName" = "Diego Converger";
            "serviceDescription" = "Identifies which actions need to take place to bring DesiredLRP state and ActualLRP state into accord";
            "startupDirectory" = $destFolder;
            "executable" = "C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe";
            "arguments" = "-ExecutionPolicy Bypass -nologo -file diego-ctl.ps1 run converger";
            "stdoutLog" = Join-Path $destFolder "logs\converger.stdout.log";
            "stderrLog" = Join-Path $destFolder "logs\converger.stderr.log";
        };
        "consul" = @{
            "serviceDisplayName" = "Diego Consul";
            "serviceDescription" = "A tool for discovering and configuring services in your infrastructure";
            "startupDirectory" = $destFolder;
            "executable" = "C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe";
            "arguments" = "-ExecutionPolicy Bypass -nologo -file diego-ctl.ps1 run consul";
            "stdoutLog" = Join-Path $destFolder "logs\consul.stdout.log";
            "stderrLog" = Join-Path $destFolder "logs\consul.stderr.log";
        };
        "rep" = @{
            "serviceDisplayName" = "Diego Rep";
            "serviceDescription" = "Represents a Cell and mediates all communication with the BBS";
            "startupDirectory" = $destFolder;
            "executable" = "C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe";
            "arguments" = "-ExecutionPolicy Bypass -nologo -file diego-ctl.ps1 run rep";
            "stdoutLog" = Join-Path $destFolder "logs\rep.stdout.log";
            "stderrLog" = Join-Path $destFolder "logs\rep.stderr.log";
        };
        "auctioneer" = @{
            "serviceDisplayName" = "Diego Auctioneer";
            "serviceDescription" = "Holds auctions for Tasks and ActualLRP instances";
            "startupDirectory" = $destFolder;
            "executable" = "C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe";
            "arguments" = "-ExecutionPolicy Bypass -nologo -file diego-ctl.ps1 run auctioneer";
            "stdoutLog" = Join-Path $destFolder "logs\auctioneer.stdout.log";
            "stderrLog" = Join-Path $destFolder "logs\auctioneer.stderr.log";
        };
        "garden-windows" = @{
            "serviceDisplayName" = "Diego Windows Garden";
            "serviceDescription" = "Provides a Windows specific implementation of a Garden interface";
            "startupDirectory" = $destFolder;
            "executable" = "C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe";
            "arguments" = "-ExecutionPolicy Bypass -nologo -file diego-ctl.ps1 run garden-windows";
            "stdoutLog" = Join-Path $destFolder "logs\garden-windows.stdout.log";
            "stderrLog" = Join-Path $destFolder "logs\garden-windows.stderr.log";
        };
    }
    
    # Setup windows services
    foreach ($serviceName in $serviceConfigs.Keys)
    {
        $serviceConfig = $serviceConfigs[$serviceName]
        $serviceDisplayName = $serviceConfig["serviceDisplayName"]
        $serviceDescription = $serviceConfig["serviceDescription"]
        $startupDirectory = $serviceConfig["startupDirectory"]
        $executable = $serviceConfig["executable"]
        $arguments = $serviceConfig["arguments"]
        $stdoutLog = $serviceConfig["stdoutLog"]
        $stderrLog = $serviceConfig["stderrLog"]
        SetupNSSMService $serviceName $serviceDisplayName $serviceDescription $startupDirectory $executable $arguments $stdoutLog $stderrLog
    }
    
    # Setup firewall rules
    if (!(Get-NetFirewallRule | where {$_.Name -eq "TCP8080"})) {
       New-NetFirewallRule -Name "TCP8080" -DisplayName "HTTP on TCP/8080" -Protocol tcp -LocalPort 8080 -Action Allow -Enabled True
    }
	
	if (!(Get-NetFirewallRule | where {$_.Name -eq "TCP1700"})) {
       New-NetFirewallRule -Name "TCP1700" -DisplayName "HTTP on TCP/1700" -Protocol tcp -LocalPort 1700 -Action Allow -Enabled True
    }
    
    # Start consul
    Start-Service -Name "consul"

    # Set windows DNS server
	$dnsServers = @("127.0.0.1") + $env:CONSUL_RECURSORS.split(",")
	Write-Output "Setting up $($dnsServers.Length) DNS servers: $($dnsServers -Join ',')"
	Set-DnsClientServerAddress -InterfaceAlias $env:DIEGO_NETADAPTER -ServerAddresses $dnsServers

    # Start all other services
    Start-Service -Name "consul"
    Start-Service -Name "converger"
    Start-Service -Name "rep"
    Start-Service -Name "auctioneer"
    Start-Service -Name "garden-windows"
}

if ($action -eq 'package')
{
    if ([string]::IsNullOrWhiteSpace($binDir))
    {
        Write-Error 'The binDir parameter is mandatory when packaging.'
        exit 1
    }
    
    $binDir = Resolve-Path $binDir
    
    if ((Test-Path $binDir) -eq $false)
    {
        Write-Error "Could not find directory ${binDir}."
        exit 1        
    }
    
    Write-Output "Using binary dir ${binDir}"
    
    DoAction-Package $binDir
}
elseif ($action -eq 'install')
{
    DoAction-Install
}