package studio

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"

	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/lingo/window"
	"gocoke/internal/client/assets/studios"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/ui"
	"gocoke/internal/client/ui/navigator"
)

// HUDManager manages the studio's bottom bar UI system
type HUDManager struct {
	// Window definition and rendering
	windowDef       *window.Definition
	assets          map[string]*ebiten.Image
	elementRenderer *ui.ElementRenderer
	fontManager     *ui.FontManager
	buttons         map[string]*ui.InteractiveElement // keyed by element ID

	// Positioning and layout
	offsetX int
	offsetY int

	// Tooltip system
	tooltipRect  [4]int
	rolloverText string

	// Chat integration (rectangles for chat manager setup)
	chatFieldRect    [4]int
	speechSwitchRect [4]int

	// Window manager for overlays (navigator, etc.)
	windowManager *ui.WindowManager

	// Dependencies
	tm           *textmgr.Manager
	catalogueManager *CatalogueManager

	// Navigator reuse from entry
	studioList    *studios.StudioList
	getScreenName func() string
	getGatewayURL func() string
	getSecret     func() string
	enterStudio   func(studioID string)

	// Messenger toggle state
	messengerOpen bool

	// Player decibel balance (simple client-side, replace with server later)
	balance int

	// Purchase handler (imageBase, price) -> success
	purchaseHandler func(imageBase string, price int) bool

	// Sound state
	soundMuted bool
}

// ToggleSound flips the sound mute state and returns the new state (true if muted).
func (h *HUDManager) ToggleSound() bool {
	h.soundMuted = !h.soundMuted
	// Swap interactive element images so the icon changes immediately
	if ie, ok := h.buttons["cc.bottombtn.sound"]; ok && ie != nil {
		if h.soundMuted {
			if img := h.assets["cc.bottombtn.soundoff.active"]; img != nil {
				ie.Assets[ui.ButtonActive] = img
			}
			if img := h.assets["cc.bottombtn.soundoff.rollover"]; img != nil {
				ie.Assets[ui.ButtonRollover] = img
			}
			if img := h.assets["cc.bottombtn.soundoff.pressed"]; img != nil {
				ie.Assets[ui.ButtonPressed] = img
			}
		} else {
			if img := h.assets["cc.bottombtn.sound.active"]; img != nil {
				ie.Assets[ui.ButtonActive] = img
			}
			if img := h.assets["cc.bottombtn.sound.rollover"]; img != nil {
				ie.Assets[ui.ButtonRollover] = img
			}
			if img := h.assets["cc.bottombtn.sound.pressed"]; img != nil {
				ie.Assets[ui.ButtonPressed] = img
			}
		}
	}
	return h.soundMuted
}

// IsSoundMuted reports current mute state.
func (h *HUDManager) IsSoundMuted() bool { return h.soundMuted }

// NewHUDManager creates a new HUD manager for the studio scene
func NewHUDManager(tm *textmgr.Manager) *HUDManager {
	return &HUDManager{
		tm:      tm,
		assets:  make(map[string]*ebiten.Image),
		buttons: make(map[string]*ui.InteractiveElement),
		offsetX: 0,
		offsetY: -75, // move HUD up as requested
		balance: 0,
	}
}

