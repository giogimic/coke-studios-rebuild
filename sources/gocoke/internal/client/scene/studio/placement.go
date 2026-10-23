package studio

import (
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/studioroom"
	"gocoke/internal/client/core"
	"gocoke/internal/client/fs"
	"gocoke/internal/client/isoengine"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/colorm"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/colornames"
)

// PlacementHelper encapsulates furniture preview, snapping, validation, and placement logic
type PlacementHelper struct {
	scene        *Scene
	mm           *lingoassets.MemberMap     // furniture member map (cc_furniture[1])
	wallItemsMM  *lingoassets.MemberMap     // wall items member map (cc_items[1])
	reg          *studioroom.MemberRegistry // registration points for furniture members
	wallItemsReg *studioroom.MemberRegistry // registration points for wall items
	zshiftCache  map[string]int             // cache for z-shift values to avoid repeated file reads
}

// Global correction applied in addition to Members.csv registration points
const (
	ghostOffsetX = 36 // left
	ghostOffsetY = 18 // up
	// Rug-specific offsets for better positioning
	rugOffsetX = 36 // left (same as ghost by default, can be adjusted)
	rugOffsetY = 54 // up (same as ghost by default, can be adjusted)
	// 2x2 furniture-specific offsets for better positioning
	table2x2OffsetX = -72 // left (adjust as needed for 2x2 tables)
	table2x2OffsetY = 36  // up (adjust as needed for 2x2 tables)
)

// inferDimensionsFromMembers attempts to detect (width, depth) by probing known size pairs
// for the base name using the furniture member map. It returns (w, d, true) on success.
func (ph *PlacementHelper) inferDimensionsFromMembers(base string) (int, int, bool) {
	if ph.mm == nil {
		return 0, 0, false
	}
	// Common furniture sizes to probe (w,d). Prioritize 2x1/1x2 sofas, include 3x3 for rugs.
	candidates := [][2]int{
		{2, 1}, {1, 2},
		{2, 2}, {3, 1}, {1, 3}, {3, 2}, {2, 3}, {3, 3},
	}
	// Try multiple directions typical for assets
	dirs := []int{2, 6, 0, 4}
	for _, sz := range candidates {
		for _, dir := range dirs {
			name := base + "_a_0_" + strconv.Itoa(sz[0]) + "_" + strconv.Itoa(sz[1]) + "_" + strconv.Itoa(dir) + "_0"
			if _, ok := ph.mm.PathFor(name); ok {
				return sz[0], sz[1], true
			}
		}
	}
	return 0, 0, false
}

func NewPlacementHelper(scene *Scene) *PlacementHelper {
	// Load member map for furniture assets once
	mm, err := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_furniture[1]"))
	if err != nil {
		// Continue without member map
	}
	// Load member map for wall items assets
	wallItemsMM, err := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_items[1]"))
	if err != nil {
		// Continue without wall items member map
	}
	// Load registration points from furniture Members.csv
	reg, err := studioroom.LoadMemberRegistry(filepath.Join("Lingo", "cc_furniture[1]", "Members.csv"))
	if err != nil {
		// Continue without registry
	}
	// Load registration points from wall items Members.csv (if exists)
	wallItemsReg, err := studioroom.LoadMemberRegistry(filepath.Join("Lingo", "cc_items[1]", "Members.csv"))
	if err != nil {
		// Continue without wall items registry
	}
	return &PlacementHelper{
		scene:        scene,
		mm:           mm,
		wallItemsMM:  wallItemsMM,
		reg:          reg,
		wallItemsReg: wallItemsReg,
		zshiftCache:  make(map[string]int),
	}
}

// computeNumFrames scans available members for this furniture base to compute highest frame index.
func (ph *PlacementHelper) computeNumFrames(pf *PlacedFurni) {
	if pf == nil || ph.mm == nil || pf.IsWallItem {
		return
	}
	if pf.AnimFrames == nil {
		pf.AnimFrames = make(map[string]int)
	}
	highest := 0
	aw := max1(pf.Width)
	ad := max1(pf.Depth)
	if aw < ad {
		aw, ad = ad, aw
	}
	w := strconv.Itoa(aw)
	d := strconv.Itoa(ad)
	altDir := mirrorEven(pf.Dir)
	// Try a few reasonable frame counts (0..20) to detect presence across current or mirrored dir
	for frame := 0; frame <= 20; frame++ {
		for _, o := range []string{"a", "b", "c", "d"} {
			candCur := fmt.Sprintf("%s_%s_%d_%s_%s_%d_%d", pf.Base, o, 0, w, d, pf.Dir, frame)
			candAlt := fmt.Sprintf("%s_%s_%d_%s_%s_%d_%d", pf.Base, o, 0, w, d, altDir, frame)
			if _, ok := ph.mm.PathFor(candCur); ok {
				if frame > highest {
					highest = frame
				}
				if frame > pf.AnimFrames[o] {
					pf.AnimFrames[o] = frame
				}
				continue
			}
			if _, ok := ph.mm.PathFor(candAlt); ok {
				if frame > highest {
					highest = frame
				}
				if frame > pf.AnimFrames[o] {
					pf.AnimFrames[o] = frame
				}
			}
		}
	}
	pf.NumFrames = highest
	// Compute contiguous max from frame 1 per slice to constrain active loops (check current dir first, then alt)
	if pf.ContigMax == nil {
		pf.ContigMax = make(map[string]int)
	}
	for _, o := range []string{"a", "b", "c", "d"} {
		contig := 0
		for f := 1; f <= pf.AnimFrames[o]; f++ {
			nameCur := fmt.Sprintf("%s_%s_%d_%s_%s_%d_%d", pf.Base, o, 0, w, d, pf.Dir, f)
			if _, ok := ph.mm.PathFor(nameCur); ok {
				contig = f
				continue
			}
			nameAlt := fmt.Sprintf("%s_%s_%d_%s_%s_%d_%d", pf.Base, o, 0, w, d, altDir, f)
			if _, ok := ph.mm.PathFor(nameAlt); ok {
				contig = f
			} else {
				break
			}
		}
		pf.ContigMax[o] = contig
	}
	// Choose driver slice based on largest contiguous range
	maxSlice := ""
	maxContig := 0
	for _, o := range []string{"a", "b", "c", "d"} {
		if pf.ContigMax[o] > maxContig {
			maxContig = pf.ContigMax[o]
			maxSlice = o
		}
	}
	pf.DriverSlice = maxSlice
}

// nameWithFrame injects the frame index (before the final frame component if not present).
func (ph *PlacementHelper) nameWithFrame(base string, frame int) string {
	// Expect pattern base_order_0_w_d_dir_0
	parts := strings.Split(base, "_")
	if len(parts) >= 7 {
		// Replace last component with frame index
		parts[len(parts)-1] = strconv.Itoa(frame)
		return strings.Join(parts, "_")
	}
	return base
}

// nameWithDir replaces the direction component in a member name with the given dir (even dir 0/2/4/6).
func (ph *PlacementHelper) nameWithDir(base string, dir int) string {
	parts := strings.Split(base, "_")
	if len(parts) >= 7 {
		// Direction is the penultimate component
		parts[len(parts)-2] = strconv.Itoa(dir)
		return strings.Join(parts, "_")
	}
	return base
}

func mirrorEven(d int) int {
	switch d {
	case 2:
		return 4
	case 4:
		return 2
	case 0:
		return 6
	case 6:
		return 0
	default:
		return d
	}
}

// resolveAnimatedOrStatic attempts to resolve an animated frame member for the given slice.
// It never falls back to frame 0 during active playback; if no >0 frame is found, returns the static member.
func (ph *PlacementHelper) resolveAnimatedOrStatic(memberMap *lingoassets.MemberMap, origName, order string, pf *PlacedFurni) (string, string, bool) {
	if memberMap == nil || pf == nil {
		return origName, "", false
	}
	if pf.State == 1 && pf.AnimFrames != nil && pf.AnimFrames[order] > 0 {
		// Use exact frame clamped to this slice's contiguous max
		target := pf.Frame
		if target < 1 {
			target = 1
		}
		maxf := pf.AnimFrames[order]
		if pf.ContigMax != nil && pf.ContigMax[order] > 0 {
			maxf = pf.ContigMax[order]
		}
		if target > maxf {
			target = maxf
		}
		candidate := ph.nameWithFrame(origName, target)
		if p, ok := memberMap.PathFor(candidate); ok {
			return candidate, p, true
		}
		// Try mirrored-even direction frame and rely on flip logic later
		alt := ph.nameWithDir(candidate, mirrorEven(pf.Dir))
		if p, ok := memberMap.PathFor(alt); ok {
			return alt, p, true
		}
	}
	// Fallback to static
	if p, ok := memberMap.PathFor(origName); ok {
		return origName, p, true
	}
	return origName, "", false
}

// tickFurnitureAnimations advances frames based on FPS for active items.
func (ph *PlacementHelper) tickFurnitureAnimations() {
	if ph == nil || ph.scene == nil {
		return
	}
	now := time.Now().UnixMilli()
	for i := range ph.scene.placedFurni {
		pf := &ph.scene.placedFurni[i]
		if pf.IsWallItem {
			continue
		}
		// Inactive: force frame 0 and skip ticking
		if pf.State == 0 {
			// Allow a brief one-shot frame (e.g., power-off flash) if scheduled
			if pf.OneShotFrame > 0 && now <= pf.OneShotUntil {
				pf.Frame = pf.OneShotFrame
			} else {
				pf.OneShotFrame = 0
				pf.Frame = 0
			}
			pf.LastFrameTS = now
			continue
		}
		if !pf.Animate || pf.FPS <= 0 {
			continue
		}
		// Determine loop upper bound from driver slice's contiguous max; fallback to NumFrames
		loopMax := pf.NumFrames
		if pf.ContigMax != nil && pf.DriverSlice != "" && pf.ContigMax[pf.DriverSlice] > 0 {
			loopMax = pf.ContigMax[pf.DriverSlice]
		}
		if loopMax <= 0 {
			continue
		}
		interval := int64(1000 / pf.FPS)
		if now-pf.LastFrameTS >= interval {
			if pf.Frame < 1 || pf.Frame > loopMax {
				pf.Frame = 1
			} else {
				pf.Frame++
				if pf.Frame > loopMax {
					pf.Frame = 1
				}
			}
			pf.LastFrameTS = now
		}
	}
}

// makeUniquePosID returns a PosID that is unique among currently placed items.
// If the provided base is already in use, it appends a numeric suffix.
func (ph *PlacementHelper) makeUniquePosID(base string) string {
	if ph == nil || ph.scene == nil {
		return strings.TrimSpace(base)
	}
	base = strings.TrimSpace(base)
	if base == "" {
		base = "pos"
	}
	candidate := base
	suffix := 1
	for {
		unique := true
		for _, pf := range ph.scene.placedFurni {
			if strings.TrimSpace(pf.PosID) == candidate {
				unique = false
				break
			}
		}
		if unique {
			return candidate
		}
		suffix++
		candidate = fmt.Sprintf("%s#%d", base, suffix)
	}
}

// PlaceFromServer is defined in pos_furni.go

// isWallItem checks if the given item is a wall item (poster) based on catalog type
func (ph *PlacementHelper) isWallItem(prodID string) bool {
	if ph.scene == nil || ph.scene.catalog == nil {
		return false
	}
	if ci := ph.scene.catalog.GetItemByID(prodID); ci != nil {
		return strings.EqualFold(ci.Type, "poster")
	}
	return false
}

// isWallSquare checks if a square is adjacent to a wall and returns the wall direction
// Returns wall direction (0=north, 2=east, 4=south, 6=west) and true if wall found
func (ph *PlacementHelper) isWallSquare(row, col int) (int, bool) {
	if ph.scene == nil || ph.scene.isoScene == nil {
		return 0, false
	}

	// Check adjacent squares for walls
	// In isometric view, walls typically block movement
	// Check all 4 cardinal directions for non-walkable squares (walls)
	directions := []struct {
		dr, dc int
		dir    int
	}{
		{-1, 0, 0}, // north
		{0, 1, 2},  // east
		{1, 0, 4},  // south
		{0, -1, 6}, // west
	}

	for _, d := range directions {
		wallRow := row + d.dr
		wallCol := col + d.dc

		if wallSq := ph.scene.isoScene.GetSquareByRowCol(wallRow, wallCol); wallSq != nil {
			if !wallSq.IsWalkable() {
				return d.dir, true
			}
		}
	}

	return 0, false
}

// resolveWallItemAssetName converts a wall item base name to the actual asset name in cc_items[1]
// Wall items have specific naming patterns like "32_leftwall poster acoustic_foam.png"
func (ph *PlacementHelper) resolveWallItemAssetName(base string, wallDir int) (string, bool) {
	if ph.wallItemsMM == nil {
		return "", false
	}

	// Wall items typically have leftwall/rightwall variants
	// wallDir 6 (west) = leftwall, wallDir 2 (east) = rightwall
	wallSide := "leftwall"
	if wallDir == 2 {
		wallSide = "rightwall"
	}

	// The catalog imageBase includes "poster " prefix, so we need to handle this correctly
	// If base already starts with "poster ", use it as-is, otherwise add the prefix
	var cleanBase string
	if strings.HasPrefix(base, "poster ") {
		cleanBase = strings.TrimPrefix(base, "poster ")
	} else {
		cleanBase = base
	}

	// Try different naming patterns found in the assets
	patterns := []string{
		wallSide + " poster " + cleanBase, // e.g., "leftwall poster acoustic_foam"
		wallSide + " " + base,             // e.g., "leftwall poster acoustic_foam" (if base includes poster)
		wallSide + "_poster_" + cleanBase, // alternative naming
	}

	// Search through all members to find matching wall item
	for _, pattern := range patterns {
		if _, ok := ph.wallItemsMM.PathFor(pattern); ok {
			return pattern, true
		}
	}

	return "", false
}

