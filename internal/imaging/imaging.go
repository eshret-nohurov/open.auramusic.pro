// Package imaging fetches covers from the CDN and draws everything the site
// serves as a picture: the page cover, its blurred backdrop, the 1200×630
// card for chat apps and the QR code.
package imaging

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
)

type Shape int

const (
	Square Shape = iota
	Circle       // artist
	Wide         // 16:9: video, video_playlist
)

func ShapeFor(itemType string) Shape {
	switch itemType {
	case "artist":
		return Circle
	case "video", "video_playlist":
		return Wide
	}
	return Square
}

// RenderVersion goes into every v= hash. Chat apps cache a card by URL
// forever, so any change to how pictures look must bump it.
const RenderVersion = "1"

func Version(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n") + "\n" + RenderVersion))
	return hex.EncodeToString(sum[:4])
}

var (
	colBg      = color.RGBA{0x12, 0x12, 0x12, 0xff}
	colSurface = color.RGBA{0x1f, 0x1f, 0x1f, 0xff}
	colRedDeep = color.RGBA{0x3b, 0x15, 0x17, 0xff}
	colRed     = color.RGBA{0xea, 0x3d, 0x3d, 0xff}
	colText    = color.RGBA{0xf5, 0xf2, 0xf2, 0xff}
	colArtist  = color.RGBA{0xcf, 0xc8, 0xc8, 0xff}
)

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// cropTo returns the centred part of r with the aspect w:h, the way CSS
// object-fit: cover picks it.
func cropTo(r image.Rectangle, w, h int) image.Rectangle {
	sw, sh := r.Dx(), r.Dy()
	if sw*h > sh*w {
		cw := sh * w / h
		x := r.Min.X + (sw-cw)/2
		return image.Rect(x, r.Min.Y, x+cw, r.Max.Y)
	}
	ch := sw * h / w
	y := r.Min.Y + (sh-ch)/2
	return image.Rect(r.Min.X, y, r.Max.X, y+ch)
}
