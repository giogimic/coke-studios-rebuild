package studio

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"gocoke/internal/client/assets/catalog"
	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/lingo/window"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/ui"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2/text"
)

// BackpackItem represents a possession in the user's backpack (minimal fields for placement)
type BackpackItem struct {
	ProdID     string
	PosID      string
	Attributes map[string]interface{}
}

// BackpackManager handles all backpack-related functionality
type BackpackManager struct {
	// Window animation state
	animating         bool
	animPhase         string // "popup" | "popdown"
	targetRect        [4]int // target rect for current phase
	frontBaseLocV     int
	frontClosedOffset int
	holdFrames        int // frames to hold closed front before dropping

	// Backpack contents
	items []BackpackItem
	slots [][2]int // slot anchors (LocH, LocV) from window definition for cc.backpack.item elements

	// Possession loading state
	possessionsLoaded  bool
	loadingPossessions bool
	loadPossessionsFn  func() // Callback to load possessions from gateway

	// Holding preview (mouse-following small icon)
	holdingActive    bool
	holdingProdID    string
	holdingPosID     string
	holdingImage     *ebiten.Image
	holdingImageBase string
	holdingFullImage *ebiten.Image
	holdingWidth     int
	holdingDepth     int
	// Preserve direction when holding from placed item (move action)
	holdingDir    int
	holdingDirSet bool

	// Dependencies
	windowManager *ui.WindowManager
	catalog       *catalog.Catalog
	smallAssets   map[string]*ebiten.Image
	tm            *textmgr.Manager
	fontManager   *ui.FontManager

	// Move/revert support when moving an already placed item
	movingFromPlaced bool
	movedOriginal    PlacedFurni

	// Debug labels
	showPosLabels bool

	// Paging
	currentPage    int
	itemsPerPage   int
	arrowLeftRect  [4]int // relative to window content area: [x, y, w, h]
	arrowRightRect [4]int // relative to window content area: [x, y, w, h]

	// Hover state
	hoverName string

	// Blank CDs
	blankCDs   int
	cdTextRect [4]int // [x,y,w,h] relative to window content
}

// NewBackpackManager creates a new backpack manager
func NewBackpackManager(windowManager *ui.WindowManager, catalog *catalog.Catalog, smallAssets map[string]*ebiten.Image, tm *textmgr.Manager, fm *ui.FontManager) *BackpackManager {
	return &BackpackManager{
		windowManager: windowManager,
		catalog:       catalog,
		smallAssets:   smallAssets,
		tm:            tm,
		fontManager:   fm,
		currentPage:   1,
		itemsPerPage:  25,
	}
}

// SetLoadPossessionsFunc sets the callback function for loading possessions
func (bm *BackpackManager) SetLoadPossessionsFunc(fn func()) {
	bm.loadPossessionsFn = fn
}

// SetItems injects the user's possessions into the backpack (replaces seeds)
func (bm *BackpackManager) SetItems(items []BackpackItem) {
	bm.items = items
	bm.possessionsLoaded = true
	bm.loadingPossessions = false

	// Log the items being set
	log.Printf("BackpackManager: SetItems called with %d items", len(items))
	for i, item := range items {
		if i < 5 { // Log first 5 items for debugging
			log.Printf("  Item %d: ProdID=%s, PosID=%s", i, item.ProdID, item.PosID)
		}
	}

	// Preload small images for smoother rendering
	if bm.catalog == nil {
		return
	}
	mmSmall, _ := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_furniture_small"))
	for _, bi := range bm.items {
		if bm.smallAssets[bi.ProdID] != nil {
			continue
		}
		if catItem := bm.catalog.GetItemByID(bi.ProdID); catItem != nil && mmSmall != nil {
			if p, ok := mmSmall.PathFor(catItem.ImageBase + "_small"); ok {
				if im, err := lingoassets.LoadImage(p); err == nil {
					bm.smallAssets[bi.ProdID] = im
				}
			}
		}
	}
}

