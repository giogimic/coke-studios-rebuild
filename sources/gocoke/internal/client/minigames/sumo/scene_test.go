package sumo

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestSumoSceneLifecycle(t *testing.T) {
	exitCalled := false
	sc, err := NewScene("TestP1", "Master_San", "", 0, func() {
		exitCalled = true
	})
	if err != nil {
		t.Fatalf("failed to create sumo scene: %v", err)
	}

	w, h := sc.Layout(0, 0)
	if w != 760 || h != 520 {
		t.Fatalf("expected layout 760x520, got %dx%d", w, h)
	}

	if len(sc.buttons) != 5 {
		t.Fatalf("expected 5 move buttons, got %d", len(sc.buttons))
	}

	// Update step in StateChoosing
	if err := sc.Update(); err != nil {
		t.Fatalf("update error: %v", err)
	}

	// Draw on offscreen image in choosing state
	screen := ebiten.NewImage(760, 520)
	sc.Draw(screen)

	// Execute a round
	sc.selectedMove = MoveHeadHit
	sc.executeRound()
	if sc.state != StateResolving {
		t.Fatalf("expected state Resolving, got %v", sc.state)
	}
	if sc.lastP1Move != MoveHeadHit {
		t.Fatalf("expected lastP1Move to be MoveHeadHit, got %v", sc.lastP1Move)
	}

	// Draw in resolving state
	sc.Draw(screen)

	// Finish round resolution
	sc.finishRoundResolution()

	// Test force win
	sc.match.Player1Score = 2
	sc.match.GameOver = true
	sc.match.WinnerName = "TestP1"
	sc.finishRoundResolution()

	if sc.state != StateGameOver {
		t.Fatalf("expected StateGameOver, got %v", sc.state)
	}
	if sc.streak != 1 {
		t.Fatalf("expected streak to be 1, got %d", sc.streak)
	}

	// Draw in game over state
	sc.Draw(screen)

	// Reset match
	sc.resetForNewMatch()
	if sc.state != StateChoosing {
		t.Fatalf("expected StateChoosing after reset, got %v", sc.state)
	}
	if sc.match.Player1Score != 0 || sc.match.Player2Score != 0 {
		t.Fatal("expected scores reset to 0")
	}

	// Test exit callback invocation
	sc.onExit()
	if !exitCalled {
		t.Fatal("expected exit callback to be invoked")
	}
}

