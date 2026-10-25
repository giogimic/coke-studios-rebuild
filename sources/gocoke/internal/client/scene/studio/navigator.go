package studio

import (
		"fmt"

	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/lingo/window"
	"gocoke/internal/client/assets/studios"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/net/gateway"
	"gocoke/internal/client/ui"
)

// NavigatorManager handles all navigator window functionality for the studio scene
type NavigatorManager struct {
	windowManager *ui.WindowManager
	tm            *textmgr.Manager
	studioList    *studios.StudioList

	// Providers for current user and gateway
	getScreenName func() string
	getGatewayURL func() string
}

// NewNavigatorManager creates a new navigator manager
func NewNavigatorManager(windowManager *ui.WindowManager, tm *textmgr.Manager) *NavigatorManager {
	return &NavigatorManager{
		windowManager: windowManager,
		tm:            tm,
	}
}

// SetProviders sets callbacks to retrieve screen name and gateway URL
func (n *NavigatorManager) SetProviders(screenNameProvider func() string, gatewayURLProvider func() string) {
	n.getScreenName = screenNameProvider
	n.getGatewayURL = gatewayURLProvider
}

// SetStudioList allows passing the studio list so the navigator can populate lists
func (n *NavigatorManager) SetStudioList(list *studios.StudioList) {
	n.studioList = list
}

// OpenNavigatorWindow opens the navigator window (same as entry)
func (n *NavigatorManager) OpenNavigatorWindow() {
	if n.windowManager == nil {
		return
	}
	// If already open, close instead (toggle via bottom bar)
	if n.windowManager.GetWindow("nav_public_start") != nil ||
		n.windowManager.GetWindow("nav_private_start") != nil ||
		n.windowManager.GetWindow("nav_private_people") != nil {
		// Close whichever is open (safe close all nav_*)
		for _, w := range n.windowManager.GetOpenWindowObjects() {
			if strings.HasPrefix(w.Name, "nav_") {
				n.windowManager.CloseWindow(w.Name)
			}
		}
		return
	}
	// Load navigator window definition and assets (data-driven)
	navPath := filepath.Join("Lingo", "cc_navigator[1]", "59_nav_public_start.window.txt")
	navDef, err := window.ParseFile(navPath)
	if err != nil {
		return
	}
	// Load all bitmap assets referenced by the navigator window definition from cc_navigator[1] and cc_interface[1]
	navigatorDir := filepath.Join("Lingo", "cc_navigator[1]")
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	navAssets := lingoassets.LoadAssetsForDefinition(navDef, navigatorDir, interfaceDir)

	// Open navigator window using window manager
	navWindow := n.windowManager.OpenWindow("nav_public_start", navDef, navAssets, n.tm)
	// Use the exact rect from the window definition (absolute positioning)
	navWindow.Rect = navDef.Rect
	// Shift the close button left by 10px via window field (affects draw and hit-test consistently)
	navWindow.CloseButtonOffsetX = -15

	// Set up callbacks and private button interactions to match entry behavior
	n.setupNavigatorCallbacks(navWindow)

	// Populate the public rooms list if we have studio list data
	n.displayPublicStudios()
}

// displayPublicStudios emulates Entry.DisplayPublicStudios for studio scene
func (n *NavigatorManager) displayPublicStudios() {
	if n.windowManager == nil {
		return
	}
	window := n.windowManager.GetWindow("nav_public_start")
	if window == nil || window.Lists == nil {
		return
	}
	list := window.Lists["nav_public_rooms_list"]
	if list == nil {
		return
	}
	var publicStudios []ui.RoomListItem
	if n.studioList != nil {
		for _, studio := range n.studioList.Studios {
			if studio.Owner == "$SYSTEM$" {
				publicStudios = append(publicStudios, ui.RoomListItem{
					StudioID:  studio.StudioID,
					Name:      studio.Name,
					UserCount: studio.UserCount,
					Capacity:  studio.Capacity,
				})
			}
		}
	}
	list.Items = publicStudios
	list.UpdateContent()
}

// displayPrivateStudios mirrors entry behavior for private studios list
func (n *NavigatorManager) displayPrivateStudios() {
	if n.windowManager == nil {
		return
	}
	window := n.windowManager.GetWindow("nav_private_start")
	if window == nil || window.Lists == nil {
		return
	}
	list := window.Lists["nav_private_rooms_list"]
	if list == nil {
		return
	}
	var privateStudios []ui.RoomListItem
	if n.studioList != nil {
		for _, studio := range n.studioList.Studios {
			if studio.Owner != "$SYSTEM$" {
				privateStudios = append(privateStudios, ui.RoomListItem{
					StudioID:  studio.StudioID,
					Name:      studio.Name,
					UserCount: studio.UserCount,
					Capacity:  studio.Capacity,
				})
			}
		}
	}
	list.Items = privateStudios
	list.UpdateContent()
}

