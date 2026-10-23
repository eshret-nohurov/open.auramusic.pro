package imaging

import (
	"image"
	"image/color"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	labelSize    = 30
	titleSize    = 64
	titleSizeAlt = 54
	artistSize   = 36
	listenSize   = 30
	lineHeight   = 1.15
	labelGap     = 10
	artistGap    = 16
	ellipsis     = "…"
)

// labelTracking matches the page's letter-spacing: .06em on the type label.
const labelTracking = 0.06 * labelSize

// faces are made per card: an opentype.Face keeps a scratch buffer and must
// not be shared between goroutines, while making one costs next to nothing.
type faces struct {
	label, title, titleAlt, artist, listen font.Face
}

func (r *Renderer) newFaces() faces {
	return faces{
		label:    newFace(r.semibold, labelSize),
		title:    newFace(r.semibold, titleSize),
		titleAlt: newFace(r.semibold, titleSizeAlt),
		artist:   newFace(r.medium, artistSize),
		listen:   newFace(r.semibold, listenSize),
	}
}

func newFace(f *opentype.Font, size float64) font.Face {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		// Only invalid options fail here, and these are constants.
		panic(err)
	}
	return face
}

type textLine struct {
	face     font.Face
	text     string
	tracking float64
	x        int
	baseline int
	width    int
	top      int // line box, for layout checks
	bottom   int
	color    color.Color
}

type cardLayout struct {
	lines     []textLine
	titleSize int
	truncated bool
}

// layoutCard stacks label, title and artist in CSS fashion: each line box is
// size×1.15 tall with capitals centred in it, and the whole block is centred
// on the cover's middle.
func layoutCard(f faces, c Card, x, centreY int) cardLayout {
	maxW := textRight - x
	var lay cardLayout
	type part struct {
		face     font.Face
		size     int
		lines    []string
		tracking float64
		gap      int
		color    color.Color
	}
	var parts []part

	if label := strings.ToUpper(strings.TrimSpace(c.Label)); label != "" {
		parts = append(parts, part{f.label, labelSize, []string{ellipsize(f.label, label, maxW, labelTracking)}, labelTracking, 0, colRed})
	}
	if title := strings.TrimSpace(c.Title); title != "" {
		lines, size, cut := layoutTitle(f, title, maxW)
		lay.titleSize, lay.truncated = size, cut
		face := f.title
		if size == titleSizeAlt {
			face = f.titleAlt
		}
		parts = append(parts, part{face, size, lines, 0, labelGap, colText})
	}
	if artist := strings.Join(strings.Fields(c.Artist), " "); artist != "" {
		parts = append(parts, part{f.artist, artistSize, []string{ellipsize(f.artist, artist, maxW, 0)}, 0, artistGap, colArtist})
	}

	total := 0.0
	for i, p := range parts {
		if i > 0 {
			total += float64(p.gap)
		}
		total += float64(len(p.lines)) * float64(p.size) * lineHeight
	}
	y := float64(centreY) - total/2
	for i, p := range parts {
		if i > 0 {
			y += float64(p.gap)
		}
		lh := float64(p.size) * lineHeight
		capH := float64(capHeight(p.face))
		for _, s := range p.lines {
			lay.lines = append(lay.lines, textLine{
				face:     p.face,
				text:     s,
				tracking: p.tracking,
				x:        x,
				baseline: int(y + (lh+capH)/2 + 0.5),
				width:    measure(p.face, s, p.tracking),
				top:      int(y),
				bottom:   int(y + lh + 0.5),
				color:    p.color,
			})
			y += lh
		}
	}
	return lay
}

// layoutTitle prefers two big lines; a longer title drops to the smaller
// size and gets a third line before it is cut.
func layoutTitle(f faces, title string, maxW int) ([]string, int, bool) {
	if lines := wrap(f.title, title, maxW); len(lines) <= 2 {
		return lines, titleSize, false
	}
	lines := wrap(f.titleAlt, title, maxW)
	if len(lines) <= 3 {
		return lines, titleSizeAlt, false
	}
	// Joining the next line in before cutting fills the last line to the
	// edge instead of leaving a gap the next word did not fit into.
	last := cut(f.titleAlt, lines[2]+" "+lines[3], maxW, 0)
	return append(lines[:2:2], last), titleSizeAlt, true
}

// wrap breaks text into lines no wider than maxW, at spaces where it can and
// inside a word only when the word alone is too wide.
func wrap(face font.Face, text string, maxW int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		next := word
		if line != "" {
			next = line + " " + word
		}
		if measure(face, next, 0) <= maxW {
			line = next
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
		line = word
		for measure(face, line, 0) > maxW {
			n := fitRunes(face, line, maxW)
			lines = append(lines, line[:n])
			line = line[n:]
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// fitRunes returns how many bytes of s fit in maxW, at least one rune so a
// single huge glyph cannot loop forever.
func fitRunes(face font.Face, s string, maxW int) int {
	n := 0
	for i, rn := range s {
		end := i + utf8.RuneLen(rn)
		if n > 0 && measure(face, s[:end], 0) > maxW {
			break
		}
		n = end
	}
	return n
}

func ellipsize(face font.Face, s string, maxW int, tracking float64) string {
	if measure(face, s, tracking) <= maxW {
		return s
	}
	return cut(face, s, maxW, tracking)
}

// cut returns the longest prefix of s that fits in maxW with "…" after it,
// dropping spaces and separators the ellipsis would otherwise follow.
func cut(face font.Face, s string, maxW int, tracking float64) string {
	runes := []rune(s)
	for n := len(runes); n > 0; n-- {
		head := strings.TrimRight(string(runes[:n]), " ,;:-–—·")
		if head != "" && measure(face, head+ellipsis, tracking) <= maxW {
			return head + ellipsis
		}
	}
	return ellipsis
}

func measure(face font.Face, s string, tracking float64) int {
	w := font.MeasureString(face, s)
	if tracking != 0 {
		w += fixed.Int26_6(tracking*64) * fixed.Int26_6(utf8.RuneCountInString(s)-1)
	}
	return w.Ceil()
}

func capHeight(face font.Face) int {
	if c := face.Metrics().CapHeight; c > 0 {
		return c.Round()
	}
	b, _ := font.BoundString(face, "H")
	return (-b.Min.Y).Round()
}

func drawText(dst *image.RGBA, l textLine) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(l.color), Face: l.face, Dot: fixed.P(l.x, l.baseline)}
	if l.tracking == 0 {
		d.DrawString(l.text)
		return
	}
	// Tracking is added between glyphs, on top of kerning.
	prev := rune(-1)
	for _, rn := range l.text {
		if prev >= 0 {
			d.Dot.X += l.face.Kern(prev, rn) + fixed.Int26_6(l.tracking*64)
		}
		d.DrawString(string(rn))
		prev = rn
	}
}
