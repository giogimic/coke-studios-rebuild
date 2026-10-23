package studio

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"

	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/lingo/window"
	"gocoke/internal/client/ui"
)

// TradeOffer represents one side's current trade offer
type TradeOffer struct {
	// Offered items by this side (store ProdID and PosID)
	Items []BackpackItem
	// Agree flag for this side
	Agrees bool
}

// TradeManager controls the Trading UI (cc_trading) and state for a local trade session
type TradeManager struct {
	scene       *Scene
	wm          *ui.WindowManager
	partnerName string

	// Current offers (my side = right boxes, partner side = left boxes)
	myOffer      TradeOffer
	partnerOffer TradeOffer

	// Cached UI geometry (absolute screen rectangles for hit testing)
	mySlotRects      [6][4]int
	partnerSlotRects [6][4]int
	confirmRect      [4]int
	partnerCheckRect [4]int
	cancelRects      [][4]int

	// Window pointer for convenience
	win *ui.Window

	// Completion marker to avoid reinserting items after successful trade
	completed bool

	// Track if window was previously open to avoid spam
	wasOpen bool

	// Network callbacks
	onSendStart  func(targetName string) error
	onSendAdd    func(posID string) error
	onSendAgree  func() error
	onSendCancel func() error

	// Track if we're in network mode
	networkMode bool
}

func NewTradeManager(scene *Scene) *TradeManager {
	return &TradeManager{scene: scene}
}

// SetCallbacks sets the network callbacks for trade operations
func (tm *TradeManager) SetCallbacks(
	onStart func(string) error,
	onAdd func(string) error,
	onAgree func() error,
	onCancel func() error,
) {
	if tm == nil {
		return
	}
	tm.onSendStart = onStart
	tm.onSendAdd = onAdd
	tm.onSendAgree = onAgree
	tm.onSendCancel = onCancel
	tm.networkMode = onStart != nil || onAdd != nil || onAgree != nil || onCancel != nil
}

// IsOpen returns true if the trading window is open
func (tm *TradeManager) IsOpen() bool {
	if tm == nil || tm.wm == nil {
		return false
	}
	isOpen := tm.wm.GetWindow("cc_trading") != nil
	// Track state changes
	if isOpen && !tm.wasOpen {
		tm.wasOpen = true
	}
	return isOpen
}

