package main

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCompositionRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "composition_test.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	d, err := store.GetOrCreateDenizen("Composer1", "1", "avatar_c", "Beats Maker")
	if err != nil {
		t.Fatalf("failed to create composer: %v", err)
	}

	testTitle := "Midnight Groove"
	testEDL := "^50,rockOne_guitar00{2,0{4,4{^50,danceOne_beatz00{4,0{^"

	songID, err := store.SaveSong(d.ScreenName, testTitle, testEDL)
	if err != nil {
		t.Fatalf("SaveSong failed: %v", err)
	}
	if songID <= 0 {
		t.Fatalf("expected positive song ID, got %d", songID)
	}

	rec, err := store.GetSong(songID)
	if err != nil {
		t.Fatalf("GetSong failed: %v", err)
	}

	if rec.ID != songID {
		t.Errorf("expected ID %d, got %d", songID, rec.ID)
	}
	if rec.Title != testTitle {
		t.Errorf("expected title %q, got %q", testTitle, rec.Title)
	}
	if rec.Artist != d.ScreenName {
		t.Errorf("expected artist %q, got %q", d.ScreenName, rec.Artist)
	}
	if rec.EDLString != testEDL {
		t.Errorf("expected EDL %q, got %q", testEDL, rec.EDLString)
	}

	list, err := store.GetSongsByArtist(d.ScreenName)
	if err != nil {
		t.Fatalf("GetSongsByArtist failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 song in list, got %d", len(list))
	}
}

func TestPerformanceDurationFromSequence(t *testing.T) {
	// EDL 1: 1 loop of 2000ms starting at 0 -> 2000ms (clamped to min 4s)
	edlShort := "^50,rockOne_guitar00{1,0{^"
	durShort := ParseEDLDuration(edlShort)
	if durShort < 4*time.Second || durShort > 5*time.Second {
		t.Errorf("expected short EDL to clamp to ~4s, got %v", durShort)
	}

	// EDL 2: Event starting at 12000 ms with 4 loops of 2000 ms -> 12000 + 8000 = 20000 ms (20 seconds)
	edl20s := "^50,danceOne_beatz00{4,12000{^"
	dur20s := ParseEDLDuration(edl20s)
	if dur20s != 20*time.Second {
		t.Errorf("expected duration 20s, got %v (not calculated from sequence!)", dur20s)
	}

	// EDL 3: Multi-track sequence:
	// Track 1 ends at 10,000 + (2*2000) = 14,000 ms
	// Track 2 ends at 30,000 + (4*2000) = 38,000 ms (38 seconds)
	edlMulti := "^50,guitar{2,10000{^50,synth{4,30000{^"
	durMulti := ParseEDLDuration(edlMulti)
	if durMulti != 38*time.Second {
		t.Errorf("expected duration 38s, got %v", durMulti)
	}

	// Verify it does NOT return a hardcoded 60s
	if dur20s == 60*time.Second || durMulti == 60*time.Second {
		t.Errorf("duration was hardcoded to 60s instead of parsed from EDL")
	}
}

func TestLateJoinPlaybackPosition(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)
	rm := srv.GetOrCreateRoom("VIS$0")

	djName := "DJLate"
	pDJ := &Player{ID: "p_dj", ScreenName: djName, RoomID: rm.ID}
	rm.Players[pDJ.ID] = pDJ

	edl := "^50,danceOne_beatz00{10,0{^" // 20s track
	_, err := srv.StartPerformance(rm, djName, djName, "Late Join Track", edl)
	if err != nil {
		t.Fatalf("StartPerformance failed: %v", err)
	}

	// Wait 50ms to simulate mid-performance playback
	time.Sleep(50 * time.Millisecond)

	elapsed, active := srv.GetLateJoinPosition(rm)
	if !active {
		t.Fatalf("expected performance to be active")
	}
	if elapsed < 40*time.Millisecond || elapsed > 5*time.Second {
		t.Fatalf("expected elapsed playback offset ~50ms, got %v", elapsed)
	}

	// Spectator late joins room
	spectator := &Player{ID: "p_spec", ScreenName: "LateSpectator", Authenticated: true}
	srv.handlePlayerMessage(spectator, `<Room.Enter roomID="VIS$0"/>`)
	if spectator.RoomID != rm.ID {
		t.Fatalf("spectator failed to join room: %s", spectator.RoomID)
	}
}

