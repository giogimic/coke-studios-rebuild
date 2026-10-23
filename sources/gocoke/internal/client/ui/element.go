// UI element system that replicates Lingo sprite and text field behavior
// Provides 1:1 visual parity with original Director/Shockwave UI elements

package ui

import (
	"image/color"
	"log"
	"strconv"
	"strings"

	"gocoke/internal/client/assets/lingo/window"
	textmgr "gocoke/internal/client/assets/text"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
)

// ElementRenderer handles rendering of Lingo UI elements with exact positioning and styling
type ElementRenderer struct {
	assets      map[string]*ebiten.Image
	tm          *textmgr.Manager
	fontManager *FontManager
}

// ButtonState represents the current state of an interactive button
type ButtonState int

const (
	ButtonActive ButtonState = iota
	ButtonRollover
	ButtonPressed
	ButtonFlash
)

// InteractiveElement represents a clickable UI element with multiple states
type InteractiveElement struct {
	ID       string
	BaseName string // e.g., "cc.entry.help"
	TextKey  string // Text key for button text (e.g., "NAV_DTL_HELP_CREATE_BUTTON")
	Rect     [4]int // [x, y, width, height]
	State    ButtonState
	Assets   map[ButtonState]*ebiten.Image

	// Animation state (for flash animations)
	IsFlashing    bool
	FlashTimer    int
	FlashDuration int // Duration in frames (Lingo used frame-based timing)
}

func NewElementRenderer(assets map[string]*ebiten.Image, tm *textmgr.Manager) *ElementRenderer {
	return &ElementRenderer{
		assets: assets,
		tm:     tm,
	}
}

// SetFontManager sets the font manager for this renderer
func (r *ElementRenderer) SetFontManager(fm *FontManager) {
	r.fontManager = fm
}

// RenderElement renders a single window element with exact Lingo positioning and styling
func (r *ElementRenderer) RenderElement(screen *ebiten.Image, element window.Element, offsetX, offsetY float64) {
	switch element.Media {
	case "bitmap":
		// Check if this is a button element
		if element.Type == "button" {
			r.renderButton(screen, element, offsetX, offsetY)
		} else {
			r.renderBitmap(screen, element, offsetX, offsetY)
		}
	case "field":
		r.renderTextField(screen, element, offsetX, offsetY)
	case "text":
		// Some window frames use #media: #text in definitions
		r.renderTextField(screen, element, offsetX, offsetY)
	}
}

// renderBitmap renders a bitmap element (sprite) with exact positioning
func (r *ElementRenderer) renderBitmap(screen *ebiten.Image, element window.Element, offsetX, offsetY float64) {
	var img *ebiten.Image

	// Check for dynamic images first (by element ID)
	if element.ID != "" {
		img = r.assets[element.ID]
	}

	// Fallback to member name if no dynamic image found
	if img == nil {
		img = r.assets[element.Member]
	}

	if img == nil {
		return
	}

	op := &ebiten.DrawImageOptions{}
	// Use nearest filter to avoid halo/bleed on pixel UI with alpha edges
	op.Filter = ebiten.FilterNearest

	// Apply scaling if width/height differ from source
	srcW := img.Bounds().Dx()
	srcH := img.Bounds().Dy()
	scaleX := 1.0
	scaleY := 1.0
	if element.Width > 0 && element.Width != srcW {
		scaleX = float64(element.Width) / float64(srcW)
	}
	if element.Height > 0 && element.Height != srcH {
		scaleY = float64(element.Height) / float64(srcH)
	}
	op.GeoM.Scale(scaleX, scaleY)
	// Position after scaling to keep top-left at locH/locV
	op.GeoM.Translate(offsetX+float64(element.LocH), offsetY+float64(element.LocV))

	// Use standard blending; matte/blend map to SourceOver in Ebiten
	op.Blend = ebiten.BlendSourceOver

	// Apply blend (opacity) - Lingo uses 0-100
	if element.Blend < 100 {
		a := float32(element.Blend)
		if a < 0 {
			a = 0
		}
		op.ColorScale.ScaleAlpha(a / 100.0)
	}

	screen.DrawImage(img, op)
}