// Update handles backpack animation and state updates
func (bm *BackpackManager) Update() error {
	if bm.windowManager == nil {
		return nil
	}

	// Allow starting popdown on closebox click even when not currently animating
	if w := bm.windowManager.GetWindow("cc_backpack"); w != nil {
		if w.TempVars != nil {
			if req, ok := w.TempVars["requestBackpackPopdown"].(bool); ok && req && bm.animPhase != "popdown" {
				w.TempVars["requestBackpackPopdown"] = false
				height := w.Rect[3] - w.Rect[1]
				bm.targetRect = [4]int{w.Rect[0], 540, w.Rect[2], 540 + height}
				bm.animPhase = "popdown"
				bm.animating = true
			}
		}
	}

	// Animate backpack window if needed (mimic 93_backpack Window.ls exitFrame)
	if bm.animating {
		if w := bm.windowManager.GetWindow("cc_backpack"); w != nil {
			// If closebox asked for popdown, switch phase and compute target to bottom
			if w.TempVars != nil {
				if req, ok := w.TempVars["requestBackpackPopdown"].(bool); ok && req && bm.animPhase != "popdown" {
					// Clear request and start popdown
					w.TempVars["requestBackpackPopdown"] = false
					bm.animPhase = "popdown"
					height := w.Rect[3] - w.Rect[1]
					bm.targetRect = [4]int{w.Rect[0], 540, w.Rect[2], 540 + height}
				}
			}
			// While animating popup, show closed front by applying an upward offset
			if w.TempVars == nil {
				w.TempVars = map[string]interface{}{}
			}
			if bm.animPhase == "popup" {
				w.TempVars["backpackFrontOffsetV"] = bm.frontClosedOffset
				w.TempVars["backpackFrontDropped"] = false
				// When we eventually drop the front, nudge the front.down by ~30px for alignment
				w.TempVars["backpackFrontDownOffset"] = 30
			}
			// Move vertical position towards target with easing; slow it down by half (orig was /5)
			curTop := w.Rect[1]
			destTop := bm.targetRect[1]
			diff := (destTop - curTop) / 7
			if diff == 0 {
				// If within 1px, snap to target and finish phase
				w.Rect = bm.targetRect
				if bm.animPhase == "popup" {
					// Hold the closed front for a short time before dropping
					if bm.holdFrames > 0 {
						bm.holdFrames--
						// Keep animating (no drop yet)
						return nil
					}
					// Remove offset and mark dropped = true so we draw front.down strip at bottom
					delete(w.TempVars, "backpackFrontOffsetV")
					w.TempVars["backpackFrontDropped"] = true
				} else if bm.animPhase == "popdown" {
					// Close at end of popdown
					bm.windowManager.CloseWindow("cc_backpack")
				}
				bm.animating = false
			} else {
				w.Rect[1] += diff
				w.Rect[3] += diff
			}
		} else {
			// Window vanished; stop animating
			bm.animating = false
		}
	}

	return nil
}

// HandleInput processes mouse input for backpack interactions
func (bm *BackpackManager) HandleInput() {
	if bm.windowManager == nil {
		return
	}

	// If backpack is open, process item click -> start holding
	if w := bm.windowManager.GetWindow("cc_backpack"); w != nil {
		mouseX, mouseY := ebiten.CursorPosition()
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			// Build slot anchors on first use
			if len(bm.slots) == 0 && w.Definition != nil {
				for _, e := range w.Definition.Elements {
					if e.ID == "cc.backpack.item" {
						bm.slots = append(bm.slots, [2]int{e.LocH, e.LocV})
					}
				}
			}

			// Handle page arrows
			if bm.arrowLeftRect != [4]int{} || bm.arrowRightRect != [4]int{} {
				winLeft := int(w.StageOffsetX) + w.Rect[0]
				winTop := int(w.StageOffsetY) + w.Rect[1]
				// Left arrow
				lx := winLeft + bm.arrowLeftRect[0]
				ly := winTop + bm.arrowLeftRect[1]
				lw := bm.arrowLeftRect[2]
				lh := bm.arrowLeftRect[3]
				if lw == 0 {
					lw = 24
				}
				if lh == 0 {
					lh = 24
				}
				if mouseX >= lx && mouseX < lx+lw && mouseY >= ly && mouseY < ly+lh {
					if bm.currentPage > 1 {
						bm.currentPage--
					}
					return
				}
				// Right arrow
				rx := winLeft + bm.arrowRightRect[0]
				ry := winTop + bm.arrowRightRect[1]
				rw := bm.arrowRightRect[2]
				rh := bm.arrowRightRect[3]
				if rw == 0 {
					rw = 24
				}
				if rh == 0 {
					rh = 24
				}
				pageCount := 1
				if bm.itemsPerPage > 0 {
					pageCount = (len(bm.items) + bm.itemsPerPage - 1) / bm.itemsPerPage
					if pageCount <= 0 {
						pageCount = 1
					}
				}
				if mouseX >= rx && mouseX < rx+rw && mouseY >= ry && mouseY < ry+rh {
					if bm.currentPage < pageCount {
						bm.currentPage++
					}
					return
				}
			}

			// Hit-test each slot against mouse for click
			winLeft := int(w.StageOffsetX) + w.Rect[0]
			winTop := int(w.StageOffsetY) + w.Rect[1]
			start := (bm.currentPage - 1) * bm.itemsPerPage
			end := start + bm.itemsPerPage
			if end > len(bm.items) {
				end = len(bm.items)
			}
			maxSlots := len(bm.slots)
			for idx := 0; idx < maxSlots && (start+idx) < end; idx++ {
				ax := winLeft + bm.slots[idx][0]
				ay := winTop + bm.slots[idx][1]
				// Items are drawn roughly 16x16 centered; use a forgiving 20x20 area
				x0 := ax - 10
				y0 := ay - 10
				x1 := ax + 10
				y1 := ay + 10
				if mouseX >= x0 && mouseX < x1 && mouseY >= y0 && mouseY < y1 {
					bm.startHolding(bm.items[start+idx])
					break
				}
			}
		}

		// Update hover name every frame based on cursor position
		// Build slot anchors if needed
		if len(bm.slots) == 0 && w.Definition != nil {
			for _, e := range w.Definition.Elements {
				if e.ID == "cc.backpack.item" {
					bm.slots = append(bm.slots, [2]int{e.LocH, e.LocV})
				}
			}
		}
		winLeft := int(w.StageOffsetX) + w.Rect[0]
		winTop := int(w.StageOffsetY) + w.Rect[1]
		start := (bm.currentPage - 1) * bm.itemsPerPage
		end := start + bm.itemsPerPage
		if end > len(bm.items) {
			end = len(bm.items)
		}
		maxSlots := len(bm.slots)
		hovered := ""
		for idx := 0; idx < maxSlots && (start+idx) < end; idx++ {
			ax := winLeft + bm.slots[idx][0]
			ay := winTop + bm.slots[idx][1]
			x0 := ax - 10
			y0 := ay - 10
			x1 := ax + 10
			y1 := ay + 10
			if mouseX >= x0 && mouseX < x1 && mouseY >= y0 && mouseY < y1 {
				name := bm.itemDisplayName(bm.items[start+idx])
				if strings.TrimSpace(name) == "" {
					name = bm.items[start+idx].ProdID
				}
				hovered = name
				break
			}
		}
		bm.hoverName = hovered
	}
}

