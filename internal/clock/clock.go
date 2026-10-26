// Package clock is the only source of "now" for the time-dependent core.
//
// The scheduler, the pipeline and the token preview take a Clock and never
// call time.Now() directly, so their tests can run on a Fake clock with fixed,
// reproducible times (DST changes, month ends, missed slots) instead of the
// wall clock. main injects Real.
package clock

import (
	"sync"
	"time"
)

// Clock tells the current time.
type Clock interface {
	Now() time.Time
}

// Real is the system wall clock, in the system time zone.
type Real struct{}

// Now returns time.Now().
func (Real) Now() time.Time { return time.Now() }

// Fake is a Clock for tests: it stays at the time it is given until Set or
// Advance moves it. Safe for concurrent use.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a Fake clock standing at t.
func NewFake(t time.Time) *Fake { return &Fake{now: t} }

// Now returns the fake current time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Set moves the clock to t (also backwards).
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}

// Advance moves the clock forward by d and returns the new time.
func (f *Fake) Advance(d time.Duration) time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	return f.now
}
