package backend

//// importing C will increase the stack size. this is useful
//// if loading a .NET COM object, because the CLR requires a larger stack
import "C"

import (
	"strconv"
	"sync"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-windows/container"
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/pivotal-golang/lager"
)

type prisonBackend struct {
	containerBinaryPath string
	containerRootPath   string

	logger lager.Logger

	containerIDs    <-chan string
	containers      map[string]garden.Container
	containersMutex *sync.RWMutex
}

func NewPrisonBackend(containerRootPath string, logger lager.Logger) (*prisonBackend, error) {
	logger.Debug("PELERINUL: prisonBackend.NewPrisonBackend")

	containerIDs := make(chan string)

	go generateContainerIDs(containerIDs)

	return &prisonBackend{
		containerRootPath: containerRootPath,

		logger: logger,

		containerIDs:    containerIDs,
		containers:      make(map[string]garden.Container),
		containersMutex: new(sync.RWMutex),
	}, nil
}

func (prisonBackend *prisonBackend) Start() error {
	prisonBackend.logger.Debug("PELERINUL: prisonBackend.Start")

	err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)

	if err != nil {
		return err
	}

	containerManager, err := oleutil.CreateObject("CloudFoundry.WindowsPrison.ComWrapper.ContainerManager")

	if err != nil {
		return err
	}

	iContainerManager, err := containerManager.QueryInterface(ole.IID_IDispatch)

	if err != nil {
		return err
	}

	oleutil.CallMethod(iContainerManager, "InitPrison")

	prisonBackend.logger.Info("prison backend started")

	return nil
}

func (prisonBackend *prisonBackend) Stop() {
	prisonBackend.logger.Info("prison backend stopped")
}

func (prisonBackend *prisonBackend) GraceTime(garden.Container) time.Duration {
	prisonBackend.logger.Debug("PELERINUL: prisonBackend.GraceTime")
	// time after which to destroy idle containers
	return 0
}

func (prisonBackend *prisonBackend) Ping() error {
	prisonBackend.logger.Debug("PELERINUL: prisonBackend.Ping")
	return nil
}

func (prisonBackend *prisonBackend) Capacity() (garden.Capacity, error) {
	prisonBackend.logger.Debug("PELERINUL: prisonBackend.Capacity")

	capacity := garden.Capacity{
		MemoryInBytes: 8 * 1024 * 1024 * 1024,
		DiskInBytes:   80 * 1024 * 1024 * 1024,
		MaxContainers: 100,
	}
	return capacity, nil
}

func (prisonBackend *prisonBackend) Create(containerSpec garden.ContainerSpec) (garden.Container, error) {
	prisonBackend.logger.Info("prison backend is going to create a new container")

	id := <-prisonBackend.containerIDs

	handle := id
	if containerSpec.Handle != "" {
		handle = containerSpec.Handle
	}

	container := container.NewContainer(id, handle, prisonBackend.containerRootPath, prisonBackend.logger)

	prisonBackend.containersMutex.Lock()
	prisonBackend.containers[handle] = container
	prisonBackend.containersMutex.Unlock()

	return container, nil
}

func (prisonBackend *prisonBackend) Destroy(handle string) error {
	prisonBackend.logger.Debug("PELERINUL: prisonBackend.Destroy")
	return nil
}

func (prisonBackend *prisonBackend) Containers(garden.Properties) (containers []garden.Container, err error) {
	prisonBackend.logger.Debug("PELERINUL: prisonBackend.Containers")
	prisonBackend.containersMutex.RLock()
	defer prisonBackend.containersMutex.RUnlock()

	for _, container := range prisonBackend.containers {
		containers = append(containers, container)
	}

	return containers, nil
}

func (prisonBackend *prisonBackend) Lookup(handle string) (garden.Container, error) {
	prisonBackend.logger.Debug("PELERINUL: prisonBackend.Lookup")
	return prisonBackend.containers[handle], nil
}

// BulkInfo returns info or error for a list of containers.
func (prisonBackend *prisonBackend) BulkInfo(handles []string) (map[string]garden.ContainerInfoEntry, error) {
	prisonBackend.logger.Debug("PELERINUL: prisonBackend.BulkInfo")
	return nil, nil
}

// BulkMetrics returns metrics or error for a list of containers.
func (prisonBackend *prisonBackend) BulkMetrics(handles []string) (map[string]garden.ContainerMetricsEntry, error) {
	prisonBackend.logger.Debug("PELERINUL: prisonBackend.BulkMetrics")
	return nil, nil
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
