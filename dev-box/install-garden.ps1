$wd="C:\diego-kit"
mkdir -Force $wd
cd $wd

# Make a local clone from the vagrant share and build garden-hcs

$gardenHcsPackage = "github.com/cloudfoundry-incubator/garden-windows"
$parentPath = Split-Path -Parent $gardenHcsPackage
$leafPath = Split-Path -Leaf $gardenHcsPackage
mkdir -Force $env:GOPATH/src/$parentPath
cd $env:GOPATH/src/$parentPath
git clone --local file:///C:/garden-hcs $leafPath

cd $env:GOPATH/src/$gardenHcsPackage
git pull
git remote add hpcloud https://github.com/hpcloud/garden-hcs
go get -v $gardenHcsPackage
$gardenExePath = "$env:GOPATH/bin/$leafPath.exe"

echo "Creating base image for garden. This takes several minutes"
cd $env:GOPATH/src/$gardenHcsPackage/rootfs
docker build -t garden-rootfs .


$baseImagePath = (docker inspect garden-rootfs  | ConvertFrom-Json).GraphDriver.Data.Dir

$machineIp = (Find-NetRoute -RemoteIPAddress "192.168.50.4")[0].IPAddress


$gardenArgs = " -listenAddr 0.0.0.0:9241 -logLevel debug -cellIP $machineIp -baseImagePath $baseImagePath "
echo  "$leafPath.exe $gardenArgs" | Out-File -Encoding ascii  $wd\start-garden.bat
echo  "gaol /t 127.0.0.1:9241 list" | Out-File -Encoding ascii  $wd\gaol-list.bat

# Install Garden as a service

Invoke-WebRequest -UseBasicParsing -URI "http://${machineIp}:1800/evacuate" -Method "Post"
Restart-Service RepService

Stop-Service GardenHcs -ErrorAction SilentlyContinue
nssm remove GardenHcs confirm
nssm install GardenHcs $gardenExePath
nssm set GardenHcs AppParameters $gardenArgs
nssm set GardenHcs AppStdout "C:\garden-stdout.txt"
nssm set GardenHcs AppStdout "C:\garden-stderr.txt"
Start-Service GardenHcs

echo " "
echo "Gatden client (gaol) usage example:"
echo " gaol /t 127.0.0.1:9241 list"
echo "To run garden manually with the following start script:"
echo  " $leafPath.exe $gardenArgs"


# Optional - Install extra dev/debug tools and push a windows app

go get -v github.com/onsi/ginkgo/ginkgo
go get -v github.com/tools/godep

go get -v github.com/stefanschneider/gaol
setx /m GAOL_TARGET 127.0.0.1:9241

gem install bosh_cli -N
bosh -n target 192.168.50.4 lite
bosh -n login admin admin
bosh -n download manifest cf-warden $wd\cf-warden.yml
bosh -n download manifest cf-warden-diego $wd\cf-warden-diego.yml
bosh deployment $wd\cf-warden-diego.yml

cf api --skip-ssl-validation api.bosh-lite.com
cf auth admin admin
# cf curl /v2/stacks -X POST -d '{"name":"windows2016","description":"Windows Server Core 2016"}' # Does not work form PS
echo '{"name":"windows2016","description":"Windows Server Core 2016"}' | Out-File -Encoding ascii ${env:TEMP}\cf-stack.json
cf curl /v2/stacks -X POST -d `@${env:TEMP}\cf-stack.json
cf enable-feature-flag diego_docker
cf create-org diego
cf target -o diego
cf create-space diego
cf target -o diego -s diego

cd $wd
git clone https://github.com/hpcloud/cf-exe-buildpack
cf create-buildpack exe_buildpack cf-exe-buildpack 100 --enable
cf update-buildpack exe_buildpack -p cf-exe-buildpack

cd $wd
git clone https://github.com/hpcloud/cf-iis8-buildpack
Push-Location .\cf-iis8-buildpack
git checkout develop
Pop-Location
cf create-buildpack iis8_buildpack cf-iis8-buildpack 100 --enable
cf update-buildpack iis8_buildpack -p cf-iis8-buildpack

cd $wd
git clone https://github.com/cloudfoundry/wats
cd wats\assets\webapp
echo 'webapp.exe' | Out-File -Encoding ascii  run.bat
# cf push exeapp -s windows2016 -b https://github.com/hpcloud/cf-exe-buildpack -c webapp.exe
cf push exeapp -s windows2016
(iwr -UseBasicParsing exeapp.bosh-lite.com).Content

cd $wd
git clone https://github.com/cloudfoundry/wats
cd wats/assets/nora/NoraPublished
cf push nora -s windows2016 -b "https://github.com/hpcloud/cf-iis8-buildpack#develop"
# cf push nora -s windows2016 -b "https://github.com/stefanschneider/windows_app_lifecycle#buildpack-extraction"
(iwr -UseBasicParsing nora.bosh-lite.com).Content
