package studio

import (
	"image/color"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"

	lingoassets "gocoke/internal/client/assets/lingo"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/isoengine"
	"gocoke/internal/client/ui"
)

// ChatBubble represents a composed chat bubble to render
type ChatBubble struct {
	img      *ebiten.Image
	posX     float64
	posY     float64
	created  time.Time
	lifetime time.Duration
}

// SpeechMode represents the current speech mode
type SpeechMode string

const (
	SpeechModeSpeak SpeechMode = "speak"
	SpeechModeShout SpeechMode = "shout"
	SpeechModeSing  SpeechMode = "sing"
)

// ChatManager handles all chat-related functionality for the studio scene
type ChatManager struct {
	// Chat input and bubbles
	fieldRect   [4]int
	focused     bool
	input       string
	assets      map[string]*ebiten.Image // cc.bubble.left/right
	bubbles     []ChatBubble
	fontManager *ui.FontManager
	textManager *textmgr.Manager

	// Speech switch (speak/shout) state
	speechSwitchRect [4]int // relative to window origin
	speechMode       SpeechMode
	speechMouseDown  bool

	// Debug state
	lastBubbleCount int

	// Key repeat state for backspace
	backspaceHeld       bool
	backspaceHoldStart  time.Time
	lastBackspaceRepeat time.Time
}

// NewChatManager creates a new ChatManager instance
func NewChatManager(fontManager *ui.FontManager, textManager *textmgr.Manager) *ChatManager {
	return &ChatManager{
		fontManager: fontManager,
		textManager: textManager,
		assets:      make(map[string]*ebiten.Image),
		speechMode:  SpeechModeSpeak,
	}
}

// LoadAssets loads chat bubble assets from the chatengine directory
func (cm *ChatManager) LoadAssets() error {
	chatDir := filepath.Join("Lingo", "chatengine")
	chatMap, err := lingoassets.LoadMemberMap(chatDir)
	if err != nil {
		return err
	}

	for _, name := range []string{"cc.bubble.left", "cc.bubble.right"} {
		if chatMap != nil {
			if p, ok := chatMap.PathFor(name); ok {
				if img, err := lingoassets.LoadImage(p); err == nil {
					cm.assets[name] = img
				}
			}
		}
	}
	return nil
}

// SetFieldRect sets the chat input field rectangle
func (cm *ChatManager) SetFieldRect(rect [4]int) {
	cm.fieldRect = rect
}

// SetSpeechSwitchRect sets the speech switch rectangle
func (cm *ChatManager) SetSpeechSwitchRect(rect [4]int) {
	cm.speechSwitchRect = rect
}

// GetSpeechMode returns the current speech mode
func (cm *ChatManager) GetSpeechMode() SpeechMode {
	return cm.speechMode
}

// GetInput returns the current chat input text
func (cm *ChatManager) GetInput() string {
	return cm.input
}

// IsFocused returns whether the chat input is currently focused
func (cm *ChatManager) IsFocused() bool {
	return cm.focused
}

