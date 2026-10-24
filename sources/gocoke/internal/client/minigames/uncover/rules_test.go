package uncover

import (
	"testing"
)

func TestUncoverGamePlayerBounds(t *testing.T) {
	_, err2 := NewGame([]string{"A", "B"})
	if err2 == nil {
		t.Error("expected error for 2 players; minimum is 3")
	}

	_, err6 := NewGame([]string{"A", "B", "C", "D", "E", "F"})
	if err6 == nil {
		t.Error("expected error for 6 players; maximum is 5")
	}

	game, err := NewGame([]string{"A", "B", "C"})
	if err != nil {
		t.Fatalf("unexpected error for 3 players: %v", err)
	}
	if len(game.Tiles) != 24 {
		t.Fatalf("expected 24 tiles, got %d", len(game.Tiles))
	}
}

func TestUncoverGamePairMatchingAndBonus(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie"}
	game, err := NewGame(players)
	if err != nil {
		t.Fatal(err)
	}

	// Manually arrange tiles for deterministic test
	// Put pair 1 at index 0 and 1
	game.Tiles[0] = Tile{Index: 0, Type: CardPair, PairID: 1}
	game.Tiles[1] = Tile{Index: 1, Type: CardPair, PairID: 1}
	// Put pair 2 at index 2 and 3
	game.Tiles[2] = Tile{Index: 2, Type: CardPair, PairID: 2}
	game.Tiles[3] = Tile{Index: 3, Type: CardPair, PairID: 2}

	// Alice's turn: matches pair 1 (0 and 1)
	res1, err := game.PlayTurn("Alice", 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res1.Matched || res1.DecibelsWon != 1 {
		t.Fatalf("expected match with 1 dB won, got matched=%v, dB=%d", res1.Matched, res1.DecibelsWon)
	}
	// Alice matched so she retains turn!
	if game.Players[game.CurrentTurnIdx] != "Alice" {
		t.Fatalf("expected Alice to retain turn after match, got %s", game.Players[game.CurrentTurnIdx])
	}

	// Alice tries mismatched tiles (2 and something else that doesn't match)
	game.Tiles[4] = Tile{Index: 4, Type: CardPair, PairID: 3}
	res2, err := game.PlayTurn("Alice", 2, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res2.Matched {
		t.Fatal("expected mismatch")
	}
	// Turn should pass to Bob
	if game.Players[game.CurrentTurnIdx] != "Bob" {
		t.Fatalf("expected turn to advance to Bob, got %s", game.Players[game.CurrentTurnIdx])
	}
}

func TestUncoverWildCardMatching(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie"}
	game, _ := NewGame(players)

	game.Tiles[0] = Tile{Index: 0, Type: CardWild, PairID: 0}
	game.Tiles[1] = Tile{Index: 1, Type: CardPair, PairID: 5}

	res, err := game.PlayTurn("Alice", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched {
		t.Fatal("wild card should match any pair")
	}
	if game.Scores["Alice"] != 1 {
		t.Fatalf("expected Alice score 1, got %d", game.Scores["Alice"])
	}
}