// getWallUnderMouse determines if mouse is over any wall area and returns wall direction
// Returns wall direction ("left" or "right") and true if over a wall, "" and false otherwise
func (ph *PlacementHelper) getWallUnderMouse(mx, my int) (string, bool) {
	if ph.scene == nil || ph.scene.isoScene == nil {
		return "", false
	}

	wallSystem := ph.scene.isoScene.GetWallSystem()
	if wallSystem == nil {
		return "", false
	}

	wallTiles := wallSystem.GetWallTiles()
	for _, tile := range wallTiles {
		if tile == nil || !tile.Visible || tile.Image == nil {
			continue
		}

		bounds := tile.Image.Bounds()
		tileLeft := tile.Position.X
		tileTop := tile.Position.Y
		tileRight := tileLeft + float64(bounds.Dx())
		tileBottom := tileTop + float64(bounds.Dy())

		if float64(mx) >= tileLeft && float64(mx) <= tileRight &&
			float64(my) >= tileTop && float64(my) <= tileBottom {
			return tile.Direction, true
		}
	}

	return "", false
}

// validateWallItemPlacement checks if a wall item can be placed at the given position
func (ph *PlacementHelper) validateWallItemPlacement(wallDirection string) bool {
	// Allow free-form placement anywhere on any wall
	return wallDirection != ""
}

// renderWallItemPreview renders a wall item preview on the wall under the mouse
func (ph *PlacementHelper) renderWallItemPreview(screen *ebiten.Image, heldBase string, mx, my int) bool {
	// Check if mouse is over any wall
	wallDirection, isOverWall := ph.getWallUnderMouse(mx, my)
	if !isOverWall {
		return false
	}

	// Validate placement (just checks if we're over a wall)
	if !ph.validateWallItemPlacement(wallDirection) {
		return false
	}

	// Resolve wall item asset name based on wall direction
	assetName, found := ph.resolveWallItemAssetName(heldBase, getWallDirectionCode(wallDirection))
	if !found {
		return false
	}

	if ph.wallItemsMM == nil {
		return false
	}

	assetPath, ok := ph.wallItemsMM.PathFor(assetName)
	if !ok {
		return false
	}

	img, err := lingoassets.LoadImage(assetPath)
	if err != nil {
		return false
	}

	// Position the wall item at exact mouse position
	op := &ebiten.DrawImageOptions{}

	// Apply registration point offset if available
	var offX, offY int
	if ph.wallItemsReg != nil {
		if rp, ok := ph.wallItemsReg.GetRegistrationPoint(assetName); ok {
			offX, offY = rp[0], rp[1]
		}
	}

	// Position at exact mouse location minus registration point offset (no snapping)
	drawX := float64(mx) - float64(offX)
	drawY := float64(my) - float64(offY)

	op.GeoM.Translate(drawX, drawY)

	// Draw with slight transparency to indicate it's a preview
	op.ColorM.Scale(1, 1, 1, 0.7)

	screen.DrawImage(img, op)

	return true
}

// getWallDirectionCode converts wall direction string to numeric code for asset resolution
func getWallDirectionCode(direction string) int {
	switch direction {
	case "left":
		return 6
	case "right":
		return 2
	default:
		return 2
	}
}

// GetWallItemUnderMouse finds the topmost wall item under the mouse cursor
// Returns the wall item and true if found, nil and false otherwise
func (ph *PlacementHelper) GetWallItemUnderMouse(mx, my int) (*PlacedFurni, bool) {
	if ph.scene == nil {
		return nil, false
	}

	var topWallItem *PlacedFurni

	// Check all wall items to see if mouse is over them
	for idx := range ph.scene.placedFurni {
		pf := &ph.scene.placedFurni[idx]
		if !pf.IsWallItem {
			continue
		}

		// Get the wall item's asset to determine its bounds
		assetName, found := ph.resolveWallItemAssetName(pf.Base, getWallDirectionCode(pf.WallDirection))
		if !found || ph.wallItemsMM == nil {
			continue
		}

		assetPath, ok := ph.wallItemsMM.PathFor(assetName)
		if !ok {
			continue
		}

		img, err := lingoassets.LoadImage(assetPath)
		if err != nil {
			continue
		}

		// Calculate the wall item's screen bounds
		var offX, offY int
		if ph.wallItemsReg != nil {
			if rp, ok := ph.wallItemsReg.GetRegistrationPoint(assetName); ok {
				offX, offY = rp[0], rp[1]
			}
		}

		bounds := img.Bounds()
		itemLeft := pf.WallX - float64(offX)
		itemTop := pf.WallY - float64(offY)
		itemRight := itemLeft + float64(bounds.Dx())
		itemBottom := itemTop + float64(bounds.Dy())

		// Check if mouse is within this wall item's bounds
		if float64(mx) >= itemLeft && float64(mx) <= itemRight &&
			float64(my) >= itemTop && float64(my) <= itemBottom {
			// This wall item is under the mouse - check if it's the topmost
			// For wall items, we can use Height as a simple z-order (higher = on top)
			if topWallItem == nil || pf.Height > topWallItem.Height {
				topWallItem = pf
			}
		}
	}

	return topWallItem, topWallItem != nil
}

// RenderFullPreview draws the full furniture preview while holding.
// Returns true if snapped to a grid tile (so caller can hide the small ghost icon).
func (ph *PlacementHelper) RenderFullPreview(screen *ebiten.Image, heldBase string, mx, my int) bool {
	if ph.scene == nil || ph.scene.isoScene == nil {
		return false
	}
	// Only draw full preview when not over a UI window
	if ph.scene.hudManager != nil && ph.scene.hudManager.GetWindowManager() != nil && ph.scene.hudManager.GetWindowManager().IsMouseOverAnyWindow() {
		return false
	}

	// Check if this is a wall item
	var holdingProdID string
	if ph.scene.backpackManager != nil {
		holdingProdID, _, _, _, _, _ = ph.scene.backpackManager.GetHoldingInfo()
	}
	isWallItem := ph.isWallItem(holdingProdID)

	// Handle wall items differently
	if isWallItem {
		return ph.renderWallItemPreview(screen, heldBase, mx, my)
	}

	// Regular furniture preview (existing logic)
	iso := ph.scene.isoScene
	sq := iso.GetMouseSquare()
	if sq == nil {
		return false
	}

	// Get dimensions first
	var width, depth int
	if ph.scene.backpackManager != nil {
		_, _, _, _, width, depth = ph.scene.backpackManager.GetHoldingInfo()
	}
	if (width <= 0 || depth <= 0) && heldBase != "" {
		if w2, d2, ok := ph.inferDimensionsFromMembers(heldBase); ok {
			width, depth = w2, d2

		}
	}
	if width <= 0 {
		width = 1
	}
	if depth <= 0 {
		depth = 1
	}

	// Determine direction for preview based on asset availability
	dir := 2 // Default direction for most furniture
	if strings.Contains(strings.ToLower(heldBase), "rug") || strings.Contains(strings.ToLower(heldBase), "hardwood") {
		dir = 0 // Rugs default to direction 0
	} else if ph.scene != nil && ph.scene.hasOnlyDir4Assets(heldBase, width, depth) {
		dir = 4 // Furniture with only dir 4 assets defaults to direction 4
	}

	// Only show full preview when current mouse square is a valid placement target
	if !ph.ValidateSnap(sq.GetRow(), sq.GetCol(), max1(width), max1(depth), dir) {
		return false
	}

	// Over a valid tile: compute a base anchor.
	// Rugs: hovered square is the top-right of the rug; shift to top-left for anchor.
	anchorRow := sq.GetRow()
	anchorCol := sq.GetCol()
	if strings.Contains(strings.ToLower(heldBase), "rug") || strings.Contains(strings.ToLower(heldBase), "hardwood") {
		anchorCol = sq.GetCol() - (max1(width) - 1)
		anchorRow = sq.GetRow() + (max1(depth) - 1)
	} else if width == 2 && depth == 2 {
		// Special handling for 2x2 furniture (tables, etc.)
		// For 2x2 items, the hovered square should be the center, so we shift to get the top-left origin
		switch dir {
		case 2, 6:
			// For 2x2, shift to get proper origin
			anchorRow = sq.GetRow()
			anchorCol = sq.GetCol() - 1
		case 0, 4:
			// For 2x2, shift to get proper origin
			anchorRow = sq.GetRow()
			anchorCol = sq.GetCol() - 1
		}
	} else {
		switch dir {
		case 2, 6:
			anchorRow = sq.GetRow() + (depth - 1)
			anchorCol = sq.GetCol() - (width - 1) + 1
		case 0, 4:
			anchorRow = sq.GetRow() + (depth - 1)
			anchorCol = sq.GetCol() + (width - 1)
		}
		// Global correction: shift the ghost preview one column to the left for non-rugs (but not 2x2)
		anchorCol -= 1
	}
	anchorSq := iso.GetSquareByRowCol(anchorRow, anchorCol)
	vp := sq.GetViewPoints().BottomLeft
	if anchorSq != nil {
		vp = anchorSq.GetViewPoints().BottomLeft
	}

	// Helper: attempt to resolve a member with fallbacks (swap w/d, try alt dirs 2,6,0,4)
	resolvePath := func(base, order string, w, d, dir int) (string, bool, string) {
		if ph.mm == nil {
			return "", false, ""
		}
		dirs := []int{dir, 6, 0, 4}
		sizes := [][2]int{{w, d}, {d, w}}
		for _, sz := range sizes {
			for _, dd := range dirs {
				candidate := base + "_" + order + "_0_" + strconv.Itoa(sz[0]) + "_" + strconv.Itoa(sz[1]) + "_" + strconv.Itoa(dd) + "_0"
				if p, ok := ph.mm.PathFor(candidate); ok {
					return p, true, candidate
				}
			}
		}
		return "", false, ""
	}

	// Helper to draw a single member if available, with robust resolution
	drawIf := func(base, order string, w, d, dir int) {
		if ph.mm == nil {
			return
		}
		if p, ok, resolved := resolvePath(base, order, w, d, dir); ok {
			// If the ghost preview is for an animated furni, switch to frame-qualified member name
			if pfGhost := ph.scene; pfGhost != nil {
				// no-op guard: we don't have a ghost PlacedFurni here; keep using base frame 0 for previews
			}
			if im, err := lingoassets.LoadImage(p); err == nil {
				op := &ebiten.DrawImageOptions{}
				// Apply registration point offset so the image's anchor sits on the square's bottom-left
				var offX, offY int
				if ph.reg != nil {
					if rp, ok := ph.reg.GetRegistrationPoint(resolved); ok {
						offX, offY = rp[0], rp[1]
					}
				}
				// Apply global ghost correction (left/up) - use specific offsets for different furniture types
				if strings.Contains(strings.ToLower(heldBase), "rug") || strings.Contains(strings.ToLower(heldBase), "hardwood") {
					op.GeoM.Translate(float64(vp.X)-float64(offX)-rugOffsetX, float64(vp.Y)-float64(offY)-rugOffsetY)
				} else if width == 2 && depth == 2 {
					// 2x2 furniture (tables) use specific offsets
					op.GeoM.Translate(float64(vp.X)-float64(offX)-table2x2OffsetX, float64(vp.Y)-float64(offY)-table2x2OffsetY)
				} else {
					op.GeoM.Translate(float64(vp.X)-float64(offX)-ghostOffsetX, float64(vp.Y)-float64(offY)-ghostOffsetY)
				}
				screen.DrawImage(im, op)
			}
		} else {
			// Missing member for furniture layer
		}
	}

	// Shadow layer with fallbacks for 2x1 sofas (e.g., Coke Couch)
	drawShadow := func() {
		// Try direction-specific shadow first, then base-specific, then generic fallback
		if ph.mm != nil {
			// Map even directions to available shadow directions (0 or 4)
			assetDir := dir
			if assetDir == 2 {
				assetDir = 4
			} else if assetDir == 6 {
				assetDir = 0
			}
			// Build candidate list: directional first, then generic
			shadowCandidates := []string{
				heldBase + "_sd_" + strconv.Itoa(assetDir),
				heldBase + "_sd",
			}

			for _, memberName := range shadowCandidates {
				if p, ok := ph.mm.PathFor(memberName); ok {
					if im, err := lingoassets.LoadImage(p); err == nil {
						var offX, offY int
						if ph.reg != nil {
							if rp, ok := ph.reg.GetRegistrationPoint(memberName); ok {
								offX, offY = rp[0], rp[1]
							}
						}
						// Determine if we need to horizontally flip
						// If using directional shadow (base_sd_0/base_sd_4), flip for mirrored even dirs
						flipH := false
						if strings.HasSuffix(memberName, "_sd_4") && dir == 2 {
							flipH = true
						} else if strings.HasSuffix(memberName, "_sd_0") && dir == 6 {
							flipH = true
						} else if strings.HasSuffix(memberName, "_sd") {
							// Generic shadow: mirror when footprint spans >1 tile and dir is a mirrored orientation (4 or 0)
							if (width > 1 || depth > 1) && (dir == 4 || dir == 0) {
								flipH = true
							}
						}
						// Compute draw position with appropriate offsets
						drawX := float64(vp.X) - float64(offX)
						drawY := float64(vp.Y) - float64(offY)
						if strings.Contains(strings.ToLower(heldBase), "rug") || strings.Contains(strings.ToLower(heldBase), "hardwood") {
							drawX -= float64(rugOffsetX)
							drawY -= float64(rugOffsetY)
						} else if width == 2 && depth == 2 {
							drawX -= float64(table2x2OffsetX)
							drawY -= float64(table2x2OffsetY)
						} else {
							drawX -= float64(ghostOffsetX)
							drawY -= float64(ghostOffsetY)
						}
						// Apply shadow opacity (40% as per Lingo blend setting)
						op := &colorm.DrawImageOptions{}
						if flipH {
							w := im.Bounds().Dx()
							op.GeoM.Scale(-1, 1)
							op.GeoM.Translate(drawX+float64(w), drawY)
						} else {
							op.GeoM.Translate(drawX, drawY)
						}
						var cm colorm.ColorM
						cm.Scale(1, 1, 1, 0.4)
						colorm.DrawImage(screen, im, cm, op)
						return
					}
				}
			}
			// Fallback to generic 2x1 sofa shadow for couches
			if width >= 2 && depth >= 1 {
				if p, ok := ph.mm.PathFor("2x1sofaistic_sd"); ok {
					if im, err := lingoassets.LoadImage(p); err == nil {
						// Registration for generic shadow
						var offX, offY int
						if ph.reg != nil {
							if rp, ok := ph.reg.GetRegistrationPoint("2x1sofaistic_sd"); ok {
								offX, offY = rp[0], rp[1]
							}
						}
						// Apply shadow opacity (40% as per Lingo blend setting)
						op := &colorm.DrawImageOptions{}
						// For large assets rely solely on registration points - use specific offsets for different furniture types
						if strings.Contains(strings.ToLower(heldBase), "rug") || strings.Contains(strings.ToLower(heldBase), "hardwood") {
							op.GeoM.Translate(float64(vp.X)-float64(offX)-float64(rugOffsetX), float64(vp.Y)-float64(offY)-float64(rugOffsetY))
						} else if width >= 2 && depth >= 1 {
							// 2x2 furniture (tables) use specific offsets for fallback shadows too
							op.GeoM.Translate(float64(vp.X)-float64(offX)-float64(table2x2OffsetX), float64(vp.Y)-float64(offY)-float64(table2x2OffsetY))
						} else {
							op.GeoM.Translate(float64(vp.X)-float64(offX)-float64(ghostOffsetX), float64(vp.Y)-float64(offY)-float64(ghostOffsetY))
						}
						var cm colorm.ColorM
						cm.Scale(1, 1, 1, 0.4) // 40% opacity for shadows
						colorm.DrawImage(screen, im, cm, op)
						return
					}
				}
			}
		}
	}
	// Skip shadow for rugs - they don't have shadow layers
	// Draw shadow only when it's NOT a rug or hardwood
	if !(strings.Contains(strings.ToLower(heldBase), "rug") || strings.Contains(strings.ToLower(heldBase), "hardwood")) {
		drawShadow()
	}
	// Ordered layers (width/depth aware)
	// Asset naming uses canonical 2x1 for sofas; use max/min to match asset map
	assetW := width
	assetD := depth
	if assetW < assetD {
		assetW, assetD = assetD, assetW
	}
	orders := []string{"a", "b", "c", "d"}
	for _, o := range orders {
		drawIf(heldBase, o, assetW, assetD, dir)
	}
	return true
}

