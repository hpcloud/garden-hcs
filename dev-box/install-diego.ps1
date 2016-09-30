$wd="C:\diego-kit"
mkdir -Force $wd
cd $wd


## Cleanup previous installations

echo "Trying to uninstall DiegoWindows"
Get-CimInstance Win32_Product  -Filter "Name = 'DiegoWindows'" | Invoke-CimMethod -MethodName Uninstall

## Download installers

$diegoReleaseVersion="v0.443"
curl -UseBasicParsing -OutFile $wd\DiegoWindows.msi https://github.com/cloudfoundry/diego-windows-release/releases/download/$diegoReleaseVersion/DiegoWindows.msi -Verbose
curl -UseBasicParsing -OutFile $wd\generate.exe https://github.com/cloudfoundry/diego-windows-release/releases/download/$diegoReleaseVersion/generate.exe -Verbose
curl -UseBasicParsing -OutFile $wd\hakim.exe https://github.com/cloudfoundry/diego-windows-release/releases/download/$diegoReleaseVersion/hakim.exe -Verbose


## Setup diego networking

$machineIp = (Find-NetRoute -RemoteIPAddress "192.168.50.4")[0].IPAddress
$diegoInterface = Get-NetIPAddress -IPAddress $machineIp
# $diegoInterface | Remove-NetIPAddress -AddressFamily IPv4 -Confirm:$false
# $diegoInterface | New-NetIPAddress -AddressFamily IPv4  -IPAddress $machineIp -PrefixLength $diegoInterface.PrefixLength

### 1.2.3.4 is used by rep to discover the IP address to be announced to the diego cluster
route delete 1.2.3.4
route add 1.2.3.4 192.168.50.4 -p

route delete 10.244.0.0
route add 10.244.0.0 mask 255.255.0.0 192.168.50.4 -p


# Set consul dns

$currentDNS = Get-DnsClientServerAddress -InterfaceIndex $diegoInterface.InterfaceIndex
$filteredDNS = ($currentDNS  | where { $_.AddressFamily -eq [System.Net.Sockets.AddressFamily]::InterNetwork }).ServerAddresses
$filteredDNS = $filteredDNS  | where { $_ -ne "127.0.0.1"}
$newDNS = @("127.0.0.1") + $filteredDNS
Set-DnsClientServerAddress -InterfaceIndex $diegoInterface.InterfaceIndex -ServerAddresses ($newDNS -join ",")


## Disable negative DNS client cache

New-Item 'HKLM:\SYSTEM\CurrentControlSet\Services\Dnscache\Parameters' -Force | `
  New-ItemProperty -Name MaxNegativeCacheTtl -PropertyType "DWord" -Value 1 -Force

Clear-DnsClientCache


## Firewall

Remove-NetFirewallRule -Name Allow_Ping  -ErrorAction SilentlyContinue
New-NetFirewallRule -Name Allow_Ping -DisplayName "Allow Ping"  -Description "Packet Internet Groper ICMPv4" -Protocol ICMPv4 -IcmpType 8 -Enabled True -Profile Any -Action Allow

$allowAdmin = "D:(A;;CC;;;S-1-5-32-544)"

Remove-NetFirewallRule -Name CFAllowAdminsO -ErrorAction SilentlyContinue
New-NetFirewallRule -Name CFAllowAdminsO -DisplayName "Allow admins" `
    -Description "Allow admin users" -RemotePort Any `
    -LocalPort Any -LocalAddress Any -RemoteAddress Any `
    -Enabled True -Profile Any -Action Allow -Direction Outbound `
    -LocalUser $allowAdmin

Remove-NetFirewallRule -Name CFAllowAdminsI -ErrorAction SilentlyContinue
New-NetFirewallRule -Name CFAllowAdminsI -DisplayName "Allow admins" `
    -Description "Allow admin users" -RemotePort Any `
    -LocalPort Any -LocalAddress Any -RemoteAddress Any -EdgeTraversalPolicy Allow `
    -Enabled True -Profile Any -Action Allow -Direction Inbound `
    -LocalUser $allowAdmin


## Setup diego


echo "Running generate.exe"
& $wd\generate.exe  -boshUrl https://admin:admin@192.168.50.4:25555  -outputDir .  -machineIp "$machineIp"

echo @"
msiexec /passive /norestart /i %~dp0\DiegoWindows.msi ^
  BBS_CA_FILE=%~dp0\bbs_ca.crt ^
  BBS_CLIENT_CERT_FILE=%~dp0\bbs_client.crt ^
  BBS_CLIENT_KEY_FILE=%~dp0\bbs_client.key ^
  CONSUL_DOMAIN=cf.internal ^
  CONSUL_IPS=10.244.0.54 ^
  CF_ETCD_CLUSTER=http://cf-etcd.service.cf.internal:4001 ^
  STACK=windows2016 ^
  REDUNDANCY_ZONE=windows ^
  LOGGREGATOR_SHARED_SECRET=loggregator-secret ^
  MACHINE_IP=$machineIp ^
  CONSUL_ENCRYPT_FILE=%~dp0\consul_encrypt.key ^
  CONSUL_CA_FILE=%~dp0\consul_ca.crt ^
  CONSUL_AGENT_CERT_FILE=%~dp0\consul_agent.crt ^
  CONSUL_AGENT_KEY_FILE=%~dp0\consul_agent.key

"@ ` | Out-File -Encoding ascii  $wd\install-diego.bat

echo "Installing DiegoWindows"
& $wd\install-diego.bat

Restart-Service ConsulService, MetronService, RepService

Get-EventLog Application -Source rep,metron,consul -Newest 100 | % {echo ($_.TimeGenerated.ToString() + " - " + $_.Message)} > $wd\bootstarp.log

echo "Diego components start logs: $wd\bootstarp.log"

# Make sure the diego windows-app-lifecycle is configured for windows2016 stack
# https://ci.appveyor.com/project/StefanSchneider/windows-app-lifecycle-qc4gr/build/artifacts

echo "Checking Consul health"
echo (curl -UseBasicParsing http://127.0.0.1:8500/).StatusDescription

echo "Consul members"
& 'C:\Program Files\CloudFoundry\DiegoWindows\consul.exe' members

#echo "Checking Rep health"
#echo (curl -UseBasicParsing "http://${machineIp}:1800/ping").StatusDescription

#echo "Interogating Rep status"
#echo (curl -UseBasicParsing "http://${machineIp}:1800/state").Content | ConvertFrom-Json