// HandleHoldingInput processes input for the holding state
func (bm *BackpackManager) HandleHoldingInput(placement *PlacementHelper, windowManager *ui.WindowManager) {
	// If currently holding an item, suppress avatar move on click and allow drop on room click
	if bm.holdingActive {
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			// Don't drop if mouse is over any window
			if !(windowManager != nil && windowManager.IsMouseOverAnyWindow()) {
				if placement != nil {
					// Capture current holding PosID before placement (holding state will be cleared on success)
					_, prePosID, _, _, _, _ := bm.GetHoldingInfo()
					placed := placement.PlaceHeldItem()
					if placed {
						// Fallback removal: ensure the exact held item is removed from backpack by original PosID
						if !bm.movingFromPlaced {
							if strings.TrimSpace(prePosID) != "" {
								_ = bm.RemoveItemByPosID(prePosID)
							}
						}
					}
					if !placed {
						// Revert to original position if we were moving an existing placed item
						if bm.movingFromPlaced {
							if placement.scene != nil {
								placement.scene.placedFurni = append(placement.scene.placedFurni, bm.movedOriginal)
								covered := placement.coverageSquares(bm.movedOriginal.Row, bm.movedOriginal.Col, bm.movedOriginal.Width, bm.movedOriginal.Depth, bm.movedOriginal.Dir)
								for _, sq := range covered {
									if sq != nil {
										placement.updateSquareWalkability(sq.GetRow(), sq.GetCol())
									}
								}
							}
						}
						// Abort (reset holding)
						bm.ClearHolding()
					}
				}
			}
		}
	}
}

// ToggleWindow toggles the backpack window open/closed
func (bm *BackpackManager) ToggleWindow() {
	if bm.windowManager == nil {
		return
	}

	// Toggle backpack: if open and not animating, start popdown; else open with popup
	if w := bm.windowManager.GetWindow("cc_backpack"); w != nil {
		if !bm.animating {
			// Animate down to bottom of stage then close
			bm.targetRect = [4]int{w.Rect[0], 540, w.Rect[2], 540 + (w.Rect[3] - w.Rect[1])}
			bm.animating = true
			bm.animPhase = "popdown"
		}
	} else {
		bm.openWindow()
	}
}