// StartTrade opens the trading window with the given partner name
func (tm *TradeManager) StartTrade(partnerName string) {
	if tm == nil || tm.scene == nil || tm.scene.hudManager == nil {
		return
	}
	// Reset state for new session
	tm.completed = false
	tm.wasOpen = true
	tm.myOffer = TradeOffer{}
	tm.partnerOffer = TradeOffer{}
	tm.wm = tm.scene.hudManager.GetWindowManager()
	if tm.wm == nil {
		return
	}
	// Load trading window definition
	defPath := filepath.Join("Lingo", "cc_room", "43_sanfo_trading.window.txt")
	def, err := window.ParseFile(defPath)
	if err != nil {
		return
	}
	// Remove the in-content closebox piece (keep Cancel button and frame close)
	if def != nil && len(def.Elements) > 0 {
		filtered := make([]window.Element, 0, len(def.Elements))
		for _, el := range def.Elements {
			// Drop the closebox sprite that appears in the content area
			if el.ID == "trading_cancel" && el.Member == "closebox.active" {
				continue
			}
			filtered = append(filtered, el)
		}
		def.Elements = filtered
	}
	// Load assets from cc_room and cc_interface[1]
	roomDir := filepath.Join("Lingo", "cc_room")
	ifaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, roomDir, ifaceDir)
	// Ensure checkbox ON/OFF and button assets are available
	extra := lingoassets.LoadAssetsByNames([]string{
		"cc.interface.checkbox.active.off",
		"cc.interface.checkbox.active.on",
		"button.b1.left.active", "button.b1.middle.active", "button.b1.right.active",
		"button.b1.left.pressed", "button.b1.middle.pressed", "button.b1.right.pressed",
	}, ifaceDir)
	for k, v := range extra {
		assets[k] = v
	}
	// Open window
	win := tm.wm.OpenWindow("cc_trading", def, assets, tm.scene.tm)
	if win == nil {
		return
	}
	// Cache pointer
	tm.win = win
	// Set title
	win.Title = "TRADE_TITLE"
	// Offset the frame close button 15px to the left
	win.CloseButtonOffsetX = -15
	// Remove frame shadow elements to avoid rogue shadow artifacts
	if win.FrameDefinition != nil && len(win.FrameDefinition.Elements) > 0 {
		clean := make([]window.Element, 0, len(win.FrameDefinition.Elements))
		for _, fe := range win.FrameDefinition.Elements {
			if strings.Contains(strings.ToLower(fe.Member), "shadow") {
				continue
			}
			clean = append(clean, fe)
		}
		win.FrameDefinition.Elements = clean
	}
	// Compute and set dynamic texts with partner name
	tm.partnerName = strings.TrimSpace(partnerName)
	for i := range win.Definition.Elements {
		el := &win.Definition.Elements[i]
		if el.ID == "trading_heoffers_text" { // left/top label with partner name
			// Lingo: TRADER_BOX_DESC with {tradername}
			key := tm.scene.tm.First("TRADER_BOX_DESC")
			if key == "" {
				key = "{Tradername} offers:"
			}
			// Normalize token case from Lingo dump
			key = strings.ReplaceAll(key, "{tradername}", "{Tradername}")
			el.Key = strings.ReplaceAll(key, "{Tradername}", tm.partnerName)
		}
		if el.ID == "trading_agrees_text" { // left/bottom agrees text with partner name
			key := tm.scene.tm.First("TRADER_AGREES")
			if key == "" {
				key = "{Tradername} agrees"
			}
			key = strings.ReplaceAll(key, "{tradername}", "{Tradername}")
			el.Key = strings.ReplaceAll(key, "{Tradername}", tm.partnerName)
		}
		if el.ID == "trading_instructions_text" { // replace instructions text with BOX_ROLL
			el.Key = "BOX_ROLL"
		}
	}
	// Build geometry for hit testing
	tm.computeGeometry()
	// Draw initial empty slots
	tm.refreshSlotImages()

	// Send TD_ST packet if in network mode
	if tm.networkMode && tm.onSendStart != nil {
		if err := tm.onSendStart(partnerName); err != nil {
			fmt.Printf("TradeManager: Failed to send TD_ST: %v\n", err)
		}
	}

	// If trading with Sophie (bot), auto-select a random item and auto-accept on partner side
	if strings.EqualFold(tm.partnerName, "Sophie") && !tm.networkMode {
		if tm.scene != nil && tm.scene.catalog != nil {
			// Deterministic pick for now: choose item 50 (Coke Couch) or first catalog item
			prodID := "50"
			if tm.scene.catalog.GetItemByID(prodID) == nil {
				if len(tm.scene.catalog.Items) > 0 {
					prodID = tm.scene.catalog.Items[0].ProdID
				}
			}
			if strings.TrimSpace(prodID) != "" {
				// Limit to 6
				if len(tm.partnerOffer.Items) < 6 {
					// Generate a globally-unique PosID across backpack and placed items
					posID := tm.scene.GenerateUniquePosID("pos")
					newItem := BackpackItem{ProdID: prodID, PosID: posID, Attributes: map[string]interface{}{}}
					tm.partnerOffer.Items = append(tm.partnerOffer.Items, newItem)
				}
				// Auto agree for partner
				tm.partnerOffer.Agrees = true
				// Refresh visuals after adding
				tm.refreshSlotImages()
			}
		}
	}
}

