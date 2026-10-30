package main

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestGoClientDJProtocolUsesOwnedCD(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "dj.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := NewServer("../../web", "127.0.0.1", "8080", store)
	_, err = store.RegisterDenizen("DJProtocol", "Password123!", "1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	songID, err := store.SaveSong("DJProtocol", "A & B", "^50,rockOne_guitar00{8,0{^")
	if err != nil {
		t.Fatal(err)
	}
	cdID, err := store.AddBackpackItem("DJProtocol", 89, map[string]any{"songId": songID})
	if err != nil {
		t.Fatal(err)
	}
	player, writer := createTestPlayer(srv, "dj", "DJProtocol", "")
	srv.handlePlayerMessage(player, `<Room.Enter msgID="2" roomID="$SYSTEM$1"/>`)
	srv.handlePlayerMessage(player, `<Room.Action roomID="$SYSTEM$1"><CD_R cdPlId="public:london:dj"/></Room.Action>`)
	if !writer.HasMessage("<CD_RA") {
		t.Fatal("clicking DJ table did not open song chooser")
	}
	srv.handlePlayerMessage(player, fmt.Sprintf(`<Room.Action roomID="$SYSTEM$1"><CD_P cdId="%d"/></Room.Action>`, cdID))
	rm, _ := srv.GetRoom("$SYSTEM$1")
	if rm.Performance == nil {
		t.Fatal("Go client's cdId packet did not start stored composition")
	}
	if rm.Performance.SongTitle != "A & B" {
		t.Fatal("composition title lost")
	}
	if !writer.HasMessage(`sn="A &amp; B"`) {
		t.Fatal("playback metadata not XML escaped")
	}
	// A second start must not replace the current song.
	first := rm.Performance
	srv.handlePlayerMessage(player, fmt.Sprintf(`<Room.Action roomID="$SYSTEM$1"><CD_P cdId="%d"/></Room.Action>`, cdID))
	if rm.Performance != first {
		t.Fatal("active performance replaced")
	}
	close(first.cancelCh)
}

func TestGoClientDJRejectsUnownedCDAndRawSequence(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "dj.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := NewServer("../../web", "127.0.0.1", "8080", store)
	player, writer := createTestPlayer(srv, "stranger", "Stranger", "")
	srv.handlePlayerMessage(player, `<Room.Enter msgID="2" roomID="$SYSTEM$1"/>`)
	srv.handlePlayerMessage(player, `<Room.Action roomID="$SYSTEM$1"><CD_R cdPlId="fake"/></Room.Action>`)
	if writer.HasMessage("<CD_RA") {
		t.Fatal("fake player accepted")
	}
	srv.handlePlayerMessage(player, `<Room.Action roomID="$SYSTEM$1"><CD_P cdId="999999"/></Room.Action>`)
	srv.handlePlayerMessage(player, `<Room.Action roomID="$SYSTEM$1"><CD_P s="^50,synth{8,0{^" title="forged"/></Room.Action>`)
	rm, _ := srv.GetRoom("$SYSTEM$1")
	if rm.Performance != nil {
		close(rm.Performance.cancelCh)
		t.Fatal("unowned CD or arbitrary sequence started a performance")
	}
}

func TestMultiplayerDJPerformanceAndVotingSync(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "dj_sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := NewServer("../../web", "127.0.0.1", "8080", store)

	// 1. Register Alice and Bob
	_, err = store.RegisterDenizen("Alice", "Pass123!", "1", "hr=001/1", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.RegisterDenizen("Bob", "Pass123!", "1", "hr=002/2", "Bob")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Alice composes and burns a CD
	songID, err := store.SaveSong("Alice", "Groove Track", "^50,rockOne_guitar00{8,0{^")
	if err != nil {
		t.Fatal(err)
	}
	cdID, err := store.AddBackpackItem("Alice", 89, map[string]any{"songId": songID})
	if err != nil {
		t.Fatal(err)
	}

	// 3. Connect Alice and Bob into the same room
	pAlice, wAlice := createTestPlayer(srv, "socket_alice", "Alice", "")
	pBob, wBob := createTestPlayer(srv, "socket_bob", "Bob", "")

	srv.handlePlayerMessage(pAlice, `<Room.Enter msgID="2" roomID="$SYSTEM$1"/>`)
	srv.handlePlayerMessage(pBob, `<Room.Enter msgID="2" roomID="$SYSTEM$1"/>`)

	// 4. Alice reserves DJ table and plays CD
	srv.handlePlayerMessage(pAlice, `<Room.Action roomID="$SYSTEM$1"><CD_R cdPlId="public:london:dj"/></Room.Action>`)
	srv.handlePlayerMessage(pAlice, fmt.Sprintf(`<Room.Action roomID="$SYSTEM$1"><CD_P cdId="%d"/></Room.Action>`, cdID))

	// 5. Bob must receive broadcast CD_P packet for Alice's song
	if !wBob.HasMessage(`CD_P av="Alice"`) {
		t.Fatalf("Bob did not receive broadcast CD_P packet: %v", wBob.GetMessages())
	}

	// 6. Bob votes Thumbs Up
	srv.handlePlayerMessage(pBob, `<Room.Action roomID="$SYSTEM$1"><CD_TU/></Room.Action>`)

	// 7. Both Alice and Bob must receive the vote reaction update CD_VR with tu="1"
	if !wAlice.HasMessage(`CD_VR tu="1"`) {
		t.Fatalf("Alice did not receive CD_VR cheer update: %v", wAlice.GetMessages())
	}
	if !wBob.HasMessage(`CD_VR tu="1"`) {
		t.Fatalf("Bob did not receive CD_VR cheer update: %v", wBob.GetMessages())
	}

	rm, _ := srv.GetRoom("$SYSTEM$1")
	if rm.Performance != nil {
		close(rm.Performance.cancelCh)
	}
}
