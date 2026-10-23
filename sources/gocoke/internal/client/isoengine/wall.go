package isoengine

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/core"
)

// WallTileSprite represents a single wall tile sprite
type WallTileSprite struct {
	// Core properties
	Row       int
	Col       int
	Direction string // "left" or "right"
	IsCorner  bool
	XOffset   int
	YOffset   int

	// Rendering properties
	Image    *ebiten.Image
	Position core.Point2D
	Depth    int
	Visible  bool
	Blend    int // Blend value (0 = transparent, 100 = opaque)

	// Asset information
	MemberName string
	AssetType  string // "color" or "texture"
}

// WallSystem manages all wall tiles in the scene
// Replicates the Lingo Wall.ls behavior with 1:1 parity
type WallSystem struct {
	// Core components
	scene   *IsoScene
	castLib string
	debug   bool

	// Wall tiles
	wallTiles []*WallTileSprite
	assets    map[string]*ebiten.Image

	// Pattern system
	patterns map[string][]WallPattern
}

// WallPattern represents a wall pattern configuration
type WallPattern struct {
	Name         string
	Field        string
	DirtStyle    string
	Color        uint32
	BGColor      uint32
	DirtBlend    int
	Palette      string
	TextureBlend int
}

// NewWallSystem creates a new wall system
func NewWallSystem(scene *IsoScene) *WallSystem {
	return &WallSystem{
		scene:     scene,
		castLib:   "Studio",
		debug:     true,
		wallTiles: make([]*WallTileSprite, 0),
		assets:    make(map[string]*ebiten.Image),
		patterns:  make(map[string][]WallPattern),
	}
}

// Init initializes the wall system from Studio XML data
// Matches Lingo Wall.Init() behavior
func (w *WallSystem) Init(studioXML *StudioXML) error {
	if studioXML == nil {
		return fmt.Errorf("studio XML is nil")
	}

	w.debugLog("Init() starting wall system initialization")

	// Load wall patterns first
	if err := w.loadPatterns(); err != nil {
		log.Printf("Wall: warning - failed to load patterns: %v", err)
		// Continue without patterns
	}

	// Clear existing wall tiles
	w.wallTiles = make([]*WallTileSprite, 0)

	// Process each wall tile from XML
	for _, wallTile := range studioXML.Wall.WallTiles {
		if err := w.createWallTile(wallTile); err != nil {
			log.Printf("Wall: warning - failed to create wall tile at [%d,%d]: %v",
				wallTile.Row, wallTile.Col, err)
			continue
		}
	}

	// Process door mask if it exists
	if studioXML.Door.Row != 0 && studioXML.Door.Col != 0 {
		if err := w.createDoorMask(studioXML.Door); err != nil {
			log.Printf("Wall: warning - failed to create door mask at [%d,%d]: %v",
				studioXML.Door.Row, studioXML.Door.Col, err)
		}
	}

	w.debugLog(fmt.Sprintf("Init() created %d wall tiles", len(w.wallTiles)))

	// Apply default pattern to all tiles
	w.setPattern(1, 1)

	return nil
}

