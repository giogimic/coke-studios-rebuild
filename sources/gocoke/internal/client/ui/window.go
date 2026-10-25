// Window management system that replicates Lingo ElementManager behavior
// Handles window layering, z-order, bring-to-front, and overlay management exactly as Lingo did

package ui

import (
	"fmt"
	"image/color"
	"log"
	"path/filepath"
	"strings"

	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/lingo/window"
	textmgr "gocoke/internal/client/assets/text"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
)

// WindowManager replicates Lingo's ElementManager for window management

// ConsoleMessage represents a single received instant message in the Shockwave console
type ConsoleMessage struct {
	ID        int
	Sender    string
	Message   string
	Timestamp string
}

// ConsoleHandlers holds callbacks for authentic Shockwave console interactions
type ConsoleHandlers struct {
	GetFriends      func() []FriendListItem
	GetRequests     func() []string
	GetMessages     func() []ConsoleMessage
	SendMessage     func(to, message string) error
	AcceptRequest   func(from string) error
	RejectRequest   func(from string) error
	RemoveFriend    func(target string) error
	MarkMessageRead func(id int) error
	GetMyInfo       func() (name, mission string, avatarBuf *ebiten.Image)
}

type WindowManager struct {
	openWindows []*Window // Ordered list of open windows (back to front)
	baseZ       int       // Base Z value for window layering (1000000 in Lingo)
	fontManager *FontManager

	// Stage offset so windows share the same origin as the centered background
	stageOffsetX float64
	stageOffsetY float64

	// Scene state hints for style selection
	entryOpen bool

	// Console state
	consoleHandlers       ConsoleHandlers
	selectedConsoleFriend string
	consoleMessagePage    int
}

// Window represents a single UI window with Lingo-style properties
type Window struct {
	Name        string
	Definition  *window.Definition
	Assets      map[string]*ebiten.Image
	Renderer    *ElementRenderer
	Rect        [4]int // [left, top, right, bottom] - Lingo format
	ClientRect  [4]int // client area relative rect (offset and size)
	Visible     bool
	Draggable   bool
	ZOrder      int
	Interactive map[string]*InteractiveElement // ID -> interactive element

	// Scrollable lists keyed by element ID (e.g., "nav_public_rooms_list")
	Lists map[string]*ScrollList
	// Scrollbars extracted from window definition (type contains "scrollbar")
	Scrollbars []scrollbarDef
	// Extra assets lazily loaded (e.g., button.scroll.*)
	ExtraAssets map[string]*ebiten.Image

	// Scrollbar interaction state
	ScrollbarState map[string]*ScrollbarInteraction

	// Callbacks for window-specific interactions (ElementMgr bridging)
	RoomListClick func(studioID string, isGoButton bool) // isGoButton = true for "Go!" area clicks
	GoButtonClick func()                                 // Called when the nav_go_public_button is clicked
	TabTransition func(fromTab, toTab int)               // Called when tab is clicked for window transition
	// Messenger friends list click
	FriendClick func(username string)
	// Generic button click callback (used by catalogue and simple windows)
	ButtonClick func(elementID string)

	// Window frame system (matches Lingo's Windows Script)
	WindowStyle     string                   // e.g., "sanfo_basic.window"
	FrameAssets     map[string]*ebiten.Image // Window frame pieces
	FrameDefinition *window.Definition       // Window frame definition
	Title           string                   // Window title text

	// Baseline content size from frame definition (used for precise stretch/move)
	BaseContentWidth  int
	BaseContentHeight int

	// Tab composition (navigator windows)
	TabsEnabled         bool
	ActiveTab           int // 1-based tab index, e.g., 1=public, 2=private, 3=search
	lastLoggedActiveTab int // Track last logged active tab to avoid spam

	// Frame metrics (derived from frame definition/assets)
	BorderLeftWidth    int
	BorderRightWidth   int
	BorderTopHeight    int
	BorderBottomHeight int
	ShadowLeftWidth    int
	ShadowRightWidth   int
	ShadowTopHeight    int
	ShadowBottomHeight int

	// Shadow stretch extras computed from baseline asset sizes
	ShadowExtraHStretch int // equals top shadow middle width - top window width (e.g., 131-120=11)
	ShadowExtraHVWidth  int // equals center shadow middle width - center window width (e.g., 154-120=34)
	ShadowExtraHVHeight int // equals center shadow middle height - center window height (e.g., 106-64=42)

	// Shadow anchors from frame definition
	ShadowRightTopLocH    int
	ShadowRightBottomLocH int
	ShadowLeftBottomLocV  int

	// Frame origins from definition (baseline offsets)
	FrameOriginX  int // cc.window.lefttop.locH
	FrameOriginY  int // cc.window.lefttop.locV
	ShadowOriginX int // cc.window.shadow.lefttop.locH
	ShadowOriginY int // cc.window.shadow.lefttop.locV

	// Dragging state
	isDragging  bool
	dragMouseDX int
	dragMouseDY int

	// Draggable regions computed from frame definition (elements with id="drag")
	DragRegions []dragRegion

	// Tab clickable regions (computed from content elements)
	TabRegions []tabRegion

	// Stage offset applied during draw and hit-test
	StageOffsetX float64
	StageOffsetY float64

	// Optional tweak to shift the close button horizontally (e.g., studio uses tighter right padding)
	CloseButtonOffsetX int
	// Close button pressed visual state
	closePressed bool

	// Temporary variables for special cases
	TempVars map[string]interface{}
}

type tabRegion struct {
	index int
	rect  [4]int // x, y, w, h in absolute window coordinates
}

type dragRegion struct {
	rect [4]int // x, y, w, h relative to window frame origin
}

// NewWindowManager creates a new window manager with Lingo-style behavior
func NewWindowManager(fontManager *FontManager) *WindowManager {
	return &WindowManager{
		openWindows: make([]*Window, 0),
		baseZ:       1000000, // Match Lingo's base Z value
		fontManager: fontManager,
	}
}


// SetConsoleHandlers registers interaction callbacks for the Shockwave messenger console
func (wm *WindowManager) SetConsoleHandlers(h ConsoleHandlers) {
	wm.consoleHandlers = h
}

// GetConsoleHandlers returns the current console callbacks
func (wm *WindowManager) GetConsoleHandlers() *ConsoleHandlers {
	return &wm.consoleHandlers
}

// OpenConsoleWindow loads and transitions to one of the 6 authentic Shockwave console windows
func (wm *WindowManager) OpenConsoleWindow(name string, prevRect [4]int) *Window {
	messengerDir := filepath.Join("Lingo", "cc_messenger[1]")
	filename := ""
	switch name {
	case "console_myinfo":
		filename = "10_console_myinfo.window.txt"
	case "console_getmessage":
		filename = "11_console_getmessage.window.txt"
	case "console_getrequest":
		filename = "12_console_getrequest.window.txt"
	case "console_friends":
		filename = "13_console_friends.window.txt"
	case "console_compose":
		filename = "14_console_compose.window.txt"
	case "console_removefriend":
		filename = "15_console_removefriend.window.txt"
	default:
		return nil
	}

	defPath := filepath.Join(messengerDir, filename)
	def, err := window.ParseFile(defPath)
	if err != nil {
		log.Printf("WindowManager: failed to parse console window %s: %v", filename, err)
		return nil
	}

	assets := lingoassets.LoadAssetsForDefinition(def, messengerDir)
	extra := lingoassets.LoadAssetsByNames([]string{
		"mmsSHELL1", "mmsSHELL2", "mmsSHELL3_b1dn", "mmsSHELL3_b2dn",
		"MMS_INT_WIN_TOP_LEFT", "MMS_INT_WIN_TOP_MIDDLE", "MMS_INT_WIN_TOP_RIGHT",
		"MMS_INT_WIN_MIDDLE_LEFT", "MMS_INT_WIN_MIDDLE", "MMS_INT_WIN_MIDDLE_RIGHT",
		"MMS_INT_WIN_BOTTOM_LEFT", "MMS_INT_WIN_BOTTOM_MIDDLE", "MMS_INT_WIN_BOTTOM_RIGHT",
		"MMS_INT_CLOSEBUTTON",
		"button.d6.left.active", "button.d6.middle.active", "button.d6.right.active",
		"button.d6.left.pressed", "button.d6.middle.pressed", "button.d6.right.pressed",
		"button.d7.left.active", "button.d7.middle.active", "button.d7.right.active",
		"button.d7.left.pressed", "button.d7.middle.pressed", "button.d7.right.pressed",
	}, messengerDir)
	for k, v := range extra {
		assets[k] = v
	}

	// Close any active console window
	for _, w := range wm.openWindows {
		if strings.HasPrefix(w.Name, "console_") {
			wm.CloseWindow(w.Name)
			break
		}
	}

	newW := wm.openWindowWithStyle(name, def, assets, nil, "")
	if prevRect[2] > prevRect[0] && prevRect[3] > prevRect[1] {
		newW.Rect = prevRect
	} else {
		newW.Rect = def.Rect
	}
	if newW.TempVars == nil {
		newW.TempVars = map[string]interface{}{}
	}
	newW.TempVars["customDragRects"] = [][4]int{{0, 0, def.Rect[2] - def.Rect[0], def.Rect[3] - def.Rect[1]}}

	// Window-specific initialization
	switch name {
	case "console_myinfo":
		if wm.consoleHandlers.GetMyInfo != nil {
			myUser, myMotto, avatarBuf := wm.consoleHandlers.GetMyInfo()
			msgCnt := 0
			if wm.consoleHandlers.GetMessages != nil {
				msgCnt = len(wm.consoleHandlers.GetMessages())
			}
			reqCnt := 0
			if wm.consoleHandlers.GetRequests != nil {
				reqCnt = len(wm.consoleHandlers.GetRequests())
			}
			for i := range newW.Definition.Elements {
				el := &newW.Definition.Elements[i]
				switch el.ID {
				case "console_myinfo_name":
					el.Key = myUser
				case "console_myinfo_mission_field":
					el.Key = myMotto
				case "console_myinfo_messages_link":
					el.Key = fmt.Sprintf("%d new message(s)", msgCnt)
				case "console_myinfo_requests_link":
					el.Key = fmt.Sprintf("%d Friends Request(s)", reqCnt)
				}
			}
			if avatarBuf != nil {
				target := ebiten.NewImage(77, 86)
				op := &ebiten.DrawImageOptions{}
				srcW := avatarBuf.Bounds().Dx()
				srcH := avatarBuf.Bounds().Dy()
				dx := float64((77 - srcW) / 2)
				if dx < 0 {
					dx = 0
				}
				dy := float64(86 - srcH)
				op.GeoM.Translate(dx, dy)
				target.DrawImage(avatarBuf, op)
				newW.Assets["console_myhead_image"] = target
			}
		}

	case "console_friends":
		var friends []FriendListItem
		if wm.consoleHandlers.GetFriends != nil {
			friends = wm.consoleHandlers.GetFriends()
		}
		if list := newW.Lists["console_friends_friendlist"]; list != nil {
			list.FriendItems = friends
			list.UpdateContent()
		}
		newW.FriendClick = func(username string) {
			wm.selectedConsoleFriend = username
			var motto, last string
			for _, f := range friends {
				if f.Username == username {
					motto = f.Motto
					last = f.LastAccess
					break
				}
			}
			for i := range newW.Definition.Elements {
				el := &newW.Definition.Elements[i]
				switch el.ID {
				case "console_friend_sanfo_name_text":
					el.Key = username
				case "console_friend_mission":
					el.Key = motto
				case "console_friend_messages":
					if last != "" {
						el.Key = "Last Access: " + last
					} else {
						el.Key = "Online Now"
					}
				}
			}
		}
		if len(friends) > 0 {
			firstUser := friends[0].Username
			if wm.selectedConsoleFriend != "" {
				for _, f := range friends {
					if f.Username == wm.selectedConsoleFriend {
						firstUser = wm.selectedConsoleFriend
						break
					}
				}
			}
			newW.FriendClick(firstUser)
		}

	case "console_compose":
		target := wm.selectedConsoleFriend
		for i := range newW.Definition.Elements {
			el := &newW.Definition.Elements[i]
			switch el.ID {
			case "console_compose_recipients":
				el.Key = target
			case "console_compose_message_field":
				el.Key = ""
			case "console_compose_lenght_text":
				el.Key = "0/120"
			}
		}

	case "console_removefriend":
		target := wm.selectedConsoleFriend
		for i := range newW.Definition.Elements {
			el := &newW.Definition.Elements[i]
			if el.ID == "console_removefriend_name" {
				el.Key = target
			}
		}

	case "console_getrequest":
		var reqs []string
		if wm.consoleHandlers.GetRequests != nil {
			reqs = wm.consoleHandlers.GetRequests()
		}
		target := ""
		if len(reqs) > 0 {
			target = reqs[0]
		}
		for i := range newW.Definition.Elements {
			el := &newW.Definition.Elements[i]
			if el.ID == "console_getrequest_sanfo_name_text" {
				el.Key = target
			}
		}

	case "console_getmessage":
		var msgs []ConsoleMessage
		if wm.consoleHandlers.GetMessages != nil {
			msgs = wm.consoleHandlers.GetMessages()
		}
		if wm.consoleMessagePage >= len(msgs) {
			wm.consoleMessagePage = 0
		}
		var curMsg ConsoleMessage
		if len(msgs) > 0 {
			curMsg = msgs[wm.consoleMessagePage]
		}
		for i := range newW.Definition.Elements {
			el := &newW.Definition.Elements[i]
			switch el.ID {
			case "console_getmessage_sender":
				el.Key = curMsg.Sender
			case "console_getmessage_field":
				el.Key = curMsg.Message
			case "console_getmessage_date":
				el.Key = curMsg.Timestamp
			}
		}
	}

	return newW
}

