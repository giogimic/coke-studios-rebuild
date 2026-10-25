package studio

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"

	lingoassets "gocoke/internal/client/assets/lingo"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/avatarengine"
	"gocoke/internal/client/isoengine"
	"gocoke/internal/client/ui"
)

// InfoStand mirrors Lingo's InfoStandObject: plaque, preview, description, and buttons.
type InfoStand struct {
	visible           bool
	fontManager       *ui.FontManager
	tm                *textmgr.Manager
	dataDir           string
	currentPlayerName string
	plaque            *ebiten.Image
	btnLeftActive     *ebiten.Image
	btnMidActive      *ebiten.Image
	btnRightActive    *ebiten.Image
	btnLeftPressed    *ebiten.Image
	btnMidPressed     *ebiten.Image
	btnRightPressed   *ebiten.Image

	// Custom Now Playing state
	nowPlayingActive bool
	customPreview    *ebiten.Image
	customTitle      string
	customBody       string
	nowPlayingOwner  string
	nowPlayingPlayer string
	// Voting state for current performance (when we are not the performer)
	nowPlayingVoted  bool
	nowPlayingVoteUp bool

	// Current selection context
	selectedAvatar *isoengine.IsoAvatar
	// TODO: furniture and wall item selections when available
	previewAvatar *avatarengine.AvatarEngine

	// Furniture context
	selectedFurni *PlacedFurni
	furniPreview  *ebiten.Image
	furniTitle    string
	furniBody     string

	// Action callbacks (provided by scene)
	OnRotateFurni func(*PlacedFurni)
	OnMoveFurni   func(*PlacedFurni)
	OnPickupFurni func(*PlacedFurni)

	// Computed UI state
	buttons []infoStandButton

	// Permissions provided by scene
	IsRoomOwner func() bool
	OnKick      func(targetName string)

	// Trade callback
	OnTrade func(targetName string)
}

type infoStandButton struct {
	ID       string
	Rect     image.Rectangle // absolute stage coordinates
	Pressed  bool
	Active   *ebiten.Image
	PressedI *ebiten.Image
}

// Fixed stage coordinates from Lingo InfoStandObject.new()
const (
	plaqueX = 695
	plaqueY = 280
	prevCX  = 723 // preview registration center X
	prevCY  = 310 // preview registration base Y
	descX   = 550
	descY   = 320
	descW   = 190
	descH   = 55
)

func NewInfoStand(fm *ui.FontManager, tm *textmgr.Manager, dataDir string) *InfoStand {
	is := &InfoStand{fontManager: fm, tm: tm, dataDir: dataDir}
	is.loadAssets()
	return is
}

// SetCurrentPlayerName sets the local player's screen name for self-button logic
func (i *InfoStand) SetCurrentPlayerName(name string) {
	i.currentPlayerName = name
}

// SetNowPlayingPlayer sets the avatar name who started the performance
func (i *InfoStand) SetNowPlayingPlayer(name string) {
	i.nowPlayingPlayer = strings.TrimSpace(name)
}

func (i *InfoStand) loadAssets() {
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	mm, _ := lingoassets.LoadMemberMap(interfaceDir)
	if mm == nil {
		return
	}

	load := func(name string) *ebiten.Image {
		if p, ok := mm.PathFor(name); ok {
			img, err := lingoassets.LoadImage(p)
			if err == nil {
				return img
			}
		}
		return nil
	}

	i.plaque = load("cc.infostand")
	i.btnLeftActive = load("button.d1.left.active")
	i.btnMidActive = load("button.d1.middle.active")
	i.btnRightActive = load("button.d1.right.active")
	i.btnLeftPressed = load("button.d1.left.pressed")
	i.btnMidPressed = load("button.d1.middle.pressed")
	i.btnRightPressed = load("button.d1.right.pressed")

	// Load Now Playing preview icon
	if p, ok := mm.PathFor("cd_nowplaying"); ok {
		img, err := lingoassets.LoadImage(p)
		if err == nil {
			i.customPreview = img
		}
	} else {
		// Fallback: direct path if not listed in Members.csv
		if img, err := lingoassets.LoadImage(filepath.Join("Lingo", "cc_interface[1]", "cd_nowplaying.png")); err == nil {
			i.customPreview = img
		}
	}
}

