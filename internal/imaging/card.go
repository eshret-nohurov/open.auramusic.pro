package imaging

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
)

type Card struct {
	Shape  Shape
	Label  string // drawn upper-cased; empty means no label line
	Title  string
	Artist string
	Listen string
}

const (
	cardW, cardH = 1200, 630
	margin       = 80
	textRight    = cardW - margin
	cardQ        = 82
	cardQSmall   = 72
	cardMaxBytes = 300 << 10

	markSize   = 56
	markRadius = 14

	shadowDY     = 16
	shadowRadius = 10 // box radius; three passes spread it to about 30 px
	shadowPad    = 3 * shadowRadius
	shadowAlpha  = 0.5
)

// slot is where a shape puts its cover on the card and where text starts.
type slot struct {
	rect   image.Rectangle
	textX  int
	mask   *image.Alpha
	shadow *image.Alpha // covers rect grown by shadowPad, already offset
}

type cardStatics struct {
	slots   [3]slot
	noCover *image.RGBA
}

func newCardStatics() cardStatics {
	var s cardStatics
	for _, sh := range []Shape{Square, Circle, Wide} {
		w, h, radius := 400, 400, float32(28)
		switch sh {
		case Circle:
			radius = 200
		case Wide:
			w, h, radius = 520, 292, 20
		}
		y := (cardH - h) / 2
		sl := slot{
			rect:  image.Rect(margin, y, margin+w, y+h),
			textX: margin + w + 64,
			mask:  roundRectMask(w, h, radius),
		}
		if sh == Wide {
			sl.textX = margin + w + 56
		}
		sl.shadow = image.NewAlpha(image.Rect(0, 0, w+2*shadowPad, h+2*shadowPad))
		draw.Draw(sl.shadow, sl.mask.Bounds().Add(image.Pt(shadowPad, shadowPad)), sl.mask, image.Point{}, draw.Src)
		blurAlpha(sl.shadow, shadowRadius, 3)
		for i, a := range sl.shadow.Pix {
			sl.shadow.Pix[i] = uint8(float64(a)*shadowAlpha + 0.5)
		}
		s.slots[sh] = sl
	}
	s.noCover = noCoverBackground()
	return s
}

// noCoverBackground is the flat page colour with a faint red glow behind
// where the cover sits, so a card without art still reads as Belet Music.
func noCoverBackground() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, cardW, cardH))
	const cx, cy, radius = 280.0, 315.0, 680.0
	for y := range cardH {
		for x := range cardW {
			d := math.Hypot((float64(x)-cx)/1.15, float64(y)-cy) / radius
			t := 0.0
			if d < 1 {
				t = (1 - d) * (1 - d)
			}
			i := img.PixOffset(x, y)
			img.Pix[i+0] = mix(colBg.R, colRedDeep.R, t)
			img.Pix[i+1] = mix(colBg.G, colRedDeep.G, t)
			img.Pix[i+2] = mix(colBg.B, colRedDeep.B, t)
			img.Pix[i+3] = 0xff
		}
	}
	return img
}

func mix(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t + 0.5)
}

func (r *Renderer) Card(src image.Image, c Card) ([]byte, error) {
	img, _ := r.card(src, c)
	return encodeCard(img)
}

func encodeCard(img image.Image) ([]byte, error) {
	b, err := encodeJPEG(img, cardQ)
	if err != nil || len(b) <= cardMaxBytes {
		return b, err
	}
	// WhatsApp drops heavy previews without a word.
	return encodeJPEG(img, cardQSmall)
}

// card draws the picture and returns the text layout with it, so tests can
// check geometry without reading pixels.
func (r *Renderer) card(src image.Image, c Card) (*image.RGBA, cardLayout) {
	if c.Shape < Square || c.Shape > Wide {
		c.Shape = Square
	}
	sl := r.statics.slots[c.Shape]
	img := image.NewRGBA(image.Rect(0, 0, cardW, cardH))

	var cover *image.RGBA
	if src != nil {
		cover = image.NewRGBA(image.Rect(0, 0, sl.rect.Dx(), sl.rect.Dy()))
		xdraw.CatmullRom.Scale(cover, cover.Bounds(), src, cropTo(src.Bounds(), cover.Bounds().Dx(), cover.Bounds().Dy()), draw.Src, nil)
		blurredBackground(img, cover)
	} else {
		copy(img.Pix, r.statics.noCover.Pix)
		cover = r.placeholder(sl.rect.Dx(), sl.rect.Dy())
	}

	shadowAt := sl.shadow.Bounds().Add(sl.rect.Min.Sub(image.Pt(shadowPad, shadowPad-shadowDY)))
	draw.DrawMask(img, shadowAt, image.Black, image.Point{}, sl.shadow, image.Point{}, draw.Over)
	draw.DrawMask(img, sl.rect, cover, image.Point{}, sl.mask, image.Point{}, draw.Over)

	f := r.newFaces()
	lay := layoutCard(f, c, sl.textX, sl.rect.Min.Y+sl.rect.Dy()/2)
	for _, l := range lay.lines {
		drawText(img, l)
	}
	if c.Listen != "" {
		r.drawListen(img, f, c.Listen)
	}
	return img, lay
}

// blurredBackground fills the card with the cover blown up from a thumbnail,
// which blurs it for a fraction of what a real blur would cost, then darkens
// it, more so towards the text on the right.
func blurredBackground(dst, cover *image.RGBA) {
	small := image.NewRGBA(image.Rect(0, 0, 75, 40))
	xdraw.BiLinear.Scale(small, small.Bounds(), cover, cropTo(cover.Bounds(), cardW, cardH), draw.Src, nil)
	boxBlur(small, 1, 2)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), small, small.Bounds(), draw.Src, nil)

	var col [cardW]uint32
	for x := range cardW {
		t := float64(x) / (cardW - 1)
		col[x] = uint32(0.45 * (1 - 0.5*t*t*(3-2*t)) * 65536)
	}
	for y := range cardH {
		row := dst.Pix[y*dst.Stride:]
		for x := range cardW {
			p := row[x*4 : x*4+3 : x*4+3]
			for i := range p {
				p[i] = uint8(uint32(p[i]) * col[x] >> 16)
			}
		}
	}
}

func (r *Renderer) drawListen(img *image.RGBA, f faces, text string) {
	w := measure(f.listen, text, 0)
	textX := textRight - w
	markX := textX - 18 - markSize
	markY := cardH - 52 - markSize
	at := image.Rect(markX, markY, markX+markSize, markY+markSize)
	draw.Draw(img, at, r.mark, image.Point{}, draw.Over)
	drawText(img, textLine{
		face:     f.listen,
		text:     text,
		x:        textX,
		baseline: markY + (markSize+capHeight(f.listen))/2,
		color:    color.RGBA{0xe8, 0xe3, 0xe3, 0xff},
	})
}
