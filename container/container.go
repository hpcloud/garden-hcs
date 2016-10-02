package container

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hpcloud/garden-hcs/mountvol"
	"github.com/hpcloud/garden-hcs/untar"
	"github.com/hpcloud/garden-hcs/windows_client"
	"github.com/hpcloud/garden-hcs/windows_containers"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/lager"
	"github.com/Microsoft/hcsshim"
	"github.com/pborman/uuid"
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

type WindowsContainer struct {
	WindowsContainerSpec

	id            string
	handle        string
	logger        lager.Logger
	hostIP        string
	hcsContainer  hcsshim.Container
	natEndpointId string

	pids map[int]*prison_client.ProcessTracker

	hostPort      int
	containerPort int
	containerIp   string
	portMappings  []garden.PortMapping
	bindMounts    []garden.BindMount

	runMutex        sync.Mutex
	startMutex      sync.Mutex
	propertiesMutex sync.RWMutex
	driverInfo      hcsshim.DriverInfo
	active          int
	virtualSwitch   string
	layerChain      []string
	volumeName      string
	layerFolderPath string
}

func NewContainer(id, handle string, containerSpec garden.ContainerSpec, logger lager.Logger, hostIP string, driverInfo hcsshim.DriverInfo, baseImagePath, virtualSwitch string) (*WindowsContainer, error) {
	logger.Debug("WC: NewContainer")

	result := &WindowsContainer{
		id:     id,
		handle: handle,

		hostIP: hostIP,
		logger: logger,

		pids:          make(map[int]*prison_client.ProcessTracker),
		driverInfo:    driverInfo,
		active:        0,
		virtualSwitch: virtualSwitch,
		bindMounts:    containerSpec.BindMounts,
	}

	result.Env = containerSpec.Env

	// The rootfs we need to use is the scheme from the
	result.RootFSPath = containerSpec.RootFSPath
	result.WindowsContainerSpec.Properties = containerSpec.Properties

	// We need to get an available port and map it now.
	// This is because there is currently no way to change network settings
	// for a compute system that's already been created.
	result.hostPort = freeTcp4Port()

	layerChain, err := windows_containers.GetLayerChain(baseImagePath)
	result.layerChain = layerChain
	if err != nil {
		return nil, err
	}

	layerFolderPath, volumePath, err := windows_containers.CreateAndActivateContainerLayer(driverInfo, id, layerChain)
	if err != nil {
		return nil, err
	}

	result.volumeName = volumePath
	result.layerFolderPath = layerFolderPath

	return result, nil
}

