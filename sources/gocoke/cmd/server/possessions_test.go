package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPlacementRequiresOwnership(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "placement_ownership.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	srv := NewServer("./web", "127.0.0.1", "8080", store)

	// Create user Bob and his studio
	bob, err := srv.store.GetOrCreateDenizen("BobBuilder", "1", "avatar_bob", "Builder")
	if err != nil {
		t.Fatalf("failed to create Bob: %v", err)
	}
	studio, err := srv.store.CreateStudio("studio_bob", bob.ID, bob.ScreenName, "Bob's Studio", "studio_model_c", 3)
	if err != nil {
		t.Fatalf("failed to create studio: %v", err)
	}

	// Bob attempts to place possession ID "99999" (not in his backpack)
	err = srv.store.PlaceBackpackItem(studio.ID, bob.ScreenName, "99999", 13, 5, 0, 5, 0, 2, 0, false, nil)
	if err == nil {
		t.Fatalf("expected placement of unowned item 99999 to fail, but succeeded")
	}
	if err != ErrItemNotOwned {
		t.Fatalf("expected ErrItemNotOwned, got: %v", err)
	}

	// Verify room remains empty
	furni, _ := srv.store.GetRoomFurniture(studio.ID)
	if len(furni) != 0 {
		t.Fatalf("expected 0 furniture items in room, got %d", len(furni))
	}
}

func TestPlacementRequiresStudioOwnership(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "studio_ownership.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	srv := NewServer("./web", "127.0.0.1", "8080", store)

	// Alice owns studio_alice
	alice, _ := srv.store.GetOrCreateDenizen("Alice", "1", "avatar_alice", "")
	studioAlice, _ := srv.store.CreateStudio("studio_alice", alice.ID, alice.ScreenName, "Alice's Studio", "studio_model_c", 3)

	// Eve buys a sofa
	eve, _ := srv.store.GetOrCreateDenizen("Eve", "1", "avatar_eve", "")
	posArr, err := srv.Purchase(eve.ScreenName, 13, nil, "")
	if err != nil {
		t.Fatalf("Eve purchase failed: %v", err)
	}
	evePosID := fmt.Sprintf("%v", int(posArr[0].(map[string]any)["id"].(float64)))

	// Eve tries to place her sofa in Alice's studio
	err = srv.store.PlaceBackpackItem(studioAlice.ID, eve.ScreenName, evePosID, 13, 5, 0, 5, 0, 2, 0, false, nil)
	if err == nil {
		t.Fatalf("expected Eve's placement in Alice's studio to fail, but succeeded")
	}
	if err != ErrNotStudioOwner {
		t.Fatalf("expected ErrNotStudioOwner, got: %v", err)
	}

	// Verify Alice's studio is still empty
	furni, _ := srv.store.GetRoomFurniture(studioAlice.ID)
	if len(furni) != 0 {
		t.Fatalf("expected 0 furniture items in Alice's room, got %d", len(furni))
	}

	// Eve's item must still be in her backpack
	bp, _ := srv.store.GetBackpack(eve.ScreenName)
	if len(bp) != 1 {
		t.Fatalf("expected Eve to still have 1 item in backpack, got %d", len(bp))
	}
}

func TestPlacementAndPickupAtomicTransfer(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "atomic_transfer.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	srv := NewServer("./web", "127.0.0.1", "8080", store)

	user, _ := srv.store.GetOrCreateDenizen("Decorator", "1", "avatar_dec", "")
	studio, _ := srv.store.CreateStudio("studio_dec", user.ID, user.ScreenName, "Decor Studio", "studio_model_c", 3)

	// Buy a 5001 Sofa (item 13)
	posArr, err := srv.Purchase(user.ScreenName, 13, map[string]any{"color": "blue"}, "")
	if err != nil {
		t.Fatalf("purchase failed: %v", err)
	}
	posID := strconv.Itoa(int(posArr[0].(map[string]any)["id"].(float64)))

	// Check backpack has 1 item
	bp1, _ := srv.store.GetBackpack(user.ScreenName)
	if len(bp1) != 1 {
		t.Fatalf("expected 1 item in backpack, got %d", len(bp1))
	}

	// 1. Place in studio: must transfer from backpack to room
	err = srv.store.PlaceBackpackItem(studio.ID, user.ScreenName, posID, 13, 6, 0, 7, 0, 2, 0, false, nil)
	if err != nil {
		t.Fatalf("placement failed: %v", err)
	}

	bp2, _ := srv.store.GetBackpack(user.ScreenName)
	if len(bp2) != 0 {
		t.Fatalf("expected backpack to be empty after placement, got %d items", len(bp2))
	}

	roomFurni1, _ := srv.store.GetRoomFurniture(studio.ID)
	if len(roomFurni1) != 1 {
		t.Fatalf("expected 1 item placed in room, got %d", len(roomFurni1))
	}
	if roomFurni1[0].PosID != posID || roomFurni1[0].X != 6 || roomFurni1[0].Z != 7 {
		t.Fatalf("placed item attributes mismatch: %+v", roomFurni1[0])
	}

	// 2. Rotate item in place (should update coords/dir without requiring backpack)
	err = srv.store.PlaceBackpackItem(studio.ID, user.ScreenName, posID, 13, 6, 0, 7, 0, 4, 1, false, nil)
	if err != nil {
		t.Fatalf("rotation failed: %v", err)
	}
	roomFurniRot, _ := srv.store.GetRoomFurniture(studio.ID)
	if roomFurniRot[0].Dir != 4 || roomFurniRot[0].State != 1 {
		t.Fatalf("expected dir=4 state=1, got dir=%d state=%d", roomFurniRot[0].Dir, roomFurniRot[0].State)
	}

	// 3. Pick up back to backpack
	err = srv.store.PickupFurnitureToBackpack(studio.ID, user.ScreenName, posID)
	if err != nil {
		t.Fatalf("pickup failed: %v", err)
	}

	roomFurni2, _ := srv.store.GetRoomFurniture(studio.ID)
	if len(roomFurni2) != 0 {
		t.Fatalf("expected room to be empty after pickup, got %d items", len(roomFurni2))
	}

	bp3, _ := srv.store.GetBackpack(user.ScreenName)
	if len(bp3) != 1 {
		t.Fatalf("expected backpack to contain 1 item after pickup, got %d", len(bp3))
	}
	if bp3[0].CatalogItemID != 13 {
		t.Fatalf("expected catalog item 13 in backpack, got %d", bp3[0].CatalogItemID)
	}
}