// ValidateSnap checks whether a furniture of width x depth can be placed at row/col with given direction.
// For regular furniture: all covered squares must be walkable (not blocked).
// For wall items: must be placed on a wall tile.
func (ph *PlacementHelper) ValidateSnap(row, col, width, depth, dir int) bool {
	if ph.scene == nil || ph.scene.isoScene == nil {
		return false
	}

	// Check if this is a wall item
	var holdingProdID string
	if ph.scene.backpackManager != nil {
		holdingProdID, _, _, _, _, _ = ph.scene.backpackManager.GetHoldingInfo()
	}
	isWallItem := ph.isWallItem(holdingProdID)

	// For wall items, use different validation - they must be placed on walls
	if isWallItem {
		// For wall items, we need to get current mouse position since row/col are floor coordinates
		mx, my := ebiten.CursorPosition()
		wallDirection, isOverWall := ph.getWallUnderMouse(mx, my)
		if !isOverWall {
			return false // Wall items must be placed on walls
		}

		// Check if we can place on this wall
		return ph.validateWallItemPlacement(wallDirection)
	}
	// For rugs (dir 0 assets), hovered square is top-right anchor; shift origin left to top-left for coverage
	adjRow, adjCol := row, col
	{
		var holdingImageBase string
		if ph.scene.backpackManager != nil {
			_, _, holdingImageBase, _, _, _ = ph.scene.backpackManager.GetHoldingInfo()
		}
		if strings.Contains(strings.ToLower(holdingImageBase), "rug") || strings.Contains(strings.ToLower(holdingImageBase), "hardwood") {
			adjCol = col - (max1(width) - 1)
		} else if width == 2 && depth == 2 {
			// For 2x2 furniture, treat hovered square as center and shift to top-left origin
			adjCol = col - 1
		}
	}
	// Determine coverage based on direction (match Lingo: 0,4 -> depth along rows; 2,6 -> depth along cols)
	covered := ph.coverageSquares(adjRow, adjCol, width, depth, dir)

	// For each covered square, check if placement is allowed
	for _, sq := range covered {
		if sq == nil {
			return false
		}

		// Find all existing furniture items that cover this square
		var coveringItems []PlacedFurni
		for _, existing := range ph.scene.placedFurni {
			existingCovered := ph.coverageSquares(existing.Row, existing.Col, existing.Width, existing.Depth, existing.Dir)
			for _, esq := range existingCovered {
				if esq != nil && esq.GetRow() == sq.GetRow() && esq.GetCol() == sq.GetCol() {
					coveringItems = append(coveringItems, existing)
					break // This item covers this square, no need to check other squares of this item
				}
			}
		}

		if len(coveringItems) > 0 {
			// Check if this is a rug - rugs can go underneath any furniture
			var holdingImageBase string
			if ph.scene.backpackManager != nil {
				_, _, holdingImageBase, _, _, _ = ph.scene.backpackManager.GetHoldingInfo()
			}
			isRug := strings.Contains(strings.ToLower(holdingImageBase), "rug") || strings.Contains(strings.ToLower(holdingImageBase), "hardwood")

			if isRug {
				// Rugs can be placed underneath any furniture, skip IAOT checks
				continue
			}

			// Find the topmost level and check if any item at that level allows stacking
			topLevel := -1
			for _, item := range coveringItems {
				if item.GridY > topLevel {
					topLevel = item.GridY
				}
			}

			// For 1x1 items, check if any item at the top level allows items on top
			// For multi-tile items, we need to be more careful about which items to check
			allowed := false
			if width == 1 && depth == 1 {
				// For 1x1 items, any top-level item with IAOT=true allows stacking
				for _, item := range coveringItems {
					if item.GridY == topLevel && ph.scene.allowsItemsOnTop(item) {
						allowed = true
						break
					}
				}
			} else {
				// For multi-tile items, we need at least one overlapping item with IAOT=true
				// But we're more permissive - if any item allows stacking, we allow it
				for _, item := range coveringItems {
					if item.GridY == topLevel && ph.scene.allowsItemsOnTop(item) {
						allowed = true
						break
					}
				}
			}

			if !allowed {
				// IAOT denied at this square
				return false
			}
		} else {
			// No existing item here: require walkable base
			if !sq.IsWalkable() {
				// Blocked by non-walkable square with no stack target
				return false
			}
		}
	}

	// Rule 2: Block placing any non-seatable/non-walkable furniture on tiles currently occupied by avatars
	isNonSeatableNew := true
	var holdingImageBase string
	if ph.scene.backpackManager != nil {
		holdingProdID, _, holdingImageBase, _, _, _ = ph.scene.backpackManager.GetHoldingInfo()
	}
	if ph.scene.catalog != nil {
		if ci := ph.scene.catalog.GetItemByID(holdingProdID); ci != nil {
			t := ci.Type
			if t == "Chair" || t == "Sofa" || t == "Stool" || t == "carfront" || strings.EqualFold(t, "rug") || strings.EqualFold(t, "hardwood") {
				isNonSeatableNew = false
			}
		}
	}
	if isNonSeatableNew && holdingImageBase != "" {
		b := strings.ToLower(holdingImageBase)
		if strings.Contains(b, "sofa") || strings.Contains(b, "couch") || strings.Contains(b, "chair") || strings.Contains(b, "stool") || strings.Contains(b, "rug") || strings.Contains(b, "hardwood") {
			isNonSeatableNew = false
		}
	}
	if isNonSeatableNew {
		for _, av := range ph.scene.avatars {
			if av == nil || av.Square == nil {
				continue
			}
			ar := av.Square.GetRow()
			ac := av.Square.GetCol()
			for _, csq := range covered {
				if csq != nil && csq.GetRow() == ar && csq.GetCol() == ac {
					return false
				}
			}
		}
	}

	return true
}

// ValidateRotate checks whether an existing furniture (exclude) can be rotated to
// width x depth at the same origin (row,col) with given direction. It excludes the
// furniture itself from IAOT/overlap checks so rotation-in-place is possible when
// the only occupant is the rotating item.
func (ph *PlacementHelper) ValidateRotate(row, col, width, depth, dir int, exclude *PlacedFurni) bool {
	if ph.scene == nil || ph.scene.isoScene == nil {
		return false
	}
	covered := ph.coverageSquares(row, col, width, depth, dir)

	for _, sq := range covered {
		if sq == nil {
			return false
		}
		// Collect covering items excluding the furniture being rotated
		var coveringItems []PlacedFurni
		for idx := range ph.scene.placedFurni {
			existing := &ph.scene.placedFurni[idx]
			if exclude != nil && existing == exclude {
				continue
			}
			existingCovered := ph.coverageSquares(existing.Row, existing.Col, existing.Width, existing.Depth, existing.Dir)
			for _, esq := range existingCovered {
				if esq != nil && esq.GetRow() == sq.GetRow() && esq.GetCol() == sq.GetCol() {
					coveringItems = append(coveringItems, *existing)
					break
				}
			}
		}

		if len(coveringItems) > 0 {
			// IAOT rules at the topmost level among other items
			topLevel := -1
			for _, item := range coveringItems {
				if item.GridY > topLevel {
					topLevel = item.GridY
				}
			}
			allowed := false
			for _, item := range coveringItems {
				if item.GridY == topLevel && ph.scene.allowsItemsOnTop(item) {
					allowed = true
					break
				}
			}
			if !allowed {
				// Disallowed stacking on another item at this square
				return false
			}
		} else {
			// No other items cover this square. For rotation-in-place, allow even if
			// the base square is currently marked unwalkable due to this same furniture.
			// So we intentionally skip the IsWalkable check here.
		}
	}

	// For rotation of non-seatable items, ensure we are not rotating onto avatars only if
	// other items aren't allowing stacking. Given we excluded self, if any avatar logic is
	// needed it should be added here. For now, match placement parity by allowing.

	return true
}

// squaresOverlap returns true if any row/col pair intersects between the two sets
func squaresOverlap(a, b []*isoengine.IsoSquare) bool {
	for _, sa := range a {
		if sa == nil {
			continue
		}
		ar := sa.GetRow()
		ac := sa.GetCol()
		for _, sb := range b {
			if sb == nil {
				continue
			}
			if sb.GetRow() == ar && sb.GetCol() == ac {
				return true
			}
		}
	}
	return false
}

// coverageSquares returns the squares covered by a furniture at row/col with width/depth and direction.
func (ph *PlacementHelper) coverageSquares(row, col, width, depth, dir int) []*isoengine.IsoSquare {
	iso := ph.scene.isoScene
	squares := make([]*isoengine.IsoSquare, 0, width*depth)
	// Ensure 2x2 furniture footprints align with visuals: expand width to the right from origin
	if width == 2 && depth == 2 {
		for d := 0; d < depth; d++ {
			r := row + d
			for w := 0; w < width; w++ {
				c := col + w
				squares = append(squares, iso.GetSquareByRowCol(r, c))
			}
		}
		return squares
	}
	switch dir {
	case 0, 4: // extend depth along rows, width along cols
		for d := 0; d < depth; d++ {
			r := row + d
			for w := 0; w < width; w++ {
				c := col + w
				squares = append(squares, iso.GetSquareByRowCol(r, c))
			}
		}
	case 2, 6: // horizontal footprint: depth along rows, width along (left) columns
		for d := 0; d < depth; d++ {
			r := row + d
			for w := 0; w < width; w++ {
				c := col - w
				squares = append(squares, iso.GetSquareByRowCol(r, c))
			}
		}
	default:
		// Default to 0/4 layout
		for d := 0; d < depth; d++ {
			r := row + d
			for w := 0; w < width; w++ {
				c := col + w
				squares = append(squares, iso.GetSquareByRowCol(r, c))
			}
		}
	}
	return squares
}

