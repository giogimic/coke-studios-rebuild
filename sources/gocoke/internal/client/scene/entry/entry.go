// Entry scene renders the main Lingo cc_entry.window background using ripped entry assets
// Provides the main desktop/background for the CokeStudios application

package entry

import (
	"fmt"
	"image/color"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"gocoke/internal/client/assets/catalog"
	"gocoke/internal/client/assets/features"
	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/lingo/window"
	"gocoke/internal/client/assets/studios"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/avatarengine"
	"gocoke/internal/client/env"
	"gocoke/internal/client/net/gateway"
	"gocoke/internal/client/ui"
	"gocoke/internal/client/ui/navigator"

	"context"
	"gocoke/internal/client/net/realtime"
	"gocoke/internal/client/session"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type Scene struct {
	tm         *textmgr.Manager
	featureSet *features.FeatureSet
	studioList *studios.StudioList
	catalog    *catalog.Catalog

	// Data directory for asset loads used by entry (e.g., avatar engine)
	dataDir string

	// Window definition and assets
	windowDef *window.Definition
	assets    map[string]*ebiten.Image

	// UI system
	fontManager     *ui.FontManager
	windowManager   *ui.WindowManager
	elementRenderer *ui.ElementRenderer
	helpButton      *ui.InteractiveElement
	mmsButton       *ui.InteractiveElement

	// Debug state tracking (to avoid spam logging)
	lastLayoutSize    [2]int
	lastButtonState   [2]bool // [helpButton != nil, mmsButton != nil]
	lastAssetCount    int
	lastRolloverState [2]bool // [help.rollover exists, mms.rollover exists]

	// State
	showNavigator bool
	enterStudioID string // Set when we want to enter a studio
	last          time.Time

	// Screen dimensions (updated in Draw, used in Update for consistent coordinate calculation)
	currentScreenW int
	currentScreenH int

	// Safety tips system (matches Lingo 66_safety tips.ls)
	safetyTips      []string
	currentTipIndex int
	tipTimer        time.Time
	showingTip      bool
	tipDisplayTime  time.Duration // 5 seconds like Lingo
	tipClearTime    time.Duration // 0.1 seconds between clear and next tip

	// Currently selected studio for Go button functionality
	selectedStudioID string

	// UI positioning (from original window definition)
	windowRect [4]int // [0, 0, 760, 520] from cc_entry.window.txt
}

func New(dataDir string, featureSet *features.FeatureSet, studioList *studios.StudioList) *Scene {
	tm := textmgr.New()
	path := filepath.Join(dataDir, "Text_English.txt")
	if err := tm.LoadFile(path); err != nil {
		log.Printf("Entry: failed to load text file %s: %v", path, err)
	}

	scene := &Scene{
		tm:             tm,
		featureSet:     featureSet,
		studioList:     studioList,
		dataDir:        dataDir,
		showNavigator:  false,
		windowRect:     [4]int{0, 0, 760, 520}, // Full screen background
		assets:         make(map[string]*ebiten.Image),
		tipDisplayTime: 5 * time.Second,        // 5 seconds like Lingo
		tipClearTime:   100 * time.Millisecond, // 0.1 seconds between tips
	}

	// Load safety tips from text file (matches Lingo TextMgr.getsafetytips())
	scene.loadSafetyTips()

	// Initialize tip timer
	scene.tipTimer = time.Now()

	// Load entry window definition (data-driven)
	windowPath := filepath.Join("Lingo", "entry", "6_cc_entry.window.txt")
	windowDef, err := window.ParseFile(windowPath)
	if err != nil {
		log.Printf("Entry: failed to load window definition: %v", err)
	} else {
		scene.windowDef = windowDef
		log.Printf("Entry: loaded window definition with %d elements", len(windowDef.Elements))
	}

	// Load entry assets from window definition (data-driven)
	entryDir := filepath.Join("Lingo", "entry")
	scene.assets = lingoassets.LoadAssetsForDefinition(scene.windowDef, entryDir)

	// Load entry button assets (help and MMS buttons with rollover states)
	scene.loadEntryButtonAssets()

	// Initialize UI system
	scene.fontManager = ui.NewFontManager()
	scene.fontManager.PreloadCommonFonts()
	scene.windowManager = ui.NewWindowManager(scene.fontManager)
	scene.elementRenderer = ui.NewElementRenderer(scene.assets, scene.tm)
	scene.elementRenderer.SetFontManager(scene.fontManager)
	scene.initializeInteractiveElements()

	// Initialize "Call for Assistance" text
	scene.updateEntryCfhText(scene.tm.First("CALL_FOR_ASSISTANCE"))

	// Open navigator window automatically on load
	scene.openNavigatorWindow()

	return scene
}

func (s *Scene) loadAssets() {
	// Load entry assets from Lingo/entry using member map
	lingoDir := filepath.Join("Lingo", "entry")
	mm, err := lingoassets.LoadMemberMap(lingoDir)
	if err != nil {
		log.Printf("Entry: member map: %v", err)
		return
	}

	// Entry assets needed for the main background
	assetNames := []string{
		"cc.entry.bg",          // Main background image
		"cc.entry.help.active", // Help button
		"cc.entry.help.rollover",
		"cc.entry.help.pressed",
		"cc.entry.mms.active", // MMS button
		"cc.entry.mms.rollover",
		"cc.entry.mms.pressed",
		"cc.entry.mms.flash",
		"cc_entry_shadow_pixel", // Shadow pixel
	}

	for _, name := range assetNames {
		if path, ok := mm.PathFor(name); ok {
			if img, err := lingoassets.LoadImage(path); err == nil {
				s.assets[name] = img
				log.Printf("Entry: loaded asset %s (%dx%d)", name, img.Bounds().Dx(), img.Bounds().Dy())
			} else {
				log.Printf("Entry: failed to load asset %s: %v", name, err)
			}
		} else {
			log.Printf("Entry: asset not found: %s", name)
		}
	}
	log.Printf("Entry: loaded %d assets total", len(s.assets))
}

func (s *Scene) initializeInteractiveElements() {
	if s.windowDef == nil {
		return
	}

	// Reset interactive elements
	s.helpButton = nil
	s.mmsButton = nil

	// Find and create interactive elements from window definition
	// Use the same logic as Window.initializeInteractiveElements() for consistency
	for _, element := range s.windowDef.Elements {
		if element.Media == "bitmap" && element.Type == "image" {
			switch element.ID {
			case "cc.entry.help":
				// Check if rollover assets exist (same logic as window interactive elements)
				baseName := element.Member
				if strings.HasSuffix(baseName, ".active") {
					baseName = strings.TrimSuffix(baseName, ".active")
				}
				if s.assets[baseName+".rollover"] != nil || s.assets[baseName+".pressed"] != nil {
					s.helpButton = s.elementRenderer.CreateInteractiveElement(element)
					log.Printf("Entry: created help button at (%d, %d) with assets %s", element.LocH, element.LocV, baseName)
				} else {
					log.Printf("Entry: help button assets missing for %s", baseName)
				}
			case "cc.entry.mms":
				// Check if rollover assets exist (same logic as window interactive elements)
				baseName := element.Member
				if strings.HasSuffix(baseName, ".active") {
					baseName = strings.TrimSuffix(baseName, ".active")
				}
				if s.assets[baseName+".rollover"] != nil || s.assets[baseName+".pressed"] != nil {
					s.mmsButton = s.elementRenderer.CreateInteractiveElement(element)
					log.Printf("Entry: created MMS button at (%d, %d) with assets %s", element.LocH, element.LocV, baseName)
				} else {
					log.Printf("Entry: MMS button assets missing for %s", baseName)
				}
			}
		}
	}
}

// loadEntryButtonAssets loads rollover assets for help and MMS buttons
func (s *Scene) loadEntryButtonAssets() {
	// Entry button assets are in Lingo/entry directory
	entryDir := filepath.Join("Lingo", "entry")

	// Assets needed for help and MMS buttons with rollover states
	buttonAssets := []string{
		"cc.entry.help.active",
		"cc.entry.help.rollover",
		"cc.entry.help.pressed",
		"cc.entry.mms.active",
		"cc.entry.mms.rollover",
		"cc.entry.mms.pressed",
		"cc.entry.mms.flash",
	}

	// Load assets using the proper asset loading system
	loadedAssets := lingoassets.LoadAssetsByNames(buttonAssets, entryDir)

	// Merge into main assets collection
	for name, img := range loadedAssets {
		s.assets[name] = img
		log.Printf("Entry: loaded button asset %s", name)
	}
}

func (s *Scene) Update() error {
	now := time.Now()
	s.last = now

	// Update window manager (handles all window input and interactions)
	if err := s.windowManager.Update(); err != nil {
		return err
	}

	// Handle mouse input for entry-level interactive elements
	mouseX, mouseY := ebiten.CursorPosition()
	mousePressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	mouseClicked := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)

	// Calculate offset for centered display (must match Draw method calculation exactly)
	// Use the screen dimensions stored from Draw method to ensure perfect coordinate alignment
	offsetX := float64(s.currentScreenW-760) / 2
	offsetY := float64(s.currentScreenH-520) / 2

	// Ensure interactive elements are properly initialized before handling input
	s.ensureInteractiveElementsReady()

	// Update button states
	if s.helpButton != nil {
		s.helpButton.HandleMouseInput(mouseX, mouseY, mousePressed, offsetX, offsetY)
		if mouseClicked && s.helpButton.State == ui.ButtonPressed {
			log.Printf("Entry: Help button clicked!")
			// TODO: Open help window
		}
	}

	if s.mmsButton != nil {
		oldState := s.mmsButton.State
		s.mmsButton.HandleMouseInput(mouseX, mouseY, mousePressed, offsetX, offsetY)

		// Handle text change on rollover (matches Lingo messenger.ls behavior)
		// Text should change to "MESSENGER" when mouse enters and stay until mouse leaves
		if s.mmsButton.State == ui.ButtonRollover && oldState != ui.ButtonRollover {
			// Mouse entered - change text to "MESSENGER"
			s.updateEntryCfhText(s.tm.First("MESSENGER"))
		} else if (s.mmsButton.State == ui.ButtonActive && oldState == ui.ButtonRollover) ||
			(s.mmsButton.State == ui.ButtonActive && oldState == ui.ButtonPressed) {
			// Mouse left (rollover->active or pressed->active) - change text back to "CALL_FOR_ASSISTANCE"
			s.updateEntryCfhText(s.tm.First("CALL_FOR_ASSISTANCE"))
		}

		if mouseClicked && s.mmsButton.State == ui.ButtonPressed {
			log.Printf("Entry: MMS button clicked!")
			// Open/toggle Messenger window (console_myinfo)
			// Load window def and assets from Lingo/cc_messenger[1]
			messengerDir := filepath.Join("Lingo", "cc_messenger[1]")
			defPath := filepath.Join(messengerDir, "10_console_myinfo.window.txt")
			def, err := window.ParseFile(defPath)
			if err != nil {
				log.Printf("Entry: failed to load messenger window def: %v", err)
			} else {
				assets := lingoassets.LoadAssetsForDefinition(def, messengerDir)
				// Merge frame-independent assets from Members.csv like button.d6/d7 & shell pieces
				for name, img := range lingoassets.LoadAssetsByNames([]string{
					"mmsSHELL1", "mmsSHELL2", "mmsSHELL3_b1dn", "mmsSHELL3_b2dn",
					"MMS_INT_WIN_TOP_LEFT", "MMS_INT_WIN_TOP_MIDDLE", "MMS_INT_WIN_TOP_RIGHT",
					"MMS_INT_WIN_MIDDLE_LEFT", "MMS_INT_WIN_MIDDLE", "MMS_INT_WIN_MIDDLE_RIGHT",
					"MMS_INT_WIN_BOTTOM_LEFT", "MMS_INT_WIN_BOTTOM_MIDDLE", "MMS_INT_WIN_BOTTOM_RIGHT",
					"MMS_INT_CLOSEBUTTON",
					"button.d6.left.active", "button.d6.middle.active", "button.d6.right.active",
					"button.d6.left.pressed", "button.d6.middle.pressed", "button.d6.right.pressed",
					"button.d7.left.active", "button.d7.middle.active", "button.d7.right.active",
					"button.d7.left.pressed", "button.d7.middle.pressed", "button.d7.right.pressed",
				}, messengerDir) {
					assets[name] = img
				}
				// Open or toggle existing
				if w := s.windowManager.GetWindow("console_myinfo"); w != nil {
					s.windowManager.CloseWindow("console_myinfo")
				} else {
					w := s.windowManager.OpenWindow("console_myinfo", def, assets, s.tm)
					w.Rect = def.Rect
					// Frameless drag: use full content area as drag region (or refine later by id="drag")
					if w.TempVars == nil {
						w.TempVars = map[string]interface{}{}
					}
					w.TempVars["customDragRects"] = [][4]int{{0, 0, def.Rect[2] - def.Rect[0], def.Rect[3] - def.Rect[1]}}
					// Populate basic fields in entry
					name, _ := session.GetCredentials()
					if name == "" {
						name = "Guest"
					}
					_, mission, _ := session.GetUserProfile()
					if mission == "" {
						mission = "Welcome to Coke Studios!"
					}
					// Update text fields by element IDs
					for i := range w.Definition.Elements {
						el := &w.Definition.Elements[i]
						switch el.ID {
						case "console_myinfo_name":
							el.Key = name
						case "console_myinfo_mission_field":
							el.Key = mission
						case "console_myinfo_messages_link":
							el.Key = "1 new message(s)"
						case "console_myinfo_requests_link":
							el.Key = "0 Friends Request(s)"
						}
					}
					// Compose and attach avatar preview for entry
					avatarString := "hr=002/238,238,238&hd=001/255,224,196&ey=003/245,226,203&fc=002/255,224,196&bd=001/255,224,196&lh=001/255,224,196&rh=001/255,224,196&ch=002/204,153,102&ls=002/204,153,102&rs=002/204,153,102&lg=008/115,142,191&sh=009/238,238,238"
					if eng, err := avatarengine.NewAvatarEngine(avatarString, 100, true, s.dataDir); err == nil && eng != nil {
						_ = eng.Draw()
						if buf := eng.GetBuffer(); buf != nil {
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
							target := ebiten.NewImage(width, height)
							op := &ebiten.DrawImageOptions{}
							dx := float64((width - buf.Bounds().Dx()) / 3)
							if dx < 0 {
								dx = 0
							}
							op.GeoM.Translate(dx, 0)
							target.DrawImage(buf, op)
							if w.Assets == nil {
								w.Assets = make(map[string]*ebiten.Image)
							}
							w.Assets["console_myhead_image"] = target
						}
					}
					// Optionally set avatar slot if we later build an avatar buffer in entry
					// (left as TODO when session/attributes are available)
				}
			}
		}
	}

	// Navigator now opens automatically on load; no key press required

	// Update safety tips rotation (matches Lingo 66_safety tips.ls exitFrame)
	s.updateSafetyTips()

	// Ensure all navigator windows have callbacks set up
	s.ensureNavigatorCallbacks()

	return nil
}

