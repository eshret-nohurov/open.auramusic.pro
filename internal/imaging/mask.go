package imaging

import (
	"image"

	"golang.org/x/image/vector"
)

// kappa places cubic control points so four curves make a near-perfect
// quarter circle each.
const kappa = 0.5522847498

// roundRectMask is an antialiased coverage mask; the rasterizer works in
// float coordinates, so curved edges come out smooth instead of stepped.
func roundRectMask(w, h int, radius float32) *image.Alpha {
	z := vector.NewRasterizer(w, h)
	fw, fh, r := float32(w), float32(h), radius
	k := r * (1 - kappa)
	z.MoveTo(r, 0)
	z.LineTo(fw-r, 0)
	z.CubeTo(fw-k, 0, fw, k, fw, r)
	z.LineTo(fw, fh-r)
	z.CubeTo(fw, fh-k, fw-k, fh, fw-r, fh)
	z.LineTo(r, fh)
	z.CubeTo(k, fh, 0, fh-k, 0, fh-r)
	z.LineTo(0, r)
	z.CubeTo(0, k, k, 0, r, 0)
	z.ClosePath()
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	z.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

// applyMask scales premultiplied pixels by the mask, which is what clipping
// with soft edges means for an RGBA image.
func applyMask(img *image.RGBA, m *image.Alpha) {
	b := img.Bounds()
	for y := range b.Dy() {
		row := img.Pix[y*img.Stride:]
		mrow := m.Pix[y*m.Stride:]
		for x := range b.Dx() {
			a := uint32(mrow[x])
			if a == 0xff {
				continue
			}
			p := row[x*4 : x*4+4 : x*4+4]
			for i := range p {
				p[i] = uint8((uint32(p[i])*a + 127) / 255)
			}
		}
	}
}
