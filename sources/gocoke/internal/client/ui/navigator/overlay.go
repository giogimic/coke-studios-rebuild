package navigator

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/lingo/window"
	"gocoke/internal/client/assets/studios"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/net/gateway"
	"gocoke/internal/client/ui"
)

// Options configures the shared navigator overlay used by both entry and studio
type Options struct {
	StudioMode     bool                      // if true, apply studio-specific tweaks (e.g., close button offset)
	EnterStudio    func(studioID string)     // called when Go! is pressed (optional in studio)
	GetScreenName  func() string             // provider used for private nav buttons (optional)
	GetGatewayURL  func() string             // provider used for private nav buttons (optional)
	StudioList     *studios.StudioList       // list for populating public/private tabs
}

// OpenOverlay opens the entry-style navigator as an overlay window into the given WindowManager
// and wires callbacks so other scenes can reuse it without duplicating code.
func OpenOverlay(wm *ui.WindowManager, tm *textmgr.Manager, opts Options) {
	if wm == nil || tm == nil {
		return
	}
	// Toggle: if any nav window is already open, close and return
	if wm.GetWindow("nav_public_start") != nil || wm.GetWindow("nav_private_start") != nil || wm.GetWindow("nav_private_people") != nil || wm.GetWindow("nav_private_info") != nil || wm.GetWindow("nav_public_info") != nil {
		for _, w := range wm.GetOpenWindowObjects() {
			if strings.HasPrefix(w.Name, "nav_") {
				wm.CloseWindow(w.Name)
			}
		}
		return
	}

	// Load definition and assets for start window
	navPath := filepath.Join("Lingo", "cc_navigator[1]", "59_nav_public_start.window.txt")
	def, err := window.ParseFile(navPath)
	if err != nil {
		log.Printf("Navigator(shared): failed to load %s: %v", navPath, err)
		return
	}
	navDir := filepath.Join("Lingo", "cc_navigator[1]")
	ifaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, navDir, ifaceDir)

	w := wm.OpenWindow("nav_public_start", def, assets, tm)
	if w == nil {
		return
	}
	// Absolute rect per definition
	w.Rect = def.Rect
	if opts.StudioMode {
		// Studio overlay uses different shell; shift close button to match visuals
		w.CloseButtonOffsetX = -15
	}
	// Wire callbacks and initial data
	setupNavigatorCallbacks(wm, tm, w, opts)
	displayPublicStudios(wm, opts.StudioList)
}

