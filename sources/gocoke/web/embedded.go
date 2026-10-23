package web

import "embed"

// Embed all files under web/ for pure WASM asset loading.
//go:embed **
var Files embed.FS
