//go:build !js || !wasm

package env

// PageOrigin returns empty on native builds.
func PageOrigin() string { return "" }

func LaunchRoom() string { return "" }
