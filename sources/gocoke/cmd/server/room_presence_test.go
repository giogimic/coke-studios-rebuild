package main

import (
	"encoding/hex"
	"fmt"
	"gocoke/internal/client/net/realtime"
	"path/filepath"
	"strings"
	"testing"
)

func presenceServer(t *testing.T) *Server {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "presence.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewServer("../../web", "localhost", "8080", store)
}

func loginPresencePlayer(t *testing.T, srv *Server, name, def, mission, suppliedDef string) (*Player, *testPacketWriter) {
	t.Helper()
	d, err := srv.store.RegisterDenizen(name, "Password123!", "1", def, mission)
	if err != nil {
		t.Fatal(err)
	}
	p, w := createTestPlayer(srv, "socket_"+name, "", "")
	p.Authenticated = false
	srv.handlePlayerMessage(p, fmt.Sprintf(`<Login username="%s" secret="%s" def="%s"/>`, name, d.Secret, suppliedDef))
	if !p.Authenticated {
		t.Fatal("login rejected")
	}
	return p, w
}

func TestRealtimeLoginKeepsSavedAppearance(t *testing.T) {
	srv := presenceServer(t)
	saved := "hd=001/115,92,69&hr=017/238,238,238"
	p, _ := loginPresencePlayer(t, srv, "AppearanceOwner", saved, "", "stale-browser-look")
	if p.Definition != saved {
		t.Fatalf("login replaced saved appearance: %q", p.Definition)
	}
}

func TestAvatarSpawnKeepsSavedAppearance(t *testing.T) {
	srv := presenceServer(t)
	saved := "hd=001/115,92,69&hr=017/238,238,238"
	p, _ := loginPresencePlayer(t, srv, "SpawnOwner", saved, "", "")
	srv.handlePlayerMessage(p, `<Room.Enter msgID="2" roomID="$SYSTEM$1"/>`)
	srv.handlePlayerMessage(p, `<Room.Action roomID="$SYSTEM$1"><AV_EN def="stale-browser-look"/></Room.Action>`)
	if p.Definition != saved {
		t.Fatalf("spawn replaced saved appearance: %q", p.Definition)
	}
}

func TestAvatarPacketsPreserveProfileForSpawnAndLateJoin(t *testing.T) {
	srv := presenceServer(t)
	def := "hd=001/115,92,69&hr=017/238,238,238"
	mission := `Mix "A&B" <live> &amp;`
	alice, aliceWriter := loginPresencePlayer(t, srv, "ProfileAlice", def, mission, "")
	srv.handlePlayerMessage(alice, `<Room.Enter msgID="2" roomID="$SYSTEM$1"/>`)
	srv.handlePlayerMessage(alice, `<Room.Action roomID="$SYSTEM$1"><AV_EN/></Room.Action>`)
	bob, bobWriter := loginPresencePlayer(t, srv, "ProfileBob", "hd=001/1,2,3&hr=002/4,5,6", "", "")
	srv.handlePlayerMessage(bob, `<Room.Enter msgID="2" roomID="$SYSTEM$1"/>`)
	for _, tc := range []struct {
		name string
		w    *testPacketWriter
	}{{"spawn", aliceWriter}, {"late join", bobWriter}} {
		t.Run(tc.name, func(t *testing.T) {
			found := false
			for _, packet := range tc.w.GetMessages() {
				if !strings.Contains(packet, `<AV av="ProfileAlice"`) {
					continue
				}
				decoded, err := realtime.DecodeAvatarPackets([]byte(packet))
				if err != nil {
					t.Fatalf("invalid avatar XML: %v", err)
				}
				if len(decoded) != 1 || decoded[0].Definition != def || decoded[0].Mission != mission {
					t.Fatalf("profile changed on wire: %+v", decoded)
				}
				found = true
			}
			if !found {
				t.Fatal("Alice absent from avatar packets")
			}
		})
	}
}

func TestInvalidRoomTransitionPreservesPresence(t *testing.T) {
	srv := presenceServer(t)
	alice, _ := createTestPlayer(srv, "socket_alice", "Alice", "")
	bob, bobWriter := createTestPlayer(srv, "socket_bob", "Bob", "")
	if err := srv.enterPlayerRoom(alice, "$SYSTEM$1", "2"); err != nil {
		t.Fatal(err)
	}
	if err := srv.enterPlayerRoom(bob, "$SYSTEM$1", "2"); err != nil {
		t.Fatal(err)
	}
	if err := srv.enterPlayerRoom(alice, "missing_studio", "3"); err == nil {
		t.Fatal("invalid room accepted")
	}
	rm, _ := srv.GetRoom("$SYSTEM$1")
	if rm.Players[alice.ID] != alice || alice.RoomID != rm.ID {
		t.Fatal("rejected transition removed Alice from existing room")
	}
	if bobWriter.HasMessage("<AV_RM") {
		t.Fatal("observer was incorrectly told Alice left")
	}
}