// Initialize loads and prepares the bottom bar UI for the studio scene
func (h *HUDManager) Initialize() error {
	// Load window definition for room bar
	windowPath := filepath.Join("Lingo", "cc_room", "62_room_bar.window.txt")
	def, err := window.ParseFile(windowPath)
	if err != nil {
		return fmt.Errorf("failed to load HUD window definition: %w", err)
	}
	h.windowDef = def

	// Capture tooltip rect and other UI element rectangles
	var chatFieldRect, speechSwitchRect [4]int
	for _, el := range def.Elements {
		if el.ID == "room_tooltip_text" {
			h.tooltipRect = [4]int{el.LocH, el.LocV, el.Width, el.Height}
		}
		if el.ID == "cc.speechswitch" {
			speechSwitchRect = [4]int{el.LocH, el.LocV, el.Width, el.Height}
		}
		if el.ID == "chat_field" {
			chatFieldRect = [4]int{el.LocH, el.LocV, el.Width, el.Height}
		}
	}

	// Store rectangles for chat manager initialization
	h.chatFieldRect = chatFieldRect
	h.speechSwitchRect = speechSwitchRect

	// Load bitmap assets from cc_interface[1]
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, interfaceDir)

	// Load rollover/pressed variants for bottom buttons explicitly (not referenced by def)
	buttonBases := []string{
		"cc.bottombtn.jukebox",
		"cc.bottombtn.navi",
		"cc.bottombtn.dbs",
		"cc.bottombtn.bag",
		"cc.bottombtn.catalogue",
		"cc.bottombtn.sound",
		"cc.bottombtn.soundoff",
		"cc.bottombtn.help",
		"cc.bottombtn.mms",
	}
	var names []string
	for _, base := range buttonBases {
		names = append(names, base+".active")
		names = append(names, base+".rollover")
		names = append(names, base+".pressed")
	}
	// Include flash state for MMS if present
	names = append(names, "cc.bottombtn.mms.flash")
	// Include speech switch states
	names = append(names, "cc.speechswitch.speak", "cc.speechswitch.mousedown", "cc.speechswitch.shout", "cc.speechswitch.sing")
	more := lingoassets.LoadAssetsByNames(names, interfaceDir)
	for k, v := range more {
		assets[k] = v
	}

	// Ensure explicit sound on/off button assets are present (active/rollover/pressed)
	extraSound := lingoassets.LoadAssetsByNames([]string{
		"cc.bottombtn.sound.active", "cc.bottombtn.sound.rollover", "cc.bottombtn.sound.pressed",
		"cc.bottombtn.soundoff.active", "cc.bottombtn.soundoff.rollover", "cc.bottombtn.soundoff.pressed",
	}, interfaceDir)
	for k, v := range extraSound {
		assets[k] = v
	}

	// Store and build renderer
	h.assets = assets
	h.fontManager = ui.NewFontManager()
	h.fontManager.PreloadCommonFonts()
	h.elementRenderer = ui.NewElementRenderer(h.assets, h.tm)
	h.elementRenderer.SetFontManager(h.fontManager)

	// Window manager for overlays (navigator)
	h.windowManager = ui.NewWindowManager(h.fontManager)
	// Studio scene uses absolute stage coords; no centering offset
	h.windowManager.SetStageOffset(0, 0)
	// In studio, use sanfo_basic.window style for nav (with close button)
	h.windowManager.SetEntryOpen(true)

	// Initialize Catalogue manager
	h.catalogueManager = NewCatalogueManager(h.windowManager, h.tm)
	// Forward purchase handler if set earlier
	if h.purchaseHandler != nil {
		h.catalogueManager.SetPurchaseHandler(h.purchaseHandler, h.GetBalance)
	}

	// Discover reference X for nav and mms to place jukebox between them
	var navX, navY, mmsX int
	for _, el := range h.windowDef.Elements {
		if el.ID == "cc.bottombtn.navi" {
			navX = el.LocH
			navY = el.LocV
		}
		if el.ID == "cc.bottombtn.mms" {
			mmsX = el.LocH
		}
	}
	midX := navX + (mmsX-navX)/2
	if midX == 0 {
		midX = 491 // fallback based on original layout
	}
	// Fine-tune alignment constants
	navShiftLeft := 24     // pixels
	jukeboxShiftDown := 0  // pixels
	jukeboxShiftLeft := 12 // pixels

	// Create interactive elements for the bottom buttons, with jukebox repositioned
	for _, el := range h.windowDef.Elements {
		if el.Media == "bitmap" && strings.ToLower(el.Type) == "image" {
			if strings.HasPrefix(el.ID, "cc.bottombtn.") {
				ie := h.elementRenderer.CreateInteractiveElement(el)
				if ie != nil {
					if el.ID == "cc.bottombtn.jukebox" {
						ie.Rect[0] = midX - jukeboxShiftLeft
						ie.Rect[1] = navY + jukeboxShiftDown
					}
					if el.ID == "cc.bottombtn.navi" {
						ie.Rect[0] = navX - navShiftLeft
					}
					h.buttons[el.ID] = ie
				}
			}
		}
	}

	return nil
}

// OpenMessenger toggles the Messenger console window
func (h *HUDManager) OpenMessenger() {
	if h.windowManager == nil {
		return
	}
	// If any console window is open, close it (toggle off)
	for _, name := range []string{"console_myinfo", "console_friends", "console_compose", "console_getmessage", "console_getrequest", "console_removefriend"} {
		if h.windowManager.GetWindow(name) != nil {
			h.windowManager.CloseWindow(name)
			h.messengerOpen = false
			return
		}
	}
	h.windowManager.OpenConsoleWindow("console_myinfo", [4]int{0, 0, 0, 0})
	h.messengerOpen = true
}

// GetChatFieldRect returns the chat field rectangle for chat manager setup
func (h *HUDManager) GetChatFieldRect() [4]int {
	return h.chatFieldRect
}

// GetSpeechSwitchRect returns the speech switch rectangle for chat manager setup
func (h *HUDManager) GetSpeechSwitchRect() [4]int {
	return h.speechSwitchRect
}

// GetFontManager returns the font manager for other systems to use
func (h *HUDManager) GetFontManager() *ui.FontManager {
	return h.fontManager
}

// GetWindowManager returns the window manager for overlay windows
func (h *HUDManager) GetWindowManager() *ui.WindowManager {
	return h.windowManager
}