// renderButton renders a button element using the button model system (matches Lingo button elements)
func (r *ElementRenderer) renderButton(screen *ebiten.Image, element window.Element, offsetX, offsetY float64) {
	// Map button model numbers to button asset names (matches Lingo button element system)
	var buttonPrefix string
	switch element.Model {
	case 1:
		// Model 1 buttons (used in various room windows) use button.b1 assets
		// Special-case choose song Play! to use Go! style (button.e1)
		if element.ID == "songlist_play" {
			buttonPrefix = "button.e1"
		} else {
			buttonPrefix = "button.b1"
		}
	case 2:
		buttonPrefix = "button.b1" // Private navigation buttons use button.b1 assets
	case 4:
		// Model 4 is used for search button - use the specific search button asset
		if element.ID == "nav_go_search_button" || element.ID == "nav_v-ego_search_button" {
			// Search button uses a single asset, not multi-part
			r.renderSearchButton(screen, element, offsetX, offsetY)
			return
		}
		buttonPrefix = "button.d1" // Default for model 4
	case 6:
		// Messenger buttons (e.g., remove/cancel) use button.d6 multipart assets
		buttonPrefix = "button.d6"
	case 7:
		// Messenger buttons (e.g., compose/send) use button.d7 multipart assets
		buttonPrefix = "button.d7"
	case 8:
		buttonPrefix = "button.e1" // Go button uses button.e1 assets
	default:
		log.Printf("ElementRenderer: unsupported button model %d", element.Model)
		return
	}

	// Determine button state suffix (active or pressed)
	stateSuffix := ".active" // default state

	// Check if this button is an interactive element with pressed state
	if element.ID != "" {
		// This will be handled by the interactive element system instead
		// Button elements should be converted to interactive elements for proper state management
		log.Printf("ElementRenderer: button element %s should be converted to interactive element", element.ID)
	}

	// Load button parts (left, middle, right) in current state
	leftImg := r.assets[buttonPrefix+".left"+stateSuffix]
	middleImg := r.assets[buttonPrefix+".middle"+stateSuffix]
	rightImg := r.assets[buttonPrefix+".right"+stateSuffix]

	if leftImg == nil || middleImg == nil || rightImg == nil {
		log.Printf("ElementRenderer: missing button assets for %s", buttonPrefix)
		return
	}

	// Calculate button dimensions
	leftWidth := leftImg.Bounds().Dx()
	rightWidth := rightImg.Bounds().Dx()
	middleWidth := element.Width - leftWidth - rightWidth

	if middleWidth < 0 {
		middleWidth = 0
	}

	x := offsetX + float64(element.LocH)
	y := offsetY + float64(element.LocV)

	// Draw left part
	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	// Optional cap-only Y offset for Play button
	capYOffset := 0.0
	if element.ID == "songlist_play" {
		capYOffset = -3.0
	}
	op.GeoM.Translate(x, y+capYOffset)
	screen.DrawImage(leftImg, op)

	// Draw middle part (stretched to fill)
	if middleWidth > 0 {
		op = &ebiten.DrawImageOptions{}
		op.Filter = ebiten.FilterNearest
		scaleX := float64(middleWidth) / float64(middleImg.Bounds().Dx())
		op.GeoM.Scale(scaleX, 1.0)
		op.GeoM.Translate(x+float64(leftWidth), y+capYOffset)
		screen.DrawImage(middleImg, op)
	}

	// Draw right part
	op = &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	op.GeoM.Translate(x+float64(leftWidth)+float64(middleWidth), y+capYOffset)
	screen.DrawImage(rightImg, op)

	// Draw button text if specified
	if element.Key != "" && r.tm != nil {
		buttonText := r.tm.First(element.Key)
		if buttonText == "" {
			buttonText = r.tm.First(strings.ToUpper(element.Key))
		}
		if buttonText == "" {
			buttonText = element.Key // Fallback to key if not found in text manager
		}

		// Get font for button text (matches Lingo button8.element.txt: Verdana, 10, bold, center)
		if r.fontManager != nil {
			font := r.fontManager.GetFont("Verdana", "bold", 10)
			if font != nil {
				// Center text in button
				textBounds := text.BoundString(font, buttonText)
				textWidth := textBounds.Dx()
				textHeight := textBounds.Dy()

				textX := x + float64(element.Width)/2 - float64(textWidth)/2
				textY := y + float64(element.Height)/2 + float64(textHeight)/2 - 2 // Slight baseline adjustment

				// Use button text color (matches Lingo: #color: "#F3E5D5")
				textColor := color.RGBA{243, 229, 213, 255} // #F3E5D5
				text.Draw(screen, buttonText, font, int(textX), int(textY), textColor)
			}
		}
	}
}

