package main

import (
	"testing"
)

func TestFriendRequestAcceptRefuse(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	defer store.Close()

	srv := NewServer(".", "127.0.0.1", "8080", store)
	rm := srv.GetOrCreateRoom("VIS$0")

	_, _ = store.RegisterDenizen("Alice", "pass123", "1", "hr=001/1", "Alice")
	_, _ = store.RegisterDenizen("Bob", "pass123", "2", "hr=002/2", "Bob")

	pAlice := &Player{ID: "p_alice", ScreenName: "Alice", Authenticated: true, RoomID: rm.ID}
	pBob := &Player{ID: "p_bob", ScreenName: "Bob", Authenticated: true, RoomID: rm.ID}

	srv.mu.Lock()
	srv.Players["p_alice"] = pAlice
	srv.Players["p_bob"] = pBob
	rm.Players["p_alice"] = pAlice
	rm.Players["p_bob"] = pBob
	srv.mu.Unlock()

	// 1. Alice sends friend request to Bob
	srv.handlePlayerMessage(pAlice, `<MMS_REQ to="Bob"/>`)

	// Bob should have pending request from Alice
	pending, err := store.GetPendingFriendRequests("Bob")
	if err != nil || len(pending) != 1 || pending[0] != "Alice" {
		t.Fatalf("Expected Bob to have pending friend request from Alice, got %v", pending)
	}

	// But Bob and Alice must NOT be listed as friends yet!
	friendsBob, _ := store.GetFriends("Bob")
	if len(friendsBob) != 0 {
		t.Fatalf("Bob should have 0 accepted friends, got %d", len(friendsBob))
	}

	// 2. Bob refuses Alice's request
	srv.handlePlayerMessage(pBob, `<MMS_REJ from="Alice"/>`)

	// Pending request should be deleted
	pendingAfterRefuse, _ := store.GetPendingFriendRequests("Bob")
	if len(pendingAfterRefuse) != 0 {
		t.Fatalf("Pending requests should be empty after refusal, got %v", pendingAfterRefuse)
	}
	friendsAfterRefuse, _ := store.GetFriends("Bob")
	if len(friendsAfterRefuse) != 0 {
		t.Fatalf("Friends list should still be empty after refusal")
	}

	// 3. Alice sends request again, and this time Bob accepts
	srv.handlePlayerMessage(pAlice, `<MSGR_INV sn="Bob"/>`)
	srv.handlePlayerMessage(pBob, `<MSGR_AC_INV inviter="Alice" invitee="Bob"/>`)

	// Bidirectional friendship must now be persisted
	friendsBobFinal, err := store.GetFriends("Bob")
	if err != nil || len(friendsBobFinal) != 1 || friendsBobFinal[0].ScreenName != "Alice" {
		t.Fatalf("Expected Bob to have Alice as accepted friend, got %v", friendsBobFinal)
	}

	friendsAliceFinal, err := store.GetFriends("Alice")
	if err != nil || len(friendsAliceFinal) != 1 || friendsAliceFinal[0].ScreenName != "Bob" {
		t.Fatalf("Expected Alice to have Bob as accepted friend, got %v", friendsAliceFinal)
	}
}

func TestMessagingIdentityAndDelivery(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	defer store.Close()

	srv := NewServer(".", "127.0.0.1", "8080", store)
	rm := srv.GetOrCreateRoom("VIS$0")

	_, _ = store.RegisterDenizen("Alice", "pass123", "1", "hr=001/1", "Alice")
	_, _ = store.RegisterDenizen("Bob", "pass123", "2", "hr=002/2", "Bob")

	pAlice := &Player{ID: "p_alice", ScreenName: "Alice", Authenticated: true, RoomID: rm.ID}
	pBob := &Player{ID: "p_bob", ScreenName: "Bob", Authenticated: true, RoomID: rm.ID}

	srv.mu.Lock()
	srv.Players["p_alice"] = pAlice
	srv.Players["p_bob"] = pBob
	rm.Players["p_alice"] = pAlice
	rm.Players["p_bob"] = pBob
	srv.mu.Unlock()

	// Alice sends message to Bob
	srv.handlePlayerMessage(pAlice, `<MMS_SND to="Bob" msg="Hey Bob, check out my new track!"/>`)

	// Check persisted messages for Bob
	msgs, err := store.GetMessages("Bob", false)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("Expected Bob to receive 1 message, got %d (err: %v)", len(msgs), err)
	}

	if msgs[0].SenderName != "Alice" {
		t.Fatalf("Expected sender to be Alice, got %q", msgs[0].SenderName)
	}
	if msgs[0].Message != "Hey Bob, check out my new track!" {
		t.Fatalf("Unexpected message text: %q", msgs[0].Message)
	}
}