// createWallTile creates a wall tile from XML data
// Matches Lingo Wall.drawWallTile() behavior
func (w *WallSystem) createWallTile(xmlTile WallTile) error {
	// CRITICAL: Apply the same 270° coordinate transformation used for PathMap
	// Studio XML coordinates are in the same system as PathMap and need transformation
	// Original: PathMap col becomes grid row, PathMap row becomes flipped grid col
	gridCols := w.scene.GetGrid().Cols
	mappedRow := xmlTile.Col                // XML col becomes grid row
	mappedCol := gridCols - xmlTile.Row + 1 // XML row becomes flipped grid col

	// Get the square for this wall tile position (using transformed coordinates)
	square := w.scene.GetSquareByRowCol(mappedRow, mappedCol)
	if square == nil {
		return fmt.Errorf("no square found at transformed [%d,%d] (original [%d,%d])",
			mappedRow, mappedCol, xmlTile.Row, xmlTile.Col)
	}

	w.debugLog(fmt.Sprintf("createWallTile() transforming XML[%d,%d] -> Grid[%d,%d]",
		xmlTile.Row, xmlTile.Col, mappedRow, mappedCol))

	// Create wall tiles for both color and texture layers
	// This matches the Lingo behavior of creating two sprites per wall tile

	// Set blend based on whether this is a corner tile
	wallBlend := 100 // Default opaque for regular walls
	if xmlTile.Corner {
		wallBlend = 0 // Make corner walls transparent
	}

	colorTile := &WallTileSprite{
		Row:       mappedRow, // Use transformed coordinates
		Col:       mappedCol, // Use transformed coordinates
		Direction: xmlTile.Dir,
		IsCorner:  xmlTile.Corner,
		XOffset:   xmlTile.XOff,
		YOffset:   xmlTile.YOff,
		AssetType: "color",
		Visible:   true,
		Blend:     wallBlend,
	}

	textureTile := &WallTileSprite{
		Row:       mappedRow, // Use transformed coordinates
		Col:       mappedCol, // Use transformed coordinates
		Direction: xmlTile.Dir,
		IsCorner:  xmlTile.Corner,
		XOffset:   xmlTile.XOff,
		YOffset:   xmlTile.YOff,
		AssetType: "texture",
		Visible:   true,
		Blend:     wallBlend,
	}

	// Set member names based on direction, type, and corner status
	if xmlTile.Corner {
		// Corner wall tiles use different assets
		switch xmlTile.Dir {
		case "right":
			colorTile.MemberName = "wall_corner_1_b_0_3_0"
			textureTile.MemberName = "wall_corner_1_a_0_3_0"
		case "left":
			colorTile.MemberName = "wall_corner_1_d_0_3_0"
			textureTile.MemberName = "wall_corner_1_c_0_3_0"
		default:
			return fmt.Errorf("unknown corner wall direction: %s", xmlTile.Dir)
		}
	} else {
		// Regular wall tiles
		switch xmlTile.Dir {
		case "right":
			colorTile.MemberName = "right_wall_1_b_0_2_0"
			textureTile.MemberName = "right_wall_1_a_0_2_0"
		case "left":
			colorTile.MemberName = "left_wall_1_b_0_0_0"
			textureTile.MemberName = "left_wall_1_a_0_0_0"
		default:
			return fmt.Errorf("unknown wall direction: %s", xmlTile.Dir)
		}
	}

	// Calculate position using bottom-left corner (matches Lingo oLoc = oSquare.oViewPoints.bl)
	viewPoints := square.GetViewPoints()
	basePos := viewPoints.BottomLeft

	// Apply offsets
	colorTile.Position = core.Point2D{
		X: basePos.X + float64(colorTile.XOffset),
		Y: basePos.Y + float64(colorTile.YOffset),
	}
	textureTile.Position = core.Point2D{
		X: basePos.X + float64(textureTile.XOffset),
		Y: basePos.Y + float64(textureTile.YOffset),
	}

	// Calculate depth using layer "h" (which maps to 15) to match avatar depth calculation
	// For left walls, use the square at (row-1, col-1) for depth calculation
	depthSquare := square
	if xmlTile.Dir == "left" {
		if depthSq := w.scene.GetSquareByRowCol(xmlTile.Row-1, xmlTile.Col-1); depthSq != nil {
			depthSquare = depthSq
		}
	}

	// Calculate depth using layer 15 (matches Lingo iDepth = oSquare.getDepthByLayer(15))
	// For left walls, use the square at (row-1, col-1) for depth calculation
	baseDepth := (depthSquare.GetRow() * 100) + depthSquare.GetCol()
	colorTile.Depth = baseDepth + 15   // Layer 15 for walls
	textureTile.Depth = baseDepth + 16 // Texture on top of color

	// Load assets
	if err := w.loadWallAsset(colorTile); err != nil {
		return fmt.Errorf("failed to load color asset: %w", err)
	}
	if err := w.loadWallAsset(textureTile); err != nil {
		return fmt.Errorf("failed to load texture asset: %w", err)
	}

	// Apply registration point offset for proper wall positioning (must be done for each tile)
	w.applyRegistrationPointOffset(colorTile)
	w.applyRegistrationPointOffset(textureTile)

	// Apply z-shift offset for proper wall positioning
	w.applyZShiftOffset(colorTile)
	w.applyZShiftOffset(textureTile)

	// Add to wall tiles list
	w.wallTiles = append(w.wallTiles, colorTile, textureTile)

	// For corner walls, also add the regular wall tiles on top with higher z-order
	if xmlTile.Corner {
		// Calculate the base position again (before registration points were applied)
		viewPoints := square.GetViewPoints()
		basePos := viewPoints.BottomLeft

		regularColorTile := &WallTileSprite{
			Row:       mappedRow,
			Col:       mappedCol,
			Direction: xmlTile.Dir,
			IsCorner:  false, // This is the regular wall part
			XOffset:   xmlTile.XOff,
			YOffset:   xmlTile.YOff,
			AssetType: "color",
			Visible:   true,
			Blend:     100, // Make regular wall tiles on corners transparent too
			Position: core.Point2D{
				X: basePos.X + float64(xmlTile.XOff),
				Y: basePos.Y + float64(xmlTile.YOff),
			},
			Depth: colorTile.Depth + 10, // Higher z-order (on top of corner)
		}

		regularTextureTile := &WallTileSprite{
			Row:       mappedRow,
			Col:       mappedCol,
			Direction: xmlTile.Dir,
			IsCorner:  false, // This is the regular wall part
			XOffset:   xmlTile.XOff,
			YOffset:   xmlTile.YOff,
			AssetType: "texture",
			Visible:   true,
			Blend:     100, // Make regular wall tiles on corners transparent too
			Position: core.Point2D{
				X: basePos.X + float64(xmlTile.XOff),
				Y: basePos.Y + float64(xmlTile.YOff),
			},
			Depth: textureTile.Depth + 10, // Higher z-order (on top of corner)
		}

		// Set regular wall member names
		switch xmlTile.Dir {
		case "right":
			regularColorTile.MemberName = "right_wall_1_b_0_2_0"
			regularTextureTile.MemberName = "right_wall_1_a_0_2_0"
		case "left":
			regularColorTile.MemberName = "left_wall_1_b_0_0_0"
			regularTextureTile.MemberName = "left_wall_1_a_0_0_0"
		}

		// Load assets for regular wall tiles
		if err := w.loadWallAsset(regularColorTile); err != nil {
			return fmt.Errorf("failed to load regular color asset for corner: %w", err)
		}
		if err := w.loadWallAsset(regularTextureTile); err != nil {
			return fmt.Errorf("failed to load regular texture asset for corner: %w", err)
		}

		// Apply registration point and z-shift offsets
		w.applyRegistrationPointOffset(regularColorTile)
		w.applyRegistrationPointOffset(regularTextureTile)
		w.applyZShiftOffset(regularColorTile)
		w.applyZShiftOffset(regularTextureTile)

		// Add regular wall tiles on top of corner
		w.wallTiles = append(w.wallTiles, regularColorTile, regularTextureTile)

		w.debugLog(fmt.Sprintf("createWallTile() added regular wall tiles on top of corner at XML[%d,%d]", xmlTile.Row, xmlTile.Col))
	}

	w.debugLog(fmt.Sprintf("createWallTile() %s wall XML[%d,%d]->Grid[%d,%d]: final pos=(%.1f,%.1f)",
		xmlTile.Dir, xmlTile.Row, xmlTile.Col, mappedRow, mappedCol, colorTile.Position.X, colorTile.Position.Y))

	return nil
}