// anchorRowColFor returns the base anchor row/col used for stacking for a given placed item
func (ph *PlacementHelper) anchorRowColFor(pf PlacedFurni) (int, int) {
	row := pf.Row
	col := pf.Col
	switch pf.Dir {
	case 2, 6:
		row = pf.Row + (pf.Depth - 1)
		col = pf.Col - (pf.Width - 1) + 1
	case 0, 4:
		row = pf.Row + (pf.Depth - 1)
		col = pf.Col + (pf.Width - 1)
	}
	return row, col
}

// computeStackLevel returns the GridY to assign for a new item at row/col given footprint/dir
// Rugs always return 0 (base level) to go underneath other furniture
func (ph *PlacementHelper) computeStackLevel(row, col, width, depth, dir int) int {
	// Check if this is for a rug by looking at the holding item
	var holdingImageBase string
	if ph.scene.backpackManager != nil {
		_, _, holdingImageBase, _, _, _ = ph.scene.backpackManager.GetHoldingInfo()
	}
	isRug := strings.Contains(strings.ToLower(holdingImageBase), "rug") || strings.Contains(strings.ToLower(holdingImageBase), "hardwood")
	if isRug {
		return 0 // Rugs always go at base level
	}

	stackLevel := 0
	if len(ph.scene.placedFurni) == 0 {
		return stackLevel
	}
	newCovered := ph.coverageSquares(row, col, width, depth, dir)
	for _, existing := range ph.scene.placedFurni {
		existingCovered := ph.coverageSquares(existing.Row, existing.Col, existing.Width, existing.Depth, existing.Dir)
		if squaresOverlap(newCovered, existingCovered) && ph.scene.allowsItemsOnTop(existing) {
			if existing.GridY+10 > stackLevel {
				stackLevel = existing.GridY + 10
			}
		}
	}
	return stackLevel
}

