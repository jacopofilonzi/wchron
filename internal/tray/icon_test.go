package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/jacopofilonzi/wchron/internal/logo"
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
		active, connected, sending, runError bool
		want                                 State
	}{
		{true, false, false, false, StateDisconnected},
		{false, false, false, false, StatePaused},
		{true, true, false, false, StateActive},
		{false, true, false, false, StatePaused},
		{true, true, true, true, StateSending},
		{false, false, true, false, StateSending},
		{true, true, false, true, StateError},
		{false, false, false, true, StateError},
	}
	for _, c := range cases {
		if got := stateFor(c.active, c.connected, c.sending, c.runError); got != c.want {
			t.Errorf("stateFor(%+v) = %v, want %v", c, got, c.want)
		}
	}
}

func TestIconDotColours(t *testing.T) {
	decode := func(b []byte) *image.NRGBA {
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		n, ok := img.(*image.NRGBA)
		if !ok {
			t.Fatalf("decoded as %T", img)
		}
		return n
	}
	purple := color.NRGBA{0x8E, 0x4E, 0xC6, 0xFF}
	yellow := color.NRGBA{0xF2, 0xB7, 0x05, 0xFF}
	img := decode(Icon(StateDisconnected, true))
	if c := img.NRGBAAt(25, 25); c != purple {
		t.Errorf("status dot %v, want purple", c)
	}
	if c := img.NRGBAAt(25, 6); c != yellow {
		t.Errorf("keep-awake dot %v, want yellow", c)
	}
	// the logo is underneath: its centre dot is the accent orange
	if c := img.NRGBAAt(16, 16); c != logo.Accent {
		t.Errorf("logo centre %v, want %v", c, logo.Accent)
	}
	// the status colours never match the logo's orange
	for s, c := range statusColour {
		if c == logo.Accent {
			t.Errorf("state %d uses the logo accent", s)
		}
	}
}
