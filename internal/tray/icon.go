package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
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
	colGlyphBg   = color.NRGBA{0x2F, 0x5B, 0xEA, 0xFF} // accent
	colGlyph     = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	colKeepAwake = color.NRGBA{0xF2, 0xB7, 0x05, 0xFF}
	statusColour = map[State]color.NRGBA{
		StateActive:       {0x1F, 0x8A, 0x4C, 0xFF}, // success
		StatePaused:       {0x8A, 0x8A, 0x94, 0xFF}, // muted grey
		StateSending:      {0x00, 0xA3, 0xE0, 0xFF}, // cyan, distinct from the accent square
		StateError:        {0xD9, 0x30, 0x25, 0xFF}, // danger
		StateDisconnected: {0xC7, 0x6A, 0x00, 0xFF}, // warning
	}
)

// Icon draws the tray icon: the "w" glyph on an accent rounded square, a
// coloured status dot at the bottom right and, when keepAwake is on, a yellow
// dot at the top right. Both dots sit in a transparent cut-out ring so they
// stay readable on light and dark taskbars. Drawn at run time (no asset per
// state) with signed distance fields, which give anti-aliased edges for free.
// It returns nil only if PNG encoding fails, which cannot happen on a bytes.Buffer.
func Icon(state State, keepAwake bool) []byte {
	const n = IconSize
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	status, ok := statusColour[state]
	if !ok {
		status = statusColour[StateError]
	}
	// The "w": four strokes through five points, in pixel coordinates.
	w := [][2]float64{{8.5, 11}, {12.25, 22}, {16, 14.5}, {19.75, 22}, {23.5, 11}}
	const (
		stroke   = 1.6 // half width
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
			var c color.NRGBA
			// background square with the glyph
			if a := cover(roundedRect(px, py, 1, 1, 31, 31, 7)); a > 0 {
				glyph := math.Inf(1)
				for i := 0; i+1 < len(w); i++ {
					glyph = math.Min(glyph, segment(px, py, w[i], w[i+1])-stroke)
				}
				c = blend(colGlyphBg, colGlyph, cover(glyph))
				c.A = uint8(float64(c.A) * a)
			}
			// cut-out rings, then the dots
			c = cutout(c, circle(px, py, statusCx, statusCy, ringR))
			if keepAwake {
				c = cutout(c, circle(px, py, awakeCx, awakeCy, ringR-1.5))
				c = over(c, colKeepAwake, cover(circle(px, py, awakeCx, awakeCy, dotR-1.5)))
			}
			c = over(c, status, cover(circle(px, py, statusCx, statusCy, dotR)))
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// cover turns a signed distance (negative inside) into pixel coverage.
func cover(d float64) float64 { return math.Max(0, math.Min(1, 0.5-d)) }

func circle(px, py, cx, cy, r float64) float64 { return math.Hypot(px-cx, py-cy) - r }

func roundedRect(px, py, x0, y0, x1, y1, r float64) float64 {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hx, hy := (x1-x0)/2-r, (y1-y0)/2-r
	dx, dy := math.Abs(px-cx)-hx, math.Abs(py-cy)-hy
	out := math.Hypot(math.Max(dx, 0), math.Max(dy, 0))
	return out + math.Min(math.Max(dx, dy), 0) - r
}

func segment(px, py float64, a, b [2]float64) float64 {
	vx, vy := b[0]-a[0], b[1]-a[1]
	t := ((px-a[0])*vx + (py-a[1])*vy) / (vx*vx + vy*vy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(a[0]+t*vx), py-(a[1]+t*vy))
}

// blend mixes two opaque colours: t=0 gives a, t=1 gives b.
func blend(a, b color.NRGBA, t float64) color.NRGBA {
	mix := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*(1-t) + float64(y)*t)) }
	return color.NRGBA{mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B), 0xFF}
}

// cutout erases dst where the distance d is inside the shape.
func cutout(dst color.NRGBA, d float64) color.NRGBA {
	dst.A = uint8(math.Round(float64(dst.A) * (1 - cover(d))))
	return dst
}

// over composites src with coverage a over dst (non-premultiplied alpha).
func over(dst, src color.NRGBA, a float64) color.NRGBA {
	if a <= 0 {
		return dst
	}
	da := float64(dst.A) / 255
	oa := a + da*(1-a)
	ch := func(s, d uint8) uint8 {
		return uint8(math.Round((float64(s)*a + float64(d)*da*(1-a)) / oa))
	}
	return color.NRGBA{ch(src.R, dst.R), ch(src.G, dst.G), ch(src.B, dst.B), uint8(math.Round(oa * 255))}
}

// stateFor picks the icon state. Paused wins over disconnected: when the user
// paused the routine nothing is due to be sent, so the connection is secondary.
// Sending and error will come from the pipeline (milestone 6).
func stateFor(routineActive, connected bool) State {
	switch {
	case !routineActive:
		return StatePaused
	case !connected:
		return StateDisconnected
	default:
		return StateActive
	}
}