// loadWallAsset loads the image asset for a wall tile and applies proper registration point offset
func (w *WallSystem) loadWallAsset(tile *WallTileSprite) error {
	// Check if already loaded
	if _, exists := w.assets[tile.MemberName]; exists {
		tile.Image = w.assets[tile.MemberName]
		return nil
	}

	// Resolve asset path via Members.csv (works in WASM; avoids filesystem globbing)
	ccStudioDir := filepath.Join("Lingo", "cc_studio")
	mm, err := lingoassets.LoadMemberMapCached(ccStudioDir)
	if err != nil {
		return fmt.Errorf("failed to load member map for %s: %w", ccStudioDir, err)
	}
	imagePath, ok := mm.PathFor(tile.MemberName)
	if !ok {
		// Fallback to unnumbered path for development gaps
		imagePath = filepath.Join(ccStudioDir, tile.MemberName+".png")
	}

	// Load the image
	img, err := lingoassets.LoadImage(imagePath)
	if err != nil {
		return fmt.Errorf("failed to load wall asset %s: %w", imagePath, err)
	}

	// Cache and assign
	w.assets[tile.MemberName] = img
	tile.Image = img

	w.debugLog(fmt.Sprintf("loadWallAsset() loaded %s from %s", tile.MemberName, imagePath))
	return nil
}