// OpenChooseSongWindow opens the choose_song window used for stereo performances.
func (h *HUDManager) OpenChooseSongWindow() {
	if h == nil || h.windowManager == nil || h.tm == nil {
		return
	}
	// If already open, bring to front
	if w := h.windowManager.GetWindow("choose_song"); w != nil {
		h.windowManager.BringToFront(w)
		return
	}
	// Load window definition and assets
	windowPath := filepath.Join("Lingo", "cc_room", "67_choose_song.window.txt")
	def, err := window.ParseFile(windowPath)
	if err != nil {
		return
	}
	roomDir := filepath.Join("Lingo", "cc_room")
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, roomDir, interfaceDir)
	// Ensure button and scrollbar assets required by this window are present
	if extra := lingoassets.LoadAssetsByNames([]string{
		"button.b1.left.active", "button.b1.middle.active", "button.b1.right.active",
		"button.b1.left.pressed", "button.b1.middle.pressed", "button.b1.right.pressed",
		"button.scroll.up.passive", "button.scroll.up.active",
		"button.scroll.down.passive", "button.scroll.down.active",
		"scrollbar.vertical.active",
		"button.e1.left.active", "button.e1.middle.active", "button.e1.right.active",
		"button.e1.left.pressed", "button.e1.middle.pressed", "button.e1.right.pressed",
	}, interfaceDir); extra != nil {
		for k, v := range extra {
			assets[k] = v
		}
	}
	// Open with name choose_song and apply absolute rect from definition
	w := h.windowManager.OpenWindow("choose_song", def, assets, h.tm)
	if w != nil {
		w.Rect = def.Rect
		// Shift close button left by 4px
		w.CloseButtonOffsetX = w.CloseButtonOffsetX - 16
		// Ensure content labels and button text keys are correct
		for i := range w.Definition.Elements {
			el := &w.Definition.Elements[i]
			if el.ID == "songlist_play" {
				el.Key = "CHOOSE_SONG_PLAY"
			}
			if el.ID == "songlist_cancel" {
				el.Key = "CHOOSE_SONG_CANCEL"
			}
		}
		// Hide CD icon until a song is selected
		if w.TempVars == nil {
			w.TempVars = map[string]interface{}{}
		}
		w.TempVars["hide_cd_icon"] = true
		w.TempVars["choose_song_deadline"] = time.Now().Add(30 * time.Second)

		// Convert Play/Cancel content buttons into interactive buttons with proper bases
		if w.Interactive == nil {
			w.Interactive = make(map[string]*ui.InteractiveElement)
		}
		var playEl, cancelEl *window.Element
		for i := range w.Definition.Elements {
			if w.Definition.Elements[i].ID == "songlist_play" {
				playEl = &w.Definition.Elements[i]
			}
			if w.Definition.Elements[i].ID == "songlist_cancel" {
				cancelEl = &w.Definition.Elements[i]
			}
		}
		if playEl != nil {
			pe := *playEl
			pe.Member = "button.e1" // Use Go!-style
			ie := w.Renderer.CreateInteractiveElement(pe)
			// Shift Play interactive button right by 10px
			ie.Rect[0] += 14
			w.Interactive["songlist_play"] = ie
		}
		if cancelEl != nil {
			ce := *cancelEl
			ce.Member = "button.b1"
			ie := w.Renderer.CreateInteractiveElement(ce)
			// Shift Cancel interactive button right by 10px
			ie.Rect[0] += 10
			w.Interactive["songlist_cancel"] = ie
		}
		// Build list from backpack CDs (prodID 89)
		if w.Lists != nil {
			var region *window.Element
			for i := range w.Definition.Elements {
				if w.Definition.Elements[i].ID == "cc_songlist" {
					region = &w.Definition.Elements[i]
					break
				}
			}
			if region != nil {
				face := h.fontManager.GetFont("Verdana", "bold", 10)
				clr := color.RGBA{0x56, 0x42, 0x32, 0xFF}
				list := ui.NewScrollList(region.LocH, region.LocV, region.Width, region.Height, 13, face, clr)
				list.SimpleTextOnly = true
				// Gather songs from active scene backpack
				var records []map[string]string
				if sc := GetActiveScene(); sc != nil && sc.backpackManager != nil {
					for _, bi := range sc.backpackManager.items {
						if strings.TrimSpace(bi.ProdID) != "89" || bi.Attributes == nil {
							continue
						}
						song := ""
						owner := ""
						if v, ok := bi.Attributes["songName"]; ok {
							if s2, ok2 := v.(string); ok2 {
								song = strings.TrimSpace(s2)
							}
						}
						if v, ok := bi.Attributes["ownerName"]; ok {
							if s2, ok2 := v.(string); ok2 {
								owner = strings.TrimSpace(s2)
							}
						}
						if song == "" {
							continue
						}
						list.Items = append(list.Items, ui.RoomListItem{StudioID: bi.PosID, Name: song})
						records = append(records, map[string]string{"posID": bi.PosID, "songName": song, "ownerName": owner})
					}
				}
				w.Lists["cc_songlist"] = list
				if w.TempVars == nil {
					w.TempVars = map[string]interface{}{}
				}
				w.TempVars["choose_song_records"] = records
				w.Lists["cc_songlist"].UpdateContent()
			}
		}
	}
}

