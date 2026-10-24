package web

import "embed"

// Embed all runtime asset directories under web/ for WASM loading (excluding main.wasm and shells).
//go:embed assets data fonts Lingo
var Files embed.FS