// OpenWindow opens a new window or brings existing window to front
// Replicates Lingo's ElementManager.newwindow() -> Windows Script.makedisplay() process
func (wm *WindowManager) OpenWindow(name string, def *window.Definition, assets map[string]*ebiten.Image, tm *textmgr.Manager) *Window {
	// Check if window is already open
	for _, w := range wm.openWindows {
		if w.Name == name {
			wm.BringToFront(w)
			return w
		}
	}

	// Determine window style based on name (matches Lingo's makedisplay() logic)
	windowStyle := wm.determineWindowStyle(name)
	log.Printf("WindowManager: determined window style '%s' for window '%s' (entryOpen=%t)", windowStyle, name, wm.entryOpen)

	// Load window frame definition and assets
	frameDefinition, frameAssets := wm.loadWindowFrame(windowStyle)

	// Create new window
	w := &Window{
		Name:            name,
		Definition:      def,
		Assets:          assets,
		Rect:            def.Rect,
		ClientRect:      def.ClientRect,
		Visible:         true,
		Draggable:       true,
		Interactive:     make(map[string]*InteractiveElement),
		WindowStyle:     windowStyle,
		FrameAssets:     frameAssets,
		FrameDefinition: frameDefinition,
		Title:           wm.determineWindowTitle(name),
	}

	// Compute baseline content size from frame definition
	w.BaseContentWidth, w.BaseContentHeight = w.computeFrameBaselines()

	// Compute frame metrics (border/shadow thickness)
	w.computeFrameMetrics()

	// Compute frame origins for precise anchoring
	w.computeFrameOrigins()

	// Compute draggable regions from frame definition
	w.computeDragRegions()

	// Enable tab composition for navigator windows
	if strings.Contains(name, "nav_") {
		w.TabsEnabled = true
		w.ActiveTab = 1
	}

	// Create element renderer for this window
	w.Renderer = NewElementRenderer(assets, tm)
	w.Renderer.SetFontManager(wm.fontManager)

	// Load extra scrollbar assets not in window definition FIRST
	w.loadScrollbarAssets()

	// Initialize scrolling lists and scrollbars from definition (requires renderer/font and extra assets)
	w.initScrollingLists()

	// Initialize scrollbar interaction state
	w.initScrollbarInteraction()

	// Initialize interactive elements
	w.initializeInteractiveElements()

	// Build tab clickable regions if applicable
	w.computeTabRegions()

	// Compute shadow anchors for precise shadow.pixel stretch (reserved for future)

	// Add to window list and bring to front
	wm.openWindows = append(wm.openWindows, w)
	wm.BringToFront(w)
	if strings.HasPrefix(name, "console_") {
		// Ensure messenger console windows render above other windows
		wm.BringToFront(w)
	}

	log.Printf("WindowManager: opened window %s (style: %s) at [%d, %d, %d, %d]",
		name, windowStyle, w.Rect[0], w.Rect[1], w.Rect[2], w.Rect[3])

	return w
}

// CloseWindow closes a window and removes it from the window list
func (wm *WindowManager) CloseWindow(name string) {
	for i, w := range wm.openWindows {
		if w.Name == name {
			// Remove from list
			wm.openWindows = append(wm.openWindows[:i], wm.openWindows[i+1:]...)

			// Recalculate Z-order for remaining windows
			wm.recalculateZOrder()

			log.Printf("WindowManager: closed window %s", name)
			return
		}
	}
}

// CloseAllWindows closes all open windows (matches Lingo ElementMgr.closeAllWindows)
func (wm *WindowManager) CloseAllWindows() {
	// Close windows in reverse order (front to back)
	for i := len(wm.openWindows) - 1; i >= 0; i-- {
		w := wm.openWindows[i]
		// Skip alert windows (matches Lingo behavior)
		if strings.Contains(w.Name, "sanfo_general_alert") {
			continue
		}
		log.Printf("WindowManager: closing window %s", w.Name)
	}

	// Clear the window list (except alerts)
	var alertWindows []*Window
	for _, w := range wm.openWindows {
		if strings.Contains(w.Name, "sanfo_general_alert") {
			alertWindows = append(alertWindows, w)
		}
	}
	wm.openWindows = alertWindows

	// Recalculate Z-order for remaining windows
	wm.recalculateZOrder()

	log.Printf("WindowManager: closed all windows, %d alert windows remain", len(wm.openWindows))
}

