package platform

import (
	"errors"
	"sync"
	"testing"
)

func TestAutostartCommand(t *testing.T) {
	got := AutostartCommand(`C:\Users\a b\AppData\Local\Programs\wchron\wchron.exe`)
	want := `"C:\Users\a b\AppData\Local\Programs\wchron\wchron.exe" --minimized`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

type fakeKey struct{ values map[string]string }

func (f *fakeKey) get(n string) (string, bool, error) { v, ok := f.values[n]; return v, ok, nil }
func (f *fakeKey) set(n, v string) error              { f.values[n] = v; return nil }
func (f *fakeKey) del(n string) error                 { delete(f.values, n); return nil }

func TestRunKeyAutostart(t *testing.T) {
	k := &fakeKey{values: map[string]string{}}
	a := &runKeyAutostart{
		open:    func() (runKey, func(), error) { return k, func() {}, nil },
		command: AutostartCommand(`C:\app\wchron.exe`),
	}
	if on, err := a.Enabled(); err != nil || on {
		t.Fatalf("fresh key: on=%v err=%v", on, err)
	}
	if err := a.Enable(); err != nil {
		t.Fatal(err)
	}
	if k.values["wchron"] != `"C:\app\wchron.exe" --minimized` {
		t.Fatalf("stored %q", k.values["wchron"])
	}
	if on, _ := a.Enabled(); !on {
		t.Fatal("expected enabled")
	}
	k.values["wchron"] = `"D:\old\wchron.exe" --minimized`
	if on, _ := a.Enabled(); on {
		t.Fatal("a value pointing at another exe is not enabled")
	}
	if err := a.Disable(); err != nil {
		t.Fatal(err)
	}
	if _, ok := k.values["wchron"]; ok {
		t.Fatal("value not removed")
	}
	if err := a.Disable(); err != nil {
		t.Fatalf("disabling twice must succeed: %v", err)
	}
}

func TestRunKeyAutostartOpenError(t *testing.T) {
	a := &runKeyAutostart{open: func() (runKey, func(), error) { return nil, nil, errors.New("denied") }}
	if err := a.Enable(); err == nil {
		t.Fatal("expected error")
	}
	if _, err := a.Enabled(); err == nil {
		t.Fatal("expected error")
	}
}

func TestThreadInhibitor(t *testing.T) {
	var mu sync.Mutex
	var calls []uint32
	fail := false
	inh := newThreadInhibitor(func(flags uint32) uint32 {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, flags)
		if fail {
			return 0
		}
		return esContinuous
	})
	if err := inh.Enable(); err != nil {
		t.Fatal(err)
	}
	if err := inh.Disable(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	fail = true
	mu.Unlock()
	if err := inh.Enable(); err == nil {
		t.Fatal("expected an error when the OS call fails")
	}
	want := []uint32{esContinuous | esSystemRequired, esContinuous, esContinuous | esSystemRequired}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != len(want) {
		t.Fatalf("calls %v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("call %d: %#x, want %#x", i, calls[i], want[i])
		}
	}
}
