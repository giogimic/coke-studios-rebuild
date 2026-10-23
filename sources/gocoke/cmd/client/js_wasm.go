//go:build js && wasm

package main

import (
	"log"
	"syscall/js"
)
import "runtime/debug"

func init() {
	// Aggressive but safe defaults for browser: allow larger live heap before GC
	// to reduce GC frequency and pauses during playback-heavy scenes.
	// Adjust as needed after measuring.
	debug.SetMemoryLimit(512 << 20) // 512MB cap
	debug.SetGCPercent(200)         // fewer GCs; reduce if memory climbs too fast
}



// readJSGlobals reads SW1 and SW2 from JavaScript globals when running in WASM
func readJSGlobals(sw1, sw2 *string) {
	if jsSW1 := js.Global().Get("sw1"); !jsSW1.IsUndefined() && jsSW1.Type() == js.TypeString {
		*sw1 = jsSW1.String()
		log.Printf("Using SW1 from JavaScript global")
	}
	if jsSW2 := js.Global().Get("sw2"); !jsSW2.IsUndefined() && jsSW2.Type() == js.TypeString {
		*sw2 = jsSW2.String()
		log.Printf("Using SW2 from JavaScript global")
	}
}