func (s *Scene) Draw(screen *ebiten.Image) {
	// Clear background to black (original stage background)
	screen.Fill(color.RGBA{0x00, 0x00, 0x00, 0xFF})

	// Ensure button assets are loaded (safeguard for screen resize)
	s.ensureButtonAssetsLoaded()

	// Calculate scaling to fit the original 760x520 background to current screen
	screenW, screenH := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Store screen dimensions for use in Update method (ensures consistent coordinate calculation)
	s.currentScreenW = screenW
	s.currentScreenH = screenH

	// For now, center the background without scaling to maintain pixel-perfect rendering
	// TODO: Add proper scaling options later
	offsetX := float64(screenW-760) / 2
	offsetY := float64(screenH-520) / 2

	// Draw entry background elements using exact coordinates from window definition
	s.drawElementsFromDefinition(screen, offsetX, offsetY)

	// Ensure overlay windows align to the same stage offset
	s.windowManager.SetStageOffset(offsetX, offsetY)

	// Draw all overlay windows (navigator, help, etc.) on top of entry background
	s.windowManager.Draw(screen)
}

func (s *Scene) drawElementsFromDefinition(screen *ebiten.Image, offsetX, offsetY float64) {
	if s.windowDef == nil {
		return
	}

	// Draw each element using the proper UI element renderer
	for _, element := range s.windowDef.Elements {
		// Skip interactive elements - they're handled separately
		if element.ID == "cc.entry.help" || element.ID == "cc.entry.mms" {
			continue
		}

		// Render using the element renderer for proper text and bitmap handling
		s.elementRenderer.RenderElement(screen, element, offsetX, offsetY)
	}

	// Draw interactive elements in their current states
	if s.helpButton != nil {
		s.elementRenderer.RenderInteractiveElement(screen, s.helpButton, offsetX, offsetY)
	}
	if s.mmsButton != nil {
		s.elementRenderer.RenderInteractiveElement(screen, s.mmsButton, offsetX, offsetY)
	}
}

// ShouldShowNavigator returns true when navigator should be opened
func (s *Scene) ShouldShowNavigator() bool {
	return s.showNavigator
}

// ShouldEnterStudio returns the studio ID if we should enter a studio, empty string otherwise
func (s *Scene) ShouldEnterStudio() string {
	return s.enterStudioID
}

// GetStudioByID returns the studio with the given ID
func (s *Scene) GetStudioByID(studioID string) *studios.Studio {
	for _, studio := range s.studioList.Studios {
		if studio.StudioID == studioID {
			return &studio
		}
	}
	return nil
}

// GetFeatureSet returns the loaded feature set
func (s *Scene) GetFeatureSet() *features.FeatureSet {
	return s.featureSet
}

// GetStudioList returns the loaded studio list
func (s *Scene) GetStudioList() *studios.StudioList {
	return s.studioList
}

// SetShowNavigator sets whether the navigator should be opened (used for autoload on return)
func (s *Scene) SetShowNavigator(show bool) {
	s.showNavigator = show
}

