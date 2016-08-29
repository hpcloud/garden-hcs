package windows_containers

import (
	"path/filepath"

	"github.com/Microsoft/hcsshim"
)

const (
	filterDriver = 1
)

func NewDriverInfo(homeDir string) hcsshim.DriverInfo {
	return hcsshim.DriverInfo{
		HomeDir: homeDir,
		Flavour: filterDriver,
	}
}

func GetLayerId(layerPath string) string {
	return filepath.Base(layerPath)
}

func GetLayerPath(di hcsshim.DriverInfo, layerId string) string {
	return filepath.Join(di.HomeDir, layerId)
}

// Returns: LayerFolderPaht, VolumePath
func CreateAndActivateContainerLayer(di hcsshim.DriverInfo, containerLayerId, parentLayerPath string) (string, string, error) {
	var err error

	err = hcsshim.CreateSandboxLayer(di, containerLayerId, parentLayerPath, []string{parentLayerPath})
	if err != nil {
		return "", "", err
	}

	err = hcsshim.ActivateLayer(di, containerLayerId)
	if err != nil {
		return "", "", err
	}

	err = hcsshim.PrepareLayer(di, containerLayerId, []string{parentLayerPath})
	if err != nil {
		return "", "", err
	}

	volumeMountPath, err := hcsshim.GetLayerMountPath(di, containerLayerId)
	if err != nil {
		return "", "", err
	}

	return GetLayerPath(di, containerLayerId), volumeMountPath, nil
}