// computeGeometry calculates absolute rectangles for slots and controls
func (tm *TradeManager) computeGeometry() {
	if tm.win == nil {
		return
	}
	// Helper to compute absolute rect from element
	abs := func(locH, locV, w, h int) [4]int {
		left := int(tm.win.StageOffsetX) + tm.win.Rect[0] + locH
		top := int(tm.win.StageOffsetY) + tm.win.Rect[1] + locV
		return [4]int{left, top, w, h}
	}
	// Map right side (my) and left side (partner) slots by their IDs
	idsMy := []string{"trading_mystuff_1", "trading_mystuff_2", "trading_mystuff_3", "trading_mystuff_4", "trading_mystuff_5", "trading_mystuff_6"}
	idsHis := []string{"trading_herstuff_1", "trading_herstuff_2", "trading_herstuff_3", "trading_herstuff_4", "trading_herstuff_5", "trading_herstuff_6"}
	for i := range tm.mySlotRects {
		tm.mySlotRects[i] = [4]int{}
		tm.partnerSlotRects[i] = [4]int{}
	}
	for i := range tm.win.Definition.Elements {
		el := tm.win.Definition.Elements[i]
		for idx, id := range idsMy {
			if el.ID == id {
				tm.mySlotRects[idx] = abs(el.LocH, el.LocV, el.Width, el.Height)
			}
		}
		for idx, id := range idsHis {
			if el.ID == id {
				tm.partnerSlotRects[idx] = abs(el.LocH, el.LocV, el.Width, el.Height)
			}
		}
		if el.ID == "trading_confirm_check" {
			tm.confirmRect = abs(el.LocH, el.LocV, el.Width, el.Height)
		}
		if el.ID == "trading_buddycheck_image" {
			tm.partnerCheckRect = abs(el.LocH, el.LocV, el.Width, el.Height)
		}
		if el.ID == "trading_cancel" {
			// Could be multiple (button + close piece)
			tm.cancelRects = append(tm.cancelRects, abs(el.LocH, el.LocV, el.Width, el.Height))
		}
	}
}

// refreshSlotImages composes per-slot images and assigns them into window assets by element ID
func (tm *TradeManager) refreshSlotImages() {
	if tm.win == nil || tm.scene == nil {
		return
	}
	// Ensure any items missing PosIDs get assigned a globally-unique PosID now
	for i := range tm.myOffer.Items {
		if strings.TrimSpace(tm.myOffer.Items[i].PosID) == "" && tm.scene != nil {
			tm.myOffer.Items[i].PosID = tm.scene.GenerateUniquePosID("pos")
		}
	}
	for i := range tm.partnerOffer.Items {
		if strings.TrimSpace(tm.partnerOffer.Items[i].PosID) == "" && tm.scene != nil {
			tm.partnerOffer.Items[i].PosID = tm.scene.GenerateUniquePosID("pos")
		}
	}
	// Compose a custom transparent slot: 1px white frame; opacity differs per side
	base := tm.win.Assets["tradingbox"]
	boxW, boxH := 40, 40
	if base != nil {
		b := base.Bounds()
		boxW, boxH = b.Dx(), b.Dy()
	}
	compose := func(item *BackpackItem, alpha uint8) *ebiten.Image {
		out := ebiten.NewImage(boxW, boxH)
		// Draw 1px frame: faint white with requested alpha
		frameColor := color.RGBA{255, 255, 255, alpha}
		// top/bottom lines
		for x := 0; x < boxW; x++ {
			out.Set(x, 0, frameColor)
			out.Set(x, boxH-1, frameColor)
		}
		// left/right lines
		for y := 0; y < boxH; y++ {
			out.Set(0, y, frameColor)
			out.Set(boxW-1, y, frameColor)
		}
		if item == nil {
			return out
		}
		// Draw small item centered
		img := tm.scene.smallAssets[item.ProdID]
		if img == nil && tm.scene.catalog != nil {
			if ci := tm.scene.catalog.GetItemByID(item.ProdID); ci != nil {
				mmSmall, _ := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_furniture_small"))
				if mmSmall != nil {
					if p, ok := mmSmall.PathFor(ci.ImageBase + "_small"); ok {
						if im, err := lingoassets.LoadImage(p); err == nil {
							img = im
							tm.scene.smallAssets[item.ProdID] = im
						}
					}
				}
			}
		}
		if img != nil {
			bb := img.Bounds()
			o2 := &ebiten.DrawImageOptions{}
			x := float64((boxW - bb.Dx()) / 2)
			y := float64((boxH - bb.Dy()) / 2)
			o2.GeoM.Translate(x, y)
			out.DrawImage(img, o2)
		}
		return out
	}
	// Assign for my slots
	for i := 0; i < 6; i++ {
		var item *BackpackItem
		if i < len(tm.myOffer.Items) {
			item = &tm.myOffer.Items[i]
		}
		// My offer: 100% opacity border
		im := compose(item, 255)
		key := fmt.Sprintf("trading_mystuff_%d", i+1)
		tm.win.Assets[key] = im
	}
	// Assign for partner slots
	for i := 0; i < 6; i++ {
		var item *BackpackItem
		if i < len(tm.partnerOffer.Items) {
			item = &tm.partnerOffer.Items[i]
		}
		// Partner offer: 80% opacity border
		im := compose(item, 204)
		key := fmt.Sprintf("trading_herstuff_%d", i+1)
		tm.win.Assets[key] = im
	}
	// Update checkbox visual based on myAgree
	tm.updateCheckboxVisuals()
}

