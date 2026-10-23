package studio

import (
	"path/filepath"
	"strings"

	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/lingo/window"
)

// OpenDecibelsWindow opens (or toggles) the Decibels balance window.
// Mirrors Lingo: ElementMgr.newwindow("cc.balance.window") then populate balance text.
func (h *HUDManager) OpenDecibelsWindow() {
	if h == nil || h.windowManager == nil || h.tm == nil {
		return
	}

	// Toggle behavior: if already open, close it
	if h.windowManager.GetWindow("cc_balance") != nil {
		h.windowManager.CloseWindow("cc_balance")
		return
	}

	// Load window definition and assets (from entry and interface dirs)
	windowPath := filepath.Join("Lingo", "entry", "89_cc.balance.window.txt")
	def, err := window.ParseFile(windowPath)
	if err != nil {
		return
	}

	entryDir := filepath.Join("Lingo", "entry")
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, entryDir, interfaceDir)

	// Populate placeholder text before opening window
	display := "Loading..."
	for i := range def.Elements {
		if def.Elements[i].Member == "balance_decibels" {
			// Center the text and show loading placeholder
			def.Elements[i].Alignment = "center"
			def.Elements[i].Key = display
			break
		}
	}

	// Open window. Name contains "balance" so window style/title are auto-selected.
	w := h.windowManager.OpenWindow("cc_balance", def, assets, h.tm)
	if w != nil {
		// Use exact rect from definition
		w.Rect = def.Rect
		// Shift close button left by 10px for better spacing
		w.CloseButtonOffsetX = w.CloseButtonOffsetX - 10
		// Remove frame shadow elements to avoid floating shadow artifacts on this window
		if w.FrameDefinition != nil {
			filtered := make([]window.Element, 0, len(w.FrameDefinition.Elements))
			for _, e := range w.FrameDefinition.Elements {
				if strings.Contains(strings.ToLower(e.Member), "shadow") {
					continue
				}
				filtered = append(filtered, e)
			}
			w.FrameDefinition.Elements = filtered
		}
	}
}
