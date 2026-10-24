package uncover

import (
	"fmt"
	"math/rand"
	"time"
)

// CardType represents tile categories in the 24-tile grid.
type CardType int

const (
	CardPair CardType = iota // Standard instrument/beat matching pair (11 pairs, IDs 1..11)
	CardWild                 // 1 Wild Card (matches any other card)
	CardPass                 // 1 Pass Card (passes turn)
)

type Tile struct {
	Index   int      // 0..23
	Type    CardType
	PairID  int      // 1..11 for matching pairs, 0 for Wild/Pass
	Matched bool     // Removed from grid
}

// UncoverGame models the authoritative Uncover the Music match state.
type UncoverGame struct {
	Tiles          [24]Tile
	Players        []string
	CurrentTurnIdx int
	Scores         map[string]int // Number of pairs matched per player
	TurnTimerSec   int            // 15 seconds
	GameOver       bool
	PairsRemaining int
}

// NewGame initializes a 24-tile grid with 11 pairs, 1 Wild, and 1 Pass card.
func NewGame(players []string) (*UncoverGame, error) {
	if len(players) < 3 || len(players) > 5 {
		return nil, fmt.Errorf("uncover the music requires 3 to 5 players, got %d", len(players))
	}

	game := &UncoverGame{
		Players:        players,
		Scores:         make(map[string]int),
		TurnTimerSec:   15,
		PairsRemaining: 11,
	}
	for _, p := range players {
		game.Scores[p] = 0
	}

	// Build 24 cards: 11 pairs (22 cards), 1 wild, 1 pass
	var cards []Tile
	for pairID := 1; pairID <= 11; pairID++ {
		cards = append(cards, Tile{Type: CardPair, PairID: pairID})
		cards = append(cards, Tile{Type: CardPair, PairID: pairID})
	}
	cards = append(cards, Tile{Type: CardWild, PairID: 0})
	cards = append(cards, Tile{Type: CardPass, PairID: 0})

	// Shuffle deterministically with seed or time
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(cards), func(i, j int) {
		cards[i], cards[j] = cards[j], cards[i]
	})

	for i := 0; i < 24; i++ {
		cards[i].Index = i
		game.Tiles[i] = cards[i]
	}

	return game, nil
}

// TurnResult conveys the outcome of clicking up to two tiles.
type TurnResult struct {
	Player        string
	Tile1Idx      int
	Tile2Idx      int
	Matched       bool
	Passed        bool
	DecibelsWon   int
	BonusDecibels map[string]int // Awarded at end of game
	GameOver      bool
}

// PlayTurn evaluates a player's move on 1 or 2 tile indices.
func (g *UncoverGame) PlayTurn(player string, tile1Idx, tile2Idx int) (*TurnResult, error) {
	if g.GameOver {
		return nil, fmt.Errorf("game is already completed")
	}
	if g.Players[g.CurrentTurnIdx] != player {
		return nil, fmt.Errorf("not %s's turn (current turn: %s)", player, g.Players[g.CurrentTurnIdx])
	}
	if tile1Idx < 0 || tile1Idx >= 24 || g.Tiles[tile1Idx].Matched {
		return nil, fmt.Errorf("tile 1 invalid or already matched")
	}

	res := &TurnResult{
		Player:   player,
		Tile1Idx: tile1Idx,
		Tile2Idx: tile2Idx,
	}

	t1 := &g.Tiles[tile1Idx]

	// Handle Pass Card clicked first
	if t1.Type == CardPass {
		t1.Matched = true
		res.Passed = true
		g.advanceTurn()
		return res, nil
	}

	// Validate second tile
	if tile2Idx < 0 || tile2Idx >= 24 || tile1Idx == tile2Idx || g.Tiles[tile2Idx].Matched {
		return nil, fmt.Errorf("tile 2 invalid or duplicate")
	}

	t2 := &g.Tiles[tile2Idx]

	// Handle Pass Card clicked second
	if t2.Type == CardPass {
		t2.Matched = true
		res.Passed = true
		g.advanceTurn()
		return res, nil
	}

	// Check match:
	// Matches if either is Wild or both share the same PairID
	matched := false
	if t1.Type == CardWild || t2.Type == CardWild {
		matched = true
	} else if t1.Type == CardPair && t2.Type == CardPair && t1.PairID == t2.PairID {
		matched = true
	}

	if matched {
		t1.Matched = true
		t2.Matched = true
		g.Scores[player]++
		g.PairsRemaining--
		res.Matched = true
		res.DecibelsWon = 1 // 1 dB per matched pair

		if g.PairsRemaining <= 0 {
			g.GameOver = true
			res.GameOver = true
			res.BonusDecibels = g.calculateBonusRewards()
		}
		// If matched and game not over, player keeps turn
		return res, nil
	}

	// No match -> turn passes to next player counterclockwise
	g.advanceTurn()
	return res, nil
}

func (g *UncoverGame) advanceTurn() {
	g.CurrentTurnIdx = (g.CurrentTurnIdx + 1) % len(g.Players)
}

func (g *UncoverGame) calculateBonusRewards() map[string]int {
	bonusMap := make(map[string]int)
	maxPairs := -1
	for _, p := range g.Players {
		if g.Scores[p] > maxPairs {
			maxPairs = g.Scores[p]
		}
	}
	var winners []string
	for _, p := range g.Players {
		if g.Scores[p] == maxPairs {
			winners = append(winners, p)
		}
	}
	if len(winners) > 0 {
		splitBonus := 60 / len(winners)
		for _, w := range winners {
			bonusMap[w] = splitBonus
		}
	}
	return bonusMap
}