func TestPossessionsSurviveRestart(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "restart_persistence.db")

	var userScreenName string
	var studioID string
	var placedPosID string

	// Phase 1: Setup, purchase, and place item
	{
		store, err := OpenStore(dbPath)
		if err != nil {
			t.Fatalf("Phase 1: failed to open store: %v", err)
		}

		srv := NewServer("./web", "127.0.0.1", "8080", store)
		user, err := srv.store.GetOrCreateDenizen("PersistentPlayer", "1", "avatar_p", "Survival Test")
		if err != nil {
			t.Fatalf("Phase 1: failed to create denizen: %v", err)
		}
		userScreenName = user.ScreenName

		st, err := srv.store.CreateStudio("studio_persist", user.ID, user.ScreenName, "Persist Room", "studio_model_c", 3)
		if err != nil {
			t.Fatalf("Phase 1: failed to create studio: %v", err)
		}
		studioID = st.ID

		// Buy 5001 Sofa (item 13)
		posArr, err := srv.Purchase(user.ScreenName, 13, map[string]any{"texture": 3}, "")
		if err != nil {
			t.Fatalf("Phase 1: purchase failed: %v", err)
		}
		placedPosID = strconv.Itoa(int(posArr[0].(map[string]any)["id"].(float64)))

		// Place in studio at (8, 0, 9, dir=6)
		err = srv.store.PlaceBackpackItem(studioID, userScreenName, placedPosID, 13, 8, 0, 9, 0, 6, 0, false, nil)
		if err != nil {
			t.Fatalf("Phase 1: placement failed: %v", err)
		}

		// Close Phase 1 store (simulating server shutdown)
		store.Close()
	}

	// Phase 2: Server restarts. Verify placed item persisted exactly!
	{
		store, err := OpenStore(dbPath)
		if err != nil {
			t.Fatalf("Phase 2: failed to reopen store: %v", err)
		}

		furni, err := store.GetRoomFurniture(studioID)
		if err != nil {
			t.Fatalf("Phase 2: failed to get room furniture: %v", err)
		}
		if len(furni) != 1 {
			t.Fatalf("Phase 2: expected 1 furniture item in room after restart, got %d", len(furni))
		}
		if furni[0].PosID != placedPosID || furni[0].CatalogItemID != 13 || furni[0].X != 8 || furni[0].Z != 9 || furni[0].Dir != 6 {
			t.Fatalf("Phase 2: furniture state corrupted after restart: %+v", furni[0])
		}

		bp, err := store.GetBackpack(userScreenName)
		if err != nil {
			t.Fatalf("Phase 2: failed to get backpack: %v", err)
		}
		if len(bp) != 0 {
			t.Fatalf("Phase 2: expected backpack to be empty, got %d items", len(bp))
		}

		// Pick up item back to backpack
		err = store.PickupFurnitureToBackpack(studioID, userScreenName, placedPosID)
		if err != nil {
			t.Fatalf("Phase 2: pickup failed: %v", err)
		}

		// Close Phase 2 store
		store.Close()
	}

	// Phase 3: Server restarts again. Verify backpack item persisted!
	{
		store, err := OpenStore(dbPath)
		if err != nil {
			t.Fatalf("Phase 3: failed to reopen store: %v", err)
		}
		defer store.Close()

		furni, _ := store.GetRoomFurniture(studioID)
		if len(furni) != 0 {
			t.Fatalf("Phase 3: expected room to be empty, got %d items", len(furni))
		}

		bp, err := store.GetBackpack(userScreenName)
		if err != nil {
			t.Fatalf("Phase 3: failed to get backpack: %v", err)
		}
		if len(bp) != 1 {
			t.Fatalf("Phase 3: expected 1 item in backpack after restart, got %d", len(bp))
		}
		if bp[0].CatalogItemID != 13 {
			t.Fatalf("Phase 3: expected catalog item 13 in backpack, got %d", bp[0].CatalogItemID)
		}
	}
}
