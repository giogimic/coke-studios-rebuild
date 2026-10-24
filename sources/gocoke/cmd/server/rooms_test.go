package main

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

type testPacketWriter struct {
	mu       sync.Mutex
	messages []string
}

func newTestPacketWriter() *testPacketWriter {
	return &testPacketWriter{}
}

func (w *testPacketWriter) WriteBinary(p []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	msg := string(bytes.TrimRight(p, "\x00"))
	w.messages = append(w.messages, msg)
	return nil
}

func (w *testPacketWriter) Close() error {
	return nil
}

func (w *testPacketWriter) GetMessages() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	copied := make([]string, len(w.messages))
	copy(copied, w.messages)
	return copied
}

func (w *testPacketWriter) HasMessage(pattern string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, m := range w.messages {
		if strings.Contains(m, pattern) {
			return true
		}
	}
	return false
}

func createTestPlayer(srv *Server, id, screenName, roomID string) (*Player, *testPacketWriter) {
	writer := newTestPacketWriter()
	player := &Player{
		ID:            id,
		ScreenName:    screenName,
		Authenticated: true,
		RoomID:        roomID,
		Conn:          writer,
	}
	srv.mu.Lock()
	srv.Players[id] = player
	srv.mu.Unlock()
	return player, writer
}

// 1. TestRoomRejectsUnknownID verifies that unknown room IDs are rejected
// and not automatically synthesized by GetOrCreateRoom.
func TestRoomRejectsUnknownID(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)
	player, writer := createTestPlayer(srv, "p_alice", "Alice", "")

	// Alice tries to enter non-existent room
	srv.handlePlayerMessage(player, `<Room.Enter msgID="2" roomID="NON_EXISTENT_9999"/>`)

	// Room must NOT be synthesized in srv.Rooms
	srv.mu.RLock()
	_, exists := srv.Rooms["NON_EXISTENT_9999"]
	srv.mu.RUnlock()
	if exists {
		t.Fatalf("server synthesized an unknown room ID NON_EXISTENT_9999!")
	}

	// Player must NOT be in room
	if player.RoomID == "NON_EXISTENT_9999" {
		t.Fatalf("player.RoomID set to non-existent room")
	}

	// Must have received rejection packet
	if !writer.HasMessage("<Room.NotFound") && !writer.HasMessage("<Alert") {
		t.Fatalf("expected rejection packet, got: %v", writer.GetMessages())
	}
}

// 2. TestMoveRejectsBlockedDestination verifies that destinations outside room bounds
// or blocked by walls/furniture are rejected by authoritative movement logic.
func TestMoveRejectsBlockedDestination(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)
	// Create studio with layout 1 (Studio A)
	_, err := srv.store.CreateStudio("test_studio_layout1", "u1", "Alice", "Alice Studio", "studio_model_a", 1)
	if err != nil {
		t.Fatalf("failed to create studio: %v", err)
	}
	player, writer := createTestPlayer(srv, "p_alice", "Alice", "")

	// Enter test_studio_layout1
	srv.handlePlayerMessage(player, `<Room.Enter msgID="2" roomID="test_studio_layout1"/>`)
	if player.RoomID != "test_studio_layout1" {
		t.Fatalf("expected player in test_studio_layout1, got %q", player.RoomID)
	}
	// Alice is at evidenced entrance (4, 8)
	if player.X != 4 || player.Z != 8 {
		t.Fatalf("expected entrance (4, 8), got (%d, %d)", player.X, player.Z)
	}

	// Attempt move to blocked destination (0, 0)
	srv.handlePlayerMessage(player, `<Room.Action roomID="test_studio_layout1"><AV_GSP><P x="0" z="0"/></AV_GSP></Room.Action>`)
	if player.X == 0 && player.Z == 0 {
		t.Fatalf("server accepted move to blocked tile (0, 0)!")
	}
	if player.X != 4 || player.Z != 8 {
		t.Fatalf("player position corrupted, expected (4, 8), got (%d, %d)", player.X, player.Z)
	}

	// Move to valid walkable tile (6, 7)
	srv.handlePlayerMessage(player, `<Room.Action roomID="test_studio_layout1"><AV_GSP><P x="6" z="7"/></AV_GSP></Room.Action>`)
	if player.X != 6 || player.Z != 7 {
		t.Fatalf("expected player at (6, 7), got (%d, %d)", player.X, player.Z)
	}
	if !writer.HasMessage(`<AV_MV av="Alice" x="6" z="7"/>`) {
		t.Fatalf("expected AV_MV broadcast for (6, 7), got: %v", writer.GetMessages())
	}
}