// openNavigatorWindow opens the navigator window as an overlay, matching Lingo's ElementManager.newwindow() behavior
func (s *Scene) openNavigatorWindow() {
	// Load navigator window definition and assets (data-driven)
	navPath := filepath.Join("Lingo", "cc_navigator[1]", "59_nav_public_start.window.txt")
	navDef, err := window.ParseFile(navPath)
	if err != nil {
		log.Printf("Entry: failed to load navigator window definition: %v", err)
		return
	}
	// Load all bitmap assets referenced by the navigator window definition from cc_navigator[1] and cc_interface[1]
	navigatorDir := filepath.Join("Lingo", "cc_navigator[1]")
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	navAssets := lingoassets.LoadAssetsForDefinition(navDef, navigatorDir, interfaceDir)

	// Open navigator window using window manager (matches Lingo's ElementManager behavior)
	// Use exact rect from window definition: [434, 66, 721, 427]
	navWindow := s.windowManager.OpenWindow("nav_public_start", navDef, navAssets, s.tm)

	// CRITICAL: Use the exact rect from the window definition, not calculated values
	navWindow.Rect = navDef.Rect // [434, 66, 721, 427] - this is absolute positioning!

	// Wire list click callback for this navigator window
	s.setupNavigatorCallbacks(navWindow)

	// Populate the public rooms list in the window from StudioList (SYSTEM-owned studios)
	if navWindow.Lists != nil {
		if list := navWindow.Lists["nav_public_rooms_list"]; list != nil {
			items := make([]ui.RoomListItem, 0, len(s.studioList.Studios))
			for _, st := range s.studioList.Studios {
				if st.Owner == "$SYSTEM$" { // public
					items = append(items, ui.RoomListItem{StudioID: st.StudioID, Name: st.Name, UserCount: st.UserCount, Capacity: st.Capacity})
				}
			}
			list.SetItems(items)
			list.UpdateContent()
		}
	}

	log.Printf("Entry: opened navigator window at rect [%d, %d, %d, %d]",
		navWindow.Rect[0], navWindow.Rect[1], navWindow.Rect[2], navWindow.Rect[3])
}

// DisplayPublicStudios emulates Lingo ElementMgr.displayPublicStudios: update the open nav list from data
func (s *Scene) DisplayPublicStudios() {
	if s.windowManager == nil {
		return
	}
	w := s.windowManager.GetWindow("nav_public_start")
	if w == nil || w.Lists == nil {
		return
	}
	list := w.Lists["nav_public_rooms_list"]
	if list == nil {
		return
	}
	items := make([]ui.RoomListItem, 0, len(s.studioList.Studios))
	for _, st := range s.studioList.Studios {
		if st.Owner == "$SYSTEM$" {
			items = append(items, ui.RoomListItem{StudioID: st.StudioID, Name: st.Name, UserCount: st.UserCount, Capacity: st.Capacity})
		}
	}
	list.SetItems(items)
	list.UpdateContent()
}

// loadSafetyTips loads all SAFETY_TIP entries from text file (matches Lingo TextMgr.getsafetytips())
func (s *Scene) loadSafetyTips() {
	s.safetyTips = s.tm.All("SAFETY_TIP")
	s.currentTipIndex = 0
	log.Printf("Entry: loaded %d safety tips", len(s.safetyTips))
}

// updateSafetyTips handles the 5-second tip rotation (matches Lingo 66_safety tips.ls)
func (s *Scene) updateSafetyTips() {
	if len(s.safetyTips) == 0 {
		return
	}

	elapsed := time.Since(s.tipTimer)

	// Clear tip after 5 seconds (matches Lingo: if (the milliSeconds - pLastUpdate) > 5000)
	if elapsed > s.tipDisplayTime && s.showingTip {
		s.showingTip = false
		s.updateEntryInfoText("") // Clear the text
	}

	// Show next tip after 5.1 seconds (matches Lingo: if (the milliSeconds - pLastUpdate) > 5100)
	if elapsed > s.tipDisplayTime+s.tipClearTime {
		s.showNextTip()
	}
}

// showNextTip displays the next safety tip (matches Lingo shownewtip())
func (s *Scene) showNextTip() {
	if len(s.safetyTips) == 0 {
		return
	}

	// Reset timer (matches Lingo: pLastUpdate = the milliSeconds)
	s.tipTimer = time.Now()

	// Get current tip (matches Lingo: mytip = pSafetyTips[pIndex])
	tip := s.safetyTips[s.currentTipIndex]

	// Display tip (matches Lingo: sprite(me.spriteNum).member.text = mytip)
	s.updateEntryInfoText(tip)
	s.showingTip = true

	// Advance to next tip (matches Lingo index cycling logic)
	s.currentTipIndex++
	if s.currentTipIndex >= len(s.safetyTips) {
		s.currentTipIndex = 0
	}
}

// ensureButtonAssetsLoaded checks if button assets are loaded and reloads them if missing
// This is a safeguard for screen resize or other events that might cause asset loss
func (s *Scene) ensureButtonAssetsLoaded() {
	// Check if key button assets are missing OR if interactive elements are nil
	assetsNeedReload := s.assets["cc.entry.help.rollover"] == nil || s.assets["cc.entry.mms.rollover"] == nil
	elementsNeedRecreate := s.helpButton == nil || s.mmsButton == nil

	if assetsNeedReload || elementsNeedRecreate {
		if assetsNeedReload {
			log.Printf("Entry: button assets missing, reloading...")
			s.loadEntryButtonAssets()
		}

		if elementsNeedRecreate {
			log.Printf("Entry: interactive elements missing, recreating...")
		}

		// Always recreate the element renderer with the current assets
		s.elementRenderer = ui.NewElementRenderer(s.assets, s.tm)
		s.elementRenderer.SetFontManager(s.fontManager)

		// Recreate the interactive elements with the updated assets
		s.initializeInteractiveElements()
	}
}

// ensureInteractiveElementsReady ensures interactive elements are always ready for input
// This is called every frame to handle any issues with screen resize or asset loss
func (s *Scene) ensureInteractiveElementsReady() {
	// Check if interactive elements exist and have proper assets
	needsRecreation := false

	if s.helpButton == nil {
		needsRecreation = true
		log.Printf("Entry: help button is nil, needs recreation")
	} else if s.helpButton.Assets[ui.ButtonRollover] == nil {
		needsRecreation = true
		log.Printf("Entry: help button missing rollover asset, needs recreation")
	}

	if s.mmsButton == nil {
		needsRecreation = true
		log.Printf("Entry: MMS button is nil, needs recreation")
	} else if s.mmsButton.Assets[ui.ButtonRollover] == nil {
		needsRecreation = true
		log.Printf("Entry: MMS button missing rollover asset, needs recreation")
	}

	if needsRecreation {
		log.Printf("Entry: recreating interactive elements due to missing elements or assets")

		// Ensure assets are loaded
		s.loadEntryButtonAssets()

		// Recreate element renderer
		s.elementRenderer = ui.NewElementRenderer(s.assets, s.tm)
		s.elementRenderer.SetFontManager(s.fontManager)

		// Recreate interactive elements
		s.initializeInteractiveElements()

		// Restore the CFH text state
		s.updateEntryCfhText(s.tm.First("CALL_FOR_ASSISTANCE"))
	}
}

// updateEntryInfoText updates the entry_info element text (dynamic text replacement)
func (s *Scene) updateEntryInfoText(text string) {
	// Find the entry_info element and update its text content
	// This simulates Lingo's sprite(me.spriteNum).member.text = mytip
	if s.windowDef != nil {
		for i := range s.windowDef.Elements {
			if s.windowDef.Elements[i].ID == "entry_info" {
				// Store the dynamic text - the element renderer will use this instead of the key
				s.windowDef.Elements[i].Key = text
				break
			}
		}
	}
}

// updateEntryCfhText updates the entry_info_cfh element text (for MMS button rollover)
func (s *Scene) updateEntryCfhText(text string) {
	// Find the entry_info_cfh element and update its text content
	// This simulates Lingo's member("cc.entry.cfh").text = TextMgr.GetRefText("MESSENGER")
	if s.windowDef != nil {
		for i := range s.windowDef.Elements {
			if s.windowDef.Elements[i].ID == "entry_info_cfh" {
				// Store the dynamic text - the element renderer will use this instead of the key
				s.windowDef.Elements[i].Key = text
				break
			}
		}
	}
}

// displayRoomDetails updates room information in navigator window (matches Lingo displayRoomDetail)
func (s *Scene) displayRoomDetails(window *ui.Window, studioID string) {
	// Track the selected studio for Go button functionality
	s.selectedStudioID = studioID

	// Find the studio in our studio list
	var selectedStudio *studios.Studio
	for _, studio := range s.studioList.Studios {
		if studio.StudioID == studioID {
			selectedStudio = &studio
			break
		}
	}

	if selectedStudio == nil {
		log.Printf("Entry: studio %s not found for room details", studioID)
		return
	}

	// Update room title (matches Lingo: member("nav_help_title_public").text = roomdetail.name && "(" & roomdetail.userCount & "/" & roomdetail.capacity & ")")
	// For private studios, truncate the room name to 37 characters before adding user count
	roomName := selectedStudio.Name
	log.Printf("Entry: original room name: '%s' (length: %d)", roomName, len(roomName))
	log.Printf("Entry: window name: '%s'", window.Name)

	// Check if this is a private studio info window (nav_private_info)
	isPrivateInfo := window.Name == "nav_private_info"
	log.Printf("Entry: isPrivateInfo: %t (window name: '%s')", isPrivateInfo, window.Name)

	if isPrivateInfo && len(roomName) > 37 {
		roomName = roomName[:37]
		log.Printf("Entry: TRUNCATED room name from '%s' to '%s' (37 chars)", selectedStudio.Name, roomName)
	}

	roomTitle := fmt.Sprintf("%s (%d/%d)", roomName, selectedStudio.UserCount, selectedStudio.Capacity)
	log.Printf("Entry: final room title: '%s' (length: %d)", roomTitle, len(roomTitle))
	s.updateWindowElementText(window, "nav_areatitle", roomTitle)

	// For private studios, display owner information below the room name
	if window.Name == "nav_private_info" {
		ownerText := fmt.Sprintf("owner : %s", selectedStudio.Owner)
		s.updateWindowElementText(window, "nav_areaowner", ownerText)
		log.Printf("Entry: set owner text to '%s'", ownerText)
	}

	// Update room description (matches Lingo: member("nav_helptext").text = roomdetail.description)
	roomDesc := selectedStudio.Description
	if roomDesc == "" {
		// Fallback if description is empty
		locationName := studios.GetLocationName(selectedStudio.Location)
		roomDesc = fmt.Sprintf("Welcome to %s! This is a public studio located in %s.", selectedStudio.Name, locationName)
	}
	s.updateWindowElementText(window, "NAV_DTL_DEST_TITLEtext", roomDesc)

	// Update room icon (matches Lingo room icon logic)
	s.updateRoomIcon(window, selectedStudio)

	// Create/update the Go button (matches Lingo button functionality)
	s.updateGoButton(window, selectedStudio)
	// In private info window, control Modify button visibility based on ownership
	if window.Name == "nav_private_info" {
		s.updateModifyButton(window, selectedStudio)
	}

	// TODO: Update location marker on world map (nav_circleanim positioning)
	// TODO: Update room icon (cc_nav_roomico) based on location

	log.Printf("Entry: displayed details for room %s (%s) - %d/%d users",
		selectedStudio.Name, selectedStudio.StudioID, selectedStudio.UserCount, selectedStudio.Capacity)
}

