// Package assets holds everything the binary serves or draws with, so a
// release is a single file.
package assets

import "embed"

//go:embed static templates wellknown fonts-ttf
var FS embed.FS