// BringToFront brings a window to the front, matching Lingo's bringtofront() behavior
func (wm *WindowManager) BringToFront(targetWindow *Window) {
	// Find window position
	targetIndex := -1
	for i, w := range wm.openWindows {
		if w == targetWindow {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return // Window not found
	}

	// Move window to end of list (front)
	wm.openWindows = append(wm.openWindows[:targetIndex], wm.openWindows[targetIndex+1:]...)
	wm.openWindows = append(wm.openWindows, targetWindow)

	// Recalculate Z-order for all windows
	wm.recalculateZOrder()

	log.Printf("WindowManager: brought window %s to front", targetWindow.Name)
}

// recalculateZOrder recalculates Z-order for all windows, matching Lingo's algorithm
func (wm *WindowManager) recalculateZOrder() {
	// Reorder open windows so messenger consoles are always last (rendered on top)
	var regular []*Window
	var consoles []*Window
	for _, w := range wm.openWindows {
		if strings.HasPrefix(w.Name, "console_") {
			consoles = append(consoles, w)
		} else {
			regular = append(regular, w)
		}
	}
	if len(consoles) > 0 {
		wm.openWindows = append(regular, consoles...)
	}

	// Lingo algorithm from bringtofront():
	// basez = 1000000
	// counter = 0
	// repeat with n = 1 to count(ElementMgr.pOpenWindows)
	//   repeat with m = 1 to count(ElementMgr.pOpenWindows[n].pSpritelist)
	//     sprite(ElementMgr.pOpenWindows[n].pSpritelist[m]).locZ = basez + counter
	//     counter = counter + 1
	//   end repeat
	// end repeat

	counter := 0
	for _, w := range wm.openWindows {
		// Each window gets a Z range based on its element count
		elementCount := len(w.Definition.Elements)
		if elementCount == 0 {
			elementCount = 1 // Minimum one layer per window
		}

		w.ZOrder = wm.baseZ + counter
		counter += elementCount + 10 // Add padding between windows
	}
}

// Update handles input and updates all windows
func (wm *WindowManager) Update() error {
	// Process windows from front to back for input handling
	// Make a copy of the slice to avoid issues if CloseAllWindows is called during update
	windowsCopy := make([]*Window, len(wm.openWindows))
	copy(windowsCopy, wm.openWindows)

	for i := len(windowsCopy) - 1; i >= 0; i-- {
		w := windowsCopy[i]
		// Check if window is still in the active list (in case it was closed during update)
		stillActive := false
		for _, activeWindow := range wm.openWindows {
			if activeWindow == w {
				stillActive = true
				break
			}
		}

		if stillActive && w.Visible {
			// Handle window input (dragging, button clicks, etc.)
			if err := w.handleInput(wm); err != nil {
				return err
			}
		}
	}

	return nil
}

// Draw renders all windows from back to front
func (wm *WindowManager) Draw(screen *ebiten.Image) {
	// Draw windows from back to front
	for _, w := range wm.openWindows {
		if w.Visible {
			w.draw(screen)
			// Ephemeral overlay: choose_song countdown
			if w.Name == "choose_song" && w.TempVars != nil {
				if label, ok := w.TempVars["choose_song_timer_label"].(string); ok && label != "" {
					if pos, ok2 := w.TempVars["choose_song_timer_pos"].([2]int); ok2 {
						var fontFace font.Face
						if w.Renderer != nil && w.Renderer.fontManager != nil {
							fontFace = w.Renderer.fontManager.GetFont("Verdana", "bold", 10)
						}
						clr := color.RGBA{0x56, 0x42, 0x32, 0xFF}
						x := int(w.StageOffsetX) + w.Rect[0] + pos[0]
						y := int(w.StageOffsetY) + w.Rect[1] + pos[1]
						text.Draw(screen, label, fontFace, x, y, clr)
					}
				}
			}
		}
	}
}

// DrawWithHook renders windows back-to-front and, after drawing the window named hookName,
// invokes hook() to allow callers to draw overlays between specific windows.
func (wm *WindowManager) DrawWithHook(screen *ebiten.Image, hookName string, hook func()) {
	for _, w := range wm.openWindows {
		if w.Visible {
			w.draw(screen)
			if hookName != "" && w.Name == hookName && hook != nil {
				hook()
			}
			// Ephemeral overlay: choose_song countdown (same as Draw)
			if w.Name == "choose_song" && w.TempVars != nil {
				if label, ok := w.TempVars["choose_song_timer_label"].(string); ok && label != "" {
					if pos, ok2 := w.TempVars["choose_song_timer_pos"].([2]int); ok2 {
						var fontFace font.Face
						if w.Renderer != nil && w.Renderer.fontManager != nil {
							fontFace = w.Renderer.fontManager.GetFont("Verdana", "bold", 10)
						}
						clr := color.RGBA{0x56, 0x42, 0x32, 0xFF}
						x := int(w.StageOffsetX) + w.Rect[0] + pos[0]
						y := int(w.StageOffsetY) + w.Rect[1] + pos[1]
						text.Draw(screen, label, fontFace, x, y, clr)
					}
				}
			}
		}
	}
}

// GetWindow returns a window by name
func (wm *WindowManager) GetWindow(name string) *Window {
	for _, w := range wm.openWindows {
		if w.Name == name {
			return w
		}
	}
	return nil
}

// GetOpenWindows returns a list of all open window names
func (wm *WindowManager) GetOpenWindows() []string {
	var names []string
	for _, w := range wm.openWindows {
		names = append(names, w.Name)
	}
	return names
}

// GetOpenWindowObjects returns a list of all open window objects
func (wm *WindowManager) GetOpenWindowObjects() []*Window {
	return wm.openWindows
}

// IsMouseOverAnyWindow returns true if the current mouse position is inside any visible window.
func (wm *WindowManager) IsMouseOverAnyWindow() bool {
	mx, my := ebiten.CursorPosition()
	return wm.IsPointOverAnyWindow(mx, my)
}

// IsPointOverAnyWindow returns true if the given point lies within any visible window's bounds.
func (wm *WindowManager) IsPointOverAnyWindow(x, y int) bool {
	for _, w := range wm.openWindows {
		if w == nil || !w.Visible {
			continue
		}
		left := int(w.StageOffsetX) + w.Rect[0]
		top := int(w.StageOffsetY) + w.Rect[1]
		right := left + (w.Rect[2] - w.Rect[0])
		bottom := top + (w.Rect[3] - w.Rect[1])
		if x >= left && x < right && y >= top && y < bottom {
			return true
		}
	}
	return false
}

// SetStageOffset sets the global stage offset so windows align with centered background
func (wm *WindowManager) SetStageOffset(x, y float64) {
	wm.stageOffsetX = x
	wm.stageOffsetY = y
	// Push into existing windows immediately
	for _, w := range wm.openWindows {
		w.StageOffsetX = x
		w.StageOffsetY = y
	}
}

// TransitionWindow transitions from one navigator window to another, preserving data and position
// Replicates Lingo's generictab.ls behavior
func (wm *WindowManager) TransitionWindow(fromWindow *Window, toWindowName string, def *window.Definition, assets map[string]*ebiten.Image, tm *textmgr.Manager) *Window {
	if fromWindow == nil {
		log.Printf("WindowManager: TransitionWindow called with nil fromWindow")
		return nil
	}

	// Preserve window position and style (matches Lingo: myRect = pParentWindow.prect)
	windowRect := fromWindow.Rect
	originalStyle := fromWindow.WindowStyle

	// Preserve scroll data for room lists (matches Lingo generictab behavior)
	var roomData []RoomListItem
	var roomScrollIndex int = 1
	var roomLastClicked int = 0
	if list := fromWindow.Lists["nav_public_rooms_list"]; list != nil {
		roomData = list.Items
		roomScrollIndex = list.ScrollIndex
		roomLastClicked = list.LastClicked
	}

	// Preserve scroll data for private room lists
	var privateRoomData []RoomListItem
	var privateScrollIndex int = 1
	if list := fromWindow.Lists["nav_private_rooms_list"]; list != nil {
		privateRoomData = list.Items
		privateScrollIndex = list.ScrollIndex
	}

	// Preserve user data for people lists (matches Lingo: userData = pParentWindow.pScrollingLists.userList.pUserData)
	var userData []UserListItem
	var userScrollIndex int = 1
	if list := fromWindow.Lists["nav_people_list"]; list != nil {
		userData = list.UserItems
		userScrollIndex = list.ScrollIndex
	}

	// Close current window (matches Lingo: pParentWindow.closeWindow())
	wm.CloseWindow(fromWindow.Name)

	// Open new window at same position with same style (matches Lingo: ElementMgr.newwindow(pDest, myRect))
	newWindow := wm.openWindowWithStyle(toWindowName, def, assets, tm, originalStyle)
	newWindow.Rect = windowRect // Preserve position
	// Preserve close button horizontal offset across transitions
	newWindow.CloseButtonOffsetX = fromWindow.CloseButtonOffsetX

	// Set the correct active tab based on the destination window type
	if strings.Contains(toWindowName, "nav_public") {
		newWindow.ActiveTab = 1
		log.Printf("WindowManager: set ActiveTab to 1 for public window %s", toWindowName)
	} else if strings.Contains(toWindowName, "nav_private") {
		newWindow.ActiveTab = 2
		log.Printf("WindowManager: set ActiveTab to 2 for private window %s", toWindowName)
	} else if strings.Contains(toWindowName, "nav_search") {
		newWindow.ActiveTab = 3
		log.Printf("WindowManager: set ActiveTab to 3 for search window %s", toWindowName)
	}

	// Restore scroll data to new window
	if len(roomData) > 0 {
		if list := newWindow.Lists["nav_public_rooms_list"]; list != nil {
			list.Items = roomData
			list.ScrollIndex = roomScrollIndex
			list.LastClicked = roomLastClicked
			list.UpdateContent()
		}
	}

	if len(privateRoomData) > 0 {
		if list := newWindow.Lists["nav_private_rooms_list"]; list != nil {
			list.Items = privateRoomData
			list.ScrollIndex = privateScrollIndex
			list.UpdateContent()
		}
	}

	if len(userData) > 0 {
		if list := newWindow.Lists["nav_people_list"]; list != nil {
			list.UserItems = userData
			list.ScrollIndex = userScrollIndex
			list.UpdateContent()
		}
	}

	log.Printf("WindowManager: transitioned from %s to %s (preserved style: %s)", fromWindow.Name, toWindowName, originalStyle)
	return newWindow
}

// openWindowWithStyle opens a window with a specific style (used for transitions)
func (wm *WindowManager) openWindowWithStyle(name string, def *window.Definition, assets map[string]*ebiten.Image, tm *textmgr.Manager, windowStyle string) *Window {
	if wm.GetWindow(name) != nil {
		log.Printf("WindowManager: window %s already exists", name)
		return wm.GetWindow(name)
	}

	// Use the provided window style instead of determining it
	log.Printf("WindowManager: using preserved window style '%s' for window '%s'", windowStyle, name)

	// Load window frame definition and assets using the preserved style
	frameDefinition, frameAssets := wm.loadWindowFrame(windowStyle)

	// Create window with preserved style (same structure as OpenWindow)
	w := &Window{
		Name:            name,
		Definition:      def,
		Assets:          assets,
		Rect:            def.Rect,
		ClientRect:      def.ClientRect,
		Visible:         true,
		Draggable:       true,
		Interactive:     make(map[string]*InteractiveElement),
		WindowStyle:     windowStyle,
		FrameAssets:     frameAssets,
		FrameDefinition: frameDefinition,
		Title:           wm.determineWindowTitle(name),
	}

	// Compute baseline content size from frame definition
	w.BaseContentWidth, w.BaseContentHeight = w.computeFrameBaselines()

	// Compute frame metrics (border/shadow thickness)
	w.computeFrameMetrics()

	// Compute frame origins for precise anchoring
	w.computeFrameOrigins()

	// Compute draggable regions from frame definition
	w.computeDragRegions()

	// Enable tab composition for navigator windows
	if strings.Contains(name, "nav_") {
		w.TabsEnabled = true
		w.ActiveTab = 1
	}

	// Initialize renderer and other components
	w.Renderer = NewElementRenderer(assets, tm)
	w.Renderer.SetFontManager(wm.fontManager)

	// Load extra scrollbar assets (including cc.navi.balls) FIRST
	w.loadScrollbarAssets()

	// Initialize scrolling lists and scrollbars from definition (requires renderer/font and extra assets)
	w.initScrollingLists()

	// Initialize scrollbar interaction state
	w.initScrollbarInteraction()

	w.computeTabRegions()

	// Set stage offset
	w.StageOffsetX = wm.stageOffsetX
	w.StageOffsetY = wm.stageOffsetY

	// Add to window list and bring to front
	wm.openWindows = append(wm.openWindows, w)
	wm.BringToFront(w)

	log.Printf("WindowManager: opened window %s (style: %s) at %v", name, windowStyle, w.Rect)
	return w
}

// SetEntryOpen informs the WindowManager whether the entry scene is open.
// This affects the window style selection for nav_* windows in Lingo.
func (wm *WindowManager) SetEntryOpen(open bool) {
	wm.entryOpen = open
	// FIX stray
}

// determineWindowStyle determines the window style based on window name
// Replicates Lingo's Windows Script.makedisplay() logic
func (wm *WindowManager) determineWindowStyle(windowName string) string {
	// Navigator windows logic from Lingo lines 30-42
	if strings.Contains(windowName, "nav_") {
		if wm.entryOpen {
			return "sanfo_basic.window"
		}
		return "sanfo_basic2.window"
	}

	// Backpack is a custom frameless window (uses its own bg/front assets)
	if strings.Contains(windowName, "cc_backpack") || strings.Contains(windowName, "backpack") {
		return "" // no frame
	}

	// Messenger consoles are frameless and use their own shell assets
	if strings.HasPrefix(windowName, "console_") {
		return "" // no frame for messenger console windows
	}

	// Other window types from Lingo lines 44-105
	if strings.Contains(windowName, "MODERATOR_HELP") {
		return "sanfo_basic.window"
	}
	if strings.Contains(windowName, "callsent") {
		return "sanfo_basic2.window"
	}
	if strings.Contains(windowName, "alert") {
		return "cc_modal.window"
	}
	if strings.Contains(windowName, "balance") ||
		strings.Contains(windowName, "call_for_help") ||
		strings.Contains(windowName, "catalogue") {
		return "sanfo_full.window"
	}
	if windowName == "purchase_info" || windowName == "burn_decision" {
		return "sanfo_basic2.window"
	}
	if windowName == "choose_song" {
		// Choose song should use the same look as navigator (two content boxes)
		return "sanfo_basic.window"
	}

	// Default fallback
	return "sanfo_full.window"
}

// determineWindowTitle determines the window title based on window name
func (wm *WindowManager) determineWindowTitle(windowName string) string {
	if strings.Contains(windowName, "nav_") {
		return "NAV_TITLE"
	}
	if strings.Contains(windowName, "MODERATOR_HELP") {
		return "MODERATOR_HELP_TITLE"
	}
	if strings.Contains(windowName, "alert") || strings.Contains(windowName, "callsent") {
		return "ALERT_TITLE"
	}
	if strings.Contains(windowName, "balance") {
		return "BALANCE_TITLE"
	}
	if strings.Contains(windowName, "call_for_help") {
		return "HELP_TITLE"
	}
	if strings.Contains(windowName, "catalogue") {
		return "CAT_TITLE"
	}
	if windowName == "purchase_info" {
		return "PURCHASE_TITLE"
	}
	if windowName == "paused" {
		return "PAUSED_TITLE"
	}
	if windowName == "choose_song" {
		return "CHOOSE_SONG_TITLE"
	}

	return ""
}

// loadWindowFrame loads the window frame definition and assets
func (wm *WindowManager) loadWindowFrame(windowStyle string) (*window.Definition, map[string]*ebiten.Image) {
	if windowStyle == "" {
		return nil, nil
	}
	// Map window style to actual file names
	var frameFileName string
	switch windowStyle {
	case "sanfo_basic.window":
		frameFileName = "23_sanfo_basic.window.txt"
	case "sanfo_basic2.window":
		frameFileName = "176_sanfo_basic2.window.txt"
	case "sanfo_full.window":
		frameFileName = "25_sanfo_full.window.txt"
	case "cc_modal.window":
		frameFileName = "22_cc_modal.window.txt"
	default:
		log.Printf("WindowManager: unknown window style: %s", windowStyle)
		return nil, nil
	}

	// Load window frame definition
	framePath := filepath.Join("Lingo", "cc_interface[1]", frameFileName)
	frameDefinition, err := window.ParseFile(framePath)
	if err != nil {
		log.Printf("WindowManager: failed to load window frame %s: %v", frameFileName, err)
		return nil, nil
	}

	// Load window frame assets
	frameAssets := make(map[string]*ebiten.Image)
	lingoDir := filepath.Join("Lingo", "cc_interface[1]")
	mm, err := lingoassets.LoadMemberMap(lingoDir)
	if err != nil {
		log.Printf("WindowManager: failed to load frame member map: %v", err)
		return frameDefinition, frameAssets
	}

	// Load all frame assets referenced in the frame definition
	for _, element := range frameDefinition.Elements {
		if element.Media == "bitmap" {
			memberName := element.Member
			if path, ok := mm.PathFor(memberName); ok {
				if img, err := lingoassets.LoadImage(path); err == nil {
					frameAssets[memberName] = img
				} else {
					log.Printf("WindowManager: failed to load frame asset %s: %v", memberName, err)
				}
			} else {
				log.Printf("WindowManager: frame asset not found: %s", memberName)
			}
		}
	}
	// Ensure we have pressed-state closebox available for visual feedback
	if _, ok := frameAssets["closebox.pressed"]; !ok {
		if path, ok := mm.PathFor("closebox.pressed"); ok {
			if img, err := lingoassets.LoadImage(path); err == nil {
				frameAssets["closebox.pressed"] = img
			}
		}
	}

	log.Printf("WindowManager: loaded window frame %s with %d assets", windowStyle, len(frameAssets))
	return frameDefinition, frameAssets
}

// initializeInteractiveElements creates interactive elements from window definition
func (w *Window) initializeInteractiveElements() {
	if w.Definition == nil {
		return
	}

	for _, element := range w.Definition.Elements {
		if element.Media == "bitmap" && element.Type == "image" {
			// Check if this is an interactive element (has multiple states)
			baseName := element.Member
			if w.Assets[baseName+".rollover"] != nil || w.Assets[baseName+".pressed"] != nil {
				ie := w.Renderer.CreateInteractiveElement(element)
				w.Interactive[element.ID] = ie
			}
		}
	}
}

// handleInput processes input for this window
func (w *Window) handleInput(wm *WindowManager) error {
	// Handle interactive elements
	mouseX, mouseY := ebiten.CursorPosition()
	mousePressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)

	// Calculate window offset (Lingo rect format: [left, top, right, bottom])
	// Include client rect offset so hit testing matches draw positions
	// Use window-cached stage offset to avoid race with SetStageOffset timing
	offsetX := w.StageOffsetX + float64(w.Rect[0])
	offsetY := w.StageOffsetY + float64(w.Rect[1])

	// Check if mouse is within window bounds before processing any input
	winLeft := int(offsetX)
	winTop := int(offsetY)
	winRight := winLeft + (w.Rect[2] - w.Rect[0])
	winBottom := winTop + (w.Rect[3] - w.Rect[1])

	mouseInWindow := mouseX >= winLeft && mouseX < winRight && mouseY >= winTop && mouseY < winBottom

	// Debug: Log any mouse clicks, but only process input if mouse is in window
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		log.Printf("Window %s: mouse click at (%d,%d), window bounds [%d,%d,%d,%d], mouseInWindow=%t",
			w.Name, mouseX, mouseY, winLeft, winTop, winRight, winBottom, mouseInWindow)
	}

	// Only process input if mouse is within window bounds
	if !mouseInWindow {
		// Allow close button to work even if it lies outside the content rect (frame area)
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			if w.isMouseInsideClose(mouseX, mouseY) {
				w.closePressed = true
				return nil
			}
		}
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			if w.closePressed && w.isMouseInsideClose(mouseX, mouseY) {
				w.closePressed = false
				wm.CloseWindow(w.Name)
				return nil
			}
			w.closePressed = false
		}

		// Still need to handle dragging even if mouse is outside window bounds
		if w.Draggable && w.isDragging {
			if !mousePressed {
				log.Printf("Window %s: stopped dragging (mouse outside window)", w.Name)
				w.isDragging = false
			} else {
				// Continue dragging even when mouse is outside window
				width := w.Rect[2] - w.Rect[0]
				height := w.Rect[3] - w.Rect[1]
				newLeft := mouseX - w.dragMouseDX
				newTop := mouseY - w.dragMouseDY
				logicalLeft := newLeft - int(w.StageOffsetX)
				logicalTop := newTop - int(w.StageOffsetY)
				w.Rect[0] = logicalLeft
				w.Rect[1] = logicalTop
				w.Rect[2] = logicalLeft + width
				w.Rect[3] = logicalTop + height
			}
		}
		return nil
	}

	// Handle window dragging (replicates Lingo windowBG script behavior)
	// Note: We handle dragging early but allow other interactions to be checked first
	if w.Draggable && w.isDragging {
		// Continue existing drag if already dragging
		if !mousePressed {
			log.Printf("Window %s: stopped dragging", w.Name)
			w.isDragging = false
		} else {
			// Move window preserving size
			width := w.Rect[2] - w.Rect[0]
			height := w.Rect[3] - w.Rect[1]
			newLeft := mouseX - w.dragMouseDX
			newTop := mouseY - w.dragMouseDY
			// Convert back from stage space to logical rect (subtract stage offset)
			logicalLeft := newLeft - int(w.StageOffsetX)
			logicalTop := newTop - int(w.StageOffsetY)
			w.Rect[0] = logicalLeft
			w.Rect[1] = logicalTop
			w.Rect[2] = logicalLeft + width
			w.Rect[3] = logicalTop + height

			// When actively dragging, suppress other interactions to prevent interference
			return nil
		}
	}

	// Direct scroll list input handling (bypasses complex window input nesting)
	if list := w.Lists["nav_public_rooms_list"]; list != nil && w.RoomListClick != nil {
		list.HandleInput(offsetX, offsetY, w.RoomListClick)
	}
	if list := w.Lists["nav_private_rooms_list"]; list != nil && w.RoomListClick != nil {
		list.HandleInput(offsetX, offsetY, w.RoomListClick)
	}
	// Messenger friends list input
	if list := w.Lists["console_friends_friendlist"]; list != nil && w.FriendClick != nil {
		list.HandleFriendInput(offsetX, offsetY, w.FriendClick)
	}

	// Update interactive elements
	for _, ie := range w.Interactive {
		ie.HandleMouseInput(mouseX, mouseY, mousePressed, offsetX, offsetY)
	}

	// Handle Go button clicks (public and private)
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		checkGo := func(id string) bool {
			if goButton := w.Interactive[id]; goButton != nil {
				// Check if click is within Go button bounds
				x := int(offsetX) + goButton.Rect[0]
				y := int(offsetY) + goButton.Rect[1]
				width := goButton.Rect[2]
				height := goButton.Rect[3]
				if mouseX >= x && mouseX < x+width && mouseY >= y && mouseY < y+height {
					log.Printf("Window %s: Go button clicked (%s)!", w.Name, id)
					if w.GoButtonClick != nil {
						w.GoButtonClick()
					}
					return true
				}
			}
			return false
		}
		_ = checkGo("nav_go_public_button") || checkGo("nav_go_private_button")
		// Backpack closebox click handling
		if cb := w.Interactive["cc.backpack.closebox"]; cb != nil {
			x := int(offsetX) + cb.Rect[0]
			y := int(offsetY) + cb.Rect[1]
			width := cb.Rect[2]
			height := cb.Rect[3]
			if mouseX >= x && mouseX < x+width && mouseY >= y && mouseY < y+height {
				// Request an animated popdown instead of immediate close
				if w.TempVars == nil {
					w.TempVars = map[string]interface{}{}
				}
				w.TempVars["requestBackpackPopdown"] = true
				return nil
			}
		}
	}

	// Handle private navigation button clicks (check BEFORE tab clicks to prevent interference)
	buttonClicked := false
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		privateButtons := []string{"nav_priv_popular", "nav_priv_own", "nav_priv_create_own", "nav_go_search_button"}
		for _, buttonID := range privateButtons {
			if button := w.Interactive[buttonID]; button != nil {
				// Check if click is within button bounds
				x := int(offsetX) + button.Rect[0]
				y := int(offsetY) + button.Rect[1]
				width := button.Rect[2]
				height := button.Rect[3]

				if mouseX >= x && mouseX < x+width && mouseY >= y && mouseY < y+height {
					log.Printf("Window %s: Private navigation button clicked: %s", w.Name, buttonID)
					buttonClicked = true

					// Handle button functionality
					switch buttonID {
					case "nav_priv_popular", "nav_priv_own", "nav_priv_create_own", "nav_go_search_button":
						if w.ButtonClick != nil {
							w.ButtonClick(buttonID)
						} else {
							log.Printf("Window %s: Private navigation button clicked but no handler set: %s", w.Name, buttonID)
						}
					}

					break
				}
			}
		}
	}

	// Authentic Shockwave console windows interaction & text composition
	if strings.HasPrefix(w.Name, "console_") {
		// Text input handling for Compose window message field
		if w.Name == "console_compose" {
			for i := range w.Definition.Elements {
				if w.Definition.Elements[i].ID == "console_compose_message_field" {
					el := &w.Definition.Elements[i]
					for _, r := range ebiten.AppendInputChars(nil) {
						if r >= 32 && r <= 126 && len(el.Key) < 120 {
							el.Key += string(r)
						}
					}
					if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
						if len(el.Key) > 0 {
							el.Key = el.Key[:len(el.Key)-1]
						}
					}
					// Update length text indicator (0/120)
					for j := range w.Definition.Elements {
						if w.Definition.Elements[j].ID == "console_compose_lenght_text" {
							w.Definition.Elements[j].Key = fmt.Sprintf("%d/120", len(el.Key))
							break
						}
					}
					break
				}
			}
		}

		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			isClickInElement := func(id string) bool {
				for _, el := range w.Definition.Elements {
					if el.ID == id {
						x := int(w.StageOffsetX) + w.Rect[0] + el.LocH
						y := int(w.StageOffsetY) + w.Rect[1] + el.LocV
						mx, my := ebiten.CursorPosition()
						if mx >= x && mx < x+el.Width && my >= y && my < y+el.Height {
							return true
						}
					}
				}
				return false
			}

			// Close button
			if isClickInElement("close") {
				wm.CloseWindow(w.Name)
				return nil
			}

			// Switch to My Info tab
			if isClickInElement("console.myinfo.button") {
				wm.OpenConsoleWindow("console_myinfo", w.Rect)
				return nil
			}

			// Switch to Friends tab
			if isClickInElement("console.myfriends.button") {
				wm.OpenConsoleWindow("console_friends", w.Rect)
				return nil
			}

			// Window-specific buttons
			switch w.Name {
			case "console_myinfo":
				if isClickInElement("console_myinfo_requests_link") {
					if reqs := wm.consoleHandlers.GetRequests; reqs != nil && len(reqs()) > 0 {
						wm.OpenConsoleWindow("console_getrequest", w.Rect)
						return nil
					}
				}
				if isClickInElement("console_myinfo_messages_link") {
					if msgs := wm.consoleHandlers.GetMessages; msgs != nil && len(msgs()) > 0 {
						wm.consoleMessagePage = 0
						wm.OpenConsoleWindow("console_getmessage", w.Rect)
						return nil
					}
				}

			case "console_friends":
				if isClickInElement("messenger_friends_compose_button") {
					if wm.selectedConsoleFriend != "" {
						wm.OpenConsoleWindow("console_compose", w.Rect)
						return nil
					}
				}
				if isClickInElement("messenger_friends_remove_button") {
					if wm.selectedConsoleFriend != "" {
						wm.OpenConsoleWindow("console_removefriend", w.Rect)
						return nil
					}
				}

			case "console_compose":
				if isClickInElement("console_compose_cancel") {
					wm.OpenConsoleWindow("console_friends", w.Rect)
					return nil
				}
				if isClickInElement("console_compose_send") {
					to := ""
					msg := ""
					for _, el := range w.Definition.Elements {
						if el.ID == "console_compose_recipients" {
							to = el.Key
						}
						if el.ID == "console_compose_message_field" {
							msg = el.Key
						}
					}
					if to != "" && msg != "" && wm.consoleHandlers.SendMessage != nil {
						_ = wm.consoleHandlers.SendMessage(to, msg)
					}
					wm.OpenConsoleWindow("console_friends", w.Rect)
					return nil
				}

			case "console_removefriend":
				if isClickInElement("console_getfriendrequest_cancel") {
					wm.OpenConsoleWindow("console_friends", w.Rect)
					return nil
				}
				if isClickInElement("console_friendrequest_remove") {
					target := ""
					for _, el := range w.Definition.Elements {
						if el.ID == "console_removefriend_name" {
							target = el.Key
						}
					}
					if target != "" && wm.consoleHandlers.RemoveFriend != nil {
						_ = wm.consoleHandlers.RemoveFriend(target)
						wm.selectedConsoleFriend = ""
					}
					wm.OpenConsoleWindow("console_friends", w.Rect)
					return nil
				}

			case "console_getrequest":
				if isClickInElement("console_friendrequest_accept") {
					from := ""
					for _, el := range w.Definition.Elements {
						if el.ID == "console_getrequest_sanfo_name_text" {
							from = el.Key
						}
					}
					if from != "" && wm.consoleHandlers.AcceptRequest != nil {
						_ = wm.consoleHandlers.AcceptRequest(from)
					}
					wm.OpenConsoleWindow("console_friends", w.Rect)
					return nil
				}
				if isClickInElement("console_getfriendrequest_reject") {
					from := ""
					for _, el := range w.Definition.Elements {
						if el.ID == "console_getrequest_sanfo_name_text" {
							from = el.Key
						}
					}
					if from != "" && wm.consoleHandlers.RejectRequest != nil {
						_ = wm.consoleHandlers.RejectRequest(from)
					}
					wm.OpenConsoleWindow("console_myinfo", w.Rect)
					return nil
				}

			case "console_getmessage":
				if isClickInElement("console_getmessage_reply") {
					sender := ""
					for _, el := range w.Definition.Elements {
						if el.ID == "console_getmessage_sender" {
							sender = el.Key
						}
					}
					if sender != "" {
						wm.selectedConsoleFriend = sender
						wm.OpenConsoleWindow("console_compose", w.Rect)
						return nil
					}
				}
				if isClickInElement("nextmessage") {
					if msgs := wm.consoleHandlers.GetMessages; msgs != nil {
						list := msgs()
						if len(list) > 0 && wm.consoleMessagePage < len(list) {
							curID := list[wm.consoleMessagePage].ID
							if wm.consoleHandlers.MarkMessageRead != nil {
								_ = wm.consoleHandlers.MarkMessageRead(curID)
							}
							if wm.consoleMessagePage+1 < len(list) {
								wm.consoleMessagePage++
								wm.OpenConsoleWindow("console_getmessage", w.Rect)
								return nil
							}
						}
					}
					wm.consoleMessagePage = 0
					wm.OpenConsoleWindow("console_myinfo", w.Rect)
					return nil
				}
			}
		}
	}

	// Generic interactive button click handling for simple windows (e.g., catalogue)
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) && w.ButtonClick != nil {
		for id, ie := range w.Interactive {
			// Limit to known simple IDs to avoid overlap with navigator internals
			if !(strings.HasPrefix(id, "catalog_") || strings.HasPrefix(id, "cat_") || strings.HasPrefix(id, "buy_") || id == "wall_ok" || id == "floor_ok" || id == "open_ranklist" || id == "purchase_decision_ok" || id == "sanfo_decision_cancel") {
				continue
			}
			x := int(offsetX) + ie.Rect[0]
			y := int(offsetY) + ie.Rect[1]
			width := ie.Rect[2]
			height := ie.Rect[3]
			if mouseX >= x && mouseX < x+width && mouseY >= y && mouseY < y+height {
				w.ButtonClick(id)
				break
			}
		}
	}

	// Close button handling (frame-level interactive)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if w.isMouseInsideClose(mouseX, mouseY) {
			w.closePressed = true
			return nil
		}
	}
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		if w.closePressed && w.isMouseInsideClose(mouseX, mouseY) {
			w.closePressed = false
			wm.CloseWindow(w.Name)
			return nil
		}
		w.closePressed = false
	}

	// Scrollbar interaction handling (matches Lingo scrollbar behavior)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		// Check scrollbar mouse down first (higher priority than other interactions)
		if w.handleScrollbarMouseDown(mouseX, mouseY) {
			return nil // Scrollbar handled the click, don't process other interactions
		}
	}

	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		w.handleScrollbarMouseUp()
	}

	// Handle scrollbar thumb dragging
	if mousePressed {
		w.handleScrollbarMouseMove(mouseX, mouseY)
	}

	// Tab clicks: detect if mouse pressed over any tab region (skip if button was clicked)
	if w.TabsEnabled && !buttonClicked && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		log.Printf("Window %s: Tab click detected at (%d,%d), checking %d tab regions", w.Name, mouseX, mouseY, len(w.TabRegions))
		for i, r := range w.TabRegions {
			// Tab regions are relative to window content area, so add window position + client offset
			x0 := int(w.StageOffsetX) + w.Rect[0] + w.ClientRect[0] + r.rect[0]
			y0 := int(w.StageOffsetY) + w.Rect[1] + w.ClientRect[1] + r.rect[1]
			x1 := x0 + r.rect[2]
			y1 := y0 + r.rect[3]
			log.Printf("Window %s: Tab region %d (index %d): [%d,%d,%d,%d]", w.Name, i, r.index, x0, y0, x1, y1)
			if mouseX >= x0 && mouseX < x1 && mouseY >= y0 && mouseY < y1 {
				log.Printf("Window %s: *** TAB CLICK HIT *** on tab %d (current ActiveTab: %d)", w.Name, r.index, w.ActiveTab)
				if r.index != w.ActiveTab {
					oldTab := w.ActiveTab
					w.ActiveTab = r.index
					log.Printf("Window %s: Tab transition from %d to %d", w.Name, oldTab, r.index)
					// Call tab transition callback if set (matches Lingo generictab behavior)
					if w.TabTransition != nil {
						log.Printf("Window %s: Calling TabTransition callback", w.Name)
						w.TabTransition(oldTab, r.index)
					} else {
						log.Printf("Window %s: TabTransition callback is nil!", w.Name)
					}
				} else {
					log.Printf("Window %s: Tab click on already active tab %d - no transition needed", w.Name, r.index)
				}
				break
			}
		}

		// Scrollbar interaction mapping (Lingo windows script behavior)
		// Handle mouse wheel for all room lists
		_, wheelY := ebiten.Wheel()
		if wheelY != 0 {
			step := 1
			if wheelY < 0 {
				step = 1
			} else {
				step = -1
			}

			// Apply to public rooms list
			if list := w.Lists["nav_public_rooms_list"]; list != nil {
				list.ScrollIndex = max(1, min(list.ScrollIndex+step, max(1, len(list.Items)-list.DisplayLines+1)))
				list.UpdateContent()
			}
			// Apply to private rooms list
			if list := w.Lists["nav_private_rooms_list"]; list != nil {
				list.ScrollIndex = max(1, min(list.ScrollIndex+step, max(1, len(list.Items)-list.DisplayLines+1)))
				list.UpdateContent()
			}
			// Apply to people list
			if list := w.Lists["nav_people_list"]; list != nil {
				totalItems := len(list.UserItems)
				if totalItems > 0 {
					list.ScrollIndex = max(1, min(list.ScrollIndex+step, max(1, totalItems-list.DisplayLines+1)))
					list.UpdateContent()
				}
			}
			// Apply to friends list
			if list := w.Lists["console_friends_friendlist"]; list != nil {
				totalItems := len(list.FriendItems)
				if totalItems > 0 {
					list.ScrollIndex = max(1, min(list.ScrollIndex+step, max(1, totalItems-list.DisplayLines+1)))
					list.UpdateContent()
				}
			}
		}

		// Row click handling for room lists (matches Lingo roomdisplay script)
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			// Check public rooms list
			if list := w.Lists["nav_public_rooms_list"]; list != nil {
				log.Printf("Window %s: checking public list click at (%d,%d), offset=(%f,%f), list.Rect=%v", w.Name, mouseX, mouseY, offsetX, offsetY, list.Rect)
				idx, isGoArea := list.HitTestRowArea(mouseX, mouseY, offsetX, offsetY)
				log.Printf("Window %s: mouse click at (%d,%d), public list hit test returned idx=%d, isGoArea=%t", w.Name, mouseX, mouseY, idx, isGoArea)
				if idx > 0 && idx <= len(list.Items) {
					log.Printf("Window %s: valid row click on public item %s", w.Name, list.Items[idx-1].StudioID)
					if w.RoomListClick != nil {
						log.Printf("Window %s: calling RoomListClick callback for public room", w.Name)
						w.RoomListClick(list.Items[idx-1].StudioID, isGoArea)
					} else {
						log.Printf("Window %s: RoomListClick callback is nil!", w.Name)
					}
				}
			}
			// Check private rooms list
			if list := w.Lists["nav_private_rooms_list"]; list != nil {
				log.Printf("Window %s: checking private list click at (%d,%d), offset=(%f,%f), list.Rect=%v", w.Name, mouseX, mouseY, offsetX, offsetY, list.Rect)
				idx, isGoArea := list.HitTestRowArea(mouseX, mouseY, offsetX, offsetY)
				log.Printf("Window %s: mouse click at (%d,%d), private list hit test returned idx=%d, isGoArea=%t", w.Name, mouseX, mouseY, idx, isGoArea)
				if idx > 0 && idx <= len(list.Items) {
					log.Printf("Window %s: valid row click on private item %s", w.Name, list.Items[idx-1].StudioID)
					if w.RoomListClick != nil {
						log.Printf("Window %s: calling RoomListClick callback for private room", w.Name)
						w.RoomListClick(list.Items[idx-1].StudioID, isGoArea)
					} else {
						log.Printf("Window %s: RoomListClick callback is nil!", w.Name)
					}
				}
			}
			// TODO: Add people list click handling when needed
			// Friend list row click handling
			if list := w.Lists["console_friends_friendlist"]; list != nil && w.FriendClick != nil {
				idx := list.HitTestRow(mouseX, mouseY, offsetX, offsetY)
				if idx > 0 && idx <= len(list.FriendItems) {
					w.FriendClick(list.FriendItems[idx-1].Username)
				}
			}
		}

	}

	// TODO: Tab switching input mapping (placeholder using number keys)
	if w.TabsEnabled {
		// ebiten input is polled at a higher level; keep minimal here
		// Future: promote a message/event for tab buttons to set w.ActiveTab
	}

	// Handle drag initiation LAST (only if no other interactions were triggered)
	// This ensures tabs, buttons, scrollbars, etc. take priority over dragging
	if w.Draggable && !w.isDragging && mousePressed && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		winLeft := int(w.StageOffsetX) + w.Rect[0]
		winTop := int(w.StageOffsetY) + w.Rect[1]

		// Check if mouse is inside any drag region (elements with id="drag")
		insideDrag := false
		// Allow custom drag rects from TempVars (relative to content)
		if w.TempVars != nil {
			if rects, ok := w.TempVars["customDragRects"].([][4]int); ok {
				for _, r := range rects {
					rx := winLeft + r[0]
					ry := winTop + r[1]
					rw := r[2]
					rh := r[3]
					if mouseX >= rx && mouseX < rx+rw && mouseY >= ry && mouseY < ry+rh {
						insideDrag = true
						break
					}
				}
			}
		}
		// Fall back to frame DragRegions
		if !insideDrag {
			for _, region := range w.DragRegions {
				// Convert drag region to absolute screen coordinates
				// Frame origin is at (winLeft + FrameOriginX, winTop + FrameOriginY)
				frameX := winLeft + w.FrameOriginX
				frameY := winTop + w.FrameOriginY
				dragX0 := frameX + region.rect[0]
				dragY0 := frameY + region.rect[1]
				dragX1 := dragX0 + region.rect[2]
				dragY1 := dragY0 + region.rect[3]

				if mouseX >= dragX0 && mouseX < dragX1 && mouseY >= dragY0 && mouseY < dragY1 {
					insideDrag = true
					break
				}
			}
		}

		if insideDrag {
			// Start dragging; remember mouse offset from window origin
			w.isDragging = true
			w.dragMouseDX = mouseX - winLeft
			w.dragMouseDY = mouseY - winTop
			log.Printf("Window %s: started dragging at (%d,%d), offset=(%d,%d)", w.Name, mouseX, mouseY, w.dragMouseDX, w.dragMouseDY)
		}
	}

	return nil
}