// setupNavigatorCallbacks wires list clicks, tab transitions, Go button, and private tab buttons
func setupNavigatorCallbacks(wm *ui.WindowManager, tm *textmgr.Manager, window *ui.Window, opts Options) {
	if window == nil {
		return
	}
	// Room list interactions
	window.RoomListClick = func(studioID string, isGoButton bool) {
		if isGoButton {
			if opts.EnterStudio != nil {
				opts.EnterStudio(studioID)
			} else {
				log.Printf("Navigator(shared): Go clicked for %s (no handler)", studioID)
			}
			return
		}
		// Transition to info windows depending on current tab
		if strings.Contains(window.Name, "nav_private") {
			transitionToNavPrivateInfo(wm, tm, window, opts.StudioList, studioID, opts)
		} else {
			transitionToNavPublicInfo(wm, tm, window, opts.StudioList, studioID, opts)
		}
	}
	// Tab transitions
	window.TabTransition = func(fromTab, toTab int) {
		handleTabTransition(wm, tm, window, fromTab, toTab, opts)
	}
	// Go button: use selected studio from window.TempVars if set by displayRoomDetails/updateGoButton
	window.GoButtonClick = func() {
		if opts.EnterStudio == nil {
			log.Printf("Navigator(shared): Go clicked (no handler)")
			return
		}
		if window.TempVars != nil {
			if sid, ok := window.TempVars["selected_studio_id"].(string); ok && strings.TrimSpace(sid) != "" {
				opts.EnterStudio(sid)
				return
			}
		}
		log.Printf("Navigator(shared): Go clicked but no studio selected")
	}
	// Convert private tab buttons to interactive elements and wire click handling
	if strings.Contains(window.Name, "nav_private") {
		setupPrivateNavigationButtons(wm, tm, window, opts)
	}
	window.ButtonClick = func(elementID string) {
		switch elementID {
		case "nav_priv_popular":
			// Refresh PRIVATE studios list from backend (active studios)
			if opts.GetGatewayURL == nil {
				log.Printf("Navigator(shared): cannot load Active Studios - missing gateway URL provider")
				return
			}
			url := opts.GetGatewayURL()
			if strings.TrimSpace(url) == "" {
				log.Printf("Navigator(shared): cannot load Active Studios - empty gateway URL")
				return
			}
			httpClient := &http.Client{Timeout: 10 * time.Second}
			go func() {
				v, err := gateway.GetAllPrivateStudios(httpClient, url)
				if err != nil {
					log.Printf("Navigator(shared): getAllPrivateStudios failed: %v", err)
					return
				}
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
					log.Printf("Navigator(shared): unexpected getAllPrivateStudios type: %T", v)
					return
				}
				items := make([]ui.RoomListItem, 0)
				for _, el := range elems {
					if m, ok := el.(map[string]any); ok {
						var studioID, name string
						var userCount, capacity int
						if v, ok := m["studioid"]; ok { if s, ok2 := v.(string); ok2 { studioID = s } }
						if v, ok := m["name"]; ok { if s, ok2 := v.(string); ok2 { name = s } }
						if v, ok := m["currentoccupancy"]; ok { switch t := v.(type) { case float64: userCount = int(t); case int: userCount = t } }
						if v, ok := m["maxoccupancy"]; ok { switch t := v.(type) { case float64: capacity = int(t); case int: capacity = t } }
						if studioID == "" { if v, ok := m["studioID"]; ok { if s, ok2 := v.(string); ok2 { studioID = s } } }
						if name == "" { if v, ok := m["Name"]; ok { if s, ok2 := v.(string); ok2 { name = s } } }
						if userCount == 0 { if v, ok := m["currentOccupancy"]; ok { switch t := v.(type) { case float64: userCount = int(t); case int: userCount = t } } }
						if capacity == 0 { if v, ok := m["maxOccupancy"]; ok { switch t := v.(type) { case float64: capacity = int(t); case int: capacity = t } } }
						if studioID != "" && name != "" {
							items = append(items, ui.RoomListItem{StudioID: studioID, Name: name, UserCount: userCount, Capacity: capacity})
						}
					}
				}
				// Update lists in both private windows
				if w := wm.GetWindow("nav_private_start"); w != nil && w.Lists != nil { if l := w.Lists["nav_private_rooms_list"]; l != nil { l.Items = items; l.ScrollIndex = 1; l.UpdateContent() } }
				if iw := wm.GetWindow("nav_private_info"); iw != nil && iw.Lists != nil { if l := iw.Lists["nav_private_rooms_list"]; l != nil { l.Items = items; l.ScrollIndex = 1; l.UpdateContent() } }
			}()
		case "nav_priv_own":
			// Show user's own studios
			if opts.GetScreenName == nil || opts.GetGatewayURL == nil {
				log.Printf("Navigator(shared): cannot load My Studios - missing providers")
				return
			}
			sn := strings.TrimSpace(opts.GetScreenName())
			url := strings.TrimSpace(opts.GetGatewayURL())
			if sn == "" || url == "" {
				log.Printf("Navigator(shared): cannot load My Studios - empty screenName or gatewayURL")
				return
			}
			httpClient := &http.Client{Timeout: 10 * time.Second}
			go func() {
				v, err := gateway.GetStudiosByOwnerName(httpClient, url, sn)
				if err != nil {
					log.Printf("Navigator(shared): getByOwnerName failed: %v", err)
					return
				}
				items := make([]ui.RoomListItem, 0)
				var mergeStudios []studios.Studio
				if arr, ok := v.([]any); ok {
					for _, it := range arr {
						if m, ok := it.(map[string]any); ok {
							var studioID, name, owner string
							var userCount, capacity, layout, location int
							if sID, ok := m["studioid"]; ok { if x, ok2 := sID.(string); ok2 { studioID = x } }
							if nm, ok := m["name"]; ok { if x, ok2 := nm.(string); ok2 { name = x } }
							if ow, ok := m["ownername"]; ok { if x, ok2 := ow.(string); ok2 { owner = x } }
							if uc, ok := m["currentoccupancy"]; ok { switch x := uc.(type) { case float64: userCount = int(x); case int: userCount = x } }
							if capv, ok := m["maxoccupancy"]; ok { switch x := capv.(type) { case float64: capacity = int(x); case int: capacity = x } }
							if ly, ok := m["layout"]; ok { switch x := ly.(type) { case float64: lt := int(x); if lt == 2 { lt = 4 } else if lt == 4 { lt = 2 }; layout = lt; case int: lt := x; if lt == 2 { lt = 4 } else if lt == 4 { lt = 2 }; layout = lt } }
							if loc, ok := m["location"]; ok { switch x := loc.(type) { case float64: location = int(x); case int: location = x } }
							if studioID == "" { if sID, ok := m["studioID"]; ok { if x, ok2 := sID.(string); ok2 { studioID = x } } }
							if name == "" { if nm, ok := m["Name"]; ok { if x, ok2 := nm.(string); ok2 { name = x } } }
							if owner == "" { if ow, ok := m["ownerName"]; ok { if x, ok2 := ow.(string); ok2 { owner = x } } }
							if userCount == 0 { if uc, ok := m["currentOccupancy"]; ok { switch x := uc.(type) { case float64: userCount = int(x); case int: userCount = x } } }
							if capacity == 0 { if capv, ok := m["maxOccupancy"]; ok { switch x := capv.(type) { case float64: capacity = int(x); case int: capacity = x } } }
							if layout == 0 { if ly, ok := m["Layout"]; ok { switch x := ly.(type) { case float64: lt := int(x); if lt == 2 { lt = 4 } else if lt == 4 { lt = 2 }; layout = lt; case int: lt := x; if lt == 2 { lt = 4 } else if lt == 4 { lt = 2 }; layout = lt } } }
							if location == 0 { if loc, ok := m["Location"]; ok { switch x := loc.(type) { case float64: location = int(x); case int: location = x } } }
							if studioID != "" && name != "" {
								items = append(items, ui.RoomListItem{StudioID: studioID, Name: name, UserCount: userCount, Capacity: capacity})
								mergeStudios = append(mergeStudios, studios.Studio{StudioID: studioID, Name: name, UserCount: userCount, Capacity: capacity, Owner: owner, Location: location, Layout: layout})
							}
						}
					}
				} else {
					log.Printf("Navigator(shared): unexpected getByOwnerName type: %T", v)
				}
				// Update private lists in both windows
				if w := wm.GetWindow("nav_private_start"); w != nil && w.Lists != nil { if l := w.Lists["nav_private_rooms_list"]; l != nil { l.Items = items; l.ScrollIndex = 1; l.UpdateContent() } }
				if iw := wm.GetWindow("nav_private_info"); iw != nil && iw.Lists != nil { if l := iw.Lists["nav_private_rooms_list"]; l != nil { l.Items = items; l.ScrollIndex = 1; l.UpdateContent() } }
				// Merge private studios into provided StudioList so later lookups succeed
				if len(mergeStudios) > 0 && opts.StudioList != nil {
					for _, ns := range mergeStudios {
						found := false
						for i := range opts.StudioList.Studios {
							if strings.EqualFold(strings.TrimSpace(opts.StudioList.Studios[i].StudioID), strings.TrimSpace(ns.StudioID)) {
								opts.StudioList.Studios[i].Name = ns.Name
								opts.StudioList.Studios[i].Owner = ns.Owner
								opts.StudioList.Studios[i].UserCount = ns.UserCount
								opts.StudioList.Studios[i].Capacity = ns.Capacity
								opts.StudioList.Studios[i].Layout = ns.Layout
								opts.StudioList.Studios[i].Location = ns.Location
								found = true
								break
							}
						}
						if !found {
							opts.StudioList.Studios = append(opts.StudioList.Studios, ns)
						}
					}
				}
			}()
		case "nav_priv_create_own":
			log.Printf("Navigator(shared): TODO - create new private studio")
		case "nav_go_search_button":
			log.Printf("Navigator(shared): TODO - perform search")
		}
	}
}

