package imaging

import (
	"image"
	"image/color"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

var (
	viewBoxRe = regexp.MustCompile(`viewBox="0 0 (\d+) (\d+)"`)
	runRe     = regexp.MustCompile(`M(\d+) (\d+)h(\d+)v1h-(\d+)z`)
)

func TestQRSVG(t *testing.T) {
	text := "https://open.auramusic.pro/s/AbCdEfGh12345678"
	svg, err := QRSVG(text)
	if err != nil {
		t.Fatal(err)
	}
	s := string(svg)
	if !strings.HasPrefix(s, "<svg ") || !strings.HasSuffix(s, "</svg>") {
		t.Fatalf("not an svg: %.80s", s)
	}
	if strings.Count(s, "<path") != 1 || !strings.Contains(s, `shape-rendering="crispEdges"`) {
		t.Errorf("want one path and crispEdges: %.200s", s)
	}
	m := viewBoxRe.FindStringSubmatch(s)
	if m == nil || m[1] != m[2] {
		t.Fatalf("viewBox: %v", m)
	}
	n, _ := strconv.Atoi(m[1])

	const scale = 6
	img := image.NewGray(image.Rect(0, 0, n*scale, n*scale))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	runs := runRe.FindAllStringSubmatch(s, -1)
	if len(runs) == 0 {
		t.Fatal("no modules")
	}
	for _, r := range runs {
		x, _ := strconv.Atoi(r[1])
		y, _ := strconv.Atoi(r[2])
		w, _ := strconv.Atoi(r[3])
		if x < quietZone || y < quietZone || x+w > n-quietZone || y >= n-quietZone {
			t.Fatalf("module at %d,%d w%d inside the %d-module quiet zone", x, y, w, quietZone)
		}
		for py := y * scale; py < (y+1)*scale; py++ {
			for px := x * scale; px < (x+w)*scale; px++ {
				img.SetGray(px, py, color.Gray{})
			}
		}
	}

	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	res, err := qrcode.NewQRCodeReader().Decode(bmp, map[gozxing.DecodeHintType]any{gozxing.DecodeHintType_PURE_BARCODE: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetText() != text {
		t.Errorf("decoded %q", res.GetText())
	}
	if lvl := res.GetResultMetadata()[gozxing.ResultMetadataType_ERROR_CORRECTION_LEVEL]; lvl != "M" {
		t.Errorf("level %v, want M", lvl)
	}
	t.Logf("QR %d modules, %d bytes", n, len(svg))
}
