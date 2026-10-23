//go:build !js || !wasm

package main

// readJSGlobals is a no-op when not building for WASM
func readJSGlobals(sw1, sw2 *string) {
	// No-op for non-WASM builds
}