func (tm *TradeManager) updateCheckboxVisuals() {
	if tm.win == nil {
		return
	}
	for i := range tm.win.Definition.Elements {
		el := &tm.win.Definition.Elements[i]
		if el.ID == "trading_confirm_check" {
			if tm.myOffer.Agrees {
				el.Member = "cc.interface.checkbox.active.on"
			} else {
				el.Member = "cc.interface.checkbox.active.off"
			}
		}
		if el.ID == "trading_buddycheck_image" {
			if tm.partnerOffer.Agrees {
				el.Member = "cc.interface.checkbox.active.on"
			} else {
				el.Member = "cc.interface.checkbox.active.off"
			}
		}
	}
}

// HandleHoldingDrop tries to add the currently held backpack item into a my-slot if mouse release hits one
// Returns true if the item was consumed by the trade UI (caller should ClearHolding and remove from backpack)
func (tm *TradeManager) HandleHoldingDrop(mouseX, mouseY int) bool {
	if tm == nil || tm.win == nil || tm.scene == nil || tm.scene.backpackManager == nil {
		return false
	}
	// Recompute geometry to respect current window position while dragging
	tm.computeGeometry()
	// Only on mouse release
	if !inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		return false
	}
	// If not holding, nothing to do
	if !tm.scene.backpackManager.IsHoldingActive() {
		return false
	}
	// Check if hit any of my slot rects
	idx := -1
	for i := 0; i < 6; i++ {
		r := tm.mySlotRects[i]
		if pointInRect(mouseX, mouseY, r) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return false
	}
	// Enforce max 6 items in my offer: if full and user attempted drop on a slot, just clear holding
	if len(tm.myOffer.Items) >= 6 {
		// Return the held item to backpack by clearing holding (no removal since we didn't consume it)
		if tm.scene != nil && tm.scene.backpackManager != nil {
			tm.scene.backpackManager.ClearHolding()
		}
		return false
	}
	// Add to first available slot (ignore exact idx click; match Lingo simple behavior)
	if len(tm.myOffer.Items) >= 6 {
		// Return the held item to backpack by clearing holding (no removal since we didn't consume it)
		if tm.scene != nil && tm.scene.backpackManager != nil {
			tm.scene.backpackManager.ClearHolding()
		}
		return false // already full; do not consume the held item
	}
	prodID, posID, _, _, _, _ := tm.scene.backpackManager.GetHoldingInfo()
	if strings.TrimSpace(prodID) == "" || strings.TrimSpace(posID) == "" {
		return false
	}
	// Append to my offer if not already present
	for _, it := range tm.myOffer.Items {
		if strings.EqualFold(strings.TrimSpace(it.PosID), strings.TrimSpace(posID)) {
			return false
		}
	}
	tm.myOffer.Items = append(tm.myOffer.Items, BackpackItem{ProdID: prodID, PosID: posID, Attributes: map[string]interface{}{}})
	// Reset agree flags on change
	tm.myOffer.Agrees = false
	tm.partnerOffer.Agrees = false
	// Refresh visuals
	tm.refreshSlotImages()

	// Send TD_AD packet if in network mode
	if tm.networkMode && tm.onSendAdd != nil {
		if err := tm.onSendAdd(posID); err != nil {
			fmt.Printf("TradeManager: Failed to send TD_AD: %v\n", err)
		}
	}
	return true
}

