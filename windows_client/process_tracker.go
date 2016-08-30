package prison_client

import (
	// "fmt"
	"strconv"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/lager"
	"github.com/Microsoft/hcsshim"
)

type ProcessTracker struct {
	pid         uint32
	hcsProcess  hcsshim.Process
	containerId string
	driverInfo  hcsshim.DriverInfo

	logger lager.Logger
}

func NewProcessTracker(containerId string, pid uint32, hcsProcess hcsshim.Process, driverInfo hcsshim.DriverInfo, logger lager.Logger) *ProcessTracker {
	ret := &ProcessTracker{
		containerId: containerId,
		pid:         pid,
		hcsProcess:  hcsProcess,
		driverInfo:  driverInfo,
		logger:      logger,
	}

	return ret
}

func (t *ProcessTracker) Release() error {
	return nil
}

func (t *ProcessTracker) ID() string {
	return strconv.FormatUint(uint64(t.pid), 10)
}

func (t *ProcessTracker) Wait() (int, error) {
	err := t.hcsProcess.Wait()
	if err != nil {
		return 0, err
	}

	exitCode, err := t.hcsProcess.ExitCode()

	return int(exitCode), nil
}

func (t *ProcessTracker) SetTTY(garden.TTYSpec) error {
	// TODO: Investigate what this is supposed to do
	// and how it can be implemented on Windows
	return nil
}

func (process ProcessTracker) Signal(signal garden.Signal) error {

	//	err := hcsshim.TerminateProcessInComputeSystem(
	//		process.containerId,
	//		process.pid,
	//	)

	//	if err != nil {
	//		process.logger.Info(fmt.Sprintf("Warning - failed to terminate pid %d in %s", process.pid, process.containerId, err))
	//	}

	// Ignoring errors based on
	// https://github.com/docker/docker/blob/master/daemon/execdriver/windows/terminatekill.go#L30

	return nil
}