// openWindow opens the backpack window
func (bm *BackpackManager) openWindow() {
	if bm.windowManager == nil {
		return
	}

	// Toggle behavior: if already open, close it
	if bm.windowManager.GetWindow("cc_backpack") != nil {
		bm.windowManager.CloseWindow("cc_backpack")
		return
	}

	// Always refresh possessions on open (fetch latest from gateway)
	if bm.loadPossessionsFn != nil {
		if !bm.loadingPossessions {
			log.Printf("BackpackManager: Refreshing possessions on backpack open")
			bm.loadingPossessions = true
			bm.loadPossessionsFn()
		} else {
			log.Printf("BackpackManager: Already loading possessions; opening with current %d items", len(bm.items))
		}
	} else {
		log.Printf("BackpackManager: Opening backpack with %d items (no loader configured)", len(bm.items))
	}

	// Parse backpack window definition from entry, per original Lingo ElementMgr.newwindow("cc.backpack.window")
	defPath := filepath.Join("Lingo", "entry", "92_cc.backpack.window.txt")
	def, err := window.ParseFile(defPath)
	if err != nil {
		return
	}

	// Load assets mentioned in the definition from cc_room (for cc.backpack.* and cc.pack.*) and entry
	// Priority: cc_room then entry so room-specific overrides win
	roomDir := filepath.Join("Lingo", "cc_room")
	entryDir := filepath.Join("Lingo", "entry")
	assets := lingoassets.LoadAssetsForDefinition(def, roomDir, entryDir)
	// Ensure cc.backpack.front.down is available
	extra := lingoassets.LoadAssetsByNames([]string{"cc.backpack.front.down"}, roomDir, entryDir)
	for k, v := range extra {
		assets[k] = v
	}

	// Open the window. Use definition rect exactly; window frame style default is sanfo_full per default.
	w := bm.windowManager.OpenWindow("cc_backpack", def, assets, bm.tm)
	if w == nil {
		return
	}
	w.Rect = def.Rect
	// Start below and animate up to target (replicates Lingo popup)
	startDown := 200
	w.Rect[1] += startDown
	w.Rect[3] += startDown
	// Compute target rect (def rect shifted up by 150, clamped)
	target := def.Rect
	shiftY := 650
	if target[1]-shiftY < 0 {
		shiftY = target[1]
	}
	target[1] -= shiftY
	target[3] -= shiftY
	bm.targetRect = target
	bm.animating = true
	bm.animPhase = "popup"
	// Initialize front positioning values and set closed pose during popup
	bm.frontBaseLocV = 338
	bm.frontClosedOffset = 130
	bm.holdFrames = int(0.25 * 60) // ~250ms at 60 FPS
	if w.Definition != nil {
		for i := range w.Definition.Elements {
			if w.Definition.Elements[i].Member == "cc.backpack.front" {
				w.Definition.Elements[i].LocV = bm.frontBaseLocV - bm.frontClosedOffset
			}
			// Shift left/right arrows by (-2, -2)
			if w.Definition.Elements[i].ID == "cc.pack.leftarrow" || w.Definition.Elements[i].ID == "cc.pack.rightarrow" {
				w.Definition.Elements[i].LocH -= 7
				w.Definition.Elements[i].LocV -= 8
				w.Definition.Elements[i].Media = "bitmap"
				w.Definition.Elements[i].Type = "image"
			}
			if w.Definition.Elements[i].ID == "cc.backpack.closebox" {
				w.Definition.Elements[i].Media = "bitmap"
				w.Definition.Elements[i].Type = "image"
			}
			// Cache blank CD text rect for drawing count
			if w.Definition.Elements[i].Member == "cc.pack.empty.cds" {
				bm.cdTextRect = [4]int{w.Definition.Elements[i].LocH, w.Definition.Elements[i].LocV, w.Definition.Elements[i].Width, w.Definition.Elements[i].Height}
			}
		}
	}
	// Create interactive elements for closebox only (arrows remain static images)
	if w.Interactive == nil {
		w.Interactive = make(map[string]*ui.InteractiveElement)
	}
	for i := range w.Definition.Elements {
		e := w.Definition.Elements[i]
		if e.ID == "cc.backpack.closebox" {
			ie := w.Renderer.CreateInteractiveElement(e)
			if ie != nil {
				w.Interactive[e.ID] = ie
			}
		}
	}
	// Provide a custom drag band: top 90px
	if w.TempVars == nil {
		w.TempVars = map[string]interface{}{}
	}
	contentW := w.Rect[2] - w.Rect[0]
	w.TempVars["customDragRects"] = [][4]int{{0, 0, contentW, 90}}

	// For close button visual alignment like Lingo hand window, shift left slightly
	w.CloseButtonOffsetX = -6

	// Preload small preview images for items we will display
	if bm.catalog != nil {
		for _, bi := range bm.items {
			if img := bm.smallAssets[bi.ProdID]; img == nil {
				if catItem := bm.catalog.GetItemByID(bi.ProdID); catItem != nil {
					base := catItem.ImageBase
					// Small asset member names are in Lingo/cc_furniture_small
					mmSmall, _ := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_furniture_small"))
					if mmSmall != nil {
						if p, ok := mmSmall.PathFor(base + "_small"); ok {
							if im, err := lingoassets.LoadImage(p); err == nil {
								bm.smallAssets[bi.ProdID] = im
							}
						}
					}
				}
			}
		}
	}

	// Cache slot anchors for faster hit testing and rendering
	bm.slots = bm.slots[:0]
	for _, e := range w.Definition.Elements {
		if e.ID == "cc.backpack.item" {
			bm.slots = append(bm.slots, [2]int{e.LocH, e.LocV})
		}
	}
}

