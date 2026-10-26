package logo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestDrawSizes(t *testing.T) {
	for _, s := range append([]int{1, 1024}, ICOSizes...) {
		if b := Draw(s).Bounds(); b.Dx() != s || b.Dy() != s {
			t.Errorf("Draw(%d) is %v", s, b)
		}
	}
}

func TestDrawCornersAndCentre(t *testing.T) {
	for _, s := range []int{16, 32, 256} {
		img := Draw(s)
		for _, p := range [][2]int{{0, 0}, {s - 1, 0}, {0, s - 1}, {s - 1, s - 1}} {
			if a := img.NRGBAAt(p[0], p[1]).A; a != 0 {
				t.Errorf("size %d: corner %v alpha %d", s, p, a)
			}
		}
		c := img.NRGBAAt(s/2, s/2)
		if c.A != 0xFF {
			t.Errorf("size %d: centre alpha %d, want opaque", s, c.A)
		}
		// at 16 px the dot is 2.25 px wide: no pixel is fully inside it
		if s >= 32 && c != Accent {
			t.Errorf("size %d: centre %v, want the accent dot", s, c)
		}
	}
}

func TestDrawSamplePoints(t *testing.T) {
	// at 64 px, pixel (x,y) has its centre at grid (x+0.5, y+0.5)
	img := Draw(64)
	cases := []struct {
		name string
		x, y int
		want color.NRGBA
	}{
		{"hand", 37, 26, Accent},
		{"bottom-left mark", 15, 47, Accent},
		{"top tick", 31, 12, Tick},
		{"right tick", 51, 31, Tick},
		{"tile", 24, 40, Tile},
		{"border", 0, 32, Border},
		{"border top", 32, 0, Border},
	}
	for _, c := range cases {
		if got := img.NRGBAAt(c.x, c.y); got != c.want {
			t.Errorf("%s at (%d,%d): %v, want %v", c.name, c.x, c.y, got, c.want)
		}
	}
}

func TestEncodeICO(t *testing.T) {
	data, err := ICO()
	if err != nil {
		t.Fatal(err)
	}
	le := binary.LittleEndian
	if le.Uint16(data[0:]) != 0 || le.Uint16(data[2:]) != 1 {
		t.Fatalf("header % x", data[:4])
	}
	if n := int(le.Uint16(data[4:])); n != len(ICOSizes) {
		t.Fatalf("count %d, want %d", n, len(ICOSizes))
	}
	next := 6 + 16*len(ICOSizes)
	for i, s := range ICOSizes {
		e := data[6+16*i : 6+16*(i+1)]
		want := byte(s)
		if s == 256 {
			want = 0
		}
		if e[0] != want || e[1] != want {
			t.Errorf("entry %d: size bytes %d×%d, want %d", i, e[0], e[1], want)
		}
		if le.Uint16(e[4:]) != 1 || le.Uint16(e[6:]) != 32 {
			t.Errorf("entry %d: planes %d, bpp %d", i, le.Uint16(e[4:]), le.Uint16(e[6:]))
		}
		length, offset := int(le.Uint32(e[8:])), int(le.Uint32(e[12:]))
		if offset != next {
			t.Errorf("entry %d: offset %d, want %d", i, offset, next)
		}
		next = offset + length
		img, err := png.Decode(bytes.NewReader(data[offset : offset+length]))
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if b := img.Bounds(); b.Dx() != s || b.Dy() != s {
			t.Errorf("entry %d: png %v, want %d", i, b, s)
		}
	}
	if next != len(data) {
		t.Errorf("data ends at %d, file is %d bytes", next, len(data))
	}
}

func TestEncodeICORejectsBadImages(t *testing.T) {
	for _, imgs := range [][]image.Image{
		nil,
		{image.NewNRGBA(image.Rect(0, 0, 16, 8))},
		{image.NewNRGBA(image.Rect(0, 0, 257, 257))},
	} {
		if _, err := EncodeICO(imgs); err == nil {
			t.Errorf("no error for %d images", len(imgs))
		}
	}
}

// The committed assets must match the generator (`make icons`). Pixels are
// compared, not PNG bytes, so a change in Go's PNG encoder doesn't fail it.
func TestCommittedAssetsInSync(t *testing.T) {
	root := filepath.Join("..", "..")
	svg, err := os.ReadFile(filepath.Join(root, "frontend", "public", "logo.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(svg) != SVG {
		t.Error("frontend/public/logo.svg differs from logo.SVG: run make icons")
	}

	ico, err := os.ReadFile(filepath.Join(root, "build", "windows", "icon.ico"))
	if err != nil {
		t.Fatal(err)
	}
	le := binary.LittleEndian
	if len(ico) < 6 || int(le.Uint16(ico[4:])) != len(ICOSizes) {
		t.Fatal("build/windows/icon.ico has the wrong entry count: run make icons")
	}
	for i, s := range ICOSizes {
		e := ico[6+16*i:]
		length, offset := int(le.Uint32(e[8:])), int(le.Uint32(e[12:]))
		if offset+length > len(ico) {
			t.Fatalf("icon.ico entry %d out of range", i)
		}
		samePixels(t, fmt.Sprintf("icon.ico %dpx", s), ico[offset:offset+length], s)
	}

	app, err := os.ReadFile(filepath.Join(root, "build", "appicon.png"))
	if err != nil {
		t.Fatal(err)
	}
	samePixels(t, "appicon.png", app, 1024)
}

func samePixels(t *testing.T, name string, pngData []byte, size int) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	want := Draw(size)
	if !img.Bounds().Eq(want.Bounds()) {
		t.Errorf("%s: %v, want %dx%d: run make icons", name, img.Bounds(), size, size)
		return
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if color.NRGBAModel.Convert(img.At(x, y)) != want.NRGBAAt(x, y) {
				t.Errorf("%s differs from logo.Draw at (%d,%d): run make icons", name, x, y)
				return
			}
		}
	}
}
