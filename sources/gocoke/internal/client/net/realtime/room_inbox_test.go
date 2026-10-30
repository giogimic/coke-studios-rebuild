package realtime

import "testing"

func TestRoomSnapshotWaitsForItsScene(t *testing.T) {
	var q roomInbox
	q.begin("London")
	for _, packet := range []string{"Alice spawn", "Bob spawn", "Bob moves", "DJ starts"} {
		if !q.deferPacket("London", "", []byte(packet)) {
			t.Fatal("room packet delivered before scene exists")
		}
	}
	if got := q.drain("old studio"); len(got) != 0 {
		t.Fatal("snapshot sent to previous scene")
	}
	got := q.drain("London")
	if len(got) != 4 || string(got[0]) != "Alice spawn" || string(got[3]) != "DJ starts" {
		t.Fatalf("missing or reordered room snapshot: %q", got)
	}
	if len(q.drain("London")) != 0 {
		t.Fatal("snapshot replayed twice")
	}
	if q.deferPacket("London", "London", []byte("chat")) {
		t.Fatal("ready scene failed to receive live chat")
	}
}

func TestRoomTransitionDiscardsStaleEvents(t *testing.T) {
	var q roomInbox
	q.begin("A")
	q.deferPacket("A", "", []byte("old avatar"))
	q.begin("B")
	if !q.deferPacket("A", "A", []byte("late old avatar")) {
		t.Fatal("old-room event escaped during transition")
	}
	q.deferPacket("B", "A", []byte("new avatar"))
	// A live packet must not overtake the queued snapshot as the scene activates.
	q.deferPacket("B", "B", []byte("new movement"))
	got := q.drain("B")
	if len(got) != 2 || string(got[0]) != "new avatar" || string(got[1]) != "new movement" {
		t.Fatalf("mixed rooms/order: %q", got)
	}
}
