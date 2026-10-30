package realtime

import "testing"

func TestAvatarPacketsDistinguishPresenceAndActions(t *testing.T) {
	packets, err := DecodeAvatarPackets([]byte(`<Room.Action roomID="room"><AV av="A" x="0" z="0" def="hd=001/1,2,3&amp;hr=002/4,5,6" m="A&amp;B &amp;amp;"/><AV av="B" act="dnc" d="4"/><AV_MV av="C" x="1" z="2"/></Room.Action>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(packets) != 2 {
		t.Fatalf("expected two AVs, got %d", len(packets))
	}
	if packets[0].X == nil || packets[0].Z == nil || *packets[0].X != 0 || *packets[0].Z != 0 {
		t.Fatal("zero coordinate lost")
	}
	if packets[0].Definition != "hd=001/1,2,3&hr=002/4,5,6" || packets[0].Mission != "A&B &amp;" {
		t.Fatal("profile entities not decoded exactly once")
	}
	if packets[1].X != nil || packets[1].Z != nil || packets[1].Action != "dnc" || packets[1].Direction != 4 {
		t.Fatal("action-only packet treated as a spawn")
	}
	if _, err := DecodeAvatarPackets([]byte(`<Room.Action><AV def="broken&profile"/></Room.Action>`)); err == nil {
		t.Fatal("malformed profile silently accepted")
	}
}