// loadDoorMaskAsset loads the base door mask texture file (like wall tiles do)
func (w *WallSystem) loadDoorMaskAsset(tile *WallTileSprite) error {
	// Use the base door mask texture as specified
	memberName := "15_wall_doormask_1_a_0_2_0"

	// Check if already loaded
	if _, exists := w.assets[memberName]; exists {
		tile.Image = w.assets[memberName]
		return nil
	}

	// Load the base door mask texture from cc_studio
	assetPath := filepath.Join("Lingo", "cc_studio", memberName+".png")

	// Load the image
	img, err := lingoassets.LoadImage(assetPath)
	if err != nil {
		return fmt.Errorf("failed to load door mask asset %s: %w", assetPath, err)
	}

	// Cache and assign
	w.assets[memberName] = img
	tile.Image = img

	w.debugLog(fmt.Sprintf("loadDoorMaskAsset() loaded base door mask from %s", assetPath))
	return nil
}

// Draw renders all wall tiles (single-pass). Prefer the two-pass helpers for correct avatar occlusion.
func (w *WallSystem) Draw(screen *ebiten.Image) {
	w.drawFiltered(screen, func(t *WallTileSprite) bool { return true })
}

// DrawNonDoorTiles renders all wall tiles except door masks. Call this BEFORE drawing avatars.
func (w *WallSystem) DrawNonDoorTiles(screen *ebiten.Image) {
	w.drawFiltered(screen, func(t *WallTileSprite) bool { return t.Direction != "door" })
}

// DrawDoorTiles renders door mask tiles except overlays. Call this AFTER drawing avatars.
func (w *WallSystem) DrawDoorTiles(screen *ebiten.Image) {
	w.drawFiltered(screen, func(t *WallTileSprite) bool { return t.Direction == "door" && t.AssetType != "overlay" })
}

// DrawDoorOverlayTiles renders only door overlay tiles (e.g., 1_d variant).
// Render these after avatars but before non-door walls if you need them beneath wall textures.
func (w *WallSystem) DrawDoorOverlayTiles(screen *ebiten.Image) {
	w.drawFiltered(screen, func(t *WallTileSprite) bool { return t.Direction == "door" && t.AssetType == "overlay" })
}

// drawFiltered sorts and draws tiles that satisfy the predicate.
func (w *WallSystem) drawFiltered(screen *ebiten.Image, pred func(*WallTileSprite) bool) {
	// Sort tiles by depth (lowest first for proper layering)
	w.sortTilesByDepth()

	// Draw each visible tile matching the predicate
	for _, tile := range w.wallTiles {
		if !pred(tile) {
			continue
		}
		if !tile.Visible {
			continue
		}

		// Skip drawing if blend is 0 (transparent)
		if tile.Blend == 0 {
			continue
		}

		// Build an element ID that matches console assignments (used by pattern manager)
		var elementID string
		if tile.Direction == "left" {
			if tile.AssetType == "texture" {
				elementID = "walltile_left_texture"
			} else {
				elementID = "walltile_left_1"
			}
		} else if tile.Direction == "door" {
			// Door masks always use texture type for pattern application
			elementID = "walltile_door_texture"
		} else { // default right
			if tile.AssetType == "texture" {
				elementID = "walltile_right_texture"
			} else {
				elementID = "walltile_right_1"
			}
		}

		// For door overlays (e.g., wall_doormask_*_d variants) draw directly without pattern processing
		if tile.Direction == "door" && tile.AssetType == "overlay" {
			if tile.Image != nil {
				opts := &ebiten.DrawImageOptions{}
				opts.GeoM.Translate(tile.Position.X, tile.Position.Y)
				if tile.Blend < 100 {
					alpha := float64(tile.Blend) / 100.0
					opts.ColorM.Scale(1, 1, 1, alpha)
				}
				screen.DrawImage(tile.Image, opts)
			}
			continue
		}

		// Defer to pattern manager to draw with two-tone + palette if there is an assignment
		if w.scene != nil && w.scene.sceneOwner != nil && w.scene.sceneOwner.patternMgr.HasPatternForElement(elementID) {
			// Important: do not prune the base image here; it is required as the mask/base for rendering
			w.scene.sceneOwner.patternMgr.ApplyPatternToElement(screen, elementID, tile.Image, tile.Position.X, tile.Position.Y)
			continue
		}

		// Fallback: draw normally (only if we have a base image)
		if tile.Image == nil {
			continue
		}
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(tile.Position.X, tile.Position.Y)
		if tile.Blend < 100 {
			alpha := float64(tile.Blend) / 100.0
			opts.ColorM.Scale(1, 1, 1, alpha)
		}
		screen.DrawImage(tile.Image, opts)
	}
}

