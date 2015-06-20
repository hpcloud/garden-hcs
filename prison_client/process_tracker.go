package prison_client

import (
	"errors"
	"log"
	"runtime"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

type ProcessTracker struct {
	pt *ole.IDispatch
}

func NewProcessTracker(pt *ole.IDispatch) *ProcessTracker {
	pt.AddRef() // call addref because we clone the pointr
	ret := &ProcessTracker{pt: pt}
	runtime.SetFinalizer(ret, finalizeProcessTracker)
	return ret
}

func finalizeProcessTracker(t *ProcessTracker) {
	if t.pt != nil {
		lastRefCount := t.pt.Release()

		log.Println("ProcessTracker ref count after finalizer: ", lastRefCount)
		t.pt = nil
	}
}

func (t *ProcessTracker) Release() error {
	if t.pt != nil {
		t.pt.Release()
		t.pt = nil
		return nil
	} else {
		return errors.New("ProcessTracker is already released")
	}
}

func (t *ProcessTracker) ID() uint32 {
	iDpid, errr := oleutil.CallMethod(t.pt, "GetPid")
	if errr != nil {
		log.Fatal(errr)
	}
	defer iDpid.Clear()

	return uint32(iDpid.Value().(int64))
}

func (t *ProcessTracker) Wait() (int, error) {
	_, err := oleutil.CallMethod(t.pt, "Wait")
	if err != nil {
		return 0, err
	}

	exitCode, err := oleutil.CallMethod(t.pt, "GetExitCode")
	if err != nil {
		return 0, err
	}
	defer exitCode.Clear()

	return int(exitCode.Value().(int64)), nil
}

func (t *ProcessTracker) SetTTY(garden.TTYSpec) error {
	log.Println("TODO: ProcessTracker SetTTY")
	return nil
}

func (process ProcessTracker) Signal(signal garden.Signal) error {
	return nil
}
