package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gocoke/internal/client/minigames/sumo"
)

func TestSumoMatchProgressionAndRewards(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	defer store.Close()

	srv := NewServer(".", "127.0.0.1", "8080", store)

	p1, _ := store.RegisterDenizen("SumoKing", "pass123", "p1@test.com", "1", "avatar_def")
	p2, _ := store.RegisterDenizen("Challenger", "pass123", "p2@test.com", "2", "avatar_def")

	// P1 and P2 join the queue
	player1 := &Player{ScreenName: p1.ScreenName, ID: p1.ID, Authenticated: true}
	player2 := &Player{ScreenName: p2.ScreenName, ID: p2.ID, Authenticated: true}

	_ = srv.sumoManager.JoinQueue(player1)
	sess := srv.sumoManager.JoinQueue(player2)
	if sess == nil {
		t.Fatal("expected match session to start when 2 players join queue")
	}

	// Round 1: P1 uses Head Hit (1), P2 uses Body Blaster (2) -> P1 wins
	_, _, _ = srv.sumoManager.SubmitMove(player1.ScreenName, sumo.MoveHeadHit)
	state, res1, err := srv.sumoManager.SubmitMove(player2.ScreenName, sumo.MoveBodyBlaster)
	if err != nil || res1 != sumo.ResultPlayer1Wins {
		t.Fatalf("round 1 expected P1 win, got %v (err: %v)", res1, err)
	}
	if state.Player1Score != 1 || state.GameOver {
		t.Fatalf("expected P1 score 1 and game not over, got score=%d, over=%v", state.Player1Score, state.GameOver)
	}

	// Round 2: P1 uses Flying Kick (4), P2 uses Overhead Toss (3) -> P1 wins (2-0) -> Match Over!
	_, _, _ = srv.sumoManager.SubmitMove(player1.ScreenName, sumo.MoveFlyingKick)
	finalState, res2, err := srv.sumoManager.SubmitMove(player2.ScreenName, sumo.MoveOverheadToss)
	if err != nil || res2 != sumo.ResultPlayer1Wins {
		t.Fatalf("round 2 expected P1 win, got %v (err: %v)", res2, err)
	}
	if !finalState.GameOver || finalState.WinnerName != player1.ScreenName {
		t.Fatalf("expected match over with winner %s, got over=%v, winner=%s",
			player1.ScreenName, finalState.GameOver, finalState.WinnerName)
	}

	// Verify streak in database
	var streak, totalWins int
	_ = store.db.QueryRow("SELECT streak, total_wins FROM vegosan_streaks WHERE denizen_id = ?", p1.ID).Scan(&streak, &totalWins)
	if streak != 1 || totalWins != 1 {
		t.Errorf("expected winner streak 1 and total_wins 1, got streak=%d, total=%d", streak, totalWins)
	}

	// Now simulate P1 reaching streak 3 (milestone reward: Tatami Mat, prodId 85)
	srv.sumoManager.awardSumoMatchVictory(player1.ScreenName, player2.ScreenName) // streak 2
	srv.sumoManager.awardSumoMatchVictory(player1.ScreenName, player2.ScreenName) // streak 3

	_ = store.db.QueryRow("SELECT streak FROM vegosan_streaks WHERE denizen_id = ?", p1.ID).Scan(&streak)
	if streak != 3 {
		t.Fatalf("expected streak 3, got %d", streak)
	}

	// Verify Tatami Mat (prodId 85) in P1's backpack
	items, err := store.GetBackpack(p1.ScreenName)
	if err != nil {
		t.Fatal(err)
	}
	hasTatami := false
	for _, it := range items {
		if it.CatalogItemID == 85 {
			hasTatami = true
			break
		}
	}
	if !hasTatami {
		t.Errorf("expected Tatami Mat (prodId 85) in backpack after streak 3, found items: %+v", items)
	}

	// Now simulate P2 losing -> verify P2 streak is 0
	var p2Streak int
	_ = store.db.QueryRow("SELECT streak FROM vegosan_streaks WHERE denizen_id = ?", p2.ID).Scan(&p2Streak)
	if p2Streak != 0 {
		t.Errorf("expected loser streak 0, got %d", p2Streak)
	}
}

