package imaging

import (
	"image"
	"image/png"
	"io/fs"
	"testing"

	"golang.org/x/image/font/sfnt"

	"github.com/eshret-nohurov/open.auramusic.pro/assets"
)

func TestFontsHaveTurkmenAndCyrillic(t *testing.T) {
	letters := "ÝýŇňŽžÇçÄäÖöÜüŞş…«»—"
	for r := 'А'; r <= 'я'; r++ {
		letters += string(r)
	}
	letters += "Ёё"
	for _, name := range []string{"fonts-ttf/jost_semibold.ttf", "fonts-ttf/jost_medium.ttf"} {
		b, err := fs.ReadFile(assets.FS, name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := sfnt.Parse(b)
		if err != nil {
			t.Fatal(err)
		}
		var buf sfnt.Buffer
		for _, r := range letters {
			if i, err := f.GlyphIndex(&buf, r); err != nil || i == 0 {
				t.Errorf("%s has no glyph for %q", name, r)
			}
		}
	}
}

func TestLogoRastersHaveAlpha(t *testing.T) {
	for _, name := range []string{"raster/logo-mark-256.png", "raster/logo-mark-mono-256.png"} {
		f, err := assets.FS.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds() != image.Rect(0, 0, 256, 256) {
			t.Errorf("%s is %v, want 256×256", name, img.Bounds())
		}
		alpha := func(x, y int) uint32 { _, _, _, a := img.At(x, y).RGBA(); return a >> 8 }
		if a := alpha(0, 0); a > 8 {
			t.Errorf("%s corner alpha %d, want transparent", name, a)
		}
		if a := alpha(128, 20); a != 255 {
			t.Errorf("%s body alpha %d, want opaque", name, a)
		}
	}
}
