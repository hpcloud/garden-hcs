$wd="C:\diego-kit"
mkdir -Force $wd
cd $wd


# Dependencies
iex ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1'))

choco install processhacker git mingw golang ruby win32-openssh -y

$env:GOPATH = "C:\gopath"
mkdir -f "$env:GOPATH\bin"
setx /m GOPATH $env:GOPATH
if ([Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine) -inotlike "*$env:GOPATH\bin*") {
  [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine) + ";$env:GOPATH\bin;", [EnvironmentVariableTarget]::Machine)
}

iwr -OutFile $wd\cf-cli.zip  "https://cli.run.pivotal.io/stable?release=windows64&source=github-rel"
Expand-Archive $wd\cf-cli.zip  -DestinationPath . -Force
& $wd\cf_installer.exe /VERYSILENT /NORESTART

iwr -OutFile $wd\nssm.zip  "https://nssm.cc/ci/nssm-2.24-87-g203bfae.zip"
Expand-Archive $wd\nssm.zip  -DestinationPath $wd\. -Force
cp $wd\nssm-2.24-87-g203bfae\win64\nssm.exe $env:SystemRoot
