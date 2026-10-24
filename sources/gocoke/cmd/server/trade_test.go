package main

import (
	"fmt"
	"strconv"
	"testing"
)

func TestTradeRequiresBothConfirmations(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	defer store.Close()

	srv := NewServer(".", "127.0.0.1", "8080", store)
	rm := srv.GetOrCreateRoom("VIS$0")

	// 1. Setup Alice and Bob
	_, _ = store.RegisterDenizen("Alice", "pass123", "1", "hr=001/1", "Alice mission")
	_, _ = store.RegisterDenizen("Bob", "pass123", "2", "hr=002/2", "Bob mission")

	posAlice, err := store.AddBackpackItem("Alice", 45, map[string]any{"color": "red"})
	if err != nil {
		t.Fatalf("AddBackpackItem Alice failed: %v", err)
	}
	posBob, err := store.AddBackpackItem("Bob", 50, map[string]any{"color": "blue"})
	if err != nil {
		t.Fatalf("AddBackpackItem Bob failed: %v", err)
	}

	pAlice := &Player{ID: "p_alice", ScreenName: "Alice", Authenticated: true, RoomID: rm.ID}
	pBob := &Player{ID: "p_bob", ScreenName: "Bob", Authenticated: true, RoomID: rm.ID}

	srv.mu.Lock()
	srv.Players["p_alice"] = pAlice
	srv.Players["p_bob"] = pBob
	rm.Players["p_alice"] = pAlice
	rm.Players["p_bob"] = pBob
	srv.mu.Unlock()

	// 2. Alice starts trade with Bob
	srv.handlePlayerMessage(pAlice, `<Room.Action roomID="VIS$0"><TD_ST tde="Bob"/></Room.Action>`)
	sess := srv.tradeManager.GetSession("Alice")
	if sess == nil {
		t.Fatalf("Expected trade session to be created")
	}

	// 3. Alice adds item and agrees
	srv.handlePlayerMessage(pAlice, fmt.Sprintf(`<Room.Action roomID="VIS$0"><TD_AD id="%d"/></Room.Action>`, posAlice))
	srv.handlePlayerMessage(pAlice, `<Room.Action roomID="VIS$0"><TD_AG/></Room.Action>`)

	// Verification: Bob has not agreed, so no items should be swapped yet
	itemAliceCheck, _ := store.GetBackpackItem("Alice", posAlice)
	if itemAliceCheck == nil {
		t.Fatalf("Alice's item should still be in Alice's backpack")
	}

	// 4. Bob adds his item
	srv.handlePlayerMessage(pBob, fmt.Sprintf(`<Room.Action roomID="VIS$0"><TD_AD id="%d"/></Room.Action>`, posBob))

	// Adding an item must reset agreement
	if sess.TraderAgreed || sess.TradeeAgreed {
		t.Fatalf("Adding an item to trade should reset agree flags")
	}

	// 5. Alice agrees again, but Bob has not
	srv.handlePlayerMessage(pAlice, `<Room.Action roomID="VIS$0"><TD_AG/></Room.Action>`)
	if srv.tradeManager.GetSession("Alice") == nil {
		t.Fatalf("Trade should not complete when only Alice has agreed")
	}

	// 6. Bob agrees -> both agreed -> atomic trade completes
	srv.handlePlayerMessage(pBob, `<Room.Action roomID="VIS$0"><TD_AG/></Room.Action>`)

	// Session should be closed
	if srv.tradeManager.GetSession("Alice") != nil {
		t.Fatalf("Trade session should be cleaned up after completion")
	}

	// Bob should now own posAlice (45)
	itemAliceNew, err := store.GetBackpackItem("Bob", posAlice)
	if err != nil || itemAliceNew == nil {
		t.Fatalf("Expected Bob to receive posAlice: %v", err)
	}
	if itemAliceNew.CatalogItemID != 45 {
		t.Fatalf("Expected Bob to receive catalog item 45, got %d", itemAliceNew.CatalogItemID)
	}

	// Alice should now own posBob (50)
	itemBobNew, err := store.GetBackpackItem("Alice", posBob)
	if err != nil || itemBobNew == nil {
		t.Fatalf("Expected Alice to receive posBob: %v", err)
	}
	if itemBobNew.CatalogItemID != 50 {
		t.Fatalf("Expected Alice to receive catalog item 50, got %d", itemBobNew.CatalogItemID)
	}
}