// Update processes clicks for confirm/cancel and completes the trade if both agree
func (tm *TradeManager) Update() {
	if tm == nil || tm.win == nil {
		return
	}
	// If the window was closed via the frame close button, return pending items
	if tm.wm != nil && tm.wm.GetWindow("cc_trading") == nil {
		// Window no longer open: treat as cancel (unless trade was completed)
		if !tm.completed {
			tm.cancelTrade()
		}
		// Clear wasOpen since window is now closed
		tm.wasOpen = false
		return
	}
	// Keep interactive rects in sync with the window's current position
	tm.computeGeometry()
	mouseX, mouseY := ebiten.CursorPosition()

	// Update hover help text: show item name when hovering a populated slot, else BOX_ROLL
	if tm.win != nil {
		hoverName := ""
		// Check my slots first
		for i := 0; i < 6; i++ {
			if pointInRect(mouseX, mouseY, tm.mySlotRects[i]) && i < len(tm.myOffer.Items) {
				it := tm.myOffer.Items[i]
				if tm.scene != nil && tm.scene.backpackManager != nil {
					name := tm.scene.backpackManager.itemDisplayName(it)
					if strings.TrimSpace(name) == "" {
						name = it.ProdID
					}
					hoverName = name
				}
				break
			}
		}
		// If not found, check partner slots
		if strings.TrimSpace(hoverName) == "" {
			for i := 0; i < 6; i++ {
				if pointInRect(mouseX, mouseY, tm.partnerSlotRects[i]) && i < len(tm.partnerOffer.Items) {
					it := tm.partnerOffer.Items[i]
					if tm.scene != nil && tm.scene.backpackManager != nil {
						name := tm.scene.backpackManager.itemDisplayName(it)
						if strings.TrimSpace(name) == "" {
							name = it.ProdID
						}
						hoverName = name
					}
					break
				}
			}
		}
		// Apply text to instructions field
		for i := range tm.win.Definition.Elements {
			el := &tm.win.Definition.Elements[i]
			if el.ID == "trading_instructions_text" {
				if strings.TrimSpace(hoverName) != "" {
					el.Key = hoverName
				} else {
					el.Key = "BOX_ROLL"
				}
				break
			}
		}
	}

	// Toggle my confirm
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		if pointInRect(mouseX, mouseY, tm.confirmRect) {
			wasAgreed := tm.myOffer.Agrees
			tm.myOffer.Agrees = !tm.myOffer.Agrees
			// No mutation to partner agree here; just update visuals
			tm.updateCheckboxVisuals()

			// Send TD_AG packet if now agreeing in network mode
			if tm.networkMode && tm.onSendAgree != nil && tm.myOffer.Agrees && !wasAgreed {
				if err := tm.onSendAgree(); err != nil {
					fmt.Printf("TradeManager: Failed to send TD_AG: %v\n", err)
				}
			}
		}
		// For local testing, allow toggling partner agree by clicking partner checkbox (only if not in network mode)
		if !tm.networkMode && pointInRect(mouseX, mouseY, tm.partnerCheckRect) {
			tm.partnerOffer.Agrees = !tm.partnerOffer.Agrees
			tm.updateCheckboxVisuals()
		}
		// Cancel
		for _, r := range tm.cancelRects {
			if pointInRect(mouseX, mouseY, r) {
				tm.cancelTrade()
				return
			}
		}
	}
	// Finalize if both agreed (only in local/offline mode, not in network mode)
	if tm.myOffer.Agrees && tm.partnerOffer.Agrees && !tm.networkMode {
		// Transfer items: my offered items go to partner (removed from my backpack),
		// partner offered items are added to my backpack with new unique PosIDs
		if tm.scene != nil && tm.scene.backpackManager != nil {
			for _, it := range tm.myOffer.Items {
				_ = tm.scene.backpackManager.RemoveItemByPosID(it.PosID)
			}
			for _, it := range tm.partnerOffer.Items {
				// Add with a globally unique PosID across placed and backpack
				newPos := tm.scene.GenerateUniquePosID("pos")
				newItem := BackpackItem{ProdID: it.ProdID, PosID: newPos, Attributes: map[string]interface{}{}}
				tm.scene.backpackManager.InsertItemAtFront(newItem)
			}
			// Show brief confirmation text in help area
			for i := range tm.win.Definition.Elements {
				el := &tm.win.Definition.Elements[i]
				if el.ID == "trading_instructions_text" {
					el.Key = "Trade completed!"
				}
			}
		}
		// Mark as completed to prevent reinsertion on close detection
		tm.completed = true
		// Close the window after a small visual delay (simulate by immediate close)
		tm.wm.CloseWindow("cc_trading")
		// Clear state
		tm.win = nil
		tm.wasOpen = false
		return
	}
}

