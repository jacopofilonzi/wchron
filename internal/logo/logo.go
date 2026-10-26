// Package logo draws the wchron logo. It is the single source of truth for the
// raster sizes (exe/installer icon, appicon, tray base); frontend/public/logo.svg
// and components/Logo.svelte carry the same geometry as vector (SVG below), and
// `make icons` regenerates every asset from here.
//
// Geometry, on a 64×64 grid (y down):
//
//   - tile: rounded square x 0..64, y 0..64, corner radius 15, fill #17171C;
//   - border: 2 units wide just inside the tile edge (centre line x 1..63,
//     radius 14), #3A3A44, so the dark tile stays visible on dark backgrounds;
//   - ticks: white, width 4, round caps: (32,10)→(32,16), (32,48)→(32,54),
//     (10,32)→(16,32), (48,32)→(54,32);
//   - hand: #FF8A3D, width 5, round caps, (32,32)→(43,21);
//   - centre dot: #FF8A3D circle at (32,32), r 4.5;
//   - bottom-left mark: #FF8A3D rounded square x 11.5..20.5, y 43.5..52.5,
//     radius 2.5.
//
// Shapes are signed distance fields (negative inside) sampled at pixel
// centres, which gives anti-aliased edges at every size without supersampling.
package logo

import (
	"image"
	"image/color"
)

// SVG is the vector logo, written verbatim to frontend/public/logo.svg; keep it
// byte-stable and in sync with Draw and components/Logo.svelte.
const SVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="15" fill="#17171C"/><rect x="1" y="1" width="62" height="62" rx="14" fill="none" stroke="#3A3A44" stroke-width="2"/><path d="M32 10v6M32 48v6M10 32h6M48 32h6" stroke="#fff" stroke-width="4" stroke-linecap="round"/><path d="M32 32l11-11" stroke="#FF8A3D" stroke-width="5" stroke-linecap="round"/><circle cx="32" cy="32" r="4.5" fill="#FF8A3D"/><rect x="11.5" y="43.5" width="9" height="9" rx="2.5" fill="#FF8A3D"/></svg>
`

// Colours of the logo.
var (
	Tile   = color.NRGBA{0x17, 0x17, 0x1C, 0xFF}
	Border = color.NRGBA{0x3A, 0x3A, 0x44, 0xFF}
	Tick   = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	Accent = color.NRGBA{0xFF, 0x8A, 0x3D, 0xFF}
)

var ticks = [][2][2]float64{
	{{32, 10}, {32, 16}},
	{{32, 48}, {32, 54}},
	{{10, 32}, {16, 32}},
	{{48, 32}, {54, 32}},
}

// Draw renders the logo as a size×size image with transparent corners.
func Draw(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	if size <= 0 {
		return img
	}
	scale := float64(size) / 64 // pixels per grid unit
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			// grid coordinates of the pixel centre; distances are scaled back
			// to pixels before turning them into coverage
			u, v := (float64(x)+0.5)/scale, (float64(y)+0.5)/scale
			px := func(d float64) float64 { return Cover(d * scale) }

			tile := RoundedRect(u, v, 0, 0, 64, 64, 15)
			a := px(tile)
			if a <= 0 {
				continue
			}
			c := Tile
			// border: the band 0..2 units inside the tile edge
			c = Over(c, Border, px(-tile-2))
			tick := 1e9
			for _, t := range ticks {
				tick = min(tick, Segment(u, v, t[0], t[1])-2)
			}
			c = Over(c, Tick, px(tick))
			c = Over(c, Accent, px(Segment(u, v, [2]float64{32, 32}, [2]float64{43, 21})-2.5))
			c = Over(c, Accent, px(Circle(u, v, 32, 32, 4.5)))
			c = Over(c, Accent, px(RoundedRect(u, v, 11.5, 43.5, 20.5, 52.5, 2.5)))
			c.A = uint8(float64(c.A)*a + 0.5)
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}
