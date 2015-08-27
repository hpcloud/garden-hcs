package backend

//// importing C will increase the stack size. this is useful
//// if loading a .NET COM object, because the CLR requires a larger stack
import "C"

import (
	"strconv"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim"
	"github.com/cloudfoundry-incubator/garden"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-windows/container"
	"github.com/cloudfoundry-incubator/garden-windows/windows_containers"
)

type windowsContainerBackend struct {
	containerBinaryPath string
	containerRootPath   string
	hostIP              string

	logger lager.Logger

	containerIDs    <-chan string
	containers      map[string]garden.Container
	containersMutex *sync.RWMutex

	driverInfo        hcsshim.DriverInfo
	virtualSwitchName string
}

func NewWindowsContainerBackend(containerRootPath, virtualSwitchName string, logger lager.Logger, hostIP string) (*windowsContainerBackend, error) {
	logger.Debug("WCB: windowsContainerBackend.NewWindowsContainerBackend")

	containerIDs := make(chan string)

	go generateContainerIDs(containerIDs)

	return &windowsContainerBackend{
		containerRootPath: containerRootPath,
		hostIP:            hostIP,

		logger: logger,

		containerIDs:    containerIDs,
		containers:      make(map[string]garden.Container),
		containersMutex: new(sync.RWMutex),

		virtualSwitchName: virtualSwitchName,
		driverInfo:        windows_containers.NewDriverInfo(containerRootPath),
	}, nil
}

func (windowsContainerBackend *windowsContainerBackend) Start() error {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.Start")

	windowsContainerBackend.logger.Info("Windows container backend started")
	return nil
}

func (windowsContainerBackend *windowsContainerBackend) Stop() {
	windowsContainerBackend.logger.Info("Prison backend stopped")
}

func (windowsContainerBackend *windowsContainerBackend) GraceTime(garden.Container) time.Duration {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.GraceTime")
	// time after which to destroy idle containers
	return 0
}

func (windowsContainerBackend *windowsContainerBackend) Ping() error {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.Ping")
	return nil
}

func (windowsContainerBackend *windowsContainerBackend) Capacity() (garden.Capacity, error) {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.Capacity")

	// TODO: these values should not be hardcoded

	capacity := garden.Capacity{
		MemoryInBytes: 8 * 1024 * 1024 * 1024,
		DiskInBytes:   80 * 1024 * 1024 * 1024,
		MaxContainers: 100,
	}
	return capacity, nil
}

func (windowsContainerBackend *windowsContainerBackend) Create(containerSpec garden.ContainerSpec) (garden.Container, error) {
	windowsContainerBackend.logger.Info("WCB: backend is going to create a new container")

	id := <-windowsContainerBackend.containerIDs

	handle := id
	if containerSpec.Handle != "" {
		handle = containerSpec.Handle
	}

	container, err := container.NewContainer(
		id,
		handle,
		containerSpec,
		windowsContainerBackend.logger,
		windowsContainerBackend.hostIP,
		windowsContainerBackend.driverInfo,
		windowsContainerBackend.virtualSwitchName,
	)

	if err != nil {
		return nil, err
	}

	windowsContainerBackend.containersMutex.Lock()
	windowsContainerBackend.containers[handle] = container
	windowsContainerBackend.containersMutex.Unlock()

	return container, nil
}

func (windowsContainerBackend *windowsContainerBackend) Destroy(handle string) error {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.Destroy")

	if container, ok := windowsContainerBackend.containers[handle]; ok {
		container.Stop(true)
	}

	return nil
}

func (windowsContainerBackend *windowsContainerBackend) Containers(garden.Properties) (containers []garden.Container, err error) {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.Containers")
	windowsContainerBackend.containersMutex.RLock()
	defer windowsContainerBackend.containersMutex.RUnlock()

	for _, container := range windowsContainerBackend.containers {
		containers = append(containers, container)
	}

	return containers, nil
}

func (windowsContainerBackend *windowsContainerBackend) Lookup(handle string) (garden.Container, error) {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.Lookup")
	return windowsContainerBackend.containers[handle], nil
}

// BulkInfo returns info or error for a list of containers.
func (windowsContainerBackend *windowsContainerBackend) BulkInfo(handles []string) (map[string]garden.ContainerInfoEntry, error) {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.BulkInfo")

	result := make(map[string]garden.ContainerInfoEntry)

	for i := 0; i < len(handles); i++ {
		handle := handles[i]

		cont, err := windowsContainerBackend.Lookup(handle)

		if err != nil {
			result[handle] = garden.ContainerInfoEntry{
				Info: garden.ContainerInfo{},
				Err: &garden.Error{
					ErrorMsg: err.Error(),
				},
			}
			continue
		}

		if cont == nil {
			continue
		}

		contInfo, err := cont.Info()

		if err != nil {
			result[handle] = garden.ContainerInfoEntry{
				Info: garden.ContainerInfo{},
				Err: &garden.Error{
					ErrorMsg: err.Error(),
				},
			}
			continue
		}

		result[handle] = garden.ContainerInfoEntry{
			Info: contInfo,
			Err:  nil,
		}
	}

	return result, nil
}

// BulkMetrics returns metrics or error for a list of containers.
func (windowsContainerBackend *windowsContainerBackend) BulkMetrics(handles []string) (map[string]garden.ContainerMetricsEntry, error) {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.BulkMetrics")
	return map[string]garden.ContainerMetricsEntry{}, nil
}

func generateContainerIDs(ids chan<- string) string {
	for containerNum := time.Now().UnixNano(); ; containerNum++ {
		containerID := []byte{}

		var i uint
		for i = 0; i < 11; i++ {
			containerID = strconv.AppendInt(
				containerID,
				(containerNum>>(55-(i+1)*5))&31,
				32,
			)
		}

		ids <- string(containerID)
	}
}