// OpenGeneralAlert opens sanfo_general_alert with custom title and message and an Ok button styled like Go!/Play.
func (h *HUDManager) OpenGeneralAlert(title, message string) {
	if h == nil || h.windowManager == nil || h.tm == nil {
		return
	}
	// Close existing to refresh content
	if w := h.windowManager.GetWindow("sanfo_general_alert"); w != nil {
		h.windowManager.CloseWindow("sanfo_general_alert")
	}
	defPath := filepath.Join("Lingo", "cc_interface[1]", "7_sanfo_general_alert.window.txt")
	def, err := window.ParseFile(defPath)
	if err != nil {
		return
	}
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, interfaceDir)
	// Ensure Go!-style button assets are available
	extra := lingoassets.LoadAssetsByNames([]string{
		"button.e1.left.active", "button.e1.middle.active", "button.e1.right.active",
		"button.e1.left.pressed", "button.e1.middle.pressed", "button.e1.right.pressed",
	}, interfaceDir)
	for k, v := range extra {
		assets[k] = v
	}

	w := h.windowManager.OpenWindow("sanfo_general_alert", def, assets, h.tm)
	if w == nil {
		return
	}
	w.Rect = def.Rect
	// Replace title and text
	for i := range w.Definition.Elements {
		el := &w.Definition.Elements[i]
		if el.ID == "general_alert_text" {
			el.Key = message
		}
	}
	// Add a centered OK interactive button using button.e1
	if w.Interactive == nil {
		w.Interactive = make(map[string]*ui.InteractiveElement)
	}
	// Create a pseudo element for the OK button placed over existing button area
	var baseBtn *window.Element
	for i := range w.Definition.Elements {
		if w.Definition.Elements[i].ID == "close" {
			baseBtn = &w.Definition.Elements[i]
			break
		}
	}
	if baseBtn != nil {
		be := *baseBtn
		be.Member = "button.e1"
		ie := w.Renderer.CreateInteractiveElement(be)
		// Move the button asset: 1px right, 2px up
		ie.Rect[0] += 1
		ie.Rect[1] -= 2
		// Register under the same ID as the content element so base is skipped during draw
		w.Interactive["close"] = ie
	}
}

// OpenPerformanceResultsDialog opens the longer alert window for performance results (votes and decibels).
func (h *HUDManager) OpenPerformanceResultsDialog(thumbsUp, thumbsDown, decibels int) {
	if h == nil || h.windowManager == nil || h.tm == nil {
		return
	}
	// Close existing to refresh content
	if w := h.windowManager.GetWindow("sanfo_general_alert_long"); w != nil {
		h.windowManager.CloseWindow("sanfo_general_alert_long")
	}
	defPath := filepath.Join("Lingo", "cc_interface[1]", "6_sanfo_general_alert_long.window.txt")
	def, err := window.ParseFile(defPath)
	if err != nil {
		return
	}
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, interfaceDir)
	// Ensure Go!-style button assets are available
	extra := lingoassets.LoadAssetsByNames([]string{
		"button.e1.left.active", "button.e1.middle.active", "button.e1.right.active",
		"button.e1.left.pressed", "button.e1.middle.pressed", "button.e1.right.pressed",
	}, interfaceDir)
	for k, v := range extra {
		assets[k] = v
	}

	w := h.windowManager.OpenWindow("sanfo_general_alert_long", def, assets, h.tm)
	if w == nil {
		return
	}
	w.Rect = def.Rect
	// Populate bold header and body text
	header := "Your Performance Results"
	body := fmt.Sprintf("Thumbs up votes: %d\nThumbs down votes: %d\n\nYou receive %d decibels.", thumbsUp, thumbsDown, decibels)
	for i := range w.Definition.Elements {
		el := &w.Definition.Elements[i]
		switch el.ID {
		case "general_alert_abstract":
			el.Key = header
		case "general_alert_text":
			el.Key = body
		}
	}
	// Add an OK interactive button over the 'close' content element using button.e1
	if w.Interactive == nil {
		w.Interactive = make(map[string]*ui.InteractiveElement)
	}
	var baseBtn *window.Element
	for i := range w.Definition.Elements {
		if w.Definition.Elements[i].ID == "close" {
			baseBtn = &w.Definition.Elements[i]
			break
		}
	}
	if baseBtn != nil {
		be := *baseBtn
		be.Member = "button.e1"
		ie := w.Renderer.CreateInteractiveElement(be)
		// Force button label to 'Ok' regardless of window key
		ie.TextKey = "Ok"
		// Align like the short alert: 1px right, 2px up visually (text already nudged globally)
		ie.Rect[0] += 1
		ie.Rect[1] -= 2
		w.Interactive["close"] = ie
	}
}

