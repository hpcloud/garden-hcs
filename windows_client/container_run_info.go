package prison_client

import (
	"errors"
	"io"
	"log"
	"runtime"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/natefinch/npipe"
)

type ContainerRunInfo struct {
	runInfo *ole.IDispatch
}

func newContainerRunInfo(runInfo *ole.IDispatch) *ContainerRunInfo {
	return &ContainerRunInfo{
		runInfo: runInfo,
	}
}

func CreateContainerRunInfo() (*ContainerRunInfo, error) {
	IUcri, err := oleutil.CreateObject("CloudFoundry.WindowsPrison.ComWrapper.ContainerRunInfo")
	if err != nil {
		log.Println(err)
		return nil, err
	}
	defer IUcri.Release()

	cri, err := IUcri.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	ret := newContainerRunInfo(cri)
	runtime.SetFinalizer(ret, finalizeContainerRunInfo)

	return ret, nil
}

func finalizeContainerRunInfo(t *ContainerRunInfo) {
	if t.runInfo != nil {
		lastRefCount := t.runInfo.Release()
		// Ref count should be 0 after finalizer
		// TODO: invoke panic if last ref count != 0 ???
		log.Println("ContainerRunInfo ref count after finalizer: ", lastRefCount)
		t.runInfo = nil
	}
}

func (t *ContainerRunInfo) Release() error {
	if t.runInfo != nil {
		log.Print("WC: release")
		t.runInfo.Release()

		//lastRefCount := t.runInfo.Release()
		//if lastRefCount != 0 {.,
		//	log.Fatalf("ContainerRunInfo ref count: %d. Expected 0.", lastRefCount)
		//}

		t.runInfo = nil
		return nil
	} else {
		return errors.New("ContainerRunInfo is already released")
	}
}

func (t *ContainerRunInfo) AddEnvironmentVariable(envName string, envValue string) {
	_, err := oleutil.CallMethod(t.runInfo, "AddEnvironmentVariable", envName, envValue)
	if err != nil {
		log.Fatal(err)
	}
}

func (t *ContainerRunInfo) SetFilename(value string) {
	_, err := oleutil.PutProperty(t.runInfo, "Filename", value)
	if err != nil {
		log.Fatal(err)
	}
}

func (t *ContainerRunInfo) GetFilename(value string) string {
	res, err := oleutil.GetProperty(t.runInfo, "Filename")
	if err != nil {
		log.Fatal(err)
	}
	defer res.Clear()

	return res.ToString()
}

func (t *ContainerRunInfo) SetArguments(value string) {
	_, err := oleutil.PutProperty(t.runInfo, "Arguments", value)
	if err != nil {
		log.Fatal(err)
	}
}

func (t *ContainerRunInfo) StdinPipe() (io.WriteCloser, error) {
	stdinPipeVariant, err := oleutil.CallMethod(t.runInfo, "RedirectStdin", true)
	if err != nil {
		return nil, err
	}
	defer stdinPipeVariant.Clear()

	stdinPipe := stdinPipeVariant.ToString()

	conn, err := npipe.Dial(`\\.\pipe\` + stdinPipe)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (t *ContainerRunInfo) StdoutPipe() (io.ReadCloser, error) {
	stdoutPipeVariant, err := oleutil.CallMethod(t.runInfo, "RedirectStdout", true)
	if err != nil {
		return nil, err
	}
	defer stdoutPipeVariant.Clear()

	stdoutPipe := stdoutPipeVariant.ToString()

	conn, err := npipe.Dial(`\\.\pipe\` + stdoutPipe)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (t *ContainerRunInfo) StderrPipe() (io.ReadCloser, error) {
	stderrPipeVariant, err := oleutil.CallMethod(t.runInfo, "RedirectStderr", true)
	if err != nil {
		return nil, err
	}
	defer stderrPipeVariant.Clear()

	stderrPipe := stderrPipeVariant.ToString()

	conn, err := npipe.Dial(`\\.\pipe\` + stderrPipe)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
