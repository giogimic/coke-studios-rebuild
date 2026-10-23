package ui

import (
	"fmt"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
)

// FPSCounter tracks and displays FPS information
type FPSCounter struct {
	lastTime    time.Time
	frameCount  int
	fps         float64
	updateTimer time.Time
	font        font.Face
}

// NewFPSCounter creates a new FPS counter
func NewFPSCounter() *FPSCounter {
	return &FPSCounter{
		lastTime:    time.Now(),
		updateTimer: time.Now(),
		font:        basicfont.Face7x13, // Use built-in font
	}
}

// Update should be called once per frame to track FPS
func (f *FPSCounter) Update() {
	now := time.Now()
	f.frameCount++
	
	// Update FPS calculation every 250ms for smooth display
	if now.Sub(f.updateTimer) >= 250*time.Millisecond {
		elapsed := now.Sub(f.lastTime)
		if elapsed > 0 {
			f.fps = float64(f.frameCount) / elapsed.Seconds()
		}
		f.lastTime = now
		f.frameCount = 0
		f.updateTimer = now
	}
}

// Draw renders the FPS counter in the top-left corner
func (f *FPSCounter) Draw(screen *ebiten.Image) {
	fpsText := fmt.Sprintf("FPS: %.1f", f.fps)
	
	// Draw with a semi-transparent black background for better readability
	textBounds := text.BoundString(f.font, fpsText)
	bgWidth := textBounds.Dx() + 8
	bgHeight := textBounds.Dy() + 6
	
	// Create background rectangle
	bgImg := ebiten.NewImage(bgWidth, bgHeight)
	bgImg.Fill(color.RGBA{0, 0, 0, 128}) // Semi-transparent black
	
	// Draw background
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(5, 5) // 5px margin from top-left corner
	screen.DrawImage(bgImg, opts)
	
	// Draw FPS text in white
	text.Draw(screen, fpsText, f.font, 9, 18, color.RGBA{255, 255, 255, 255})
}