func TestTradeCannotDuplicatePossession(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	defer store.Close()

	srv := NewServer(".", "127.0.0.1", "8080", store)
	rm := srv.GetOrCreateRoom("VIS$0")

	_, _ = store.RegisterDenizen("Alice", "pass123", "1", "hr=001/1", "Alice")
	_, _ = store.RegisterDenizen("Bob", "pass123", "2", "hr=002/2", "Bob")
	_, _ = store.CreateStudio("studio_alice", "alice_id", "Alice", "Alice Studio", "studio_model_c", 3)

	posAlice, _ := store.AddBackpackItem("Alice", 45, map[string]any{})
	posBob, _ := store.AddBackpackItem("Bob", 50, map[string]any{})

	pAlice := &Player{ID: "p_alice", ScreenName: "Alice", Authenticated: true, RoomID: rm.ID}
	pBob := &Player{ID: "p_bob", ScreenName: "Bob", Authenticated: true, RoomID: rm.ID}

	srv.mu.Lock()
	srv.Players["p_alice"] = pAlice
	srv.Players["p_bob"] = pBob
	rm.Players["p_alice"] = pAlice
	rm.Players["p_bob"] = pBob
	srv.mu.Unlock()

	// 1. Alice starts trade and adds posAlice
	srv.handlePlayerMessage(pAlice, `<Room.Action roomID="VIS$0"><TD_ST tde="Bob"/></Room.Action>`)
	srv.handlePlayerMessage(pAlice, fmt.Sprintf(`<Room.Action roomID="VIS$0"><TD_AD id="%d"/></Room.Action>`, posAlice))

	// 2. Alice attempts to add the same posAlice a second time -> should be rejected
	err = srv.tradeManager.AddItem("Alice", posAlice)
	if err == nil {
		t.Fatalf("Expected duplicate item add to be rejected")
	}

	// 3. Alice concurrently places posAlice onto her studio floor
	err = store.PlaceBackpackItem("studio_alice", "Alice", strconv.Itoa(posAlice), 45, 1, 1, 1, 0, 2, 0, false, nil)
	if err != nil {
		t.Fatalf("PlaceBackpackItem failed: %v", err)
	}

	// 4. Bob adds his item and both agree
	srv.handlePlayerMessage(pBob, fmt.Sprintf(`<Room.Action roomID="VIS$0"><TD_AD id="%d"/></Room.Action>`, posBob))
	srv.handlePlayerMessage(pAlice, `<Room.Action roomID="VIS$0"><TD_AG/></Room.Action>`)

	// Bob agrees -> completion triggered, but posAlice is no longer in Alice's backpack!
	srv.handlePlayerMessage(pBob, `<Room.Action roomID="VIS$0"><TD_AG/></Room.Action>`)

	// Verification: Atomic transaction should fail, so Bob does NOT receive posAlice
	itemAliceOnBob, _ := store.GetBackpackItem("Bob", posAlice)
	if itemAliceOnBob != nil {
		t.Fatalf("Bob should NOT receive posAlice because Alice placed it on the floor!")
	}

	// Bob still owns his own posBob in backpack
	itemBobCheck, err := store.GetBackpackItem("Bob", posBob)
	if err != nil || itemBobCheck == nil {
		t.Fatalf("Bob should still own posBob in backpack: %v", err)
	}
}