// startHolding begins a mouse-following preview for a backpack item
func (bm *BackpackManager) startHolding(item BackpackItem) {
	if bm.catalog == nil {
		return
	}
	// Lookup small image by prodId in preloaded map; fallback to load now
	img := bm.smallAssets[item.ProdID]
	if img == nil {
		if catItem := bm.catalog.GetItemByID(item.ProdID); catItem != nil {
			base := catItem.ImageBase
			mmSmall, _ := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_furniture_small"))
			if mmSmall != nil {
				if p, ok := mmSmall.PathFor(base + "_small"); ok {
					if im, err := lingoassets.LoadImage(p); err == nil {
						img = im
						bm.smallAssets[item.ProdID] = im
					}
				}
			}
		}
	}
	if img == nil {
		return
	}
	bm.holdingActive = true
	bm.holdingProdID = item.ProdID
	bm.holdingPosID = item.PosID
	bm.holdingImage = img
	// Prepare full ghost preview: load the default facing frame for the 'a' order piece
	if catItem := bm.catalog.GetItemByID(item.ProdID); catItem != nil {
		base := catItem.ImageBase
		bm.holdingImageBase = base
		bm.holdingWidth = catItem.Width
		bm.holdingDepth = catItem.Depth
		// Try to find a common default frame e.g. base + "_a_0_1_1_2_0"
		// Rely on member map in cc_furniture[1]
		mm, _ := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_furniture[1]"))
		candidates := []string{
			base + "_a_0_1_1_2_0",
			base + "_a_0_1_1_0_0",
		}
		for _, name := range candidates {
			if p, ok := mm.PathFor(name); ok {
				if im, err := lingoassets.LoadImage(p); err == nil {
					bm.holdingFullImage = im
					break
				}
			}
		}
	}
}

// ClearHolding resets the holding state
func (bm *BackpackManager) ClearHolding() {
	bm.holdingActive = false
	bm.holdingImage = nil
	bm.holdingFullImage = nil
	bm.holdingProdID = ""
	bm.holdingPosID = ""
	bm.holdingImageBase = ""
	bm.holdingWidth = 0
	bm.holdingDepth = 0
	bm.holdingDir = 0
	bm.holdingDirSet = false
	bm.movingFromPlaced = false
	bm.movedOriginal = PlacedFurni{}
}

// StartHoldingFromPlaced begins holding state from an already placed item, preserving rotation.
func (bm *BackpackManager) StartHoldingFromPlaced(pf PlacedFurni) {
	if bm.catalog == nil {
		return
	}
	// Resolve small preview image for cursor ghost
	mmSmall, _ := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_furniture_small"))
	var img *ebiten.Image
	if mmSmall != nil {
		if p, ok := mmSmall.PathFor(pf.Base + "_small"); ok {
			if im, err := lingoassets.LoadImage(p); err == nil {
				img = im
			}
		}
	}
	bm.holdingActive = true
	bm.holdingProdID = pf.ProdID
	bm.holdingPosID = pf.PosID
	bm.holdingImage = img
	bm.holdingImageBase = pf.Base
	bm.holdingWidth = pf.Width
	bm.holdingDepth = pf.Depth
	bm.holdingDir = pf.Dir
	bm.holdingDirSet = true
	// Mark move session and save original for potential revert
	bm.movingFromPlaced = true
	bm.movedOriginal = pf
	// No need to set holdingFullImage; full preview is rendered via placement using base and dir
}