// updateWindowElementText updates a text element in a window (helper for dynamic text updates)
func (s *Scene) updateWindowElementText(window *ui.Window, elementID, text string) {
	if window == nil || window.Definition == nil {
		log.Printf("Entry: updateWindowElementText failed - window or definition is nil")
		return
	}

	// Find the element and update its text content
	found := false
	for i := range window.Definition.Elements {
		if window.Definition.Elements[i].ID == elementID {
			// Store the dynamic text - the element renderer will use this instead of the key
			window.Definition.Elements[i].Key = text
			log.Printf("Entry: updated element %s with text: %s", elementID, text)
			found = true
			break
		}
	}

	if !found {
		log.Printf("Entry: element %s not found in window %s", elementID, window.Name)
		// List all available element IDs for debugging
		var availableIDs []string
		for _, elem := range window.Definition.Elements {
			if elem.ID != "" {
				availableIDs = append(availableIDs, elem.ID)
			}
		}
		log.Printf("Entry: available element IDs: %v", availableIDs)
	}
}

// updateRoomIcon updates the room icon based on location (matches Lingo room icon logic)
func (s *Scene) updateRoomIcon(window *ui.Window, studio *studios.Studio) {
	if window == nil || studio == nil {
		return
	}

	// Location mapping (matches Lingo: mylocs = ["london", "miami", "mombasa", "newyork", "rio", "seattle", "sydney", "tokyo", "goa", "alaska", "moscow", "redroom"])
	locations := []string{"london", "miami", "mombasa", "newyork", "rio", "seattle", "sydney", "tokyo", "goa", "alaska", "moscow", "redroom"}

	// The studio.Location is an int ID, we need to map it to the correct icon file
	locationIndex := studio.Location

	if locationIndex < 1 || locationIndex > len(locations) {
		log.Printf("Entry: location ID %d out of range for studio %s", studio.Location, studio.StudioID)
		return
	}

	// Get location name for logging
	locationName := studios.GetLocationName(studio.Location)

	// Calculate the correct file number and icon number based on the actual file pattern
	// Pattern: {266+locationIndex}_nav_icon_unit{locationIndex:03d}.png
	fileNumber := 266 + locationIndex
	iconNumber := fmt.Sprintf("%03d", locationIndex)

	// Load the room icon asset (matches actual file naming pattern)
	iconPath := filepath.Join("Lingo", "cc_interface[1]", fmt.Sprintf("%d_nav_icon_unit%s.png", fileNumber, iconNumber))
	log.Printf("Entry: loading room icon from %s for location %s (ID: %d)", iconPath, locationName, studio.Location)

	// Load and set the icon image dynamically
	s.updateWindowElementImage(window, "public_room_icon", iconPath)

	log.Printf("Entry: room icon update for studio %s, location %s (ID: %d) -> icon unit%s", studio.StudioID, locationName, studio.Location, iconNumber)
}

// updateWindowElementImage loads and sets a dynamic image for a window element
func (s *Scene) updateWindowElementImage(window *ui.Window, elementID, imagePath string) {
	if window == nil {
		log.Printf("Entry: updateWindowElementImage failed - window is nil")
		return
	}

	// Load the image from file using our custom loader (applies white->transparent cleanup)
	img, err := lingoassets.LoadImage(imagePath)
	if err != nil {
		log.Printf("Entry: failed to load image %s: %v", imagePath, err)
		return
	}

	// Update the window's assets with the new image
	if window.Assets == nil {
		window.Assets = make(map[string]*ebiten.Image)
	}

	// Store the image with the element ID as the key
	window.Assets[elementID] = img

	// Find the element and update its member to point to the dynamic asset
	if window.Definition != nil {
		for i := range window.Definition.Elements {
			if window.Definition.Elements[i].ID == elementID {
				// Update the member name to match the asset key
				window.Definition.Elements[i].Member = elementID
				log.Printf("Entry: updated element %s with dynamic image from %s", elementID, imagePath)
				return
			}
		}
	}

	log.Printf("Entry: element %s not found for image update", elementID)
}

// updateGoButton creates/updates the interactive Go button for room entry
func (s *Scene) updateGoButton(window *ui.Window, studio *studios.Studio) {
	if window == nil || studio == nil {
		return
	}

	// Load the button assets if not already loaded
	buttonAssets := []string{
		"button.e1.left.active",
		"button.e1.middle.active",
		"button.e1.right.active",
		"button.e1.left.pressed",
		"button.e1.middle.pressed",
		"button.e1.right.pressed",
	}

	searchDirs := []string{"Lingo/cc_interface[1]"}
	loadedAssets := lingoassets.LoadAssetsByNames(buttonAssets, searchDirs...)

	// Add the loaded assets to the window's asset collection
	if window.Assets == nil {
		window.Assets = make(map[string]*ebiten.Image)
	}

	for name, img := range loadedAssets {
		if window.Assets[name] == nil {
			window.Assets[name] = img
			log.Printf("Entry: loaded Go button asset %s", name)
		}
	}

	// Find the Go button element and convert it to an interactive element
	if window.Definition != nil {
		for i := range window.Definition.Elements {
			element := &window.Definition.Elements[i]
			if element.ID == "nav_go_public_button" || element.ID == "nav_go_private_button" {
				// Update element to be a proper interactive button (use same asset as Public)
				element.Media = "bitmap"
				element.Type = "image"                   // Interactive elements need type="image"
				element.Member = "button.e1.left.active" // Base member name for interactive system

				// Create the interactive element map if needed
				if window.Interactive == nil {
					window.Interactive = make(map[string]*ui.InteractiveElement)
				}

				// Create the interactive element if it doesn't exist
				if window.Interactive[element.ID] == nil {
					ie := window.Renderer.CreateInteractiveElement(*element)
					if ie != nil {
						window.Interactive[element.ID] = ie
						log.Printf("Entry: created interactive Go button (%s) for studio %s", element.ID, studio.StudioID)
					}
				}
			}
		}
	}
}

// updateModifyButton shows/hides the Modify button in private info based on ownership and makes it interactive
func (s *Scene) updateModifyButton(window *ui.Window, studio *studios.Studio) {
	if window == nil || studio == nil || window.Definition == nil {
		return
	}
	owner, _ := session.GetCredentials()
	trimmedOwner := strings.TrimSpace(owner)
	trimmedStudioOwner := strings.TrimSpace(studio.Owner)
	isOwner := strings.EqualFold(trimmedOwner, trimmedStudioOwner)

	log.Printf("Entry: updateModifyButton DEBUG:")
	log.Printf("  - Current user: '%s' (len=%d)", trimmedOwner, len(trimmedOwner))
	log.Printf("  - Studio owner: '%s' (len=%d)", trimmedStudioOwner, len(trimmedStudioOwner))
	log.Printf("  - Studio ID: '%s'", studio.StudioID)
	log.Printf("  - Studio Name: '%s'", studio.Name)
	log.Printf("  - IsOwner: %t", isOwner)
	log.Printf("  - Window name: '%s'", window.Name)

	// Load button assets for Modify button (same as other buttons)
	if isOwner {
		buttonAssets := []string{
			"button.b1.left.active",
			"button.b1.middle.active",
			"button.b1.right.active",
			"button.b1.left.pressed",
			"button.b1.middle.pressed",
			"button.b1.right.pressed",
		}

		searchDirs := []string{"Lingo/cc_interface[1]"}
		loadedAssets := lingoassets.LoadAssetsByNames(buttonAssets, searchDirs...)

		// Add the loaded assets to the window's asset collection
		if window.Assets == nil {
			window.Assets = make(map[string]*ebiten.Image)
		}

		for name, img := range loadedAssets {
			if window.Assets[name] == nil {
				window.Assets[name] = img
			}
		}
	}

	// Find the modify button element
	var modifyButtonIndex = -1
	for i := range window.Definition.Elements {
		if window.Definition.Elements[i].ID == "nav_modify_button" {
			modifyButtonIndex = i
			break
		}
	}

	if modifyButtonIndex >= 0 {
		el := &window.Definition.Elements[modifyButtonIndex]

		if isOwner {
			// Show and make interactive
			el.Blend = 100
			log.Printf("Entry: Modify button set to visible for owner")

			// Make it interactive if it's not already
			if window.Interactive == nil {
				window.Interactive = make(map[string]*ui.InteractiveElement)
			}

			if window.Interactive[el.ID] == nil {
				// Convert to interactive element
				el.Media = "bitmap"
				el.Type = "image"
				el.Member = "button.b1.left.active" // Base member name for interactive system

				ie := window.Renderer.CreateInteractiveElement(*el)
				if ie != nil {
					window.Interactive[el.ID] = ie
					log.Printf("Entry: created interactive Modify button for studio %s", studio.StudioID)
				}
			}
		} else {
			// Completely hide the button by removing it from the window entirely
			log.Printf("Entry: Hiding Modify button for non-owner (user: '%s', owner: '%s')",
				strings.TrimSpace(owner), strings.TrimSpace(studio.Owner))

			// Remove from interactive elements if it exists
			if window.Interactive != nil {
				delete(window.Interactive, el.ID)
				log.Printf("Entry: removed Modify button from interactive elements")
			}

			// Hide the element by changing type and using shadow pixel (matches Lingo behavior)
			// This prevents the element renderer from processing it as a button
			el.Blend = 0
			el.Type = "image"          // Change from "button" to prevent button processing
			el.Member = "shadow.pixel" // Use shadow pixel like original Lingo code
			el.LocH = -1000            // Move far off-screen
			el.LocV = -1000
			el.Width = 0
			el.Height = 0
		}
	} else {
		log.Printf("Entry: nav_modify_button element not found in window definition")
	}
}