func (i *InfoStand) Clear() {
	i.visible = false
	i.selectedAvatar = nil
	i.buttons = nil
	i.nowPlayingActive = false
}

// ShowForAvatar populates the InfoStand for the given avatar.
func (i *InfoStand) ShowForAvatar(av *isoengine.IsoAvatar) {
	i.selectedAvatar = av
	i.selectedFurni = nil
	i.previewAvatar = nil
	// Disable now playing mode when explicitly showing an avatar (retain custom text for restore)
	i.nowPlayingActive = false
	i.visible = true
	// Build or update static preview avatar, facing SE with no animations
	if av != nil && av.Engine != nil {
		if i.previewAvatar == nil {
			eng, err := avatarengine.NewAvatarEngine("", 100, true, i.dataDir)
			if err == nil {
				i.previewAvatar = eng
			}
		}
		if i.previewAvatar != nil {
			// Copy attributes
			for k, v := range av.Engine.Attributes {
				i.previewAvatar.Attributes[k] = v
			}
			// Face southeast, stand, no emote/dance/blink
			i.previewAvatar.SetDirection(2) // SE
			i.previewAvatar.SetBodyAction("std")
			i.previewAvatar.SetFaceAction("std")
			i.previewAvatar.EyeAction = "std"
			i.previewAvatar.IsDancing = false
			i.previewAvatar.IsEmoting = false
			i.previewAvatar.IsBlink = false
			i.previewAvatar.ResetAnimation()
			// Draw once to buffer
			i.previewAvatar.Draw()
		}
	}
	i.rebuildButtonsForAvatar()
}

func (i *InfoStand) rebuildButtonsForAvatar() {
	i.buttons = nil
	// Now Playing mode: two vote buttons unless owner is current player
	if i.nowPlayingActive {
		if i.fontManager == nil {
			return
		}
		// Do not show vote buttons for self performer or after we've voted
		if strings.EqualFold(i.nowPlayingPlayer, i.currentPlayerName) || i.nowPlayingVoted {
			return
		}
		curRight := descX + descW
		// Order: Thumbs Up then Thumbs Down (left to right when right-aligned)
		labels := []string{"Thumbs Up", "Thumbs Down"}
		for _, label := range labels {
			active := i.composeButton(label, false)
			pressed := i.composeButton(label, true)
			if active == nil || pressed == nil {
				continue
			}
			b := active.Bounds()
			rect := image.Rect(curRight-b.Dx(), descY+descH, curRight, descY+descH+b.Dy())
			curRight = rect.Min.X - 3
			i.buttons = append(i.buttons, infoStandButton{ID: label, Rect: rect, Active: active, PressedI: pressed})
		}
		return
	}
	if i.selectedAvatar == nil || i.fontManager == nil {
		return
	}

	// Determine button set
	self := i.isMe(i.selectedAvatar)
	buttons := []string{}
	if self {
		if i.selectedAvatar.Engine.IsDancing {
			buttons = append(buttons, "Stop Dancing")
		} else if i.selectedAvatar.Engine.GetBodyAction() != "sit" {
			buttons = append(buttons, "Dance")
		}
		// If a performance is ongoing by this user, include Stop Performing to the left of Dance
		if sc := GetActiveScene(); sc != nil {
			if sc.nowPlayingActive && strings.EqualFold(i.nowPlayingPlayer, i.currentPlayerName) {
				// Order matters: add after Dance/Stop Dancing so it renders to the left
				buttons = append(buttons, "Stop Performing")
			}
		}
	} else {
		buttons = append(buttons, "Whisper")
		buttons = append(buttons, "Trade")
		buttons = append(buttons, "Ask to become a friend")
		// Show Kick Out when current user is room owner (authentic Lingo label)
		if i.IsRoomOwner != nil && i.IsRoomOwner() {
			buttons = append(buttons, "Kick Out")
		}
	}

	// Right-align buttons below description rect, 3px spacing (Lingo parity)
	curRight := descX + descW
	for _, label := range buttons {
		active := i.composeButton(label, false)
		pressed := i.composeButton(label, true)
		if active == nil || pressed == nil {
			continue
		}
		b := active.Bounds()
		rect := image.Rect(curRight-b.Dx(), descY+descH, curRight, descY+descH+b.Dy())
		curRight = rect.Min.X - 3
		i.buttons = append(i.buttons, infoStandButton{ID: label, Rect: rect, Active: active, PressedI: pressed})
	}
}

