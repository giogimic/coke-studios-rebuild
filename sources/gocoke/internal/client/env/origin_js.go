//go:build js && wasm

package env

import "syscall/js"

// PageOrigin returns window.location.origin when running in the browser.
func PageOrigin() string {
	loc := js.Global().Get("location")
	if !loc.Truthy() {
		return ""
	}
	origin := loc.Get("origin")
	if !origin.Truthy() {
		return ""
	}
	return origin.String()
}
