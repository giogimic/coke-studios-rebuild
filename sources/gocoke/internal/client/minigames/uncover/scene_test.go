package uncover

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestUncoverSceneLifecycle(t *testing.T) {
	sc, err := NewScene("LocalPlayer", []string{"P2", "P3"}, "", func() {})
	if err != nil {
		t.Fatalf("failed to create uncover scene: %v", err)
	}

	w, h := sc.Layout(0, 0)
	if w != 760 || h != 520 {
		t.Fatalf("expected layout 760x520, got %dx%d", w, h)
	}

	if len(sc.visualTiles) != 24 {
		t.Fatalf("expected 24 visual tiles, got %d", len(sc.visualTiles))
	}

	// Update step
	if err := sc.Update(); err != nil {
		t.Fatalf("update error: %v", err)
	}

	// Draw on offscreen image
	screen := ebiten.NewImage(760, 520)
	sc.Draw(screen)

	// Simulate tile click
	sc.handleTileClick(0)
	if !sc.visualTiles[0].IsRevealed {
		t.Fatal("expected tile 0 to be revealed")
	}

	// Turn timeout advance test
	sc.advanceTurnOnTimeout()
	if sc.firstTileIdx != -1 {
		t.Fatal("expected firstTileIdx reset after timeout")
	}
}

