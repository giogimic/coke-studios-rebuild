package studio

import (
	"path/filepath"
	"testing"

	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/lingo/window"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/ui"
)

func TestMixerWindowDefinitionAndInteractiveElements(t *testing.T) {
	// 1. Verify 6_cc_mixer.window.txt parses cleanly
	windowPath := filepath.Join("Lingo", "mixer", "6_cc_mixer.window.txt")
	def, err := window.ParseFile(windowPath)
	if err != nil {
		t.Fatalf("failed to parse mixer window definition: %v", err)
	}

	if def.Name != "cc_mixer" {
		t.Errorf("expected window name cc_mixer, got %s", def.Name)
	}
	if def.Rect != [4]int{0, 0, 760, 520} {
		t.Errorf("expected rect [0, 0, 760, 520], got %v", def.Rect)
	}

	// 2. Verify all 5 slot buttons exist
	expectedButtons := []string{
		"closemixer",
		"mixer_play_1", "mixer_edit_1", "mixer_burn_1",
		"mixer_play_2", "mixer_edit_2", "mixer_burn_2",
		"mixer_play_3", "mixer_edit_3", "mixer_burn_3",
		"mixer_play_4", "mixer_edit_4", "mixer_burn_4",
		"mixer_play_5", "mixer_edit_5", "mixer_burn_5",
	}

	foundButtons := make(map[string]bool)
	for _, el := range def.Elements {
		for _, eb := range expectedButtons {
			if el.ID == eb {
				foundButtons[eb] = true
			}
		}
	}

	for _, eb := range expectedButtons {
		if !foundButtons[eb] {
			t.Errorf("missing expected interactive element ID %q in 6_cc_mixer.window.txt", eb)
		}
	}

	// 3. Test HUDManager OpenMixerWindow integration
	tm := textmgr.New()
	hud := NewHUDManager(tm)
	hud.windowManager = ui.NewWindowManager(hud.fontManager)

	mixerDir := filepath.Join("Lingo", "mixer")
	assets := lingoassets.LoadAssetsForDefinition(def, mixerDir)
	extra := lingoassets.LoadAssetsByNames([]string{
		"cc.mixer.closebox.pressed",
		"cc.mixer.playbutton.pressed", "cc.mixer.playbutton.on",
		"cc.mixer.editbutton.pressed", "cc.mixer.editbutton.on",
		"cc.mixer.burnbutton.pressed", "cc.mixer.burnbutton.on",
	}, mixerDir)
	for k, v := range extra {
		assets[k] = v
	}

	w := hud.windowManager.OpenWindow("cc_mixer", def, assets, tm)
	if w == nil {
		t.Fatal("failed to open cc_mixer window")
	}

	// Create interactive elements
	for _, el := range w.Definition.Elements {
		if el.ID == "closemixer" || foundButtons[el.ID] {
			ie := w.Renderer.CreateInteractiveElement(el)
			if ie != nil {
				w.Interactive[el.ID] = ie
			}
		}
	}

	// Check interactive buttons created
	for _, eb := range expectedButtons {
		if w.Interactive[eb] == nil {
			t.Errorf("expected interactive button for %s", eb)
		}
	}

	// Wire and trigger close
	closed := false
	w.ButtonClick = func(buttonID string) {
		if buttonID == "closemixer" {
			hud.windowManager.CloseWindow("cc_mixer")
			closed = true
		}
	}

	w.ButtonClick("closemixer")
	if !closed || hud.windowManager.GetWindow("cc_mixer") != nil {
		t.Error("expected closemixer to close window")
	}
}

func TestHUDManagerOpenMixerWindow(t *testing.T) {
	tm := textmgr.New()
	hud := NewHUDManager(tm)
	hud.windowManager = ui.NewWindowManager(hud.fontManager)

	hud.OpenMixerWindow()
	w := hud.windowManager.GetWindow("cc_mixer")
	if w == nil {
		t.Fatal("expected cc_mixer window to be open")
	}
	if w.Rect != [4]int{0, 0, 760, 520} {
		t.Errorf("expected rect [0, 0, 760, 520], got %v", w.Rect)
	}

	slots, ok := w.TempVars["mixer_slots"].([]*MixerSlot)
	if !ok || len(slots) != 5 {
		t.Fatalf("expected 5 mixer slots in TempVars, got %v", w.TempVars["mixer_slots"])
	}

	if w.Interactive["closemixer"] == nil {
		t.Error("expected closemixer button to be interactive")
	}
	if w.Interactive["mixer_play_1"] == nil {
		t.Error("expected mixer_play_1 button to be interactive")
	}

	// Test close
	w.ButtonClick("closemixer")
	if hud.windowManager.GetWindow("cc_mixer") != nil {
		t.Error("expected cc_mixer window to close on closemixer click")
	}
}