// supportStackHeightPx sums the heights of all lower stacked items that overlap with this item
func (ph *PlacementHelper) supportStackHeightPx(pf PlacedFurni) int {
	// Get all squares covered by this furniture item
	thisCovered := ph.coverageSquares(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
	total := 0
	for _, existing := range ph.scene.placedFurni {
		if existing.GridY >= pf.GridY {
			continue
		}
		// Check if this existing item overlaps with any square of the current item
		existingCovered := ph.coverageSquares(existing.Row, existing.Col, existing.Width, existing.Depth, existing.Dir)
		if squaresOverlap(thisCovered, existingCovered) {
			total += existing.Height
		}
	}
	// If the item is floating, retain at least the cached floating offset so it doesn't drop
	if pf.Floating {
		if pf.FloatingOffsetPx > total {
			return pf.FloatingOffsetPx
		}
	}
	return total
}

// placeWallItem places a wall item on a wall
func (ph *PlacementHelper) placeWallItem() bool {
	// Only the room owner can place or move wall items
	if ph.scene != nil && !ph.scene.IsRoomOwner() {
		return false
	}
	// We need to get mouse coordinates from the scene
	// For now, let's use a different approach - check if we can get them from ebiten
	mx, my := ebiten.CursorPosition()

	// Check if over any wall
	wallDirection, isOverWall := ph.getWallUnderMouse(mx, my)
	if !isOverWall {
		return false
	}

	// Validate placement
	if !ph.validateWallItemPlacement(wallDirection) {
		return false
	}

	// Get holding info
	var holdingProdID, holdingPosID, holdingImageBase string
	if ph.scene.backpackManager != nil {
		holdingProdID, holdingPosID, holdingImageBase, _, _, _ = ph.scene.backpackManager.GetHoldingInfo()
	}

	// Compute Shockwave-compatible pixel translations for wall items:
	// x = mouseX - 3; z = mouseY - 15; y = mouseY + 64
	px := mx + 3
	py := my + 61
	pz := my + 14

	// Create wall item record positioned to where the server will place it
	pf := PlacedFurni{
		ProdID:        holdingProdID,
		PosID:         holdingPosID,
		Base:          holdingImageBase,
		Row:           0, // Not applicable for free-form wall items
		Col:           0, // Not applicable for free-form wall items
		Width:         1,
		Depth:         1,
		Dir:           0,
		GridY:         0,
		IsWallItem:    true,
		WallDirection: wallDirection,
		WallX:         float64(mx), // Use raw mouse X; server offset is for packet only
		WallY:         float64(my), // Use raw mouse Y; avoid double-applying server offsets clientside
	}
	// Initialize animation defaults
	pf.FPS = 8
	ph.computeNumFrames(&pf)

	// Set Height from catalog
	if ph.scene.catalog != nil {
		if ci := ph.scene.catalog.GetItemByID(pf.ProdID); ci != nil {
			pf.Height = ci.Height
		}
	}

	// Add to placed furniture list
	ph.scene.placedFurni = append(ph.scene.placedFurni, pf)

	// Send wall item placement packet over network
	if ph.scene.sendFurnitureIfAvailable != nil {
		// Posters/wall items: direction forced to 0 in send; use computed px/py/pz
		if err := ph.scene.sendFurnitureIfAvailable(ph.scene.studio.StudioID, pf.PosID, pf.ProdID,
			px, py, pz, 0, pf.State, 0); err != nil {
			log.Printf("Studio: failed to send wall item placement: %v", err)
		}
	}

	// On successful placement, if this originated from backpack (not moving an existing placed item),
	// remove the source item from the backpack, then clear holding.
	if ph.scene.backpackManager != nil {
		if !ph.scene.backpackManager.movingFromPlaced {
			var holdingProdID, holdingPosID string
			holdingProdID, holdingPosID, _, _, _, _ = ph.scene.backpackManager.GetHoldingInfo()
			if strings.TrimSpace(holdingPosID) != "" {
				ph.scene.backpackManager.RemoveItemByPosID(holdingPosID)
			} else if strings.TrimSpace(holdingProdID) != "" {
				ph.scene.backpackManager.RemoveFirstItemByProdID(holdingProdID)
			}
		}
		ph.scene.backpackManager.ClearHolding()
	}

	return true
}

// PlaceHeldItem attempts to place the currently held item at the mouse square.
// On success, it registers the placed furniture so the scene draws it, and clears holding state.
func (ph *PlacementHelper) PlaceHeldItem() bool {
	if ph.scene == nil || ph.scene.isoScene == nil {
		return false
	}
	// Only the room owner can place or move furniture
	if ph.scene != nil && !ph.scene.IsRoomOwner() {
		return false
	}

	// Check if this is a wall item
	var holdingProdID string
	if ph.scene.backpackManager != nil {
		holdingProdID, _, _, _, _, _ = ph.scene.backpackManager.GetHoldingInfo()
	}
	isWallItem := ph.isWallItem(holdingProdID)

	// Handle wall items differently
	if isWallItem {
		return ph.placeWallItem()
	}

	// Regular furniture placement (existing logic)
	sq := ph.scene.isoScene.GetMouseSquare()
	if sq == nil {
		return false
	}
	row, col := sq.GetRow(), sq.GetCol()
	log.Printf("PlaceHeldItem: Mouse square at client grid [row=%d, col=%d]", row, col)
	var width, depth int
	var holdingImageBase2 string
	if ph.scene.backpackManager != nil {
		_, _, holdingImageBase2, _, width, depth = ph.scene.backpackManager.GetHoldingInfo()
	}
	if (width <= 0 || depth <= 0) && holdingImageBase2 != "" {
		if w2, d2, ok := ph.inferDimensionsFromMembers(holdingImageBase2); ok {
			width, depth = w2, d2
		}
	}
	if width <= 0 {
		width = 1
	}
	if depth <= 0 {
		depth = 1
	}
	// Determine default direction based on asset availability
	dir := 2 // Default direction for most furniture
	var holdingImageBaseForDir string
	if ph.scene.backpackManager != nil {
		_, _, holdingImageBaseForDir, _, _, _ = ph.scene.backpackManager.GetHoldingInfo()
	}

	// If we are moving an existing placed item, preserve its current direction
	if ph.scene != nil && ph.scene.backpackManager != nil {
		if ph.scene.backpackManager.holdingDirSet {
			dir = ph.scene.backpackManager.holdingDir
		}
	}

	// Rugs default to direction 0
	if strings.Contains(strings.ToLower(holdingImageBaseForDir), "rug") || strings.Contains(strings.ToLower(holdingImageBaseForDir), "hardwood") {
		dir = 0
	} else if ph.scene != nil && ph.scene.hasOnlyDir4Assets(holdingImageBaseForDir, width, depth) {
		// Furniture with only dir 4 assets defaults to direction 4
		dir = 4
	}
	if !ph.ValidateSnap(row, col, width, depth, dir) {
		return false
	}
	// Determine stacking GridY based on overlapped IAOT items (topmost level + step)
	// Rugs always go at base level (GridY = 0) underneath other furniture
	stackLevel := 0
	computedHO := 0
	// Check if this is a rug
	isRug := strings.Contains(strings.ToLower(holdingImageBaseForDir), "rug") || strings.Contains(strings.ToLower(holdingImageBaseForDir), "hardwood")
	if !isRug && len(ph.scene.placedFurni) > 0 {
		newCovered := ph.coverageSquares(row, col, width, depth, dir)
		// First pass: find the topmost GridY among overlapped IAOT items
		topGY := -1
		for _, existing := range ph.scene.placedFurni {
			existingCovered := ph.coverageSquares(existing.Row, existing.Col, existing.Width, existing.Depth, existing.Dir)
			if squaresOverlap(newCovered, existingCovered) && ph.scene.allowsItemsOnTop(existing) {
				if existing.GridY+10 > stackLevel {
					stackLevel = existing.GridY + 10
				}
				if existing.GridY > topGY {
					topGY = existing.GridY
				}
			}
		}
		// Second pass: sum heights of all overlapped IAOT items at or below topGY
		computedHO = 0
		for _, existing := range ph.scene.placedFurni {
			existingCovered := ph.coverageSquares(existing.Row, existing.Col, existing.Width, existing.Depth, existing.Dir)
			if squaresOverlap(newCovered, existingCovered) && ph.scene.allowsItemsOnTop(existing) {
				if existing.GridY <= topGY {
					computedHO += existing.Height
				}
			}
		}
	}
	// Get holding info
	var holdingPosID, holdingImageBase3 string
	if ph.scene.backpackManager != nil {
		holdingProdID, holdingPosID, holdingImageBase3, _, _, _ = ph.scene.backpackManager.GetHoldingInfo()
	}
	// Ensure a unique PosID per placed instance so per-PosID depth boosts remain isolated
	uniquePosID := ph.makeUniquePosID(holdingPosID)
	// Adjust storage origin based on furniture type and size
	storeRow, storeCol := row, col
	if strings.Contains(strings.ToLower(holdingImageBase3), "rug") || strings.Contains(strings.ToLower(holdingImageBase3), "hardwood") {
		// For rugs, store origin shifted so that hovered (top-right) becomes proper top-left origin
		storeCol = col - (max1(width) - 1)
		log.Printf("PlaceHeldItem: Rug adjustment - shifted col from %d to %d (width=%d)", col, storeCol, width)
	} else if width == 2 && depth == 2 {
		// For 2x2 furniture, align with ghost/validation: only shift one column to the left
		storeCol = col - 1
		log.Printf("PlaceHeldItem: 2x2 adjustment - shifted col from %d to %d", col, storeCol)
	}
	log.Printf("PlaceHeldItem: Final storage position [row=%d, col=%d] (width=%d, depth=%d, dir=%d)",
		storeRow, storeCol, width, depth, dir)
	pf := PlacedFurni{
		ProdID:     holdingProdID,
		PosID:      uniquePosID,
		Base:       holdingImageBase3,
		Row:        storeRow,
		Col:        storeCol,
		Width:      width,
		Depth:      depth,
		Dir:        dir,
		GridY:      stackLevel,
		IsWallItem: ph.isWallItem(holdingProdID),
	}
	// If we're moving an existing placed item, preserve its pixel offsets (from rotation adjustments)
	if ph.scene != nil && ph.scene.backpackManager != nil && ph.scene.backpackManager.movingFromPlaced {
		pf.PixelOffsetX = ph.scene.backpackManager.movedOriginal.PixelOffsetX
		pf.PixelOffsetY = ph.scene.backpackManager.movedOriginal.PixelOffsetY
	}
	// Initialize animation defaults (Lingo FurnitureItem defaults to FPS=8, frame=0)
	pf.FPS = 8
	ph.computeNumFrames(&pf)
	// Set Height from catalog for stacking Y offset
	if ph.scene.catalog != nil {
		if ci := ph.scene.catalog.GetItemByID(pf.ProdID); ci != nil {
			pf.Height = ci.Height
		}
	}
	// If stacked on an IAOT support, set ho to that support's catalog height for outgoing POS
	if computedHO > 0 {
		pf.ServerHO = computedHO
	}
	ph.scene.placedFurni = append(ph.scene.placedFurni, pf)

	// Apply automatic sofa slice depth boosting
	ph.scene.applySofaSliceBoosts(pf)
	// Apply Pinball-specific slice depth boosting
	ph.scene.applyPinballSliceBoosts(pf)
	// Apply Vanilla Coke Table slice depth boosting
	ph.scene.applyVanillaCokeTableSliceBoosts(pf)
	// Update walkability for all affected squares AFTER the furniture is added to the list
	// Use stored origin (which may be adjusted for rugs) so coverage matches actual placement
	covered := ph.coverageSquares(pf.Row, pf.Col, width, depth, dir)
	for _, csq := range covered {
		if csq != nil {
			ph.updateSquareWalkability(csq.GetRow(), csq.GetCol())
		}
	}

	// Send furniture placement packet over network
	if ph.scene.sendFurnitureIfAvailable != nil {
		// Use the stored row/col (which may be adjusted for rugs/2x2) as the placement origin
		// Y is the stack level (GridY / 10 for server compatibility, as server uses 1 per level, we use 10)
		y := pf.GridY / 10
		ho := pf.ServerHO
		// For 2x2 furniture, we store origin shifted left by 1 for correct client coverage/rendering,
		// but the server expects the unshifted column. Send adjusted coordinates.
		sendRow := pf.Row
		sendCol := pf.Col
		if pf.Width == 2 && pf.Depth == 2 {
			sendCol = pf.Col + 1
		}
		log.Printf("PlaceHeldItem: Sending to network - PosID=%s ProdID=%s client[row=%d,col=%d] y=%d ho=%d dir=%d state=%d",
			pf.PosID, pf.ProdID, sendRow, sendCol, y, ho, pf.Dir, pf.State)
		if err := ph.scene.sendFurnitureIfAvailable(ph.scene.studio.StudioID, pf.PosID, pf.ProdID,
			sendRow, sendCol, y, pf.Dir, pf.State, ho); err != nil {
			log.Printf("Studio: failed to send furniture placement: %v", err)
		} else {
			log.Printf("PlaceHeldItem: Successfully sent furniture placement packet")
		}
	}
	// On successful placement, if this originated from backpack (not moving an existing placed item),
	// remove the source item from the backpack, then clear holding.
	if ph.scene.backpackManager != nil {
		if !ph.scene.backpackManager.movingFromPlaced {
			var holdingProdID, holdingPosID string
			holdingProdID, holdingPosID, _, _, _, _ = ph.scene.backpackManager.GetHoldingInfo()
			if strings.TrimSpace(holdingPosID) != "" {
				ph.scene.backpackManager.RemoveItemByPosID(holdingPosID)
			} else if strings.TrimSpace(holdingProdID) != "" {
				ph.scene.backpackManager.RemoveFirstItemByProdID(holdingProdID)
			}
		}
		ph.scene.backpackManager.ClearHolding()
	}

	return true
}

// PlacedFurni is a minimal placed furniture record for rendering
type PlacedFurni struct {
	ProdID       string
	PosID        string
	Base         string
	Row          int
	Col          int
	Width        int
	Depth        int
	Dir          int
	GridY        int
	Height       int
	PixelOffsetX int  // Additional pixel offset for fine positioning
	PixelOffsetY int  // Additional pixel offset for fine positioning
	IsWallItem   bool // True if this is a wall item (poster)
	ServerHO     int  // Server-provided vertical pixel offset (ho). If >0, use instead of computed stack height

	// IAOT override from server POS (if provided). When true/false and IAOTProvided=true,
	// this value takes precedence over catalog ItemsAllowedOnTop for stacking rules.
	IAOT         bool
	IAOTProvided bool

	// Floating state: when supports are removed/moved, items above should visually "float"
	// at their previous stacked height until they are themselves moved or picked up.
	Floating           bool // true if this item is currently floating (supports removed/moved)
	FloatingOffsetPx   int  // cached cumulative support height in pixels at time of floating
	FloatingIAOTHeight int  // cached cumulative IAOT height units below at time of floating (for avatar depth)

	// Animation/state (Lingo parity)
	State        int            // 0=off, 1=on
	Animate      bool           // whether to step frames
	Frame        int            // current frame index (0..NumFrames)
	NumFrames    int            // highest available frame index for this asset set
	FPS          int            // frames per second (default 8)
	LastFrameTS  int64          // last frame tick timestamp (ms)
	AnimFrames   map[string]int // per-slice max frame index (a/b/c/d)
	OneShotFrame int            // transient frame to display once (e.g., 8 for TV power-off)
	OneShotUntil int64          // timestamp millis until which oneshot is allowed to render
	DriverSlice  string         // slice with the most frames to drive cycle
	ContigMax    map[string]int // per-slice contiguous max from frame 1 (loop upper bound)

	// Wall item specific fields
	WallDirection string  // "left" or "right" - which wall this item is on
	WallX         float64 // X position on the wall (relative to wall tile)
	WallY         float64 // Y position on the wall (relative to wall tile)
}

type layerDraw struct {
	img   *ebiten.Image
	x, y  float64
	depth int
	flipH bool
	order string // Layer order (sd, a, b, c, d, wall)
	pf    *PlacedFurni
}

// applySliceBoosts applies per-PosID slice depth boosts only
func (ph *PlacementHelper) applySliceBoosts(pf PlacedFurni, order string, z int) int {
	if ph.scene == nil {
		return z
	}

	// Apply per-PosID slice boosts only
	if m, ok := ph.scene.depthBoostsBySlicePerPos[strings.TrimSpace(pf.PosID)]; ok && m != nil {
		if sdelta, ok2 := m[strings.ToLower(order)]; ok2 {
			z += sdelta
		}
	}

	return z
}

// DrawDepthSorted renders placed furniture and, if applicable, the held ghost, depth-sorted together.
// Returns true if a ghost was drawn.
func (ph *PlacementHelper) DrawDepthSorted(screen *ebiten.Image) bool {
	if ph.scene == nil || ph.scene.isoScene == nil {
		return false
	}
	iso := ph.scene.isoScene
	var draws []layerDraw

	// Helpers for depth mapping and base-square rules
	// Simple depth rule: higher row -> on top, if same row then lower column -> on top.
	// We keep lower numeric depth = closer to viewer to match the existing descending sort.
	simpleDepth := func(row, col, weight int) int {
		return row*10000 - col*5
	}
	// Removed per-layer ordering; zshift files should encode any layer-specific ordering
	baseSquareFor := func(pf PlacedFurni, order string) *isoengine.IsoSquare {
		row := pf.Row
		col := pf.Col
		// Special anchoring for 2x2 furniture to center them properly
		if pf.Width == 2 && pf.Depth == 2 {
			// For 2x2 furniture, use center point for better visual positioning
			switch pf.Dir {
			case 2, 6:
				row = pf.Row
				col = pf.Col
			case 0, 4:
				row = pf.Row
				col = pf.Col
			}
		}
		// Special anchoring for 3x3 rugs to center them properly (treat hardwood as rug too)
		isRugish := ph.scene.isRugFurniture(pf) || strings.Contains(strings.ToLower(pf.Base), "hardwood") || strings.Contains(strings.ToLower(pf.Base), "rug")
		if pf.Width == 3 && pf.Depth == 3 && isRugish {
			// For 3x3 rugs, use center square as anchor for better visual positioning
			switch pf.Dir {
			case 2, 6:
				row = pf.Row + 1
				col = pf.Col - 1
			case 0, 4:
				row = pf.Row + 1
				col = pf.Col + 1
			}
		}
		if sq := iso.GetSquareByRowCol(row, col); sq != nil {
			return sq
		}
		return iso.GetSquareByRowCol(pf.Row, pf.Col)
	}

	appendLayer := func(pf PlacedFurni, name, order string) {
		// Use wall items member map for wall items, furniture map for furniture
		var memberMap *lingoassets.MemberMap
		var registry *studioroom.MemberRegistry

		if pf.IsWallItem {
			memberMap = ph.wallItemsMM
			registry = ph.wallItemsReg
		} else {
			memberMap = ph.mm
			registry = ph.reg
		}

		if memberMap == nil {
			return
		}

		var p string
		var ok bool

		if pf.IsWallItem {
			// For wall items, use stored wall direction and resolve asset name
			wallDir := getWallDirectionCode(pf.WallDirection)

			// Wall items don't use the traditional a/b/c/d/sd naming
			// They are single assets with leftwall/rightwall variants
			if resolvedName, found := ph.resolveWallItemAssetName(pf.Base, wallDir); found {
				name = resolvedName
				p, ok = memberMap.PathFor(name)
			} else {
				return // Asset not found
			}
		} else {
			// For shadow layers, prefer direction-specific members like base_sd_0/base_sd_4
			if order == "sd" {
				// Map even directions to available shadow directions (0 or 4)
				shadowDir := pf.Dir
				if shadowDir == 2 {
					shadowDir = 4
				} else if shadowDir == 6 {
					shadowDir = 0
				}
				candidate := pf.Base + "_sd_" + strconv.Itoa(shadowDir)
				if p2, ok2 := memberMap.PathFor(candidate); ok2 {
					name = candidate
					p = p2
					ok = true
				} else {
					p, ok = memberMap.PathFor(name)
				}
			} else {
				// Replace rule: when active, choose any available >0 frame for this slice (starting from current),
				// never falling back to frame 0 during playback. If none exist, draw static.
				origName := name
				var path string
				name, path, ok = ph.resolveAnimatedOrStatic(memberMap, origName, order, &pf)
				p = path
			}
		}

		if !ok {
			// Fallback for shadow only if item is Sofa (not applicable to wall items)
			if order == "sd" && !pf.IsWallItem {
				isSofa := false
				if ph.scene != nil && ph.scene.catalog != nil {
					if ci := ph.scene.catalog.GetItemByID(pf.ProdID); ci != nil && ci.Type == "Sofa" {
						isSofa = true
					}
				}
				if !isSofa && (strings.Contains(pf.Base, "sofa") || strings.Contains(pf.Base, "couch")) {
					isSofa = true
				}
				if isSofa {
					if p2, ok2 := memberMap.PathFor("2x1sofaistic_sd"); ok2 {
						name = "2x1sofaistic_sd"
						p = p2
					}
				}
			}
			// Directional fallback via horizontal mirror for a/b/c/d layers
			if p == "" && order != "sd" {
				// For Sofa-type 2x1/1x2 items, avoid mirrored-direction fallback so we don't hflip; use only partials present.
				isSofaTwoWide := false
				if ph.scene != nil && ph.scene.catalog != nil {
					if ci := ph.scene.catalog.GetItemByID(strings.TrimSpace(pf.ProdID)); ci != nil && strings.EqualFold(ci.Type, "Sofa") {
						isSofaTwoWide = true
					}
				}
				if !isSofaTwoWide {
					b := strings.ToLower(pf.Base)
					isSofaTwoWide = strings.Contains(b, "sofa") || strings.Contains(b, "couch")
				}
				isSofaTwoWide = isSofaTwoWide && (pf.Width == 2 || pf.Depth == 2)
				// For 3x3 rugs with single assets, allow hflip for all directions
				isRug3x3 := pf.Width == 3 && pf.Depth == 3 && ph.scene.isRugFurniture(pf)
				if !isSofaTwoWide || isRug3x3 {
					mirrorEven := func(d int) int {
						switch d {
						case 0:
							return 6
						case 6:
							return 0
						case 2:
							return 4
						case 4:
							return 2
						default:
							return d
						}
					}
					// Recompute canonical sizes for name building
					aw := max1(pf.Width)
					ad := max1(pf.Depth)
					if aw < ad {
						aw, ad = ad, aw
					}
					w := strconv.Itoa(aw)
					d := strconv.Itoa(ad)
					altDir := mirrorEven(pf.Dir)
					altName := pf.Base + "_" + order + "_0_" + w + "_" + d + "_" + strconv.Itoa(altDir) + "_0"
					if p2, ok2 := ph.mm.PathFor(altName); ok2 {
						name = altName
						p = p2
						// Mark flip via name suffix in a closure var using map (hack not needed here; we'll detect by dir diff below)
					}
				}
			}
			// Re-check availability after fallback; if still missing, log and exit
			if p == "" {
				return
			}
		}
		im, err := lingoassets.LoadImage(p)
		if err != nil {
			return
		}

		var vp core.Point2D
		var offX, offY int

		if pf.IsWallItem {
			// For wall items, use absolute screen coordinates stored in WallX/WallY
			vp.X = pf.WallX
			vp.Y = pf.WallY
		} else {
			// For regular furniture, position from base square bottom-left + registration correction
			sq := baseSquareFor(pf, order)
			if sq == nil {
				return
			}
			vp = sq.GetViewPoints().BottomLeft
		}

		// Get registration point offset
		if registry != nil {
			if rp, ok := registry.GetRegistrationPoint(name); ok {
				offX, offY = rp[0], rp[1]
			}
		}
		// Detect if this layer needs a horizontal mirror (2<>4, 0<>6).
		// For non-shadow layers, infer by comparing asset dir with pf.Dir.
		// For shadow layers, if we loaded a directional shadow (base_sd_0 or base_sd_4), flip when pf.Dir is 2 or 6 accordingly.
		// Skip flipping for wall items since they have separate left/right assets
		flipForThis := false
		if !pf.IsWallItem && order != "sd" {
			parts := strings.Split(name, "_")
			if len(parts) >= 2 {
				if dirVal, err := strconv.Atoi(parts[len(parts)-2]); err == nil && dirVal != pf.Dir {
					mirrorPair := (pf.Dir == 2 && dirVal == 4) || (pf.Dir == 4 && dirVal == 2) || (pf.Dir == 0 && dirVal == 6) || (pf.Dir == 6 && dirVal == 0)
					if mirrorPair {
						flipForThis = true
					}
				}
			}
		} else if !pf.IsWallItem {
			// Shadow layer: if using directional shadow (name contains _sd_0 or _sd_4), flip for pf.Dir 2/6 accordingly
			if strings.Contains(name, "_sd_4") && pf.Dir == 2 {
				flipForThis = true
			} else if strings.Contains(name, "_sd_0") && pf.Dir == 6 {
				flipForThis = true
			}
		}
		if flipForThis {
			// Convert registration for horizontal flip: x' = imageWidth - x
			imgW := im.Bounds().Dx()
			offX = imgW - offX
		}
		// For placed items, respect the same offsets used at place time (do not drop ghost offsets)
		x := float64(vp.X) - float64(offX)
		y := float64(vp.Y) - float64(offY)
		// Directional correction for shadow: shift X for dirs 6 and 0
		if order == "sd" {
			// Special-case correction for vanillacoketable (2x2 table)
			if pf.Width == 2 && pf.Depth == 2 {
				switch pf.Dir {
				case 6:
					x += 0
				case 0:
					x -= 0
				}
			} else {
				switch pf.Dir {
				case 6:
					x += -72
				case 0:
					x += 72
				}
			}
		}
		// Pinball-specific correction: when facing dir 2, shift slices c and d left by 72px
		if order != "sd" {
			isPinball := false
			if ph.scene != nil && ph.scene.catalog != nil {
				if ci := ph.scene.catalog.GetItemByID(strings.TrimSpace(pf.ProdID)); ci != nil && strings.EqualFold(ci.Type, "Pinball") {
					isPinball = true
				}
			}
			if !isPinball {
				b := strings.ToLower(pf.Base)
				if strings.Contains(b, "pinball") {
					isPinball = true
				}
			}
			if isPinball && pf.Dir == 2 && (order == "c" || order == "d") {
				x += 72
			}
		}
		// Apply stacking vertical offset based on cumulative height of lower items
		stackPx := 0
		if pf.ServerHO > 0 {
			stackPx = pf.ServerHO
		} else if pf.GridY > 0 {
			stackPx = ph.supportStackHeightPx(pf)
		}
		// If floating, keep at least the cached height
		if pf.Floating && pf.FloatingOffsetPx > stackPx {
			stackPx = pf.FloatingOffsetPx
		}
		y -= float64(stackPx)
		// Apply additional pixel offsets for fine positioning (e.g., 2x1/1x2 rotation offsets)
		x += float64(pf.PixelOffsetX)
		y += float64(pf.PixelOffsetY)
		// Depth uses only stacking and zshift; no per-layer implicit ordering
		weight := 0
		// Apply stacking height so items placed on top are drawn above
		// Higher GridY values should have lower depth values (drawn on top)
		weight -= 0

		// Use the actual position where this layer is drawn for depth calculation
		// This ensures multi-tile furniture has correct per-tile depth
		layerSq := baseSquareFor(pf, order)
		var depthRow, depthCol int
		if layerSq != nil {
			depthRow = layerSq.GetRow()
			depthCol = layerSq.GetCol()
		} else {
			// Fallback to furniture origin if square not found
			depthRow = pf.Row
			depthCol = pf.Col
		}
		z := simpleDepth(depthRow, depthCol, weight)
		// Boost depth so stacked items render on top; larger z draws later
		if stackPx > 0 && ph.scene != nil {
			z += stackPx * ph.scene.heightDepthAdjust
		}

		// Apply per-PosID slice boosts only
		if ph.scene != nil {
			z = ph.applySliceBoosts(pf, order, z)
		}

		// Apply z-shift from .zshift.txt file for this specific layer
		zshift := ph.parseZShift(pf.Base, order)
		z += zshift

		// Rule boosts (constant depth adjustments independent of row/col)
		if ph.scene != nil && ph.scene.catalog != nil {
			if ci := ph.scene.catalog.GetItemByID(pf.ProdID); ci != nil {
				if strings.EqualFold(ci.Type, "separator") && ci.ItemsAllowedOnTop && (pf.Dir == 2 || pf.Dir == 6) {
					z += 10000
				}
			}
		}

		// Store with coordinates; flipping handled at draw time via GeoM
		draws = append(draws, layerDraw{img: im, x: x, y: y, depth: z, flipH: flipForThis, order: order})
	}

	for _, pf := range ph.scene.placedFurni {
		if pf.IsWallItem {
			// Wall items are single assets, not multi-layered like furniture
			appendLayer(pf, "", "wall") // Empty name - will be resolved by appendLayer
		} else {
			// Shadow (skip for rugs - they don't have shadow layers)
			if !ph.scene.isRugFurniture(pf) {
				appendLayer(pf, pf.Base+"_sd", "sd")
			}
			// a/b/c/d
			aw := max1(pf.Width)
			ad := max1(pf.Depth)
			if aw < ad {
				aw, ad = ad, aw
			}
			w := strconv.Itoa(aw)
			d := strconv.Itoa(ad)
			dirS := strconv.Itoa(pf.Dir)
			for _, o := range []string{"a", "b", "c", "d"} {
				name := pf.Base + "_" + o + "_0_" + w + "_" + d + "_" + dirS + "_0"
				appendLayer(pf, name, o)
			}
		}
	}

	// Add ghost layers if holding over a walkable tile and mouse not over any window
	ghostDrawn := false
	holdingActive := ph.scene.backpackManager != nil && ph.scene.backpackManager.IsHoldingActive()
	if holdingActive && (ph.scene.hudManager == nil || ph.scene.hudManager.GetWindowManager() == nil || !ph.scene.hudManager.GetWindowManager().IsMouseOverAnyWindow()) {
		var holdingProdID, holdingImageBase string
		var holdingWidth, holdingDepth int
		if ph.scene.backpackManager != nil {
			holdingProdID, _, holdingImageBase, _, holdingWidth, holdingDepth = ph.scene.backpackManager.GetHoldingInfo()
		}

		isWallItem := ph.isWallItem(holdingProdID)

		if isWallItem {
			// Handle wall item ghost preview
			mx, my := ebiten.CursorPosition()
			wallDirection, isOverWall := ph.getWallUnderMouse(mx, my)
			if isOverWall && ph.validateWallItemPlacement(wallDirection) && holdingImageBase != "" {
				pf := PlacedFurni{
					Base:          holdingImageBase,
					Row:           0, // Not applicable for wall items
					Col:           0, // Not applicable for wall items
					Width:         1,
					Depth:         1,
					Dir:           0,
					GridY:         0,
					IsWallItem:    true,
					WallDirection: wallDirection,
					WallX:         float64(mx), // Use current mouse coordinates
					WallY:         float64(my), // Use current mouse coordinates
				}
				// Look up height from catalog for ghost offset when previewing stacked
				if ph.scene.catalog != nil {
					if ci := ph.scene.catalog.GetItemByID(holdingProdID); ci != nil {
						pf.Height = ci.Height
					}
				}
				// Wall items are single assets
				appendLayer(pf, "", "wall")
				ghostDrawn = true
			}
		} else if sq := iso.GetMouseSquare(); sq != nil && holdingImageBase != "" {
			// Handle regular furniture ghost preview
			w := max1(holdingWidth)
			d := max1(holdingDepth)
			// Determine ghost direction based on asset availability
			ghostDir := 2 // Default direction for most furniture
			// Preserve direction if holding originated from a placed item move
			if ph.scene != nil && ph.scene.backpackManager != nil {
				if ph.scene.backpackManager.holdingDirSet {
					ghostDir = ph.scene.backpackManager.holdingDir
				}
			}
			if strings.Contains(strings.ToLower(holdingImageBase), "rug") || strings.Contains(strings.ToLower(holdingImageBase), "hardwood") {
				ghostDir = 0 // Rugs use dir 0 assets
			} else if ph.scene != nil && ph.scene.hasOnlyDir4Assets(holdingImageBase, holdingWidth, holdingDepth) {
				ghostDir = 4 // Furniture with only dir 4 assets defaults to direction 4
			}
			if ph.ValidateSnap(sq.GetRow(), sq.GetCol(), w, d, ghostDir) {
				pf := PlacedFurni{
					Base:       holdingImageBase,
					Row:        sq.GetRow(),
					Col:        sq.GetCol(),
					Width:      max1(holdingWidth),
					Depth:      max1(holdingDepth),
					Dir:        ghostDir,
					GridY:      0,
					IsWallItem: ph.isWallItem(holdingProdID),
				}
				// Provide ProdID and stable PosID so slice boosts can be applied for ghost preview
				pf.ProdID = holdingProdID
				pf.PosID = "ghost-preview"
				// Apply slice depth boosts for ghost preview to match placed behavior
				if ph.scene != nil {
					ph.scene.applySofaSliceBoosts(pf)
					ph.scene.applyPinballSliceBoosts(pf)
					ph.scene.applyVanillaCokeTableSliceBoosts(pf)
				}
				// Adjust ghost origin to match placement logic
				if strings.Contains(strings.ToLower(pf.Base), "rug") || strings.Contains(strings.ToLower(pf.Base), "hardwood") {
					// For rugs (dir 0 assets), shift ghost origin from hovered top-right to top-left so it matches placement
					pf.Col = pf.Col - (max1(pf.Width) - 1)
				} else if pf.Width == 2 && pf.Depth == 2 {
					// For 2x2 furniture, shift ghost origin to match placement anchor calculation
					pf.Col = pf.Col - 1
				}
				// look up height from catalog for ghost offset when previewing stacked
				if ph.scene.catalog != nil {
					if ci := ph.scene.catalog.GetItemByID(holdingProdID); ci != nil {
						pf.Height = ci.Height
					}
				}

				if pf.IsWallItem {
					// Wall items are single assets
					appendLayer(pf, "", "wall")
				} else {
					// Shadow (skip for rugs - they don't have shadow layers)
					if !ph.scene.isRugFurniture(pf) {
						appendLayer(pf, pf.Base+"_sd", "sd")
					}
					aw := max1(pf.Width)
					ad := max1(pf.Depth)
					if aw < ad {
						aw, ad = ad, aw
					}
					w := strconv.Itoa(aw)
					d := strconv.Itoa(ad)
					dirS := strconv.Itoa(pf.Dir)
					for _, o := range []string{"a", "b", "c", "d"} {
						name := pf.Base + "_" + o + "_0_" + w + "_" + d + "_" + dirS + "_0"
						appendLayer(pf, name, o)
					}
				}
				ghostDrawn = true
			}
		}
	}

	// Sort: draw lower depth first (further back), higher depth later (on top)
	sort.SliceStable(draws, func(i, j int) bool { return draws[i].depth < draws[j].depth })
	for _, d := range draws {
		if d.order == "sd" {
			// Shadow layers use 40% opacity as per Lingo blend setting
			op := &colorm.DrawImageOptions{}
			if d.flipH {
				w := d.img.Bounds().Dx()
				op.GeoM.Scale(-1, 1)
				op.GeoM.Translate(d.x+float64(w), d.y)
			} else {
				op.GeoM.Translate(d.x, d.y)
			}
			var cm colorm.ColorM
			cm.Scale(1, 1, 1, 0.4) // 40% opacity for shadows
			colorm.DrawImage(screen, d.img, cm, op)
		} else {
			// Regular layers use full opacity
			op := &ebiten.DrawImageOptions{}
			if d.flipH {
				w := d.img.Bounds().Dx()
				op.GeoM.Scale(-1, 1)
				op.GeoM.Translate(d.x+float64(w), d.y)
			} else {
				op.GeoM.Translate(d.x, d.y)
			}
			screen.DrawImage(d.img, op)

			// Debug label: draw PosID above the furniture when enabled
			if ph.scene != nil && ph.scene.backpackManager != nil && ph.scene.backpackManager.showPosLabels && ph.scene.hudManager != nil && d.pf != nil {
				fm := ph.scene.hudManager.GetFontManager()
				if fm != nil {
					face := fm.GetFont("Verdana", "bold", 11)
					if face != nil {
						label := strings.TrimSpace(d.pf.PosID)
						if label != "" {
							bounds := d.img.Bounds()
							tx := d.x + float64(bounds.Dx())/2 - float64(text.BoundString(face, label).Dx())/2
							ty := d.y - 4
							text.Draw(screen, label, face, int(tx), int(ty), colornames.Green)
						}
					}
				}
			}
		}
	}
	return ghostDrawn
}

// BuildDrawsSimple builds all furniture layers (and ghost if present) using the simple
// row/column-based depth rule and returns the list without drawing. The caller is
// responsible for sorting and rendering. It returns whether a ghost was added.
func (ph *PlacementHelper) BuildDrawsSimple() ([]layerDraw, bool) {
	if ph.scene == nil || ph.scene.isoScene == nil {
		return nil, false
	}
	iso := ph.scene.isoScene
	var draws []layerDraw

	// Helpers for depth mapping and base-square rules
	baseSquareFor := func(pf PlacedFurni, order string) *isoengine.IsoSquare {
		row := pf.Row
		col := pf.Col
		// Anchor square determined solely by footprint orientation
		switch pf.Dir {
		case 2, 6:
			row = pf.Row
			col = pf.Col
		case 0, 4:
			row = pf.Row
			col = pf.Col
		}
		if pf.Width == 2 && pf.Depth == 1 && (pf.Dir == 2 || pf.Dir == 6) {
			row -= 1
		}
		// Special anchoring for 2x2 furniture to center them properly
		if pf.Width == 2 && pf.Depth == 2 {
			// For 2x2 furniture, use center point for better visual positioning
			switch pf.Dir {
			case 2, 6:
				row = pf.Row
				col = pf.Col
			case 0, 4:
				row = pf.Row
				col = pf.Col
			}
		}
		// Special anchoring for 3x3 rugs to center them properly
		if pf.Width == 3 && pf.Depth == 3 && ph.scene.isRugFurniture(pf) {
			// For 3x3 rugs, use center square as anchor for better visual positioning
			switch pf.Dir {
			case 2, 6:
				row = pf.Row + 1
				col = pf.Col - 1
			case 0, 4:
				row = pf.Row + 1
				col = pf.Col + 1
			}
		}
		if sq := iso.GetSquareByRowCol(row, col); sq != nil {
			return sq
		}
		return iso.GetSquareByRowCol(pf.Row, pf.Col)
	}

	// Simple depth rule: higher row -> on top, if same row then lower column -> on top.
	simpleDepth := func(row, col, weight int) int {
		return row*10000 - col*5
	}

	appendLayer := func(pf *PlacedFurni, name, order string) {
		// Use wall items member map for wall items, furniture map for furniture
		var memberMap *lingoassets.MemberMap
		var registry *studioroom.MemberRegistry

		if pf.IsWallItem {
			memberMap = ph.wallItemsMM
			registry = ph.wallItemsReg
		} else {
			memberMap = ph.mm
			registry = ph.reg
		}

		if memberMap == nil {
			return
		}

		var p string
		var ok bool

		if pf.IsWallItem {
			// For wall items, use stored wall direction and resolve asset name
			wallDir := getWallDirectionCode(pf.WallDirection)

			// Wall items don't use the traditional a/b/c/d/sd naming
			// They are single assets with leftwall/rightwall variants
			if resolvedName, found := ph.resolveWallItemAssetName(pf.Base, wallDir); found {
				name = resolvedName
				p, ok = memberMap.PathFor(name)
			} else {
				return // Asset not found
			}
		} else {
			// For shadow layers, prefer direction-specific members like base_sd_0/base_sd_4
			if order == "sd" {
				shadowDir := pf.Dir
				if shadowDir == 2 {
					shadowDir = 4
				} else if shadowDir == 6 {
					shadowDir = 0
				}
				candidate := pf.Base + "_sd_" + strconv.Itoa(shadowDir)
				if p2, ok2 := memberMap.PathFor(candidate); ok2 {
					name = candidate
					p = p2
					ok = true
				} else {
					p, ok = memberMap.PathFor(name)
				}
			} else {
				// Replace rule: when active, choose any available >0 frame for this slice (starting from current),
				// never falling back to frame 0 during playback. If none exist, draw static.
				origName := name
				var path string
				name, path, ok = ph.resolveAnimatedOrStatic(memberMap, origName, order, pf)
				p = path
			}
		}

		if !ok {
			if order == "sd" && !pf.IsWallItem {
				isSofa := false
				if ph.scene != nil && ph.scene.catalog != nil {
					if ci := ph.scene.catalog.GetItemByID(pf.ProdID); ci != nil && ci.Type == "Sofa" {
						isSofa = true
					}
				}
				if !isSofa && (strings.Contains(pf.Base, "sofa") || strings.Contains(pf.Base, "couch")) {
					isSofa = true
				}
				if isSofa {
					if p2, ok2 := memberMap.PathFor("2x1sofaistic_sd"); ok2 {
						name = "2x1sofaistic_sd"
						p = p2
					}
				}
			}
			// Directional fallback via horizontal mirror for a/b/c/d layers (2<>4, 0<>6)
			// For 3x3 rugs with single assets, always allow hflip
			if p == "" && order != "sd" {
				mirrorEven := func(d int) int {
					switch d {
					case 2:
						return 4
					case 4:
						return 2
					case 0:
						return 6
					case 6:
						return 0
					default:
						return d
					}
				}
				aw := max1(pf.Width)
				ad := max1(pf.Depth)
				if aw < ad {
					aw, ad = ad, aw
				}
				w := strconv.Itoa(aw)
				d := strconv.Itoa(ad)
				altDir := mirrorEven(pf.Dir)
				altName := pf.Base + "_" + order + "_0_" + w + "_" + d + "_" + strconv.Itoa(altDir) + "_0"
				if p2, ok2 := ph.mm.PathFor(altName); ok2 {
					name = altName
					p = p2
				}
			}
			if p == "" {
				return
			}
		}
		im, err := lingoassets.LoadImage(p)
		if err != nil {
			return
		}
		var vp core.Point2D
		var offX, offY int

		if pf.IsWallItem {
			// For wall items, use absolute screen coordinates stored in WallX/WallY
			vp.X = pf.WallX
			vp.Y = pf.WallY
		} else {
			// For regular furniture, position from base square bottom-left + registration correction
			sq := baseSquareFor(*pf, order)
			if sq == nil {
				return
			}
			vp = sq.GetViewPoints().BottomLeft
		}

		// Get registration point offset
		if registry != nil {
			if rp, ok := registry.GetRegistrationPoint(name); ok {
				offX, offY = rp[0], rp[1]
			}
		}
		// Detect mirrored even direction and convert X registration accordingly
		// Skip flipping for wall items since they have separate left/right assets
		flipForThis := false
		if !pf.IsWallItem && order != "sd" {
			parts := strings.Split(name, "_")
			if len(parts) >= 2 {
				if dirVal, err := strconv.Atoi(parts[len(parts)-2]); err == nil && dirVal != pf.Dir {
					mirrorPair := (pf.Dir == 2 && dirVal == 4) || (pf.Dir == 4 && dirVal == 2) || (pf.Dir == 0 && dirVal == 6) || (pf.Dir == 6 && dirVal == 0)
					if mirrorPair {
						flipForThis = true
					}
				}
			}
		} else if !pf.IsWallItem {
			// Shadow layer: if using directional shadow (name contains _sd_0 or _sd_4), flip when pf.Dir is 2 or 6 accordingly
			if strings.Contains(name, "_sd_4") && pf.Dir == 2 {
				flipForThis = true
			} else if strings.Contains(name, "_sd_0") && pf.Dir == 6 {
				flipForThis = true
			} else if strings.HasSuffix(name, "_sd") {
				// Generic shadow: mirror when footprint spans >1 tile and dir is a mirrored orientation (4 or 0)
				if (pf.Width > 1 || pf.Depth > 1) && (pf.Dir == 4 || pf.Dir == 0) {
					flipForThis = true
				}
			}
		}
		if flipForThis {
			imgW := im.Bounds().Dx()
			offX = imgW - offX
		}
		x := float64(vp.X) - float64(offX)
		y := float64(vp.Y) - float64(offY)

		// Apply appropriate offsets - specific offsets for different furniture types
		// Skip offsets for wall items since they use absolute positioning
		if !pf.IsWallItem {
			// For ghost previews, check base name directly since ProdID may be empty
			isRugish := ph.scene.isRugFurniture(*pf) || strings.Contains(strings.ToLower(pf.Base), "rug") || strings.Contains(strings.ToLower(pf.Base), "hardwood")
			if isRugish {
				x -= float64(rugOffsetX)
				y -= float64(rugOffsetY)
			} else if pf.Width == 2 && pf.Depth == 2 {
				// 2x2 furniture (tables) use specific offsets
				x -= float64(table2x2OffsetX)
				y -= float64(table2x2OffsetY)
			} else if !(pf.Width == 2 && pf.Depth == 1) {
				x -= float64(ghostOffsetX)
				y -= float64(ghostOffsetY)
			}
		}
		// Directional correction for shadow in simple builder: shift X for dirs 6 and 0
		if order == "sd" {
			// Special-case correction for vanillacoketable (2x2 table)
			if pf.Width == 2 && pf.Depth == 2 {
				switch pf.Dir {
				case 6:
					x += 0
				case 0:
					x -= 0
				}
			} else {
				switch pf.Dir {
				case 6:
					x += -72
				case 0:
					x += 72
				}
			}
		}
		// Pinball-specific correction (simple builder): when dir 2, shift c and d left by 72px
		if order != "sd" {
			isPinball := false
			if ph.scene != nil && ph.scene.catalog != nil {
				if ci := ph.scene.catalog.GetItemByID(strings.TrimSpace(pf.ProdID)); ci != nil && strings.EqualFold(ci.Type, "Pinball") {
					isPinball = true
				}
			}
			if !isPinball {
				b := strings.ToLower(pf.Base)
				if strings.Contains(b, "pinball") {
					isPinball = true
				}
			}
			if isPinball && pf.Dir == 2 && (order == "c" || order == "d") {
				x += 72
			}
		}
		// Apply additional pixel offsets for fine positioning
		x += float64(pf.PixelOffsetX)
		y += float64(pf.PixelOffsetY)
		// Apply stacking offset in simple builder path as well
		stackPx2 := 0
		if pf.ServerHO > 0 {
			stackPx2 = pf.ServerHO
		} else if pf.GridY > 0 {
			stackPx2 = ph.supportStackHeightPx(*pf)
		}
		if pf.Floating && pf.FloatingOffsetPx > stackPx2 {
			stackPx2 = pf.FloatingOffsetPx
		}
		y -= float64(stackPx2)
		// Depth uses only stacking and zshift (no per-layer implicit ordering)
		weight := 0

		// Apply stacking height so items placed on top are drawn above
		// Higher GridY values should have lower depth values (drawn on top)
		weight -= 0 // Multiply by large factor to ensure stacked items are always on top

		// Use the actual position where this layer is drawn for depth calculation
		// This ensures multi-tile furniture has correct per-tile depth
		layerSq := baseSquareFor(*pf, order)
		var depthRow, depthCol int
		if layerSq != nil {
			depthRow = layerSq.GetRow()
			depthCol = layerSq.GetCol()
		} else {
			// Fallback to furniture origin if square not found
			depthRow = pf.Row
			depthCol = pf.Col
		}
		z := simpleDepth(depthRow, depthCol, weight)
		if stackPx2 > 0 && ph.scene != nil {
			z += stackPx2 * ph.scene.heightDepthAdjust
		}

		// Apply per-PosID slice boosts only
		if ph.scene != nil {
			z = ph.applySliceBoosts(*pf, order, z)
		}

		// Apply z-shift from .zshift.txt file for this specific layer
		zshift := ph.parseZShift(pf.Base, order)
		z += zshift

		// Rule boosts (constant depth adjustments independent of row/col)
		if ph.scene != nil && ph.scene.catalog != nil {
			if ci := ph.scene.catalog.GetItemByID(pf.ProdID); ci != nil {
				if strings.EqualFold(ci.Type, "separator") && ci.ItemsAllowedOnTop && (pf.Dir == 2 || pf.Dir == 6) {
					z += 10000
				}
			}
		}

		// Store with coordinates; flipping handled at draw time via GeoM
		draws = append(draws, layerDraw{img: im, x: x, y: y, depth: z, flipH: flipForThis, order: order, pf: pf})
	}

	for idx := range ph.scene.placedFurni {
		pf := &ph.scene.placedFurni[idx]
		if pf.IsWallItem {
			// Wall items are single assets, not multi-layered like furniture
			appendLayer(pf, "", "wall") // Empty name - will be resolved by appendLayer
		} else {
			// Shadow (skip for rugs - they don't have shadow layers)
			if !ph.scene.isRugFurniture(*pf) {
				appendLayer(pf, pf.Base+"_sd", "sd")
			}
			// a/b/c/d
			aw := max1(pf.Width)
			ad := max1(pf.Depth)
			if aw < ad {
				aw, ad = ad, aw
			}
			w := strconv.Itoa(aw)
			d := strconv.Itoa(ad)
			dirS := strconv.Itoa(pf.Dir)
			for _, o := range []string{"a", "b", "c", "d"} {
				name := pf.Base + "_" + o + "_0_" + w + "_" + d + "_" + dirS + "_0"
				appendLayer(pf, name, o)
			}
		}
	}

	ghostDrawn := false
	// Add ghost preview when holding an item and mouse is not over any window
	holdingActive := ph.scene.backpackManager != nil && ph.scene.backpackManager.IsHoldingActive()
	if holdingActive && (ph.scene.hudManager == nil || ph.scene.hudManager.GetWindowManager() == nil || !ph.scene.hudManager.GetWindowManager().IsMouseOverAnyWindow()) {
		var holdingProdID, holdingImageBase string
		var holdingWidth, holdingDepth int
		if ph.scene.backpackManager != nil {
			holdingProdID, _, holdingImageBase, _, holdingWidth, holdingDepth = ph.scene.backpackManager.GetHoldingInfo()
		}

		isWallItem := ph.isWallItem(holdingProdID)

		if isWallItem {
			// Handle wall item ghost preview
			mx, my := ebiten.CursorPosition()
			wallDirection, isOverWall := ph.getWallUnderMouse(mx, my)
			if isOverWall && ph.validateWallItemPlacement(wallDirection) && holdingImageBase != "" {
				pf := PlacedFurni{
					Base:          holdingImageBase,
					Row:           0, // Not applicable for wall items
					Col:           0, // Not applicable for wall items
					Width:         1,
					Depth:         1,
					Dir:           0,
					GridY:         0,
					IsWallItem:    true,
					WallDirection: wallDirection,
					WallX:         float64(mx), // Use current mouse coordinates
					WallY:         float64(my), // Use current mouse coordinates
				}
				// Look up height from catalog
				if ph.scene.catalog != nil {
					if ci := ph.scene.catalog.GetItemByID(holdingProdID); ci != nil {
						pf.Height = ci.Height
					}
				}
				appendLayer(&pf, "", "wall")
				ghostDrawn = true
			}
		} else if sq := iso.GetMouseSquare(); sq != nil && holdingImageBase != "" {
			// Handle regular furniture ghost preview
			wInt := max1(holdingWidth)
			dInt := max1(holdingDepth)
			// Determine ghost direction based on asset availability
			ghostDir := 2 // Default direction for most furniture
			// Preserve direction if holding originated from a placed item move
			if ph.scene != nil && ph.scene.backpackManager != nil {
				if ph.scene.backpackManager.holdingDirSet {
					ghostDir = ph.scene.backpackManager.holdingDir
				}
			}
			if strings.Contains(strings.ToLower(holdingImageBase), "rug") || strings.Contains(strings.ToLower(holdingImageBase), "hardwood") {
				ghostDir = 0 // Rugs use dir 0 assets
			} else if ph.scene != nil && ph.scene.hasOnlyDir4Assets(holdingImageBase, holdingWidth, holdingDepth) {
				ghostDir = 4 // Furniture with only dir 4 assets defaults to direction 4
			}
			if ph.ValidateSnap(sq.GetRow(), sq.GetCol(), wInt, dInt, ghostDir) {
				pf := PlacedFurni{
					Base:       holdingImageBase,
					Row:        sq.GetRow(),
					Col:        sq.GetCol(),
					Width:      wInt,
					Depth:      dInt,
					Dir:        ghostDir,
					GridY:      0,
					IsWallItem: ph.isWallItem(holdingProdID),
				}
				// Preserve pixel offsets during ghost preview when moving an existing placed item
				if ph.scene != nil && ph.scene.backpackManager != nil && ph.scene.backpackManager.movingFromPlaced {
					pf.PixelOffsetX = ph.scene.backpackManager.movedOriginal.PixelOffsetX
					pf.PixelOffsetY = ph.scene.backpackManager.movedOriginal.PixelOffsetY
				}
				pf.ProdID = holdingProdID
				pf.PosID = "ghost-preview"
				// Apply sofa/pinball boosts for ghost preview to match placed behavior
				if ph.scene != nil {
					ph.scene.applySofaSliceBoosts(pf)
					ph.scene.applyPinballSliceBoosts(pf)
					ph.scene.applyVanillaCokeTableSliceBoosts(pf)
				}
				// Adjust ghost origin to match placement logic
				if strings.Contains(strings.ToLower(pf.Base), "rug") || strings.Contains(strings.ToLower(pf.Base), "hardwood") {
					pf.Col = pf.Col - (max1(pf.Width) - 1)
				} else if pf.Width == 2 && pf.Depth == 2 {
					pf.Col = pf.Col - 1
				}
				// Tentative stack level/height for ghost
				pf.GridY = ph.computeStackLevel(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
				if ph.scene.catalog != nil {
					if ci := ph.scene.catalog.GetItemByID(holdingProdID); ci != nil {
						pf.Height = ci.Height
					}
				}
				if pf.IsWallItem {
					appendLayer(&pf, "", "wall")
				} else {
					if !ph.scene.isRugFurniture(pf) {
						appendLayer(&pf, pf.Base+"_sd", "sd")
					}
					aw := wInt
					ad := dInt
					if aw < ad {
						aw, ad = ad, aw
					}
					w := strconv.Itoa(aw)
					d := strconv.Itoa(ad)
					dirS := strconv.Itoa(pf.Dir)
					for _, o := range []string{"a", "b", "c", "d"} {
						name := pf.Base + "_" + o + "_0_" + w + "_" + d + "_" + dirS + "_0"
						appendLayer(&pf, name, o)
					}
				}
				ghostDrawn = true
			}
		}
	}
	return draws, ghostDrawn
}

// HitTestAt performs pixel-accurate, depth-aware hit detection over furniture and wall items.
// It returns the top-most PlacedFurni under the given screen coordinates.
func (ph *PlacementHelper) HitTestAt(mx, my int) (*PlacedFurni, bool) {
	if ph == nil || ph.scene == nil {
		return nil, false
	}
	// Build draws without relying on on-screen rendering; exclude ghost by filtering later
	draws, _ := ph.BuildDrawsSimple()
	if len(draws) == 0 {
		return nil, false
	}
	// Filter out ghost preview and shadow layers; we only want furniture layers and wall items
	filtered := make([]layerDraw, 0, len(draws))
	for _, d := range draws {
		if d.pf == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(d.pf.PosID), "ghost-preview") {
			continue
		}
		if d.order == "sd" {
			continue
		}
		// Keep a/b/c/d and wall
		filtered = append(filtered, d)
	}
	if len(filtered) == 0 {
		return nil, false
	}
	// Sort by depth ascending (back to front) then iterate from front to back
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].depth < filtered[j].depth })
	for i := len(filtered) - 1; i >= 0; i-- {
		d := filtered[i]
		if d.img == nil {
			continue
		}
		w := d.img.Bounds().Dx()
		h := d.img.Bounds().Dy()
		var localX float64
		if d.flipH {
			// For flipped layers, screenX = (d.x + w) - srcX
			localX = float64(w) - (float64(mx) - d.x)
		} else {
			localX = float64(mx) - d.x
		}
		localY := float64(my) - d.y
		px := int(localX)
		py := int(localY)
		if px < 0 || py < 0 || px >= w || py >= h {
			continue
		}
		_, _, _, a := d.img.At(px, py).RGBA()
		if a>>8 > 0 {
			return d.pf, true
		}
	}
	return nil, false
}