func (i *InfoStand) isMe(av *isoengine.IsoAvatar) bool {
	if av == nil || av.Engine == nil {
		return false
	}
	if i.currentPlayerName == "" {
		return false
	}
	return strings.EqualFold(av.GetScreenName(), i.currentPlayerName)
}

// HandleMouse processes mouse interactions; returns true if the click was consumed
func (i *InfoStand) HandleMouse(mouseX, mouseY int, mousePressed, mouseClicked, mouseReleased bool) bool {
	if !i.visible {
		return false
	}
	consumed := false
	pendingClickID := ""
	for idx := range i.buttons {
		btn := &i.buttons[idx]
		inside := mouseX >= btn.Rect.Min.X && mouseX < btn.Rect.Max.X && mouseY >= btn.Rect.Min.Y && mouseY < btn.Rect.Max.Y
		if inside && mouseClicked {
			btn.Pressed = true
			consumed = true
		}
		if btn.Pressed && mouseReleased {
			btn.Pressed = false
			if inside && pendingClickID == "" {
				// Defer action until after loop to avoid mutating i.buttons during iteration
				pendingClickID = btn.ID
				consumed = true
			}
		}
	}
	if pendingClickID != "" {
		i.onButtonClick(pendingClickID)
	}
	return consumed
}

func (i *InfoStand) onButtonClick(id string) {
	switch id {
	case "Dance":
		if sc := GetActiveScene(); sc != nil {
			sc.SendAvatarAction("dnc")
		} else if i.selectedAvatar != nil && i.selectedAvatar.Engine != nil {
			i.selectedAvatar.Engine.SetDance(true)
		}
		i.rebuildButtonsForAvatar()
	case "Stop Dancing":
		if sc := GetActiveScene(); sc != nil {
			sc.SendAvatarAction("")
		} else if i.selectedAvatar != nil && i.selectedAvatar.Engine != nil {
			i.selectedAvatar.Engine.SetDance(false)
		}
		i.rebuildButtonsForAvatar()
	case "Whisper":
		if i.selectedAvatar != nil {
			if sc := GetActiveScene(); sc != nil && sc.chatManager != nil {
				sc.chatManager.SetInput(":whisper " + i.selectedAvatar.GetScreenName() + " ")
				sc.chatManager.SetFocused(true)
				i.Clear()
			}
		}
	case "Trade":
		if i.selectedAvatar != nil && i.OnTrade != nil {
			i.OnTrade(i.selectedAvatar.GetScreenName())
		}
	case "Ask to become a friend":
		if i.selectedAvatar != nil {
			if sc := GetActiveScene(); sc != nil {
				sc.SendFriendRequest(i.selectedAvatar.GetScreenName())
			}
		}
	case "Rotate":
		if i.selectedFurni != nil && i.OnRotateFurni != nil {
			i.OnRotateFurni(i.selectedFurni)
		}
	case "Move":
		if i.selectedFurni != nil && i.OnMoveFurni != nil {
			i.OnMoveFurni(i.selectedFurni)
		}
	case "Pick up":
		if i.selectedFurni != nil && i.OnPickupFurni != nil {
			i.OnPickupFurni(i.selectedFurni)
		}
	case "Kick", "Kick Out":
		if i.selectedAvatar != nil && i.OnKick != nil {
			i.OnKick(i.selectedAvatar.GetScreenName())
		}
	case "Thumbs Up":
		if sc := GetActiveScene(); sc != nil && sc.sendThumbIfAvailable != nil {
			_ = sc.sendThumbIfAvailable(true)
		}
		// Record vote and update description; hide buttons for this performance
		i.nowPlayingVoted = true
		i.nowPlayingVoteUp = true
		if !strings.Contains(i.customBody, "Your vote:") {
			i.customBody += "\nYour vote: Thumbs Up"
		} else {
			i.customBody = strings.ReplaceAll(i.customBody, "Your vote: Thumbs Down", "Your vote: Thumbs Up")
		}
		i.rebuildButtonsForAvatar()
	case "Thumbs Down":
		if sc := GetActiveScene(); sc != nil && sc.sendThumbIfAvailable != nil {
			_ = sc.sendThumbIfAvailable(false)
		}
		// Record vote and update description; hide buttons for this performance
		i.nowPlayingVoted = true
		i.nowPlayingVoteUp = false
		if !strings.Contains(i.customBody, "Your vote:") {
			i.customBody += "\nYour vote: Thumbs Down"
		} else {
			i.customBody = strings.ReplaceAll(i.customBody, "Your vote: Thumbs Up", "Your vote: Thumbs Down")
		}
		i.rebuildButtonsForAvatar()
	case "Stop Performing":
		if sc := GetActiveScene(); sc != nil && sc.sendCDEndIfAvailable != nil {
			_ = sc.sendCDEndIfAvailable(sc.GetStudio().StudioID)
		}
	default:
		// Button clicked but no handler
	}
}