// draw renders this window using the two-layer system (frame + content)
// Replicates Lingo's Windows Script rendering process
func (w *Window) draw(screen *ebiten.Image) {
	if w.Definition == nil {
		return
	}

	// CRITICAL: Lingo window rect is [left, top, right, bottom]; apply stage offset
	windowLeft := w.StageOffsetX + float64(w.Rect[0])
	windowTop := w.StageOffsetY + float64(w.Rect[1])
	windowRight := float64(w.Rect[2])
	windowBottom := float64(w.Rect[3])
	_ = windowRight
	_ = windowBottom

	// STEP 1: Draw window frame (matches Lingo's renderBG() process) unless frameless
	if w.FrameDefinition != nil && w.FrameAssets != nil {
		// Lingo passes prect (window content rect) to renderBG; use w.Rect dimensions
		contentWidth := float64(w.Rect[2] - w.Rect[0])
		contentHeight := float64(w.Rect[3] - w.Rect[1])
		w.drawWindowFrame(screen, windowLeft, windowTop, contentWidth, contentHeight)
	}

	// STEP 2: Draw content elements inside the frame's client area
	// In Lingo, content origin is exactly at prect.left/top
	clientOffsetX := windowLeft
	clientOffsetY := windowTop
	w.drawWindowContent(screen, clientOffsetX, clientOffsetY)
}

