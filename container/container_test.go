package container

import (
	"bytes"
	"flag"
	"os"
	"testing"

	"code.google.com/p/go-uuid/uuid"
	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/garden"
	"github.com/stretchr/testify/assert"

	"github.com/cloudfoundry-incubator/garden-windows/windows_containers"
)

func TestMain(m *testing.M) {
	cf_lager.AddFlags(flag.CommandLine)

	retCode := m.Run()

	os.Exit(retCode)
}

func TestCreateContainer(t *testing.T) {
	assert := assert.New(t)

	logger, _ := cf_lager.New("windows-garden-tests")

	id := uuid.New()
	handle := id
	rootPath := "WindowsServerCore"
	hostIP := "127.0.0.1"
	driverInfo := windows_containers.NewDriverInfo("c:\\garden-windows\\tests")
	properties := garden.Properties{}

	container, err := NewContainer(id, handle, rootPath, logger, hostIP, driverInfo, properties)
	defer container.Stop(true)

	assert.Nil(err)

}

func TestRunInContainer(t *testing.T) {
	assert := assert.New(t)

	logger, _ := cf_lager.New("windows-garden-tests")

	id := uuid.New()
	handle := id
	rootPath := "WindowsServerCore"
	hostIP := "127.0.0.1"
	driverInfo := windows_containers.NewDriverInfo("c:\\garden-windows\\tests")
	properties := garden.Properties{}

	container, err := NewContainer(id, handle, rootPath, logger, hostIP, driverInfo, properties)
	defer container.Stop(true)

	assert.Nil(err)

	processSpec := garden.ProcessSpec{
		Path: "cmd.exe",
		Args: []string{"/v", "ver"},
		Env:  []string{},
		Dir:  "c:\\",
	}

	pio := garden.ProcessIO{
		Stdin:  nil,
		Stdout: nil,
		Stderr: nil,
	}

	pt, err := container.Run(processSpec, pio)
	assert.Nil(err)

	exitCode, err := pt.Wait()

	assert.Nil(err)
	assert.Equal(0, exitCode)
}

func TestRunInContainerWithOutput(t *testing.T) {
	assert := assert.New(t)

	logger, _ := cf_lager.New("windows-garden-tests")

	id := uuid.New()
	handle := id
	rootPath := "WindowsServerCore"
	hostIP := "127.0.0.1"
	driverInfo := windows_containers.NewDriverInfo("c:\\garden-windows\\tests")
	properties := garden.Properties{}

	container, err := NewContainer(id, handle, rootPath, logger, hostIP, driverInfo, properties)
	defer container.Stop(true)

	assert.Nil(err)

	processSpec := garden.ProcessSpec{
		Path: "cmd.exe",
		Args: []string{"/v", "ver"},
		Env:  []string{},
		Dir:  "c:\\",
	}

	stdout := bytes.NewBufferString("")

	pio := garden.ProcessIO{
		Stdin:  nil,
		Stdout: stdout,
		Stderr: nil,
	}

	pt, err := container.Run(processSpec, pio)
	assert.Nil(err)

	exitCode, err := pt.Wait()

	assert.Nil(err)
	assert.Equal(0, exitCode)

	output := stdout.String()
	assert.Contains(output, "Windows")
}
