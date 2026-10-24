package sumo

import (
	"testing"
)

func TestSumoRuleMatrixComplete(t *testing.T) {
	// Matrix of expected results for EvaluateRound(m1, m2)
	// Key: [m1][m2] -> RoundResult
	expected := map[Move]map[Move]RoundResult{
		MoveHeadHit: {
			MoveHeadHit:      ResultDraw,
			MoveBodyBlaster:  ResultPlayer1Wins,
			MoveOverheadToss: ResultPlayer2Wins,
			MoveFlyingKick:   ResultPlayer2Wins,
			MoveFlyingPunch:  ResultPlayer1Wins,
		},
		MoveBodyBlaster: {
			MoveHeadHit:      ResultPlayer2Wins,
			MoveBodyBlaster:  ResultDraw,
			MoveOverheadToss: ResultPlayer2Wins,
			MoveFlyingKick:   ResultPlayer1Wins,
			MoveFlyingPunch:  ResultPlayer1Wins,
		},
		MoveOverheadToss: {
			MoveHeadHit:      ResultPlayer1Wins,
			MoveBodyBlaster:  ResultPlayer1Wins,
			MoveOverheadToss: ResultDraw,
			MoveFlyingKick:   ResultPlayer2Wins,
			MoveFlyingPunch:  ResultPlayer2Wins,
		},
		MoveFlyingKick: {
			MoveHeadHit:      ResultPlayer1Wins,
			MoveBodyBlaster:  ResultPlayer2Wins,
			MoveOverheadToss: ResultPlayer1Wins,
			MoveFlyingKick:   ResultDraw,
			MoveFlyingPunch:  ResultPlayer2Wins,
		},
		MoveFlyingPunch: {
			MoveHeadHit:      ResultPlayer2Wins,
			MoveBodyBlaster:  ResultPlayer2Wins,
			MoveOverheadToss: ResultPlayer1Wins,
			MoveFlyingKick:   ResultPlayer1Wins,
			MoveFlyingPunch:  ResultDraw,
		},
	}

	for m1, row := range expected {
		for m2, exp := range row {
			got := EvaluateRound(m1, m2)
			if got != exp {
				t.Errorf("EvaluateRound(%s, %s) = %v; want %v", m1, m2, got, exp)
			}
		}
	}
}

func TestSumoBestOfThreeProgression(t *testing.T) {
	m := NewMatch("Alice", "Bob")
	if m.GameOver {
		t.Fatal("new match should not be game over")
	}

	// Round 1: Alice uses Head Hit (1), Bob uses Body Blaster (2) -> Alice wins
	res1, _, _ := m.SubmitMoves(MoveHeadHit, MoveBodyBlaster)
	if res1 != ResultPlayer1Wins {
		t.Fatalf("Round 1 expected P1 win, got %v", res1)
	}
	if m.Player1Score != 1 || m.Player2Score != 0 || m.GameOver {
		t.Fatalf("unexpected state after round 1: P1=%d, P2=%d, Over=%v", m.Player1Score, m.Player2Score, m.GameOver)
	}

	// Round 2: Draw: both choose Flying Kick (4)
	res2, _, _ := m.SubmitMoves(MoveFlyingKick, MoveFlyingKick)
	if res2 != ResultDraw {
		t.Fatalf("Round 2 expected Draw, got %v", res2)
	}
	if m.Player1Score != 1 || m.Player2Score != 0 || m.GameOver {
		t.Fatalf("unexpected state after draw: P1=%d, P2=%d, Over=%v", m.Player1Score, m.Player2Score, m.GameOver)
	}

	// Round 3: Bob uses Overhead Toss (3), Alice uses Head Hit (1) -> Bob wins
	res3, _, _ := m.SubmitMoves(MoveHeadHit, MoveOverheadToss)
	if res3 != ResultPlayer2Wins {
		t.Fatalf("Round 3 expected P2 win, got %v", res3)
	}
	if m.Player1Score != 1 || m.Player2Score != 1 || m.GameOver {
		t.Fatalf("unexpected state after round 3: P1=%d, P2=%d, Over=%v", m.Player1Score, m.Player2Score, m.GameOver)
	}

	// Round 4: Alice uses Flying Punch (5), Bob uses Overhead Toss (3) -> Alice wins 2nd point -> Match over
	res4, _, _ := m.SubmitMoves(MoveFlyingPunch, MoveOverheadToss)
	if res4 != ResultPlayer1Wins {
		t.Fatalf("Round 4 expected P1 win, got %v", res4)
	}
	if m.Player1Score != 2 || m.Player2Score != 1 || !m.GameOver {
		t.Fatalf("expected game over after 2 wins: P1=%d, P2=%d, Over=%v", m.Player1Score, m.Player2Score, m.GameOver)
	}
	if m.WinnerName != "Alice" || m.LoserName != "Bob" {
		t.Fatalf("unexpected winner/loser: winner=%s, loser=%s", m.WinnerName, m.LoserName)
	}
}

func TestSumoTurnTimerFallback(t *testing.T) {
	m := NewMatch("Alice", "Bob")
	// Alice submits 0 (no move before timer expires); Bob submits valid move
	_, p1Actual, p2Actual := m.SubmitMoves(MoveNone, MoveHeadHit)
	if p1Actual < 1 || p1Actual > 5 {
		t.Fatalf("expected P1 fallback to valid move 1..5, got %v", p1Actual)
	}
	if p2Actual != MoveHeadHit {
		t.Fatalf("expected P2 move to be Head Hit, got %v", p2Actual)
	}
}

func TestSumoStreakRewards(t *testing.T) {
	tier1 := GetRewardForStreak(1)
	if tier1 == nil || tier1.RewardDB != 25 {
		t.Errorf("streak 1 expected 25 dB, got %+v", tier1)
	}

	tier3 := GetRewardForStreak(3)
	if tier3 == nil || tier3.RewardFurni != 85 || tier3.RewardDB != 75 {
		t.Errorf("streak 3 expected Tatami Mat (85) and 75 dB, got %+v", tier3)
	}

	tier5 := GetRewardForStreak(5)
	if tier5 == nil || tier5.RewardFurni != 86 || tier5.RewardDB != 150 {
		t.Errorf("streak 5 expected Rice Paper Divider (86) and 150 dB, got %+v", tier5)
	}

	tier7 := GetRewardForStreak(7)
	if tier7 == nil || tier7.RewardFurni != 87 || tier7.RewardDB != 300 {
		t.Errorf("streak 7 expected Gong (87) and 300 dB, got %+v", tier7)
	}
}
