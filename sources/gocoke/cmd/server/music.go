package main

import (
	"errors"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrPerformanceNotActive   = errors.New("no active performance in room")
	ErrPerformanceAlreadyDone = errors.New("performance has already settled")
	ErrDJCannotVote           = errors.New("performing DJ cannot vote on own track")
	ErrAlreadyVoted           = errors.New("denizen has already voted in this performance")
	ErrNotInRoom              = errors.New("voter must be present in the room to vote")
	ErrInvalidEDL             = errors.New("invalid or empty EDL sequence")
)

// SongComposition represents a persisted user composition in the database.
type SongComposition struct {
	ID        int       `json:"id"`
	DenizenID string    `json:"denizenId"`
	Artist    string    `json:"artist"`
	Title     string    `json:"title"`
	EDL       string    `json:"edl"`
	CreatedAt time.Time `json:"createdAt"`
}

// ActivePerformance represents an authoritative live DJ performance in a room.
type ActivePerformance struct {
	mu        sync.Mutex
	RoomID    string
	DJ        string
	Artist    string
	SongTitle string
	EDL       string
	StartedAt time.Time
	Duration  time.Duration
	Cheers    int
	Boos      int
	Voters    map[string]int // screenName -> +1 (cheer) or -1 (boo)
	Settled   bool
	SettledAt time.Time
	AwardedDB int
	cancelCh  chan struct{}
}

// ParseEDLDuration calculates the exact duration of a Coke Studios EDL sequence.
// EDL tracks are separated by '^'. Each track contains events formatted as {loopCount,startMS{...
// In the 120 BPM engine, standard 4-beat loop samples are 2000 ms.
func ParseEDLDuration(edl string) time.Duration {
	clean := strings.TrimSpace(edl)
	if clean == "" {
		return 30 * time.Second
	}

	parts := strings.Split(clean, "^")
	if len(parts) <= 1 {
		return 30 * time.Second
	}

	maxEndMS := 0

	for idx := 1; idx < len(parts); idx++ {
		seg := parts[idx]
		if seg == "" {
			continue
		}

		cursor := seg
		// Advance past track metadata to first event '{'
		metaEnd := strings.Index(cursor, "{")
		if metaEnd < 0 {
			continue
		}
		cursor = cursor[metaEnd:]

		// Parse {loopCount,startMS{
		for {
			lb := strings.Index(cursor, "{")
			if lb < 0 {
				break
			}
			cursor = cursor[lb+1:]

			// Read loopCount up to comma
			cidx := strings.Index(cursor, ",")
			if cidx < 0 {
				break
			}
			lcStr := strings.TrimSpace(cursor[:cidx])
			cursor = cursor[cidx+1:]

			// Read startMS up to next comma, '{', or end
			endIdx := len(cursor)
			if k := strings.Index(cursor, ",{"); k >= 0 {
				endIdx = k
			} else if k := strings.Index(cursor, ","); k >= 0 {
				endIdx = k
			} else if k := strings.Index(cursor, "{"); k >= 0 {
				endIdx = k
			}
			stStr := strings.TrimSpace(cursor[:endIdx])
			cursor = cursor[endIdx:]

			lc, _ := strconv.Atoi(lcStr)
			st, _ := strconv.Atoi(stStr)

			if lc > 0 {
				// Each loop iteration is 2000 ms (standard 4-beat bar at 120 BPM)
				sampleDurMS := 2000
				eventEndMS := st + (lc * sampleDurMS)
				if eventEndMS > maxEndMS {
					maxEndMS = eventEndMS
				}
			}
		}
	}

	if maxEndMS <= 0 {
		return 30 * time.Second
	}

	dur := time.Duration(maxEndMS) * time.Millisecond
	// Clamp duration to minimum 4s, maximum 180s (3 minutes)
	if dur < 4*time.Second {
		dur = 4 * time.Second
	}
	if dur > 180*time.Second {
		dur = 180 * time.Second
	}

	return dur
}

// StartPerformance begins an authoritative DJ performance in the given room.
func (s *Server) StartPerformance(room *Room, dj, artist, title, edl string) (*ActivePerformance, error) {
	if room == nil {
		return nil, fmt.Errorf("room is nil")
	}

	cleanEDL := strings.TrimSpace(edl)
	if cleanEDL == "" {
		return nil, ErrInvalidEDL
	}

	room.mu.Lock()
	if room.Performance != nil && !room.Performance.Done {
		room.mu.Unlock()
		return nil, fmt.Errorf("a song is already playing")
	}

	duration := ParseEDLDuration(cleanEDL)

	session := &PerformanceSession{
		DJ:        dj,
		Artist:    artist,
		SongTitle: title,
		EDL:       cleanEDL,
		StartedAt: time.Now(),
		Duration:  duration,
		Cheers:    0,
		Boos:      0,
		Voters:    make(map[string]bool),
		cancelCh:  make(chan struct{}),
	}
	room.Performance = session
	room.mu.Unlock()

	// Broadcast song start to all room occupants
	cdpXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_P av=\"%s\" a=\"%s\" sn=\"%s\" title=\"%s\" s=\"%s\" catId=\"89\" dur=\"%d\"/></Room.Action>",
		room.ID, html.EscapeString(dj), html.EscapeString(artist), html.EscapeString(title), html.EscapeString(title), html.EscapeString(cleanEDL), int(duration.Seconds()))
	room.Broadcast(cdpXML, "")
	log.Printf("[MUSIC] DJ %s started track %q in %s (Duration: %v)", dj, title, room.ID, duration)

	// Timer for performance completion
	go func() {
		select {
		case <-time.After(duration):
			s.SettlePerformance(room)
		case <-session.cancelCh:
			return
		}
	}()

	return &ActivePerformance{
		RoomID:    room.ID,
		DJ:        dj,
		Artist:    artist,
		SongTitle: title,
		EDL:       cleanEDL,
		StartedAt: session.StartedAt,
		Duration:  duration,
		Voters:    make(map[string]int),
		cancelCh:  session.cancelCh,
	}, nil
}