func max1(v int) int {
	if v <= 0 {
		return 1
	}
	return v
}

// getFurnitureDepth calculates the final depth of a furniture piece including all adjustments
func (ph *PlacementHelper) getFurnitureDepth(pf PlacedFurni) int {
	// Calculate base depth using same logic as rendering
	simpleDepth := func(row, col, weight int) int {
		return row*10000 - col*5
	}

	// For avatar depth inheritance, use the furniture's base anchor position
	// (This is different from layer rendering which uses per-layer positions)
	depthRow := pf.Row
	depthCol := pf.Col
	// if pf.Dir == 2 || pf.Dir == 6 {
	// 	depthCol = pf.Col + (pf.Width + 1)
	// } else if pf.Dir == 0 || pf.Dir == 4 {
	// 	depthCol = pf.Col + (pf.Width - 1)
	// }

	// Base depth with furniture layer weight (2 for body layer to match avatar)
	z := simpleDepth(depthRow, depthCol, 2)

	// Apply z-shift from .zshift.txt file (use "a" layer as representative)
	zshift := ph.parseZShift(pf.Base, "a")
	z += zshift

	// Preserve slice boosts for this furniture's posID even while floating
	if ph.scene != nil {
		z = ph.applySliceBoosts(pf, "a", z)
	}

	return z
}

