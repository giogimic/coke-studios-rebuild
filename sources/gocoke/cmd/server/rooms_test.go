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
	closed   bool
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
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

func (w *testPacketWriter) IsClosed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closed
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
	// Alice is at evidenced entrance (Col=8, Row=4 in Studio A MapXml)
	if player.X != 8 || player.Z != 4 {
		t.Fatalf("expected entrance (8, 4), got (%d, %d)", player.X, player.Z)
	}

	// Attempt move to blocked destination (0, 0)
	srv.handlePlayerMessage(player, `<Room.Action roomID="test_studio_layout1"><AV_GSP><P x="0" z="0"/></AV_GSP></Room.Action>`)
	if player.X == 0 && player.Z == 0 {
		t.Fatalf("server accepted move to blocked tile (0, 0)!")
	}
	if player.X != 8 || player.Z != 4 {
		t.Fatalf("player position corrupted, expected (8, 4), got (%d, %d)", player.X, player.Z)
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

func TestMultiTileFurnitureFootprintCollision(t *testing.T) {
	rm := NewRoom("test_footprint", "Footprint Test Room", 2)
	rm.EntranceX = 0
	rm.EntranceZ = 0

	// 1. Initially (5, 5), (5, 6), (6, 5), (6, 6) are walkable
	for _, pt := range [][2]int{{5, 5}, {5, 6}, {6, 5}, {6, 6}} {
		if !rm.IsWalkable(pt[0], pt[1]) {
			t.Fatalf("expected (%d,%d) to be initially walkable", pt[0], pt[1])
		}
	}

	// 2. Place a 2x2 solid item at (5, 5) with dir=2
	rm.SetFurnitureFootprint("furni_2x2", 5, 5, 2, 2, 2, true)

	// All 4 tiles must now be blocked
	for _, pt := range [][2]int{{5, 5}, {5, 6}, {6, 5}, {6, 6}} {
		if rm.IsWalkable(pt[0], pt[1]) {
			t.Fatalf("expected (%d,%d) to be blocked by 2x2 furniture", pt[0], pt[1])
		}
	}

	// 3. Remove the furniture
	rm.RemoveFurnitureSolid("furni_2x2")

	// All 4 tiles must be walkable again
	for _, pt := range [][2]int{{5, 5}, {5, 6}, {6, 5}, {6, 6}} {
		if !rm.IsWalkable(pt[0], pt[1]) {
			t.Fatalf("expected (%d,%d) to be walkable after furniture removal", pt[0], pt[1])
		}
	}

	// 4. Test 1x2 item with orientation swap
	// dir=2: width=1, depth=2 -> occupies (10, 10) and (10, 11)
	rm.SetFurnitureFootprint("furni_1x2", 10, 10, 1, 2, 2, true)
	if rm.IsWalkable(10, 10) || rm.IsWalkable(10, 11) {
		t.Fatalf("expected (10,10) and (10,11) to be blocked for dir 2")
	}
	if !rm.IsWalkable(11, 10) {
		t.Fatalf("expected (11,10) to remain walkable for dir 2")
	}

	// Rotate to dir=0: dimensions swap to width=2, depth=1 -> occupies (10, 10) and (11, 10)
	rm.RemoveFurnitureSolid("furni_1x2")
	rm.SetFurnitureFootprint("furni_1x2", 10, 10, 1, 2, 0, true)
	if rm.IsWalkable(10, 10) || rm.IsWalkable(11, 10) {
		t.Fatalf("expected (10,10) and (11,10) to be blocked for dir 0")
	}
	if !rm.IsWalkable(10, 11) {
		t.Fatalf("expected (10,11) to be walkable for dir 0")
	}
}

// 5. TestFurniturePlacementRejectionSendsPOSReject verifies that when an unauthorized or
// unowned furniture placement is attempted, the server rejects with both <Alert and <POS_REJECT.
func TestFurniturePlacementRejectionSendsPOSReject(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)
	_, _ = srv.store.CreateStudio("room_alice", "u1", "Alice", "Alice Studio", "studio_model_a", 1)

	// Bob enters Alice's room and tries to place furniture
	bob, bobWriter := createTestPlayer(srv, "p_bob", "Bob", "")
	srv.handlePlayerMessage(bob, `<Room.Enter msgID="2" roomID="room_alice"/>`)

	// Bob attempts POS_STD in Alice's room (Bob is not the owner)
	srv.handlePlayerMessage(bob, `<Room.Action roomID="room_alice"><POS_STD id="9999" cid="10" x="5" y="0" z="5" ho="0" d="2" st="0"/></Room.Action>`)

	if !bobWriter.HasMessage("<POS_REJECT") {
		t.Fatalf("expected Bob to receive <POS_REJECT on unauthorized placement, got: %v", bobWriter.GetMessages())
	}
	if !bobWriter.HasMessage("<Alert") {
		t.Fatalf("expected Bob to receive <Alert on unauthorized placement, got: %v", bobWriter.GetMessages())
	}
}

func TestPublicVenuesEntranceAndPathMap(t *testing.T) {
	srv := NewServer("../../web", "127.0.0.1", "8080", nil)

	venues := []struct {
		roomID   string
		name     string
		entX     int
		entZ     int
		location int
	}{
		{"$SYSTEM$1", "London I", 11, 2, 1},
		{"$SYSTEM$19", "Alaska I", 15, 24, 10},
		{"$SYSTEM$3", "Miami I", 22, 22, 2},
		{"$SYSTEM$5", "Mombasa I", 19, 25, 3},
		{"$SYSTEM$17", "Goa I", 13, 14, 9},
		{"$SYSTEM$21", "Moscow I", 12, 25, 11},
		{"$SYSTEM$27", "Central Park I", 16, 25, 14},
	}

	for _, v := range venues {
		t.Run(v.name, func(t *testing.T) {
			rm, err := srv.GetRoom(v.roomID)
			if err != nil {
				t.Fatalf("failed to get room %s: %v", v.roomID, err)
			}
			if rm.Location != v.location {
				t.Errorf("expected location %d for %s, got %d", v.location, v.name, rm.Location)
			}
			if rm.EntranceX != v.entX || rm.EntranceZ != v.entZ {
				t.Errorf("expected entrance (%d,%d) for %s, got (%d,%d)", v.entX, v.entZ, v.name, rm.EntranceX, rm.EntranceZ)
			}
			if len(rm.walkableGrid) == 0 {
				t.Errorf("expected non-empty walkable grid for %s", v.name)
			}
			// The entrance tile must be walkable
			if !rm.IsWalkable(v.entX, v.entZ) {
				t.Errorf("entrance tile (%d,%d) should be walkable for %s", v.entX, v.entZ, v.name)
			}
		})
	}
}