// DrawItems draws item thumbnails into backpack slots
func (bm *BackpackManager) DrawItems(screen *ebiten.Image) {
	if bm.windowManager == nil || bm.catalog == nil {
		return
	}

	w := bm.windowManager.GetWindow("cc_backpack")
	if w == nil {
		return
	}

	// Ensure slots are cached
	if len(bm.slots) == 0 && w.Definition != nil {
		for _, e := range w.Definition.Elements {
			if e.ID == "cc.backpack.item" {
				bm.slots = append(bm.slots, [2]int{e.LocH, e.LocV})
			}
		}
	}

	winLeft := int(w.StageOffsetX) + w.Rect[0]
	winTop := int(w.StageOffsetY) + w.Rect[1]

	// Paging calculations
	if bm.currentPage <= 0 {
		bm.currentPage = 1
	}
	pageCount := 1
	if bm.itemsPerPage > 0 {
		pageCount = (len(bm.items) + bm.itemsPerPage - 1) / bm.itemsPerPage
		if pageCount <= 0 {
			pageCount = 1
		}
	}
	if bm.currentPage > pageCount {
		bm.currentPage = pageCount
	}
	start := (bm.currentPage - 1) * bm.itemsPerPage
	end := start + bm.itemsPerPage
	if end > len(bm.items) {
		end = len(bm.items)
	}
	visible := end - start
	if visible > len(bm.slots) {
		visible = len(bm.slots)
	}

	for i := 0; i < visible; i++ {
		bi := bm.items[start+i]
		img := bm.smallAssets[bi.ProdID]
		if img == nil {
			if catItem := bm.catalog.GetItemByID(bi.ProdID); catItem != nil {
				base := catItem.ImageBase
				if mmSmall, err := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_furniture_small")); err == nil {
					if p, ok := mmSmall.PathFor(base + "_small"); ok {
						if im, err2 := lingoassets.LoadImage(p); err2 == nil {
							img = im
							bm.smallAssets[bi.ProdID] = im
						}
					}
				}
			}
		}
		if img == nil {
			continue
		}
		ax, ay := bm.slots[i][0], bm.slots[i][1]
		b := img.Bounds()
		op := &ebiten.DrawImageOptions{}
		// center the small thumbnail at the slot anchor
		op.GeoM.Translate(float64(winLeft+ax-b.Dx()/2), float64(winTop+ay-b.Dy()/2))
		screen.DrawImage(img, op)

		// Optional: draw PosID label over the thumbnail when enabled
		if bm.showPosLabels && bm.fontManager != nil {
			face := bm.fontManager.GetFont("Verdana", "bold", 11)
			if face != nil {
				label := strings.TrimSpace(bi.PosID)
				if label != "" {
					bounds := text.BoundString(face, label)
					tx := float64(winLeft+ax) - float64(bounds.Dx())/2
					ty := float64(winTop + ay - b.Dy()/2 - 2)
					text.Draw(screen, label, face, int(tx), int(ty), color.RGBA{0, 255, 0, 255})
				}
			}
		}
	}

	// Draw page indicator between arrows
	if bm.fontManager != nil {
		face := bm.fontManager.GetFont("Verdana", "bold", 11)
		if face != nil {
			label := fmt.Sprintf("Page %d of %d", bm.currentPage, pageCount)
			bounds := text.BoundString(face, label)
			// Determine arrow positions from definition and cache
			if (bm.arrowLeftRect == [4]int{}) || (bm.arrowRightRect == [4]int{}) {
				if w.Definition != nil {
					for _, e := range w.Definition.Elements {
						if e.ID == "cc.pack.leftarrow" {
							bm.arrowLeftRect = [4]int{e.LocH, e.LocV, e.Width, e.Height}
						}
						if e.ID == "cc.pack.rightarrow" {
							bm.arrowRightRect = [4]int{e.LocH, e.LocV, e.Width, e.Height}
						}
					}
				}
			}
			lx := bm.arrowLeftRect[0]
			rx := bm.arrowRightRect[0]
			y := bm.arrowLeftRect[1]
			if rx <= lx {
				// Fallback center if arrows not found
				lx = 200
				rx = 300
				y = 120
			}
			cx := (lx + rx) / 2
			tx := float64(winLeft+cx) - float64(bounds.Dx())/2 + 5
			ty := float64(winTop+y) + 10
			text.Draw(screen, label, face, int(tx), int(ty), color.RGBA{255, 255, 255, 255})
		}
	}
}

// SetShowPosLabels toggles drawing of PosID labels on backpack items
func (bm *BackpackManager) SetShowPosLabels(show bool) {
	bm.showPosLabels = show
}

