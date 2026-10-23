package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"sync"
	"testing"
)

var (
	sharedRenderer *Renderer
	rendererOnce   sync.Once
)

func renderer(t testing.TB) *Renderer {
	t.Helper()
	rendererOnce.Do(func() {
		r, err := NewRenderer()
		if err != nil {
			t.Fatal(err)
		}
		sharedRenderer = r
	})
	if sharedRenderer == nil {
		t.Fatal("renderer failed to load")
	}
	return sharedRenderer
}

func gradient(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 160, 255})
		}
	}
	return img
}

func jpegSize(t *testing.T, b []byte) image.Point {
	t.Helper()
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return image.Pt(cfg.Width, cfg.Height)
}

func TestShapeFor(t *testing.T) {
	for typ, want := range map[string]Shape{
		"artist": Circle, "video": Wide, "video_playlist": Wide,
		"track": Square, "album": Square, "playlist": Square, "user_playlist": Square, "": Square,
	} {
		if got := ShapeFor(typ); got != want {
			t.Errorf("ShapeFor(%q) = %v, want %v", typ, got, want)
		}
	}
}

func TestVersion(t *testing.T) {
	a := Version("https://cdn/x.jpg")
	if len(a) != 8 {
		t.Fatalf("Version length %d", len(a))
	}
	if a != Version("https://cdn/x.jpg") || a == Version("https://cdn/y.jpg") {
		t.Error("Version is not a stable function of its input")
	}
	if Version("a", "b") == Version("a\nb", "") || Version("a", "b") == Version("ab") {
		t.Error("Version parts collide")
	}
}

func TestCoverSizes(t *testing.T) {
	r := renderer(t)
	cases := []struct {
		name  string
		src   image.Image
		shape Shape
		want  image.Point
	}{
		{"square", gradient(1000, 1000), Square, image.Pt(640, 640)},
		{"circle from wide", gradient(1280, 720), Circle, image.Pt(640, 640)},
		{"wide", gradient(1280, 720), Wide, image.Pt(640, 360)},
		{"wide from square", gradient(1000, 1000), Wide, image.Pt(640, 360)},
		{"small not upscaled", gradient(300, 300), Square, image.Pt(300, 300)},
		{"small wide not upscaled", gradient(320, 180), Wide, image.Pt(320, 180)},
		{"no cover", nil, Square, image.Pt(640, 640)},
		{"no cover circle", nil, Circle, image.Pt(640, 640)},
		{"no cover wide", nil, Wide, image.Pt(640, 360)},
	}
	for _, c := range cases {
		b, err := r.Cover(c.src, c.shape)
		if err != nil {
			t.Fatal(err)
		}
		if got := jpegSize(t, b); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBackgroundSizes(t *testing.T) {
	r := renderer(t)
	for _, c := range []struct {
		src   image.Image
		shape Shape
		want  image.Point
	}{
		{gradient(1000, 1000), Square, image.Pt(48, 48)},
		{gradient(1000, 1000), Circle, image.Pt(48, 48)},
		{gradient(1280, 720), Wide, image.Pt(64, 36)},
		{nil, Square, image.Pt(48, 48)},
		{nil, Wide, image.Pt(64, 36)},
	} {
		b, err := r.Background(c.src, c.shape)
		if err != nil {
			t.Fatal(err)
		}
		if got := jpegSize(t, b); got != c.want {
			t.Errorf("shape %v: %v, want %v", c.shape, got, c.want)
		}
		if len(b) > 3<<10 {
			t.Errorf("shape %v: background weighs %d bytes", c.shape, len(b))
		}
	}
}

func TestDefaultCard(t *testing.T) {
	b := renderer(t).DefaultCard()
	if got := jpegSize(t, b); got != image.Pt(1200, 630) {
		t.Errorf("default card %v", got)
	}
	if len(b) > cardMaxBytes {
		t.Errorf("default card weighs %d bytes", len(b))
	}
}
