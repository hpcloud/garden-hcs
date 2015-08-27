package windows_containers

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim"
)

const (
	diffDriver = iota
	filterDriver
)

func GetSharedBaseImages() (map[string]SharedBaseImage, error) {
	imageData, err := hcsshim.GetSharedBaseImages()
	result := map[string]SharedBaseImage{}

	if err != nil {
		return nil, err
	}

	images := SharedBaseImages{}

	json.Unmarshal([]byte(imageData), &images)

	for _, image := range images.Images {
		// We need to normalize our image names, since Diego
		// does some matching with URIs
		result[strings.ToLower(image.Name)] = image
	}

	return result, nil
}

func GetSharedBaseImageByName(name string) (*SharedBaseImage, error) {
	// Normalize image name
	name = strings.ToLower(name)

	images, err := GetSharedBaseImages()

	if err != nil {
		return nil, err
	}

	if image, ok := images[name]; ok {
		return &image, nil
	}

	return nil, fmt.Errorf("Could not find shared base image '%s'.", name)
}

func NewDriverInfo(homeDir string) hcsshim.DriverInfo {
	return hcsshim.DriverInfo{
		HomeDir: homeDir,
		// There are two types of drivers for container support on Windows
		// filterDriver and diffDriver
		// So far, only the filterDriver seems to work ok.
		// It also looks like this is the driver used for Docker on Windows
		Flavour: filterDriver,
	}
}

func (image *SharedBaseImage) GetId() string {
	_, folderName := filepath.Split(image.Path)

	return folderName
}
