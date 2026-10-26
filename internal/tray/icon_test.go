package tray

import (
	"bytes"
	"image/png"
	"testing"
)

func TestIconIsAValidPNG(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(Icon(StateActive, false)))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != IconSize || b.Dy() != IconSize {
		t.Fatalf("size %v", b)
	}
	// corners outside the rounded square are transparent
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Fatalf("corner alpha %d", a)
	}
}

func TestIconsDifferPerStateAndKeepAwake(t *testing.T) {
	seen := map[string]string{}
	for _, s := range []State{StateActive, StatePaused, StateSending, StateError, StateDisconnected} {
		for _, k := range []bool{false, true} {
			b := Icon(s, k)
			if b == nil {
				t.Fatal("nil icon")
			}
			name := string(rune('0'+s)) + map[bool]string{false: "", true: "+awake"}[k]
			if other, dup := seen[string(b)]; dup {
				t.Fatalf("%s and %s draw the same icon", name, other)
			}
			seen[string(b)] = name
		}
	}
}

func TestStateFor(t *testing.T) {
	cases := []struct {
		active, connected bool
		want              State
	}{
		{true, false, StateDisconnected},
		{false, false, StatePaused},
		{true, true, StateActive},
		{false, true, StatePaused},
	}
	for _, c := range cases {
		if got := stateFor(c.active, c.connected); got != c.want {
			t.Errorf("stateFor(%v, %v) = %v, want %v", c.active, c.connected, got, c.want)
		}
	}
}
