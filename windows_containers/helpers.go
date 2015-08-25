package windows_containers

import (
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"path/filepath"

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
		result[image.Name] = image
	}

	return result, nil
}

func GetSharedBaseImageByName(name string) (*SharedBaseImage, error) {
	images, err := GetSharedBaseImages()

	if err != nil {
		return nil, err
	}

	if image, ok := images[name]; ok {
		return &image, nil
	}

	return nil, fmt.Errorf("Could not find shared base image %s.", name)
}

func NewDriverInfo(homeDir string) hcsshim.DriverInfo {
	return hcsshim.DriverInfo{
		HomeDir: homeDir,
		Flavour: filterDriver,
	}
}

func (image *SharedBaseImage) GetId() string {
	_, folderName := filepath.Split(image.Path)
	h := sha512.Sum384([]byte(folderName))
	id := fmt.Sprintf("%x", h[:32])

	return id
}
