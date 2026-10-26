package tray

import (
	"bytes"
	"image/color"
	"image/png"

	"github.com/jacopofilonzi/wchron/internal/logo"
)

// State is what the tray icon shows, from most to least urgent.
type State int

const (
	StateActive State = iota
	StatePaused
	StateSending
	StateError
	StateDisconnected
)

// IconSize is the PNG side in pixels. Windows scales it to the small-icon size
// (16 px at 100 %, 32 px at 200 %), so 32 keeps it sharp on HiDPI.
const IconSize = 32

var (
	colKeepAwake = color.NRGBA{0xF2, 0xB7, 0x05, 0xFF} // yellow
	// Status colours stay clear of the logo's orange (logo.Accent): that is why
	// "disconnected" is purple and not the UI's amber warning.
	statusColour = map[State]color.NRGBA{
		StateActive:       {0x1F, 0x8A, 0x4C, 0xFF}, // green (success)
		StatePaused:       {0x8A, 0x8A, 0x94, 0xFF}, // muted grey
		StateSending:      {0x00, 0xA3, 0xE0, 0xFF}, // cyan
		StateError:        {0xD9, 0x30, 0x25, 0xFF}, // red (danger)
		StateDisconnected: {0x8E, 0x4E, 0xC6, 0xFF}, // purple
	}
)

// Icon draws the tray icon: the logo (internal/logo), a coloured status dot at
// the bottom right and, when keepAwake is on, a yellow dot at the top right.
// Both dots sit in a transparent cut-out ring so they stay readable on light
// and dark taskbars. Drawn at run time (no asset per state) with signed
// distance fields, which give anti-aliased edges for free.
// It returns nil only if PNG encoding fails, which cannot happen on a bytes.Buffer.
func Icon(state State, keepAwake bool) []byte {
	const n = IconSize
	img := logo.Draw(n)
	status, ok := statusColour[state]
	if !ok {
		status = statusColour[StateError]
	}
	const (
		dotR     = 5.5
		ringR    = 7.5
		statusCx = 25.0
		statusCy = 25.0
		awakeCx  = 25.5
		awakeCy  = 6.5
	)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			c := img.NRGBAAt(x, y)
			// cut-out rings, then the dots
			c = logo.Cutout(c, logo.Circle(px, py, statusCx, statusCy, ringR))
			if keepAwake {
				c = logo.Cutout(c, logo.Circle(px, py, awakeCx, awakeCy, ringR-1.5))
				c = logo.Over(c, colKeepAwake, logo.Cover(logo.Circle(px, py, awakeCx, awakeCy, dotR-1.5)))
			}
			c = logo.Over(c, status, logo.Cover(logo.Circle(px, py, statusCx, statusCy, dotR)))
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// stateFor picks the icon state. Sending wins (it is live, and a "send now"
// can run while paused), then a failed run not yet seen (it stays until that
// schedule succeeds again or the Status page is opened), then paused over
// disconnected: when the user paused the routine nothing is due to be sent,
// so the connection is secondary.
func stateFor(routineActive, connected, sending, runError bool) State {
	switch {
	case sending:
		return StateSending
	case runError:
		return StateError
	case !routineActive:
		return StatePaused
	case !connected:
		return StateDisconnected
	default:
		return StateActive
	}
}