// sortTilesByDepth sorts wall tiles by depth for proper rendering order
func (w *WallSystem) sortTilesByDepth() {
	// Simple bubble sort by depth (lowest first)
	for i := 0; i < len(w.wallTiles)-1; i++ {
		for j := 0; j < len(w.wallTiles)-i-1; j++ {
			if w.wallTiles[j].Depth > w.wallTiles[j+1].Depth {
				w.wallTiles[j], w.wallTiles[j+1] = w.wallTiles[j+1], w.wallTiles[j]
			}
		}
	}
}

// loadPatterns loads wall patterns from text files
// Matches Lingo Wall.loadPatterns() behavior
func (w *WallSystem) loadPatterns() error {
	// This would load from wallpattern_patterns_index.txt and related files
	// For now, create a simple default pattern
	defaultPattern := WallPattern{
		Name:         "wall_tile",
		Field:        "wallpattern_tile",
		DirtStyle:    "1",
		Color:        0xFFFFFFFF, // White
		BGColor:      0xFF000000, // Black
		DirtBlend:    100,
		Palette:      "default",
		TextureBlend: 100,
	}

	w.patterns["wall_tile"] = []WallPattern{defaultPattern}
	w.debugLog("loadPatterns() loaded default wall pattern")
	return nil
}

// setPattern applies a pattern to all wall tiles
// Matches Lingo Wall.setPattern() behavior
func (w *WallSystem) setPattern(patternIndex, colorIndex int) {
	if len(w.patterns) == 0 {
		return
	}

	// For now, just log the pattern application
	// Full pattern system would modify sprite colors and textures
	w.debugLog(fmt.Sprintf("setPattern() applied pattern %d color %d to %d tiles",
		patternIndex, colorIndex, len(w.wallTiles)))
}

// ToggleDisplay toggles visibility of all wall tiles
// Matches Lingo Wall.toggleDisplay() behavior
func (w *WallSystem) ToggleDisplay() {
	for _, tile := range w.wallTiles {
		tile.Visible = !tile.Visible
	}
	w.debugLog(fmt.Sprintf("ToggleDisplay() toggled visibility for %d tiles", len(w.wallTiles)))
}

// HideDisplay hides all wall tiles
// Matches Lingo Wall.hideDisplay() behavior
func (w *WallSystem) HideDisplay() {
	for _, tile := range w.wallTiles {
		tile.Visible = false
	}
	w.debugLog(fmt.Sprintf("HideDisplay() hid %d tiles", len(w.wallTiles)))
}

// GetSpriteUnderPoint finds a wall tile at the given screen coordinates
// Matches Lingo Wall.getSpriteUnderPoint() behavior
func (w *WallSystem) GetSpriteUnderPoint(x, y float64) *WallTileSprite {
	for _, tile := range w.wallTiles {
		if !tile.Visible || tile.Image == nil {
			continue
		}

		bounds := tile.Image.Bounds()
		tileRect := core.Rect{
			X:      tile.Position.X,
			Y:      tile.Position.Y,
			Width:  float64(bounds.Dx()),
			Height: float64(bounds.Dy()),
		}

		if x >= tileRect.X && x <= tileRect.X+tileRect.Width &&
			y >= tileRect.Y && y <= tileRect.Y+tileRect.Height {
			return tile
		}
	}
	return nil
}

// GetWallTiles returns all wall tiles (for debugging)
func (w *WallSystem) GetWallTiles() []*WallTileSprite {
	return w.wallTiles
}

// Destroy cleans up the wall system
// Matches Lingo Wall.destroy() behavior
func (w *WallSystem) Destroy() {
	w.wallTiles = nil
	w.assets = nil
	w.patterns = nil
	w.debugLog("Destroy() cleaned up wall system")
}

// debugLog logs debug messages if debug is enabled
func (w *WallSystem) debugLog(message string) {
	if w.debug {
		log.Printf("Wall: %s", message)
	}
}