func TestPerformanceVoteEligibility(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)
	rm := srv.GetOrCreateRoom("VIS$0")

	dj := "MainDJ"
	pDJ := &Player{ID: "p_dj", ScreenName: dj, RoomID: rm.ID}
	rm.Players[pDJ.ID] = pDJ

	fan := "VoterFan"
	pFan := &Player{ID: "p_fan", ScreenName: fan, RoomID: rm.ID}
	rm.Players[pFan.ID] = pFan

	edl := "^50,synth{8,0{^" // 16s track
	_, err := srv.StartPerformance(rm, dj, dj, "Eligible Track", edl)
	if err != nil {
		t.Fatalf("StartPerformance failed: %v", err)
	}

	// 1. DJ cannot vote on own track
	err = srv.RecordPerformanceVote(rm, dj, 1)
	if err != ErrDJCannotVote {
		t.Fatalf("expected ErrDJCannotVote for DJ, got %v", err)
	}

	// 2. Outsider not in room cannot vote
	err = srv.RecordPerformanceVote(rm, "Outsider", 1)
	if err != ErrNotInRoom {
		t.Fatalf("expected ErrNotInRoom for outsider, got %v", err)
	}

	// 3. Room spectator votes successfully
	err = srv.RecordPerformanceVote(rm, fan, 1)
	if err != nil {
		t.Fatalf("expected fan vote to succeed, got %v", err)
	}
	if rm.Performance.Cheers != 1 {
		t.Fatalf("expected 1 cheer, got %d", rm.Performance.Cheers)
	}

	// 4. Same spectator cannot vote again
	err = srv.RecordPerformanceVote(rm, fan, 1)
	if err != ErrAlreadyVoted {
		t.Fatalf("expected ErrAlreadyVoted on duplicate vote, got %v", err)
	}
	if rm.Performance.Cheers != 1 {
		t.Fatalf("cheers increased on duplicate vote: %d", rm.Performance.Cheers)
	}
}

func TestPerformanceSettlesOnce(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "settle_test.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	srv := NewServer("./web", "127.0.0.1", "8080", store)
	rm := srv.GetOrCreateRoom("VIS$0")

	djDenizen, _ := store.GetOrCreateDenizen("DJOnce", "1", "avatar_dj", "")
	pDJ := &Player{ID: "p_dj", ScreenName: djDenizen.ScreenName, RoomID: rm.ID}
	rm.Players[pDJ.ID] = pDJ

	fanDenizen, _ := store.GetOrCreateDenizen("FanOnce", "1", "avatar_fan", "")
	pFan := &Player{ID: "p_fan", ScreenName: fanDenizen.ScreenName, RoomID: rm.ID}
	rm.Players[pFan.ID] = pFan

	initialBal, _, _ := store.GetBalance(djDenizen.ScreenName)

	edl := "^50,synth{4,0{^"
	_, err = srv.StartPerformance(rm, djDenizen.ScreenName, djDenizen.ScreenName, "Single Settle", edl)
	if err != nil {
		t.Fatalf("StartPerformance failed: %v", err)
	}

	_ = srv.RecordPerformanceVote(rm, fanDenizen.ScreenName, 1)

	// Call SettlePerformance concurrently from multiple goroutines
	var wg sync.WaitGroup
	var successCount int
	var failCount int
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := srv.SettlePerformance(rm)
			mu.Lock()
			if err == nil {
				successCount++
			} else if err == ErrPerformanceAlreadyDone {
				failCount++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful settlement, got %d", successCount)
	}
	if failCount != 4 {
		t.Fatalf("expected 4 duplicate settlements to be rejected with ErrPerformanceAlreadyDone, got %d", failCount)
	}

	finalBal, _, _ := store.GetBalance(djDenizen.ScreenName)
	// Cheers = 1 -> reward = (1*2) + 10 = 12 dB
	expectedAward := 12
	if finalBal != initialBal+expectedAward {
		t.Fatalf("expected final balance %d, got %d (award applied multiple times!)", initialBal+expectedAward, finalBal)
	}
}

func TestTop40ReturnsGenuineEmptyWhenNoActivity(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "top40_empty_test.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// With no votes in database, Top 40 must return empty array, NOT synthetic charts
	charts, err := store.GetTop40("daily")
	if err != nil {
		t.Fatalf("GetTop40 failed: %v", err)
	}
	if len(charts) != 0 {
		t.Fatalf("expected 0 entries in fresh Top 40, got %d (canned charts returned!)", len(charts))
	}
}