// OpenHelpWindow toggles the authentic Coke Studios Help window (20_cc_help.window.txt)
func (h *HUDManager) OpenHelpWindow() {
	if h == nil || h.windowManager == nil || h.tm == nil {
		return
	}
	if w := h.windowManager.GetWindow("cc_help"); w != nil {
		h.windowManager.CloseWindow("cc_help")
		return
	}
	defPath := filepath.Join("Lingo", "cc_interface[1]", "20_cc_help.window.txt")
	def, err := window.ParseFile(defPath)
	if err != nil {
		return
	}
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, interfaceDir)
	extra := lingoassets.LoadAssetsByNames([]string{
		"button.e1.left.active", "button.e1.middle.active", "button.e1.right.active",
		"button.e1.left.pressed", "button.e1.middle.pressed", "button.e1.right.pressed",
	}, interfaceDir)
	for k, v := range extra {
		assets[k] = v
	}

	w := h.windowManager.OpenWindow("cc_help", def, assets, h.tm)
	if w == nil {
		return
	}
	w.Rect = def.Rect

	// Wire OK interactive button
	if w.Interactive == nil {
		w.Interactive = make(map[string]*ui.InteractiveElement)
	}
	var okBtn *window.Element
	for i := range w.Definition.Elements {
		if w.Definition.Elements[i].ID == "help_ok" {
			okBtn = &w.Definition.Elements[i]
			break
		}
	}
	if okBtn != nil {
		be := *okBtn
		be.Member = "button.e1"
		ie := w.Renderer.CreateInteractiveElement(be)
		if ie != nil {
			ie.TextKey = "Ok"
			w.Interactive["help_ok"] = ie
		}
	}
}

// OpenJukeboxWindow toggles the authentic Infinite Jukebox window (158_cc.infinite_jukebox.catalog.window.txt)
func (h *HUDManager) OpenJukeboxWindow() {
	if h == nil || h.windowManager == nil || h.tm == nil {
		return
	}
	if w := h.windowManager.GetWindow("cc_jukebox.catalog"); w != nil {
		h.windowManager.CloseWindow("cc_jukebox.catalog")
		return
	}
	roomDir := filepath.Join("Lingo", "cc_room")
	defPath := filepath.Join(roomDir, "158_cc.infinite_jukebox.catalog.window.txt")
	def, err := window.ParseFile(defPath)
	if err != nil {
		return
	}
	assets := lingoassets.LoadAssetsForDefinition(def, roomDir)
	extra := lingoassets.LoadAssetsByNames([]string{
		"cc.jukebox.closebox.active", "cc.jukebox.closebox.pressed",
		"cc.jukebox.catalog.drawer", "cc.jukebox.bg",
	}, roomDir)
	for k, v := range extra {
		assets[k] = v
	}

	w := h.windowManager.OpenWindow("cc_jukebox.catalog", def, assets, h.tm)
	if w == nil {
		return
	}
	w.Rect = def.Rect
	if w.Interactive == nil {
		w.Interactive = make(map[string]*ui.InteractiveElement)
	}
	var closeBtn *window.Element
	for i := range w.Definition.Elements {
		if w.Definition.Elements[i].ID == "cc.jukebox.catalog.close" {
			closeBtn = &w.Definition.Elements[i]
			break
		}
	}
	if closeBtn != nil {
		ie := w.Renderer.CreateInteractiveElement(*closeBtn)
		if ie != nil {
			w.Interactive["cc.jukebox.catalog.close"] = ie
		}
	}
}

// UpdateMessengerMyInfo populates the console_myinfo window with avatar preview and texts
func (h *HUDManager) UpdateMessengerMyInfo(name string, mission string, messages int, requests int, avatarBuf *ebiten.Image) {
	if h.windowManager == nil {
		return
	}
	w := h.windowManager.GetWindow("console_myinfo")
	if w == nil || w.Definition == nil {
		return
	}
	// Update text fields by element IDs from 10_console_myinfo.window.txt
	for i := range w.Definition.Elements {
		el := &w.Definition.Elements[i]
		switch el.ID {
		case "console_myinfo_name":
			el.Key = name
		case "console_myinfo_mission_field":
			el.Key = mission
		case "console_myinfo_messages_link":
			el.Key = fmt.Sprintf("%d new message(s)", messages)
		case "console_myinfo_requests_link":
			el.Key = fmt.Sprintf("%d Friends Request(s)", requests)
		}
	}
	// Update avatar preview image into the image slot ID
	if avatarBuf != nil {
		// Find the preview element to size correctly
		width, height := 77, 86
		for _, el := range w.Definition.Elements {
			if el.ID == "console_myhead_image" {
				if el.Width > 0 {
					width = el.Width
				}
				if el.Height > 0 {
					height = el.Height
				}
				break
			}
		}
		// Create target and bottom-align the avatar so feet aren't cropped
		target := ebiten.NewImage(width, height)
		op := &ebiten.DrawImageOptions{}
		srcW := avatarBuf.Bounds().Dx()
		srcH := avatarBuf.Bounds().Dy()
		// Center horizontally without scaling
		dx := float64((width - srcW) / 2)
		if dx < 0 {
			dx = 0
		}
		// Bottom-align vertically; if avatar taller than box, shift up so bottom is visible
		dy := float64(height - srcH)
		op.GeoM.Translate(dx, dy)
		target.DrawImage(avatarBuf, op)
		if w.Assets == nil {
			w.Assets = make(map[string]*ebiten.Image)
		}
		w.Assets["console_myhead_image"] = target
	}
}

// GetAssets returns the HUD assets for other systems to use (e.g., chat)
func (h *HUDManager) GetAssets() map[string]*ebiten.Image {
	return h.assets
}