// Draw renders the InfoStand overlay.
func (i *InfoStand) Draw(screen *ebiten.Image) {
	if !i.visible {
		return
	}
	// 1) Plaque
	if i.plaque != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(plaqueX), float64(plaqueY))
		screen.DrawImage(i.plaque, op)
	}

	// 2) Preview image: custom (now playing) OR avatar/furniture
	if i.nowPlayingActive && i.customPreview != nil {
		op := &ebiten.DrawImageOptions{}
		b := i.customPreview.Bounds()
		// Scale to 50% with nearest neighbor (crisp pixels)
		scale := 0.5
		op.Filter = ebiten.FilterNearest
		op.GeoM.Scale(scale, scale)
		sw := int(float64(b.Dx()) * scale)
		sh := int(float64(b.Dy()) * scale)
		x := prevCX - (sw / 2)
		// Shift up an additional 10px
		y := prevCY - sh - 10
		op.GeoM.Translate(float64(x), float64(y))
		screen.DrawImage(i.customPreview, op)
	} else if i.previewAvatar != nil {
		buf := i.previewAvatar.GetBuffer()
		if buf != nil {
			// White background box under full-size preview
			bw := buf.Bounds().Dx()
			bh := buf.Bounds().Dy()
			w := bw
			h := bh
			x := prevCX - (w / 2)
			y := prevCY - h
			// Draw avatar unscaled (transparent background)
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(x), float64(y))
			screen.DrawImage(buf, op)
		}
	}
	// Furniture preview
	if !i.nowPlayingActive && i.previewAvatar == nil && i.selectedFurni != nil {
		i.RenderFurniPreview(screen)
	}

	// 3) Description text
	i.drawDescription(screen)

	// 4) Buttons
	for _, btn := range i.buttons {
		img := btn.Active
		if btn.Pressed {
			img = btn.PressedI
		}
		if img == nil {
			continue
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(btn.Rect.Min.X), float64(btn.Rect.Min.Y))
		screen.DrawImage(img, op)
	}
}