// renderSearchButton renders the search button which uses a single asset instead of multi-part
func (r *ElementRenderer) renderSearchButton(screen *ebiten.Image, element window.Element, offsetX, offsetY float64) {
	// Search button uses cc.navi.search_btn.active or cc.navi.search_btn.pressed
	assetName := "cc.navi.search_btn.active" // default state

	// Check if this button is pressed (would need interactive element state)
	// For now, just use active state

	img := r.assets[assetName]
	if img == nil {
		log.Printf("ElementRenderer: search button asset %s not found", assetName)
		return
	}

	// Draw the search button image
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(offsetX+float64(element.LocH), offsetY+float64(element.LocV))
	screen.DrawImage(img, op)

	log.Printf("ElementRenderer: rendered search button %s", element.ID)
}

// renderTextField renders a text field with exact Lingo typography
func (r *ElementRenderer) renderTextField(screen *ebiten.Image, element window.Element, offsetX, offsetY float64) {
	// Get text content from text manager using the key (case-insensitive)
	var textContent string
	if element.Key != "" {
		textContent = r.tm.First(element.Key)
		if textContent == "" {
			textContent = r.tm.First(strings.ToUpper(element.Key))
		}
	}
	if textContent == "" {
		textContent = element.Key // Fallback to key if not found
	}

	// Interpret escaped newlines ("\n") as actual newlines for multi-line titles/texts
	if strings.Contains(textContent, "\\n") {
		textContent = strings.ReplaceAll(textContent, "\\n", "\n")
	}

	// Parse text color from hex string (e.g., "#381C0F")
	textColor := parseHexColor(element.TxtColor)

	// Normalize font style token(s); support combined tokens like "bold,underline"
	styleStr := strings.ToLower(element.FontStyle)
	fontStyle := "plain"
	if strings.Contains(styleStr, "bold") {
		fontStyle = "bold"
	} else if strings.Contains(styleStr, "italic") {
		fontStyle = "italic"
	}
	underline := strings.Contains(styleStr, "underline")

	// Get font face using font manager
	fontFace := r.fontManager.GetFont(element.Font, fontStyle, element.FontSize)

	// Calculate base position
	baseX := offsetX + float64(element.LocH)
	baseY := offsetY + float64(element.LocV) + float64(element.FontSize) // Baseline adjustment

	// Word wrap within element.Width if requested
	if element.WordWrap != 0 && element.Width > 0 {
		parts := strings.Split(textContent, "\n")
		for i := range parts {
			parts[i] = wrapText(fontFace, parts[i], element.Width)
		}
		textContent = strings.Join(parts, "\n")
	}

	// Support explicit newlines by splitting and drawing per line
	lines := strings.Split(textContent, "\n")
	// Compute line height from font size (window.Element has no explicit line height here)
	lineHeight := element.FontSize + 3
	if lineHeight <= 0 {
		lineHeight = 13
	}
	for i, line := range lines {
		x := baseX
		y := baseY + float64(i*lineHeight)
		// Apply alignment per line
		align := strings.ToLower(element.Alignment)
		if align == "#center" || align == "center" {
			b := text.BoundString(fontFace, line)
			width := float64(b.Dx())
			x += (float64(element.Width) - width) / 2
		} else if align == "#right" || align == "right" {
			b := text.BoundString(fontFace, line)
			width := float64(b.Dx())
			x += float64(element.Width) - width
		}
		text.Draw(screen, line, fontFace, int(x), int(y), textColor)

		// Draw underline if requested
		if underline && line != "" {
			b := text.BoundString(fontFace, line)
			uw := b.Dx()
			if uw > 0 {
				ul := ebiten.NewImage(uw, 1)
				ul.Fill(textColor)
				op := &ebiten.DrawImageOptions{}
				// Position underline slightly below baseline
				op.GeoM.Translate(x, y+2)
				screen.DrawImage(ul, op)
			}
		}
	}
}

// parseHexColor converts a hex color string (e.g., "#381C0F") to color.RGBA
func parseHexColor(hexColor string) color.RGBA {
	if hexColor == "" || !strings.HasPrefix(hexColor, "#") {
		return color.RGBA{0, 0, 0, 255} // Default to black
	}
	hex := hexColor[1:]
	if len(hex) != 6 {
		return color.RGBA{0, 0, 0, 255}
	}
	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}

