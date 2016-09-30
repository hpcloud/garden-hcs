$wd="C:\diego-kit"
mkdir -Force $wd
cd $wd

# Disable Windows Defender for extra performance
Write-Output "Disable Windows-Defender"
Set-MpPreference -DisableRealtimeMonitoring $true
# Remove-WindowsFeature -Name Windows-Defender

# Dependencies
iex ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1'))


# Install golang

choco install processhacker git mingw ruby win32-openssh visualstudiocode -y

choco install golang --version 1.7.1 -y

$env:GOPATH = "C:\gopath"
mkdir -f "$env:GOPATH\bin"
setx /m GOPATH $env:GOPATH
if ([Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine) -inotlike "*$env:GOPATH\bin*") {
  [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine) + ";$env:GOPATH\bin", [EnvironmentVariableTarget]::Machine)
}

# Install nssm cli
if (-not (Test-Path "$env:SystemRoot\nssm.exe")) {
  iwr -OutFile $wd\nssm.zip  "https://nssm.cc/ci/nssm-2.24-87-g203bfae.zip"
  Expand-Archive $wd\nssm.zip  -DestinationPath $wd\. -Force
  cp $wd\nssm-2.24-87-g203bfae\win64\nssm.exe $env:SystemRoot
}

# Install liteide
if (-not (Test-Path "$env:ProgramFiles\liteide")) {
  Invoke-WebRequest -OutFile "$env:TEMP\liteide-windows.zip" -UseBasicParsing "http://vorboss.dl.sourceforge.net/project/liteide/X30.2/liteidex30.2.windows-qt5.zip"
  Expand-Archive -Path "$env:TEMP\liteide-windows.zip" -DestinationPath $env:ProgramFiles

  if ([Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine) -inotlike "*$env:ProgramFiles\liteide\bin*") {
    [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine) + ";$env:ProgramFiles\liteide\bin", [EnvironmentVariableTarget]::Machine)
  }

  $TargetFile = "$env:ProgramFiles\liteide\bin\liteide.exe"
  $ShortcutFile = "$env:Public\Desktop\Liteide.lnk"
  $WScriptShell = New-Object -ComObject WScript.Shell
  $Shortcut = $WScriptShell.CreateShortcut($ShortcutFile)
  $Shortcut.TargetPath = $TargetFile
  $Shortcut.Save()

  # Run as admin http://stackoverflow.com/questions/28997799/how-to-create-a-run-as-administrator-shortcut-using-powershell
  $bytes = [System.IO.File]::ReadAllBytes($ShortcutFile)
  $bytes[0x15] = $bytes[0x15] -bor 0x20 #set byte 21 (0x15) bit 6 (0x20) ON
  [System.IO.File]::WriteAllBytes($ShortcutFile, $bytes)
}

# Install cf cli
iwr -OutFile $wd\cf-cli.zip  "https://cli.run.pivotal.io/stable?release=windows64&source=github-rel"
Expand-Archive $wd\cf-cli.zip  -DestinationPath . -Force
& $wd\cf_installer.exe /VERYSILENT /NORESTART