// handleTabTransition switches navigator windows and populates appropriate data
func handleTabTransition(wm *ui.WindowManager, tm *textmgr.Manager, current *ui.Window, fromTab, toTab int, opts Options) {
	if wm == nil || current == nil {
		return
	}
	var (
		windowPath string
		destWindow string
	)
	switch toTab {
	case 1:
		windowPath = filepath.Join("Lingo", "cc_navigator[1]", "59_nav_public_start.window.txt")
		destWindow = "nav_public_start"
	case 2:
		windowPath = filepath.Join("Lingo", "cc_navigator[1]", "63_nav_private_start.window.txt")
		destWindow = "nav_private_start"
	case 3:
		windowPath = filepath.Join("Lingo", "cc_navigator[1]", "65_nav_private_people.window.txt")
		destWindow = "nav_private_people"
	default:
		return
	}
	def, err := window.ParseFile(windowPath)
	if err != nil { return }
	navDir := filepath.Join("Lingo", "cc_navigator[1]")
	ifaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, navDir, ifaceDir)
	// Ensure private tab button multiparts when needed
	if destWindow == "nav_private_start" || destWindow == "nav_private_people" {
		btnAssets := []string{"button.b1.left.active", "button.b1.middle.active", "button.b1.right.active", "button.b1.left.pressed", "button.b1.middle.pressed", "button.b1.right.pressed"}
		for k, v := range lingoassets.LoadAssetsByNames(btnAssets, ifaceDir) {
			assets[k] = v
		}
	}
	newW := wm.TransitionWindow(current, destWindow, def, assets, tm)
	if newW == nil { return }
	// Preserve studio-mode close offset across transitions
	if opts.StudioMode {
		newW.CloseButtonOffsetX = current.CloseButtonOffsetX
	}
	setupNavigatorCallbacks(wm, tm, newW, opts)
	switch destWindow {
	case "nav_public_start":
		displayPublicStudios(wm, opts.StudioList)
	case "nav_private_start":
		displayPrivateStudios(wm, opts.StudioList)
	case "nav_private_people":
		displayPrivatePeople(wm)
	}
}