// RecordPerformanceVote records a spectator cheer (+1) or boo (-1) with strict eligibility checks.
func (s *Server) RecordPerformanceVote(room *Room, voter string, delta int) error {
	if room == nil {
		return fmt.Errorf("room is nil")
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	session := room.Performance
	if session == nil || session.Done {
		return ErrPerformanceNotActive
	}

	// 1. Voter cannot be the performing DJ
	if strings.EqualFold(strings.TrimSpace(voter), strings.TrimSpace(session.DJ)) {
		return ErrDJCannotVote
	}

	// 2. Voter must be present in the room
	voterPresent := false
	for _, p := range room.Players {
		if strings.EqualFold(strings.TrimSpace(p.ScreenName), strings.TrimSpace(voter)) {
			voterPresent = true
			break
		}
	}
	if !voterPresent {
		return ErrNotInRoom
	}

	// 3. Max 1 vote per spectator per performance session
	if session.Voters[voter] {
		return ErrAlreadyVoted
	}

	session.Voters[voter] = true
	if delta > 0 {
		session.Cheers++
	} else if delta < 0 {
		session.Boos++
	}

	// Broadcast vote reaction to room
	tu := session.Cheers
	td := session.Boos
	log.Printf("[VOTE] [%s] %s voted (delta: %d, Cheers: %d, Boos: %d)", room.ID, voter, delta, tu, td)

	// Record qualifying vote in persistent top40_votes table
	if s.store != nil {
		_, _ = s.store.db.Exec("INSERT INTO top40_votes (creator_name, song_title, voter_name) VALUES (?, ?, ?)",
			session.DJ, session.SongTitle, voter)
	}

	return nil
}

// SettlePerformance finalizes the performance, awards Decibels once, and broadcasts settlement results.
func (s *Server) SettlePerformance(room *Room) (int, error) {
	if room == nil {
		return 0, fmt.Errorf("room is nil")
	}

	room.mu.Lock()
	session := room.Performance
	if session == nil {
		room.mu.Unlock()
		return 0, ErrPerformanceNotActive
	}
	if session.Done {
		room.mu.Unlock()
		return 0, ErrPerformanceAlreadyDone
	}

	session.Done = true
	cheers := session.Cheers
	boos := session.Boos
	dj := session.DJ
	room.mu.Unlock()

	// Reward formula: max(0, (Cheers * 2) - Boos) + 10 audience participation bonus
	rewardDB := (cheers * 2) - boos
	if rewardDB < 0 {
		rewardDB = 0
	}
	rewardDB += 10

	// Credit Decibels to DJ in persistent storage
	if s.store != nil && dj != "" {
		_, _ = s.store.AdjustBalance(dj, rewardDB, 0)
	}

	log.Printf("[MUSIC] Performance settled for %s in %s: Cheers=%d, Boos=%d, Awarded=+%d dB",
		dj, room.ID, cheers, boos, rewardDB)

	// 1. Broadcast CD_VR with official voting results dialog
	vrXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_VR tur=\"%d\" tdr=\"%d\" pa=\"%d\"/></Room.Action>",
		room.ID, cheers, boos, rewardDB)
	room.Broadcast(vrXML, "")

	// 2. Broadcast CD_PRS with crowd reaction audio
	prsXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_PRS av=\"%s\" tur=\"%d\" tdr=\"%d\" tu=\"%d\" td=\"%d\" db=\"%d\"/></Room.Action>",
		room.ID, dj, cheers, boos, cheers, boos, rewardDB)
	room.Broadcast(prsXML, "")
	room.Broadcast(fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_E/></Room.Action>", room.ID), "")

	return rewardDB, nil
}

// GetLateJoinPosition calculates current playback offset for spectators joining mid-song.
func (s *Server) GetLateJoinPosition(room *Room) (time.Duration, bool) {
	if room == nil {
		return 0, false
	}

	room.mu.RLock()
	defer room.mu.RUnlock()

	session := room.Performance
	if session == nil || session.Done {
		return 0, false
	}

	elapsed := time.Since(session.StartedAt)
	if elapsed >= session.Duration {
		return 0, false
	}

	return elapsed, true
}
