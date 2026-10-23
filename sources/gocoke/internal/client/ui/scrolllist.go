package ui

import (
	"image"
	"image/color"
	"log"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
)

// RoomListItem matches Lingo roomlist content needs
// (subset of studios.Studio to avoid tight coupling here)
type RoomListItem struct {
	StudioID  string
	Name      string
	UserCount int
	Capacity  int
}

// UserListItem matches Lingo userlist content needs for people lists
type UserListItem struct {
	UserID   string
	Username string
	Status   string // online, away, etc.
}

// FriendListItem represents a Messenger Friends list entry
// Renders as: [online circle] username  .........  X msgs
// Also carries motto and last access for detail panel population
type FriendListItem struct {
	Username   string
	Online     bool
	MessageCnt int
	Motto      string
	LastAccess string // e.g., "9/2/2025 7:42AM"
}

// ScrollList emulates Lingo roomlist script behavior (pScrollIndex, pDisplayLines)
type ScrollList struct {
	Rect         [4]int // relative to window content origin (locH, locV, width, height)
	Items        []RoomListItem
	UserItems    []UserListItem   // for people lists
	FriendItems  []FriendListItem // for messenger friends lists
	LineSpace    int              // default 14
	ScrollIndex  int              // 1-based
	DisplayLines int
	LastClicked  int // for preserving last clicked item

	// Rendering mode
	ShowPlayerCount bool // true for private rooms (show numbers), false for public rooms (show bullets)
	SimpleTextOnly  bool // when true, render names only (no underline, dots, or Go!)

	// Rendering
	offscreen *ebiten.Image

	// Assets required for occupancy bullets (only used when ShowPlayerCount = false)
	Bullets map[int]*ebiten.Image // 0..6

	// Text rendering
	fontFace font.Face
	color    color.Color
}

func NewScrollList(x, y, w, h int, lineSpace int, face font.Face, clr color.Color) *ScrollList {
	if lineSpace <= 0 {
		lineSpace = 14
	}
	return &ScrollList{
		Rect:         [4]int{x, y, w, h},
		LineSpace:    lineSpace,
		ScrollIndex:  1,
		DisplayLines: max(1, h/lineSpace),
		fontFace:     face,
		color:        clr,
	}
}

func (sl *ScrollList) SetItems(items []RoomListItem) {
	sl.Items = items
	// Rebuild offscreen next UpdateContent
	sl.offscreen = nil
	if sl.ScrollIndex < 1 {
		sl.ScrollIndex = 1
	}
	maxStart := max(1, len(sl.Items)-sl.DisplayLines+1)
	if sl.ScrollIndex > maxStart {
		sl.ScrollIndex = maxStart
	}
}

// UpdateContent rebuilds the tall offscreen image with all rows
func (sl *ScrollList) UpdateContent() {
	if sl == nil {
		return
	}

	// Determine which type of items to render
	var totalLines int
	if len(sl.Items) > 0 {
		totalLines = len(sl.Items)
	} else if len(sl.UserItems) > 0 {
		totalLines = len(sl.UserItems)
	} else if len(sl.FriendItems) > 0 {
		totalLines = len(sl.FriendItems)
	}

	if totalLines == 0 {
		sl.offscreen = nil
		return
	}
	h := totalLines * sl.LineSpace
	img := ebiten.NewImage(sl.Rect[2], h)

	// Clear to transparent (Lingo lists render on transparent background)
	img.Fill(color.RGBA{0, 0, 0, 0})

	// Render room items (original behavior)
	if len(sl.Items) > 0 {
		sl.renderRoomItems(img)
	} else if len(sl.UserItems) > 0 {
		sl.renderUserItems(img)
	} else if len(sl.FriendItems) > 0 {
		sl.renderFriendItems(img)
	}

	sl.offscreen = img
}

