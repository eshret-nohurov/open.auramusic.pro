package imaging

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font/opentype"

	"github.com/eshret-nohurov/open.auramusic.pro/assets"
)

// Renderer holds what every picture needs and is read-only after
// NewRenderer, so one value serves all requests.
type Renderer struct {
	semibold *opentype.Font
	medium   *opentype.Font
	mark     *image.RGBA // colour logo mark, 56×56, corners already rounded
	mono     *image.RGBA // grey mark for placeholders, 256×256
	statics  cardStatics
	defCard  []byte
}

const (
	coverMax  = 640
	coverQ    = 85
	bgQ       = 70
	markAlpha = 102 // 40 % opacity for the grey mark on a placeholder
)

func NewRenderer() (*Renderer, error) {
	r := &Renderer{}
	var err error
	if r.semibold, err = loadFont("fonts-ttf/jost_semibold.ttf"); err != nil {
		return nil, err
	}
	if r.medium, err = loadFont("fonts-ttf/jost_medium.ttf"); err != nil {
		return nil, err
	}
	mark, err := loadPNG("raster/logo-mark-256.png")
	if err != nil {
		return nil, err
	}
	if r.mono, err = loadPNG("raster/logo-mark-mono-256.png"); err != nil {
		return nil, err
	}
	r.mark = image.NewRGBA(image.Rect(0, 0, markSize, markSize))
	xdraw.CatmullRom.Scale(r.mark, r.mark.Bounds(), mark, mark.Bounds(), draw.Src, nil)
	applyMask(r.mark, roundRectMask(markSize, markSize, markRadius))

	r.statics = newCardStatics()

	def, err := loadPNG("static/og-default.png")
	if err != nil {
		return nil, err
	}
	flat := image.NewRGBA(def.Bounds())
	draw.Draw(flat, flat.Bounds(), image.NewUniform(colBg), image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), def, def.Bounds().Min, draw.Over)
	if r.defCard, err = encodeCard(flat); err != nil {
		return nil, err
	}
	return r, nil
}

func loadFont(name string) (*opentype.Font, error) {
	b, err := fs.ReadFile(assets.FS, name)
	if err != nil {
		return nil, err
	}
	f, err := opentype.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return f, nil
}

func loadPNG(name string) (*image.RGBA, error) {
	f, err := assets.FS.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return toRGBA(img), nil
}

func toRGBA(img image.Image) *image.RGBA {
	if m, ok := img.(*image.RGBA); ok && m.Bounds().Min == (image.Point{}) {
		return m
	}
	b := img.Bounds()
	m := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(m, m.Bounds(), img, b.Min, draw.Src)
	return m
}

func aspect(s Shape) (int, int) {
	if s == Wide {
		return 16, 9
	}
	return 1, 1
}

// Cover is the picture on the page. A circle is left square: CSS rounds it,
// and a JPEG has no transparency to round it with.
func (r *Renderer) Cover(src image.Image, shape Shape) ([]byte, error) {
	aw, ah := aspect(shape)
	if src == nil {
		w := coverMax
		return encodeJPEG(r.placeholder(w, w*ah/aw), coverQ)
	}
	crop := cropTo(src.Bounds(), aw, ah)
	w := min(coverMax, crop.Dx())
	h := w * ah / aw
	if w == crop.Dx() {
		h = crop.Dy()
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if crop.Dx() == w && crop.Dy() == h {
		draw.Draw(dst, dst.Bounds(), src, crop.Min, draw.Src)
	} else {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Src, nil)
	}
	return encodeJPEG(dst, coverQ)
}

// Background is the tiny backdrop the page stretches and blurs behind the
// cover. It is meant to weigh a couple of kilobytes on a 3G line.
func (r *Renderer) Background(src image.Image, shape Shape) ([]byte, error) {
	w, h := 48, 48
	if shape == Wide {
		w, h = 64, 36
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if src == nil {
		draw.Draw(dst, dst.Bounds(), image.NewUniform(colBg), image.Point{}, draw.Src)
		return encodeJPEG(dst, bgQ)
	}
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, cropTo(src.Bounds(), w, h), draw.Src, nil)
	boxBlur(dst, 1, 3)
	return encodeJPEG(dst, bgQ)
}

// DefaultCard is shared: callers must not modify the slice.
func (r *Renderer) DefaultCard() []byte { return r.defCard }

// placeholder stands in for a missing cover: a dark tile with the grey mark.
func (r *Renderer) placeholder(w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(colSurface), image.Point{}, draw.Src)
	side := min(w, h) * 36 / 100
	at := image.Rect((w-side)/2, (h-side)/2, (w+side)/2, (h+side)/2)
	mark := image.NewRGBA(image.Rect(0, 0, side, side))
	xdraw.CatmullRom.Scale(mark, mark.Bounds(), r.mono, r.mono.Bounds(), draw.Src, nil)
	draw.DrawMask(dst, at, mark, image.Point{}, image.NewUniform(color.Alpha{markAlpha}), image.Point{}, draw.Over)
	return dst
}

// boxBlur runs passes of a (2r+1)-wide box filter in each direction; three
// passes come close to a gaussian. Edges repeat the border pixel.
func boxBlur(img *image.RGBA, radius, passes int) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	tmp := make([]uint8, len(img.Pix))
	for range passes {
		blurLines(img.Pix, tmp, w, h, 4, img.Stride, 4, radius)
		blurLines(tmp, img.Pix, h, w, img.Stride, 4, 4, radius)
	}
}

func blurAlpha(img *image.Alpha, radius, passes int) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	tmp := make([]uint8, len(img.Pix))
	for range passes {
		blurLines(img.Pix, tmp, w, h, 1, img.Stride, 1, radius)
		blurLines(tmp, img.Pix, h, w, img.Stride, 1, 1, radius)
	}
}

// blurLines blurs n lines of length l from src into dst with a running sum.
// step moves along a line, next moves to the following one.
func blurLines(src, dst []uint8, l, n, step, next, channels, radius int) {
	span := 2*radius + 1
	for line := range n {
		base := line * next
		for c := range channels {
			at := func(i int) int { return int(src[base+min(max(i, 0), l-1)*step+c]) }
			sum := 0
			for i := -radius; i <= radius; i++ {
				sum += at(i)
			}
			for i := range l {
				dst[base+i*step+c] = uint8((sum + span/2) / span)
				sum += at(i+radius+1) - at(i-radius)
			}
		}
	}
}