// HandleInput processes chat input and focus changes
func (cm *ChatManager) HandleInput(mouseX, mouseY int, mouseClicked bool, offsetX, offsetY float64, isMouseOverWindow bool) {
	// Compute absolute chat field rect on stage
	cfX := int(offsetX) + cm.fieldRect[0]
	cfY := int(offsetY) + cm.fieldRect[1]
	cfW := cm.fieldRect[2]
	cfH := cm.fieldRect[3]
	inChat := mouseX >= cfX && mouseX < cfX+cfW && mouseY >= cfY && mouseY < cfY+cfH

	if mouseClicked && inChat {
		cm.focused = true
	}
	if mouseClicked && !inChat && !isMouseOverWindow {
		cm.focused = false
	}

	if cm.focused {
		// Append printable characters with pixel-width limit to prevent overflow
		font := cm.fontManager.GetFont("Verdana", "bold", 10)
		maxWidth := cm.fieldRect[2] - 24 // ~20px left padding + ~4px right padding
		for _, r := range ebiten.AppendInputChars(nil) {
			if r >= 32 && r <= 126 {
				candidate := cm.input + string(r)
				if font != nil {
					w := text.BoundString(font, candidate).Dx()
					if w > maxWidth {
						continue // skip chars that would overflow the chat field
					}
				}
				cm.input += string(r)
			}
		}

		// Backspace with key repeat when held
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
			if len(cm.input) > 0 {
				cm.input = cm.input[:len(cm.input)-1]
			}
			cm.backspaceHeld = true
			cm.backspaceHoldStart = time.Now()
			cm.lastBackspaceRepeat = cm.backspaceHoldStart
		} else if ebiten.IsKeyPressed(ebiten.KeyBackspace) {
			// Start/continue repeat deletion after initial delay
			now := time.Now()
			if !cm.backspaceHeld {
				cm.backspaceHeld = true
				cm.backspaceHoldStart = now
				cm.lastBackspaceRepeat = now
			} else {
				initialDelay := 350 * time.Millisecond
				repeatEvery := 40 * time.Millisecond
				if now.Sub(cm.backspaceHoldStart) >= initialDelay && now.Sub(cm.lastBackspaceRepeat) >= repeatEvery {
					if len(cm.input) > 0 {
						cm.input = cm.input[:len(cm.input)-1]
					}
					cm.lastBackspaceRepeat = now
				}
			}
		} else {
			cm.backspaceHeld = false
		}

		// Enter to send
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) && len(strings.TrimSpace(cm.input)) > 0 {
			return // Let caller handle the send via EmitChatBubble
		}
	}
}

// ShouldSendMessage returns true if Enter was pressed with non-empty input
func (cm *ChatManager) ShouldSendMessage() bool {
	if !cm.focused {
		return false
	}
	return inpututil.IsKeyJustPressed(ebiten.KeyEnter) && len(strings.TrimSpace(cm.input)) > 0
}

// ClearInput clears the current chat input
func (cm *ChatManager) ClearInput() {
	cm.input = ""
}

// HandleSpeechSwitch processes speech switch input (speak/shout toggle)
func (cm *ChatManager) HandleSpeechSwitch(mouseX, mouseY int, mouseClicked, mouseReleased bool, offsetX, offsetY float64) {
	if cm.speechMode == SpeechModeSing { // sing locks the switch per Lingo
		return
	}

	sx := int(offsetX) + cm.speechSwitchRect[0]
	sy := int(offsetY) + cm.speechSwitchRect[1]
	sw := cm.speechSwitchRect[2]
	sh := cm.speechSwitchRect[3]
	inside := mouseX >= sx && mouseX < sx+sw && mouseY >= sy && mouseY < sy+sh

	if mouseClicked && inside {
		cm.speechMouseDown = true
	}
	if mouseReleased {
		if cm.speechMouseDown && inside {
			if cm.speechMode == SpeechModeSpeak {
				cm.speechMode = SpeechModeShout
			} else if cm.speechMode == SpeechModeShout {
				cm.speechMode = SpeechModeSpeak
			}
		}
		cm.speechMouseDown = false
	}
}

