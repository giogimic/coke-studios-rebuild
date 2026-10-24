package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gocoke/internal/client/minigames/sumo"
)

// SumoMatchSession manages an active 1v1 Sumo match between two players in the dojo.
type SumoMatchSession struct {
	ID        string
	Match     *sumo.MatchState
	P1        *Player
	P2        *Player
	P1Move    sumo.Move
	P2Move    sumo.Move
	P1Ready   bool
	P2Ready   bool
	TimerEnd  time.Time
	mu        sync.Mutex
}

// SumoManager manages all active Sumo games, queues, and authoritative awards.
type SumoManager struct {
	sessions map[string]*SumoMatchSession
	queue    []*Player
	mu       sync.Mutex
	server   *Server
}

func NewSumoManager(s *Server) *SumoManager {
	return &SumoManager{
		sessions: make(map[string]*SumoMatchSession),
		queue:    make([]*Player, 0),
		server:   s,
	}
}

// JoinQueue adds a player to the Sumo queue and starts a match if 2 players are waiting.
func (sm *SumoManager) JoinQueue(p *Player) *SumoMatchSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Avoid duplicates
	for _, qp := range sm.queue {
		if qp.ScreenName == p.ScreenName {
			return nil
		}
	}
	sm.queue = append(sm.queue, p)

	if len(sm.queue) >= 2 {
		p1 := sm.queue[0]
		p2 := sm.queue[1]
		sm.queue = sm.queue[2:]

		sessID := fmt.Sprintf("sumo_%s_%s_%d", p1.ScreenName, p2.ScreenName, time.Now().UnixNano())
		sess := &SumoMatchSession{
			ID:       sessID,
			Match:    sumo.NewMatch(p1.ScreenName, p2.ScreenName),
			P1:       p1,
			P2:       p2,
			TimerEnd: time.Now().Add(10 * time.Second),
		}
		sm.sessions[sessID] = sess
		sm.sessions[p1.ScreenName] = sess
		sm.sessions[p2.ScreenName] = sess

		log.Printf("[SUMO] Match started between %s (P1) and %s (P2) (Session: %s)", p1.ScreenName, p2.ScreenName, sessID)
		return sess
	}
	return nil
}

// SubmitMove processes a player's move input for the current round.
func (sm *SumoManager) SubmitMove(screenName string, move sumo.Move) (*sumo.MatchState, sumo.RoundResult, error) {
	sm.mu.Lock()
	sess, ok := sm.sessions[screenName]
	sm.mu.Unlock()

	if !ok || sess == nil {
		return nil, sumo.ResultDraw, fmt.Errorf("no active sumo match found for %s", screenName)
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.Match.GameOver {
		return sess.Match, sumo.ResultDraw, fmt.Errorf("match already ended")
	}

	if sess.P1 != nil && sess.P1.ScreenName == screenName {
		sess.P1Move = move
		sess.P1Ready = true
	} else if sess.P2 != nil && sess.P2.ScreenName == screenName {
		sess.P2Move = move
		sess.P2Ready = true
	} else {
		return nil, sumo.ResultDraw, fmt.Errorf("player %s not in this match", screenName)
	}

	// If both players submitted moves or timer expired, resolve round
	if sess.P1Ready && sess.P2Ready {
		res, actualP1, actualP2 := sess.Match.SubmitMoves(sess.P1Move, sess.P2Move)
		sess.P1Ready = false
		sess.P2Ready = false
		sess.P1Move = sumo.MoveNone
		sess.P2Move = sumo.MoveNone

		log.Printf("[SUMO] Round %d resolved: P1(%s)=%s vs P2(%s)=%s => Result: %v (Score: %d - %d)",
			sess.Match.CurrentRound, sess.Match.Player1Name, actualP1,
			sess.Match.Player2Name, actualP2, res,
			sess.Match.Player1Score, sess.Match.Player2Score)

		if sess.Match.GameOver {
			sm.awardSumoMatchVictory(sess.Match.WinnerName, sess.Match.LoserName)
			sm.cleanupSession(sess)
		}

		return sess.Match, res, nil
	}

	return sess.Match, sumo.ResultDraw, nil
}

func (sm *SumoManager) cleanupSession(sess *SumoMatchSession) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.sessions, sess.ID)
	if sess.P1 != nil {
		delete(sm.sessions, sess.P1.ScreenName)
	}
	if sess.P2 != nil {
		delete(sm.sessions, sess.P2.ScreenName)
	}
}

