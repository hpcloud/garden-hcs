package backend

import (
	"sync"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/lager"
	"github.com/Microsoft/hcsshim"
	"github.com/docker/docker/pkg/stringid"

	"github.com/hpcloud/garden-hcs/container"
	"github.com/hpcloud/garden-hcs/windows_containers"
)

type windowsContainerBackend struct {
	garden.Backend

	containerRootPath string
	hostIP            string

	logger lager.Logger

	//	containerIDs    <-chan string
	containers      map[string]*container.WindowsContainer
	containersMutex *sync.RWMutex

	driverInfo        hcsshim.DriverInfo
	baseImagePath     string
	virtualSwitchName string
}

func NewWindowsContainerBackend(containerRootPath, baseImagePath string, logger lager.Logger, hostIP string) (*windowsContainerBackend, error) {
	logger.Debug("WCB: windowsContainerBackend.NewWindowsContainerBackend")

	//	containerIDs := make(chan string)
	//	go generateContainerIDs(containerIDs)

	return &windowsContainerBackend{
		containerRootPath: containerRootPath,
		hostIP:            hostIP,

		logger: logger,

		// containerIDs:    containerIDs,
		containers:      make(map[string]*container.WindowsContainer),
		containersMutex: new(sync.RWMutex),

		driverInfo:    windows_containers.NewDriverInfo(baseImagePath),
		baseImagePath: baseImagePath,
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

	// TODO: not implemented
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

	// id := <-windowsContainerBackend.containerIDs
	id := stringid.GenerateNonCryptoID()

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
		windowsContainerBackend.baseImagePath,
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
		err := container.Destroy()
		if err != nil {
			return err
		}

		windowsContainerBackend.containersMutex.Lock()
		delete(windowsContainerBackend.containers, handle)
		windowsContainerBackend.containersMutex.Unlock()
	}

	return nil
}

func (windowsContainerBackend *windowsContainerBackend) Containers(filterProperties garden.Properties) (containers []garden.Container, err error) {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.Containers", lager.Data{"filterProperties": filterProperties})
	windowsContainerBackend.containersMutex.RLock()
	defer windowsContainerBackend.containersMutex.RUnlock()

	for _, c := range windowsContainerBackend.containers {
		allPropertiesMatch := true
		for filterKey, filterValue := range filterProperties {
			if actualValue, err := c.Property(filterKey); err != nil || actualValue != filterValue {
				allPropertiesMatch = false
				break
			}
		}

		if allPropertiesMatch {
			containers = append(containers, c)
		}
	}

	return containers, nil
}

func (windowsContainerBackend *windowsContainerBackend) Lookup(handle string) (garden.Container, error) {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.Lookup")

	res, found := windowsContainerBackend.containers[handle]
	if found {
		return res, nil
	}

	return nil, garden.ContainerNotFoundError{handle}
}

// BulkInfo returns info or error for a list of containers.
func (windowsContainerBackend *windowsContainerBackend) BulkInfo(handles []string) (map[string]garden.ContainerInfoEntry, error) {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.BulkInfo")

	result := make(map[string]garden.ContainerInfoEntry)

	for _, handle := range handles {
		var err error
		cont, err := windowsContainerBackend.Lookup(handle)

		if err == nil {
			contInfo, err := cont.Info()

			if err == nil {
				result[handle] = garden.ContainerInfoEntry{
					Info: contInfo,
					Err:  nil,
				}
			}
		}

		if err != nil {
			result[handle] = garden.ContainerInfoEntry{
				Info: garden.ContainerInfo{},
				Err: &garden.Error{
					Err: err,
				},
			}
		}
	}

	return result, nil
}

// BulkMetrics returns metrics or error for a list of containers.
func (windowsContainerBackend *windowsContainerBackend) BulkMetrics(handles []string) (map[string]garden.ContainerMetricsEntry, error) {
	windowsContainerBackend.logger.Debug("WCB: windowsContainerBackend.BulkMetrics")

	result := make(map[string]garden.ContainerMetricsEntry)

	for _, handle := range handles {
		var err error
		cont, err := windowsContainerBackend.Lookup(handle)

		if err == nil {
			metrics, err := cont.Metrics()

			if err == nil {
				result[handle] = garden.ContainerMetricsEntry{
					Metrics: metrics,
					Err:     nil,
				}
			}
		}

		if err != nil {
			result[handle] = garden.ContainerMetricsEntry{
				Metrics: garden.Metrics{},
				Err: &garden.Error{
					Err: err,
				},
			}
		}
	}

	return result, nil
}