// DrawFrontPanel draws the backpack front panel overlay
func (bm *BackpackManager) DrawFrontPanel(screen *ebiten.Image) {
	if bm.windowManager == nil {
		return
	}

	w := bm.windowManager.GetWindow("cc_backpack")
	if w == nil || w.Assets == nil {
		return
	}

	frontImg, ok := w.Assets["cc.backpack.front"]
	if !ok || frontImg == nil {
		return
	}

	// Only when the front has not been dropped
	dropped := false
	if w.TempVars != nil {
		if v, ok := w.TempVars["backpackFrontDropped"]; ok {
			if vb, ok2 := v.(bool); ok2 {
				dropped = vb
			}
		}
	}
	if !dropped && w.Definition != nil {
		// Find the element position for cc.backpack.front
		locH, locV := 0, 0
		for _, e := range w.Definition.Elements {
			if e.Member == "cc.backpack.front" {
				locH, locV = e.LocH, e.LocV
				break
			}
		}
		// Apply temporary upward offset during popup if present
		offsetV := 0
		if w.TempVars != nil {
			if v, ok := w.TempVars["backpackFrontOffsetV"]; ok {
				if vi, ok2 := v.(int); ok2 {
					offsetV = vi
				}
			}
		}
		winLeft := int(w.StageOffsetX) + w.Rect[0]
		winTop := int(w.StageOffsetY) + w.Rect[1]
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(winLeft+locH), float64(winTop+locV-offsetV))
		screen.DrawImage(frontImg, op)
	}

	// Always draw hover name centered over the front panel's bottom area (even when dropped)
	if w.Definition != nil && strings.TrimSpace(bm.hoverName) != "" && bm.fontManager != nil {
		// Resolve the front panel anchor again (works whether dropped or not)
		locH2, locV2 := 0, 0
		for _, e := range w.Definition.Elements {
			if e.Member == "cc.backpack.front" {
				locH2, locV2 = e.LocH, e.LocV
				break
			}
		}
		face := bm.fontManager.GetFont("Verdana", "bold", 11)
		if face != nil {
			b := frontImg.Bounds()
			textW := text.BoundString(face, bm.hoverName).Dx()
			winLeft2 := int(w.StageOffsetX) + w.Rect[0]
			winTop2 := int(w.StageOffsetY) + w.Rect[1]
			tx := float64(winLeft2+locH2) + float64(b.Dx()-textW)/2
			// Place slightly above the bottom of the front panel area and raise by 100px
			ty := float64(winTop2+locV2) + float64(b.Dy()) - 118
			text.Draw(screen, bm.hoverName, face, int(tx), int(ty), color.RGBA{255, 255, 255, 255})
		}
	}

	// Draw blank CDs count text to the right of the CD icon
	if bm.fontManager != nil && bm.cdTextRect != [4]int{} {
		face := bm.fontManager.GetFont("Verdana", "bold", 10)
		if face != nil {
			label := fmt.Sprintf("%d Blank CDs", bm.blankCDs)
			winLeft := int(w.StageOffsetX) + w.Rect[0]
			winTop := int(w.StageOffsetY) + w.Rect[1]
			tx := float64(winLeft + bm.cdTextRect[0])
			ty := float64(winTop + bm.cdTextRect[1] + bm.cdTextRect[3] - 2)
			text.Draw(screen, label, face, int(tx), int(ty), color.RGBA{0, 0, 0, 255})
		}
	}
}

// DrawHoldingPreview draws the small cursor ghost when holding an item
func (bm *BackpackManager) DrawHoldingPreview(screen *ebiten.Image, ghostDrawn bool) {
	// Draw small cursor ghost if no full ghost was drawn
	if bm.holdingActive && !ghostDrawn && bm.holdingImage != nil {
		mx, my := ebiten.CursorPosition()
		op := &ebiten.DrawImageOptions{}
		b := bm.holdingImage.Bounds()
		op.GeoM.Translate(float64(mx-b.Dx()/2), float64(my-b.Dy()/2))
		screen.DrawImage(bm.holdingImage, op)
	}
}

// IsHoldingActive returns whether an item is currently being held
func (bm *BackpackManager) IsHoldingActive() bool {
	return bm.holdingActive
}

// GetHoldingInfo returns information about the currently held item
func (bm *BackpackManager) GetHoldingInfo() (prodID, posID, imageBase string, fullImage *ebiten.Image, width, depth int) {
	return bm.holdingProdID, bm.holdingPosID, bm.holdingImageBase, bm.holdingFullImage, bm.holdingWidth, bm.holdingDepth
}

// IsAnimating returns whether the backpack window is currently animating
func (bm *BackpackManager) IsAnimating() bool {
	return bm.animating
}

// RemoveItemByPosID removes the first item with the given PosID
func (bm *BackpackManager) RemoveItemByPosID(posID string) bool {
	for i, it := range bm.items {
		if strings.TrimSpace(it.PosID) == strings.TrimSpace(posID) {
			// Remove index i
			bm.items = append(bm.items[:i], bm.items[i+1:]...)
			return true
		}
	}
	return false
}

// RemoveFirstItemByProdID removes the first item matching the given ProdID
func (bm *BackpackManager) RemoveFirstItemByProdID(prodID string) bool {
	for i, it := range bm.items {
		if strings.TrimSpace(it.ProdID) == strings.TrimSpace(prodID) {
			bm.items = append(bm.items[:i], bm.items[i+1:]...)
			return true
		}
	}
	return false
}

