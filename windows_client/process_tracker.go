package prison_client

import (
	"fmt"

	"github.com/Microsoft/hcsshim"
	"github.com/cloudfoundry-incubator/garden"
	"github.com/pivotal-golang/lager"
)

type ProcessTracker struct {
	pid         uint32
	containerId string
	driverInfo  hcsshim.DriverInfo

	logger lager.Logger
}

func NewProcessTracker(containerId string, pid uint32, driverInfo hcsshim.DriverInfo, logger lager.Logger) *ProcessTracker {
	ret := &ProcessTracker{
		containerId: containerId,
		pid:         pid,
		driverInfo:  driverInfo,
		logger:      logger,
	}

	return ret
}

func (t *ProcessTracker) Release() error {
	return nil
}

func (t *ProcessTracker) ID() uint32 {
	return t.pid
}

func (t *ProcessTracker) Wait() (int, error) {
	exitCode, err := hcsshim.WaitForProcessInComputeSystem(
		t.containerId,
		t.pid,
	)

	return int(exitCode), err
}

func (t *ProcessTracker) SetTTY(garden.TTYSpec) error {
	// TODO: Investigate what this is supposed to do
	// and how it can be implemented on Windows
	return nil
}

func (process ProcessTracker) Signal(signal garden.Signal) error {

	err := hcsshim.TerminateProcessInComputeSystem(
		process.containerId,
		process.pid,
	)

	if err != nil {
		process.logger.Info(fmt.Sprintf("Warning - failed to terminate pid %d in %s", process.pid, process.containerId, err))
	}

	// Ignoring errors based on
	// https://github.com/docker/docker/blob/master/daemon/execdriver/windows/terminatekill.go#L30

	return nil
}
