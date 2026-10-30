package studio

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"gocoke/internal/client/assets/studios"
	"image"
	"image/png"
	"os"
	"testing"
)

// Explicit fixture only: never runs in normal play or the default test suite.
func TestLondonRenderFixture(t *testing.T) {
	output := os.Getenv("GOCOKE_RENDER_SNAPSHOT")
	if output == "" {
		t.Skip("set GOCOKE_RENDER_SNAPSHOT to capture the isolated fixture")
	}
	ebiten.SetWindowTitle("Coke Studios render test")
	ebiten.SetWindowSize(1, 1)
	g := &renderFixture{output: output}
	if err := ebiten.RunGame(g); err != nil {
		t.Fatal(err)
	}
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("no rendered frame captured")
	}
}

type renderFixture struct {
	scene  *Scene
	output string
	saved  bool
	err    error
}

func (g *renderFixture) Update() error {
	if g.saved {
		return ebiten.Termination
	}
	if g.scene != nil {
		return nil
	}
	s, err := New("data", &studios.Studio{StudioID: "$SYSTEM$1", Name: "London I - isolated rendering fixture", Layout: 5, Owner: "$SYSTEM$"})
	if err != nil {
		return err
	}
	s.showStudioLoader = false
	s.SetCurrentUserName("FixtureAlice")
	s.UpsertAvatarAtGrid("FixtureAlice", "hr=001/1&hd=001/1&ch=001/1&lg=001/1&sh=001/1&g=1", "", 1, 2, 12, 26, false)
	s.UpsertAvatarAtGrid("FixtureBob", "hr=004/3&hd=003/1&ch=006/1&lg=002/1&sh=002/1&fa=002/1&ey=001/1&g=1", "", 1, 4, 13, 26, false)
	if len(s.avatars) != 2 {
		return fmt.Errorf("fixture did not create two avatars: %d", len(s.avatars))
	}
	g.scene = s
	return nil
}
func (g *renderFixture) Layout(int, int) (int, int) { return 760, 520 }
func (g *renderFixture) Draw(screen *ebiten.Image) {
	if g.scene == nil {
		return
	}
	g.scene.Draw(screen)
	if g.saved {
		return
	}
	pixels := make([]byte, 760*520*4)
	screen.ReadPixels(pixels)
	img := &image.RGBA{Pix: pixels, Stride: 760 * 4, Rect: image.Rect(0, 0, 760, 520)}
	f, err := os.Create(g.output)
	if err == nil {
		err = png.Encode(f, img)
		_ = f.Close()
	}
	g.err = err
	g.saved = true
}