func TestPlayerIDRemainsStableConnectionKey(t *testing.T) {
	srv := presenceServer(t)
	d, err := srv.store.RegisterDenizen("StableKeyUser", "Secret123!", "1", "hr=001/1&hd=001/1", "Mission")
	if err != nil {
		t.Fatal(err)
	}
	connID := "socket_unique_conn_999"
	p, _ := createTestPlayer(srv, connID, "", "")
	p.Authenticated = false
	srv.handlePlayerMessage(p, fmt.Sprintf(`<Login username="StableKeyUser" secret="%s"/>`, d.Secret))
	if !p.Authenticated {
		t.Fatal("login rejected")
	}
	if p.ID != connID {
		t.Fatalf("expected player.ID to remain stable connection key %q, got %q", connID, p.ID)
	}
	if p.DenizenID != d.ID {
		t.Fatalf("expected player.DenizenID %q, got %q", d.ID, p.DenizenID)
	}
	srv.mu.RLock()
	mappedPlayer := srv.Players[connID]
	srv.mu.RUnlock()
	if mappedPlayer != p {
		t.Fatalf("srv.Players[%q] does not point to player", connID)
	}
}

func TestConcurrentRoomEntrantsSeeEachOther(t *testing.T) {
	srv := presenceServer(t)
	alice, aliceWriter := loginPresencePlayer(t, srv, "Alice", "hr=001/1", "Alice Mission", "")
	bob, bobWriter := loginPresencePlayer(t, srv, "Bob", "hr=002/2", "Bob Mission", "")

	// Alice enters room first
	if err := srv.enterPlayerRoom(alice, "$SYSTEM$1", "1"); err != nil {
		t.Fatal(err)
	}

	// Bob enters room
	if err := srv.enterPlayerRoom(bob, "$SYSTEM$1", "2"); err != nil {
		t.Fatal(err)
	}

	// Bob must receive Alice's presence
	if !bobWriter.HasMessage(`<AV av="Alice"`) {
		t.Fatalf("Bob did not receive Alice's presence packet, messages: %v", bobWriter.GetMessages())
	}
	// Alice must receive Bob's presence
	if !aliceWriter.HasMessage(`<AV av="Bob"`) {
		t.Fatalf("Alice did not receive Bob's presence packet, messages: %v", aliceWriter.GetMessages())
	}
}

func TestMultiplayerMovementAndChatExchange(t *testing.T) {
	srv := presenceServer(t)
	alice, aliceWriter := loginPresencePlayer(t, srv, "Alice", "hr=001/1", "Alice Mission", "")
	bob, bobWriter := loginPresencePlayer(t, srv, "Bob", "hr=002/2", "Bob Mission", "")

	_ = srv.enterPlayerRoom(alice, "$SYSTEM$1", "1")
	_ = srv.enterPlayerRoom(bob, "$SYSTEM$1", "2")

	// Alice moves to (12, 6)
	moveXML := `<Room.Action roomID="$SYSTEM$1"><AV_GSP><P x="12" z="5"/><P x="12" z="6"/></AV_GSP></Room.Action>`
	srv.handlePlayerMessage(alice, moveXML)

	// Bob must receive Alice's AV_MV packet
	if !bobWriter.HasMessage(`<AV_MV av="Alice" x="12" z="6"/>`) {
		t.Fatalf("Bob did not receive Alice's movement packet: %v", bobWriter.GetMessages())
	}

	// Bob shouts "Hey Alice!"
	hexMsg := hex.EncodeToString([]byte("Hey Alice!"))
	chatXML := fmt.Sprintf(`<Room.Action roomID="$SYSTEM$1"><CH sm="2" m="%s"/></Room.Action>`, hexMsg)
	srv.handlePlayerMessage(bob, chatXML)

	// Alice must receive Bob's chat packet
	if !aliceWriter.HasMessage(fmt.Sprintf(`<CH av="Bob" sm="2" m="%s"/>`, hexMsg)) {
		t.Fatalf("Alice did not receive Bob's chat packet: %v", aliceWriter.GetMessages())
	}

	// Alice leaves room
	srv.handlePlayerMessage(alice, `<Room.Exit msgID="3" roomID="$SYSTEM$1"/>`)

	// Bob must receive AV_RM packet for Alice
	if !bobWriter.HasMessage(`<AV_RM av="Alice"/>`) {
		t.Fatalf("Bob did not receive Alice's exit packet: %v", bobWriter.GetMessages())
	}
}