// wrapText performs a simple greedy word wrap within a given width using the fontFace
func wrapText(fontFace font.Face, s string, maxWidth int) string {
	if s == "" || maxWidth <= 0 {
		return s
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	var lines []string
	line := words[0]
	for _, w := range words[1:] {
		test := line + " " + w
		b := text.BoundString(fontFace, test)
		if b.Dx() <= maxWidth {
			line = test
		} else {
			lines = append(lines, line)
			line = w
		}
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n")
}

// CreateInteractiveElement creates a button element with multiple states
func (r *ElementRenderer) CreateInteractiveElement(element window.Element) *InteractiveElement {
	baseName := element.Member

	// Handle multi-part buttons (e.g., "button.e1.left.active" -> "button.e1", "button.b1.left.active" -> "button.b1")
	if strings.Contains(baseName, "button.e1") {
		baseName = "button.e1"
	} else if strings.Contains(baseName, "button.b1") {
		baseName = "button.b1"
	} else if strings.HasSuffix(baseName, ".active") {
		baseName = strings.TrimSuffix(baseName, ".active")
	}

	ie := &InteractiveElement{
		ID:            element.ID,
		BaseName:      baseName,
		TextKey:       element.Key, // Store the text key for button text
		Rect:          [4]int{element.LocH, element.LocV, element.Width, element.Height},
		State:         ButtonActive,
		Assets:        make(map[ButtonState]*ebiten.Image),
		IsFlashing:    false,
		FlashTimer:    0,
		FlashDuration: 30, // 30 frames = 0.5 seconds at 60 FPS (match Lingo timing)
	}

	// Load all button states
	ie.Assets[ButtonActive] = r.assets[baseName+".active"]
	ie.Assets[ButtonRollover] = r.assets[baseName+".rollover"]
	ie.Assets[ButtonPressed] = r.assets[baseName+".pressed"]
	ie.Assets[ButtonFlash] = r.assets[baseName+".flash"]

	return ie
}

// RenderInteractiveElement renders a button in its current state
func (r *ElementRenderer) RenderInteractiveElement(screen *ebiten.Image, ie *InteractiveElement, offsetX, offsetY float64) {
	// Update animation state
	ie.updateAnimation()

	// Check if this is a multi-part button (e.g., button.e1, button.b1)
	if strings.Contains(ie.BaseName, "button.e1") || strings.Contains(ie.BaseName, "button.b1") {
		r.renderMultiPartButton(screen, ie, offsetX, offsetY)
		return
	}

	// Standard single-image button rendering
	var img *ebiten.Image
	if ie.IsFlashing && ie.Assets[ButtonFlash] != nil {
		// Show flash state during animation
		img = ie.Assets[ButtonFlash]
	} else {
		// Show current interaction state
		img = ie.Assets[ie.State]
		if img == nil {
			// Fallback to active state
			img = ie.Assets[ButtonActive]
			if img == nil {
				return
			}
		}
	}

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(offsetX+float64(ie.Rect[0]), offsetY+float64(ie.Rect[1]))
	screen.DrawImage(img, op)
}

// renderMultiPartButton renders a multi-part button (left, middle, right) in its current state
func (r *ElementRenderer) renderMultiPartButton(screen *ebiten.Image, ie *InteractiveElement, offsetX, offsetY float64) {
	// Determine state suffix based on button state
	var stateSuffix string
	switch ie.State {
	case ButtonPressed:
		stateSuffix = ".pressed"
	case ButtonRollover:
		stateSuffix = ".active" // Use active for rollover since we don't have rollover assets
	default:
		stateSuffix = ".active"
	}

	// Load button parts for current state
	leftImg := r.assets[ie.BaseName+".left"+stateSuffix]
	middleImg := r.assets[ie.BaseName+".middle"+stateSuffix]
	rightImg := r.assets[ie.BaseName+".right"+stateSuffix]

	if leftImg == nil || middleImg == nil || rightImg == nil {
		log.Printf("ElementRenderer: missing multi-part button assets for %s%s", ie.BaseName, stateSuffix)
		return
	}

	// Calculate button dimensions
	leftWidth := leftImg.Bounds().Dx()
	rightWidth := rightImg.Bounds().Dx()
	middleWidth := ie.Rect[2] - leftWidth - rightWidth

	if middleWidth < 0 {
		middleWidth = 0
	}

	x := offsetX + float64(ie.Rect[0])
	y := offsetY + float64(ie.Rect[1])

	// Optional Y adjustment for specific buttons
	// Shift Go! button assets: Public up by 2px, Private up by 3px (labels remain anchored)
	yAdjust := 0.0
	if ie.ID == "nav_go_public_button" {
		yAdjust = -2.0
	} else if ie.ID == "nav_go_private_button" {
		yAdjust = -3.0
	}

	// Draw left part
	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	// Optional cap-only Y offset for Play button
	capYOffset := 0.0
	if ie.ID == "songlist_play" {
		capYOffset = -3.0
	}
	op.GeoM.Translate(x, y+yAdjust+capYOffset)
	screen.DrawImage(leftImg, op)

	// Draw middle part (stretched to fill)
	if middleWidth > 0 {
		op = &ebiten.DrawImageOptions{}
		op.Filter = ebiten.FilterNearest
		scaleX := float64(middleWidth) / float64(middleImg.Bounds().Dx())
		op.GeoM.Scale(scaleX, 1.0)
		op.GeoM.Translate(x+float64(leftWidth), y+yAdjust+capYOffset)
		screen.DrawImage(middleImg, op)
	}

	// Draw right part
	op = &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	op.GeoM.Translate(x+float64(leftWidth)+float64(middleWidth), y+yAdjust+capYOffset)
	screen.DrawImage(rightImg, op)

	// Draw button text using the stored text key
	if r.tm != nil {
		var buttonText string
		if ie.TextKey != "" {
			// Use the text key from the interactive element
			buttonText = r.tm.First(ie.TextKey)
			if buttonText == "" {
				buttonText = r.tm.First(strings.ToUpper(ie.TextKey))
			}
		}
		if buttonText == "" {
			// Fallback for Go button or unknown keys
			if ie.ID == "nav_go_public_button" {
				buttonText = r.tm.First("GO")
				if buttonText == "" {
					buttonText = "Go!" // Final fallback for Go button
				}
			} else {
				buttonText = ie.TextKey // Show the key itself as fallback
			}
		}

		// Get font for button text (matches Lingo button8.element.txt: Verdana, 10, bold, center)
		if r.fontManager != nil {
			font := r.fontManager.GetFont("Verdana", "bold", 10)
			if font != nil {
				// Center text in button
				textBounds := text.BoundString(font, buttonText)
				textWidth := textBounds.Dx()
				textHeight := textBounds.Dy()

				textX := x + float64(ie.Rect[2])/2 - float64(textWidth)/2
				textY := y + float64(ie.Rect[3])/2 + float64(textHeight)/2 - 2 // Slight baseline adjustment (text anchored, not affected by yAdjust)
				// General alert OK label fine-tune: left 1px, down 2px
				if ie.ID == "close" {
					textX -= 1
					textY += 3
				}
				// Shift purchase button labels down by 3px
				if ie.ID == "purchase_decision_ok" || ie.ID == "sanfo_decision_cancel" {
					textY += 3
				}

				// Use button text color
				// For purchase OK, force white text per spec; otherwise use default scheme
				var textColor color.RGBA
				if ie.ID == "purchase_decision_ok" {
					textColor = color.RGBA{255, 255, 255, 255}
				} else if ie.State == ButtonPressed {
					textColor = color.RGBA{78, 98, 122, 255} // #4E627A (pressed)
				} else {
					textColor = color.RGBA{243, 229, 213, 255} // #F3E5D5 (active)
				}
				text.Draw(screen, buttonText, font, int(textX), int(textY), textColor)
			}
		}
	}
}

// HandleMouseInput updates button state based on mouse position and clicks
func (ie *InteractiveElement) HandleMouseInput(mouseX, mouseY int, mousePressed bool, offsetX, offsetY float64) {
	// Don't change state during flash animation
	if ie.IsFlashing {
		return
	}

	// Check if mouse is within button bounds
	x := int(offsetX) + ie.Rect[0]
	y := int(offsetY) + ie.Rect[1]
	w := ie.Rect[2]
	h := ie.Rect[3]

	mouseInside := mouseX >= x && mouseX < x+w && mouseY >= y && mouseY < y+h

	if mouseInside {
		if mousePressed {
			ie.State = ButtonPressed
		} else {
			ie.State = ButtonRollover
		}
	} else {
		ie.State = ButtonActive
	}
}

// StartFlash starts a flash animation (matches Lingo's flash behavior)
func (ie *InteractiveElement) StartFlash() {
	ie.IsFlashing = true
	ie.FlashTimer = 0
}

// updateAnimation updates the flash animation state
func (ie *InteractiveElement) updateAnimation() {
	if !ie.IsFlashing {
		return
	}

	ie.FlashTimer++
	if ie.FlashTimer >= ie.FlashDuration {
		ie.IsFlashing = false
		ie.FlashTimer = 0
	}
}
