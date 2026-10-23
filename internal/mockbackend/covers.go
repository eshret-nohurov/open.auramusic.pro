package mockbackend

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// cover serves generated pictures, so the repo carries no image files and
// every fixture still gets a distinct, recognisable cover.
func (s *Server) cover(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	switch name {
	case "broken.jpg":
		// A CDN error page sent with an image content type.
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("<!doctype html><html><body><h1>502 Bad Gateway</h1></body></html>"))
		return
	case "huge.png":
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngHeader(8000, 8000))
		return
	case "redirect.jpg":
		http.Redirect(w, r, "https://example.com/cover.jpg", http.StatusFound)
		return
	case "missing.jpg":
		http.NotFound(w, r)
		return
	}
	if !strings.HasSuffix(name, ".jpg") {
		http.NotFound(w, r)
		return
	}

	s.coversMu.Lock()
	b, ok := s.covers[name]
	if !ok {
		b = generate(name)
		s.covers[name] = b
	}
	s.coversMu.Unlock()

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.Write(b)
}

func coverSize(name string) (int, int) {
	switch {
	case strings.HasPrefix(name, "video"):
		return 1280, 720
	case strings.HasPrefix(name, "small"):
		return 300, 300
	case strings.Contains(name, "thumb"):
		return 200, 200
	}
	return 1000, 1000
}

// generate draws a diagonal gradient with rings; colours come from the name.
func generate(name string) []byte {
	w, h := coverSize(name)
	f := fnv.New32a()
	f.Write([]byte(name))
	seed := f.Sum32()
	c1 := hsv(float64(seed%360), 0.75, 0.9)
	c2 := hsv(float64((seed/360+150)%360), 0.8, 0.35)

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	cx, cy := float64(w)*0.62, float64(h)*0.38
	ring := float64(min(w, h)) / 9
	for y := range h {
		for x := range w {
			t := float64(x+y) / float64(w+h)
			c := lerp(c1, c2, t)
			if int(math.Hypot(float64(x)-cx, float64(y)-cy)/ring)%2 == 0 {
				c = lerp(c, color.RGBA{255, 255, 255, 255}, 0.12)
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 255}
}

func hsv(h, s, v float64) color.RGBA {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	m := v - c
	return color.RGBA{uint8((r + m) * 255), uint8((g + m) * 255), uint8((b + m) * 255), 255}
}

// pngHeader is a PNG that claims the given size and carries no pixels: the
// cheapest way to test that the site checks dimensions before decoding.
func pngHeader(w, h uint32) []byte {
	var buf bytes.Buffer
	buf.WriteString("\x89PNG\r\n\x1a\n")
	chunk := func(typ string, data []byte) {
		binary.Write(&buf, binary.BigEndian, uint32(len(data)))
		buf.WriteString(typ)
		buf.Write(data)
		binary.Write(&buf, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 2 // 8-bit RGB
	chunk("IHDR", ihdr)
	chunk("IEND", nil)
	return buf.Bytes()
}
