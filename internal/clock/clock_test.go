package clock

import (
	"testing"
	"time"
)

func TestFake(t *testing.T) {
	start := time.Date(2026, 3, 29, 1, 30, 0, 0, time.UTC)
	var c Clock = NewFake(start)
	if !c.Now().Equal(start) {
		t.Fatalf("got %v", c.Now())
	}
	f := c.(*Fake)
	if got := f.Advance(time.Hour); !got.Equal(start.Add(time.Hour)) || !f.Now().Equal(got) {
		t.Fatalf("advance: got %v", got)
	}
	f.Set(start)
	if !f.Now().Equal(start) {
		t.Fatalf("set: got %v", f.Now())
	}
}

func TestReal(t *testing.T) {
	before := time.Now()
	got := Real{}.Now()
	if got.Before(before) || got.Sub(before) > time.Minute {
		t.Fatalf("got %v, want about %v", got, before)
	}
}