// awardSumoMatchVictory authoritatively updates winner streaks and grants milestone rewards.
func (sm *SumoManager) awardSumoMatchVictory(winnerName, loserName string) {
	if sm.server == nil || sm.server.store == nil {
		return
	}

	winner, _ := sm.server.store.GetDenizenByName(winnerName)
	loser, _ := sm.server.store.GetDenizenByName(loserName)

	if loser != nil {
		// Reset loser's streak to 0
		_ = sm.server.store.ResetDenizenSumoStreak(loser.ID)
	}

	if winner != nil {
		streak, totalWins, _ := sm.server.store.IncrementDenizenSumoStreak(winner.ID)
		tier := sumo.GetRewardForStreak(streak)
		if tier != nil {
			if tier.RewardDB > 0 {
				_ = sm.server.store.AddDenizenDecibels(winner.ID, tier.RewardDB)
				log.Printf("[SUMO] Winner %s awarded %d dB for streak %d", winnerName, tier.RewardDB, streak)
			}
			if tier.RewardFurni > 0 {
				props := map[string]any{"sumo_prize": true, "streak": streak, "name": tier.RewardName}
				_, _ = sm.server.store.AddBackpackItem(winnerName, tier.RewardFurni, props)
				log.Printf("[SUMO] Winner %s awarded furni %s (prodId %d) for streak %d", winnerName, tier.RewardName, tier.RewardFurni, streak)
			}
		}
		_ = totalWins
	}
}

// --- HTTP Handlers for Minigames ---

// handleGamesUserValidate handles Director Shockwave login validation: /gamesUser.do?scAction=validate&game=Sumo
func (s *Server) handleGamesUserValidate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/xml")
	sn := r.URL.Query().Get("screenName")
	if sn == "" {
		sn = r.URL.Query().Get("user")
	}

	var d *DenizenRecord
	var err error
	if sn != "" {
		d, err = s.store.GetDenizenByName(sn)
	}

	if err != nil || d == nil {
		fmt.Fprintf(w, `<Result code="-1"><error message="User not found"/></Result>`)
		return
	}

	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}

	// State "C" = Connected / Authorized
	fmt.Fprintf(w, `<Result code="0"><login screenName="%s" avatar="%s" state="C" server="%s"/></Result>`,
		d.ScreenName, d.AvatarDef, host)
}

// handleSumoPrizeHTTP handles Director / Flash HTTP callback: /sumo.do?screenname=...&prize=...
func (s *Server) handleSumoPrizeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/xml")
	sn := strings.TrimSpace(r.URL.Query().Get("screenname"))
	prizeLevelStr := strings.TrimSpace(r.URL.Query().Get("prize"))
	prizeLevel, _ := strconv.Atoi(prizeLevelStr)

	d, err := s.store.GetDenizenByName(sn)
	if err != nil || d == nil {
		fmt.Fprintf(w, `<Result code="-1"><error message="User not found"/></Result>`)
		return
	}

	rewardDB := 25
	prodID := 0
	prodName := ""
	switch prizeLevel {
	case 1:
		rewardDB = 25
	case 2:
		rewardDB = 50
	case 3:
		rewardDB = 75
		prodID = 85 // Tatami Mat
		prodName = "Tatami Mat"
	case 4:
		rewardDB = 100
	case 5:
		rewardDB = 150
		prodID = 86 // Rice Paper Divider
		prodName = "Rice Paper Divider"
	case 6:
		rewardDB = 200
	case 7:
		rewardDB = 300
		prodID = 87 // Gong
		prodName = "Gong"
	}

	_ = s.store.AddDenizenDecibels(d.ID, rewardDB)
	if prodID > 0 {
		props := map[string]any{"sumo_prize": true, "prize_level": prizeLevel, "name": prodName}
		_, _ = s.store.AddBackpackItem(sn, prodID, props)
	}

	updated, _ := s.store.GetDenizenByName(sn)
	decibels := 0
	if updated != nil {
		decibels = updated.Decibels
	}

	fmt.Fprintf(w, `<Result code="0" decibels="%d" prize="%s"/>`, decibels, prodName)
}