// setupNavigatorCallbacks sets up room list click callbacks for any navigator window
func (s *Scene) setupNavigatorCallbacks(window *ui.Window) {
	if window == nil {
		return
	}

	// Set up room list click callback (matches Lingo roomclicked behavior)
	window.RoomListClick = func(studioID string, isGoButton bool) {
		if isGoButton {
			log.Printf("Entry: Go button clicked for studio %s - entering studio", studioID)
			s.enterStudio(studioID)
		} else {
			if strings.Contains(window.Name, "nav_private") {
				log.Printf("Entry: Room name clicked for studio %s - transitioning to nav_private_info", studioID)
				s.transitionToNavPrivateInfo(studioID)
			} else {
				log.Printf("Entry: Room name clicked for studio %s - transitioning to nav_public_info", studioID)
				s.transitionToNavPublicInfo(studioID)
			}
		}
	}

	// Set up tab transition callback (matches Lingo generictab behavior)
	window.TabTransition = func(fromTab, toTab int) {
		s.handleTabTransition(window, fromTab, toTab)
	}

	// Set up Go button callback (for nav_go_public_button interactive element)
	window.GoButtonClick = func() {
		if s.selectedStudioID != "" {
			log.Printf("Entry: Go button clicked - entering studio %s", s.selectedStudioID)
			s.enterStudio(s.selectedStudioID)
		} else {
			log.Printf("Entry: Go button clicked but no studio selected")
		}
	}

	// Convert private navigation buttons to interactive elements
	if strings.Contains(window.Name, "nav_private") {
		s.setupPrivateNavigationButtons(window)
	}

	// Handle private navigation button clicks (Show My Studios)
	window.ButtonClick = func(elementID string) {
		switch elementID {
		case "nav_priv_popular":
			// Show Active Studios: refresh PRIVATE studios list using the same call as initial load
			url := session.GetGatewayURL()
			if strings.TrimSpace(url) == "" {
				log.Printf("Entry: cannot load Active Studios - missing gateway URL")
				return
			}
			httpClient := &http.Client{Timeout: 10 * time.Second}
			go func() {
				v, err := gateway.GetAllPrivateStudios(httpClient, url)
				if err != nil {
					log.Printf("Entry: getAllPrivateStudios failed: %v", err)
					return
				}
				// Normalize AMF array (StrictArray or ECMAArray)
				var elems []any
				switch x := v.(type) {
				case []any:
					elems = x
				case map[string]any:
					length := 0
					if l, ok := x["length"].(float64); ok {
						length = int(l)
					}
					if length == 0 {
						length = len(x)
					}
					for i := 0; i < length; i++ {
						key := fmt.Sprintf("%d", i)
						if el, ok := x[key]; ok {
							elems = append(elems, el)
						}
					}
				default:
					log.Printf("Entry: unexpected getAllPrivateStudios type: %T", v)
					return
				}
				// Build private studios list from backend response
				items := make([]ui.RoomListItem, 0)
				for _, el := range elems {
					if m, ok := el.(map[string]any); ok {
						var studioID, name string
						var userCount, capacity int
						// Prefer lowercase keys
						if v, ok := m["studioid"]; ok {
							if s, ok2 := v.(string); ok2 {
								studioID = s
							}
						}
						if v, ok := m["name"]; ok {
							if s, ok2 := v.(string); ok2 {
								name = s
							}
						}
						if v, ok := m["currentoccupancy"]; ok {
							switch t := v.(type) {
							case float64:
								userCount = int(t)
							case int:
								userCount = t
							}
						}
						if v, ok := m["maxoccupancy"]; ok {
							switch t := v.(type) {
							case float64:
								capacity = int(t)
							case int:
								capacity = t
							}
						}
						// Fallback to mixed-case keys
						if studioID == "" {
							if v, ok := m["studioID"]; ok {
								if s, ok2 := v.(string); ok2 {
									studioID = s
								}
							}
						}
						if name == "" {
							if v, ok := m["Name"]; ok {
								if s, ok2 := v.(string); ok2 {
									name = s
								}
							}
						}
						if userCount == 0 {
							if v, ok := m["currentOccupancy"]; ok {
								switch t := v.(type) {
								case float64:
									userCount = int(t)
								case int:
									userCount = t
								}
							}
						}
						if capacity == 0 {
							if v, ok := m["maxOccupancy"]; ok {
								switch t := v.(type) {
								case float64:
									capacity = int(t)
								case int:
									capacity = t
								}
							}
						}
						if studioID != "" && name != "" {
							items = append(items, ui.RoomListItem{StudioID: studioID, Name: name, UserCount: userCount, Capacity: capacity})
						}
					}
				}
				// Update the private tab lists in both nav_private_start and nav_private_info (if open)
				if w := s.windowManager.GetWindow("nav_private_start"); w != nil && w.Lists != nil {
					if list := w.Lists["nav_private_rooms_list"]; list != nil {
						list.Items = items
						list.ScrollIndex = 1
						list.UpdateContent()
					}
				}
				if iw := s.windowManager.GetWindow("nav_private_info"); iw != nil && iw.Lists != nil {
					if list := iw.Lists["nav_private_rooms_list"]; list != nil {
						list.Items = items
						list.ScrollIndex = 1
						list.UpdateContent()
					}
				}
			}()
		case "nav_priv_own":
			// Call gateway getByOwnerName with current screen name
			sn, _ := session.GetCredentials()
			url := session.GetGatewayURL()
			if strings.TrimSpace(sn) == "" || strings.TrimSpace(url) == "" {
				log.Printf("Entry: cannot load My Studios - missing credentials or gateway URL")
				return
			}
			httpClient := &http.Client{Timeout: 10 * time.Second}
			go func() {
				v, err := gateway.GetStudiosByOwnerName(httpClient, url, sn)
				if err != nil {
					log.Printf("Entry: getByOwnerName failed: %v", err)
					return
				}
				items := make([]ui.RoomListItem, 0)
				// Collect full studio records to merge into s.studioList
				var mergeStudios []studios.Studio
				if arr, ok := v.([]any); ok {
					for _, it := range arr {
						if m, ok := it.(map[string]any); ok {
							var studioID, name, owner string
							var userCount, capacity, layout, location int
							// Lowercase keys from AMF typed object
							if sID, ok := m["studioid"]; ok {
								if x, ok2 := sID.(string); ok2 {
									studioID = x
								}
							}
							if nm, ok := m["name"]; ok {
								if x, ok2 := nm.(string); ok2 {
									name = x
								}
							}
							if ow, ok := m["ownername"]; ok {
								if x, ok2 := ow.(string); ok2 {
									owner = x
								}
							}
							if uc, ok := m["currentoccupancy"]; ok {
								switch x := uc.(type) {
								case float64:
									userCount = int(x)
								case int:
									userCount = x
								}
							}
							if capv, ok := m["maxoccupancy"]; ok {
								switch x := capv.(type) {
								case float64:
									capacity = int(x)
								case int:
									capacity = x
								}
							}
							if ly, ok := m["layout"]; ok {
								switch x := ly.(type) {
								case float64:
									lt := int(x)

									layout = lt
								case int:
									lt := x

									layout = lt
								}
							}
							if loc, ok := m["location"]; ok {
								switch x := loc.(type) {
								case float64:
									location = int(x)
								case int:
									location = x
								}
							}
							// Fallbacks to mixed-case keys
							if studioID == "" {
								if sID, ok := m["studioID"]; ok {
									if x, ok2 := sID.(string); ok2 {
										studioID = x
									}
								}
							}
							if name == "" {
								if nm, ok := m["Name"]; ok {
									if x, ok2 := nm.(string); ok2 {
										name = x
									}
								}
							}
							if owner == "" {
								if ow, ok := m["ownerName"]; ok {
									if x, ok2 := ow.(string); ok2 {
										owner = x
									}
								}
							}
							if userCount == 0 {
								if uc, ok := m["currentOccupancy"]; ok {
									switch x := uc.(type) {
									case float64:
										userCount = int(x)
									case int:
										userCount = x
									}
								}
							}
							if capacity == 0 {
								if capv, ok := m["maxOccupancy"]; ok {
									switch x := capv.(type) {
									case float64:
										capacity = int(x)
									case int:
										capacity = x
									}
								}
							}
							if layout == 0 {
								if ly, ok := m["Layout"]; ok {
									switch x := ly.(type) {
									case float64:
										lt := int(x)

										layout = lt
									case int:
										lt := x

										layout = lt
									}
								}
							}
							if location == 0 {
								if loc, ok := m["Location"]; ok {
									switch x := loc.(type) {
									case float64:
										location = int(x)
									case int:
										location = x
									}
								}
							}
							if studioID != "" && name != "" {
								items = append(items, ui.RoomListItem{StudioID: studioID, Name: name, UserCount: userCount, Capacity: capacity})
								mergeStudios = append(mergeStudios, studios.Studio{StudioID: studioID, Name: name, UserCount: userCount, Capacity: capacity, Owner: owner, Location: location, Layout: layout})
							}
						}
					}
				} else {
					log.Printf("Entry: unexpected getByOwnerName response type: %T", v)
				}
				// Update the private rooms lists in both nav_private_start and nav_private_info (tab 2)
				if w := s.windowManager.GetWindow("nav_private_start"); w != nil && w.Lists != nil {
					if list := w.Lists["nav_private_rooms_list"]; list != nil {
						list.Items = items
						list.ScrollIndex = 1
						list.UpdateContent()
					}
				}
				if iw := s.windowManager.GetWindow("nav_private_info"); iw != nil && iw.Lists != nil {
					if list := iw.Lists["nav_private_rooms_list"]; list != nil {
						list.Items = items
						list.ScrollIndex = 1
						list.UpdateContent()
					}
				}
				// Merge private studios into scene studio list so enterStudio can find them
				if len(mergeStudios) > 0 && s.studioList != nil {
					for _, ns := range mergeStudios {
						found := false
						for i := range s.studioList.Studios {
							if strings.EqualFold(strings.TrimSpace(s.studioList.Studios[i].StudioID), strings.TrimSpace(ns.StudioID)) {
								// Update existing
								s.studioList.Studios[i].Name = ns.Name
								s.studioList.Studios[i].Owner = ns.Owner
								s.studioList.Studios[i].UserCount = ns.UserCount
								s.studioList.Studios[i].Capacity = ns.Capacity
								s.studioList.Studios[i].Layout = ns.Layout
								s.studioList.Studios[i].Location = ns.Location
								found = true
								break
							}
						}
						if !found {
							s.studioList.Studios = append(s.studioList.Studios, ns)
						}
					}
				}
			}()
		}
	}

	log.Printf("Entry: set up navigator callbacks for window %s", window.Name)
}