// drawWindowFrame renders the window frame using dynamic sizing (matches Lingo renderBG)
// Ensures proper layer ordering: shadows (blend:25) first, then borders (blend:100)
func (w *Window) drawWindowFrame(screen *ebiten.Image, x, y, width, height float64) {
	if w.FrameDefinition == nil || w.FrameAssets == nil {
		return
	}

	// Create frame renderer with frame assets
	frameRenderer := NewElementRenderer(w.FrameAssets, nil)
	frameRenderer.SetFontManager(w.Renderer.fontManager)

	// Anchor: place the composed frame so that element locH/locV are relative to
	// the top-left of the composed image at (prect.left - leftBorder, prect.top - topBorder)
	baseX := x - float64(w.BorderLeftWidth)
	baseY := y - float64(w.BorderTopHeight)

	// Separate elements by layer (shadow vs window frame) for correct rendering order
	backElements := []window.Element{}
	shadowElements := []window.Element{}
	windowElements := []window.Element{}

	for _, element := range w.FrameDefinition.Elements {
		// Skip non-bitmap elements here (e.g., frame title text). We'll draw title separately.
		if element.Media != "bitmap" {
			continue
		}

		// Separate by element ID and blend level
		if element.ID == "drag" {
			backElements = append(backElements, element)
		} else if element.Blend == 25 { // Shadow elements
			shadowElements = append(shadowElements, element)
		} else { // Window frame elements (blend 100)
			windowElements = append(windowElements, element)
		}
	}

	// STEP 1: Render shadow elements first (behind window frame)
	for _, element := range shadowElements {
		dynamicElement := element
		w.applyDynamicSizing(&dynamicElement, width, height)
		// Apply optional close button offset
		if dynamicElement.ID == "close" {
			dynamicElement.LocH += w.CloseButtonOffsetX
			if w.closePressed {
				// Use pressed visual if available
				dynamicElement.Member = "closebox.pressed"
			}
		}
		frameRenderer.RenderElement(screen, dynamicElement, baseX, baseY)
	}

	// STEP 2: Render all window frame elements (blend 100) on top of shadows
	frameElements := append([]window.Element{}, backElements...)
	frameElements = append(frameElements, windowElements...)
	for _, element := range frameElements {
		dynamicElement := element
		w.applyDynamicSizing(&dynamicElement, width, height)
		// Apply optional close button offset and pressed visual
		if dynamicElement.ID == "close" {
			dynamicElement.LocH += w.CloseButtonOffsetX
			if w.closePressed {
				dynamicElement.Member = "closebox.pressed"
			}
		}
		frameRenderer.RenderElement(screen, dynamicElement, baseX, baseY)
	}

	// STEP 3: Render window title if present (on top of everything)
	if w.Title != "" {
		// Center title across the content width starting at prect.left (not baseX)
		w.drawWindowTitle(screen, baseX+float64(w.BorderLeftWidth), baseY, width)
	}
}

