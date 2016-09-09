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

git apply << EOM
diff --git a/manifest-generation/diego.yml b/manifest-generation/diego.yml
index b20b724..9d64b4f 100644
--- a/manifest-generation/diego.yml
+++ b/manifest-generation/diego.yml
@@ -602,6 +602,12 @@ properties:
       dropsonde_port: (( config_from_cf.metron_agent.dropsonde_incoming_port ))
       log_level: (( property_overrides.cc_uploader.log_level || nil ))
     nsync:
+      lifecycle_bundles:
+        - "buildpack/cflinuxfs2:buildpack_app_lifecycle/buildpack_app_lifecycle.tgz"
+        - "buildpack/windows2012R2:windows_app_lifecycle/windows_app_lifecycle.tgz"
+        - "buildpack/windows2016:windows2016_app_lifecycle/windows2016_app_lifecycle.tgz"
+        - "docker:docker_app_lifecycle/docker_app_lifecycle.tgz"
+
       diego_privileged_containers: (( property_overrides.nsync.diego_privileged_containers || nil ))
       dropsonde_port: (( config_from_cf.metron_agent.dropsonde_incoming_port ))
       bbs:
@@ -618,6 +624,12 @@ properties:
         basic_auth_password: (( config_from_cf.cc.internal_api_password ))
       log_level: (( property_overrides.nsync.log_level || nil ))
     stager:
+      lifecycle_bundles:
+        - "buildpack/cflinuxfs2:buildpack_app_lifecycle/buildpack_app_lifecycle.tgz"
+        - "buildpack/windows2012R2:windows_app_lifecycle/windows_app_lifecycle.tgz"
+        - "buildpack/windows2016:windows2016_app_lifecycle/windows2016_app_lifecycle.tgz"
+        - "docker:docker_app_lifecycle/docker_app_lifecycle.tgz"
+
       diego_privileged_containers: (( property_overrides.stager.diego_privileged_containers || nil ))
       dropsonde_port: (( config_from_cf.metron_agent.dropsonde_incoming_port ))
       cc:
EOM

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
