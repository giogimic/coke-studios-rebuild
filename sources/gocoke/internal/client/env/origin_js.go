//go:build js && wasm

package env

import (
	"net/url"
	"syscall/js"
)

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

// LaunchRoom returns the requested room, never authentication identity.
func LaunchRoom() string {
	loc := js.Global().Get("location")
	if !loc.Truthy() {
		return ""
	}
	search := loc.Get("search").String()
	if len(search) == 0 {
		return ""
	}
	query, err := url.ParseQuery(search[1:])
	if err != nil {
		return ""
	}
	return query.Get("room")
}