// CancelIfPending cancels and returns items only if the trade wasn't completed
func (tm *TradeManager) CancelIfPending() {
	if tm == nil {
		return
	}
	// Only cancel if we were actually in a trade (window was open)
	if !tm.wasOpen {
		return
	}
	if tm.completed {
		tm.wasOpen = false
		return
	}
	tm.cancelTrade()
	tm.wasOpen = false
}

func (tm *TradeManager) cancelTrade() {
	if tm == nil {
		return
	}

	// Send TD_CN packet if in network mode
	if tm.networkMode && tm.onSendCancel != nil {
		if err := tm.onSendCancel(); err != nil {
			fmt.Printf("TradeManager: Failed to send TD_CN: %v\n", err)
		}
	}

	// Return my offered items back to backpack (if removed elsewhere)
	if tm.scene != nil && tm.scene.backpackManager != nil {
		for _, it := range tm.myOffer.Items {
			// Reinsert at front if not present
			found := false
			for _, cur := range tm.scene.backpackManager.items {
				if strings.TrimSpace(cur.PosID) == strings.TrimSpace(it.PosID) {
					found = true
					break
				}
			}
			if !found {
				tm.scene.backpackManager.InsertItemAtFront(it)
			}
		}
	}
	if tm.wm != nil {
		tm.wm.CloseWindow("cc_trading")
	}
	// Clear
	tm.win = nil
	tm.wasOpen = false
	tm.myOffer = TradeOffer{}
	tm.partnerOffer = TradeOffer{}
}

// Draw overlays if needed (not used currently, but kept for future custom paint)
func (tm *TradeManager) Draw(screen *ebiten.Image) {
	// Draw PosID labels over trade slots when debug Pos labels are enabled (F2)
	if tm == nil || tm.win == nil || tm.scene == nil || tm.scene.backpackManager == nil {
		return
	}
	if !tm.scene.backpackManager.showPosLabels {
		return
	}
	// Ensure rectangles reflect current window position (e.g., while dragging)
	tm.computeGeometry()
	fm := tm.scene.hudManager.GetFontManager()
	if fm == nil {
		return
	}
	face := fm.GetFont("Verdana", "bold", 11)
	if face == nil {
		return
	}
	// Helper to draw label centered above rect
	drawLabel := func(r [4]int, label string, clr color.RGBA) {
		if strings.TrimSpace(label) == "" {
			return
		}
		b := text.BoundString(face, label)
		x := r[0] + (r[2]-b.Dx())/2
		y := r[1] - 4
		text.Draw(screen, label, face, x, y, clr)
	}
	green := color.RGBA{0, 255, 0, 255}
	blue := color.RGBA{0, 170, 255, 255}
	// My slots
	for i := 0; i < 6 && i < len(tm.myOffer.Items); i++ {
		it := tm.myOffer.Items[i]
		drawLabel(tm.mySlotRects[i], strings.TrimSpace(it.PosID), green)
	}
	// Partner slots
	for i := 0; i < 6 && i < len(tm.partnerOffer.Items); i++ {
		it := tm.partnerOffer.Items[i]
		drawLabel(tm.partnerSlotRects[i], strings.TrimSpace(it.PosID), blue)
	}
}

