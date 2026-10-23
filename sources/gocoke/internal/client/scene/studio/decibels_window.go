package studio

import (
	"fmt"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"

	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/lingo/window"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/ui"
	"image/color"
)

// DecibelsWindow displays the "You got X decibels" message
type DecibelsWindow struct {
	// Window assets (loaded directly, not through window manager)
	shadowImg     *ebiten.Image
	textShadowImg *ebiten.Image
	backgroundImg *ebiten.Image
	textBgImg     *ebiten.Image

	// Position and size from window definition
	rect [4]int // x, y, x2, y2 from window definition

	// Element positions (relative to window)
	shadowPos     [2]int
	textShadowPos [2]int
	bgPos         [2]int
	textBgPos     [2]int
	textRect      [4]int // x, y, width, height relative to window

	// State
	decibels int
	message  string
	visible  bool

	// Font
	font font.Face

	// Dependencies
	tm *textmgr.Manager
}

// NewDecibelsWindow creates a new decibels display window
func NewDecibelsWindow(tm *textmgr.Manager) *DecibelsWindow {
	return &DecibelsWindow{
		tm: tm,
	}
}

// LoadAssets loads the window assets based on cc_capdecibels.window.txt
func (dw *DecibelsWindow) LoadAssets() error {
	// Load the window definition file to get positions
	windowPath := filepath.Join("Lingo", "cc_interface[1]", "298_cc_capdecibels.window.txt")
	def, err := window.ParseFile(windowPath)
	if err != nil {
		return fmt.Errorf("failed to parse decibels window: %w", err)
	}

	// Store window rect
	if len(def.Rect) == 4 {
		dw.rect = def.Rect
	} else {
		// Default position if not specified
		dw.rect = [4]int{310, 198, 443, 333}
	}

	// Load assets from cc_interface[1] directory
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, interfaceDir)

	// Parse element positions from definition and assign assets
	for _, el := range def.Elements {
		switch el.Member {
		case "cc.capdecibels.shadow":
			dw.shadowPos = [2]int{el.LocH, el.LocV}
			if img, ok := assets[el.Member]; ok {
				dw.shadowImg = img
			}
		case "cc.capdecibels.textbg.shadow":
			dw.textShadowPos = [2]int{el.LocH, el.LocV}
			if img, ok := assets[el.Member]; ok {
				dw.textShadowImg = img
			}
		case "cc.capdecibels.bg":
			dw.bgPos = [2]int{el.LocH, el.LocV}
			if img, ok := assets[el.Member]; ok {
				dw.backgroundImg = img
			}
		case "cc.capdecibels.textbg":
			dw.textBgPos = [2]int{el.LocH, el.LocV}
			if img, ok := assets[el.Member]; ok {
				dw.textBgImg = img
			}
		case "cc.capdecibel.text":
			// This is the text field
			dw.textRect = [4]int{el.LocH, el.LocV, el.Width, el.Height}
		}
	}

	// Load font
	fm := ui.NewFontManager()
	fm.PreloadCommonFonts()
	dw.font = fm.GetFont("Verdana", "bold", 10)

	dw.visible = true
	return nil
}

// SetDecibels sets the number of decibels to display
func (dw *DecibelsWindow) SetDecibels(decibels int) {
	dw.decibels = decibels
	dw.message = fmt.Sprintf("You got %d decibels", decibels)
}

// Draw renders the decibels window directly (no window frame)
func (dw *DecibelsWindow) Draw(screen *ebiten.Image) {
	if !dw.visible {
		return
	}

	// Window position (shifted up by 100px)
	winX := float64(dw.rect[0])
	winY := float64(dw.rect[1] - 100)

	// Draw shadow layers first
	if dw.shadowImg != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(winX+float64(dw.shadowPos[0]), winY+float64(dw.shadowPos[1]))
		// Shadow blend mode (ink 32, blend 30)
		op.ColorM.Scale(1, 1, 1, 0.3)
		screen.DrawImage(dw.shadowImg, op)
	}

	if dw.textShadowImg != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(winX+float64(dw.textShadowPos[0]), winY+float64(dw.textShadowPos[1]))
		op.ColorM.Scale(1, 1, 1, 0.3)
		screen.DrawImage(dw.textShadowImg, op)
	}

	// Draw main background
	if dw.backgroundImg != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(winX+float64(dw.bgPos[0]), winY+float64(dw.bgPos[1]))
		screen.DrawImage(dw.backgroundImg, op)
	}

	// Draw text background
	if dw.textBgImg != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(winX+float64(dw.textBgPos[0]), winY+float64(dw.textBgPos[1]))
		screen.DrawImage(dw.textBgImg, op)
	}

	// Draw text
	if dw.font != nil && dw.message != "" {
		// Text position (centered in text rect)
		textX := int(winX) + dw.textRect[0] + dw.textRect[2]/2
		textY := int(winY) + dw.textRect[1] + dw.textRect[3]/2

		// Measure text to center it
		bounds := text.BoundString(dw.font, dw.message)
		textX -= bounds.Dx() / 2
		textY += bounds.Dy() / 2 // Adjust for baseline

		// Draw with specified color #564232
		textColor := color.RGBA{0x56, 0x42, 0x32, 0xFF}
		text.Draw(screen, dw.message, dw.font, textX, textY, textColor)
	}
}

// Hide hides the window
func (dw *DecibelsWindow) Hide() {
	dw.visible = false
}

// Show makes the window visible again
func (dw *DecibelsWindow) Show() {
	dw.visible = true
}

// IsVisible returns true if the window should be drawn
func (dw *DecibelsWindow) IsVisible() bool {
	return dw.visible
}
