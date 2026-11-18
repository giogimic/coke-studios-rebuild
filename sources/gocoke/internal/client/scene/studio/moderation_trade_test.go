package studio

import (
	"path/filepath"
	"strings"
	"testing"

	"gocoke/internal/client/assets/lingo/window"
	"gocoke/internal/client/assets/studios"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/ui"
)

func TestDecisionDialogAndDoorbellWindows(t *testing.T) {
	// 1. Verify 9_sanfo_decision_dialog.window.txt parses cleanly
	defPath := filepath.Join("Lingo", "cc_interface[1]", "9_sanfo_decision_dialog.window.txt")
	def, err := window.ParseFile(defPath)
	if err != nil {
		t.Fatalf("failed to parse decision dialog definition: %v", err)
	}

	if def.Name != "sanfo_decision_dialog" {
		t.Errorf("expected window name sanfo_decision_dialog, got %s", def.Name)
	}

	foundOk := false
	foundCancel := false
	foundTextA := false
	foundTextB := false
	for _, el := range def.Elements {
		if el.ID == "sanfo_decision_ok" {
			foundOk = true
		}
		if el.ID == "sanfo_decision_cancel" {
			foundCancel = true
		}
		if el.ID == "sanfo_decision_text_a" {
			foundTextA = true
		}
		if el.ID == "sanfo_decision_text_b" {
			foundTextB = true
		}
	}
	if !foundOk || !foundCancel || !foundTextA || !foundTextB {
		t.Errorf("missing expected elements in 9_sanfo_decision_dialog: ok=%v cancel=%v textA=%v textB=%v",
			foundOk, foundCancel, foundTextA, foundTextB)
	}

	// 2. Test HUDManager OpenDecisionDialog
	tm := textmgr.New()
	hud := NewHUDManager(tm)
	hud.windowManager = ui.NewWindowManager(hud.fontManager)

	okCalled := false
	cancelCalled := false

	hud.OpenDecisionDialog("Visitor at Door", "Bob is knocking. Let them in?", "Let In", "Deny",
		func() { okCalled = true },
		func() { cancelCalled = true },
	)

	w := hud.windowManager.GetWindow("sanfo_decision_dialog")
	if w == nil {
		t.Fatalf("expected sanfo_decision_dialog window to be open in windowManager")
	}

	// Verify text content
	for _, el := range w.Definition.Elements {
		if el.ID == "sanfo_decision_text_a" && el.Key != "Visitor at Door" {
			t.Errorf("expected title 'Visitor at Door', got %q", el.Key)
		}
		if el.ID == "sanfo_decision_text_b" && !strings.Contains(el.Key, "Bob is knocking") {
			t.Errorf("expected body text with 'Bob is knocking', got %q", el.Key)
		}
	}

	// Verify callbacks in TempVars
	if fn, ok := w.TempVars["on_ok"].(func()); !ok || fn == nil {
		t.Errorf("missing on_ok callback in TempVars")
	} else {
		fn()
		if !okCalled {
			t.Errorf("on_ok callback did not fire")
		}
	}

	if fn, ok := w.TempVars["on_cancel"].(func()); !ok || fn == nil {
		t.Errorf("missing on_cancel callback in TempVars")
	} else {
		fn()
		if !cancelCalled {
			t.Errorf("on_cancel callback did not fire")
		}
	}

	// 3. Test Door Opens and Door Not Opens windows
	hud.OpenDoorOpensDialog("Alice's Studio")
	if hud.windowManager.GetWindow("nav_dooropens") == nil {
		t.Errorf("expected nav_dooropens window to be open")
	}

	hud.OpenDoorNotOpensDialog()
	if hud.windowManager.GetWindow("nav_dooropens") != nil {
		t.Errorf("expected nav_dooropens to be closed after OpenDoorNotOpensDialog")
	}
	if hud.windowManager.GetWindow("nav_doornotopens") == nil {
		t.Errorf("expected nav_doornotopens to be open")
	}
}