// 3. TestRoomTransitionRemovesOldPresence verifies that when a player leaves a room,
// AV_RM is broadcast to existing occupants, the player is removed from the room,
// and late joiners do not see phantom avatars.
func TestRoomTransitionRemovesOldPresence(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)
	_, _ = srv.store.CreateStudio("room_a", "u1", "Alice", "Room A", "studio_model_a", 1)
	_, _ = srv.store.CreateStudio("room_b", "u2", "Bob", "Room B", "studio_model_a", 1)

	alice, aliceWriter := createTestPlayer(srv, "p_alice", "Alice", "")
	bob, _ := createTestPlayer(srv, "p_bob", "Bob", "")

	// Both enter room_a
	srv.handlePlayerMessage(alice, `<Room.Enter msgID="2" roomID="room_a"/>`)
	srv.handlePlayerMessage(bob, `<Room.Enter msgID="2" roomID="room_a"/>`)

	rmA, err := srv.GetRoom("room_a")
	if err != nil || len(rmA.Players) != 2 {
		t.Fatalf("expected 2 players in room_a, got %d (err: %v)", len(rmA.Players), err)
	}

	// Bob transitions from room_a to room_b
	srv.handlePlayerMessage(bob, `<Room.Enter msgID="3" roomID="room_b"/>`)

	// 1. Alice must receive AV_RM for Bob in room_a
	if !aliceWriter.HasMessage(`<AV_RM av="Bob"/>`) {
		t.Fatalf("expected Alice to receive AV_RM for Bob, got: %v", aliceWriter.GetMessages())
	}

	// 2. Bob must not be in room_a Players
	rmA.mu.RLock()
	_, bobInA := rmA.Players[bob.ID]
	rmA.mu.RUnlock()
	if bobInA {
		t.Fatalf("Bob still in room_a Players map!")
	}

	// 3. Charlie joins room_a -> Charlie must see Alice, NOT Bob
	charlie, charlieWriter := createTestPlayer(srv, "p_charlie", "Charlie", "")
	srv.handlePlayerMessage(charlie, `<Room.Enter msgID="2" roomID="room_a"/>`)
	if !charlieWriter.HasMessage(`av="Alice"`) {
		t.Fatalf("expected Charlie to see Alice in room_a")
	}
	if charlieWriter.HasMessage(`av="Bob"`) {
		t.Fatalf("Charlie saw phantom Bob in room_a!")
	}
}

// 4. TestReconnectRestoresRoomState verifies that reconnecting restores the exact room snapshot,
// including decor, placed furniture, and present avatars.
func TestReconnectRestoresRoomState(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)
	_, _ = srv.store.CreateStudio("studio_decor", "u1", "Alice", "Alice Pad", "studio_model_c", 3)
	// Alice updates decor and places furniture
	_ = srv.store.UpdateStudioDecor("studio_decor", 3, 2, 2)
	_ = srv.store.PlaceFurniture("studio_decor", "sofa_101", 45, 8, 0, 8, 0, 2, 0, false, nil)

	alice, _ := createTestPlayer(srv, "p_alice", "Alice", "")
	srv.handlePlayerMessage(alice, `<Room.Enter msgID="2" roomID="studio_decor"/>`)
	srv.handlePlayerMessage(alice, `<Room.Action roomID="studio_decor"><AV_EN def="avatar_alice"/></Room.Action>`)

	// Bob connects and enters
	bob, _ := createTestPlayer(srv, "p_bob", "Bob", "")
	srv.handlePlayerMessage(bob, `<Room.Enter msgID="2" roomID="studio_decor"/>`)

	// Bob disconnects
	srv.removePlayerFromRoom(bob, "studio_decor")
	srv.mu.Lock()
	delete(srv.Players, bob.ID)
	srv.mu.Unlock()

	// Bob reconnects with a new session
	bob2, bob2Writer := createTestPlayer(srv, "p_bob_recon", "Bob", "")
	srv.handlePlayerMessage(bob2, `<Room.Enter msgID="2" roomID="studio_decor"/>`)

	// Verify Bob2 receives decor, placed furniture, and Alice avatar
	if !bob2Writer.HasMessage(`wp="3" fl="2" db="2"`) {
		t.Fatalf("Bob did not receive persisted decor wp=3 fl=2 db=2: %v", bob2Writer.GetMessages())
	}
	if !bob2Writer.HasMessage(`id="sofa_101" cid="45"`) {
		t.Fatalf("Bob did not receive placed furniture sofa_101: %v", bob2Writer.GetMessages())
	}
	if !bob2Writer.HasMessage(`av="Alice"`) {
		t.Fatalf("Bob did not receive present avatar Alice: %v", bob2Writer.GetMessages())
	}
}