// handleSumoMatch handles POST /api/minigames/sumo/match
func (s *Server) handleSumoMatch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Player1 string `json:"player1"`
		Player2 string `json:"player2"`
		P1Move  int    `json:"p1_move"`
		P2Move  int    `json:"p2_move"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m := sumo.NewMatch(req.Player1, req.Player2)
	res, p1Act, p2Act := m.SubmitMoves(sumo.Move(req.P1Move), sumo.Move(req.P2Move))

	// If match is over, grant authoritative victory rewards
	if m.GameOver && s.sumoManager != nil {
		s.sumoManager.awardSumoMatchVictory(m.WinnerName, m.LoserName)
	}

	resp := map[string]interface{}{
		"result":        int(res),
		"p1_actual":     int(p1Act),
		"p2_actual":     int(p2Act),
		"p1_score":      m.Player1Score,
		"p2_score":      m.Player2Score,
		"round":         m.CurrentRound,
		"game_over":     m.GameOver,
		"winner":        m.WinnerName,
		"loser":         m.LoserName,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handleUncoverPlay handles POST /api/minigames/uncover/play
func (s *Server) handleUncoverPlay(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Player   string `json:"player"`
		Tile1    int    `json:"tile1"`
		Tile2    int    `json:"tile2"`
		GameOver bool   `json:"game_over"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	d, err := s.store.GetDenizenByName(req.Player)
	if err != nil || d == nil {
		http.Error(w, "Denizen not found", http.StatusNotFound)
		return
	}

	// Enforce 300 dB daily cap for Uncover the Music as documented in RevealChallengeRules.htm
	dailyEarned, _ := s.store.GetDailyMinigameEarnings(d.ID, "uncover")
	if dailyEarned >= 300 {
		resp := map[string]interface{}{
			"status":       "capped",
			"message":      "Daily Uncover the Music maximum (300 dB) reached.",
			"daily_earned": dailyEarned,
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	award := 1
	if req.GameOver {
		award += 60 // 60 dB victory bonus
	}
	if dailyEarned+award > 300 {
		award = 300 - dailyEarned
	}

	if award > 0 {
		_ = s.store.AddDenizenDecibels(d.ID, award)
		_ = s.store.RecordDailyMinigameEarnings(d.ID, "uncover", award)
	}

	resp := map[string]interface{}{
		"status":       "success",
		"awarded_db":   award,
		"daily_earned": dailyEarned + award,
		"balance":      d.Decibels + award,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// PopQuizQuestion represents an authentic music / pop culture trivia item.
type PopQuizQuestion struct {
	ID            int      `json:"id"`
	Question      string   `json:"question"`
	Options       []string `json:"options"`
	CorrectIndex  int      `json:"correct_index"`
	RewardDB      int      `json:"reward_db"`
}

var HistoricalPopQuizQuestions = []PopQuizQuestion{
	{
		ID:           1,
		Question:     "Which format introduced the portable CD player in 1984?",
		Options:      []string{"Walkman", "Discman", "Soundwave", "PodOne"},
		CorrectIndex: 1, // Discman
		RewardDB:     15,
	},
	{
		ID:           2,
		Question:     "How many beats per minute (BPM) does standard Coke Studios Rock beats use?",
		Options:      []string{"80 BPM", "100 BPM", "120 BPM", "140 BPM"},
		CorrectIndex: 2, // 120 BPM
		RewardDB:     15,
	},
	{
		ID:           3,
		Question:     "Which public studio features the outdoor Central Park skyline in Coke Studios?",
		Options:      []string{"London Studio", "Central Park Studio", "San Francisco Studio", "Goa Studio"},
		CorrectIndex: 1, // Central Park
		RewardDB:     20,
	},
	{
		ID:           4,
		Question:     "In V-ego San Sumo, which move counters both Overhead Toss and Flying Kick?",
		Options:      []string{"Head Hit", "Body Blaster", "Flying Punch", "None"},
		CorrectIndex: 2, // Flying Punch
		RewardDB:     25,
	},
}

// handlePopQuizQuestion handles GET /api/minigames/quiz/question
func (s *Server) handlePopQuizQuestion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.Atoi(idStr)
	if id < 1 || id > len(HistoricalPopQuizQuestions) {
		id = 1
	}

	q := HistoricalPopQuizQuestions[id-1]
	// Omit CorrectIndex in client response
	resp := map[string]interface{}{
		"id":        q.ID,
		"question":  q.Question,
		"options":   q.Options,
		"reward_db": q.RewardDB,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handlePopQuizAnswer handles POST /api/minigames/quiz/answer
func (s *Server) handlePopQuizAnswer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Player string `json:"player"`
		ID     int    `json:"id"`
		Choice int    `json:"choice"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.ID < 1 || req.ID > len(HistoricalPopQuizQuestions) {
		http.Error(w, "Invalid question ID", http.StatusBadRequest)
		return
	}

	q := HistoricalPopQuizQuestions[req.ID-1]
	correct := (req.Choice == q.CorrectIndex)

	awarded := 0
	if correct {
		awarded = q.RewardDB
		if d, _ := s.store.GetDenizenByName(req.Player); d != nil {
			_ = s.store.AddDenizenDecibels(d.ID, awarded)
		}
	}

	resp := map[string]interface{}{
		"correct":    correct,
		"awarded_db": awarded,
		"message":    fmt.Sprintf("Answer evaluated: correct=%v", correct),
	}
	_ = json.NewEncoder(w).Encode(resp)
}