// applyDynamicSizing applies Lingo's dynamic window frame sizing logic
// Replicates the exact logic from 76_windows script.ls lines 714-754
func (w *Window) applyDynamicSizing(element *window.Element, contentWidth, contentHeight float64) {
	strech := element.Strech
	myRectWidth := int(contentWidth)
	myRectHeight := int(contentHeight)

	// Helper: read flags immediately following a token, e.g., "strechHV" or "moveV"
	parseFlags := func(s, token string) (hasH, hasV bool) {
		idx := strings.Index(s, token)
		if idx == -1 {
			return false, false
		}
		i := idx + len(token)
		for i < len(s) {
			c := s[i]
			if c == 'H' {
				hasH = true
				i++
				continue
			}
			if c == 'V' {
				hasV = true
				i++
				continue
			}
			break
		}
		return
	}

	strechH, strechV := parseFlags(strech, "strech")
	moveH, moveV := parseFlags(strech, "move")
	isShadow := strings.Contains(element.Member, "shadow")

	// Get shadow reference points (matches Lingo lines 709-712)
	shadowRight := 0
	if w.FrameDefinition != nil {
		for _, e := range w.FrameDefinition.Elements {
			if e.Member == "cc.window.shadow.lefttop" {
				shadowRight = e.LocH + e.Width // 0 + 9 = 9
				break
			}
		}
	}

	// Apply stretch logic (matches Lingo lines 714-731), using flags after "strech"
	if strechH || strechV {
		if strechH {
			if isShadow {
				if strechV { // center shadow area (HV stretch)
					element.Width = myRectWidth + w.ShadowExtraHVWidth
				} else { // top/bottom shadow rows
					element.Width = myRectWidth + w.ShadowExtraHStretch
				}
			} else {
				element.Width = myRectWidth
			}
		}
		if strechV {
			if isShadow {
				element.Height = myRectHeight + w.ShadowExtraHVHeight
			} else {
				element.Height = myRectHeight
			}
		}
	}

	// Apply move logic (matches Lingo lines 733-754), using flags after "move"
	if moveH || moveV {
		if moveH {
			if isShadow {
				element.LocH = shadowRight + myRectWidth + w.ShadowExtraHStretch
			} else {
				element.LocH = w.BorderLeftWidth + myRectWidth
			}
		}
		if moveV {
			if isShadow {
				element.LocV = myRectHeight + w.ShadowExtraHVHeight + w.ShadowTopHeight
			} else {
				element.LocV = w.BorderTopHeight + myRectHeight
			}
		}
	}
}

// computeFrameBaselines inspects the frame definition to get the baseline content size
func (w *Window) computeFrameBaselines() (int, int) {
	baseW := 0
	baseH := 0
	if w.FrameDefinition != nil {
		for _, e := range w.FrameDefinition.Elements {
			if e.Member == "cc.window.top" && e.Width > 0 {
				baseW = e.Width
			}
			if e.Member == "cc.window.center" && e.Height > 0 {
				baseH = e.Height
			}
		}
	}
	// Fallback to client rect size if specific pieces not found
	if baseW == 0 {
		baseW = w.ClientRect[2] - w.ClientRect[0]
	}
	if baseH == 0 {
		baseH = w.ClientRect[3] - w.ClientRect[1]
	}
	return baseW, baseH
}

// drawWindowContent renders the window content elements
func (w *Window) drawWindowContent(screen *ebiten.Image, offsetX, offsetY float64) {
	// Build a set of base names that have active variants to suppress passive duplicates
	activeBases := map[string]struct{}{}
	for _, el := range w.Definition.Elements {
		if el.Media == "bitmap" && strings.Contains(el.Member, ".active") {
			base := strings.TrimSuffix(el.Member, ".active")
			activeBases[base] = struct{}{}
		}
	}

	// Defer certain elements to ensure correct stacking (e.g., avatar portrait on top)
	var deferred []window.Element

	// Draw all content elements at their exact positions relative to window origin
	for _, element := range w.Definition.Elements {
		// Hide CD icon until a song is selected (uses TempVars flag set by scene)
		if w.Name == "choose_song" && element.ID == "song_choose_cdicon" {
			if w.TempVars != nil {
				if hide, ok := w.TempVars["hide_cd_icon"].(bool); ok && hide {
					continue
				}
			}
		}
		// Tab composition filtering for navigator windows
		if w.TabsEnabled {
			// Determine tab index from member naming convention cc.tab{n}.*
			if strings.HasPrefix(element.Member, "cc.tab") {
				// For now, show all cc.tab1.* elements - let Lingo positioning handle the layout
				// The original Lingo system must handle tab visibility differently than expected
				if w.lastLoggedActiveTab != w.ActiveTab {
					log.Printf("Window %s: showing tab element %s (all tabs visible)", w.Name, element.Member)
				}
			}
			// For text labels that identify tabs in navigator window
			if element.Media == "text" && (element.Key == "NAV_USER_VIEW_TITLE" || element.Key == "NAV_WORLD_VIEW_TITLE" || element.Key == "NAV_VEGOS_VIEW_TITLE") {
				// Show only the active one by index mapping
				// 1: NAV_USER_VIEW_TITLE, 2: NAV_WORLD_VIEW_TITLE, 3: NAV_VEGOS_VIEW_TITLE
				want := 0
				switch element.Key {
				case "NAV_USER_VIEW_TITLE":
					want = 1
				case "NAV_WORLD_VIEW_TITLE":
					want = 2
				case "NAV_VEGOS_VIEW_TITLE":
					want = 3
				}
				if want != 0 && want != w.ActiveTab {
					continue
				}
			}
		}

		// Skip interactive elements - they're handled separately
		if _, isInteractive := w.Interactive[element.ID]; isInteractive {
			continue
		}

		// Skip most shadow.pixel elements used for lists/scrollbars, but allow dynamic content like catalogue items list
		if element.Member == "shadow.pixel" && (element.Type == "image" || strings.Contains(element.Type, "scrollbar")) {
			if element.ID != "texts.items" && element.ID != "console_myhead_image" {
				// These elements define interaction areas for lists/scrollbars but should not render black backgrounds
				continue
			}
		}

		// Defer avatar portrait to draw above any box pieces
		if element.ID == "console_myhead_image" {
			deferred = append(deferred, element)
			continue
		}

		// Backpack: control front panel
		if w.Name == "cc_backpack" && element.Member == "cc.backpack.front" {
			// If dropped, don't draw the full front anymore
			if w.TempVars != nil {
				if dropped, ok := w.TempVars["backpackFrontDropped"].(bool); ok && dropped {
					continue
				}
				// During popup, apply temporary upward offset
				if v, ok := w.TempVars["backpackFrontOffsetV"]; ok {
					if dv, ok2 := v.(int); ok2 {
						element.LocV = element.LocV - dv
					}
				}
			}
		}

		// Render using the content element renderer
		w.Renderer.RenderElement(screen, element, offsetX, offsetY)
	}

	// After base content, draw all lists
	for _, list := range w.Lists {
		if list != nil {
			list.Draw(screen, offsetX, offsetY)
		}
	}
	// Draw scrollbar(s) overlay
	w.drawScrollbars(screen, offsetX, offsetY)

	// Draw any deferred elements last to ensure they are on top
	for _, element := range deferred {
		w.Renderer.RenderElement(screen, element, offsetX, offsetY)
	}

	// If backpack front is dropped, draw the small front-down strip aligned to bottom
	if w.Name == "cc_backpack" && w.TempVars != nil {
		if dropped, ok := w.TempVars["backpackFrontDropped"].(bool); ok && dropped {
			if img, ok := w.Assets["cc.backpack.front.down"]; ok && img != nil {
				// X position: align with cc.backpack.front locH if present, else 7
				x := 7
				for _, e := range w.Definition.Elements {
					if e.Member == "cc.backpack.front" {
						x = e.LocH
						break
					}
				}
				b := img.Bounds()
				// Y position: bottom of window content area plus optional offset
				winHeight := w.Rect[3] - w.Rect[1]
				y := winHeight - b.Dy()
				if off, ok := w.TempVars["backpackFrontDownOffset"].(int); ok {
					y += off
				}
				if y < 0 {
					y = 0
				}
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(offsetX+float64(x), offsetY+float64(y))
				screen.DrawImage(img, op)
			}
		}
	}

	// Update logged active tab to prevent spam
	if w.lastLoggedActiveTab != w.ActiveTab {
		w.lastLoggedActiveTab = w.ActiveTab
	}

	// Draw interactive elements in their current states
	for _, ie := range w.Interactive {
		w.Renderer.RenderInteractiveElement(screen, ie, offsetX, offsetY)
	}
}

// drawWindowTitle renders the window title text
func (w *Window) drawWindowTitle(screen *ebiten.Image, x, y, width float64) {
	if w.Renderer == nil || w.Renderer.fontManager == nil {
		return
	}

	// Title style derived from frame definition (Verdana, bold, 12, color #FCEDD9)
	fontFace := w.Renderer.fontManager.GetFont("Verdana", "bold", 12)
	if fontFace == nil {
		return
	}

	// Look up localized title text key
	textKey := w.Title
	// Use the renderer's text manager if available
	if w.Renderer.tm == nil {
		return
	}
	content := w.Renderer.tm.First(textKey)
	if content == "" {
		content = textKey
	}

	// Compute position roughly matching frame def (#locH:46, #locV:13) and center horizontally
	titleY := y + 13 + float64(12) // baseline adjust by font size

	// Center across content width
	bounds := text.BoundString(fontFace, content)
	tw := float64(bounds.Dx())
	tx := x + (width-tw)/2

	// Draw
	text.Draw(screen, content, fontFace, int(tx), int(titleY), color.RGBA{0xFC, 0xED, 0xD9, 0xFF})
}

// computeTabRegions determines clickable tab regions from content element slices
func (w *Window) computeTabRegions() {
	w.TabRegions = nil
	if !w.TabsEnabled || w.Definition == nil {
		log.Printf("Window %s: computeTabRegions skipped (TabsEnabled=%t, Definition=%v)", w.Name, w.TabsEnabled, w.Definition != nil)
		return
	}
	log.Printf("Window %s: Computing tab regions...", w.Name)
	// For navigator windows, we need to identify tabs by their text labels and positions
	// since all tabs use cc.tab1.* elements but are positioned differently
	type tabInfo struct {
		index                  int
		minX, minY, maxX, maxY int
		inited                 bool
	}

	var tabs []*tabInfo

	// Find text elements that identify the tabs
	log.Printf("Window %s: Scanning %d elements for tab text labels", w.Name, len(w.Definition.Elements))
	for _, e := range w.Definition.Elements {
		log.Printf("Window %s: Element - Media: %s, Type: %s, Key: %s", w.Name, e.Media, e.Type, e.Key)
		if e.Media == "field" && e.Type == "text" {
			var tabIndex int
			switch e.Key {
			case "NAV_WORLD_VIEW_TITLE": // Public tab
				tabIndex = 1
			case "NAV_USER_VIEW_TITLE": // Private/User Studios tab
				tabIndex = 2
			case "NAV_VEGOS_VIEW_TITLE": // V-egos tab
				tabIndex = 3
			default:
				log.Printf("Window %s: Unrecognized tab key: %s", w.Name, e.Key)
				continue
			}

			// Create tab region based on text position and reasonable click area
			// Use relative coordinates (relative to window content area)
			x := e.LocH - 10        // extend left
			y := e.LocV - 10        // extend up
			width := e.Width + 20   // extend right
			height := e.Height + 20 // extend down

			tab := &tabInfo{
				index:  tabIndex,
				minX:   x,
				minY:   y,
				maxX:   x + width,
				maxY:   y + height,
				inited: true,
			}
			tabs = append(tabs, tab)
			log.Printf("Window %s: Found tab %d (%s) at text position [%d,%d,%d,%d]", w.Name, tabIndex, e.Key, e.LocH, e.LocV, e.Width, e.Height)
		}
	}
	for _, tab := range tabs {
		if tab == nil || !tab.inited {
			continue
		}
		region := tabRegion{
			index: tab.index,
			rect:  [4]int{tab.minX, tab.minY, tab.maxX - tab.minX, tab.maxY - tab.minY},
		}
		w.TabRegions = append(w.TabRegions, region)
		log.Printf("Window %s: Tab region %d: [%d,%d,%d,%d]", w.Name, tab.index, region.rect[0], region.rect[1], region.rect[2], region.rect[3])
	}
	log.Printf("Window %s: Computed %d tab regions", w.Name, len(w.TabRegions))
}