func TestModerationKnockAndKickFlow(t *testing.T) {
	tm := textmgr.New()
	hud := NewHUDManager(tm)
	hud.windowManager = ui.NewWindowManager(hud.fontManager)

	scene := &Scene{
		currentUserName: "Alice",
		hudManager:      hud,
	}
	scene.studio = &studios.Studio{
		StudioID: "studio_alice",
		Owner:    "Alice",
		Name:     "Alice's Studio",
	}

	// 1. Verify owner check
	if !scene.IsRoomOwner() {
		t.Fatalf("expected Alice to be owner of Alice's Studio")
	}

	doorbellResTarget := ""
	doorbellResAllow := false
	scene.SetDoorbellResponseSender(func(target string, allow bool) error {
		doorbellResTarget = target
		doorbellResAllow = allow
		return nil
	})

	// 2. HandleDoorbellKnock opens decision dialog
	scene.HandleDoorbellKnock("Bob")
	w := hud.windowManager.GetWindow("sanfo_decision_dialog")
	if w == nil {
		t.Fatalf("expected sanfo_decision_dialog to open on visitor knock")
	}

	// Simulate clicking "Let In"
	if fn, ok := w.TempVars["on_ok"].(func()); ok && fn != nil {
		fn()
	}
	if doorbellResTarget != "Bob" || !doorbellResAllow {
		t.Errorf("expected doorbell allow=true for Bob, got target=%q allow=%v", doorbellResTarget, doorbellResAllow)
	}

	// 3. Test MarkKicked
	if scene.ShouldExit() {
		t.Errorf("scene should not exit before being kicked")
	}
	scene.MarkKicked()
	if !scene.kicked {
		t.Errorf("expected scene.kicked to be true")
	}
	if !scene.ShouldExit() {
		t.Errorf("expected ShouldExit() to return true when kicked")
	}
}

func TestTradingWindowDefinitionAndCallbacks(t *testing.T) {
	// 1. Verify 43_sanfo_trading.window.txt parses cleanly
	defPath := filepath.Join("Lingo", "cc_room", "43_sanfo_trading.window.txt")
	def, err := window.ParseFile(defPath)
	if err != nil {
		t.Fatalf("failed to parse trading window definition: %v", err)
	}

	if def.Name != "cc_trading" {
		t.Errorf("expected window name cc_trading, got %s", def.Name)
	}
	if def.Rect != [4]int{208, 178, 548, 396} {
		t.Errorf("expected rect [208, 178, 548, 396], got %v", def.Rect)
	}

	// 2. Test TradeManager initialization and network callbacks
	tm := textmgr.New()
	hud := NewHUDManager(tm)
	hud.windowManager = ui.NewWindowManager(hud.fontManager)

	scene := &Scene{
		currentUserName: "Alice",
		hudManager:      hud,
		tm:              tm,
	}

	tradeMgr := NewTradeManager(scene)

	startCalledWith := ""
	addCalledWith := ""
	agreeCalled := false
	cancelCalled := false

	tradeMgr.SetCallbacks(
		func(target string) error {
			startCalledWith = target
			return nil
		},
		func(posID string) error {
			addCalledWith = posID
			return nil
		},
		func() error {
			agreeCalled = true
			return nil
		},
		func() error {
			cancelCalled = true
			return nil
		},
	)

	// Start trade with Bob
	tradeMgr.StartTrade("Bob")
	if !tradeMgr.IsOpen() {
		t.Fatalf("expected trade manager window to be open")
	}
	if startCalledWith != "Bob" {
		t.Errorf("expected onSendStart to be called with Bob, got %q", startCalledWith)
	}

	// Verify window elements
	w := hud.windowManager.GetWindow("cc_trading")
	if w == nil {
		t.Fatalf("expected cc_trading window in window manager")
	}

	// Verify update from network TD_DISPL packet
	tradeMgr.UpdateFromNetwork("Alice", "Bob", "10,20", "30", false, true)
	if !tradeMgr.partnerOffer.Agrees {
		t.Errorf("expected partner offer to agree based on TD_DISPL")
	}
	if len(tradeMgr.partnerOffer.Items) != 1 || tradeMgr.partnerOffer.Items[0].ProdID != "30" {
		t.Errorf("expected partner to have 1 item prodID 30, got %v", tradeMgr.partnerOffer.Items)
	}

	// Verify callbacks work when invoked
	if tradeMgr.onSendAdd != nil {
		_ = tradeMgr.onSendAdd("10")
		if addCalledWith != "10" {
			t.Errorf("expected onSendAdd '10', got %q", addCalledWith)
		}
	}
	if tradeMgr.onSendAgree != nil {
		_ = tradeMgr.onSendAgree()
		if !agreeCalled {
			t.Errorf("expected onSendAgree to be called")
		}
	}
	if tradeMgr.onSendCancel != nil {
		_ = tradeMgr.onSendCancel()
		if !cancelCalled {
			t.Errorf("expected onSendCancel to be called")
		}
	}
}