// InsertItemAtFront inserts a new item at the first slot, shifting others
func (bm *BackpackManager) InsertItemAtFront(item BackpackItem) {
	// If the item already exists, remove existing occurrence to avoid duplication
	for i, it := range bm.items {
		if strings.TrimSpace(it.PosID) == strings.TrimSpace(item.PosID) {
			bm.items = append(bm.items[:i], bm.items[i+1:]...)
			break
		}
	}
	bm.items = append([]BackpackItem{item}, bm.items...)
}

// ToggleOpen opens backpack if not already open
func (bm *BackpackManager) ToggleOpen() {
	if bm.windowManager == nil {
		return
	}
	if bm.windowManager.GetWindow("cc_backpack") == nil {
		bm.openWindow()
	}
}

// AddItemAtFront generates and inserts a new item at the first slot
func (bm *BackpackManager) AddItemAtFront(prodID string) BackpackItem {
	// Generate unique PosID within backpack
	makePos := func(base string) string {
		if base == "" {
			base = "pos"
		}
		candidate := base
		counter := 1
		exists := func(id string) bool {
			for _, it := range bm.items {
				if strings.TrimSpace(it.PosID) == strings.TrimSpace(id) {
					return true
				}
			}
			return false
		}
		for exists(candidate) {
			counter++
			candidate = fmt.Sprintf("%s-%d", base, counter)
		}
		return candidate
	}
	posID := makePos("pos")
	item := BackpackItem{ProdID: prodID, PosID: posID, Attributes: map[string]interface{}{}}
	// Preload small image if available
	if bm.catalog != nil && bm.smallAssets != nil {
		if bm.smallAssets[prodID] == nil {
			if catItem := bm.catalog.GetItemByID(prodID); catItem != nil {
				base := catItem.ImageBase
				mmSmall, _ := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_furniture_small"))
				if mmSmall != nil {
					if p, ok := mmSmall.PathFor(base + "_small"); ok {
						if im, err := lingoassets.LoadImage(p); err == nil {
							bm.smallAssets[prodID] = im
						}
					}
				}
			}
		}
	}
	// Insert at front
	bm.items = append([]BackpackItem{item}, bm.items...)
	return item
}

// AddItem appends a new backpack item by prodID, generating a unique PosID and preloading a small image
func (bm *BackpackManager) AddItem(prodID string) BackpackItem {
	if bm.catalog == nil {
		// Append with a generated PosID anyway
		posID := fmt.Sprintf("pos-%d", len(bm.items)+1)
		item := BackpackItem{ProdID: prodID, PosID: posID, Attributes: map[string]interface{}{}}
		bm.items = append(bm.items, item)
		return item
	}
	// Ensure unique PosID among current items
	makePos := func(base string) string {
		if base == "" {
			base = "pos"
		}
		candidate := base
		counter := 1
		exists := func(id string) bool {
			for _, it := range bm.items {
				if strings.TrimSpace(it.PosID) == strings.TrimSpace(id) {
					return true
				}
			}
			return false
		}
		for exists(candidate) {
			counter++
			candidate = fmt.Sprintf("%s-%d", base, counter)
		}
		return candidate
	}
	posID := makePos("pos")
	item := BackpackItem{ProdID: prodID, PosID: posID, Attributes: map[string]interface{}{}}
	bm.items = append(bm.items, item)
	// Preload small image for this prodID if possible
	if bm.smallAssets != nil {
		if bm.smallAssets[prodID] == nil {
			if catItem := bm.catalog.GetItemByID(prodID); catItem != nil {
				base := catItem.ImageBase
				mmSmall, _ := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_furniture_small"))
				if mmSmall != nil {
					if p, ok := mmSmall.PathFor(base + "_small"); ok {
						if im, err := lingoassets.LoadImage(p); err == nil {
							bm.smallAssets[prodID] = im
						}
					}
				}
			}
		}
	}
	return item
}

// itemDisplayName returns a human-readable name for a backpack item from the catalog
func (bm *BackpackManager) itemDisplayName(it BackpackItem) string {
	if bm == nil || bm.catalog == nil {
		return ""
	}
	// Burned CD (prodID 89): prefer ownerName + ": " + songName if available
	if strings.TrimSpace(it.ProdID) == "89" && it.Attributes != nil {
		owner := ""
		song := ""
		if v, ok := it.Attributes["ownerName"]; ok {
			if s, ok2 := v.(string); ok2 {
				owner = strings.TrimSpace(s)
			}
		}
		if v, ok := it.Attributes["songName"]; ok {
			if s, ok2 := v.(string); ok2 {
				song = strings.TrimSpace(s)
			}
		}
		if song != "" {
			if owner != "" {
				return owner + ": " + song
			}
			return song
		}
	}
	if ci := bm.catalog.GetItemByID(it.ProdID); ci != nil {
		name := strings.TrimSpace(ci.Name)
		if name != "" {
			return name
		}
	}
	return ""
}