// applyRegistrationPointOffset applies the correct registration point offset to position the wall tile
func (w *WallSystem) applyRegistrationPointOffset(tile *WallTileSprite) {
	// Registration points from Members.csv define where the "anchor" of the image is
	// When Lingo positions a sprite, the position refers to where the registration point goes
	// To simulate this in Go, we need to subtract the registration point from our draw position

	var regX, regY float64
	switch tile.MemberName {
	case "left_wall_1_b_0_0_0", "left_wall_1_a_0_0_0":
		regX, regY = 0, 131 // From Members.csv: (0, 0)
	case "left_wall_2_b_0_0_0", "left_wall_2_a_0_0_0":
		regX, regY = 0, 131 // From Members.csv: (0, 0)
	case "right_wall_1_b_0_2_0", "right_wall_1_a_0_2_0":
		regX, regY = 36, 131 // From Members.csv: (0, 115)
	case "right_wall_2_b_0_2_0", "right_wall_2_a_0_2_0":
		regX, regY = 36, 131 // From Members.csv: (0, 115)
	case "wall_corner_1_b_0_3_0", "wall_corner_1_a_0_3_0":
		regX, regY = 36, 131 // From Members.csv: (0, 115)
	case "wall_corner_2_b_0_3_0", "wall_corner_2_a_0_3_0":
		regX, regY = 36, 131 // From Members.csv: (0, 115)
	case "wall_corner_1_d_0_3_0", "wall_corner_1_c_0_3_0":
		regX, regY = 0, 135 // From Members.csv: (-36, 114)
	case "wall_corner_2_d_0_3_0", "wall_corner_2_c_0_3_0":
		regX, regY = 0, 135 // From Members.csv: (-36, 114)
	// Door mask registration points from Members.csv
	case "wall_doormask_1_b_0_2_0", "wall_doormask_1_a_0_2_0":
		regX, regY = 37, 128 // From Members.csv: (0, 115)
	case "wall_doormask_1_c_0_2_0":
		regX, regY = 3700, 130 // From Members.csv: (0, 128)
	case "wall_doormask_1_d_0_2_0":
		regX, regY = 2, 124 // From Members.csv: (-35, 110)
	case "wall_doormask_2_b_0_2_0":
		regX, regY = 37, 128 // From Members.csv: (0, 115)
	// Exit shape registration points from Members.csv
	case "studio_exitshape":
		regX, regY = 36, 130 // From Members.csv: (0, 115)
	// Door floor tile registration points from Members.csv
	case "studiofloor_door_1":
		regX, regY = 0, 19 // From Members.csv: (0, 19)
	// Base door mask texture registration point
	case "15_wall_doormask_1_a_0_2_0":
		regX, regY = 37, 130 // Door mask registration point
	}

	// Fallback: if no specific case above set a reg point, use standard doormask anchor
	if regX == 0 && regY == 0 && strings.Contains(strings.ToLower(tile.MemberName), "doormask") {
		regX, regY = 37, 131
	}

	// Adjust position: subtract registration point offset
	// This makes our draw position equivalent to where Lingo would place the registration point
	tile.Position.X -= regX
	tile.Position.Y -= regY

	w.debugLog(fmt.Sprintf("applyRegistrationPointOffset() %s: before=(%.1f,%.1f) reg=(%.0f,%.0f) final=(%.1f,%.1f)",
		tile.MemberName, tile.Position.X+regX, tile.Position.Y+regY, regX, regY, tile.Position.X, tile.Position.Y))
}