// setupPrivateNavigationButtons converts private navigation buttons to interactive elements (same method as Go button)
func (s *Scene) setupPrivateNavigationButtons(window *ui.Window) {
	if window == nil || window.Definition == nil {
		return
	}

	// List of private navigation buttons that need to be converted
	buttonIDs := []string{"nav_priv_popular", "nav_priv_own", "nav_priv_create_own", "nav_go_search_button"}

	for i := range window.Definition.Elements {
		element := &window.Definition.Elements[i]

		// Check if this is one of the buttons we need to convert
		for _, buttonID := range buttonIDs {
			if element.ID == buttonID && element.Type == "button" {
				// Convert to interactive element (same method as Go button)
				element.Media = "bitmap"
				element.Type = "image" // Interactive elements need type="image"

				// Set appropriate member based on button model (same as Go button method)
				switch element.Model {
				case 2:
					element.Member = "button.b1.left.active" // Base member for model 2 (multi-part)
				case 4:
					element.Member = "cc.navi.search_btn.active" // Search button (single asset)
				}

				// Create the interactive element if it doesn't exist
				if window.Interactive == nil {
					window.Interactive = make(map[string]*ui.InteractiveElement)
				}

				// Create interactive element using renderer (same as Go button)
				if window.Interactive[element.ID] == nil {
					ie := window.Renderer.CreateInteractiveElement(*element)
					if ie != nil {
						window.Interactive[element.ID] = ie
						log.Printf("Entry: created interactive element for button %s (model %d)", element.ID, element.Model)
					} else {
						log.Printf("Entry: failed to create interactive element for button %s", element.ID)
					}
				}

				break
			}
		}
	}
}

// createPrivateButtonCallback creates a callback function for private navigation buttons
func (s *Scene) createPrivateButtonCallback(buttonID string) func() {
	return func() {
		log.Printf("Entry: private navigation button clicked: %s", buttonID)

		switch buttonID {
		case "nav_priv_popular", "nav_priv_own":
			w := s.windowManager.GetWindow("nav_private_start")
			if w != nil && w.ButtonClick != nil {
				w.ButtonClick(buttonID)
			}
		case "nav_priv_create_own":
			w := s.windowManager.GetWindow("nav_private_start")
			if w == nil {
				w = s.windowManager.GetWindow("nav_private_info")
			}
			if w != nil {
				navigator.OpenCreateStudioDialog(s.windowManager, s.tm, w, navigator.Options{
					EnterStudio:   s.enterStudio,
					GetScreenName: func() string { sn, _ := session.GetCredentials(); return sn },
					GetGatewayURL: session.GetGatewayURL,
					GetSecret:     func() string { _, sec := session.GetCredentials(); return sec },
					StudioList:    s.studioList,
				})
			}
		case "nav_go_search_button":
			log.Printf("Entry: TODO - perform search")
		default:
			log.Printf("Entry: unknown private button: %s", buttonID)
		}
	}
}

// transitionToNavPublicInfo transitions from nav_public_start to nav_public_info (matches Lingo roomclicked behavior)
func (s *Scene) transitionToNavPublicInfo(studioID string) {
	// Check if nav_public_info already exists (matches Lingo: if voidp(getaProp(ElementMgr.pOpenWindows, "nav_public_info")))
	if existingWindow := s.windowManager.GetWindow("nav_public_info"); existingWindow != nil {
		log.Printf("Entry: nav_public_info already exists, just updating room details")
		s.displayRoomDetails(existingWindow, studioID)
		return
	}

	// Find current navigator window (matches Lingo: repeat with n = 1 to count(ElementMgr.pOpenWindows))
	var currentWindow *ui.Window
	for _, window := range s.windowManager.GetOpenWindowObjects() {
		if window.Name != "" && len(window.Name) >= 4 && window.Name[:4] == "nav_" {
			currentWindow = window
			break
		}
	}

	if currentWindow == nil {
		log.Printf("Entry: no current navigator window found for transition")
		return
	}

	// Preserve window position and scroll state (matches Lingo: myRect = MyWindow.prect)
	windowRect := currentWindow.Rect
	var scrollIndex int = 1
	var roomData []ui.RoomListItem
	if list := currentWindow.Lists["nav_public_rooms_list"]; list != nil {
		scrollIndex = list.ScrollIndex
		roomData = list.Items
	}

	// Close current window (matches Lingo: MyWindow.closeWindow())
	s.windowManager.CloseWindow(currentWindow.Name)

	// Load nav_public_info window definition (matches Lingo: ElementMgr.newwindow("nav_public_info.window", myRect))
	infoPath := filepath.Join("Lingo", "cc_navigator[1]", "60_nav_public_info.window.txt")
	infoDef, err := window.ParseFile(infoPath)
	if err != nil {
		log.Printf("Entry: failed to load nav_public_info window definition: %v", err)
		return
	}

	// Load assets for nav_public_info
	navigatorDir := filepath.Join("Lingo", "cc_navigator[1]")
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	infoAssets := lingoassets.LoadAssetsForDefinition(infoDef, navigatorDir, interfaceDir)

	// Open nav_public_info window at same position
	infoWindow := s.windowManager.OpenWindow("nav_public_info", infoDef, infoAssets, s.tm)
	infoWindow.Rect = windowRect // Preserve position

	// Set up callbacks for the new window
	s.setupNavigatorCallbacks(infoWindow)

	// Restore room list data and scroll position
	if infoWindow.Lists != nil {
		if list := infoWindow.Lists["nav_public_rooms_list"]; list != nil {
			list.SetItems(roomData)
			list.ScrollIndex = scrollIndex
			list.UpdateContent()
		}
	}

	// Display room details in the new window
	s.displayRoomDetails(infoWindow, studioID)

	log.Printf("Entry: transitioned to nav_public_info window for studio %s", studioID)
}