func (i *InfoStand) drawDescription(screen *ebiten.Image) {
	if i.fontManager == nil {
		return
	}
	var title, body string
	if i.nowPlayingActive {
		title = i.customTitle
		body = i.customBody
	} else if i.selectedAvatar != nil {
		title = i.selectedAvatar.GetScreenName()
		body = i.selectedAvatar.GetMission()
		if i.selectedAvatar.Engine != nil && i.selectedAvatar.Engine.IsDrinking {
			if body != "" {
				body += "\n"
			}
			body += "Carrying: Coke Bottle"
		}
	} else if i.selectedFurni != nil {
		title = i.furniTitle
		body = i.furniBody
	}
	// Text styles
	ffTitle := i.fontManager.GetFont("Verdana", "bold", 10)
	ffBody := i.fontManager.GetFont("Verdana", "plain", 10)
	if ffTitle == nil || ffBody == nil {
		return
	}

	wrap := func(textStr string, face font.Face, maxWidth int) []string {
		if textStr == "" {
			return nil
		}
		words := strings.Fields(textStr)
		var lines []string
		var cur string
		for _, w := range words {
			try := w
			if cur != "" {
				try = cur + " " + w
			}
			bounds := text.BoundString(face, try)
			if bounds.Dx() <= maxWidth {
				cur = try
			} else {
				if cur != "" {
					lines = append(lines, cur)
				}
				cur = w
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		return lines
	}

	// Draw title
	boundsTitle := text.BoundString(ffTitle, title)
	ty := descY + boundsTitle.Dy()
	// Right-align title within desc rect
	tx := descX + (descW - boundsTitle.Dx())
	text.Draw(screen, title, ffTitle, tx, ty, color.RGBA{0xEE, 0xDB, 0xC6, 0xFF})

	// Draw body with explicit newline support, wrapping each segment
	var lines []string
	if strings.Contains(body, "\n") {
		segs := strings.Split(body, "\n")
		for _, seg := range segs {
			if seg == "" {
				lines = append(lines, "")
				continue
			}
			wrapped := wrap(seg, ffBody, descW)
			lines = append(lines, wrapped...)
		}
	} else {
		lines = wrap(body, ffBody, descW)
	}
	cy := ty + 2
	for _, line := range lines {
		b := text.BoundString(ffBody, line)
		if cy+b.Dy() > descY+descH {
			break
		}
		// Right-align each wrapped line within desc rect
		lx := descX + (descW - b.Dx())
		text.Draw(screen, line, ffBody, lx, cy+b.Dy(), color.RGBA{0xEE, 0xDB, 0xC6, 0xFF})
		cy += b.Dy()
	}
}

// composeButton creates a button image for the given label and state.
func (i *InfoStand) composeButton(label string, pressed bool) *ebiten.Image {
	if i.btnLeftActive == nil || i.btnMidActive == nil || i.btnRightActive == nil {
		return nil
	}
	left := i.btnLeftActive
	mid := i.btnMidActive
	right := i.btnRightActive
	if pressed {
		if i.btnLeftPressed != nil {
			left = i.btnLeftPressed
		}
		if i.btnMidPressed != nil {
			mid = i.btnMidPressed
		}
		if i.btnRightPressed != nil {
			right = i.btnRightPressed
		}
	}

	ff := i.fontManager.GetFont("Verdana", "bold", 10)
	if ff == nil {
		return nil
	}

	leftB := left.Bounds()
	midB := mid.Bounds()
	rightB := right.Bounds()
	bounds := text.BoundString(ff, label)
	textW := bounds.Dx()
	textH := bounds.Dy()

	// Compose width: left + textW + right
	w := leftB.Dx() + textW + rightB.Dx()
	h := midB.Dy()
	btn := ebiten.NewImage(w, h)

	// Draw left
	op := &ebiten.DrawImageOptions{}
	btn.DrawImage(left, op)

	// Draw middle stretch area and text
	if textW > 0 {
		// Middle area spans exactly text width
		midScaled := ebiten.NewImage(textW, midB.Dy())
		sx := float64(textW) / float64(midB.Dx())
		opMid := &ebiten.DrawImageOptions{}
		opMid.GeoM.Scale(sx, 1)
		midScaled.DrawImage(mid, opMid)
		op2 := &ebiten.DrawImageOptions{}
		op2.GeoM.Translate(float64(leftB.Dx()), 0)
		btn.DrawImage(midScaled, op2)
	}

	// Draw right
	op = &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(leftB.Dx()+textW), 0)
	btn.DrawImage(right, op)

	// Draw text centered vertically
	textX := leftB.Dx()
	textY := (h - textH) / 2
	text.Draw(btn, label, ff, textX, textY+textH, color.RGBA{0xEE, 0xDB, 0xC6, 0xFF})
	return btn
}

// TryGetAvatarHit checks if a click at (mouseX, mouseY) hits the avatar's current render rect.
// Returns true if hit.
func TryGetAvatarHit(av *isoengine.IsoAvatar, mouseX, mouseY int) bool {
	if av == nil || av.Engine == nil || av.Square == nil {
		return false
	}
	buf := av.Engine.GetBuffer()
	if buf == nil {
		return false
	}
	b := buf.Bounds()
	pos := av.GetScreenPos()
	// Roughly align with draw offsets used in studio.go
	x := pos.X - float64(b.Dx())/2.1
	y := pos.Y - float64(b.Dy())/1.2
	rect := image.Rect(int(x), int(y), int(x)+b.Dx(), int(y)+b.Dy())
	return mouseX >= rect.Min.X && mouseX < rect.Max.X && mouseY >= rect.Min.Y && mouseY < rect.Max.Y
}

// BuildDescriptionForFurni and ShowForFurni can be added once furniture selection is implemented.

// Note: avatar screen name and mission are obtained via isoengine.IsoAvatar getters.