// GetHUDOffsets returns the current HUD positioning offsets for chat system integration
func (h *HUDManager) GetHUDOffsets() (float64, float64) {
	if h.windowDef != nil {
		return float64(h.windowDef.Rect[0] + h.offsetX), float64(h.windowDef.Rect[1] + h.offsetY)
	}
	return float64(h.offsetX), float64(h.offsetY)
}

// Update handles HUD input and state updates
func (h *HUDManager) Update() error {
	// Update overlay windows
	if h.windowManager != nil {
		if err := h.windowManager.Update(); err != nil {
			return err
		}
	}

	// Choose song interactions: list click populates bottom box and shows icon
	if h.windowManager != nil {
		if w := h.windowManager.GetWindow("choose_song"); w != nil {
			// 1) Countdown timer: show to the right of Cancel, auto-close at 0
			if w.TempVars != nil {
				if dl, ok := w.TempVars["choose_song_deadline"].(time.Time); ok {
					rem := time.Until(dl)
					if rem <= 0 {
						h.windowManager.CloseWindow("choose_song")
						return nil
					}
					secs := int(rem.Seconds())
					if secs < 0 {
						secs = 0
					}
					// Draw countdown label using same font/color as list values
					// Place to the right of Cancel button interactive rect
					if ie := w.Interactive["songlist_cancel"]; ie != nil {
						face := h.fontManager.GetFont("Verdana", "bold", 10)
						clr := color.RGBA{0x56, 0x42, 0x32, 0xFF}
						label := fmt.Sprintf("%d secs", secs)
						w.TempVars["choose_song_timer_label"] = label
						w.TempVars["choose_song_timer_pos"] = [2]int{ie.Rect[0] + ie.Rect[2] + 20, ie.Rect[1] + 10}
						w.TempVars["choose_song_timer_font"] = face
						w.TempVars["choose_song_timer_color"] = clr
					}
				}
			}
			if list := w.Lists["cc_songlist"]; list != nil {
				if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
					mouseX, mouseY := ebiten.CursorPosition()
					idx := list.HitTestRow(mouseX, mouseY, w.StageOffsetX+float64(w.Rect[0]), w.StageOffsetY+float64(w.Rect[1]))
					if idx > 0 && idx <= list.TotalItems() {
						// Show icon
						if w.TempVars == nil {
							w.TempVars = map[string]interface{}{}
						}
						w.TempVars["hide_cd_icon"] = false
						// Populate bottom text
						recs, _ := w.TempVars["choose_song_records"].([]map[string]string)
						if idx-1 < len(recs) {
							rec := recs[idx-1]
							text := "Name: " + rec["songName"]
							if rec["ownerName"] != "" {
								text += "\nAuthor: " + rec["ownerName"]
							}
							for i := range w.Definition.Elements {
								if w.Definition.Elements[i].ID == "song_choose_data" {
									w.Definition.Elements[i].Key = text
									break
								}
							}
							// Store selection for Play button
							w.TempVars["choose_song_selected_posID"] = rec["posID"]
						}
					}
				}
			}
			// Handle Play/Cancel buttons
			if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
				if ie := w.Interactive["songlist_play"]; ie != nil {
					x := int(w.StageOffsetX) + w.Rect[0] + ie.Rect[0]
					y := int(w.StageOffsetY) + w.Rect[1] + ie.Rect[1]
					if mx, my := ebiten.CursorPosition(); mx >= x && mx < x+ie.Rect[2] && my >= y && my < y+ie.Rect[3] {
						// Send CD_P
						posID, _ := w.TempVars["choose_song_selected_posID"].(string)
						if posID != "" {
							if sc := GetActiveScene(); sc != nil && sc.sendCDPlayIfAvailable != nil {
								_ = sc.sendCDPlayIfAvailable(sc.studio.StudioID, posID)
								// Auto-close window on request sent
								h.windowManager.CloseWindow("choose_song")
							}
						}
					}
				}
				if ie := w.Interactive["songlist_cancel"]; ie != nil {
					x := int(w.StageOffsetX) + w.Rect[0] + ie.Rect[0]
					y := int(w.StageOffsetY) + w.Rect[1] + ie.Rect[1]
					if mx, my := ebiten.CursorPosition(); mx >= x && mx < x+ie.Rect[2] && my >= y && my < y+ie.Rect[3] {
						// Close choose_song window
						h.windowManager.CloseWindow("choose_song")
					}
				}
			}
		}
		// General alert OK click-to-close
		if w := h.windowManager.GetWindow("sanfo_general_alert"); w != nil {
			if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
				if ie := w.Interactive["close"]; ie != nil {
					x := int(w.StageOffsetX) + w.Rect[0] + ie.Rect[0]
					y := int(w.StageOffsetY) + w.Rect[1] + ie.Rect[1]
					if mx, my := ebiten.CursorPosition(); mx >= x && mx < x+ie.Rect[2] && my >= y && my < y+ie.Rect[3] {
						h.windowManager.CloseWindow("sanfo_general_alert")
					}
				}
			}
		}
		// Long general alert OK click-to-close
		if w := h.windowManager.GetWindow("sanfo_general_alert_long"); w != nil {
			if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
				if ie := w.Interactive["close"]; ie != nil {
					x := int(w.StageOffsetX) + w.Rect[0] + ie.Rect[0]
					y := int(w.StageOffsetY) + w.Rect[1] + ie.Rect[1]
					if mx, my := ebiten.CursorPosition(); mx >= x && mx < x+ie.Rect[2] && my >= y && my < y+ie.Rect[3] {
						h.windowManager.CloseWindow("sanfo_general_alert_long")
					}
				}
			}
		}
		// Help window OK click-to-close
		if w := h.windowManager.GetWindow("cc_help"); w != nil {
			if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
				if ie := w.Interactive["help_ok"]; ie != nil {
					x := int(w.StageOffsetX) + w.Rect[0] + ie.Rect[0]
					y := int(w.StageOffsetY) + w.Rect[1] + ie.Rect[1]
					if mx, my := ebiten.CursorPosition(); mx >= x && mx < x+ie.Rect[2] && my >= y && my < y+ie.Rect[3] {
						h.windowManager.CloseWindow("cc_help")
					}
				}
			}
		}
		// Jukebox catalog window close click-to-close
		if w := h.windowManager.GetWindow("cc_jukebox.catalog"); w != nil {
			if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
				if ie := w.Interactive["cc.jukebox.catalog.close"]; ie != nil {
					x := int(w.StageOffsetX) + w.Rect[0] + ie.Rect[0]
					y := int(w.StageOffsetY) + w.Rect[1] + ie.Rect[1]
					if mx, my := ebiten.CursorPosition(); mx >= x && mx < x+ie.Rect[2] && my >= y && my < y+ie.Rect[3] {
						h.windowManager.CloseWindow("cc_jukebox.catalog")
					}
				}
			}
		}
	}

	return nil
}

