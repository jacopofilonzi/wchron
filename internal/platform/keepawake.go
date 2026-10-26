package platform

import (
	"fmt"
	"runtime"
)

// executionStateFunc calls the OS with the given flags and returns the previous
// state; 0 means failure (the SetThreadExecutionState convention).
type executionStateFunc func(flags uint32) uint32

const (
	esContinuous     uint32 = 0x80000000
	esSystemRequired uint32 = 0x00000001
)

// threadInhibitor serializes every execution-state call on one goroutine locked
// to its OS thread, because the state is per thread (see the package doc).
type threadInhibitor struct {
	reqs chan inhibitReq
}

type inhibitReq struct {
	flags uint32
	done  chan error
}

func newThreadInhibitor(call executionStateFunc) *threadInhibitor {
	t := &threadInhibitor{reqs: make(chan inhibitReq)}
	go func() {
		// Never unlocked: the thread must outlive every Enable so the state holds.
		runtime.LockOSThread()
		for r := range t.reqs {
			var err error
			if call(r.flags) == 0 {
				err = fmt.Errorf("SetThreadExecutionState(%#x) failed", r.flags)
			}
			r.done <- err
		}
	}()
	return t
}

func (t *threadInhibitor) do(flags uint32) error {
	done := make(chan error, 1)
	t.reqs <- inhibitReq{flags: flags, done: done}
	return <-done
}

func (t *threadInhibitor) Enable() error  { return t.do(esContinuous | esSystemRequired) }
func (t *threadInhibitor) Disable() error { return t.do(esContinuous) }
