package prison_client

import (
	"github.com/Microsoft/hcsshim"
	"github.com/cloudfoundry-incubator/garden"
)

type ProcessTracker struct {
	pid         uint32
	containerId string
	driverInfo  hcsshim.DriverInfo
}

func NewProcessTracker(containerId string, pid uint32, driverInfo hcsshim.DriverInfo) *ProcessTracker {
	ret := &ProcessTracker{
		containerId: containerId,
		pid:         pid,
		driverInfo:  driverInfo,
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

	return err
}