// computeFrameMetrics extracts border/shadow thickness from frame definition/assets
func (w *Window) computeFrameMetrics() {
	if w.FrameDefinition == nil {
		return
	}
	// Prefer actual image dimensions if available
	getImgSize := func(name string) (int, int, bool) {
		if img, ok := w.FrameAssets[name]; ok && img != nil {
			b := img.Bounds()
			return b.Dx(), b.Dy(), true
		}
		return 0, 0, false
	}

	for _, e := range w.FrameDefinition.Elements {
		switch e.Member {
		case "cc.window.left":
			if w.BorderLeftWidth == 0 && e.Width > 0 {
				w.BorderLeftWidth = e.Width
			}
		case "cc.window.right":
			if w.BorderRightWidth == 0 && e.Width > 0 {
				w.BorderRightWidth = e.Width
			}
		case "cc.window.top":
			if w.BorderTopHeight == 0 && e.Height > 0 {
				w.BorderTopHeight = e.Height
			}
		case "cc.window.bottom":
			if w.BorderBottomHeight == 0 && e.Height > 0 {
				w.BorderBottomHeight = e.Height
			}
		case "cc.window.shadow.lefttop":
			if w.ShadowLeftWidth == 0 && e.Width > 0 {
				w.ShadowLeftWidth = e.Width
			}
			if w.ShadowTopHeight == 0 && e.Height > 0 {
				w.ShadowTopHeight = e.Height
			}
		case "cc.window.shadow.righttop":
			if w.ShadowRightWidth == 0 && e.Width > 0 {
				w.ShadowRightWidth = e.Width
			}
		case "cc.window.shadow.leftbottom":
			if w.ShadowBottomHeight == 0 && e.Height > 0 {
				w.ShadowBottomHeight = e.Height
			}
		}
	}

	// Fill remaining from actual images
	if w.BorderLeftWidth == 0 {
		if dx, _, ok := getImgSize("cc.window.left"); ok {
			w.BorderLeftWidth = dx
		}
	}
	if w.BorderRightWidth == 0 {
		if dx, _, ok := getImgSize("cc.window.right"); ok {
			w.BorderRightWidth = dx
		}
	}
	if w.BorderTopHeight == 0 {
		if _, dy, ok := getImgSize("cc.window.top"); ok {
			w.BorderTopHeight = dy
		}
	}
	if w.BorderBottomHeight == 0 {
		if _, dy, ok := getImgSize("cc.window.bottom"); ok {
			w.BorderBottomHeight = dy
		}
	}
	if w.ShadowLeftWidth == 0 || w.ShadowTopHeight == 0 {
		if dx, dy, ok := getImgSize("cc.window.shadow.lefttop"); ok {
			if w.ShadowLeftWidth == 0 {
				w.ShadowLeftWidth = dx
			}
			if w.ShadowTopHeight == 0 {
				w.ShadowTopHeight = dy
			}
		}
	}
	if w.ShadowRightWidth == 0 {
		if dx, _, ok := getImgSize("cc.window.shadow.righttop"); ok {
			w.ShadowRightWidth = dx
		}
	}
	if w.ShadowBottomHeight == 0 {
		if _, dy, ok := getImgSize("cc.window.shadow.leftbottom"); ok {
			w.ShadowBottomHeight = dy
		}
	}

	// Compute shadow stretch extras from actual assets (matches Lingo window definition)
	// From sanfo_basic.window.txt: shadow.pixel top row width=131, cc.window.top width=120 → extra=11
	if shadowPixelW, _, ok := getImgSize("shadow.pixel"); ok {
		if windowTopW, _, ok2 := getImgSize("cc.window.top"); ok2 {
			w.ShadowExtraHStretch = shadowPixelW - windowTopW // Should be 131-120=11
		}
	}
	// Fallback to Lingo specification if assets not available
	if w.ShadowExtraHStretch == 0 {
		w.ShadowExtraHStretch = 11 // From Lingo analysis
	}

	// For HV shadow extras, use the frame definition values
	// From sanfo_basic.window.txt: shadow.pixel HV width=154, height=106 vs content 120x64
	if w.FrameDefinition != nil {
		for _, e := range w.FrameDefinition.Elements {
			if e.Member == "shadow.pixel" && e.LocH == 0 && e.LocV == 11 { // HV stretch piece
				if windowCenterW, windowCenterH, ok := getImgSize("cc.window.center"); ok {
					w.ShadowExtraHVWidth = e.Width - windowCenterW   // Should be 154-120=34
					w.ShadowExtraHVHeight = e.Height - windowCenterH // Should be 106-64=42
				}
				break
			}
		}
	}
	// Fallback to Lingo specification
	if w.ShadowExtraHVWidth == 0 {
		w.ShadowExtraHVWidth = 34 // From Lingo analysis
	}
	if w.ShadowExtraHVHeight == 0 {
		w.ShadowExtraHVHeight = 42 // From Lingo analysis
	}
}

// computeFrameOrigins extracts the baseline locH/locV for top-left pieces
func (w *Window) computeFrameOrigins() {
	if w.FrameDefinition == nil {
		return
	}
	for _, e := range w.FrameDefinition.Elements {
		switch e.Member {
		case "cc.window.lefttop":
			w.FrameOriginX = e.LocH
			w.FrameOriginY = e.LocV
		case "cc.window.shadow.lefttop":
			w.ShadowOriginX = e.LocH
			w.ShadowOriginY = e.LocV
		}
	}
}

// computeDragRegions extracts draggable areas from frame definition (elements with id="drag")
// This replicates Lingo's windowBG script behavior where pDefList contains drag regions
func (w *Window) computeDragRegions() {
	w.DragRegions = nil
	if w.FrameDefinition == nil {
		return
	}

	// Find all elements with id="drag" and compute their final positions after dynamic sizing
	for _, e := range w.FrameDefinition.Elements {
		if e.ID == "drag" {
			// Apply the same dynamic sizing logic as in applyDynamicSizing
			element := e // Make a copy to avoid modifying the original
			w.applyDynamicSizingToElement(&element)

			// Create drag region relative to frame origin
			region := dragRegion{
				rect: [4]int{
					element.LocH - w.FrameOriginX, // x relative to frame
					element.LocV - w.FrameOriginY, // y relative to frame
					element.Width,                 // width
					element.Height,                // height
				},
			}
			w.DragRegions = append(w.DragRegions, region)
		}
	}

	log.Printf("Window %s: computed %d drag regions", w.Name, len(w.DragRegions))
}

// applyDynamicSizingToElement applies the same dynamic sizing logic as applyDynamicSizing to a single element
func (w *Window) applyDynamicSizingToElement(element *window.Element) {
	// Extract the same logic from applyDynamicSizing for a single element
	myRectWidth := w.Rect[2] - w.Rect[0]
	myRectHeight := w.Rect[3] - w.Rect[1]

	// Parse stretch flags (same logic as in applyDynamicSizing)
	strech := strings.ToLower(element.Strech)
	strechH := strings.Contains(strech, "strechh")
	strechV := strings.Contains(strech, "strechv")
	moveH := strings.Contains(strech, "moveh")
	moveV := strings.Contains(strech, "movev")

	// Check if this is a shadow element
	isShadow := strings.Contains(element.Member, "shadow")

	// Apply stretch logic (matches Lingo lines 717-732)
	if strechH || strechV {
		if strechH {
			if isShadow {
				shadowRight := w.ShadowOriginX + myRectWidth + w.ShadowExtraHStretch
				element.Width = shadowRight - element.LocH
			} else {
				element.Width = w.BorderLeftWidth + myRectWidth - element.LocH
			}
		}
		if strechV {
			if isShadow {
				element.Height = myRectHeight + w.ShadowExtraHVHeight + w.ShadowTopHeight - element.LocV
			} else {
				element.Height = w.BorderTopHeight + myRectHeight - element.LocV
			}
		}
	}

	// Apply move logic (matches Lingo lines 733-754)
	if moveH || moveV {
		if moveH {
			if isShadow {
				shadowRight := w.ShadowOriginX + myRectWidth + w.ShadowExtraHStretch
				element.LocH = shadowRight + myRectWidth + w.ShadowExtraHStretch
			} else {
				element.LocH = w.BorderLeftWidth + myRectWidth
			}
		}
		if moveV {
			if isShadow {
				element.LocV = myRectHeight + w.ShadowExtraHVHeight + w.ShadowTopHeight
			} else {
				element.LocV = w.BorderTopHeight + myRectHeight
			}
		}
	}
}

// --- Navigator Scroll Lists & Scrollbars ---

type scrollbarDef struct {
	Model  int
	Client string
	Rect   [4]int // x,y,w,h for bar area; arrows/lift computed from assets
}

// ScrollbarInteraction tracks the state of scrollbar mouse interactions (matches Lingo behavior)
type ScrollbarInteraction struct {
	// Component rectangles (absolute screen coordinates)
	UpArrowRect   [4]int // [x, y, w, h]
	DownArrowRect [4]int
	TrackRect     [4]int
	ThumbRect     [4]int

	// Interaction state
	DraggingThumb bool
	ClickedTrack  int // 0=none, 1=below thumb (page down), 2=above thumb (page up)
	ClickedArrow  int // 0=none, 1=up, 2=down
	DragOffset    int // Y offset from thumb top to mouse when dragging started

	// Visual state for asset switching
	UpArrowPressed   bool
	DownArrowPressed bool
	ThumbPressed     bool
}

func (w *Window) initScrollingLists() {
	w.Lists = make(map[string]*ScrollList)
	// Build scrollbars list for later input handling (optional)
	w.Scrollbars = nil

	if w.Definition == nil || w.Renderer == nil || w.Renderer.fontManager == nil {
		return
	}

	// Gather list regions (image elements with specific IDs)
	for _, e := range w.Definition.Elements {
		if e.Media == "bitmap" && e.Type == "image" {
			switch e.ID {
			case "nav_public_rooms_list":
				// Create public room list with bullets
				face := w.Renderer.fontManager.GetFont("Verdana", "plain", 10)
				clr := color.RGBA{0x6C, 0x47, 0x2D, 0xFF}
				list := NewScrollList(e.LocH, e.LocV, e.Width, e.Height, 14, face, clr)
				list.ShowPlayerCount = false // Use bullets for public rooms
				// Load occupancy bullets 0..6 from ExtraAssets
				bullets := map[int]*ebiten.Image{}
				for i := 0; i <= 6; i++ {
					name := fmt.Sprintf("cc.navi.balls.%d", i)
					// Check both regular assets and extra assets
					if img := w.Assets[name]; img != nil {
						bullets[i] = img
						log.Printf("Window %s: loaded bullet %s from Assets", w.Name, name)
					} else if img := w.ExtraAssets[name]; img != nil {
						bullets[i] = img
						log.Printf("Window %s: loaded bullet %s from ExtraAssets", w.Name, name)
					} else {
						log.Printf("Window %s: bullet %s not found in Assets or ExtraAssets", w.Name, name)
					}
				}
				list.Bullets = bullets
				log.Printf("Window %s: public room list created with %d bullets", w.Name, len(bullets))
				w.Lists[e.ID] = list
			case "nav_private_rooms_list":
				// Create private room list with player count numbers
				face := w.Renderer.fontManager.GetFont("Verdana", "plain", 10)
				clr := color.RGBA{0x6C, 0x47, 0x2D, 0xFF}
				list := NewScrollList(e.LocH, e.LocV, e.Width, e.Height, 14, face, clr)
				list.ShowPlayerCount = true // Use player count numbers for private rooms
				// No bullets needed for private rooms
				w.Lists[e.ID] = list
			case "nav_people_list":
				// Create people list with smaller line spacing for user lists
				face := w.Renderer.fontManager.GetFont("Verdana", "plain", 10)
				clr := color.RGBA{0x6C, 0x47, 0x2D, 0xFF}
				list := NewScrollList(e.LocH, e.LocV, e.Width, e.Height, 10, face, clr) // 10px line spacing for people
				w.Lists[e.ID] = list
			case "console_friends_friendlist":
				// Messenger friends list: dotted leader format with online circle and right text
				face := w.Renderer.fontManager.GetFont("Verdana", "plain", 10)
				clr := color.RGBA{0x6C, 0x47, 0x2D, 0xFF}
				list := NewScrollList(e.LocH, e.LocV, e.Width, e.Height, 14, face, clr)
				w.Lists[e.ID] = list
			}
		}
	}
	// Record scrollbar bands
	for _, e := range w.Definition.Elements {
		if strings.Contains(e.Type, "scrollbar") && e.Client != "" {
			sb := scrollbarDef{Model: e.Model, Client: e.Client, Rect: [4]int{e.LocH, e.LocV, e.Width, e.Height}}
			w.Scrollbars = append(w.Scrollbars, sb)
		}
	}
}

