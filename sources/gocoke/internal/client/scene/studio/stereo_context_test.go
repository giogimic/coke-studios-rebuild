package studio

import "testing"

func TestStereoPlayersShareAudioContext(t *testing.T) {
	first := NewStereoPlayer()
	second := NewStereoPlayer()
	if first.ctx != second.ctx {
		t.Fatal("room changes created separate Ebitengine audio contexts")
	}
}
