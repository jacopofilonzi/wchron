package logo

import (
	"image/color"
	"math"
)

// Signed distance helpers (negative inside), shared with the tray icon.

// Cover turns a signed distance in pixels into pixel coverage (0..1).
func Cover(d float64) float64 { return math.Max(0, math.Min(1, 0.5-d)) }

// Circle is the distance from (px,py) to the circle at (cx,cy) of radius r.
func Circle(px, py, cx, cy, r float64) float64 { return math.Hypot(px-cx, py-cy) - r }

// RoundedRect is the distance to the rectangle x0..x1, y0..y1 with corner radius r.
func RoundedRect(px, py, x0, y0, x1, y1, r float64) float64 {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hx, hy := (x1-x0)/2-r, (y1-y0)/2-r
	dx, dy := math.Abs(px-cx)-hx, math.Abs(py-cy)-hy
	out := math.Hypot(math.Max(dx, 0), math.Max(dy, 0))
	return out + math.Min(math.Max(dx, dy), 0) - r
}

// Segment is the distance to the segment a→b; subtract the half width for a
// stroke with round caps.
func Segment(px, py float64, a, b [2]float64) float64 {
	vx, vy := b[0]-a[0], b[1]-a[1]
	t := ((px-a[0])*vx + (py-a[1])*vy) / (vx*vx + vy*vy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(a[0]+t*vx), py-(a[1]+t*vy))
}

// Cutout erases dst by the coverage of a shape at distance d.
func Cutout(dst color.NRGBA, d float64) color.NRGBA {
	dst.A = uint8(math.Round(float64(dst.A) * (1 - Cover(d))))
	return dst
}

// Over composites opaque src with coverage a over dst (non-premultiplied alpha).
func Over(dst, src color.NRGBA, a float64) color.NRGBA {
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
