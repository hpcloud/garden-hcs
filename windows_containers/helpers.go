package windows_containers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim"
)

// defaultOwner is a tag passed to HCS to allow it to differentiate between
// container creator management stacks. We hard code "garden-windows" in the case
// of garden-windows.
const DefaultOwner = "garden-windows"

const filterDriver = 1

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
func CreateAndActivateContainerLayer(di hcsshim.DriverInfo, containerLayerId string, layerChain []string) (string, string, error) {
	var err error

	err = hcsshim.CreateSandboxLayer(di, containerLayerId, layerChain[0], layerChain)
	if err != nil {
		return "", "", err
	}

	err = hcsshim.ActivateLayer(di, containerLayerId)
	if err != nil {
		return "", "", err
	}

	err = hcsshim.PrepareLayer(di, containerLayerId, layerChain)
	if err != nil {
		return "", "", err
	}

	volumeMountPath, err := hcsshim.GetLayerMountPath(di, containerLayerId)
	if err != nil {
		return "", "", err
	}

	return GetLayerPath(di, containerLayerId), volumeMountPath, nil
}

func GetLayerChain(layerPath string) ([]string, error) {
	// Ref: https://github.com/docker/docker/blob/34cc19f6702c23b2ae4aad2b169ca64154404f9f/daemon/graphdriver/windows/windows.go#L752-L768

	jPath := filepath.Join(layerPath, "layerchain.json")
	content, err := ioutil.ReadFile(jPath)
	if os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("Unable to read layerchain file - %s", err)
	}

	var layerChain []string
	err = json.Unmarshal(content, &layerChain)
	if err != nil {
		return nil, fmt.Errorf("Failed to unmarshall layerchain json - %s", err)
	}

	layerChain = append([]string{layerPath}, layerChain...)

	return layerChain, nil
}
