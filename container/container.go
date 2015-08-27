package container

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/cloudfoundry-incubator/garden-windows/mountvol"
	"github.com/cloudfoundry-incubator/garden-windows/untar"
	"github.com/cloudfoundry-incubator/garden-windows/windows_client"
	"github.com/cloudfoundry-incubator/garden-windows/windows_containers"

	"code.google.com/p/go-uuid/uuid"
	"github.com/Microsoft/hcsshim"
	"github.com/cloudfoundry-incubator/garden"
	"github.com/pivotal-golang/lager"
)

const (
	ContainerPort = 8080
)

type WindowsContainerSpec struct {
	garden.ContainerSpec
}

type UndefinedPropertyError struct {
	Key string
}

func (err UndefinedPropertyError) Error() string {
	return fmt.Sprintf("property does not exist: %s", err.Key)
}

type container struct {
	id     string
	handle string
	logger lager.Logger
	hostIP string

	WindowsContainerSpec

	rpc  *rpc.Client
	pids map[int]*prison_client.ProcessTracker

	hostPort      int
	containerPort int

	runMutex        sync.Mutex
	propertiesMutex sync.RWMutex
	driverInfo      hcsshim.DriverInfo
	active          int
	virtualSwitch   string
	layerChain      []string
	volumeName      string
}