// displayPrivatePeople mirrors entry placeholder people list
func (n *NavigatorManager) displayPrivatePeople() {
	if n.windowManager == nil {
		return
	}
	window := n.windowManager.GetWindow("nav_private_people")
	if window == nil || window.Lists == nil {
		return
	}
	list := window.Lists["nav_people_list"]
	if list == nil {
		return
	}
	// Real people list: honest empty state instead of seeded players
	users := make([]ui.UserListItem, 0)
	list.UserItems = users
	list.UpdateContent()
}

	// displayRoomDetails updates destination info fields for the selected studio
	func (n *NavigatorManager) displayRoomDetails(window *ui.Window, studioID string) {
		if window == nil || n.studioList == nil {
			return
		}
		// Find the studio
		var selected *studios.Studio
		for i := range n.studioList.Studios {
			if n.studioList.Studios[i].StudioID == studioID {
				selected = &n.studioList.Studios[i]
				break
			}
		}
		if selected == nil {
			log.Printf("Studio Nav: studio %s not found for details", studioID)
			return
		}
		// Title and people count
		title := fmt.Sprintf("%s (%d/%d)", selected.Name, selected.UserCount, selected.Capacity)
		n.updateWindowElementText(window, "nav_areatitle", title)
		// Description
		desc := selected.Description
		if strings.TrimSpace(desc) == "" {
			loc := studios.GetLocationName(selected.Location)
			desc = fmt.Sprintf("Welcome to %s! This studio is located in %s.", selected.Name, loc)
		}
		n.updateWindowElementText(window, "NAV_DTL_DEST_TITLEtext", desc)
	}

	// updateWindowElementText updates a text element in a window definition
	func (n *NavigatorManager) updateWindowElementText(window *ui.Window, elementID, text string) {
		if window == nil || window.Definition == nil {
			return
		}
		for i := range window.Definition.Elements {
			if window.Definition.Elements[i].ID == elementID {
				window.Definition.Elements[i].Key = text
				return
			}
		}
	}