// EmitChatBubble composes a chat bubble based on Lingo style and adds it to render list
func (cm *ChatManager) EmitChatBubble(message string, playerAvatar *isoengine.IsoAvatar) {
	if playerAvatar == nil {
		return
	}

	left := cm.assets["cc.bubble.left"]
	right := cm.assets["cc.bubble.right"]
	if left == nil || right == nil || cm.fontManager == nil {
		log.Printf("ChatManager: bubble assets missing - left:%v right:%v fontManager:%v", left != nil, right != nil, cm.fontManager != nil)
		return
	}
	log.Printf("ChatManager: emitting bubble for %s: %q", playerAvatar.GetScreenName(), message)

	// Name and message composition roughly matching Lingo (bold name, normal msg)
	name := playerAvatar.GetScreenName()
	nameFont := cm.fontManager.GetFont("Verdana", "bold", 10)
	msgFont := cm.fontManager.GetFont("Verdana", "plain", 10)
	if cm.speechMode == SpeechModeShout {
		if bold := cm.fontManager.GetFont("Verdana", "bold", 10); bold != nil {
			msgFont = bold
		}
	}
	if nameFont == nil || msgFont == nil {
		return
	}

	nameBounds := text.BoundString(nameFont, name+" ")
	msgBounds := text.BoundString(msgFont, message+" ")
	margin := 10

	// Build left bubble image tinted (approx) and render name text on it
	leftW := nameBounds.Dx() + margin*2
	leftH := left.Bounds().Dy()
	leftImg := ebiten.NewImage(leftW, leftH)

	// Tint base with avatar chest/body color using ColorM
	{
		op := &ebiten.DrawImageOptions{}
		sx := float64(leftW) / float64(left.Bounds().Dx())
		sy := float64(leftH) / float64(left.Bounds().Dy())
		op.GeoM.Scale(sx, sy)
		// Determine tint color from avatar attributes
		tint := color.RGBA{255, 255, 255, 255}
		if playerAvatar.Engine != nil {
			if c, ok := playerAvatar.Engine.GetBodyPartColor("ch"); ok {
				if cc, ok2 := c.(color.RGBA); ok2 {
					tint = cc
				}
			} else if c, ok := playerAvatar.Engine.GetBodyPartColor("bd"); ok {
				if cc, ok2 := c.(color.RGBA); ok2 {
					tint = cc
				}
			}
		}
		op.ColorM.Scale(float64(tint.R)/255.0, float64(tint.G)/255.0, float64(tint.B)/255.0, 1.0)
		leftImg.DrawImage(left, op)
	}

	// Draw name text centered vertically
	nameImg := ebiten.NewImage(leftW, leftH)
	text.Draw(nameImg, name+" ", nameFont, margin, leftH-(leftH-nameBounds.Dy())/2-2, color.RGBA{0, 0, 0, 255})
	{
		op := &ebiten.DrawImageOptions{}
		op.Blend = ebiten.BlendSourceOver
		leftImg.DrawImage(nameImg, op)
	}

	// Build right bubble for message
	rightW := msgBounds.Dx() + margin*2
	rightH := right.Bounds().Dy()
	rightImg := ebiten.NewImage(rightW, rightH)
	{
		op := &ebiten.DrawImageOptions{}
		sx := float64(rightW) / float64(right.Bounds().Dx())
		sy := float64(rightH) / float64(right.Bounds().Dy())
		op.GeoM.Scale(sx, sy)
		rightImg.DrawImage(right, op)
	}
	msgImg := ebiten.NewImage(rightW, rightH)
	text.Draw(msgImg, message+" ", msgFont, margin, rightH-(rightH-msgBounds.Dy())/2-2, color.RGBA{0, 0, 0, 255})
	{
		op := &ebiten.DrawImageOptions{}
		op.Blend = ebiten.BlendSourceOver
		rightImg.DrawImage(msgImg, op)
	}

	// Stitch together
	bubbleW := leftW + rightW
	bubbleH := rightH
	final := ebiten.NewImage(bubbleW, bubbleH)
	{
		op := &ebiten.DrawImageOptions{}
		final.DrawImage(leftImg, op)
	}
	{
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(leftW), 0)
		final.DrawImage(rightImg, op)
	}

	// Place above avatar head, clamp within stage width
	pos := playerAvatar.GetScreenPos()
	x := pos.X - float64(bubbleW)/2
	if x < 0 {
		x = 0
	}
	stageW := float64(760) // Logical viewport width matching Lingo
	if x+float64(bubbleW) > stageW {
		x = stageW - float64(bubbleW) - 75 // match Lingo right padding
	}

	// Fixed position for newest bubble - always at the bottom of the chat area
	// All new bubbles start at the same Y position regardless of which avatar is speaking
	baseY := float64(100) // Base Y position for the bottom-most bubble (raised by 150px)
	y := baseY

	// Lifetime based on speak/shout/sing
	life := 60 * time.Second // 10x longer baseline
	switch cm.speechMode {
	case SpeechModeShout:
		life = 80 * time.Second
	case SpeechModeSing:
		life = 100 * time.Second
	}

	// Push ALL existing bubbles up by one bubble height + gap
	// This ensures newest message is always at bottom, older messages move up
	const bubbleGap = 5
	bubbleH = final.Bounds().Dy()
	pushDistance := float64(bubbleH + bubbleGap)

	for i := range cm.bubbles {
		cm.bubbles[i].posY -= pushDistance
	}
	cm.bubbles = append(cm.bubbles, ChatBubble{
		img:      final,
		posX:     x,
		posY:     y,
		created:  time.Now(),
		lifetime: life,
	})
	// Clean up very old expired bubbles to prevent memory leak
	// Keep the last 10 bubbles maximum (even if expired)
	if len(cm.bubbles) > 5 {
		cm.bubbles = cm.bubbles[len(cm.bubbles)-5:]
	}
	log.Printf("ChatManager: bubble created at %.0f,%.0f lifetime=%v total=%d", x, y, life, len(cm.bubbles))
}