func TestHostPermissionsAndModeration(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	defer store.Close()

	srv := NewServer(".", "127.0.0.1", "8080", store)

	// Alice registers and owns studio_alice
	_, _ = store.RegisterDenizen("Alice", "pass123", "1", "hr=001/1", "Alice")
	_, _ = store.RegisterDenizen("Charlie", "pass123", "1", "hr=002/2", "Charlie")
	_, _ = store.RegisterDenizen("Dave", "pass123", "2", "hr=003/1", "Dave")

	st, err := store.CreateStudio("studio_alice", "alice_id", "Alice", "Alice's Pad", "studio_model_c", 3)
	if err != nil {
		t.Fatalf("CreateStudio failed: %v", err)
	}

	studio := srv.GetOrCreateRoom(st.ID)
	studio.OwnerName = "Alice"

	pAlice := &Player{ID: "p_alice", ScreenName: "Alice", Authenticated: true, RoomID: studio.ID}
	pCharlie := &Player{ID: "p_charlie", ScreenName: "Charlie", Authenticated: true, RoomID: studio.ID}
	pDave := &Player{ID: "p_dave", ScreenName: "Dave", Authenticated: true, RoomID: studio.ID}

	srv.mu.Lock()
	srv.Players["p_alice"] = pAlice
	srv.Players["p_charlie"] = pCharlie
	srv.Players["p_dave"] = pDave
	studio.Players["p_alice"] = pAlice
	studio.Players["p_charlie"] = pCharlie
	studio.Players["p_dave"] = pDave
	srv.mu.Unlock()

	// 1. Host Moderation Kick:
	// Charlie (visitor, non-owner) attempts to kick Dave
	srv.handlePlayerMessage(pCharlie, `<ST_KICK target="Dave"/>`)

	// Verification: Dave must NOT be kicked by Charlie
	studio.mu.RLock()
	daveInRoom := studio.Players["p_dave"] != nil
	studio.mu.RUnlock()
	if !daveInRoom {
		t.Fatalf("Charlie is not the host! Dave should not have been kicked by Charlie")
	}

	// Alice (host owner) kicks Dave
	srv.handlePlayerMessage(pAlice, `<ST_KICK target="Dave"/>`)

	studio.mu.RLock()
	daveStillInRoom := studio.Players["p_dave"] != nil
	studio.mu.RUnlock()
	if daveStillInRoom {
		t.Fatalf("Alice is the host owner! Dave should have been kicked by Alice")
	}
	if pDave.RoomID != "$LOBBY$" {
		t.Fatalf("Kicked player should be moved to $LOBBY$, got %q", pDave.RoomID)
	}

	// 2. Host Doorbell Control:
	// Dave knocks
	srv.handlePlayerMessage(pDave, `<ST_KNOCK roomID="studio_alice"/>`)

	// Charlie tries to respond to knock -> unauthorized
	srv.handlePlayerMessage(pCharlie, `<ST_KNOCK_RES target="Dave" allow="1"/>`)

	// Alice (host owner) responds to knock -> authorized
	srv.handlePlayerMessage(pAlice, `<ST_KNOCK_RES target="Dave" allow="1"/>`)

	// 3. Host Decor Update:
	// Charlie tries to update decor
	srv.handlePlayerMessage(pCharlie, `<ST_DECOR wp="3" fl="4" db="2"/>`)
	if studio.Wallpaper == 3 || studio.Floor == 4 {
		t.Fatalf("Charlie is not the host! Decor should not have been updated by Charlie")
	}

	// Alice updates decor
	srv.handlePlayerMessage(pAlice, `<ST_DECOR wp="3" fl="4" db="2"/>`)
	if studio.Wallpaper != 3 || studio.Floor != 4 || studio.Doorbell != 2 {
		t.Fatalf("Alice is the host! Expected wp=3, fl=4, db=2, got wp=%d, fl=%d, db=%d",
			studio.Wallpaper, studio.Floor, studio.Doorbell)
	}

	// Check DB persistence of studio decor
	stRecord, err := store.GetStudio("studio_alice")
	if err != nil {
		t.Fatalf("GetStudio failed: %v", err)
	}
	if stRecord.Wallpaper != 3 || stRecord.Floor != 4 || stRecord.Doorbell != 2 {
		t.Fatalf("DB studio decor mismatch: wp=%d fl=%d db=%d", stRecord.Wallpaper, stRecord.Floor, stRecord.Doorbell)
	}
}

func TestCallForHelpReport(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	defer store.Close()

	srv := NewServer(".", "127.0.0.1", "8080", store)
	rm := srv.GetOrCreateRoom("VIS$0")

	_, _ = store.RegisterDenizen("ReporterUser", "pass123", "1", "hr=001/1", "Reporter")
	pReporter := &Player{ID: "p_rep", ScreenName: "ReporterUser", Authenticated: true, RoomID: rm.ID}

	srv.mu.Lock()
	srv.Players["p_rep"] = pReporter
	rm.Players["p_rep"] = pReporter
	srv.mu.Unlock()

	srv.handlePlayerMessage(pReporter, `<CFH msg="Inappropriate behavior in studio" av="BadUser"/>`)

	// Check database record
	var reporter, suspect, msg, status string
	err = store.db.QueryRow("SELECT reporter_name, suspect_name, message, status FROM call_for_help WHERE reporter_name = 'ReporterUser'").
		Scan(&reporter, &suspect, &msg, &status)
	if err != nil {
		t.Fatalf("Expected CFH to be recorded in call_for_help table: %v", err)
	}

	if suspect != "BadUser" || msg != "Inappropriate behavior in studio" || status != "open" {
		t.Fatalf("CFH record mismatch: reporter=%q suspect=%q msg=%q status=%q", reporter, suspect, msg, status)
	}
}
