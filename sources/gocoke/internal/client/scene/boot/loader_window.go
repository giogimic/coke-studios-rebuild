package boot

// Note: Option C selected by user — plan is to pre-render loading.swf HTML5
// animation into a sprite sheet or frame sequence, then play in Ebiten.
// TODO: Implement a tiny frame player once the frames are available.

import (
	lingoassets "gocoke/internal/client/assets/lingo"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
)

// loaderWindow draws Lingo general_loader.window using ripped bitmap slices from
// Lingo/cc_interface[1]. This is a simplified renderer that uses absolute
// locH/locV and piece dimensions present in the .window.txt.

type loaderWindow struct {
	parts map[string]*ebiten.Image
}

func newLoaderWindow(dir string) *loaderWindow {
	// dir is path to Lingo/cc_interface[1]
	mm, err := lingoassets.LoadMemberMap(dir)
	if err != nil {
		log.Printf("loaderWindow: member map: %v", err)
	}
	load := func(name string) *ebiten.Image {
		if p, ok := mm.PathFor(name); ok {
			img, err := lingoassets.LoadImage(p)
			if err == nil {
				return img
			}
			log.Printf("loaderWindow: load %s: %v", name, err)
		}
		return ebiten.NewImage(1, 1)
	}
	lw := &loaderWindow{parts: map[string]*ebiten.Image{
		"cc.content.top.left":      load("cc.content.top.left"),
		"cc.content.top.middle":    load("cc.content.top.middle"),
		"cc.content.top.right":     load("cc.content.top.right"),
		"cc.content.middle.left":   load("cc.content.middle.left"),
		"cc.content.middle.middle": load("cc.content.middle.middle"),
		"cc.content.middle.right":  load("cc.content.middle.right"),
		"cc.content.bottom.left":   load("cc.content.bottom.left"),
		"cc.content.bottom.middle": load("cc.content.bottom.middle"),
		"cc.content.bottom.right":  load("cc.content.bottom.right"),
	}}
	return lw
}

func (w *loaderWindow) Draw(screen *ebiten.Image) {
	w.DrawAt(screen, 40, 40) // Default position for backward compatibility
}

func (w *loaderWindow) DrawAt(screen *ebiten.Image, x, y float64) {
	// Compose a 298x94 window starting at rect [206,156,495,250] per general_loader.window
	canvas := ebiten.NewImage(298, 94)
	// Tile slices at fixed positions according to the window definition
	drawAt := func(img *ebiten.Image, px, py int) {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(px), float64(py))
		canvas.DrawImage(img, op)
	}
	drawAt(w.parts["cc.content.top.left"], 0, 0)
	drawAt(w.parts["cc.content.top.middle"], 9, 0)
	drawAt(w.parts["cc.content.top.right"], 280, 0)
	drawAt(w.parts["cc.content.middle.left"], 0, 9)
	drawAt(w.parts["cc.content.middle.middle"], 9, 9)
	drawAt(w.parts["cc.content.middle.right"], 280, 9)
	drawAt(w.parts["cc.content.bottom.left"], 0, 85)
	drawAt(w.parts["cc.content.bottom.middle"], 9, 85)
	drawAt(w.parts["cc.content.bottom.right"], 280, 85)
	// Blit to screen at specified position
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(x, y)
	screen.DrawImage(canvas, op)
}
