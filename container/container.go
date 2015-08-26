package container

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/cloudfoundry-incubator/garden-windows/windows_client"
	"github.com/cloudfoundry-incubator/garden-windows/windows_containers"

	"github.com/Microsoft/hcsshim"
	"github.com/cloudfoundry-incubator/garden"
	"github.com/pivotal-golang/lager"
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

	rpc             *rpc.Client
	pids            map[int]*prison_client.ProcessTracker
	lastNetInPort   uint32
	runMutex        sync.Mutex
	propertiesMutex sync.RWMutex
	driverInfo      hcsshim.DriverInfo
	active          int
}

func NewContainer(id, handle, rootfs string, logger lager.Logger, hostIP string, driverInfo hcsshim.DriverInfo, properties garden.Properties) (*container, error) {
	logger.Debug("WC: NewContainer")

	result := &container{
		id:     id,
		handle: handle,

		hostIP: hostIP,
		logger: logger,

		pids:       make(map[int]*prison_client.ProcessTracker),
		driverInfo: driverInfo,
		active:     0,
	}

	result.RootFSPath = rootfs
	result.WindowsContainerSpec.Properties = properties

	// Get the shared base image based on our "RootFSPath"
	// This should be something like "WindowsServerCore"
	sharedBaseImage, err := windows_containers.GetSharedBaseImageByName(result.RootFSPath)
	if err != nil {
		return nil, err
	}

	// This implementation does not currently support a rootfs with multiple layers
	// So we create our layer chain based on the base shared image
	layerChain := []string{sharedBaseImage.Path}

	// Next, create a sandbox layer for the container
	// Our layer will have the same id as our container
	err = hcsshim.CreateSandboxLayer(driverInfo, id, layerChain[0], layerChain)
	if err != nil {
		return nil, err
	}

	// Retrieve the mount path for the new layer
	// This should be something like this: "\\?\Volume{bd05d44f-0000-0000-0000-100000000000}\"
	mountPath, err := result.getMountPathForLayer(id, layerChain)
	if err != nil {
		return nil, err
	}

	// Build a configuration json for creating a compute system
	configuration, err := result.getComputeSystemConfiguration(mountPath, layerChain)

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
				HostPort:      container.lastNetInPort,
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

	container.logger.Info(fmt.Sprintf("StreamIn dstPath:", spec.Path))

	absDestPath := filepath.Join(container.driverInfo.HomeDir, container.handle, spec.Path)
	container.logger.Info(fmt.Sprintf("Streaming in to file: ", absDestPath))

	err := os.MkdirAll(absDestPath, 0777)
	if err != nil {
		container.logger.Error("Error trying to create destination path for in-stream", err)
	}

	tarPath := "C:\\Program Files (x86)\\Git\\bin\\tar.exe"
	cmdPath := "C:\\Windows\\System32\\cmd.exe"

	cmd := &exec.Cmd{
		Path: cmdPath,
		Dir:  absDestPath,
		Args: []string{
			"/c",
			tarPath,
			"xf",
			"-",
			"-C",
			"./",
		},
		Stdin: spec.TarStream,
	}

	err = cmd.Run()
	if err != nil {
		container.logger.Error("Error trying to run tar for in-stream", err)
	}

	return err
}

// StreamOut streams a file out of a container.
//
// Errors:
// * TODO.
func (container *container) StreamOut(spec garden.StreamOutSpec) (io.ReadCloser, error) {
	container.logger.Debug("WC: StreamOut")

	container.logger.Info(fmt.Sprintf("StreamOut srcPath:", spec.Path))

	containerPath := filepath.Join(container.driverInfo.HomeDir, container.handle)

	workingDir := filepath.Dir(spec.Path)
	compressArg := filepath.Base(spec.Path)
	if strings.HasSuffix(spec.Path, "/") {
		workingDir = spec.Path
		compressArg = "."
	}

	workingDir = filepath.Join(containerPath, workingDir)

	tarRead, tarWrite := io.Pipe()

	tarPath := "C:\\Program Files (x86)\\Git\\bin\\tar.exe"
	cmdPath := "C:\\Windows\\System32\\cmd.exe"

	cmd := &exec.Cmd{
		Path: cmdPath,
		// Dir:  workingDir,
		Args: []string{
			"/c",
			tarPath,
			"cf",
			"-",
			"-C",
			workingDir,
			compressArg,
		},
		Stdout: tarWrite,
	}

	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	go func() {
		cmd.Wait()
		tarWrite.Close()
	}()

	return tarRead, nil
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
	freePort := freeTcp4Port()
	container.lastNetInPort = freePort
	return freePort, freePort, nil
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

	//	containerRunInfo.SetFilename(cmdPath)
	//	containerRunInfo.SetArguments(concatArgs)

	//	stdinWriter, err := containerRunInfo.StdinPipe()
	//	if err != nil {
	//		return nil, err
	//	}

	//	stdoutReader, err := containerRunInfo.StdoutPipe()
	//	if err != nil {
	//		return nil, err
	//	}

	//	stderrReader, err := containerRunInfo.StderrPipe()
	//	if err != nil {
	//		return nil, err
	//	}

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

	pid, stdin, stdout, stderr, err := hcsshim.CreateProcessInComputeSystem(
		container.id,
		pio.Stdin != nil,
		pio.Stdout != nil,
		pio.Stderr != nil,
		createProcessParms,
	)

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

func freeTcp4Port() uint32 {
	l, _ := net.Listen("tcp4", ":0")
	defer l.Close()
	freePort := strings.Split(l.Addr().String(), ":")[1]
	ret, _ := strconv.ParseUint(freePort, 10, 32)
	return uint32(ret)
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
