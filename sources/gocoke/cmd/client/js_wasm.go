//go:build js && wasm

package main

import (
	"encoding/base64"
	"log"
	"net/url"
	"runtime/debug"
	"syscall/js"
)

func init() {
	// Aggressive but safe defaults for browser: allow larger live heap before GC
	// to reduce GC frequency and pauses during playback-heavy scenes.
	debug.SetMemoryLimit(512 << 20) // 512MB cap
	debug.SetGCPercent(200)         // fewer GCs; reduce if memory climbs too fast
}

// readJSGlobals reads SW1 and SW2 from JavaScript globals, URL parameters, or localStorage when running in WASM.
func readJSGlobals(sw1, sw2 *string) {
	if jsSW1 := js.Global().Get("sw1"); !jsSW1.IsUndefined() && jsSW1.Type() == js.TypeString && jsSW1.String() != "" {
		*sw1 = jsSW1.String()
		log.Printf("Using SW1 from JavaScript global")
	}
	if jsSW2 := js.Global().Get("sw2"); !jsSW2.IsUndefined() && jsSW2.Type() == js.TypeString && jsSW2.String() != "" {
		*sw2 = jsSW2.String()
		log.Printf("Using SW2 from JavaScript global")
	}

	// Fallback to location.search query params
	loc := js.Global().Get("location")
	if loc.Truthy() {
		search := loc.Get("search").String()
		if len(search) > 1 {
			q, err := url.ParseQuery(search[1:])
			if err == nil {
				if *sw1 == "" {
					if v := q.Get("sw1"); v != "" {
						*sw1 = v
					} else if u := q.Get("user"); u != "" {
						*sw1 = base64.StdEncoding.EncodeToString([]byte(u))
					} else if u := q.Get("screenName"); u != "" {
						*sw1 = base64.StdEncoding.EncodeToString([]byte(u))
					} else if u := q.Get("name"); u != "" {
						*sw1 = base64.StdEncoding.EncodeToString([]byte(u))
					}
				}
				if *sw2 == "" {
					if v := q.Get("sw2"); v != "" {
						*sw2 = v
					} else if sid := q.Get("sid"); sid != "" {
						*sw2 = sid
					} else if pass := q.Get("pass"); pass != "" {
						*sw2 = pass
					}
				}
			}
		}
	}

	// Fallback to localStorage
	ls := js.Global().Get("localStorage")
	if ls.Truthy() {
		if *sw1 == "" {
			storedUser := ls.Call("getItem", "coke_user")
			if storedUser.Truthy() && storedUser.Type() == js.TypeString && storedUser.String() != "" {
				*sw1 = base64.StdEncoding.EncodeToString([]byte(storedUser.String()))
				log.Printf("Using SW1 derived from localStorage coke_user")
			}
		}
		if *sw2 == "" {
			storedSession := ls.Call("getItem", "coke_session")
			if storedSession.Truthy() && storedSession.Type() == js.TypeString && storedSession.String() != "" {
				*sw2 = storedSession.String()
				log.Printf("Using SW2 from localStorage coke_session")
			}
		}
	}
}