// applyZShiftOffset applies the Z-shift offset from .zshift.txt files to wall tiles
func (w *WallSystem) applyZShiftOffset(tile *WallTileSprite) {
	// ZShift values from .zshift.txt files - these adjust the Z-ORDER/DEPTH for layer ordering
	// All the "1_a" variants (texture layers) use -49 depth offset
	// All the "1_b" variants (color layers) use -50 depth offset

	var zShift int
	switch {
	case tile.MemberName == "left_wall_1_a_0_0_0" ||
		tile.MemberName == "right_wall_1_a_0_2_0" ||
		tile.MemberName == "wall_corner_1_a_0_3_0" ||
		tile.MemberName == "wall_corner_1_c_0_3_0":
		zShift = -49 // All 1_a variants
	case tile.MemberName == "left_wall_1_b_0_0_0" ||
		tile.MemberName == "right_wall_1_b_0_2_0" ||
		tile.MemberName == "wall_corner_1_b_0_3_0" ||
		tile.MemberName == "wall_corner_1_d_0_3_0":
		zShift = -50 // All 1_b variants
	// Door mask z-shift values from .zshift.txt files
	case tile.MemberName == "wall_doormask_1_b_0_2_0":
		zShift = 937 // From wall_doormask_1_b.zshift.txt
	case tile.MemberName == "wall_doormask_1_a_0_2_0":
		zShift = 950 // From wall_doormask_1_a.zshift.txt
	case tile.MemberName == "wall_doormask_1_c_0_2_0":
		zShift = 900 // From wall_doormask_1_c.zshift.txt
	case tile.MemberName == "wall_doormask_1_d_0_2_0":
		zShift = 335 // From wall_doormask_1_d.zshift.txt
	default:
		zShift = 0
	}

	// Apply zshift to DEPTH (z-order), not Y position
	tile.Depth += zShift

	w.debugLog(fmt.Sprintf("applyZShiftOffset() %s: applied zshift=%d, final depth=%d",
		tile.MemberName, zShift, tile.Depth))
}