// transitionToNavPublicInfo opens nav_public_info at the same rect and wires callbacks
func transitionToNavPublicInfo(wm *ui.WindowManager, tm *textmgr.Manager, current *ui.Window, list *studios.StudioList, studioID string, opts Options) {
	if wm == nil || current == nil { return }
	// Preserve list content and scroll index from current window
	var roomData []ui.RoomListItem
	scrollIndex := 1
	if current.Lists != nil {
		if l := current.Lists["nav_public_rooms_list"]; l != nil {
			roomData = append(roomData, l.Items...)
			scrollIndex = l.ScrollIndex
		}
	}
	// Close current and open info window at same rect
	rect := current.Rect
	wm.CloseWindow(current.Name)
	infoPath := filepath.Join("Lingo", "cc_navigator[1]", "60_nav_public_info.window.txt")
	def, err := window.ParseFile(infoPath)
	if err != nil { return }
	navDir := filepath.Join("Lingo", "cc_navigator[1]")
	ifaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, navDir, ifaceDir)
	w := wm.OpenWindow("nav_public_info", def, assets, tm)
	if w == nil { return }
	w.Rect = rect
	w.CloseButtonOffsetX = current.CloseButtonOffsetX
	// Restore list into info window
	if w.Lists != nil {
		if l := w.Lists["nav_public_rooms_list"]; l != nil {
			l.Items = roomData
			l.ScrollIndex = scrollIndex
			l.UpdateContent()
		}
	}
	setupNavigatorCallbacks(wm, tm, w, opts)
	displayRoomDetails(w, list, studioID, opts)
}