// getAvatarDepthOnFurniture returns the depth of an avatar sitting on the given furniture
// Returns 0 if no avatar is sitting on this furniture
func (ph *PlacementHelper) getAvatarDepthOnFurniture(pf PlacedFurni) int {
	if ph.scene == nil {
		return 0
	}

	// Check if any avatar is sitting on this furniture
	for _, avatar := range ph.scene.avatars {
		if avatar == nil || avatar.Square == nil {
			continue
		}

		row := avatar.Square.GetRow()
		col := avatar.Square.GetCol()

		// Check if avatar is on this furniture
		covered := ph.coverageSquares(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
		for _, sq := range covered {
			if sq != nil && sq.GetRow() == row && sq.GetCol() == col {
				// Avatar is on this furniture, calculate its depth using the same logic as drawing
				simpleDepth := func(row, col, weight int) int {
					return row*10000 - col*5
				}

				// Return avatar body depth (weight 2 for body)
				return simpleDepth(row, col, 2)
			}
		}
	}

	return 0 // No avatar sitting on this furniture
}

// parseZShift reads and parses a z-shift value from a .zshift.txt file
// Returns the z-shift value, or 0 if the file doesn't exist or can't be parsed
func (ph *PlacementHelper) parseZShift(furnitureBase, layer string) int {
	// Check cache first
	cacheKey := furnitureBase + "_" + layer
	if zshift, exists := ph.zshiftCache[cacheKey]; exists {
		return zshift
	}

	// The z-shift files are named with numeric prefixes like "211_sofa_coke_d.zshift.txt"
	// We need to find the file that matches "*_base_layer.zshift.txt" pattern

	// Since we can't use filepath.Glob with our fs package, try common numeric prefixes
	// Based on the files we saw, try a range of common prefixes
	for prefix := 1; prefix <= 999; prefix++ {
		filename := fmt.Sprintf("%d_%s_%s.zshift.txt", prefix, furnitureBase, layer)
		filePath := filepath.Join("Lingo", "cc_furniture[1]", filename)

		content, err := fs.ReadFile(filePath)
		if err == nil {
			// File found, parse the content
			zshiftStr := strings.TrimSpace(string(content))
			zshift, err := strconv.Atoi(zshiftStr)
			if err != nil {
				ph.zshiftCache[cacheKey] = 0 // cache the failure
				return 0
			}

			ph.zshiftCache[cacheKey] = zshift // cache the result
			return zshift
		}
	}

	ph.zshiftCache[cacheKey] = 0 // cache the failure
	return 0
}

// updateSquareWalkability updates the walkability of a square based on all furniture covering it
// A square is walkable based on the topmost furniture layer - rugs underneath blocking furniture don't make it walkable
func (ph *PlacementHelper) updateSquareWalkability(row, col int) {
	if ph.scene == nil || ph.scene.isoScene == nil {
		return
	}

	// Find the topmost furniture on this square to determine walkability
	var topmostFurniture *PlacedFurni
	topmostLevel := -1

	for idx := range ph.scene.placedFurni {
		pf := &ph.scene.placedFurni[idx]
		covered := ph.coverageSquares(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
		for _, sq := range covered {
			if sq != nil && sq.GetRow() == row && sq.GetCol() == col {
				// This furniture covers this square - check if it's the topmost
				if pf.GridY > topmostLevel {
					topmostLevel = pf.GridY
					topmostFurniture = pf
				}
				break
			}
		}
	}

	// Set walkability based on the topmost furniture:
	// - If topmost furniture is seatable or rug: walkable (weight 0)
	// - If topmost furniture is non-seatable blocking: not walkable (weight 10)
	// - If no furniture covers this square: restore original PathMap weight
	if topmostFurniture != nil {
		if ph.scene.isSeatableFurniture(*topmostFurniture) || ph.scene.isRugFurniture(*topmostFurniture) {
			// Seatable or rug furniture makes the square walkable (weight 0 = walkable)
			ph.scene.isoScene.SetSquareWeight(row, col, 0)
		} else {
			// Non-seatable furniture blocks the square (weight 10 = blocked)
			ph.scene.isoScene.SetSquareWeight(row, col, 10)
		}
	} else {
		// No furniture covers this square: restore original PathMap weight
		if square := ph.scene.isoScene.GetSquareByRowCol(row, col); square != nil {
			originalWeight := square.GetOriginalWeight()
			ph.scene.isoScene.SetSquareWeight(row, col, originalWeight)
		}
	}
}

// updateAllSquareWalkability recalculates walkability for all squares that have furniture on them
func (ph *PlacementHelper) updateAllSquareWalkability() {
	if ph.scene == nil || ph.scene.isoScene == nil {
		return
	}

	// Collect all squares that have furniture on them
	affectedSquares := make(map[[2]int]bool)
	for _, pf := range ph.scene.placedFurni {
		covered := ph.coverageSquares(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
		for _, sq := range covered {
			if sq != nil {
				affectedSquares[[2]int{sq.GetRow(), sq.GetCol()}] = true
			}
		}
	}

	// Update walkability for all affected squares
	for coords := range affectedSquares {
		ph.updateSquareWalkability(coords[0], coords[1])
	}
}

// DebugDescribe returns a short string describing current holding state
func (ph *PlacementHelper) DebugDescribe() string {
	if ph.scene.backpackManager == nil {
		return "holding=false (no backpack manager)"
	}
	holdingActive := ph.scene.backpackManager.IsHoldingActive()
	holdingProdID, holdingPosID, holdingImageBase, _, _, _ := ph.scene.backpackManager.GetHoldingInfo()
	return fmt.Sprintf("holding=%t prod=%s pos=%s base=%s", holdingActive, holdingProdID, holdingPosID, holdingImageBase)
}