// transitionToNavPrivateInfo transitions from nav_private_start to nav_private_info (private tab info view)
func (s *Scene) transitionToNavPrivateInfo(studioID string) {
	// If nav_private_info already exists, just update details
	if existingWindow := s.windowManager.GetWindow("nav_private_info"); existingWindow != nil {
		log.Printf("Entry: nav_private_info already exists, updating room details")
		s.displayRoomDetails(existingWindow, studioID)
		return
	}

	// Find current navigator window (any nav_*)
	var currentWindow *ui.Window
	for _, w := range s.windowManager.GetOpenWindowObjects() {
		if w.Name != "" && len(w.Name) >= 4 && w.Name[:4] == "nav_" {
			currentWindow = w
			break
		}
	}
	if currentWindow == nil {
		log.Printf("Entry: no current navigator window for private info transition")
		return
	}

	// Preserve private list state from current window (if available)
	var roomData []ui.RoomListItem
	scrollIndex := 1
	if currentWindow.Lists != nil {
		if list := currentWindow.Lists["nav_private_rooms_list"]; list != nil {
			roomData = append(roomData, list.Items...)
			scrollIndex = list.ScrollIndex
		}
	}

	// Load nav_private_info definition and assets
	infoPath := filepath.Join("Lingo", "cc_navigator[1]", "64_nav_private_info.window.txt")
	infoDef, err := window.ParseFile(infoPath)
	if err != nil {
		log.Printf("Entry: failed to parse nav_private_info: %v", err)
		return
	}
	navigatorDir := filepath.Join("Lingo", "cc_navigator[1]")
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	infoAssets := lingoassets.LoadAssetsForDefinition(infoDef, navigatorDir, interfaceDir)
	// Load button.b1 assets used by private nav buttons (consistency)
	buttonAssets := []string{"button.b1.left.active", "button.b1.middle.active", "button.b1.right.active", "button.b1.left.pressed", "button.b1.middle.pressed", "button.b1.right.pressed"}
	for name, img := range lingoassets.LoadAssetsByNames(buttonAssets, interfaceDir) {
		infoAssets[name] = img
	}

	// Open nav_private_info at same position as current window
	windowRect := currentWindow.Rect
	infoWindow := s.windowManager.OpenWindow("nav_private_info", infoDef, infoAssets, s.tm)
	infoWindow.Rect = windowRect

	// Set up callbacks for the new window
	s.setupNavigatorCallbacks(infoWindow)

	// Restore private room list data and scroll position into nav_private_info
	if infoWindow.Lists != nil {
		if list := infoWindow.Lists["nav_private_rooms_list"]; list != nil {
			list.SetItems(roomData)
			list.ScrollIndex = scrollIndex
			list.UpdateContent()
		}
	}

	// Display room details in the new window (also sets up Go button and Modify visibility)
	s.displayRoomDetails(infoWindow, studioID)

	log.Printf("Entry: transitioned to nav_private_info window for studio %s", studioID)
}

// ensureNavigatorCallbacks ensures all open navigator windows have callbacks set up
func (s *Scene) ensureNavigatorCallbacks() {
	if s.windowManager == nil {
		return
	}

	// Check all open windows for navigator windows that need callbacks
	for _, window := range s.windowManager.GetOpenWindowObjects() {
		// Check if this is a navigator window (matches Lingo window naming pattern)
		if window.Name != "" && len(window.Name) >= 4 && window.Name[:4] == "nav_" {
			// Only set up callbacks if they're not already set
			if window.RoomListClick == nil {
				s.setupNavigatorCallbacks(window)
			}
		}
	}
}

// loadNavigatorAssets is no longer needed; assets are loaded from the window definition via lingoassets.LoadAssetsForDefinition.

// Layout implements the Scene interface
func (s *Scene) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Debug: Track when Layout is called and check interactive element state (only log changes)
	currentSize := [2]int{outsideWidth, outsideHeight}
	currentButtonState := [2]bool{s.helpButton != nil, s.mmsButton != nil}
	currentAssetCount := len(s.assets)
	currentRolloverState := [2]bool{
		s.assets["cc.entry.help.rollover"] != nil,
		s.assets["cc.entry.mms.rollover"] != nil,
	}

	// Only log if something changed
	if s.lastLayoutSize != currentSize || s.lastButtonState != currentButtonState ||
		s.lastAssetCount != currentAssetCount || s.lastRolloverState != currentRolloverState {

		log.Printf("Entry: Layout called with size (%d, %d) [changed from (%d, %d)]",
			outsideWidth, outsideHeight, s.lastLayoutSize[0], s.lastLayoutSize[1])
		log.Printf("Entry: helpButton=%v, mmsButton=%v [was %v, %v]",
			currentButtonState[0], currentButtonState[1], s.lastButtonState[0], s.lastButtonState[1])
		log.Printf("Entry: scene assets count: %d [was %d]", currentAssetCount, s.lastAssetCount)
		log.Printf("Entry: rollover assets: help=%v, mms=%v [was %v, %v]",
			currentRolloverState[0], currentRolloverState[1], s.lastRolloverState[0], s.lastRolloverState[1])

		// Update tracking state
		s.lastLayoutSize = currentSize
		s.lastButtonState = currentButtonState
		s.lastAssetCount = currentAssetCount
		s.lastRolloverState = currentRolloverState
	}

	return 760, 520
}

// handleTabTransition handles tab clicks in navigator windows (matches Lingo generictab.ls behavior)
func (s *Scene) handleTabTransition(currentWindow *ui.Window, fromTab, toTab int) {
	log.Printf("Entry: *** TAB TRANSITION DETECTED *** from %d to %d in window %s", fromTab, toTab, currentWindow.Name)

	// Determine destination window based on current window and target tab
	var destWindow string
	switch {
	case strings.Contains(currentWindow.Name, "nav_public"):
		// Public navigation tabs: 1=public, 2=private, 3=search
		switch toTab {
		case 1:
			destWindow = "nav_public_start"
		case 2:
			destWindow = "nav_private_start"
		case 3:
			destWindow = "nav_search_start"
		}
	case strings.Contains(currentWindow.Name, "nav_private"):
		// Private navigation tabs: 1=public, 2=private, 3=search
		switch toTab {
		case 1:
			destWindow = "nav_public_start"
		case 2:
			destWindow = "nav_private_start"
		case 3:
			destWindow = "nav_search_start"
		}
	case strings.Contains(currentWindow.Name, "nav_search"):
		// Search navigation tabs: 1=public, 2=private, 3=search
		switch toTab {
		case 1:
			destWindow = "nav_public_start"
		case 2:
			destWindow = "nav_private_start"
		case 3:
			destWindow = "nav_search_start"
		}
	}

	if destWindow == "" {
		log.Printf("Entry: unknown tab transition destination for tab %d", toTab)
		return
	}

	// Don't transition to the same window type
	if strings.Contains(currentWindow.Name, destWindow) {
		log.Printf("Entry: tab transition to same window type, ignoring")
		return
	}

	// Handle feature checks (matches Lingo generictab.ls)
	if destWindow == "nav_private_start" {
		if !s.featureSet.IsEnabled("PRIVATE_STUDIOS") {
			log.Printf("Entry: private studios feature disabled, showing alert")
			// TODO: implement alert system
			return
		}
	}

	// Load and transition to destination window
	s.transitionToNavigatorWindow(currentWindow, destWindow)
}

// transitionToNavigatorWindow transitions between navigator windows (matches Lingo generictab.ls)
func (s *Scene) transitionToNavigatorWindow(currentWindow *ui.Window, destWindowName string) {
	// Map window names to file paths
	var windowPath string
	switch destWindowName {
	case "nav_public_start":
		windowPath = filepath.Join("Lingo", "cc_navigator[1]", "59_nav_public_start.window.txt")
	case "nav_private_start":
		windowPath = filepath.Join("Lingo", "cc_navigator[1]", "63_nav_private_start.window.txt")
	case "nav_private_people":
		windowPath = filepath.Join("Lingo", "cc_navigator[1]", "65_nav_private_people.window.txt")
	case "nav_search_start":
		// TODO: implement search window when available
		log.Printf("Entry: nav_search_start not yet implemented")
		return
	default:
		log.Printf("Entry: unknown destination window: %s", destWindowName)
		return
	}

	// Load destination window definition
	destDef, err := window.ParseFile(windowPath)
	if err != nil {
		log.Printf("Entry: failed to load %s window definition: %v", destWindowName, err)
		return
	}

	// Load assets for destination window
	navigatorDir := filepath.Join("Lingo", "cc_navigator[1]")
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	destAssets := lingoassets.LoadAssetsForDefinition(destDef, navigatorDir, interfaceDir)

	// Load additional button assets for private navigation windows
	if destWindowName == "nav_private_start" || destWindowName == "nav_private_people" {
		buttonAssets := []string{
			"button.b1.left.active", "button.b1.middle.active", "button.b1.right.active",
			"button.b1.left.pressed", "button.b1.middle.pressed", "button.b1.right.pressed",
		}
		buttonImages := lingoassets.LoadAssetsByNames(buttonAssets, interfaceDir)
		// Merge button assets into destAssets
		for name, img := range buttonImages {
			destAssets[name] = img
		}
		log.Printf("Entry: loaded %d button.b1 assets for %s", len(buttonImages), destWindowName)
	}

	// Use WindowManager.TransitionWindow to handle the transition with data preservation
	newWindow := s.windowManager.TransitionWindow(currentWindow, destWindowName, destDef, destAssets, s.tm)
	if newWindow == nil {
		log.Printf("Entry: failed to transition to %s", destWindowName)
		return
	}

	// Set up callbacks for the new window
	s.setupNavigatorCallbacks(newWindow)

	// Load appropriate data for the new window
	switch destWindowName {
	case "nav_public_start":
		s.DisplayPublicStudios()
	case "nav_private_start":
		s.displayPrivateStudios()
	case "nav_private_people":
		s.displayPrivatePeople()
	}

	log.Printf("Entry: successfully transitioned to %s", destWindowName)
}

