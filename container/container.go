package container

import (
	"fmt"
	"io"
	"net"
	"net/rpc"
	"path/filepath"
	"strconv"
	"sync"

	"os"
	"os/exec"
	"strings"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-windows/prison_client"
	"github.com/pivotal-golang/lager"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

type PrisonContainerSpec struct {
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

	rootPath string
	hostIP   string

	PrisonContainerSpec

	rpc           *rpc.Client
	pids          map[int]*prison_client.ProcessTracker
	lastNetInPort uint32
	prison        *ole.IDispatch

	runMutex        sync.Mutex
	propertiesMutex sync.RWMutex
}

func NewContainer(id, handle string, rootPath string, logger lager.Logger, hostIP string, properties garden.Properties) *container {
	logger.Debug("PELERINUL: NewContainer")
	iUcontainer, errr := oleutil.CreateObject("CloudFoundry.WindowsPrison.ComWrapper.Container")

	if errr != nil {
		logger.Fatal("Error creating the COM object for the prison", errr)
	}
	defer iUcontainer.Release()

	iDcontainer, errr := iUcontainer.QueryInterface(ole.IID_IDispatch)
	if errr != nil {
		logger.Fatal("Error trying to query COM interface", errr)
	}

	result := &container{
		id:     id,
		handle: handle,

		rootPath: rootPath,
		hostIP:   hostIP,
		logger:   logger,

		pids:   make(map[int]*prison_client.ProcessTracker),
		prison: iDcontainer,
	}

	result.PrisonContainerSpec.Properties = properties

	return result
}

func (container *container) Handle() string {
	container.logger.Debug("PELERINUL: Handle")
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
	container.logger.Debug("PELERINUL: Stop")

	container.runMutex.Lock()
	defer container.runMutex.Unlock()

	//ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)

	isLocked, errr := oleutil.CallMethod(container.prison, "IsLockedDown")
	if errr != nil {
		container.logger.Error("Error trying to retrieve prison lockdown status", errr)
	}
	defer isLocked.Clear()
	blocked := isLocked.Value().(bool)

	if blocked == true {
		container.logger.Info(fmt.Sprintf("Stop with kill", kill))
		_, errr = oleutil.CallMethod(container.prison, "Terminate")
		if errr != nil {
			container.logger.Error("Error trying to stop prison", errr)
		}

		if kill == true {
			container.destroy()
		}
	}

	return errr

	return nil
}

// Returns information about a container.
func (container *container) Info() (garden.ContainerInfo, error) {
	container.logger.Debug("PELERINUL: Info")

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
				ContainerPort: container.lastNetInPort,
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
func (container *container) StreamIn(dstPath string, source io.Reader) error {
	container.logger.Debug("PELERINUL: StreamIn")

	container.logger.Info(fmt.Sprintf("StreamIn dstPath:", dstPath))

	absDestPath := filepath.Join(container.rootPath, container.handle, dstPath)
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
		Stdin: source,
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
func (container *container) StreamOut(srcPath string) (io.ReadCloser, error) {
	container.logger.Debug("PELERINUL: StreamOut")

	container.logger.Info(fmt.Sprintf("StreamOut srcPath:", srcPath))

	containerPath := filepath.Join(container.rootPath, container.handle)

	workingDir := filepath.Dir(srcPath)
	compressArg := filepath.Base(srcPath)
	if strings.HasSuffix(srcPath, "/") {
		workingDir = srcPath
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
	container.logger.Debug("PELERINUL: LimitBandwidth")

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

	cmdPath := "C:\\Windows\\System32\\cmd.exe"
	rootPath := filepath.Join(container.rootPath, container.handle)
	strings.Replace(rootPath, "/", "\\", -1)

	spec.Dir = filepath.Join(rootPath, spec.Dir)

	if !filepath.IsAbs(spec.Path) {
		spec.Path = filepath.Join(rootPath, spec.Path)
	}

	envs := spec.Env

	// TOTO: remove this (HACK?!) port overriding
	// after somebody cleans up this hardcoded values:
	// https://github.com/cloudfoundry-incubator/app-manager/blob/master/start_message_builder/start_message_builder.go#L182
	envs = append(envs, "NETIN_PORT="+strconv.FormatUint(uint64(container.lastNetInPort), 10))
	envs = append(envs, "PORT="+strconv.FormatUint(uint64(container.lastNetInPort), 10))

	containerPath := filepath.Join(container.rootPath, container.handle)

	envs = append(envs, "HOMEPATH="+containerPath)
	envs = append(envs, "HOME="+filepath.Join(containerPath, "app"))

	containerRunInfo, err := prison_client.CreateContainerRunInfo()

	defer func() {
		container.logger.Debug("PELERINUL: Releasing container run info ...")
		containerRunInfo.Release()
	}()

	if err != nil {
		container.logger.Error("Error trying to create ContainerRunInfo for a prison", err)
		return nil, err
	}

	for _, env := range envs {
		spltiEnv := strings.SplitN(env, "=", 2)
		containerRunInfo.AddEnvironmentVariable(spltiEnv[0], spltiEnv[1])
	}

	concatArgs := ""
	for _, v := range spec.Args {
		concatArgs = concatArgs + " " + v
	}

	spec.Path = strings.Replace(spec.Path, "/", "\\", -1)

	concatArgs = " /c " + spec.Path + " " + concatArgs
	container.logger.Info(fmt.Sprintf("Filename ", spec.Path, "Arguments: ", concatArgs, "Concat Args: ", concatArgs))

	containerRunInfo.SetFilename(cmdPath)
	containerRunInfo.SetArguments(concatArgs)

	stdinWriter, err := containerRunInfo.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdoutReader, err := containerRunInfo.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderrReader, err := containerRunInfo.StderrPipe()
	if err != nil {
		return nil, err
	}

	go func() {
		container.logger.Info(fmt.Sprintf("Streaming stdout ", stdoutReader))

		io.Copy(pio.Stdout, stdoutReader)
		stdoutReader.Close()

		container.logger.Info(fmt.Sprintf("Stdout pipe closed", stdoutReader))
	}()

	go func() {
		container.logger.Info(fmt.Sprintf("Streaming stderr ", stderrReader))

		io.Copy(pio.Stderr, stderrReader)
		stderrReader.Close()

		container.logger.Info(fmt.Sprintf("Stderr pipe closed", stderrReader))
	}()

	go func() {
		container.logger.Info(fmt.Sprintf("Streaming stdin ", stdinWriter))

		io.Copy(stdinWriter, pio.Stdin)
		stdinWriter.Close()

		container.logger.Info(fmt.Sprintf("Stdin pipe closed", stdinWriter))
	}()

	isLocked, errr := oleutil.CallMethod(container.prison, "IsLockedDown")
	if errr != nil {
		container.logger.Error("Error trying to retrieve prison lock-down status", errr)
	}

	defer func() {
		container.logger.Debug("PELERINUL: Clearing isLocked response ...")
		isLocked.Clear()
	}()

	if isLocked.Value().(bool) == false {

		oleutil.PutProperty(container.prison, "HomePath", rootPath)
		oleutil.PutProperty(container.prison, "NetworkPort", uint(container.lastNetInPort))
		oleutil.PutProperty(container.prison, "MemoryLimitBytes", int64(1024*1024*512))

		container.logger.Info("Locking down...")
		_, errr = oleutil.CallMethod(container.prison, "Lockdown")
		if errr != nil {
			container.logger.Error("Error trying to lock prison down", errr)
			return nil, errr
		}
		container.logger.Info("Locked down.")
	}

	container.logger.Info("Running process...")

	iDcri, _ := containerRunInfo.GetIDispatch()

	defer func() {
		container.logger.Debug("PELERINUL: Releasing container run information interface ...")
		iDcri.Release()
	}()

	ptrackerRes, errr := oleutil.CallMethod(container.prison, "Run", iDcri)

	defer func() {
		container.logger.Debug("PELERINUL: Clearing process tracker response ...")
		ptrackerRes.Clear()
	}()

	if errr != nil {
		container.logger.Error("Error trying to run process in prison", errr)
		return nil, errr
	}

	ptracker := ptrackerRes.ToIDispatch()
	ptracker.AddRef() // ToIDispatch does not increase ref count

	defer func() {
		container.logger.Debug("PELERINUL: Releasing process tracker ...")
		ptracker.Release()
	}()

	pt := prison_client.NewProcessTracker(ptracker)

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
		"properties": container.PrisonContainerSpec.Properties,
	})

	return container.PrisonContainerSpec.Properties, nil
}

// Property returns the value of the property with the specified name.
//
// Errors:
// * When the property does not exist on the container.
func (container *container) Property(name string) (string, error) {
	container.logger.Info("getting-property", lager.Data{"name": name})

	container.propertiesMutex.RLock()
	defer container.propertiesMutex.RUnlock()

	value, found := container.PrisonContainerSpec.Properties[name]
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
	for k, v := range container.PrisonContainerSpec.Properties {
		props[k] = v
	}

	props[name] = value

	container.PrisonContainerSpec.Properties = props

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

	if _, found := container.PrisonContainerSpec.Properties[name]; !found {
		return UndefinedPropertyError{name}
	}

	delete(container.PrisonContainerSpec.Properties, name)

	return nil
}

func (container *container) destroy() error {
	container.logger.Debug("PELERINUL: Destroy")

	defer container.prison.Release()

	isLocked, errr := oleutil.CallMethod(container.prison, "IsLockedDown")
	if errr != nil {
		container.logger.Error("Error trying to retrive prison lock-down status", errr)
	}
	defer isLocked.Clear()
	blocked := isLocked.Value().(bool)

	if blocked == true {

		container.logger.Info("Invoking destory on prison")
		_, errr = oleutil.CallMethod(container.prison, "Destroy")
		if errr != nil {
			container.logger.Error("Error trying to destroy prison", errr)
			return errr
		}
		container.logger.Info(fmt.Sprintf("Container destroyed: ", container.id))
	}
	return nil
}

func (container *container) getContainerPath() string {
	return filepath.Join(container.rootPath, container.handle)
}

func freeTcp4Port() uint32 {
	l, _ := net.Listen("tcp4", ":0")
	defer l.Close()
	freePort := strings.Split(l.Addr().String(), ":")[1]
	ret, _ := strconv.ParseUint(freePort, 10, 32)
	return uint32(ret)
}