// renderRoomItems renders room list items with different modes for public vs private
func (sl *ScrollList) renderRoomItems(img *ebiten.Image) {
	for i, it := range sl.Items {
		y := i*sl.LineSpace + sl.LineSpace - 5 // baseline tweak - moved up to align underline with dots

		// Left side content: bullets for public rooms, player count for private rooms
		leftWidth := 0
		if sl.ShowPlayerCount && !sl.SimpleTextOnly {
			// Private rooms: show player count as text (from item UserCount)
			playerCount := strconv.Itoa(it.UserCount)
			if sl.fontFace != nil {
				text.Draw(img, playerCount, sl.fontFace, 5, y, sl.color)
			}
			leftWidth = sl.measureText(playerCount) + 10 // player count width + spacing
		} else if !sl.SimpleTextOnly {
			// Public rooms: show occupancy bullets
			if sl.Bullets != nil && it.Capacity > 0 {
				// 6 bins for occupancy levels
				bin := int(float64(it.UserCount) / (float64(it.Capacity) / 6.0))
				if bin < 0 {
					bin = 0
				}
				if bin > 6 {
					bin = 6
				}
				if b := sl.Bullets[bin]; b != nil {
					bx := 0                  // bullets start at left edge
					by := i*sl.LineSpace + 2 // slight vertical offset
					op := &ebiten.DrawImageOptions{}
					op.GeoM.Translate(float64(bx), float64(by))
					img.DrawImage(b, op)
					leftWidth = b.Bounds().Dx() + 5 // bullets width + 5px spacing
				}
			}
		}

		// Draw room name with underline (like Lingo: listmember.fontStyle = [#underline])
		name := strings.TrimSpace(it.Name)
		textX := leftWidth
		if sl.SimpleTextOnly {
			if sl.fontFace != nil {
				text.Draw(img, name, sl.fontFace, textX, y, sl.color)
			}
		} else {
			// Draw underlined name, dotted leaders, and Go!
			sl.drawUnderlinedText(img, name, textX, y)
			goText := "Go!"
			goWidth := sl.measureText(goText)
			goX := sl.Rect[2] - goWidth - 6
			nameWidth := sl.measureText(name)
			dotStartX := textX + nameWidth + 5
			dotEndX := goX - 5
			sl.drawDottedLine(img, dotStartX, dotEndX, y)
			sl.drawUnderlinedText(img, goText, goX, y)
		}
	}
}

// renderUserItems renders user list items for people lists
func (sl *ScrollList) renderUserItems(img *ebiten.Image) {
	for i, user := range sl.UserItems {
		y := i*sl.LineSpace + sl.LineSpace - 5 // baseline tweak

		// Draw username (simple text for now)
		textX := 5
		if sl.fontFace != nil {
			text.Draw(img, user.Username, sl.fontFace, textX, y, sl.color)
		}

		// Draw status indicator (simple text for now)
		statusX := sl.Rect[2] - sl.measureText(user.Status) - 5
		if sl.fontFace != nil {
			text.Draw(img, user.Status, sl.fontFace, statusX, y, sl.color)
		}
	}
}

// renderFriendItems renders friend list entries with an online indicator, underlined username,
// dotted leader, and right-aligned "X msgs"
func (sl *ScrollList) renderFriendItems(img *ebiten.Image) {
	// Prebuild a small dark buddy circle (6x6) once per render
	circleW, circleH := 6, 6
	circle := ebiten.NewImage(circleW, circleH)
	// Draw a simple filled circle using midpoint check
	cx, cy := float64(circleW-1)/2.0, float64(circleH-1)/2.0
	r2 := (float64(circleW-1) * float64(circleW-1)) / 4.0
	dark := color.RGBA{0x2A, 0x2A, 0x2A, 0xFF}
	for y := 0; y < circleH; y++ {
		for x := 0; x < circleW; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			if dx*dx+dy*dy <= r2 {
				circle.Set(x, y, dark)
			}
		}
	}

	for i, f := range sl.FriendItems {
		// Baseline similar to room list
		y := i*sl.LineSpace + sl.LineSpace - 5

		leftX := 0
		// Online circle at left
		if f.Online {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(leftX), float64(i*sl.LineSpace+2))
			img.DrawImage(circle, op)
			leftX += circleW + 5
		} else {
			leftX += circleW + 5 // keep alignment
		}

		// Username underlined
		name := strings.TrimSpace(f.Username)
		sl.drawUnderlinedText(img, name, leftX, y)

		// Right-aligned "X msgs"
		rightText := "0 msgs"
		if f.MessageCnt > 0 {
			rightText = strconv.Itoa(f.MessageCnt) + " msgs"
		}
		rightW := sl.measureText(rightText)
		rightX := sl.Rect[2] - rightW - 6

		// Dotted leader between end of name and right text
		nameW := sl.measureText(name)
		dotStart := leftX + nameW + 5
		dotEnd := rightX - 5
		sl.drawDottedLine(img, dotStart, dotEnd, y)

		// Draw right-aligned text
		text.Draw(img, rightText, sl.fontFace, rightX, y, sl.color)
	}
}

func (sl *ScrollList) Draw(screen *ebiten.Image, offsetX, offsetY float64) {
	if sl == nil || sl.Rect[2] <= 0 || sl.Rect[3] <= 0 {
		return
	}
	if sl.offscreen == nil {
		sl.UpdateContent()
	}
	if sl.offscreen == nil {
		return
	}
	// Compute source rect from ScrollIndex
	srcY := (sl.ScrollIndex - 1) * sl.LineSpace
	srcH := sl.Rect[3]
	// Draw visible slice
	sub := sl.offscreen.SubImage(image.Rect(0, srcY, sl.Rect[2], min(srcY+srcH, sl.offscreen.Bounds().Dy()))).(*ebiten.Image)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(offsetX+float64(sl.Rect[0]), offsetY+float64(sl.Rect[1]))
	screen.DrawImage(sub, op)
}