// displayPrivateStudios displays private/user studios in nav_private_start window
func (s *Scene) displayPrivateStudios() {
	window := s.windowManager.GetWindow("nav_private_start")
	if window == nil {
		log.Printf("Entry: nav_private_start window not found")
		return
	}

	// Find the private rooms list
	list := window.Lists["nav_private_rooms_list"]
	if list == nil {
		log.Printf("Entry: nav_private_rooms_list not found in nav_private_start window")
		return
	}

	// Filter studios for private/user studios (not system studios)
	// In the original Lingo, private studios are those owned by users, not "$SYSTEM$"
	var privateStudios []ui.RoomListItem
	log.Printf("Entry: filtering %d total studios for private studios", len(s.studioList.Studios))

	// In real implementation, this would filter for user-owned studios
	// For now, show empty list since all current studios are $SYSTEM$ owned
	for _, studio := range s.studioList.Studios {
		if studio.Owner != "$SYSTEM$" {
			privateStudios = append(privateStudios, ui.RoomListItem{
				StudioID:  studio.StudioID,
				Name:      studio.Name,
				UserCount: studio.UserCount,
				Capacity:  studio.Capacity,
			})
			log.Printf("Entry: added private studio: %s", studio.Name)
		}
	}

	// Update list content
	list.Items = privateStudios
	list.UpdateContent()

	log.Printf("Entry: displayed %d private studios", len(privateStudios))
}

// displayPrivatePeople displays people list in nav_private_people window
func (s *Scene) displayPrivatePeople() {
	window := s.windowManager.GetWindow("nav_private_people")
	if window == nil {
		log.Printf("Entry: nav_private_people window not found")
		return
	}

	// Find the people list
	list := window.Lists["nav_people_list"]
	if list == nil {
		log.Printf("Entry: nav_people_list not found in nav_private_people window")
		return
	}

	// Real people list: empty state until actual users are discovered (no fake seeded players)
	users := make([]ui.UserListItem, 0)

	// Update list content
	list.UserItems = users
	list.UpdateContent()

	log.Printf("Entry: displayed %d users in people list", len(users))
}

// enterStudio initiates studio entry (matches Lingo goToStudio behavior)
func (s *Scene) enterStudio(studioID string) {
	log.Printf("Entry: entering studio %s", studioID)

	// Find the studio
	studio := s.GetStudioByID(studioID)
	if studio == nil {
		log.Printf("Entry: studio %s not found", studioID)
		return
	}

	// Check if this is a private studio (non-$SYSTEM$ owner)
	if studio.Owner != "$SYSTEM$" {
		log.Printf("Entry: entering private studio %s (layout %d)", studio.Name, studio.Layout)

		// Close all windows (matches Lingo ElementMgr.closeAllWindows())
		if s.windowManager != nil {
			s.windowManager.CloseAllWindows()
		}

		// Load backend server info for this studio
		if s.elementRenderer != nil { /* renderer exists so gateway URL injection path likely exists via client */
		}
		// Use boot scene’s gateway URL by reconstructing from page origin and static path
		gatewayURL := fmt.Sprintf("%s%s", env.PageOrigin(), "/sf/gateway")
		if srv, err := gateway.GetGameServerByStudioID(http.DefaultClient, gatewayURL, studioID); err == nil {
			if m, ok := srv.(map[string]any); ok {
				host := fmt.Sprint(m["server"])
				if strings.TrimSpace(host) == "" || host == "<nil>" {
					host = fmt.Sprint(m["dns"])
				}
				port := fmt.Sprint(m["port"])
				log.Printf("GameServer: host=%s port=%s", host, port)

				// Connect realtime socket, login, and enter room (prefer reusing boot-time session)
				if sn, sec := session.GetCredentials(); sn != "" && sec != "" {
					if rc := session.GetRealtimeClient(); rc != nil && session.IsRealtimeReady() {
						sh, sp := session.GetRealtimeEndpoint()
						if sh == host && sp == port {
							log.Printf("Realtime: reusing existing connection to %s:%s", host, port)
							if err := rc.EnterRoom(studioID); err != nil {
								log.Printf("Realtime enter room failed (reused client): %v", err)
							}
						} else {
							log.Printf("Realtime: existing connection is to %s:%s (needs %s:%s) -> new connect", sh, sp, host, port)
							ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
							defer cancel()
							rc2 := realtime.New(sec, log.Default())
							if err := rc2.Connect(ctx, host, port); err != nil {
								log.Printf("Realtime connect failed: %v", err)
							} else {
								loginCtx, cancelLogin := context.WithTimeout(context.Background(), 6*time.Second)
								defer cancelLogin()
								if err := rc2.SendLoginAndWait(loginCtx, sn, sec); err != nil {
									log.Printf("Realtime login send failed: %v", err)
								} else {
									session.SetRealtimeClient(rc2)
									session.SetRealtimeEndpoint(host, port)
									session.SetRealtimeReady(true)
									if err := rc2.EnterRoom(studioID); err != nil {
										log.Printf("Realtime enter room failed: %v", err)
									}
								}
							}
						}
					} else {
						log.Printf("Realtime: no existing connection; connecting to %s:%s", host, port)
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						rc := realtime.New(sec, log.Default())
						if err := rc.Connect(ctx, host, port); err != nil {
							log.Printf("Realtime connect failed: %v", err)
						} else {
							// Send login and wait for Accepted before proceeding
							loginCtx, cancelLogin := context.WithTimeout(context.Background(), 6*time.Second)
							defer cancelLogin()
							if err := rc.SendLoginAndWait(loginCtx, sn, sec); err != nil {
								log.Printf("Realtime login send failed: %v", err)
							} else {
								session.SetRealtimeClient(rc)
								session.SetRealtimeEndpoint(host, port)
								session.SetRealtimeReady(true)
								if err := rc.EnterRoom(studioID); err != nil {
									log.Printf("Realtime enter room failed: %v", err)
								}
							}
						}
					}
				} else {
					log.Printf("Realtime: missing credentials; cannot login")
				}
			}
		} else {
			log.Printf("Entry: getGameServerByStudioID failed: %v", err)
		}

		// Main creates and installs the scene once; room snapshots wait in its inbox.
		s.enterStudioID = studioID
		return
	} else { // Public room ($SYSTEM$)
		log.Printf("Entry: entering public room %s (layout %d)", studio.Name, studio.Layout)

		// Close all windows as we transition into the scene
		if s.windowManager != nil {
			s.windowManager.CloseAllWindows()
		}

		// Resolve game server and ensure realtime connection is established
		gatewayURL := fmt.Sprintf("%s%s", env.PageOrigin(), "/sf/gateway")
		if srv, err := gateway.GetGameServerByStudioID(http.DefaultClient, gatewayURL, studioID); err == nil {
			if m, ok := srv.(map[string]any); ok {
				host := fmt.Sprint(m["server"])
				if strings.TrimSpace(host) == "" || host == "<nil>" {
					host = fmt.Sprint(m["dns"])
				}
				port := fmt.Sprint(m["port"])
				log.Printf("GameServer: host=%s port=%s", host, port)

				if sn, sec := session.GetCredentials(); sn != "" && sec != "" {
					if rc := session.GetRealtimeClient(); rc != nil && session.IsRealtimeReady() {
						sh, sp := session.GetRealtimeEndpoint()
						if sh == host && sp == port {
							log.Printf("Realtime: reusing existing connection to %s:%s", host, port)
							if err := rc.EnterRoom(studioID); err != nil {
								log.Printf("Realtime enter room failed (reused client): %v", err)
							}
						} else {
							log.Printf("Realtime: switching connection from %s:%s to %s:%s", sh, sp, host, port)
							ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
							defer cancel()
							rc2 := realtime.New(sec, log.Default())
							if err := rc2.Connect(ctx, host, port); err != nil {
								log.Printf("Realtime connect failed: %v", err)
							} else {
								loginCtx, cancelLogin := context.WithTimeout(context.Background(), 6*time.Second)
								defer cancelLogin()
								if err := rc2.SendLoginAndWait(loginCtx, sn, sec); err != nil {
									log.Printf("Realtime login send failed: %v", err)
								} else {
									session.SetRealtimeClient(rc2)
									session.SetRealtimeEndpoint(host, port)
									session.SetRealtimeReady(true)
									if err := rc2.EnterRoom(studioID); err != nil {
										log.Printf("Realtime enter room failed: %v", err)
									}
								}
							}
						}
					} else {
						log.Printf("Realtime: connecting to %s:%s", host, port)
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						rc := realtime.New(sec, log.Default())
						if err := rc.Connect(ctx, host, port); err != nil {
							log.Printf("Realtime connect failed: %v", err)
						} else {
							loginCtx, cancelLogin := context.WithTimeout(context.Background(), 6*time.Second)
							defer cancelLogin()
							if err := rc.SendLoginAndWait(loginCtx, sn, sec); err != nil {
								log.Printf("Realtime login send failed: %v", err)
							} else {
								session.SetRealtimeClient(rc)
								session.SetRealtimeEndpoint(host, port)
								session.SetRealtimeReady(true)
								if err := rc.EnterRoom(studioID); err != nil {
									log.Printf("Realtime enter room failed: %v", err)
								}
							}
						}
					}
				} else {
					log.Printf("Realtime: missing credentials; cannot login")
				}
			}
		} else {
			log.Printf("Entry: getGameServerByStudioID failed: %v", err)
		}

		// Main constructs and installs the requested room once.
		s.enterStudioID = studioID
		return
	}
}

// GetCatalog returns the parsed catalog for downstream scenes
func (s *Scene) GetCatalog() *catalog.Catalog {
	return s.catalog
}

// SetCatalog allows boot to inject the catalog into entry
func (s *Scene) SetCatalog(c *catalog.Catalog) {
	s.catalog = c
}

// RequestInitialStudio follows the same authenticated entry path as the navigator.
func (s *Scene) RequestInitialStudio(roomID string) {
 if s.GetStudioByID(roomID) == nil { log.Printf("Entry: requested launch room %q is unavailable", roomID); return }
 go s.enterStudio(roomID)
}