// HandleInput processes mouse input for HUD buttons and interactions
func (h *HUDManager) HandleInput() (string, error) {
	if h.windowDef == nil || h.elementRenderer == nil {
		return "", nil
	}

	// Suppress HUD button clicks when mouse is over any open overlay window (e.g., catalogue)
	if h.windowManager != nil && h.windowManager.IsMouseOverAnyWindow() {
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) ||
			inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) ||
			ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			return "", nil
		}
	}

	mouseX, mouseY := ebiten.CursorPosition()
	mousePressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	mouseClicked := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)

	// Window rect gives absolute position on stage, apply HUD offsets
	offsetX := float64(h.windowDef.Rect[0] + h.offsetX)
	offsetY := float64(h.windowDef.Rect[1] + h.offsetY)

	// Default: clear rollover text
	h.rolloverText = ""

	var clickedButton string

	for id, btn := range h.buttons {
		btn.HandleMouseInput(mouseX, mouseY, mousePressed, offsetX, offsetY)
		if btn.State == ui.ButtonRollover {
			h.rolloverText = h.getRolloverLabelFor(id)
		}
		if mouseClicked && btn.State == ui.ButtonPressed {
			clickedButton = id
		}
	}

	return clickedButton, nil
}

// Draw renders the studio bottom bar UI
func (h *HUDManager) Draw(screen *ebiten.Image, chatManager *ChatManager) {
	if h.windowDef == nil || h.elementRenderer == nil {
		return
	}
	// Use window rect origin as offset (absolute positioning) and apply HUD offsets
	offsetX := float64(h.windowDef.Rect[0] + h.offsetX)
	offsetY := float64(h.windowDef.Rect[1] + h.offsetY)

	// Draw non-interactive elements first
	for _, element := range h.windowDef.Elements {
		// Skip interactive button images; they'll be drawn via interactive renderer
		if element.Media == "bitmap" && strings.ToLower(element.Type) == "image" {
			if _, isBtn := h.buttons[element.ID]; isBtn {
				continue
			}
			// Also skip speechswitch, we'll draw it using current state
			if element.ID == "cc.speechswitch" {
				continue
			}
		}
		h.elementRenderer.RenderElement(screen, element, offsetX, offsetY)
	}

	// Draw speech switch using current state image
	if chatManager != nil {
		chatManager.DrawSpeechSwitch(screen, h.assets, offsetX, offsetY)
	}

	// Draw interactive buttons in current state
	for id, btn := range h.buttons {
		// For sound button, override assets based on mute state
		if id == "cc.bottombtn.sound" {
			if h.soundMuted {
				btn.BaseName = "cc.bottombtn.soundoff"
			} else {
				btn.BaseName = "cc.bottombtn.sound"
			}
		}
		h.elementRenderer.RenderInteractiveElement(screen, btn, offsetX, offsetY)
	}

	// Draw chat input text in chat field when focused
	if chatManager != nil {
		chatManager.DrawChatInput(screen, offsetX, offsetY)
	}

	// Draw rollover tooltip text (overlays the empty field from the window definition)
	if h.rolloverText != "" && h.fontManager != nil {
		font := h.fontManager.GetFont("Verdana", "bold", 10)
		if font != nil {
			// Match text color from window def (#2E1B0B)
			col := color.RGBA{0x2E, 0x1B, 0x0B, 0xFF}
			// Center horizontally within tooltip rect
			b := text.BoundString(font, h.rolloverText)
			w := b.Dx()
			height := b.Dy()
			textX := int(offsetX) + h.tooltipRect[0] + (h.tooltipRect[2]-w)/2
			// Vertically center baseline in the rect (approx), with minor adjustment
			textY := int(offsetY) + h.tooltipRect[1] + (h.tooltipRect[3]+height)/2 - 2 + 4
			text.Draw(screen, h.rolloverText, font, textX, textY, col)
		}
	}
}

