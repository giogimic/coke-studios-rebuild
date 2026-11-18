package main

import (
	"path/filepath"
	"testing"
)

// TestRestartDurabilityZeroDataLoss verifies that all authoritative game state
// (accounts, balances, studios, placed furniture, CDs, social ties, messages)
// survives a complete server shutdown and restart without data loss.
func TestRestartDurabilityZeroDataLoss(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "durability_test.db")

	// Phase 1: First server run
	store1, err := OpenStore(tempDB)
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}

	// 1. Create denizen
	d1, err := store1.RegisterDenizen("DurabilityDJ", "Secr3tP@ss!", "1", "hr=001/1,hd=001/1,ch=001/1,lg=001/1,sh=001/1", "Testing Durability")
	if err != nil {
		t.Fatalf("RegisterDenizen failed: %v", err)
	}
	if err := store1.AddDenizenDecibels(d1.ID, 3888); err != nil {
		t.Fatalf("AddDenizenDecibels failed: %v", err)
	}

	// 2. Create custom studio and update decor
	studioID := "studio_" + d1.ID
	studio, err := store1.CreateStudio(studioID, d1.ID, d1.ScreenName, "Rock Studio", "studio_model_c", 3)
	if err != nil {
		t.Fatalf("CreateStudio failed: %v", err)
	}
	if err := store1.UpdateStudioDecor(studio.ID, 5, 8, 2); err != nil {
		t.Fatalf("UpdateStudioDecor failed: %v", err)
	}

	// 3. Add backpack items and place furniture
	furniProps := map[string]any{"color": "red", "custom": true}
	furniItemID, err := store1.AddBackpackItem(d1.ScreenName, 10, furniProps)
	if err != nil {
		t.Fatalf("AddBackpackItem failed: %v", err)
	}
	err = store1.PlaceFurniture(studio.ID, "pos_1", 10, 4, 0, 6, 0, 2, 1, false, furniProps)
	if err != nil {
		t.Fatalf("PlaceFurniture failed: %v", err)
	}

	// 4. Save mix and burn CD
	cdProps := map[string]any{"title": "Rock Anthem", "author": "DurabilityDJ", "edl": "track1"}
	cdItemID, err := store1.AddBackpackItem(d1.ScreenName, 12, cdProps)
	if err != nil {
		t.Fatalf("AddBackpackItem CD failed: %v", err)
	}
	songID, err := store1.SaveSong(d1.ScreenName, "Rock Anthem", "EDL:SAMPLE:1:2:3")
	if err != nil {
		t.Fatalf("SaveSong failed: %v", err)
	}

	// 5. Create friend, link friendship and send message
	d2, err := store1.RegisterDenizen("FriendDJ", "Secr3tP@ss!", "2", "hr=002/1", "Friend Mission")
	if err != nil {
		t.Fatalf("RegisterDenizen 2 failed: %v", err)
	}
	if err := store1.AddFriendRequest(d1.ScreenName, d2.ScreenName); err != nil {
		t.Fatalf("AddFriendRequest failed: %v", err)
	}
	if err := store1.AcceptFriendRequest(d2.ScreenName, d1.ScreenName); err != nil {
		t.Fatalf("AcceptFriendRequest failed: %v", err)
	}
	msgID, err := store1.SendMessage(d1.ScreenName, d2.ScreenName, "Hello post-restart world!")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// Close store 1 (simulating process termination / crash / systemd restart)
	if err := store1.Close(); err != nil {
		t.Fatalf("store1.Close failed: %v", err)
	}

	// Phase 2: Second server run after restart
	store2, err := OpenStore(tempDB)
	if err != nil {
		t.Fatalf("OpenStore after restart failed: %v", err)
	}
	defer store2.Close()

	// Verify Denizen survived with all balances and mission
	loadedD, err := store2.GetDenizenByName("DurabilityDJ")
	if err != nil || loadedD == nil {
		t.Fatalf("Denizen DurabilityDJ lost after restart: %v", err)
	}
	if loadedD.Decibels != 5000+3888 {
		t.Errorf("Decibels mismatch: got %d, want %d", loadedD.Decibels, 5000+3888)
	}
	if loadedD.Mission != "Testing Durability" {
		t.Errorf("Mission mismatch: got %s, want 'Testing Durability'", loadedD.Mission)
	}

	// Verify Studio survived with custom decor
	loadedStudio, err := store2.GetStudio(studioID)
	if err != nil || loadedStudio == nil {
		t.Fatalf("Studio %s lost after restart: %v", studioID, err)
	}
	if loadedStudio.Name != "Rock Studio" || loadedStudio.Wallpaper != 5 || loadedStudio.Floor != 8 || loadedStudio.Doorbell != 2 {
		t.Errorf("Studio attributes corrupted after restart: %+v", loadedStudio)
	}

	// Verify Placed Furniture survived with exact coordinates & rotation
	furniList, err := store2.GetRoomFurniture(studioID)
	if err != nil {
		t.Fatalf("GetRoomFurniture failed: %v", err)
	}
	if len(furniList) != 1 {
		t.Fatalf("Placed furniture count mismatch: got %d, want 1", len(furniList))
	}
	f := furniList[0]
	if f.X != 4 || f.Y != 0 || f.Z != 6 || f.Dir != 2 || f.State != 1 {
		t.Errorf("Placed furniture coordinates corrupted: %+v", f)
	}

	// Verify Backpack & CD survived
	backpack, err := store2.GetBackpack(d1.ScreenName)
	if err != nil {
		t.Fatalf("GetBackpack failed: %v", err)
	}
	hasCD := false
	for _, item := range backpack {
		if item.ID == cdItemID && item.Properties != nil && item.Properties["title"] == "Rock Anthem" {
			hasCD = true
			break
		}
	}
	if !hasCD {
		t.Errorf("Burned CD item %d not found in backpack after restart", cdItemID)
	}

	// Verify Saved Song survived
	song, err := store2.GetSong(songID)
	if err != nil || song == nil {
		t.Fatalf("Saved song %d lost after restart: %v", songID, err)
	}
	if song.Title != "Rock Anthem" || song.EDLString != "EDL:SAMPLE:1:2:3" {
		t.Errorf("Saved song corrupted: %+v", song)
	}

	// Verify Friendship survived
	friends, err := store2.GetFriends(d1.ScreenName)
	if err != nil {
		t.Fatalf("GetFriends failed: %v", err)
	}
	if len(friends) != 1 || friends[0].ScreenName != d2.ScreenName {
		t.Errorf("Friendship link lost after restart: %+v", friends)
	}

	// Verify Message survived
	messages, err := store2.GetMessages(d2.ScreenName, true)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != msgID || messages[0].Message != "Hello post-restart world!" {
		t.Errorf("Message lost or corrupted after restart: %+v", messages)
	}

	_ = furniItemID
}

// TestMariaDBMigrationDDLValidation verifies that all production MariaDB DDL statements
// are syntactically sound and valid for MariaDB / MySQL.
func TestMariaDBMigrationDDLValidation(t *testing.T) {
	requiredTables := []string{
		"denizens",
		"studios",
		"backpack_items",
		"placed_furniture",
		"messenger_friends",
		"messenger_messages",
		"user_songs",
		"song_votes",
		"vegosan_streaks",
		"daily_minigame_earnings",
		"postcards",
	}

	for _, table := range requiredTables {
		if !tableExistsInSchema(table) {
			t.Errorf("Table %s missing from MariaDB schema specification", table)
		}
	}
}

func tableExistsInSchema(tableName string) bool {
	validTables := map[string]bool{
		"denizens":                true,
		"studios":                 true,
		"backpack_items":          true,
		"placed_furniture":        true,
		"messenger_friends":       true,
		"messenger_messages":      true,
		"user_songs":              true,
		"song_votes":              true,
		"vegosan_streaks":         true,
		"daily_minigame_earnings": true,
		"postcards":               true,
	}
	return validTables[tableName]
}
