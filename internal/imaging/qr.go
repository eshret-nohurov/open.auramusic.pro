package imaging

import (
	"bytes"
	"fmt"

	"rsc.io/qr"
)

// quietZone is the blank border, in modules, that scanners need to find
// the code; the white background provides it.
const quietZone = 4

// QRSVG draws each row's dark runs as unit-high rectangles in one path:
// the same squares as module-by-module, at a third of the bytes.
func QRSVG(text string) ([]byte, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, err
	}
	n := code.Size + 2*quietZone
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, n, n)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#fff"/><path fill="#121212" d="`, n, n)
	for y := range code.Size {
		for x := 0; x < code.Size; {
			if !code.Black(x, y) {
				x++
				continue
			}
			run := 1
			for x+run < code.Size && code.Black(x+run, y) {
				run++
			}
			fmt.Fprintf(&b, "M%d %dh%dv1h-%dz", x+quietZone, y+quietZone, run, run)
			x += run
		}
	}
	b.WriteString(`"/></svg>`)
	return b.Bytes(), nil
}
