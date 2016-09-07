$wd="C:\diego-kit"
mkdir -Force $wd
cd $wd


go get -v github.com/onsi/ginkgo/ginkgo
go get -v github.com/stefanschneider/gaol
go get -v github.com/tools/godep


gem install bosh_cli -N

bosh -n target 192.168.50.4 lite
bosh -n login admin admin
bosh -n download manifest cf-warden-diego $wd\cf-warden-diego.yml
bosh deployment $wd\cf-warden-diego.yml

# Download a build for https://github.com/stefanschneider/windows_app_lifecycle/tree/w2016 - https://ci.appveyor.com/project/StefanSchneider/windows-app-lifecycle-qc4gr/build/artifacts
# iwr -UseBasicParsing -Verbose -OutFile windows2016_app_lifecycle.tgz https://ci.appveyor.com/api/buildjobs/pd548dr6m8900v8s/artifacts/output%2Fwindows_app_lifecycle-97ebc3a.tgz
echo @"
# Run this from a linux box with bosh_cli installed and access to bosh-lite

bosh -n target 192.168.50.4 lite
bosh -n login admin admin
bosh -n download manifest cf-warden-diego cf-warden-diego.yml
bosh deployment cf-warden-diego.yml

curl -o windows2016_app_lifecycle.tgz https://ci.appveyor.com/api/buildjobs/pd548dr6m8900v8s/artifacts/output%2Fwindows_app_lifecycle-97ebc3a.tgz
bosh scp access_z1/0 windows2016_app_lifecycle.tgz /tmp/windows_app_lifecycle.tgz  --upload
bosh ssh access_z1/0 -- sudo mkdir -p /var/vcap/jobs/file_server/packages/windows2016_app_lifecycle "&&" sudo cp /tmp/windows_app_lifecycle.tgz /var/vcap/jobs/file_server/packages/windows2016_app_lifecycle/windows2016_app_lifecycle.tgz

"@

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
cf create-buildpack cf-exe-buildpack cf-exe-buildpack 100 --enable

cd $wd
git clone https://github.com/cloudfoundry/wats
cd wats\assets\webapp
echo 'webapp.exe' | Out-File -Encoding ascii  run.bat
# cf push exeapp -s windows2016 -b https://github.com/hpcloud/cf-exe-buildpack -c webapp.exe
cf push exeapp -s windows2016
cf logs exeapp --recent


$gardenHcsPackage = "github.com/cloudfoundry-incubator/garden-windows"
$parentPath = Split-Path -Parent $gardenHcsPackage
$leafPath = Split-Path -Leaf $gardenHcsPackage
cd $env:GOPATH/src/$parentPath
git clone --local file:///C:/garden-hcs $leafPath

cd $env:GOPATH/src/$gardenHcsPackage
git pull
git remote add hpcloud https://github.com/hpcloud/garden-hcs
go get -v $gardenHcsPackage


$baseImagePath = (docker inspect microsoft/windowsservercore  | ConvertFrom-Json).GraphDriver.Data.Dir
$baseImageId = Split-Path -Leaf (docker inspect microsoft/windowsservercore  | ConvertFrom-Json).GraphDriver.Data.Dir

$machineIp = (Find-NetRoute -RemoteIPAddress "192.168.50.4")[0].IPAddress

echo  "$leafPath.exe -listenAddr 0.0.0.0:9241 -logLevel debug -cellIP $machineIp -baseImagePath $baseImagePath" | `
 Out-File -Encoding ascii  $wd\start-garden.bat
echo  "gaol /t 127.0.0.1:9241 list" | `
 Out-File -Encoding ascii  $wd\gaol-list.bat

echo " "
echo "Gatden client (gaol) usage example:"
echo " gaol /t 127.0.0.1:9241 list"
echo "Run garden with the following arguments:"
echo  " $leafPath.exe -listenAddr 0.0.0.0:9241 -logLevel debug -cellIP $machineIp -baseImagePath $baseImagePath"
