# Windows 2016 Cell for Cloud Foundry with bosh-lite

## Requirements
- Running Bosh Lite - https://github.com/cloudfoundry/bosh-lite
- Cloud Foundry deployment on Bosh Lite - https://github.com/cloudfoundry/cf-release
- Diego deployment ( v0.1484.0 ) on Bosh Lite - https://github.com/cloudfoundry/diego-release/tree/develop/examples/bosh-lite
- Vagrant ( >= 1.8.5) and Virtualbox
- Add windows2016 to diego lifecycle_bundles: "buildpack/windows2016:windows2016_app_lifecycle/windows2016_app_lifecycle.tgz"
```
# Run the following script to change the diego deployment manifest to add the extra windows2016 lifecycle_bundle
cd ~/workspace/diego-release
curl https://gist.githubusercontent.com/stefanschneider/96c509bdeb1197ed99eba174d4b95d0c/raw/36e95b722697724b61011f48463f478b12464ced/windows2016_stack.patch  | git apply
./scripts/generate-bosh-lite-manifests
bosh -n --deployment ~/workspace/diego-release/bosh-lite/deployments/diego.yml deploy
```

- Upload the windows2016 lifecycle to "access_z1" bosh job
```
# Use the following script to upload the new lifecycle.
# N.B. Run this script again if the bosh job is restarted or recreated

cd /tmp
bosh -n target 192.168.50.4 lite
bosh -n login admin admin
bosh -n download manifest cf-warden-diego cf-warden-diego.yml
bosh deployment cf-warden-diego.yml

curl -L -o windows2016_app_lifecycle.tgz "https://ci.appveyor.com/api/buildjobs/pd548dr6m8900v8s/artifacts/output%2Fwindows_app_lifecycle-97ebc3a.tgz"
bosh scp access_z1/0 windows2016_app_lifecycle.tgz /tmp/windows2016_app_lifecycle.tgz  --upload
bosh ssh access_z1/0 -- sudo mkdir -p /var/vcap/jobs/file_server/packages/windows2016_app_lifecycle "&&" sudo cp /tmp/windows2016_app_lifecycle.tgz /var/vcap/jobs/file_server/packages/windows2016_app_lifecycle/windows2016_app_lifecycle.tgz
```

## Usage
Run `vagrant up` from the dev-box directory and wait for the deployment to complete.
To access the Windows VM use `vagrant rdp`, or connect directly to `192.168.50.16` using Remote Desktop Connection.
