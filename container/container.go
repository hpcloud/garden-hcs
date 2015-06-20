package container

import (
	"io"
	"net/rpc"
	"path"
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

type container struct {
	id     string
	handle string

	logger lager.Logger

	rootPath string

	rpc           *rpc.Client
	pids          map[int]*prison_client.ProcessTracker
	lastNetInPort uint32
	prison        *ole.IDispatch
	runMutex      *sync.Mutex
}

func NewContainer(id, handle string, rootPath string, logger lager.Logger) *container {
	iUcontainer, errr := oleutil.CreateObject("CloudFoundry.WindowsPrison.ComWrapper.Container")

	if errr != nil {
		logger.Error(errr)
	}
	defer iUcontainer.Release()

	iDcontainer, errr := iUcontainer.QueryInterface(ole.IID_IDispatch)
	if errr != nil {
		logger.Fatal(errr)
	}

	return &container{
		id:     id,
		handle: handle,

		rootPath: rootPath,
		logger:   logger,

		pids:     make(map[int]*prison_client.ProcessTracker),
		prison:   iDcontainer,
		runMutex: &sync.Mutex{},
	}
}

func (container *container) Handle() string {
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
	container.runMutex.Lock()
	defer container.runMutex.Unlock()

	//ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)

	isLocked, errr := oleutil.CallMethod(container.prison, "IsLockedDown")
	if errr != nil {
		container.logger.Error(errr)
	}
	defer isLocked.Clear()
	blocked := isLocked.Value().(bool)

	if blocked == true {
		container.logger.Info("Stop with kill", kill)
		_, errr = oleutil.CallMethod(container.prison, "Stop")
		if errr != nil {
			container.logger.Error(errr)
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
	container.logger.Info("Info called")
	return garden.ContainerInfo{Events: []string{"party"}, ProcessIDs: []uint32{}, MappedPorts: []garden.PortMapping{}}, nil
}

// StreamIn streams data into a file in a container.
//
// Errors:
// *  TODO.
func (container *container) StreamIn(dstPath string, tarStream io.Reader) error {
	container.logger.Info("StreamIn dstPath:", dstPath)

	absDestPath := path.Join(container.rootPath, container.handle, dstPath)
	container.logger.Info("Streaming in to file: ", absDestPath)

	err := os.MkdirAll(absDestPath, 0777)
	if err != nil {
		container.logger.Error(err)
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
		container.logger.Error(err)
	}

	return err
}

// StreamOut streams a file out of a container.
//
// Errors:
// * TODO.
func (container *container) StreamOut(srcPath string) (io.ReadCloser, error) {
	container.logger.Info("StreamOut srcPath:", srcPath)

	containerPath := path.Join(container.rootPath, container.handle)

	workingDir := filepath.Dir(srcPath)
	compressArg := filepath.Base(srcPath)
	if strings.HasSuffix(srcPath, "/") {
		workingDir = srcPath
		compressArg = "."
	}

	workingDir = path.Join(containerPath, workingDir)

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
	log.Println("TODO LimitBandwidth")
	return nil
}

func (container *container) CurrentBandwidthLimits() (garden.BandwidthLimits, error) {
	log.Println("TODO CurrentBandwidthLimits")
	return garden.BandwidthLimits{}, nil
}

// Limits the CPU shares for a container.
func (container *container) LimitCPU(limits garden.CPULimits) error {
	log.Println("TODO LimitCPU")
	return nil
}

func (container *container) CurrentCPULimits() (garden.CPULimits, error) {
	log.Println("TODO CurrentCPULimits")
	return garden.CPULimits{}, nil
}

// Limits the disk usage for a container.
//
// The disk limits that are set by this command only have effect for the container's unprivileged user.
// Files/directories created by its privileged user are not subject to these limits.
//
// TODO: explain how disk management works.
func (container *container) LimitDisk(limits garden.DiskLimits) error {
	log.Println("TODO LimitDisk")
	return nil
}

func (container *container) CurrentDiskLimits() (garden.DiskLimits, error) {
	log.Println("TODO CurrentDiskLimits")
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
	log.Println("TODO LimitMemory")
	return nil
}

func (container *container) CurrentMemoryLimits() (garden.MemoryLimits, error) {
	log.Println("TODO CurrentMemoryLimits")
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
	log.Println("TODO NetIn", hostPort, containerPort)
	freePort := freeTcp4Port()
	container.lastNetInPort = freePort
	return freePort, containerPort, nil
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
func (container *container) Run(garden.ProcessSpec, garden.ProcessIO) (garden.Process, error) {
	container.runMutex.Lock()
	defer container.runMutex.Unlock()
	// ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)

	log.Println("Run command: ", spec.Path, spec.Args, spec.Dir, spec.Privileged, spec.Env)

	cmdPath := "C:\\Windows\\System32\\cmd.exe"
	rootPath := path.Join(container.rootPath, container.handle)
	strings.Replace(rootPath, "/", "\\", -1)

	spec.Dir = path.Join(rootPath, spec.Dir)
	spec.Path = path.Join(rootPath, spec.Path)

	envs := spec.Env

	// TOTD: remove this (HACK?!) port overriding
	// after somebody cleans up this hardcoded values:
	// https://github.com/cloudfoundry-incubator/app-manager/blob/master/start_message_builder/start_message_builder.go#L182
	envs = append(envs, "NETIN_PORT="+strconv.FormatUint(uint64(container.lastNetInPort), 10))

	cri, err := prison_client.CreateContainerRunInfo()
	defer cri.Release()
	if err != nil {
		log.Println(err)
		return nil, err
	}

	for _, env := range envs {
		spltiEnv := strings.SplitN(env, "=", 2)
		cri.AddEnvironemntVariable(spltiEnv[0], spltiEnv[1])
	}

	concatArgs := ""
	for _, v := range spec.Args {
		concatArgs = concatArgs + " " + v
	}

	spec.Path = strings.Replace(spec.Path, "/", "\\", -1)

	concatArgs = " /c " + spec.Path + " " + concatArgs
	log.Println("Filename ", spec.Path, "Arguments: ", concatArgs, "Concat Args: ", concatArgs)

	cri.SetFilename(cmdPath)
	cri.SetArguments(concatArgs)

	stdinWriter, err := cri.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdoutReader, err := cri.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderrReader, err := cri.StderrPipe()
	if err != nil {
		return nil, err
	}

	go func() {
		log.Println("Streaming stdout ", stdoutReader)

		io.Copy(pio.Stdout, stdoutReader)
		stdoutReader.Close()

		log.Println("Stdout pipe closed", stdoutReader)
	}()

	go func() {
		log.Println("Streaming stderr ", stderrReader)

		io.Copy(pio.Stderr, stderrReader)
		stderrReader.Close()

		log.Println("Stderr pipe closed", stderrReader)
	}()

	go func() {
		log.Println("Streaming stdin ", stdinWriter)

		io.Copy(stdinWriter, pio.Stdin)
		stdinWriter.Close()

		log.Println("Stdin pipe closed", stdinWriter)
	}()

	isLocked, errr := oleutil.CallMethod(container.prison, "IsLockedDown")
	if errr != nil {
		log.Println(errr)
	}
	defer isLocked.Clear()
	blocked := isLocked.Value().(bool)

	if blocked == false {

		oleutil.PutProperty(container.prison, "HomePath", rootPath)
		// oleutil.PutProperty(container, "MemoryLimitBytes", 1024*1024*300)

		log.Println("Locking down...")
		_, errr = oleutil.CallMethod(container.prison, "Lockdown")
		if errr != nil {
			log.Println(errr)
			return nil, errr
		}
		log.Println("Locked down.")
	}

	log.Println("Running process...")
	iDcri, _ := cri.GetIDispatch()
	defer iDcri.Release()
	ptrackerRes, errr := oleutil.CallMethod(container.prison, "Run", iDcri)

	if errr != nil {
		log.Println(errr)
		return nil, errr
	}
	defer ptrackerRes.Clear()
	ptracker := ptrackerRes.ToIDispatch()
	ptracker.AddRef() // ToIDispatch does not incease ref count
	defer ptracker.Release()

	pt := prison_client.NewProcessTracker(ptracker)

	return pt, nil
}

// Attach starts streaming the output back to the client from a specified process.
//
// Errors:
// * processID does not refer to a running process.
func (container *container) Attach(processID uint32, io garden.ProcessIO) (garden.Process, error) {
	log.Println("Attaching to: ", processID)

	cmd := container.pids[int(processID)]

	return cmd, nil
}

// Metrics returns the current set of metrics for a container
func (container *container) Metrics() (garden.Metrics, error) {

}

// Properties returns the current set of properties
func (container *container) Properties() (garden.Properties, error) {

}

// Property returns the value of the property with the specified name.
//
// Errors:
// * When the property does not exist on the container.
func (container *container) Property(name string) (string, error) {

}

// Set a named property on a container to a specified value.
//
// Errors:
// * None.
func (container *container) SetProperty(name string, value string) error {

}

// Remove a property with the specified name from a container.
//
// Errors:
// * None.
func (container *container) RemoveProperty(name string) error {

}

func (container *container) destroy() error {
	defer container.prison.Release()

	isLocked, errr := oleutil.CallMethod(container.prison, "IsLockedDown")
	if errr != nil {
		log.Println(errr)
	}
	defer isLocked.Clear()
	blocked := isLocked.Value().(bool)

	if blocked == true {

		log.Println("Invoking destory on prison")
		_, errr = oleutil.CallMethod(container.prison, "Destroy")
		if errr != nil {
			log.Println(errr)
			return errr
		}
		log.Println("Container destoryed: ", container.id)
	}
	return nil
}

func freeTcp4Port() uint32 {
	l, _ := net.Listen("tcp4", ":0")
	defer l.Close()
	freePort := strings.Split(l.Addr().String(), ":")[1]
	ret, _ := strconv.ParseUint(freePort, 10, 32)
	return uint32(ret)
}
