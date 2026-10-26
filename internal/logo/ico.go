package logo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
)

// ICOSizes are the entries of build/windows/icon.ico: the small/large icon
// sizes Windows asks for at 100–200 % scaling, plus 256 for Explorer tiles.
var ICOSizes = []int{16, 20, 24, 32, 40, 48, 64, 128, 256}

// EncodeICO writes a Windows .ico whose entries are PNG-compressed (read by
// Windows Vista and later, by the Wails syso generator and by NSIS 3). Each
// image must be square, 1..256 px.
//
// Layout: ICONDIR (6 bytes: reserved 0, type 1, count), one 16-byte
// ICONDIRENTRY per image (width and height as one byte, 0 meaning 256; colour
// count 0; planes 1; 32 bits per pixel; data length; data offset), then the
// PNG streams in the same order.
func EncodeICO(images []image.Image) ([]byte, error) {
	if len(images) == 0 || len(images) > 0xFFFF {
		return nil, fmt.Errorf("ico: %d images", len(images))
	}
	pngs := make([][]byte, len(images))
	for i, img := range images {
		b := img.Bounds()
		if b.Dx() != b.Dy() || b.Dx() < 1 || b.Dx() > 256 {
			return nil, fmt.Errorf("ico: image %d is %dx%d, want square 1..256", i, b.Dx(), b.Dy())
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("ico: encode image %d: %w", i, err)
		}
		pngs[i] = buf.Bytes()
	}

	var out bytes.Buffer
	le := binary.LittleEndian
	head := make([]byte, 6)
	le.PutUint16(head[2:], 1)
	le.PutUint16(head[4:], uint16(len(images)))
	out.Write(head)
	offset := 6 + 16*len(images)
	for i, img := range images {
		side := img.Bounds().Dx()
		e := make([]byte, 16)
		e[0] = byte(side % 256) // 256 is stored as 0
		e[1] = byte(side % 256)
		le.PutUint16(e[4:], 1)
		le.PutUint16(e[6:], 32)
		le.PutUint32(e[8:], uint32(len(pngs[i])))
		le.PutUint32(e[12:], uint32(offset))
		out.Write(e)
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		out.Write(p)
	}
	return out.Bytes(), nil
}

// ICO renders the logo at every ICOSizes size and encodes it.
func ICO() ([]byte, error) {
	images := make([]image.Image, len(ICOSizes))
	for i, s := range ICOSizes {
		images[i] = Draw(s)
	}
	return EncodeICO(images)
}