func NewContainer(id, handle string, containerSpec garden.ContainerSpec, logger lager.Logger, hostIP string, driverInfo hcsshim.DriverInfo, virtualSwitch string) (*container, error) {
	logger.Debug("WC: NewContainer")

	result := &container{
		id:     id,
		handle: handle,

		logger: logger,

		pids:          make(map[int]*prison_client.ProcessTracker),
		driverInfo:    driverInfo,
		active:        0,
		virtualSwitch: virtualSwitch,
	}

	result.RootFSPath = containerSpec.RootFSPath
	result.WindowsContainerSpec.Properties = containerSpec.Properties

	// We need to get an available port and map it now.
	// This is because there is currently no way to change network settings
	// for a compute system that's already been created.
	result.hostPort = freeTcp4Port()
	result.containerPort = ContainerPort

	// Get the shared base image based on our "RootFSPath"
	// This should be something like "WindowsServerCore"
	sharedBaseImage, err := windows_containers.GetSharedBaseImageByName(result.RootFSPath)
	if err != nil {
		return nil, err
	}

	// This implementation does not currently support a rootfs with multiple layers
	// So we create our layer chain based on the base shared image
	result.layerChain = []string{sharedBaseImage.Path}

	// Next, create a sandbox layer for the container
	// Our layer will have the same id as our container
	err = hcsshim.CreateSandboxLayer(driverInfo, id, result.layerChain[0], result.layerChain)
	if err != nil {
		return nil, err
	}

	// Retrieve the mount path for the new layer
	// This should be something like this: "\\?\Volume{bd05d44f-0000-0000-0000-100000000000}\"
	result.volumeName, err = result.getMountPathForLayer(id, result.layerChain)
	if err != nil {
		return nil, err
	}

	// Build a configuration json for creating a compute system
	configuration, err := result.getComputeSystemConfiguration(result.volumeName, result.layerChain)

	// Create a compute system.
	// This is our container.
	err = hcsshim.CreateComputeSystem(id, configuration)
	if err != nil {
		return nil, err
	}

	// Start the container
	err = hcsshim.StartComputeSystem(id)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (container *container) Handle() string {
	container.logger.Debug("WC: Handle")
	return container.handle
}

// Stop stops a container.
//
// If kill is false, garden stops a container by sending the processes running inside it the SIGTERM signal.
// It then waits for the processes to terminate before returning a response.
// If one or more processes do not terminate within 10 seconds,
// garden sends these processes the SIGKILL signal, killing them ungracefully.
//
// If kill is true, garden stops a container by sending the processing running inside it a SIGKILL signal.
//
// Once a container is stopped, garden does not allow spawning new processes inside the container.
// It is possible to copy files in to and out of a stopped container.
// It is only when a container is destroyed that its filesystem is cleaned up.
//
// Errors:
// * None.
func (container *container) Stop(kill bool) error {
	container.logger.Debug("WC: Stop")

	container.runMutex.Lock()
	defer container.runMutex.Unlock()

	// TODO: investigate how shutdown and terminate work.

	// Shutdown the compute system
	hcsshim.ShutdownComputeSystem(container.id)

	// Terminate the compute system
	hcsshim.TerminateComputeSystem(container.id)

	// Deactivate our layer
	hcsshim.DeactivateLayer(container.driverInfo, container.id)

	// Destroy the sandbox layer
	hcsshim.DestroyLayer(container.driverInfo, container.id)

	return nil
}

// Returns information about a container.
func (container *container) Info() (garden.ContainerInfo, error) {
	container.logger.Debug("WC: Info")

	properties, _ := container.Properties()

	result := garden.ContainerInfo{
		State:       "active",
		ExternalIP:  container.hostIP,
		HostIP:      container.hostIP,
		ContainerIP: container.hostIP,
		Events:      []string{"party"},
		ProcessIDs:  []uint32{},
		Properties:  properties,
		MappedPorts: []garden.PortMapping{
			garden.PortMapping{
				HostPort:      uint32(container.hostPort),
				ContainerPort: 8080,
			},
		},
	}

	container.logger.Info("Info called", lager.Data{
		"State":         result.State,
		"HostIP":        result.HostIP,
		"ContainerIP":   result.ContainerIP,
		"HostPort":      result.MappedPorts[0].HostPort,
		"ContainerPort": result.MappedPorts[0].ContainerPort,
	})

	return result, nil
}

// StreamIn streams data into a file in a container.
//
// Errors:
// *  TODO.
func (container *container) StreamIn(spec garden.StreamInSpec) error {
	container.logger.Debug("WC: StreamIn")

	// Get container directory
	layerFolder := container.dir(container.id)

	// Generate a temporary directory path
	tempFolder := layerFolder + "-" + uuid.New()

	// Mount the volume
	err := mountvol.MountVolume(container.volumeName, tempFolder)
	if err != nil {
		return err
	}

	// Dismount when we're done
	defer mountvol.UnmountVolume(tempFolder)

	// Write the tar stream to the directory
	outDir := filepath.Join(tempFolder, spec.Path)
	err = os.MkdirAll(outDir, 0755)
	if err != nil {
		return err
	}
	return untar.Untar(spec.TarStream, outDir)
}

// StreamOut streams a file out of a container.
//
// Errors:
// * TODO.
func (container *container) StreamOut(spec garden.StreamOutSpec) (io.ReadCloser, error) {
	container.logger.Debug("WC: StreamOut")

	// TODO: investigate a proper implementation
	// It is unclear if it's ok to keep the container mounted until the reader
	// is closed.

	return nil, nil
}

// Limits the network bandwidth for a container.
func (container *container) LimitBandwidth(limits garden.BandwidthLimits) error {
	container.logger.Debug("WC: LimitBandwidth")

	container.logger.Info("TODO LimitBandwidth")
	return nil
}

func (container *container) CurrentBandwidthLimits() (garden.BandwidthLimits, error) {
	container.logger.Info("TODO CurrentBandwidthLimits")
	return garden.BandwidthLimits{}, nil
}

// Limits the CPU shares for a container.
func (container *container) LimitCPU(limits garden.CPULimits) error {
	container.logger.Info("TODO LimitCPU")
	return nil
}

func (container *container) CurrentCPULimits() (garden.CPULimits, error) {
	container.logger.Info("TODO CurrentCPULimits")
	return garden.CPULimits{}, nil
}

// Limits the disk usage for a container.
//
// The disk limits that are set by this command only have effect for the container's unprivileged user.
// Files/directories created by its privileged user are not subject to these limits.
//
// TODO: explain how disk management works.
func (container *container) LimitDisk(limits garden.DiskLimits) error {
	container.logger.Info("TODO LimitDisk")
	return nil
}

func (container *container) CurrentDiskLimits() (garden.DiskLimits, error) {
	container.logger.Info("TODO CurrentDiskLimits")
	return garden.DiskLimits{}, nil
}

// Limits the memory usage for a container.
//
// The limit applies to all process in the container. When the limit is
// exceeded, the container will be automatically stopped.
//
// Errors:
// * The kernel does not support setting memory.memsw.limit_in_bytes.
func (container *container) LimitMemory(limits garden.MemoryLimits) error {
	container.logger.Info("TODO LimitMemory")
	return nil
}

func (container *container) CurrentMemoryLimits() (garden.MemoryLimits, error) {
	container.logger.Info("TODO CurrentMemoryLimits")
	return garden.MemoryLimits{}, nil
}

// Map a port on the host to a port in the container so that traffic to the
// host port is forwarded to the container port.
//
// If a host port is not given, a port will be acquired from the server's port
// pool.
//
// If a container port is not given, the port will be the same as the
// container port.
//
// The two resulting ports are returned in the response.
//
// Errors:
// * When no port can be acquired from the server's port pool.
func (container *container) NetIn(hostPort, containerPort uint32) (uint32, uint32, error) {
	container.logger.Info(fmt.Sprintf("TODO NetIn", hostPort, containerPort))

	return uint32(container.hostPort), ContainerPort, nil
}

// Whitelist outbound network traffic.
//
// If the configuration directive deny_networks is not used,
// all networks are already whitelisted and this command is effectively a no-op.
//
// Later NetOut calls take precedence over earlier calls, which is
// significant only in relation to logging.
//
// Errors:
// * An error is returned if the NetOut call fails.
func (container *container) NetOut(netOutRule garden.NetOutRule) error {
	container.logger.Info("TODO NetOut")
	return nil
}

// Run a script inside a container.
//
// The root user will be mapped to a non-root UID in the host unless the container (not this process) was created with 'privileged' true.
//
// Errors:
// * TODO.
func (container *container) Run(spec garden.ProcessSpec, pio garden.ProcessIO) (garden.Process, error) {

	container.runMutex.Lock()
	defer container.runMutex.Unlock()

	container.logger.Info(fmt.Sprintf("Run command: ", spec.Path, spec.Args, spec.Dir, spec.User, spec.Env))

	// Combine all arguments using ' ' as a separator
	concatArgs := ""
	for _, v := range spec.Args {
		concatArgs = concatArgs + " " + v
	}

	// Create the command line that we're going to run
	commandLine := fmt.Sprintf("%s %s", spec.Path, concatArgs)

	// TODO: Investigate what exactly EmulateConsole does,
	// as well as console size
	createProcessParms := hcsshim.CreateProcessParams{
		EmulateConsole:   false,
		WorkingDirectory: spec.Dir,
		ConsoleSize:      [2]int{80, 120},
		CommandLine:      commandLine,
	}

	// Create the process
	pid, stdin, stdout, stderr, err := hcsshim.CreateProcessInComputeSystem(
		container.id,
		pio.Stdin != nil,
		pio.Stdout != nil,
		pio.Stderr != nil,
		createProcessParms,
	)

	// Hook our stdin/stdout/stderr pipes
	go func() {
		if pio.Stdout != nil {
			io.Copy(pio.Stdout, stdout)
			stdout.Close()
		}
	}()

	go func() {
		if pio.Stderr != nil {
			io.Copy(pio.Stderr, stderr)
			stderr.Close()
		}
	}()

	go func() {
		if pio.Stdin != nil {
			io.Copy(stdin, pio.Stdin)
			stdin.Close()
		}
	}()
	if err != nil {
		return nil, err
	}

	// Create a new process tracker for the process we've just created
	pt := prison_client.NewProcessTracker(container.id, pid, container.driverInfo)

	container.logger.Debug("Container run created new process.", lager.Data{
		"PID": pt.ID(),
	})

	return pt, nil
}

// Attach starts streaming the output back to the client from a specified process.
//
// Errors:
// * processID does not refer to a running process.
func (container *container) Attach(processID uint32, io garden.ProcessIO) (garden.Process, error) {
	container.logger.Info(fmt.Sprintf("Attaching to: ", processID))

	cmd := container.pids[int(processID)]

	return cmd, nil
}

// Metrics returns the current set of metrics for a container
func (container *container) Metrics() (garden.Metrics, error) {
	container.logger.Info("TODO: implement Metrics()")
	return garden.Metrics{
		MemoryStat: garden.ContainerMemoryStat{},
		CPUStat:    garden.ContainerCPUStat{},
		DiskStat:   garden.ContainerDiskStat{},
	}, nil
}

// Properties returns the current set of properties
func (container *container) Properties() (garden.Properties, error) {
	container.propertiesMutex.RLock()
	defer container.propertiesMutex.RUnlock()

	container.logger.Info("getting-properties", lager.Data{
		"properties": container.WindowsContainerSpec.Properties,
	})

	return container.WindowsContainerSpec.Properties, nil
}

// Property returns the value of the property with the specified name.
//
// Errors:
// * When the property does not exist on the container.
func (container *container) Property(name string) (string, error) {
	container.logger.Info("getting-property", lager.Data{"name": name})

	container.propertiesMutex.RLock()
	defer container.propertiesMutex.RUnlock()

	value, found := container.WindowsContainerSpec.Properties[name]
	if !found {
		return "", UndefinedPropertyError{name}
	}

	return value, nil
}

// Set a named property on a container to a specified value.
//
// Errors:
// * None.
func (container *container) SetProperty(name string, value string) error {
	container.logger.Info("setting-property", lager.Data{"name": name})

	container.propertiesMutex.Lock()
	defer container.propertiesMutex.Unlock()

	props := garden.Properties{}
	for k, v := range container.WindowsContainerSpec.Properties {
		props[k] = v
	}

	props[name] = value

	container.WindowsContainerSpec.Properties = props

	return nil
}

// Remove a property with the specified name from a container.
//
// Errors:
// * None.
func (container *container) RemoveProperty(name string) error {
	container.logger.Info("removing-property", lager.Data{"name": name})

	container.propertiesMutex.Lock()
	defer container.propertiesMutex.Unlock()

	if _, found := container.WindowsContainerSpec.Properties[name]; !found {
		return UndefinedPropertyError{name}
	}

	delete(container.WindowsContainerSpec.Properties, name)

	return nil
}

func (container *container) destroy() error {
	container.logger.Debug("WC: Destroy")

	return nil
}

func freeTcp4Port() int {
	l, _ := net.Listen("tcp4", ":0")
	defer l.Close()
	freePort := strings.Split(l.Addr().String(), ":")[1]
	ret, _ := strconv.ParseUint(freePort, 10, 32)
	return int(ret)
}

// *************************************************************************
// This is where we start implementing things we need for windows containers
// Based on:
// https://github.com/docker/docker/blob/master/daemon/execdriver/windows/windows.go
// https://github.com/docker/docker/blob/master/daemon/execdriver/windows/run.go
// https://github.com/docker/docker/blob/master/daemon/graphdriver/windows/windows.go
// *************************************************************************

func (c *container) dir(id string) string {
	return filepath.Join(c.driverInfo.HomeDir, filepath.Base(id))
}

// Get returns the rootfs path for the id. This will mount the dir at it's given path
func (c *container) getMountPathForLayer(rId string, layerChain []string) (string, error) {
	var dir string

	if c.active == 0 {
		if err := hcsshim.ActivateLayer(c.driverInfo, rId); err != nil {
			return "", err
		}
		if err := hcsshim.PrepareLayer(c.driverInfo, rId, layerChain); err != nil {
			if err2 := hcsshim.DeactivateLayer(c.driverInfo, rId); err2 != nil {
				c.logger.Info(fmt.Sprintf("Failed to Deactivate %s: %s", rId, err))
			}
			return "", err
		}
	}

	mountPath, err := hcsshim.GetLayerMountPath(c.driverInfo, rId)
	if err != nil {
		if err2 := hcsshim.DeactivateLayer(c.driverInfo, rId); err2 != nil {
			c.logger.Info(fmt.Sprintf("Failed to Deactivate %s: %s", rId, err))
		}
		return "", err
	}

	c.active++

	// If the layer has a mount path, use that. Otherwise, use the
	// folder path.
	if mountPath != "" {
		dir = mountPath
	} else {
		dir = c.dir(rId)
	}

	return dir, nil
}

func (c *container) getComputeSystemConfiguration(mountPath string, layerChain []string) (string, error) {
	layerFolderPath := c.dir(c.id)

	// TODO: investigate further when IgnoreFlushesDuringBoot should be false
	cu := &windows_containers.ContainerInit{
		SystemType:              "Container",
		Name:                    c.id,
		Owner:                   windows_containers.DefaultOwner,
		IsDummy:                 false,
		VolumePath:              mountPath,
		IgnoreFlushesDuringBoot: true,
		LayerFolderPath:         layerFolderPath,
		Layers:                  []windows_containers.Layer{},
		Devices: []windows_containers.Device{
			c.getComputeSystemNetworkDevice(),
		},
	}

	for i := 0; i < len(layerChain); i++ {
		_, filename := filepath.Split(layerChain[i])
		g, err := hcsshim.NameToGuid(filename)
		if err != nil {
			return "", err
		}
		cu.Layers = append(cu.Layers, windows_containers.Layer{
			ID:   g.ToString(),
			Path: layerChain[i],
		})
	}

	configurationb, err := json.Marshal(cu)
	if err != nil {
		return "", err
	}

	configuration := string(configurationb)

	return configuration, nil
}

// This is a very basic implementation for port mappings
// TODO: investigate how we can improve this, perhaps try to configure
// port mappings on NetIn, not when the compute service is created
func (c *container) getComputeSystemNetworkDevice() windows_containers.Device {

	protocol := "TCP"
	pbs := []windows_containers.PortBinding{
		windows_containers.PortBinding{
			ExternalPort: c.hostPort,
			InternalPort: c.containerPort,
			Protocol:     protocol,
		},
	}

	dev := windows_containers.Device{
		DeviceType: "Network",
		Connection: &windows_containers.NetworkConnection{
			NetworkName: c.virtualSwitch,
			// TODO Windows: Fixme, next line. Needs HCS fix.
			EnableNat: false,
			Nat: windows_containers.NatSettings{
				Name:         windows_containers.DefaultContainerNAT,
				PortBindings: pbs,
			},
		},
	}

	return dev
}