// drawScrollbars renders model-1 vertical scrollbar with proper Lingo assets and interaction states
func (w *Window) drawScrollbars(screen *ebiten.Image, offsetX, offsetY float64) {
	for _, sb := range w.Scrollbars {
		list := w.Lists[sb.Client]
		if list == nil || len(list.Items) == 0 {
			continue
		}

		// Update geometry for interaction
		w.updateScrollbarGeometry(sb, offsetX, offsetY)
		state := w.ScrollbarState[sb.Client]

		// Get scrollbar assets (choose based on interaction state)
		upBtnAsset := "button.scroll.up.passive"
		downBtnAsset := "button.scroll.down.passive"
		if state != nil {
			if state.UpArrowPressed {
				upBtnAsset = "button.scroll.up.active"
			}
			if state.DownArrowPressed {
				downBtnAsset = "button.scroll.down.active"
			}
		}

		upBtn := w.ExtraAssets[upBtnAsset]
		downBtn := w.ExtraAssets[downBtnAsset]
		trackAsset := w.ExtraAssets["scrollbar.vertical.active"]

		baseX := int(offsetX) + sb.Rect[0]
		baseY := int(offsetY) + sb.Rect[1]

		// Draw up arrow button
		if upBtn != nil {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(baseX), float64(baseY))
			screen.DrawImage(upBtn, op)
		}

		// Calculate track area (between up and down buttons)
		upH := 0
		downH := 0
		if upBtn != nil {
			upH = upBtn.Bounds().Dy()
		}
		if downBtn != nil {
			downH = downBtn.Bounds().Dy()
		}

		trackY := baseY + upH
		trackH := sb.Rect[3] - upH - downH

		// Draw track background
		if trackAsset != nil && trackH > 0 {
			// Scale track asset to fit the track area
			op := &ebiten.DrawImageOptions{}
			scaleY := float64(trackH) / float64(trackAsset.Bounds().Dy())
			op.GeoM.Scale(1.0, scaleY)
			op.GeoM.Translate(float64(baseX), float64(trackY))
			screen.DrawImage(trackAsset, op)
		}

		// Draw down arrow button
		if downBtn != nil {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(baseX), float64(baseY+sb.Rect[3]-downH))
			screen.DrawImage(downBtn, op)
		}

		// Draw thumb/slider
		if trackH > 0 && state != nil {
			thumbRect := state.ThumbRect
			thumbColor := color.RGBA{0xFC, 0xED, 0xD9, 0xFF} // light UI color
			if state.ThumbPressed {
				thumbColor = color.RGBA{0xE0, 0xD0, 0xB0, 0xFF} // darker when pressed
			}

			thumbImg := ebiten.NewImage(thumbRect[2], thumbRect[3])
			thumbImg.Fill(thumbColor)
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(thumbRect[0]), float64(thumbRect[1]))
			screen.DrawImage(thumbImg, op)
		}
	}
}

// loadScrollbarAssets loads additional assets needed for scrollbars that aren't in window definitions
func (w *Window) loadScrollbarAssets() {
	if w.ExtraAssets == nil {
		w.ExtraAssets = make(map[string]*ebiten.Image)
	}

	// Assets needed for model 1 scrollbars (button prefix) and occupancy balls
	scrollbarAssets := []string{
		"button.scroll.up.passive",
		"button.scroll.up.active",
		"button.scroll.down.passive",
		"button.scroll.down.active",
		"scrollbar.vertical.active",
		"cc.navi.balls.0",
		"cc.navi.balls.1",
		"cc.navi.balls.2",
		"cc.navi.balls.3",
		"cc.navi.balls.4",
		"cc.navi.balls.5",
		"cc.navi.balls.6",
	}

	// Use the proper asset loading system with member maps
	searchDirs := []string{"Lingo/cc_navigator[1]", "Lingo/cc_interface[1]"}
	extraAssets := lingoassets.LoadAssetsByNames(scrollbarAssets, searchDirs...)

	// Merge into ExtraAssets
	for name, img := range extraAssets {
		w.ExtraAssets[name] = img
		log.Printf("Window: loaded extra asset %s", name)
	}
}

// isMouseInsideClose computes the current close button rect based on frame definition and client size
func (w *Window) isMouseInsideClose(mouseX, mouseY int) bool {
	if w.FrameDefinition == nil {
		// Frameless windows (e.g., messenger consoles) may have a content-level close element
		if w.Definition == nil {
			return false
		}
		// Find an element with id "close" in content and hit-test it
		for i := range w.Definition.Elements {
			el := w.Definition.Elements[i]
			if el.ID == "close" && el.Media == "bitmap" {
				left := int(w.StageOffsetX) + w.Rect[0] + el.LocH
				top := int(w.StageOffsetY) + w.Rect[1] + el.LocV
				right := left + el.Width
				bottom := top + el.Height
				if mouseX >= left && mouseX < right && mouseY >= top && mouseY < bottom {
					return true
				}
				break
			}
		}
		return false
	}
	// Find the close element definition
	var closeElem *window.Element
	for i := range w.FrameDefinition.Elements {
		if w.FrameDefinition.Elements[i].ID == "close" && w.FrameDefinition.Elements[i].Media == "bitmap" {
			closeElem = &w.FrameDefinition.Elements[i]
			break
		}
	}
	if closeElem == nil {
		return false
	}

	// Apply the same dynamic sizing used during draw for precise hit testing
	dynamic := *closeElem
	contentWidth := float64(w.Rect[2] - w.Rect[0])
	contentHeight := float64(w.Rect[3] - w.Rect[1])
	w.applyDynamicSizing(&dynamic, contentWidth, contentHeight)

	// Base anchor used in drawWindowFrame (prect.left/top - border thickness), then apply stage offset
	baseX := int(w.StageOffsetX) + w.Rect[0] - w.BorderLeftWidth
	baseY := int(w.StageOffsetY) + w.Rect[1] - w.BorderTopHeight

	x := baseX + dynamic.LocH + w.CloseButtonOffsetX
	y := baseY + dynamic.LocV
	width := dynamic.Width
	height := dynamic.Height
	return mouseX >= x && mouseX < x+width && mouseY >= y && mouseY < y+height
}

// initScrollbarInteraction sets up interaction state for all scrollbars
func (w *Window) initScrollbarInteraction() {
	if w.ScrollbarState == nil {
		w.ScrollbarState = make(map[string]*ScrollbarInteraction)
	}

	for _, sb := range w.Scrollbars {
		w.ScrollbarState[sb.Client] = &ScrollbarInteraction{}
	}
}

// updateScrollbarGeometry calculates component rectangles for scrollbar interaction
func (w *Window) updateScrollbarGeometry(sb scrollbarDef, offsetX, offsetY float64) {
	state := w.ScrollbarState[sb.Client]
	if state == nil {
		return
	}

	// Get asset dimensions
	upBtn := w.ExtraAssets["button.scroll.up.passive"]
	downBtn := w.ExtraAssets["button.scroll.down.passive"]

	upH := 0
	downH := 0
	if upBtn != nil {
		upH = upBtn.Bounds().Dy()
	}
	if downBtn != nil {
		downH = downBtn.Bounds().Dy()
	}

	baseX := int(offsetX) + sb.Rect[0]
	baseY := int(offsetY) + sb.Rect[1]

	// Up arrow rectangle
	state.UpArrowRect = [4]int{baseX, baseY, sb.Rect[2], upH}

	// Down arrow rectangle
	state.DownArrowRect = [4]int{baseX, baseY + sb.Rect[3] - downH, sb.Rect[2], downH}

	// Track rectangle (between arrows)
	trackY := baseY + upH
	trackH := sb.Rect[3] - upH - downH
	state.TrackRect = [4]int{baseX, trackY, sb.Rect[2], trackH}

	// Thumb rectangle (calculated based on list scroll position)
	list := w.Lists[sb.Client]
	if list != nil && len(list.Items) > 0 {
		maxStart := max(1, len(list.Items)-list.DisplayLines+1)
		frac := float64(list.DisplayLines) / float64(len(list.Items))
		if frac > 1 {
			frac = 1
		}
		thumbH := int(frac * float64(trackH))
		if thumbH < 8 {
			thumbH = 8
		}

		// Thumb position based on ScrollIndex
		posFrac := 0.0
		if maxStart > 1 {
			posFrac = float64(list.ScrollIndex-1) / float64(maxStart-1)
		}
		thumbY := trackY + int(posFrac*float64(trackH-thumbH))

		state.ThumbRect = [4]int{baseX, thumbY, sb.Rect[2], thumbH}
	}
}

// handleScrollbarMouseDown processes mouse down events on scrollbars (matches Lingo behavior)
func (w *Window) handleScrollbarMouseDown(mouseX, mouseY int) bool {
	for _, sb := range w.Scrollbars {
		state := w.ScrollbarState[sb.Client]
		list := w.Lists[sb.Client]
		if state == nil || list == nil || len(list.Items) == 0 {
			continue
		}

		// Check if mouse is in any scrollbar component
		if w.pointInRect(mouseX, mouseY, state.UpArrowRect) {
			// Up arrow clicked - scroll up one line
			state.ClickedArrow = 1
			state.UpArrowPressed = true
			if list.ScrollIndex > 1 {
				list.ScrollIndex--
				list.UpdateContent()
			}
			return true
		}

		if w.pointInRect(mouseX, mouseY, state.DownArrowRect) {
			// Down arrow clicked - scroll down one line
			state.ClickedArrow = 2
			state.DownArrowPressed = true
			maxStart := max(1, len(list.Items)-list.DisplayLines+1)
			if list.ScrollIndex < maxStart {
				list.ScrollIndex++
				list.UpdateContent()
			}
			return true
		}

		if w.pointInRect(mouseX, mouseY, state.ThumbRect) {
			// Thumb clicked - start dragging
			state.DraggingThumb = true
			state.ThumbPressed = true
			state.DragOffset = mouseY - state.ThumbRect[1]
			return true
		}

		if w.pointInRect(mouseX, mouseY, state.TrackRect) {
			// Track clicked - page up/down
			if mouseY < state.ThumbRect[1] {
				// Clicked above thumb - page up
				state.ClickedTrack = 2
				newIndex := list.ScrollIndex - list.DisplayLines
				list.ScrollIndex = max(1, newIndex)
			} else if mouseY > state.ThumbRect[1]+state.ThumbRect[3] {
				// Clicked below thumb - page down
				state.ClickedTrack = 1
				maxStart := max(1, len(list.Items)-list.DisplayLines+1)
				newIndex := list.ScrollIndex + list.DisplayLines
				list.ScrollIndex = min(maxStart, newIndex)
			}
			list.UpdateContent()
			return true
		}
	}
	return false
}

// handleScrollbarMouseUp processes mouse up events on scrollbars
func (w *Window) handleScrollbarMouseUp() {
	for _, state := range w.ScrollbarState {
		state.DraggingThumb = false
		state.ClickedTrack = 0
		state.ClickedArrow = 0
		state.UpArrowPressed = false
		state.DownArrowPressed = false
		state.ThumbPressed = false
	}
}

// handleScrollbarMouseMove processes mouse move events for thumb dragging
func (w *Window) handleScrollbarMouseMove(mouseX, mouseY int) {
	for _, sb := range w.Scrollbars {
		state := w.ScrollbarState[sb.Client]
		list := w.Lists[sb.Client]
		if state == nil || list == nil || !state.DraggingThumb {
			continue
		}

		// Calculate new thumb position
		newThumbY := mouseY - state.DragOffset
		trackRect := state.TrackRect

		// Constrain thumb to track bounds
		minY := trackRect[1]
		maxY := trackRect[1] + trackRect[3] - state.ThumbRect[3]
		newThumbY = max(minY, min(maxY, newThumbY))

		// Convert thumb position to scroll index (matches Lingo formula)
		if trackRect[3] > state.ThumbRect[3] {
			maxStart := max(1, len(list.Items)-list.DisplayLines+1)
			relativePos := float64(newThumbY-minY) / float64(maxY-minY)
			newIndex := int(relativePos*float64(maxStart-1)) + 1
			newIndex = max(1, min(maxStart, newIndex))

			if newIndex != list.ScrollIndex {
				list.ScrollIndex = newIndex
				list.UpdateContent()
			}
		}
	}
}

// pointInRect checks if a point is inside a rectangle [x, y, w, h]
func (w *Window) pointInRect(px, py int, rect [4]int) bool {
	return px >= rect[0] && px < rect[0]+rect[2] && py >= rect[1] && py < rect[1]+rect[3]
}
