package studio

import (
	"bytes"
	"io"
	"testing"
)

func TestStreamMixerStopsAtEventBoundary(t *testing.T) {
	pcm := []byte{1, 0, 1, 0, 2, 0, 2, 0}
	m := &streamMixer{srcPCM: map[string]*trackSource{"clip": {pcm: pcm, loopBytes: 8}}, schedule: []scheduledEvent{{startBytes: 4, loops: 1, asset: "clip"}}, totalBytes: 20}
	got := make([]byte, 32)
	n, err := m.Read(got)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append(make([]byte, 4), pcm...), make([]byte, 8)...)
	if n != len(want) || !bytes.Equal(got[:n], want) {
		t.Fatalf("one-loop event leaked beyond its duration: n=%d pcm=%v", n, got[:n])
	}
	if n, err = m.Read(got); n != 0 || err != io.EOF {
		t.Fatalf("expected EOF, got %d %v", n, err)
	}
}

func TestStreamMixerSeeksToLateJoinPosition(t *testing.T) {
	pcm := []byte{1, 0, 1, 0, 2, 0, 2, 0, 3, 0, 3, 0}
	m := &streamMixer{srcPCM: map[string]*trackSource{"clip": {pcm: pcm, loopBytes: 12}}, schedule: []scheduledEvent{{loops: 1, asset: "clip"}}, totalBytes: 12}
	seeker, ok := any(m).(io.Seeker)
	if !ok {
		t.Fatal("streaming playback cannot seek for a late join")
	}
	if _, err := seeker.Seek(4, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 8)
	n, err := m.Read(got)
	if err != nil || n != 8 || !bytes.Equal(got, pcm[4:]) {
		t.Fatalf("late join restarted or misaligned PCM: %v %v", got, err)
	}
	if _, err = seeker.Seek(-4, io.SeekStart); err == nil {
		t.Fatal("negative seek accepted")
	}
}