// HitTestRow returns 1-based row index if mouse is inside list, otherwise 0
func (sl *ScrollList) HitTestRow(mouseX, mouseY int, offsetX, offsetY float64) int {
	x0 := int(offsetX) + sl.Rect[0]
	y0 := int(offsetY) + sl.Rect[1]
	w := sl.Rect[2]
	h := sl.Rect[3]
	if mouseX < x0 || mouseX >= x0+w || mouseY < y0 || mouseY >= y0+h {
		return 0
	}
	relY := mouseY - y0
	row := relY/sl.LineSpace + 1
	idx := sl.ScrollIndex + row - 1
	if idx >= 1 && idx <= sl.TotalItems() {
		return idx
	}
	return 0
}

// HitTestRowArea returns the row index and whether the click was in the "Go!" area (matches Lingo roomdisplay logic)
func (sl *ScrollList) HitTestRowArea(mouseX, mouseY int, offsetX, offsetY float64) (int, bool) {
	idx := sl.HitTestRow(mouseX, mouseY, offsetX, offsetY)
	if idx == 0 {
		return 0, false
	}

	// Check if click is in the rightmost 20 pixels (matches Lingo: iMouseLoc[1] >= (iMySpriteRight - 20))
	x0 := int(offsetX) + sl.Rect[0]
	rightEdge := x0 + sl.Rect[2]
	isGoArea := mouseX >= (rightEdge - 20)

	return idx, isGoArea
}

// HandleFriendInput processes mouse clicks for friend lists and invokes callback with username
func (sl *ScrollList) HandleFriendInput(offsetX, offsetY float64, clickCallback func(username string)) {
	if sl == nil || clickCallback == nil {
		return
	}
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		mouseX, mouseY := ebiten.CursorPosition()
		idx := sl.HitTestRow(mouseX, mouseY, offsetX, offsetY)
		if idx > 0 && idx <= len(sl.FriendItems) {
			clickCallback(sl.FriendItems[idx-1].Username)
		}
	}
}

// TotalItems returns the total number of rows for the current list content
func (sl *ScrollList) TotalItems() int {
	if len(sl.Items) > 0 {
		return len(sl.Items)
	}
	if len(sl.UserItems) > 0 {
		return len(sl.UserItems)
	}
	if len(sl.FriendItems) > 0 {
		return len(sl.FriendItems)
	}
	return 0
}

// drawUnderlinedText draws text with an underline (matches Lingo fontStyle = [#underline])
func (sl *ScrollList) drawUnderlinedText(img *ebiten.Image, txt string, x, y int) {
	// Draw the text
	text.Draw(img, txt, sl.fontFace, x, y, sl.color)

	// Draw underline
	textWidth := sl.measureText(txt)
	underlineY := y + 2 // 2 pixels below baseline
	for i := 0; i < textWidth; i++ {
		img.Set(x+i, underlineY, sl.color)
	}
}

// measureText calculates the width of text in pixels
func (sl *ScrollList) measureText(txt string) int {
	bounds := text.BoundString(sl.fontFace, txt)
	return bounds.Dx()
}

// drawDottedLine draws a line of dots between two X positions (simulates Lingo TAB behavior)
// Original Lingo: dotsimg.setPixel(n * 2, 0, rgb(108, 71, 45)) - dots every 2 pixels
func (sl *ScrollList) drawDottedLine(img *ebiten.Image, startX, endX, y int) {
	if endX <= startX {
		return
	}

	dotSpacing := 2 // pixels between dots (matches original Lingo: n * 2)
	dotY := y + 2   // align with underline position (same as underline offset)

	for x := startX; x < endX; x += dotSpacing {
		img.Set(x, dotY, sl.color)
	}
}

// HandleInput processes mouse input directly for the scroll list (bypasses window input complexity)
func (sl *ScrollList) HandleInput(offsetX, offsetY float64, clickCallback func(studioID string, isGoButton bool)) {
	if sl == nil || clickCallback == nil {
		return
	}

	// Check for mouse clicks
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		mouseX, mouseY := ebiten.CursorPosition()

		// Test if click is in scroll list area
		idx, isGoArea := sl.HitTestRowArea(mouseX, mouseY, offsetX, offsetY)
		if idx > 0 && idx <= len(sl.Items) {
			// Valid click on a room row
			studioID := sl.Items[idx-1].StudioID
			log.Printf("ScrollList: direct click on row %d, studio %s, isGoArea=%t", idx, studioID, isGoArea)
			clickCallback(studioID, isGoArea)
		}
	}
}

// Utility
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
