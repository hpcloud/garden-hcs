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
	"github.com/mattn/go-ole"
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
	err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)

	prisonBackend.logger.Info("prison backend started")

	return err
}

func (prisonBackend *prisonBackend) Stop() {
	prisonBackend.logger.Info("prison backend stopped")
}

func (prisonBackend *prisonBackend) GraceTime(garden.Container) time.Duration {
	// time after which to destroy idle containers
	return 0
}

func (prisonBackend *prisonBackend) Ping() error {
	return nil
}

func (prisonBackend *prisonBackend) Capacity() (garden.Capacity, error) {
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

	container := container.NewContainer(id, handle, prisonBackend.containerRootPath)

	prisonBackend.containersMutex.Lock()
	prisonBackend.containers[handle] = container
	prisonBackend.containersMutex.Unlock()

	return container, nil
}

func (prisonBackend *prisonBackend) Destroy(handle string) error {
	return nil
}

func (prisonBackend *prisonBackend) Containers(garden.Properties) (containers []garden.Container, err error) {
	prisonBackend.containersMutex.RLock()
	defer prisonBackend.containersMutex.RUnlock()

	for _, container := range prisonBackend.containers {
		containers = append(containers, container)
	}

	return containers, nil
}

func (prisonBackend *prisonBackend) Lookup(handle string) (garden.Container, error) {
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

//type dotNetBackend struct {
//	containerizerURL url.URL
//	logger           lager.Logger
//}

//func NewDotNetBackend(containerizerURL string, logger lager.Logger) (*dotNetBackend, error) {
//	u, err := url.Parse(containerizerURL)
//	if err != nil {
//		return nil, err
//	}
//	return &dotNetBackend{
//		containerizerURL: *u,
//		logger:           logger,
//	}, nil
//}

//func (dotNetBackend *dotNetBackend) ContainerizerURL() string {
//	return dotNetBackend.containerizerURL.String()
//}

//func (dotNetBackend *dotNetBackend) Start() error {
//	return nil
//}

//func (dotNetBackend *dotNetBackend) Stop() {}

//func (dotNetBackend *dotNetBackend) GraceTime(garden.Container) time.Duration {
//	// FIXME -- what should this do.
//	return time.Hour
//}

//func (dotNetBackend *dotNetBackend) Ping() error {
//	resp, err := http.Get(dotNetBackend.containerizerURL.String() + "/api/ping")
//	if err != nil {
//		return err
//	}
//	resp.Body.Close()
//	return nil
//}

//func (dotNetBackend *dotNetBackend) Capacity() (garden.Capacity, error) {
//	capacity := garden.Capacity{
//		MemoryInBytes: 8 * 1024 * 1024 * 1024,
//		DiskInBytes:   80 * 1024 * 1024 * 1024,
//		MaxContainers: 100,
//	}
//	return capacity, nil
//}

//func (dotNetBackend *dotNetBackend) Create(containerSpec garden.ContainerSpec) (garden.Container, error) {
//	url := dotNetBackend.containerizerURL.String() + "/api/containers"
//	containerSpecJSON, err := json.Marshal(containerSpec)
//	if err != nil {
//		return nil, err
//	}
//	resp, err := http.Post(url, "application/json", strings.NewReader(string(containerSpecJSON)))
//	if err != nil {
//		return nil, err
//	}
//	resp.Body.Close()

//	netContainer := container.NewContainer(dotNetBackend.containerizerURL, containerSpec.Handle, dotNetBackend.logger)
//	return netContainer, nil
//}

//func (dotNetBackend *dotNetBackend) Destroy(handle string) error {
//	url := dotNetBackend.containerizerURL.String() + "/api/containers/" + handle

//	req, err := http.NewRequest("DELETE", url, nil)
//	if err != nil {
//		return err
//	}

//	resp, err := http.DefaultClient.Do(req)
//	if err != nil {
//		return err
//	}
//	resp.Body.Close()

//	return nil
//}

//func (dotNetBackend *dotNetBackend) Containers(garden.Properties) ([]garden.Container, error) {
//	url := dotNetBackend.containerizerURL.String() + "/api/containers"
//	response, err := http.Get(url)
//	if err != nil {
//		return nil, err
//	}
//	defer response.Body.Close()

//	var ids []string
//	body, err := ioutil.ReadAll(response.Body)
//	if err != nil {
//		return nil, err
//	}
//	err = json.Unmarshal(body, &ids)

//	containers := []garden.Container{}
//	for _, containerId := range ids {
//		containers = append(containers, container.NewContainer(dotNetBackend.containerizerURL, containerId, dotNetBackend.logger))
//	}
//	return containers, nil
//}

//func (dotNetBackend *dotNetBackend) Lookup(handle string) (garden.Container, error) {
//	netContainer := container.NewContainer(dotNetBackend.containerizerURL, handle, dotNetBackend.logger)
//	return netContainer, nil
//}