// DrawChatInput draws the chat input text when focused
func (cm *ChatManager) DrawChatInput(screen *ebiten.Image, offsetX, offsetY float64) {
	if !cm.focused || cm.fontManager == nil {
		return
	}

	font := cm.fontManager.GetFont("Verdana", "bold", 10)
	if font != nil {
		col := color.RGBA{0x56, 0x42, 0x32, 0xFF}
		textX := int(offsetX) + cm.fieldRect[0] + 20
		textY := int(offsetY) + cm.fieldRect[1] + cm.fieldRect[3] - 2
		// Draw input text
		text.Draw(screen, cm.input, font, textX, textY, col)
		// Blinking caret at insertion point (end of text)
		if (time.Now().UnixMilli()/500)%2 == 0 {
			caretX := textX + text.BoundString(font, cm.input).Dx()
			text.Draw(screen, "|", font, caretX, textY, col)
		}
	}
}

// DrawSpeechSwitch draws the speech switch using current state image
func (cm *ChatManager) DrawSpeechSwitch(screen *ebiten.Image, assets map[string]*ebiten.Image, offsetX, offsetY float64) {
	if cm.speechSwitchRect[2] <= 0 || cm.speechSwitchRect[3] <= 0 {
		return
	}

	member := "cc.speechswitch." + string(cm.speechMode)
	if cm.speechMouseDown {
		member = "cc.speechswitch.mousedown"
	}
	img := assets[member]
	if img == nil {
		// fallback to speak
		img = assets["cc.speechswitch.speak"]
	}
	if img != nil {
		op := &ebiten.DrawImageOptions{}
		op.Filter = ebiten.FilterNearest
		op.GeoM.Translate(offsetX+float64(cm.speechSwitchRect[0]), offsetY+float64(cm.speechSwitchRect[1]))
		screen.DrawImage(img, op)
	}
}

// DrawChatBubbles draws all active chat bubbles
func (cm *ChatManager) DrawChatBubbles(screen *ebiten.Image) {
	// Draw all bubbles - expired ones simply won't be visible
	drawnCount := 0
	for i := 0; i < len(cm.bubbles); i++ {
		b := cm.bubbles[i]
		// Skip expired bubbles
		if time.Since(b.created) > b.lifetime {
			continue
		}
		drawnCount++
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(b.posX, b.posY)
		screen.DrawImage(b.img, op)
	}

	// Only log when bubble count changes
	if drawnCount != cm.lastBubbleCount {
		log.Printf("ChatManager: drawing %d active bubbles", drawnCount)
		cm.lastBubbleCount = drawnCount
	}
}