func TestSumoHTTPPrizeCallback(t *testing.T) {
	store, _ := OpenStore(":memory:")
	defer store.Close()
	srv := NewServer(".", "127.0.0.1", "8080", store)

	user, _ := store.RegisterDenizen("SumoWinner", "pass123", "winner@test.com", "1", "avatar_def")

	// Call /sumo.do?screenname=SumoWinner&prize=5 (Prize 5 = Rice Paper Divider, prodId 86)
	req := httptest.NewRequest("GET", "/sumo.do?screenname=SumoWinner&prize=5", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	respXML := w.Body.String()
	if !strings.Contains(respXML, `code="0"`) || !strings.Contains(respXML, `prize="Rice Paper Divider"`) {
		t.Errorf("unexpected XML response: %s", respXML)
	}

	// Verify Rice Paper Divider in backpack
	items, _ := store.GetBackpack(user.ScreenName)
	hasRicePaper := false
	for _, it := range items {
		if it.CatalogItemID == 86 {
			hasRicePaper = true
			break
		}
	}
	if !hasRicePaper {
		t.Errorf("expected Rice Paper Divider (prodId 86) in backpack, got: %+v", items)
	}
}

func TestUncoverTheMusicRulesAndDailyCap(t *testing.T) {
	store, _ := OpenStore(":memory:")
	defer store.Close()
	srv := NewServer(".", "127.0.0.1", "8080", store)

	d, _ := store.RegisterDenizen("TileMaster", "pass123", "tile@test.com", "1", "avatar_def")

	// 1. Play normal pair match (+1 dB)
	body, _ := json.Marshal(map[string]interface{}{
		"player":    d.ScreenName,
		"tile1":     0,
		"tile2":     1,
		"game_over": false,
	})
	req := httptest.NewRequest("POST", "/api/minigames/uncover/play", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var res map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res["awarded_db"] != float64(1) {
		t.Errorf("expected 1 dB awarded for match, got %v", res["awarded_db"])
	}

	// 2. Play game over match (+61 dB)
	bodyOver, _ := json.Marshal(map[string]interface{}{
		"player":    d.ScreenName,
		"tile1":     2,
		"tile2":     3,
		"game_over": true,
	})
	reqOver := httptest.NewRequest("POST", "/api/minigames/uncover/play", bytes.NewReader(bodyOver))
	wOver := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wOver, reqOver)

	var resOver map[string]interface{}
	_ = json.Unmarshal(wOver.Body.Bytes(), &resOver)
	if resOver["awarded_db"] != float64(61) {
		t.Errorf("expected 61 dB (1 + 60 bonus), got %v", resOver["awarded_db"])
	}

	// 3. Test daily 300 dB cap enforcement
	_ = store.RecordDailyMinigameEarnings(d.ID, "uncover", 290) // Total now > 300
	reqCapped := httptest.NewRequest("POST", "/api/minigames/uncover/play", bytes.NewReader(body))
	wCapped := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wCapped, reqCapped)

	var resCapped map[string]interface{}
	_ = json.Unmarshal(wCapped.Body.Bytes(), &resCapped)
	if resCapped["status"] != "capped" {
		t.Errorf("expected status 'capped' when exceeding 300 dB daily limit, got: %+v", resCapped)
	}
}

func TestPopQuizTriviaEvaluation(t *testing.T) {
	store, _ := OpenStore(":memory:")
	defer store.Close()
	srv := NewServer(".", "127.0.0.1", "8080", store)

	d, _ := store.RegisterDenizen("QuizWiz", "pass123", "quiz@test.com", "1", "avatar_def")
	initialDB := d.Decibels

	// 1. Fetch Question 1
	reqQ := httptest.NewRequest("GET", "/api/minigames/quiz/question?id=1", nil)
	wQ := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wQ, reqQ)
	if wQ.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wQ.Code)
	}

	// 2. Submit wrong answer (Choice 0)
	bodyWrong, _ := json.Marshal(map[string]interface{}{
		"player": d.ScreenName,
		"id":     1,
		"choice": 0,
	})
	reqWrong := httptest.NewRequest("POST", "/api/minigames/quiz/answer", bytes.NewReader(bodyWrong))
	wWrong := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wWrong, reqWrong)

	var resWrong map[string]interface{}
	_ = json.Unmarshal(wWrong.Body.Bytes(), &resWrong)
	if resWrong["correct"] == true || resWrong["awarded_db"] != float64(0) {
		t.Errorf("expected wrong answer to award 0 dB, got %+v", resWrong)
	}

	// 3. Submit correct answer (Choice 1 = Discman)
	bodyRight, _ := json.Marshal(map[string]interface{}{
		"player": d.ScreenName,
		"id":     1,
		"choice": 1,
	})
	reqRight := httptest.NewRequest("POST", "/api/minigames/quiz/answer", bytes.NewReader(bodyRight))
	wRight := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wRight, reqRight)

	var resRight map[string]interface{}
	_ = json.Unmarshal(wRight.Body.Bytes(), &resRight)
	if resRight["correct"] != true || resRight["awarded_db"] != float64(15) {
		t.Errorf("expected correct answer to award 15 dB, got %+v", resRight)
	}

	// Verify player balance updated
	updated, _ := store.GetDenizenByName(d.ScreenName)
	if updated.Decibels != initialDB+15 {
		t.Errorf("expected balance %d, got %d", initialDB+15, updated.Decibels)
	}
}

func TestPublicRoomsCompleteInventory(t *testing.T) {
	store, _ := OpenStore(":memory:")
	defer store.Close()
	srv := NewServer(".", "127.0.0.1", "8080", store)

	// Verify all 44 $SYSTEM$ rooms can be retrieved
	for i := 1; i <= 44; i++ {
		roomID := fmt.Sprintf("$SYSTEM$%d", i)
		rm, err := srv.GetRoom(roomID)
		if err != nil || rm == nil {
			t.Errorf("failed to retrieve public room %s: %v", roomID, err)
			continue
		}
		if rm.Name == "" || rm.Layout < 1 {
			t.Errorf("room %s has invalid metadata: Name=%q, Layout=%d", roomID, rm.Name, rm.Layout)
		}
	}

	// Verify web aliases
	aliases := []string{"VIS$0", "VIS$1", "VIS$2"}
	for _, alias := range aliases {
		rm, err := srv.GetRoom(alias)
		if err != nil || rm == nil {
			t.Errorf("failed to retrieve room alias %s: %v", alias, err)
		}
	}

	// Unknown room ID must still be rejected
	_, errUnknown := srv.GetRoom("INVALID_ROOM_99999")
	if errUnknown == nil {
		t.Error("expected error for non-existent room ID, got nil")
	}
}
