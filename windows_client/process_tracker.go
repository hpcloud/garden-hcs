package windows_client

import (
	"strconv"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/lager"
	"github.com/Microsoft/hcsshim"
)

type ProcessTracker struct {
	garden.Process

	pid         uint32
	hcsProcess  hcsshim.Process
	containerId string
	driverInfo  hcsshim.DriverInfo
	stoppedChan chan interface{}
	exitCode    int

	logger lager.Logger
}

func NewProcessTracker(containerId string, pid uint32, hcsProcess hcsshim.Process, driverInfo hcsshim.DriverInfo, logger lager.Logger) *ProcessTracker {
	ret := &ProcessTracker{
		containerId: containerId,
		pid:         pid,
		hcsProcess:  hcsProcess,
		driverInfo:  driverInfo,
		logger:      logger,
		stoppedChan: make(chan interface{}),
	}

	go func() {
		ret.waitForExitcode()
		close(ret.stoppedChan)
		ret.release()
	}()

	return ret
}

func (t *ProcessTracker) ID() string {
	return strconv.FormatUint(uint64(t.pid), 10)
}

func (t *ProcessTracker) Wait() (int, error) {
	<-t.stoppedChan

	return t.exitCode, nil
}

func (t *ProcessTracker) SetTTY(ttySpec garden.TTYSpec) error {
	err := t.hcsProcess.ResizeConsole(uint16(ttySpec.WindowSize.Columns), uint16(ttySpec.WindowSize.Rows))
	if err != nil {
		t.logger.Error("hcsProcess.ResizeConsole error", err)
	}

	return err
}

func (t *ProcessTracker) Signal(signal garden.Signal) error {
	if signal == garden.SignalKill || signal == garden.SignalTerminate {
		err := t.hcsProcess.Kill()
		t.logger.Error("hcsProcess.Kill error", err)
		return err
	}

	return nil
}

func (t *ProcessTracker) waitForExitcode() {
	// https://github.com/docker/docker/blob/97660c6ec55f45416cb2b2d4c116267864b62b65/libcontainerd/container_windows.go#L164-L191

	err := t.hcsProcess.Wait()
	if err != nil {
		t.logger.Error("hcsProcess.Wait error", err)
	}

	exitCode, err := t.hcsProcess.ExitCode()
	if err != nil {
		t.logger.Error("hcsProcess.ExitCode error", err)

		exitCode = -1
	}

	t.exitCode = exitCode
}

func (t *ProcessTracker) release() {
	err := t.hcsProcess.Close()
	if err != nil {
		t.logger.Error("hcsProcess.Close error", err)
	}
}