// UpdateFromNetwork updates the trade display from an incoming TD_DISPL packet
func (tm *TradeManager) UpdateFromNetwork(traderName, tradeeName, traderItems, tradeeItems string, traderAccepts, tradeeAccepts bool) {
	// Determine if we are the trader or tradee based on our screen name
	weAreTrader := false
	partnerName := ""
	myItems := ""
	theirItems := ""
	myAccepts := false
	theirAccepts := false

	if tm != nil && tm.scene != nil {
		myName := tm.scene.currentUserName
		if strings.EqualFold(strings.TrimSpace(traderName), strings.TrimSpace(myName)) {
			// We are the trader
			weAreTrader = true
			partnerName = tradeeName
			myItems = traderItems
			theirItems = tradeeItems
			myAccepts = traderAccepts
			theirAccepts = tradeeAccepts
		} else {
			// We are the tradee
			weAreTrader = false
			partnerName = traderName
			myItems = tradeeItems
			theirItems = traderItems
			myAccepts = tradeeAccepts
			theirAccepts = traderAccepts
		}
	}

	if tm == nil || !tm.IsOpen() {
		// If we receive TD_DISPL but window isn't open, open it
		if tm != nil && !tm.IsOpen() && partnerName != "" {
			// Open window without sending TD_ST (it's already been initiated)
			oldNetworkMode := tm.networkMode
			tm.networkMode = false // Temporarily disable to avoid sending TD_ST
			tm.StartTrade(partnerName)
			tm.networkMode = oldNetworkMode
			tm.wasOpen = true // Mark as open since we just opened it for incoming trade
		}
	}

	if tm == nil || tm.win == nil {
		return
	}

	// Update based on our role (trader vs tradee)
	// For our items: only update if we don't have items locally (preserve actual PosIDs from HandleHoldingDrop)
	if len(tm.myOffer.Items) == 0 && myItems != "" {
		// Initial sync or if items were cleared
		tm.myOffer.Items = tm.parseNetworkItems(myItems)
	}
	tm.myOffer.Agrees = myAccepts

	// Update partner's items and agree state from network
	tm.partnerOffer.Items = tm.parseNetworkItems(theirItems)
	tm.partnerOffer.Agrees = theirAccepts

	// Refresh UI
	tm.refreshSlotImages()

	fmt.Printf("TradeManager: Updated from network - we are %s, my items: %s, their items: %s, agrees: %v/%v\n",
		map[bool]string{true: "trader", false: "tradee"}[weAreTrader],
		myItems, theirItems, myAccepts, theirAccepts)
}

// parseNetworkItems parses comma-separated prodIDs from network
func (tm *TradeManager) parseNetworkItems(itemStr string) []BackpackItem {
	items := make([]BackpackItem, 0, 6)
	if itemStr == "" {
		return items
	}

	parts := strings.Split(itemStr, ",")
	for _, prodID := range parts {
		prodID = strings.TrimSpace(prodID)
		if prodID != "" {
			// Generate a unique PosID for network items
			posID := ""
			if tm.scene != nil {
				posID = tm.scene.GenerateUniquePosID("trade")
			}
			items = append(items, BackpackItem{
				ProdID:     prodID,
				PosID:      posID,
				Attributes: make(map[string]interface{}),
			})
		}
	}
	return items
}

// CompleteTradeFromNetwork handles TD_CPLT packet - trade completed
func (tm *TradeManager) CompleteTradeFromNetwork(receivedItems []map[string]string) {
	if tm == nil {
		return
	}

	fmt.Printf("TradeManager: Trade completed from network! Received %d items\n", len(receivedItems))

	// Mark as completed
	tm.completed = true

	if tm.scene != nil && tm.scene.backpackManager != nil {
		// Note: Items have already been removed from backpack when they were added to trade (HandleHoldingDrop)
		// We don't need to remove them again here

		// Add received items to backpack
		for _, item := range receivedItems {
			posID := item["id"]
			prodID := item["cid"]
			if posID != "" && prodID != "" {
				backpackItem := BackpackItem{
					PosID:      posID,
					ProdID:     prodID,
					Attributes: make(map[string]interface{}),
				}
				tm.scene.backpackManager.InsertItemAtFront(backpackItem)
				fmt.Printf("TradeManager: Added received item to backpack - pos:%s prod:%s\n", posID, prodID)
			}
		}
	}

	// Close the window
	if tm.wm != nil {
		tm.wm.CloseWindow("cc_trading")
	}
	tm.win = nil
	tm.wasOpen = false
	tm.myOffer = TradeOffer{}
	tm.partnerOffer = TradeOffer{}
}

func pointInRect(px, py int, r [4]int) bool {
	return px >= r[0] && px < r[0]+r[2] && py >= r[1] && py < r[1]+r[3]
}