// transitionToNavPrivateInfo opens nav_private_info and populates details
func transitionToNavPrivateInfo(wm *ui.WindowManager, tm *textmgr.Manager, current *ui.Window, list *studios.StudioList, studioID string, opts Options) {
	if wm == nil || current == nil { return }
	// Preserve private list state
	var roomData []ui.RoomListItem
	scrollIndex := 1
	if current.Lists != nil {
		if l := current.Lists["nav_private_rooms_list"]; l != nil {
			roomData = append(roomData, l.Items...)
			scrollIndex = l.ScrollIndex
		}
	}
	infoPath := filepath.Join("Lingo", "cc_navigator[1]", "64_nav_private_info.window.txt")
	def, err := window.ParseFile(infoPath)
	if err != nil { return }
	navDir := filepath.Join("Lingo", "cc_navigator[1]")
	ifaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, navDir, ifaceDir)
	// Ensure button.b1 parts exist for private info (interactive conversion)
	btnAssets := []string{"button.b1.left.active", "button.b1.middle.active", "button.b1.right.active", "button.b1.left.pressed", "button.b1.middle.pressed", "button.b1.right.pressed"}
	for k, v := range lingoassets.LoadAssetsByNames(btnAssets, ifaceDir) { assets[k] = v }
	w := wm.TransitionWindow(current, "nav_private_info", def, assets, tm)
	if w == nil { return }
	// Restore list into info window
	if w.Lists != nil {
		if l := w.Lists["nav_private_rooms_list"]; l != nil {
			l.Items = roomData
			l.ScrollIndex = scrollIndex
			l.UpdateContent()
		}
	}
	setupNavigatorCallbacks(wm, tm, w, opts)
	displayRoomDetails(w, list, studioID, opts)
}

// displayPublicStudios populates the public rooms list
func displayPublicStudios(wm *ui.WindowManager, list *studios.StudioList) {
	if wm == nil { return }
	w := wm.GetWindow("nav_public_start")
	if w == nil || w.Lists == nil { return }
	l := w.Lists["nav_public_rooms_list"]
	if l == nil { return }
	l.Items = nil
	if list != nil {
		for _, st := range list.Studios {
			if st.Owner == "$SYSTEM$" {
				l.Items = append(l.Items, ui.RoomListItem{StudioID: st.StudioID, Name: st.Name, UserCount: st.UserCount, Capacity: st.Capacity})
			}
		}
	}
	l.UpdateContent()
}

// displayPrivateStudios populates the private rooms list
func displayPrivateStudios(wm *ui.WindowManager, list *studios.StudioList) {
	if wm == nil { return }
	w := wm.GetWindow("nav_private_start")
	if w == nil || w.Lists == nil { return }
	l := w.Lists["nav_private_rooms_list"]
	if l == nil { return }
	l.Items = nil
	if list != nil {
		for _, st := range list.Studios {
			if st.Owner != "$SYSTEM$" {
				l.Items = append(l.Items, ui.RoomListItem{StudioID: st.StudioID, Name: st.Name, UserCount: st.UserCount, Capacity: st.Capacity})
			}
		}
	}
	l.UpdateContent()
}

// displayPrivatePeople: leave empty for now to match entry parity (no static placeholder data)
func displayPrivatePeople(wm *ui.WindowManager) {
	if wm == nil { return }
	if w := wm.GetWindow("nav_private_people"); w != nil && w.Lists != nil {
		if l := w.Lists["nav_people_list"]; l != nil {
			l.UserItems = nil
			l.UpdateContent()
		}
	}
}

