
$binDir = "C:\Users\Administrator\code\diego-release\src\github.com\cloudfoundry-incubator\garden-windows\diego\artifacts"
$pidDir = "c:\Workspace\DiegoPids"
$logDir = "c:\Workspace\DiegoLogs"

function Run-Daemon{[CmdletBinding()]param($exe, $args, $log, $pidFile)
    $process = Start-Process 'cmd.exe' "/c ${exe} ${args} 2>&1 1>${log}"
    $process.Id | Out-File $pidFile
}

function Start-Converger{[CmdletBinding()]param()
    Write-Output "Trying to start converger ..."

    $convergerBin = Join-Path $binDir 'converger.exe'
    $logFile = Join-Path $logDir 'converger.log'
    $pidFile = Join-Path $pidDir 'converger.pid'
    
    $etcdCluster = 'http://etcd.service.dc1.consul:4001'
    $consulCluster = 'http://127.0.0.1:8500'

    $args = "-etcdCluster ${etcdCluster} -consulCluster=`"${consulCluster}`""

    Run-Daemon $convergerBin $args $logFile $pidFile
}

function Start-Consul{[CmdletBinding()]param()
    Write-Output "Trying to start consul ..."

    $consulBin = Join-Path $binDir 'consul.exe'
    $logFile = Join-Path $logDir 'consul.log'
    $pidFile = Join-Path $pidDir 'consul.pid'

    $consulJsonConfig = "c:\workspace\consul\consul.json"
    $consulDataDir = "c:\workspace\consul\data"
    $consulServerIp = "127.0.0.1"

    $args = "agent -config-file ${consulJsonConfig} -data-dir ${consulDataDir} -join ${consulServerIp}"

    Run-Daemon $consulBin $args $logFile $pidFile
}

function Start-Rep{[CmdletBinding()]param()
#    tee2metron -dropsondeDestination=127.0.0.1:3457 -sourceInstance=$LATTICE_CELL_ID \

    Write-Output "Trying to start rep ..."

    $repBin = Join-Path $binDir 'rep.exe'
    $logFile = Join-Path $logDir 'rep.log'
    $pidFile = Join-Path $pidDir 'rep.pid'

    $etcdCluster = 'http://etcd.service.dc1.consul:4001'
    $consulCluster='http://127.0.0.1:8500'
    $cellID='$LATTICE_CELL_ID'
    $zone='z1'
    $rootFSProvider='docker'
    $listenAddr='0.0.0.0:1700'
    $gardenNetwork='tcp'
    $gardenAddr='127.0.0.1:7777'
    $memoryMB='auto'
    $diskMB='auto'
    
    $args = "-etcdCluster ${etcdCluster} -consulCluster=`"${consulCluster}`" -cellID=${cellID} -zone=${zone} -rootFSProvider=${rootFSProvider} -listenAddr=${listenAddr} -gardenNetwork=${gardenNetwork} -gardenAddr=${gardenAddr} -memoryMB=${memoryMB} -diskMB=${diskMB}"

    Run-Daemon $repBin $args $logFile $pidFile
}

function Start-Auctioneer{[CmdletBinding()]param()
    Write-Output "Trying to start auctioneer ..."

    $auctioneerBin = Join-Path $binDir 'auctioneer.exe'
    $logFile = Join-Path $logDir 'auctioneer.log'
    $pidFile = Join-Path $pidDir 'auctioneer.pid'
    
    $etcdCluster = 'http://etcd.service.dc1.consul:4001'
    $consulCluster='http://127.0.0.1:8500'

    $args = "-etcdCluster ${etcdCluster} -consulCluster=`"${consulCluster}`""

    Run-Daemon $auctioneerBin $args $logFile $pidFile
}

function Start-Garden{[CmdletBinding()]param()
#    tee2metron -dropsondeDestination=127.0.0.1:3457 -sourceInstance=$LATTICE_CELL_ID \

    Write-Output "Trying to start garden ..."

    $gardenBin = Join-Path $binDir 'garden-windows.exe'
    $logFile = Join-Path $logDir 'garden-windows.log'
    $pidFile = Join-Path $pidDir 'garden-windows.pid'
    
    $listenNetwork="tcp"
    $listenAddr="0.0.0.0:58008"
    $logLevel="info"

    $args = "-listenNetwork=${listenNetwork} -listenAddr=${listenAddr} -logLevel=${logLevel}"

    Run-Daemon $gardenBin $args $logFile $pidFile
}

#CONSUL_SERVER_IP=10.0.1.2
#SYSTEM_DOMAIN=15.126.240.42.xip.io
#LATTICE_CELL_ID=lattice-cell-0
#GARDEN_EXTERNAL_IP=10.0.1.5