package prison_client

import (
	"log"

	"github.com/cloudfoundry-incubator/garden"
)

type ProcessTracker struct {
}

func NewProcessTracker() *ProcessTracker {
	ret := &ProcessTracker{}
	return ret
}

func (t *ProcessTracker) Release() error {
	return nil
}

func (t *ProcessTracker) ID() uint32 {
	return 0
}

func (t *ProcessTracker) Wait() (int, error) {
	return 0, nil
}

func (t *ProcessTracker) SetTTY(garden.TTYSpec) error {
	log.Println("TODO: ProcessTracker SetTTY")
	return nil
}

func (process ProcessTracker) Signal(signal garden.Signal) error {
	return nil
}