func (container *WindowsContainer) Handle() string {
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
func (container *WindowsContainer) Stop(kill bool) error {
	const shutdownTimeout = time.Minute * 5
	const terminateTimeout = time.Minute * 5

	var err error

	container.logger.Debug("WC: Stop")

	container.runMutex.Lock()
	defer container.runMutex.Unlock()

	// https://github.com/docker/docker/blob/cf58eb437c4229e876f2d952a228b603a074e584/libcontainerd/container_windows.go#L281-L318

	if container.hcsContainer != nil {
		if kill == true {
			// Terminate the compute system
			err = container.hcsContainer.Terminate()
			if hcsshim.IsPending(err) {
				err = container.hcsContainer.WaitTimeout(terminateTimeout)
				if err != nil {
					container.logger.Error("hcsContainer.WaitTimeout error", err)
				}
			} else if !hcsshim.IsAlreadyStopped(err) {
				container.logger.Error("hcsContainer.Terminate error", err)
				return err
			}
		} else {
			err = container.hcsContainer.Shutdown()
			if hcsshim.IsPending(err) {
				err = container.hcsContainer.WaitTimeout(shutdownTimeout)
				if err != nil {
					container.logger.Error("hcsContainer.WaitTimeout error", err)
				}
			} else if !hcsshim.IsAlreadyStopped(err) {
				container.logger.Error("hcsContainer.Shutdown error", err)

				// Call terminate if shutdown fails
				err = container.hcsContainer.Terminate()
				if hcsshim.IsPending(err) {
					err = container.hcsContainer.WaitTimeout(terminateTimeout)
					if err != nil {
						container.logger.Error("hcsContainer.WaitTimeout error", err)
					}
				} else if !hcsshim.IsAlreadyStopped(err) {
					container.logger.Error("hcsContainer.Terminate error", err)
				}

				return err
			}
		}

		err = container.hcsContainer.Close()
		if err != nil {
			container.logger.Error("hcsContainer.Close error", err)
			return err
		}

		container.hcsContainer = nil
	}

	return nil
}

func (container *WindowsContainer) Destroy() error {
	var err error

	container.logger.Debug("WC: Destroy")

	container.Stop(true)

	container.runMutex.Lock()
	defer container.runMutex.Unlock()

	if container.natEndpointId != "" {
		_, err = hcsshim.HNSEndpointRequest("DELETE", container.natEndpointId, "")
		if err != nil {
			container.logger.Error("hcsshim.HNSEndpointRequest error", err)
		} else {
			container.natEndpointId = ""
		}
	}

	// Deactivate our layer
	err = hcsshim.DeactivateLayer(container.driverInfo, container.id)
	if err != nil {
		container.logger.Error("hcsshim.DeactivateLayer error", err)
	}

	// Destroy the sandbox layer
	err = hcsshim.DestroyLayer(container.driverInfo, container.id)
	if err != nil {
		container.logger.Error("hcsshim.DestroyLayer error", err)
	}

	return err
}

// Returns information about a container.
func (container *WindowsContainer) Info() (garden.ContainerInfo, error) {
	container.logger.Debug("WC: Info")

	properties, _ := container.Properties()

	processIDs := []string{}
	for _, pt := range container.pids {
		processIDs = append(processIDs, pt.ID())
	}

	result := garden.ContainerInfo{
		State:       "active",
		ExternalIP:  container.hostIP,
		HostIP:      container.hostIP,
		ContainerIP: container.containerIp,
		Events:      []string{},
		ProcessIDs:  processIDs,
		Properties:  properties,
		MappedPorts: container.portMappings,
	}

	return result, nil
}

// StreamIn streams data into a file in a container.
//
// Errors:
// *  TODO.
func (container *WindowsContainer) StreamIn(spec garden.StreamInSpec) error {
	container.logger.Debug("WC: StreamIn")

	container.runMutex.Lock()
	defer container.runMutex.Unlock()

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
	defer func() {
		mountvol.UnmountVolume(tempFolder)
	}()

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
func (container *WindowsContainer) StreamOut(spec garden.StreamOutSpec) (io.ReadCloser, error) {
	container.logger.Debug("TODO: StreamOut")

	// TODO: investigate a proper implementation
	// It is unclear if it's ok to keep the container mounted until the reader
	// is closed.

	tarReaderPipe, tarWriterPipe := io.Pipe()

	// Get container directory
	layerFolder := container.dir(container.id)

	// Generate a temporary directory path
	tempFolder := layerFolder + "-" + uuid.New()

	// Mount the volume
	err := mountvol.MountVolume(container.volumeName, tempFolder)
	if err != nil {
		return nil, err
	}

	// Write the tar stream to the directory
	sourceDir := filepath.Join(tempFolder, spec.Path)

	go func() {
		untar.Tarit(sourceDir, tarWriterPipe)
		mountvol.UnmountVolume(tempFolder)
	}()

	return tarReaderPipe, nil
}

func (container *WindowsContainer) CurrentBandwidthLimits() (garden.BandwidthLimits, error) {
	container.logger.Debug("WC: CurrentBandwidthLimits")
	return garden.BandwidthLimits{}, nil
}

func (container *WindowsContainer) CurrentCPULimits() (garden.CPULimits, error) {
	container.logger.Debug("WC: CurrentCPULimits")
	return garden.CPULimits{}, nil
}

func (container *WindowsContainer) CurrentDiskLimits() (garden.DiskLimits, error) {
	container.logger.Debug("WC: CurrentDiskLimits")
	return garden.DiskLimits{}, nil
}

func (container *WindowsContainer) CurrentMemoryLimits() (garden.MemoryLimits, error) {
	container.logger.Debug("WC: CurrentMemoryLimits")
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
func (container *WindowsContainer) NetIn(hostPort, containerPort uint32) (uint32, uint32, error) {
	container.logger.Debug("TODO: NetIn")

	if container.hcsContainer != nil {
		return 0, 0, errors.New("NetIn calls are not supported after the first Run in a Container")
	}

	if hostPort == 0 {
		freePort := freeTcp4Port()

		hostPort = uint32(freePort)
	}

	if containerPort == 0 {
		containerPort = hostPort
	}

	container.portMappings = append(container.portMappings,
		garden.PortMapping{
			HostPort:      hostPort,
			ContainerPort: containerPort,
		})

	return hostPort, containerPort, nil
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
func (container *WindowsContainer) NetOut(netOutRule garden.NetOutRule) error {
	container.logger.Debug("TODO: NetOut")

	return nil
}

// Run a script inside a container.
//
// The root user will be mapped to a non-root UID in the host unless the container (not this process) was created with 'privileged' true.
//
// Errors:
// * TODO.
func (container *WindowsContainer) Run(spec garden.ProcessSpec, pio garden.ProcessIO) (garden.Process, error) {

	if container.hcsContainer == nil {
		err := container.startContainer()
		if err != nil {
			return nil, err
		}
	}

	container.runMutex.Lock()
	defer container.runMutex.Unlock()

	//https://blogs.msdn.microsoft.com/twistylittlepassagesallalike/2011/04/23/everyone-quotes-command-line-arguments-the-wrong-way/
	// Combine all arguments using ' ' as a separator

	// TODO: EscapeArg from https://golang.org/src/syscall/exec_windows.go
	concatArgs := ""
	for _, v := range spec.Args {
		vp := v
		vp = strings.Replace(vp, "\n", "\r\n", -1)

		if strings.Count(vp, " ") != 0 {
			concatArgs = concatArgs + " " + "\"" + vp + "\""
		} else {
			concatArgs = concatArgs + " " + vp
		}
	}

	executablePath := spec.Path
	if executablePath[1] != ':' {
		executablePath = "C:" + filepath.FromSlash(executablePath)
	}

	// Create the command line that we're going to run
	commandLine := fmt.Sprintf("%s %s", executablePath, concatArgs)

	// Create an env var map
	envs := map[string]string{}
	for _, env := range spec.Env {
		splitEnv := strings.SplitN(env, "=", 2)
		envs[splitEnv[0]] = splitEnv[1]
	}
	// Also add container's env vars
	for _, env := range container.Env {
		splitEnv := strings.SplitN(env, "=", 2)
		envs[splitEnv[0]] = splitEnv[1]
	}

	emulateConsole := true
	consoleSize := [2]uint{0, 0}

	if spec.TTY != nil && spec.TTY.WindowSize != nil {
		// https://github.com/docker/docker/blob/25587906d122c4fce0eb1fbd3dfb805914455f59/api/client/container/run.go#L145
		// https: //github.com/docker/docker/blob/3d42bf5f120b6cbd38b54f71dff4343c316939a0/api/client/utils.go#L24
		consoleSize = [2]uint{uint(spec.TTY.WindowSize.Rows), uint(spec.TTY.WindowSize.Columns)}
	} else {
		emulateConsole = false
	}

	createProcessConfig := &hcsshim.ProcessConfig{
		CommandLine:      commandLine,
		WorkingDirectory: spec.Dir,
		CreateStdErrPipe: pio.Stderr != nil,
		CreateStdInPipe:  pio.Stdin != nil,
		CreateStdOutPipe: pio.Stdout != nil,
		EmulateConsole:   emulateConsole,
		ConsoleSize:      consoleSize,
		Environment:      envs,
	}

	// Create the process
	hcsProcess, err := container.hcsContainer.CreateProcess(createProcessConfig)
	if err != nil {
		return nil, err
	}

	pid := hcsProcess.Pid()
	stdin, stdout, stderr, err := hcsProcess.Stdio()
	if err != nil {
		return nil, err
	}

	if emulateConsole {
		hcsProcess.ResizeConsole(uint16(spec.TTY.WindowSize.Columns), uint16(spec.TTY.WindowSize.Rows))
	}

	if err != nil {
		return nil, err
	}

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

	// Create a new process tracker for the process we've just created
	pt := prison_client.NewProcessTracker(container.id, uint32(pid), hcsProcess, container.driverInfo, container.logger)

	container.logger.Debug("Container run created new process.", lager.Data{
		"PID": pt.ID(),
	})

	container.pids[pid] = pt

	return pt, nil
}

// Attach starts streaming the output back to the client from a specified process.
//
// Errors:
// * processID does not refer to a running process.
func (container *WindowsContainer) Attach(processID string, io garden.ProcessIO) (garden.Process, error) {
	pid, err := strconv.Atoi(processID)
	if err != nil {
		return nil, err
	}

	pt := container.pids[pid]

	return pt, nil
}

// Metrics returns the current set of metrics for a container
func (container *WindowsContainer) Metrics() (garden.Metrics, error) {
	container.logger.Debug("TODO: Metrics")
	return garden.Metrics{
		MemoryStat:  garden.ContainerMemoryStat{},
		CPUStat:     garden.ContainerCPUStat{},
		DiskStat:    garden.ContainerDiskStat{},
		NetworkStat: garden.ContainerNetworkStat{},
	}, nil
}

// Properties returns the current set of properties
func (container *WindowsContainer) Properties() (garden.Properties, error) {
	container.propertiesMutex.RLock()
	defer container.propertiesMutex.RUnlock()

	return container.WindowsContainerSpec.Properties, nil
}

// Property returns the value of the property with the specified name.
//
// Errors:
// * When the property does not exist on the container.
func (container *WindowsContainer) Property(name string) (string, error) {
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
func (container *WindowsContainer) SetProperty(name string, value string) error {
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
func (container *WindowsContainer) RemoveProperty(name string) error {
	container.propertiesMutex.Lock()
	defer container.propertiesMutex.Unlock()

	if _, found := container.WindowsContainerSpec.Properties[name]; !found {
		return UndefinedPropertyError{name}
	}

	delete(container.WindowsContainerSpec.Properties, name)

	return nil
}

func (container *WindowsContainer) SetGraceTime(graceTime time.Duration) error {
	// TODO: not implemented

	return nil
}

func freeTcp4Port() int {
	l, _ := net.Listen("tcp4", ":0")
	defer l.Close()
	freePort := strings.Split(l.Addr().String(), ":")[1]
	ret, _ := strconv.ParseUint(freePort, 10, 32)
	return int(ret)
}

func (c *WindowsContainer) dir(id string) string {
	return filepath.Join(c.driverInfo.HomeDir, filepath.Base(id))
}

// Get returns the rootfs path for the id. This will mount the dir at it's given path
func (c *WindowsContainer) getMountPathForLayer(rId string, layerChain []string) (string, error) {
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

func (c *WindowsContainer) getComputeSystemConfiguration() (*hcsshim.ContainerConfig, error) {
	layerFolderPath := c.dir(c.id)

	// TODO: investigate further when IgnoreFlushesDuringBoot should be false
	cu := &hcsshim.ContainerConfig{
		SystemType:              "Container",
		IgnoreFlushesDuringBoot: true,
		Name:              c.id,
		Owner:             windows_containers.DefaultOwner,
		IsDummy:           false,
		VolumePath:        c.volumeName,
		LayerFolderPath:   layerFolderPath,
		Layers:            []hcsshim.Layer{},
		EndpointList:      []string{c.natEndpointId},
		MappedDirectories: []hcsshim.MappedDir{},
	}

	for _, layerPath := range c.layerChain {
		layerId := filepath.Base(layerPath)
		layerGuid, err := hcsshim.NameToGuid(layerId)
		if err != nil {
			return nil, err
		}

		cu.Layers = append(cu.Layers, hcsshim.Layer{
			ID:   layerGuid.ToString(),
			Path: layerPath,
		})
	}

	// https://github.com/cloudfoundry/garden/blob/f90a312c91dc1d586c15a42ca29959445bdd25a1/client.go#L161
	for _, bindMount := range c.bindMounts {
		srcPath := bindMount.SrcPath
		dstPath := bindMount.DstPath

		if dstPath[1] != ':' {
			dstPath = "C:" + filepath.FromSlash(dstPath)
		}

		if bindMount.Origin == garden.BindMountOriginHost {
			readOnly := bindMount.Mode == garden.BindMountModeRO
			mappedDir := hcsshim.MappedDir{
				HostPath:      srcPath,
				ContainerPath: dstPath,
				ReadOnly:      readOnly,
			}
			cu.MappedDirectories = append(cu.MappedDirectories, mappedDir)

			fmt.Println(srcPath, dstPath)
		}
		if bindMount.Origin == garden.BindMountOriginContainer {
			return nil, errors.New("'Container' bind mount mode not supported")
		}
	}

	return cu, nil
}

func (c *WindowsContainer) createNatNetworkEndpoint() error {
	hcsNets, err := hcsshim.HNSListNetworkRequest("GET", "", "")
	if err != nil {
		return err
	}

	natNetworkId := ""
	for _, n := range hcsNets {
		if n.Name == "nat" {
			natNetworkId = n.Id
		}
	}

	if natNetworkId == "" {
		return errors.New("Nat network not found")
	}

	// https://github.com/docker/libnetwork/blob/f9a1590164b878e668eabf889dd79fb6af8eaced/drivers/windows/windows.go#L284

	netInPolicies := []json.RawMessage{}

	for _, pm := range c.portMappings {
		natPolicy, err := json.Marshal(hcsshim.NatPolicy{
			Type:         "NAT",
			ExternalPort: uint16(pm.HostPort),
			InternalPort: uint16(pm.ContainerPort),
			Protocol:     "TCP",
		})
		if err != nil {
			return err
		}

		netInPolicies = append(netInPolicies, natPolicy)
	}

	endpointRequest := hcsshim.HNSEndpoint{
		// Name:           "endpoint 1",
		VirtualNetwork: natNetworkId,
	}

	endpointRequest.Policies = netInPolicies

	endpointRequestJson, err := json.Marshal(endpointRequest)
	if err != nil {
		return err
	}
	endpoint, err := hcsshim.HNSEndpointRequest("POST", "", string(endpointRequestJson))
	if err != nil {
		return err
	}

	c.containerIp = endpoint.IPAddress.String()
	c.natEndpointId = endpoint.Id
	c.logger.Info(fmt.Sprintf("Nat endpoint created %v IP Addrss %v", endpoint.Id, endpoint.IPAddress.String()))

	return nil
}

func (c *WindowsContainer) startContainer() error {
	c.startMutex.Lock()
	defer c.startMutex.Unlock()

	if c.hcsContainer == nil {
		c.logger.Info(fmt.Sprintf("Starting container %v", c.id))

		err := c.createNatNetworkEndpoint()
		if err != nil {
			return err
		}

		// Build a configuration json for creating a compute system
		configuration, err := c.getComputeSystemConfiguration()
		if err != nil {
			return err
		}

		// Create a compute system.
		// This is our container.
		c.logger.Info(fmt.Sprintf("HCS create container", lager.Data{"HCSContainerConfig": configuration}))
		hcsContainer, err := hcsshim.CreateContainer(c.id, configuration)
		if err != nil {
			return err
		}
		c.hcsContainer = hcsContainer

		// Start the container
		c.logger.Info(fmt.Sprintf("HCS start container"))
		err = c.hcsContainer.Start()
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *WindowsContainer) getContainerIp() (string, error) {

	// TODO: this is a very inefficient and bad workaround to get the IP
	// of the container.

	processSpec := garden.ProcessSpec{
		Path: "ipconfig",
		Args: []string{},
		Env:  []string{},
		Dir:  "c:\\",
	}

	stdout := bytes.NewBufferString("")

	pio := garden.ProcessIO{
		Stdin:  nil,
		Stdout: stdout,
		Stderr: nil,
	}

	pt, err := c.Run(processSpec, pio)
	if err != nil {
		return "", err
	}

	_, err = pt.Wait()
	if err != nil {
		return "", err
	}

	output, err := ioutil.ReadAll(stdout)

	re, err := regexp.Compile(`IPv4 Address. . . . . . . . . . . : (?P<ip>\d+\.\d+\.\d+\.\d+)`)
	if err != nil {
		return "", err
	}

	match := re.FindStringSubmatch(string(output))

	for i, name := range re.SubexpNames() {
		if name == "ip" {
			if i < len(match) {
				return match[i], nil
			}
		}
	}

	return "", fmt.Errorf("Could not detect container IP address from ipconfig: %s", output)
}
