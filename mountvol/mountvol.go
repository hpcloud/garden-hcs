package mountvol

import (
	"os"
	"os/exec"
)

func MountVolume(volumeName, mountPath string) error {
	err := os.MkdirAll(mountPath, 0755)

	if err != nil {
		return err
	}

	cmd := exec.Command("mountvol", mountPath, volumeName)
	return cmd.Run()
}

func UnmountVolume(mountPath string) error {
	cmd := exec.Command("mountvol", mountPath, "/D")
	err := cmd.Run()

	if err != nil {
		return err
	}

	return os.RemoveAll(mountPath)
}