// displayRoomDetails updates info windows based on selected studio and wires Go/Modify buttons like entry
func displayRoomDetails(w *ui.Window, list *studios.StudioList, studioID string, opts Options) {
	if w == nil || list == nil { return }
	var selected *studios.Studio
	for i := range list.Studios {
		if list.Studios[i].StudioID == studioID {
			selected = &list.Studios[i]
			break
		}
	}
	if selected == nil { return }
	if w.TempVars == nil { w.TempVars = map[string]interface{}{} }
	w.TempVars["selected_studio_id"] = studioID
	// Title: truncate to 37 chars for private info, keep full for public
	roomName := selected.Name
	if w.Name == "nav_private_info" {
		if len(roomName) > 37 { roomName = roomName[:37] }
	}
	title := fmt.Sprintf("%s (%d/%d)", roomName, selected.UserCount, selected.Capacity)
	updateWindowElementText(w, "nav_areatitle", title)
	// Owner label for private info
	if w.Name == "nav_private_info" {
		ownerText := fmt.Sprintf("owner : %s", strings.TrimSpace(selected.Owner))
		updateWindowElementText(w, "nav_areaowner", ownerText)
	}
	// Description
	desc := strings.TrimSpace(selected.Description)
	if desc == "" {
		loc := studios.GetLocationName(selected.Location)
		desc = fmt.Sprintf("Welcome to %s! This studio is located in %s.", selected.Name, loc)
	}
	updateWindowElementText(w, "NAV_DTL_DEST_TITLEtext", desc)
	// Room icon and Go button
	updateRoomIcon(w, selected)
	updateGoButton(w, selected)
	// Modify visibility in private info based on ownership
	if w.Name == "nav_private_info" {
		updateModifyButton(w, selected, opts)
	}
}

func updateWindowElementText(w *ui.Window, elementID, text string) {
	if w == nil || w.Definition == nil { return }
	for i := range w.Definition.Elements {
		if w.Definition.Elements[i].ID == elementID {
			w.Definition.Elements[i].Key = text
			return
		}
	}
}

// setupPrivateNavigationButtons converts private navigation buttons to interactive elements (same as entry)
func setupPrivateNavigationButtons(wm *ui.WindowManager, tm *textmgr.Manager, window *ui.Window, opts Options) {
	if window == nil || window.Definition == nil { return }
	buttonIDs := []string{"nav_priv_popular", "nav_priv_own", "nav_priv_create_own", "nav_go_search_button"}
	for i := range window.Definition.Elements {
		element := &window.Definition.Elements[i]
		for _, id := range buttonIDs {
			if element.ID == id && element.Type == "button" {
				// Convert to interactive image
				element.Media = "bitmap"
				element.Type = "image"
				// Choose base member
				switch element.Model {
				case 2:
					element.Member = "button.b1.left.active"
				case 4:
					element.Member = "cc.navi.search_btn.active"
				}
				// Create interactive element
				if window.Interactive == nil { window.Interactive = make(map[string]*ui.InteractiveElement) }
				if window.Interactive[element.ID] == nil {
					if ie := window.Renderer.CreateInteractiveElement(*element); ie != nil {
						window.Interactive[element.ID] = ie
					}
				}
				break
			}
		}
	}
}

// updateWindowElementImage loads an image and assigns it to a window element by ID
func updateWindowElementImage(w *ui.Window, elementID, imagePath string) {
	if w == nil { return }
	img, err := lingoassets.LoadImage(imagePath)
	if err != nil { return }
	if w.Assets == nil { w.Assets = make(map[string]*ebiten.Image) }
	w.Assets[elementID] = img
	if w.Definition != nil {
		for i := range w.Definition.Elements {
			if w.Definition.Elements[i].ID == elementID {
				w.Definition.Elements[i].Member = elementID
				return
			}
		}
	}
}