func TestTradeDisconnectDoesNotLoseItems(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	defer store.Close()

	srv := NewServer(".", "127.0.0.1", "8080", store)
	rm := srv.GetOrCreateRoom("VIS$0")

	_, _ = store.RegisterDenizen("Alice", "pass123", "1", "hr=001/1", "Alice")
	_, _ = store.RegisterDenizen("Bob", "pass123", "2", "hr=002/2", "Bob")

	posAlice, _ := store.AddBackpackItem("Alice", 45, map[string]any{})
	posBob, _ := store.AddBackpackItem("Bob", 50, map[string]any{})

	pAlice := &Player{ID: "p_alice", ScreenName: "Alice", Authenticated: true, RoomID: rm.ID}
	pBob := &Player{ID: "p_bob", ScreenName: "Bob", Authenticated: true, RoomID: rm.ID}

	srv.mu.Lock()
	srv.Players["p_alice"] = pAlice
	srv.Players["p_bob"] = pBob
	rm.Players["p_alice"] = pAlice
	rm.Players["p_bob"] = pBob
	srv.mu.Unlock()

	// 1. Start trade and add items
	srv.handlePlayerMessage(pAlice, `<Room.Action roomID="VIS$0"><TD_ST tde="Bob"/></Room.Action>`)
	srv.handlePlayerMessage(pAlice, fmt.Sprintf(`<Room.Action roomID="VIS$0"><TD_AD id="%d"/></Room.Action>`, posAlice))
	srv.handlePlayerMessage(pBob, fmt.Sprintf(`<Room.Action roomID="VIS$0"><TD_AD id="%d"/></Room.Action>`, posBob))
	srv.handlePlayerMessage(pAlice, `<Room.Action roomID="VIS$0"><TD_AG/></Room.Action>`)

	// 2. Alice abruptly disconnects mid-trade
	srv.tradeManager.AbortTradeForPlayer("Alice")

	// Trade session should be canceled for Bob as well
	if srv.tradeManager.GetSession("Bob") != nil {
		t.Fatalf("Trade session should be cancelled on player disconnect")
	}

	// 3. Check that zero items were lost or transferred
	itAlice, err := store.GetBackpackItem("Alice", posAlice)
	if err != nil || itAlice == nil {
		t.Fatalf("Alice should retain posAlice in backpack: %v", err)
	}

	itBob, err := store.GetBackpackItem("Bob", posBob)
	if err != nil || itBob == nil {
		t.Fatalf("Bob should retain posBob in backpack: %v", err)
	}
}

func TestTradePreservesSongAuthor(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	defer store.Close()

	srv := NewServer(".", "127.0.0.1", "8080", store)
	rm := srv.GetOrCreateRoom("VIS$0")

	_, _ = store.RegisterDenizen("Alice", "pass123", "1", "hr=001/1", "Alice")
	_, _ = store.RegisterDenizen("Bob", "pass123", "2", "hr=002/2", "Bob")

	// Alice burns a CD: author/artist is Alice
	cdProps := map[string]any{
		"songId":    float64(10),
		"songName":  "Alice Sunset",
		"ownerName": "Alice",
		"artist":    "Alice",
		"edl":       "^50,sample1{1,0{^",
	}
	posCD, err := store.AddBackpackItem("Alice", 89, cdProps)
	if err != nil {
		t.Fatalf("AddBackpackItem CD failed: %v", err)
	}

	// Bob has a blank CD or furniture item
	posBobItem, err := store.AddBackpackItem("Bob", 92, map[string]any{})
	if err != nil {
		t.Fatalf("AddBackpackItem Bob failed: %v", err)
	}

	pAlice := &Player{ID: "p_alice", ScreenName: "Alice", Authenticated: true, RoomID: rm.ID}
	pBob := &Player{ID: "p_bob", ScreenName: "Bob", Authenticated: true, RoomID: rm.ID}

	srv.mu.Lock()
	srv.Players["p_alice"] = pAlice
	srv.Players["p_bob"] = pBob
	rm.Players["p_alice"] = pAlice
	rm.Players["p_bob"] = pBob
	srv.mu.Unlock()

	// Alice and Bob trade
	srv.handlePlayerMessage(pAlice, `<Room.Action roomID="VIS$0"><TD_ST tde="Bob"/></Room.Action>`)
	srv.handlePlayerMessage(pAlice, fmt.Sprintf(`<Room.Action roomID="VIS$0"><TD_AD id="%d"/></Room.Action>`, posCD))
	srv.handlePlayerMessage(pBob, fmt.Sprintf(`<Room.Action roomID="VIS$0"><TD_AD id="%d"/></Room.Action>`, posBobItem))

	srv.handlePlayerMessage(pAlice, `<Room.Action roomID="VIS$0"><TD_AG/></Room.Action>`)
	srv.handlePlayerMessage(pBob, `<Room.Action roomID="VIS$0"><TD_AG/></Room.Action>`)

	// Bob now owns the CD
	tradedCD, err := store.GetBackpackItem("Bob", posCD)
	if err != nil || tradedCD == nil {
		t.Fatalf("Expected Bob to own traded CD: %v", err)
	}

	// OwnerName must be Bob
	if tradedCD.Properties["ownerName"] != "Bob" {
		t.Fatalf("Expected CD ownerName to be Bob, got %v", tradedCD.Properties["ownerName"])
	}

	// Artist MUST strictly remain Alice (original author preserved!)
	if tradedCD.Properties["artist"] != "Alice" {
		t.Fatalf("Expected CD artist to remain Alice, got %v", tradedCD.Properties["artist"])
	}
}