// getRolloverLabelFor returns localized label for button rollover
func (h *HUDManager) getRolloverLabelFor(id string) string {
	switch id {
	case "cc.bottombtn.navi":
		if v := h.tm.First("NAVIGATOR"); v != "" {
			return v
		}
		return "Navigator"
	case "cc.bottombtn.mms":
		if v := h.tm.First("MESSENGER"); v != "" {
			return v
		}
		return "Messenger"
	case "cc.bottombtn.bag":
		if v := h.tm.First("BACKPACK"); v != "" {
			return v
		}
		return "Backpack"
	case "cc.bottombtn.catalogue":
		if v := h.tm.First("CATALOGUE"); v != "" {
			return v
		}
		return "Catalogue"
	case "cc.bottombtn.sound":
		if h.soundMuted {
			if v := h.tm.First("SOUND_OFF"); v != "" {
				return v
			}
			return "Sound Off"
		}
		if v := h.tm.First("SOUND_ON"); v != "" {
			return v
		}
		return "Sound On"
	case "cc.bottombtn.help":
		if v := h.tm.First("HELP"); v != "" {
			return v
		}
		return "Help"
	case "cc.bottombtn.jukebox":
		if v := h.tm.First("JUKEBOX"); v != "" {
			return v
		}
		return "Jukebox"
	case "cc.bottombtn.dbs":
		if v := h.tm.First("DECIBELS"); v != "" {
			return v
		}
		return "Decibels"
	default:
		return ""
	}
}

// OpenNavigatorWindow opens the navigator window (delegated to NavigatorManager)
func (h *HUDManager) OpenNavigatorWindow() {
	if h == nil || h.windowManager == nil || h.tm == nil {
		return
	}
	navigator.OpenOverlay(h.windowManager, h.tm, navigator.Options{
		StudioMode:    true,
		EnterStudio:   h.enterStudio,
		GetScreenName: h.getScreenName,
		GetGatewayURL: h.getGatewayURL,
		GetSecret:     h.getSecret,
		StudioList:    h.studioList,
	})
}

// SetStudioList allows passing the studio list so the navigator in studio can populate lists
func (h *HUDManager) SetStudioList(list *studios.StudioList) {
	h.studioList = list
}

// SetNavigatorProviders allows the scene to provide screen name and gateway URL providers for private nav actions
func (h *HUDManager) SetNavigatorProviders(screenNameProvider func() string, gatewayURLProvider func() string, secretProvider func() string) {
	h.getScreenName = screenNameProvider
	h.getGatewayURL = gatewayURLProvider
	h.getSecret = secretProvider
}

// SetEnterStudio sets the callback for entering a studio from the in-studio navigator
func (h *HUDManager) SetEnterStudio(fn func(studioID string)) {
	h.enterStudio = fn
}

// Balance helpers
func (h *HUDManager) GetBalance() int {
	return h.balance
}

func (h *HUDManager) AdjustBalance(delta int) int {
	h.balance += delta
	if h.balance < 0 {
		h.balance = 0
	}
	return h.balance
}


// SetBalance sets the absolute decibels balance.
func (h *HUDManager) SetBalance(b int) {
	if b < 0 {
		b = 0
	}
	h.balance = b
}

// RefreshDecibelsWindow updates the open decibels window with the current balance.
func (h *HUDManager) RefreshDecibelsWindow() {
	if h == nil || h.windowManager == nil || h.tm == nil {
		return
	}
	w := h.windowManager.GetWindow("cc_balance")
	if w == nil || w.Definition == nil {
		return
	}
	// Recompute display text and update the window's content element
	display := h.tm.First("BALANCE_DISPLAY")
	if display == "" {
		display = "You have {balance} decibels."
	}
	display = strings.ReplaceAll(display, "{balance}", strconv.Itoa(h.GetBalance()))
	for i := range w.Definition.Elements {
		if w.Definition.Elements[i].Member == "balance_decibels" {
			w.Definition.Elements[i].Key = display
			break
		}
	}
}

// SetPurchaseHandler sets callback for purchases and wires it to the catalogue manager
func (h *HUDManager) SetPurchaseHandler(handler func(imageBase string, price int) bool) {
	h.purchaseHandler = handler
	if h.catalogueManager != nil {
		h.catalogueManager.SetPurchaseHandler(handler, h.GetBalance)
	}
}