// createDoorMask creates door mask tiles from Door XML data
func (w *WallSystem) createDoorMask(door Door) error {
	w.debugLog(fmt.Sprintf("createDoorMask() creating door mask at XML[%d,%d]", door.Row, door.Col))

	// Apply the same 270° coordinate transformation used for walls
	gridCols := w.scene.GetGrid().Cols
	mappedRow := door.Col                // XML col becomes grid row
	mappedCol := gridCols - door.Row + 1 // XML row becomes flipped grid col

	// Get the square for this door position (using transformed coordinates)
	square := w.scene.GetSquareByRowCol(mappedRow, mappedCol)
	if square == nil {
		return fmt.Errorf("no square found at transformed coordinates [%d,%d]", mappedRow, mappedCol)
	}

	// Calculate position using bottom-left corner (matches Lingo behavior)
	viewPoints := square.GetViewPoints()
	basePos := viewPoints.BottomLeft

	// Create door mask tile using the pattern system (like wall tiles)
	// Use element ID that the pattern system can recognize
	doorMaskTile := &WallTileSprite{
		Row:       mappedRow,
		Col:       mappedCol,
		Direction: "door", // Special direction for door masks
		IsCorner:  false,
		XOffset:   door.DoorMask.XOff,
		YOffset:   door.DoorMask.YOff,
		AssetType: "texture", // Use texture type like wall tiles
		// Use the actual asset member name so registration/zshift logic works
		MemberName: "15_wall_doormask_1_a_0_2_0",
		Visible:    true,
		Blend:      100, // Visible door mask
		Position: core.Point2D{
			X: basePos.X + float64(door.DoorMask.XOff),
			Y: basePos.Y + float64(door.DoorMask.YOff),
		},
	}

	// Calculate depth - door masks should be above walls
	baseDepth := (square.GetRow() * 100) + square.GetCol()
	doorMaskTile.Depth = baseDepth + 20 // Layer 20+ for door masks

	// Load the base door mask texture (tile_doormask.png)
	if err := w.loadDoorMaskAsset(doorMaskTile); err != nil {
		log.Printf("Wall: ERROR - failed to load door mask asset: %v", err)
		log.Printf("Wall: Door mask will be added without base texture - pattern system will handle loading")
	}

	// Also add a right-side door overlay (variant 'd') so avatars are properly obscured on entry
	overlayTile := &WallTileSprite{
		Row:        mappedRow,
		Col:        mappedCol,
		Direction:  "door", // keep as door so it draws in the door pass (after avatars)
		IsCorner:   false,
		XOffset:    door.DoorMask.XOff,
		YOffset:    door.DoorMask.YOff,
		AssetType:  "overlay", // special-case to bypass pattern processing
		MemberName: "wall_doormask_1_d_0_2_0",
		Visible:    true,
		Blend:      100,
		Position: core.Point2D{
			X: basePos.X + float64(door.DoorMask.XOff),
			Y: basePos.Y + float64(door.DoorMask.YOff),
		},
	}
	// Place above walls; zshift will refine
	baseDepth = (square.GetRow() * 100) + square.GetCol()
	overlayTile.Depth = baseDepth + 20
	if err := w.loadWallAsset(overlayTile); err != nil {
		log.Printf("Wall: warning - failed to load door overlay asset %s: %v", overlayTile.MemberName, err)
	} else {
		w.applyRegistrationPointOffset(overlayTile)
		w.applyZShiftOffset(overlayTile)
		w.wallTiles = append(w.wallTiles, overlayTile)
		w.debugLog(fmt.Sprintf("createDoorMask() created door overlay %s at pos=(%.1f,%.1f) depth=%d",
			overlayTile.MemberName, overlayTile.Position.X, overlayTile.Position.Y, overlayTile.Depth))
	}

	// Always add the door mask tile - pattern system will handle texture loading
	// Apply registration point and z-shift offsets
	w.applyRegistrationPointOffset(doorMaskTile)
	w.applyZShiftOffset(doorMaskTile)

	// Add to wall tiles list
	w.wallTiles = append(w.wallTiles, doorMaskTile)

	w.debugLog(fmt.Sprintf("createDoorMask() created %s at pos=(%.1f,%.1f) depth=%d",
		doorMaskTile.MemberName, doorMaskTile.Position.X, doorMaskTile.Position.Y, doorMaskTile.Depth))

	// Create the door floor tile if it exists
	if door.FloorTile.Member != "" {
		doorFloorTile := &WallTileSprite{
			Row:        mappedRow,
			Col:        mappedCol,
			Direction:  "doorfloor", // Special direction for door floor tiles
			IsCorner:   false,
			XOffset:    0, // Door floor tile doesn't have explicit offsets in XML
			YOffset:    0,
			AssetType:  "doorfloor",
			MemberName: door.FloorTile.Member, // "studiofloor_door_1"
			Visible:    false,
			Blend:      door.FloorTile.Blend, // Use blend from XML
			Position: core.Point2D{
				X: basePos.X,
				Y: basePos.Y,
			},
		}

		// Calculate depth - door floor tiles should be below walls (like regular floor)
		baseDepth := (square.GetRow() * 100) + square.GetCol()
		doorFloorTile.Depth = baseDepth + 5 // Layer 5 for door floor tiles (below walls)

		// Load asset
		if err := w.loadWallAsset(doorFloorTile); err != nil {
			log.Printf("Wall: warning - failed to load door floor asset %s: %v", door.FloorTile.Member, err)
		} else {
			// Apply registration point and z-shift offsets
			w.applyRegistrationPointOffset(doorFloorTile)
			w.applyZShiftOffset(doorFloorTile)

			// Add to wall tiles list
			w.wallTiles = append(w.wallTiles, doorFloorTile)

			w.debugLog(fmt.Sprintf("createDoorMask() created door floor tile %s at pos=(%.1f,%.1f) depth=%d",
				doorFloorTile.MemberName, doorFloorTile.Position.X, doorFloorTile.Position.Y, doorFloorTile.Depth))
		}
	}

	// Also create the Exit shape if it exists
	if door.Exit.Member != "" {
		exitTile := &WallTileSprite{
			Row:        mappedRow,
			Col:        mappedCol,
			Direction:  "exit", // Special direction for exit shapes
			IsCorner:   false,
			XOffset:    0, // Exit doesn't have explicit offsets in XML
			YOffset:    0,
			AssetType:  "exit",
			MemberName: door.Exit.Member, // "studio_exitshape"
			Visible:    false,
			Blend:      0, // Use blend from XML (0 = transparent)
			Position: core.Point2D{
				X: basePos.X,
				Y: basePos.Y,
			},
		}

		// Calculate depth - exit shapes should be above door masks
		baseDepth := (square.GetRow() * 100) + square.GetCol()
		exitTile.Depth = baseDepth + 1000 // Layer 1000+ for exit shapes (above door masks which are ~950+)

		// Load asset
		if err := w.loadWallAsset(exitTile); err != nil {
			log.Printf("Wall: warning - failed to load exit asset %s: %v", door.Exit.Member, err)
		} else {
			// Apply registration point and z-shift offsets
			w.applyRegistrationPointOffset(exitTile)
			w.applyZShiftOffset(exitTile)

			// Add to wall tiles list
			w.wallTiles = append(w.wallTiles, exitTile)

			w.debugLog(fmt.Sprintf("createDoorMask() created exit shape %s at pos=(%.1f,%.1f) depth=%d",
				exitTile.MemberName, exitTile.Position.X, exitTile.Position.Y, exitTile.Depth))
		}
	}

	w.debugLog("createDoorMask() completed door mask and exit shape creation")
	return nil
}