// updateRoomIcon updates the room icon image based on studio.Location (same mapping as entry)
func updateRoomIcon(w *ui.Window, studio *studios.Studio) {
	if w == nil || studio == nil { return }
	locationIndex := studio.Location
	if locationIndex < 1 { return }
	fileNumber := 266 + locationIndex
	iconNumber := fmt.Sprintf("%03d", locationIndex)
	iconPath := filepath.Join("Lingo", "cc_interface[1]", fmt.Sprintf("%d_nav_icon_unit%s.png", fileNumber, iconNumber))
	updateWindowElementImage(w, "public_room_icon", iconPath)
}


// updateGoButton creates/updates the interactive Go button for room entry (parity with entry)
func updateGoButton(w *ui.Window, studio *studios.Studio) {
	if w == nil || studio == nil { return }
	// Load button assets
	buttonAssets := []string{
		"button.e1.left.active", "button.e1.middle.active", "button.e1.right.active",
		"button.e1.left.pressed", "button.e1.middle.pressed", "button.e1.right.pressed",
	}
	ifaceDir := "Lingo/cc_interface[1]"
	loaded := lingoassets.LoadAssetsByNames(buttonAssets, ifaceDir)
	if w.Assets == nil { w.Assets = make(map[string]*ebiten.Image) }
	for name, img := range loaded { if w.Assets[name] == nil { w.Assets[name] = img } }
	// Convert Go button element to interactive
	if w.Definition == nil { return }
	if w.Interactive == nil { w.Interactive = make(map[string]*ui.InteractiveElement) }
	for i := range w.Definition.Elements {
		e := &w.Definition.Elements[i]
		if e.ID == "nav_go_public_button" || e.ID == "nav_go_private_button" {
			// Convert to interactive image using same multi-part button assets
			e.Media = "bitmap"
			e.Type = "image"
			e.Member = "button.e1.left.active"
			if w.Interactive[e.ID] == nil {
				if ie := w.Renderer.CreateInteractiveElement(*e); ie != nil {
					w.Interactive[e.ID] = ie
				}
			}
		}
	}
}

// updateModifyButton shows/hides the Modify button in private info based on ownership and makes it interactive
func updateModifyButton(w *ui.Window, studio *studios.Studio, opts Options) {
	if w == nil || studio == nil || w.Definition == nil { return }
	owner := ""
	if opts.GetScreenName != nil { owner = strings.TrimSpace(opts.GetScreenName()) }
	studioOwner := strings.TrimSpace(studio.Owner)
	isOwner := strings.EqualFold(owner, studioOwner)
	// Find the modify button element
	idx := -1
	for i := range w.Definition.Elements {
		if w.Definition.Elements[i].ID == "nav_modify_button" { idx = i; break }
	}
	if idx < 0 { return }
	el := &w.Definition.Elements[idx]
	if isOwner {
		// Ensure assets exist (button.b1 family, same as Entry)
		buttonAssets := []string{
			"button.b1.left.active", "button.b1.middle.active", "button.b1.right.active",
			"button.b1.left.pressed", "button.b1.middle.pressed", "button.b1.right.pressed",
		}
		ifaceDir := "Lingo/cc_interface[1]"
		loaded := lingoassets.LoadAssetsByNames(buttonAssets, ifaceDir)
		if w.Assets == nil { w.Assets = make(map[string]*ebiten.Image) }
		for name, img := range loaded { if w.Assets[name] == nil { w.Assets[name] = img } }
		// Show and make interactive
		el.Blend = 100
		if w.Interactive == nil { w.Interactive = make(map[string]*ui.InteractiveElement) }
		if w.Interactive[el.ID] == nil {
			// Convert to interactive image using b1 model
			el.Media = "bitmap"
			el.Type = "image"
			el.Member = "button.b1.left.active"
			if ie := w.Renderer.CreateInteractiveElement(*el); ie != nil {
				w.Interactive[el.ID] = ie
			}
		}
	} else {
		// Hide completely (mirror Entry behavior)
		if w.Interactive != nil { delete(w.Interactive, el.ID) }
		el.Blend = 0
		el.Type = "image"
		el.Member = "shadow.pixel"
		el.LocH = -1000
		el.LocV = -1000
		el.Width = 0
		el.Height = 0
	}
}
