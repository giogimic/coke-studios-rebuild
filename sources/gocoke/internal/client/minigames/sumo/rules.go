package sumo

import (
	"fmt"
	"math/rand"
	"time"
)

// Move represents one of the 5 authentic V-ego San Sumo moves.
type Move int

const (
	MoveNone         Move = 0
	MoveHeadHit      Move = 1 // Head Hit
	MoveBodyBlaster  Move = 2 // Body Blaster
	MoveOverheadToss Move = 3 // Overhead Toss
	MoveFlyingKick   Move = 4 // Flying Kick
	MoveFlyingPunch  Move = 5 // Flying Punch
)

// MoveNames maps move constants to historical names.
var MoveNames = map[Move]string{
	MoveNone:         "None",
	MoveHeadHit:      "Head Hit",
	MoveBodyBlaster:  "Body Blaster",
	MoveOverheadToss: "Overhead Toss",
	MoveFlyingKick:   "Flying Kick",
	MoveFlyingPunch:  "Flying Punch",
}

func (m Move) String() string {
	if name, ok := MoveNames[m]; ok {
		return name
	}
	return fmt.Sprintf("Move(%d)", int(m))
}

// RoundResult indicates the outcome of a single round.
type RoundResult int

const (
	ResultDraw        RoundResult = 0
	ResultPlayer1Wins RoundResult = 1
	ResultPlayer2Wins RoundResult = 2
)

// EvaluateRound evaluates two moves according to the evidenced Coke Studios V-ego San win/loss graph.
//
// Move interactions:
//   - Head Hit (1) beats Body Blaster (2) & Flying Punch (5)
//   - Body Blaster (2) beats Flying Kick (4) & Flying Punch (5)
//   - Overhead Toss (3) beats Head Hit (1) & Body Blaster (2)
//   - Flying Kick (4) beats Head Hit (1) & Overhead Toss (3)
//   - Flying Punch (5) beats Overhead Toss (3) & Flying Kick (4)
func EvaluateRound(m1, m2 Move) RoundResult {
	if m1 < 1 || m1 > 5 || m2 < 1 || m2 > 5 {
		if m1 >= 1 && m1 <= 5 {
			return ResultPlayer1Wins
		}
		if m2 >= 1 && m2 <= 5 {
			return ResultPlayer2Wins
		}
		return ResultDraw
	}
	if m1 == m2 {
		return ResultDraw
	}

	switch m1 {
	case MoveHeadHit:
		if m2 == MoveBodyBlaster || m2 == MoveFlyingPunch {
			return ResultPlayer1Wins
		}
	case MoveBodyBlaster:
		if m2 == MoveFlyingKick || m2 == MoveFlyingPunch {
			return ResultPlayer1Wins
		}
	case MoveOverheadToss:
		if m2 == MoveHeadHit || m2 == MoveBodyBlaster {
			return ResultPlayer1Wins
		}
	case MoveFlyingKick:
		if m2 == MoveHeadHit || m2 == MoveOverheadToss {
			return ResultPlayer1Wins
		}
	case MoveFlyingPunch:
		if m2 == MoveOverheadToss || m2 == MoveFlyingKick {
			return ResultPlayer1Wins
		}
	}
	return ResultPlayer2Wins
}

// RandomMove returns a valid random move (1..5) for the turn timer timeout fallback.
func RandomMove() Move {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return Move(r.Intn(5) + 1)
}

// MatchState models an authoritative best-of-three match between two V-egos.
type MatchState struct {
	Player1Name  string
	Player2Name  string
	Player1Score int // Rounds won (first to 2 wins match)
	Player2Score int
	CurrentRound int
	P1Move       Move
	P2Move       Move
	TurnTimerSec int
	GameOver     bool
	WinnerName   string
	LoserName    string
}

// NewMatch creates a new match instance.
func NewMatch(p1, p2 string) *MatchState {
	return &MatchState{
		Player1Name:  p1,
		Player2Name:  p2,
		CurrentRound: 1,
		TurnTimerSec: 10, // 10 second authentic turn timer
	}
}

// SubmitMoves resolves the round with the two players' chosen moves.
// If a player did not submit a move (MoveNone), RandomMove() is applied as fallback.
func (m *MatchState) SubmitMoves(p1Move, p2Move Move) (res RoundResult, actualP1, actualP2 Move) {
	if m.GameOver {
		return ResultDraw, MoveNone, MoveNone
	}

	if p1Move < 1 || p1Move > 5 {
		p1Move = RandomMove()
	}
	if p2Move < 1 || p2Move > 5 {
		p2Move = RandomMove()
	}

	m.P1Move = p1Move
	m.P2Move = p2Move

	res = EvaluateRound(p1Move, p2Move)
	switch res {
	case ResultPlayer1Wins:
		m.Player1Score++
	case ResultPlayer2Wins:
		m.Player2Score++
	}

	// Best of three format: first to 2 points wins
	if m.Player1Score >= 2 {
		m.GameOver = true
		m.WinnerName = m.Player1Name
		m.LoserName = m.Player2Name
	} else if m.Player2Score >= 2 {
		m.GameOver = true
		m.WinnerName = m.Player2Name
		m.LoserName = m.Player1Name
	} else {
		m.CurrentRound++
	}

	return res, p1Move, p2Move
}

// RewardTier represents the authentic reward for consecutive Sumo wins.
type RewardTier struct {
	StreakMin    int
	RewardDB     int
	RewardFurni  int // Catalog prodId (85 = Tatami Mat, 86 = Rice Paper Divider, 87 = Gong)
	RewardName   string
}

// Evidenced streak rewards
var StreakRewards = []RewardTier{
	{StreakMin: 7, RewardDB: 300, RewardFurni: 87, RewardName: "Gong"},
	{StreakMin: 5, RewardDB: 150, RewardFurni: 86, RewardName: "Rice Paper Divider"},
	{StreakMin: 3, RewardDB: 75, RewardFurni: 85, RewardName: "Tatami Mat"},
	{StreakMin: 2, RewardDB: 50, RewardFurni: 0, RewardName: "Bonus Decibels"},
	{StreakMin: 1, RewardDB: 25, RewardFurni: 0, RewardName: "Victory Decibels"},
}

// GetRewardForStreak returns the matching reward tier for a given win streak.
func GetRewardForStreak(streak int) *RewardTier {
	for i := range StreakRewards {
		if streak >= StreakRewards[i].StreakMin {
			return &StreakRewards[i]
		}
	}
	return nil
}