// setupNavigatorCallbacks mirrors the entry scene's navigator callbacks for studio
func (n *NavigatorManager) setupNavigatorCallbacks(window *ui.Window) {
	if window == nil {
		return
	}
	// Room list click: same semantics as entry (Go button or transition to info)
	window.RoomListClick = func(studioID string, isGoButton bool) {
		if isGoButton {
			// TODO: enter studio from studio scene
		} else {
			// If we're in a private nav window, open private info; otherwise public
			if strings.Contains(window.Name, "nav_private") {
				n.transitionToNavPrivateInfo(window, studioID)
			} else {
				n.transitionToNavPublicInfo(window, studioID)
			}
		}
	}
	// Tab transitions
	window.TabTransition = func(fromTab, toTab int) {
		n.handleTabTransition(window, fromTab, toTab)
	}
	// Go button inside navigator
	window.GoButtonClick = func() {
		// No-op in studio for now
	}
	// Initialize private navigation buttons in this window
	n.setupPrivateNavigationButtons(window)

	// Wire private buttons to actions
	window.ButtonClick = func(elementID string) {
		switch elementID {

			case "nav_priv_popular":
				// Show Active Studios: refresh PRIVATE studios list using the same call as initial load
				var url string
				if n.getGatewayURL != nil { url = n.getGatewayURL() }
				if strings.TrimSpace(url) == "" {
					log.Printf("Navigator: cannot load Active Studios - missing gateway URL")
					return
				}
				httpClient := &http.Client{Timeout: 10 * time.Second}
				go func() {
					v, err := gateway.GetAllPrivateStudios(httpClient, url)
					if err != nil {
						log.Printf("Navigator: getAllPrivateStudios failed: %v", err)
						return
					}
					// Normalize AMF array (StrictArray or ECMAArray)
					var elems []any
					switch x := v.(type) {
					case []any:
						elems = x
					case map[string]any:
						length := 0
						if l, ok := x["length"].(float64); ok { length = int(l) }
						if length == 0 { length = len(x) }
						for i := 0; i < length; i++ { key := fmt.Sprintf("%d", i); if el, ok := x[key]; ok { elems = append(elems, el) } }
					default:
						log.Printf("Navigator: unexpected getAllPrivateStudios type: %T", v)
						return
					}
					items := make([]ui.RoomListItem, 0)
					for _, it := range elems {
						if m, ok := it.(map[string]any); ok {
							var studioID, name string
							var userCount, capacity int
							// Prefer lowercase keys first
							if v, ok := m["studioid"]; ok { if s, ok2 := v.(string); ok2 { studioID = s } }
							if v, ok := m["name"]; ok { if s, ok2 := v.(string); ok2 { name = s } }
							if v, ok := m["currentoccupancy"]; ok { switch t := v.(type) { case float64: userCount = int(t); case int: userCount = t } }
							if v, ok := m["maxoccupancy"]; ok { switch t := v.(type) { case float64: capacity = int(t); case int: capacity = t } }
							// Fallback to mixed-case keys
							if studioID == "" { if v, ok := m["studioID"]; ok { if s, ok2 := v.(string); ok2 { studioID = s } } }
							if name == "" { if v, ok := m["Name"]; ok { if s, ok2 := v.(string); ok2 { name = s } } }
							if userCount == 0 { if v, ok := m["currentOccupancy"]; ok { switch t := v.(type) { case float64: userCount = int(t); case int: userCount = t } } }
							if capacity == 0 { if v, ok := m["maxOccupancy"]; ok { switch t := v.(type) { case float64: capacity = int(t); case int: capacity = t } } }
							// Populate with all private studios from backend (not filtering by activity)
							if studioID != "" && name != "" {
								items = append(items, ui.RoomListItem{StudioID: studioID, Name: name, UserCount: userCount, Capacity: capacity})
							}
						}
					}
					// Update private tab list in studio navigator
					if w := n.windowManager.GetWindow("nav_private_start"); w != nil && w.Lists != nil {
						if list := w.Lists["nav_private_rooms_list"]; list != nil {
							list.Items = items
							list.ScrollIndex = 1
							list.UpdateContent()
						}
					}
				}()
		case "nav_priv_own":
			// Show my Studios: fetch by owner and populate tab 2 list
			var sn, url string
			if n.getScreenName != nil {
				sn = n.getScreenName()
			}
			if n.getGatewayURL != nil {
				url = n.getGatewayURL()
			}
			if strings.TrimSpace(sn) == "" || strings.TrimSpace(url) == "" {
				log.Printf("Navigator: missing credentials or gateway URL; cannot load my studios")
				return
			}
			httpClient := &http.Client{Timeout: 10 * time.Second}
			go func() {
				log.Printf("Navigator: loading studios for owner %s...", sn)
				v, err := gateway.GetStudiosByOwnerName(httpClient, url, sn)
				if err != nil {
					log.Printf("Navigator: getByOwnerName failed: %v", err)
					return
				}
				items := make([]ui.RoomListItem, 0)
				if arr, ok := v.([]any); ok {
					for _, it := range arr {
						if m, ok := it.(map[string]any); ok {
							var studioID, name string
							var userCount, capacity int
							// Prefer lowercase keys
							if sID, ok := m["studioid"]; ok {
								if x, ok2 := sID.(string); ok2 {
									studioID = x
								}
							}
							if n2, ok := m["name"]; ok {
								if x, ok2 := n2.(string); ok2 {
									name = x
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
							// Fallback to mixed-case keys
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
							if studioID != "" && name != "" {
								items = append(items, ui.RoomListItem{StudioID: studioID, Name: name, UserCount: userCount, Capacity: capacity})
							}
						}
					}
				} else {
					log.Printf("Navigator: unexpected getByOwnerName response type: %T", v)
				}
				if w := n.windowManager.GetWindow("nav_private_start"); w != nil && w.Lists != nil {
					if list := w.Lists["nav_private_rooms_list"]; list != nil {
						list.Items = items
						list.ScrollIndex = 1
						list.UpdateContent()
					}
				}
			}()
		}
	}
}

// setupPrivateNavigationButtons converts private navigation buttons to interactive elements (same as entry)
func (n *NavigatorManager) setupPrivateNavigationButtons(window *ui.Window) {
	if window == nil || window.Definition == nil {
		return
	}
	buttonIDs := []string{"nav_priv_popular", "nav_priv_own", "nav_priv_create_own", "nav_go_search_button"}
	for i := range window.Definition.Elements {
		element := &window.Definition.Elements[i]
		for _, buttonID := range buttonIDs {
			if element.ID == buttonID && element.Type == "button" {
				// Convert to interactive button rendering
				element.Media = "bitmap"
				element.Type = "image"
				switch element.Model {
				case 2:
					element.Member = "button.b1.left.active"
				case 4:
					element.Member = "cc.navi.search_btn.active"
				}
				if window.Interactive == nil {
					window.Interactive = make(map[string]*ui.InteractiveElement)
				}
				if window.Interactive[element.ID] == nil {
					ie := window.Renderer.CreateInteractiveElement(*element)
					if ie != nil {
						window.Interactive[element.ID] = ie
					}
				}
				break
			}
		}
	}
}

// transitionToNavPublicInfo opens the nav_public_info window from current navigator window
func (n *NavigatorManager) transitionToNavPublicInfo(currentWindow *ui.Window, studioID string) {
	if currentWindow == nil || n.windowManager == nil {
		return
	}
	windowRect := currentWindow.Rect
	// Load nav_public_info
	infoPath := filepath.Join("Lingo", "cc_navigator[1]", "60_nav_public_info.window.txt")
	infoDef, err := window.ParseFile(infoPath)
	if err != nil {
		return
	}
	navigatorDir := filepath.Join("Lingo", "cc_navigator[1]")
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	infoAssets := lingoassets.LoadAssetsForDefinition(infoDef, navigatorDir, interfaceDir)
	infoWindow := n.windowManager.OpenWindow("nav_public_info", infoDef, infoAssets, n.tm)
	infoWindow.Rect = windowRect
	// Keep close button horizontally shifted consistently across navigator sub-windows
	infoWindow.CloseButtonOffsetX = currentWindow.CloseButtonOffsetX
	// Callbacks for new window
	n.setupNavigatorCallbacks(infoWindow)
}

// handleTabTransition switches between nav windows (public/private/search)
func (n *NavigatorManager) handleTabTransition(currentWindow *ui.Window, fromTab, toTab int) {
	if currentWindow == nil || n.windowManager == nil {
		return
	}
	var windowPath string


	var destWindowName string
	switch toTab {
	case 1:
		windowPath = filepath.Join("Lingo", "cc_navigator[1]", "59_nav_public_start.window.txt")
		destWindowName = "nav_public_start"
	case 2:
		windowPath = filepath.Join("Lingo", "cc_navigator[1]", "63_nav_private_start.window.txt")
		destWindowName = "nav_private_start"
	case 3:
		windowPath = filepath.Join("Lingo", "cc_navigator[1]", "65_nav_private_people.window.txt")
		destWindowName = "nav_private_people"
	default:
		return
	}
	// Load destination window
	def, err := window.ParseFile(windowPath)
	if err != nil {
		return
	}
	navigatorDir := filepath.Join("Lingo", "cc_navigator[1]")
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, navigatorDir, interfaceDir)
	// Load additional button assets for private navigation windows (button.b1 multiparts)
	if destWindowName == "nav_private_start" || destWindowName == "nav_private_people" {
		buttonAssets := []string{
			"button.b1.left.active", "button.b1.middle.active", "button.b1.right.active",
			"button.b1.left.pressed", "button.b1.middle.pressed", "button.b1.right.pressed",
		}
		buttonImages := lingoassets.LoadAssetsByNames(buttonAssets, interfaceDir)
		for name, img := range buttonImages {
			assets[name] = img
		}
	}
	// Transition and preserve position
	newWindow := n.windowManager.TransitionWindow(currentWindow, destWindowName, def, assets, n.tm)
	if newWindow != nil {
		// Re-apply callbacks for the new window
		n.setupNavigatorCallbacks(newWindow)
		// Populate lists depending on destination
		switch destWindowName {
		case "nav_public_start":
			n.displayPublicStudios()
		case "nav_private_start":
			n.displayPrivateStudios()
		case "nav_private_people":
			n.displayPrivatePeople()
		}
	}
}


// transitionToNavPrivateInfo opens nav_private_info and preserves tab 2 context (list, buttons), then populates details
func (n *NavigatorManager) transitionToNavPrivateInfo(currentWindow *ui.Window, studioID string) {
	if currentWindow == nil || n.windowManager == nil {
		return
	}
	// If already open, just update details
	if existing := n.windowManager.GetWindow("nav_private_info"); existing != nil {
		n.displayRoomDetails(existing, studioID)
		return
	}
	// Load nav_private_info definition and assets
	infoPath := filepath.Join("Lingo", "cc_navigator[1]", "64_nav_private_info.window.txt")
	infoDef, err := window.ParseFile(infoPath)
	if err != nil {
		return
	}
	navigatorDir := filepath.Join("Lingo", "cc_navigator[1]")
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(infoDef, navigatorDir, interfaceDir)
	// Ensure private button assets exist (needed after interactive conversion)
	buttonAssets := []string{
		"button.b1.left.active", "button.b1.middle.active", "button.b1.right.active",
		"button.b1.left.pressed", "button.b1.middle.pressed", "button.b1.right.pressed",
	}
	buttonImages := lingoassets.LoadAssetsByNames(buttonAssets, interfaceDir)
	for name, img := range buttonImages { assets[name] = img }
	// Transition to nav_private_info (preserves position and list data)
	newWindow := n.windowManager.TransitionWindow(currentWindow, "nav_private_info", infoDef, assets, n.tm)
	if newWindow == nil { return }
	// Re-apply callbacks and populate details
	n.setupNavigatorCallbacks(newWindow)
	n.displayRoomDetails(newWindow, studioID)
}
