package isoengine

import (
	"fmt"
	"image/color"
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"

	"gocoke/internal/client/ui"
)

// IsoGrid manages the isometric grid system
// Replicates the Lingo IsoGrid.ls behavior with 1:1 parity
type IsoGrid struct {
	// Grid dimensions
	Rows   int
	Cols   int
	SqSize int

	// Grid positioning
	XOff int // X offset for screen positioning
	YOff int // Y offset for screen positioning

	// Calculated dimensions
	GridWidth      int // Total grid width in grid units
	GridDepth      int // Total grid depth in grid units
	GridViewWidth  int // Grid width in screen pixels
	GridViewHeight int // Grid height in screen pixels

	// Grid data
	Squares [][]*IsoSquare // 2D array of squares [row][col]

	// Components
	constants *IsoConstants
	math2d    *Math2d

	// Debug and display
	GridSprite    *ebiten.Image // Debug grid overlay
	GridDrawn     bool          // Whether debug grid has been drawn
	ShowDebugGrid bool          // Whether to show debug grid overlay
	DebugMode     int           // 0=off, 1=dots, 2=tiles, 3=tiles+delineation
}

// NewIsoGrid creates a new IsoGrid instance
// Matches Lingo IsoGrid.new() behavior
func NewIsoGrid(rows, cols, sqSize, xOff, yOff int, constants *IsoConstants) *IsoGrid {
	grid := &IsoGrid{
		Rows:          rows,
		Cols:          cols,
		SqSize:        sqSize,
		XOff:          xOff,
		YOff:          yOff,
		constants:     constants,
		math2d:        NewMath2d(constants),
		GridDrawn:     false,
		ShowDebugGrid: false,
	}

	// Initialize the grid
	grid.Init()

	log.Printf("IsoGrid: created %dx%d grid with square size %d at offset (%d,%d)",
		rows, cols, sqSize, xOff, yOff)

	return grid
}

// Init initializes the grid system
// Matches Lingo IsoGrid.Init() behavior
func (g *IsoGrid) Init() {
	// Calculate grid dimensions
	g.GridWidth = g.Cols * g.SqSize
	g.GridDepth = g.Rows * g.SqSize

	// Create squares
	g.createSquares()

	// Perform diagonal base depth calculations like Lingo doDepthCalcs()
	g.doDepthCalcs()

	// Calculate view dimensions
	g.calcGridViewWidth()
	g.calcGridViewHeight()

	log.Printf("IsoGrid: initialized grid - GridWidth:%d, GridDepth:%d, ViewWidth:%d, ViewHeight:%d",
		g.GridWidth, g.GridDepth, g.GridViewWidth, g.GridViewHeight)
}

// createSquares creates the 2D array of IsoSquare objects
// Matches Lingo IsoGrid.createSquares() behavior
func (g *IsoGrid) createSquares() {
	g.Squares = make([][]*IsoSquare, g.Rows)

	for row := 0; row < g.Rows; row++ {
		g.Squares[row] = make([]*IsoSquare, g.Cols)
		for col := 0; col < g.Cols; col++ {
			// Create square with 1-based indexing (row+1, col+1)
			square := NewIsoSquare(g, row+1, col+1, 0, 0, "")
			g.Squares[row][col] = square
		}
	}

	log.Printf("IsoGrid: created %d squares (%dx%d)", g.Rows*g.Cols, g.Rows, g.Cols)
}

// calcGridViewWidth calculates the total width of the grid in screen pixels
// Matches Lingo IsoGrid.calcGridViewWidth() behavior
func (g *IsoGrid) calcGridViewWidth() {
	if g.Rows == 0 || g.Cols == 0 {
		g.GridViewWidth = 0
		return
	}

	// Get leftmost and rightmost squares
	leftSquare := g.Squares[g.Rows-1][0]  // Bottom-left square
	rightSquare := g.Squares[0][g.Cols-1] // Top-right square

	// Calculate width from leftmost to rightmost points
	leftX := leftSquare.ViewPoints.BottomLeft.X
	rightX := rightSquare.ViewPoints.TopRight.X
	width := int(rightX - leftX)

	// Ensure positive width
	if width <= 0 {
		width = g.Cols * g.SqSize // Fallback calculation
	}

	g.GridViewWidth = width
}

// calcGridViewHeight calculates the total height of the grid in screen pixels
// Matches Lingo IsoGrid.calcGridViewHeight() behavior
func (g *IsoGrid) calcGridViewHeight() {
	if g.Rows == 0 || g.Cols == 0 {
		g.GridViewHeight = 0
		return
	}

	// Get topmost and bottommost squares
	topSquare := g.Squares[0][0]                  // Top-left square
	bottomSquare := g.Squares[g.Rows-1][g.Cols-1] // Bottom-right square

	// Calculate height from topmost to bottommost points
	topY := topSquare.ViewPoints.TopLeft.Y
	bottomY := bottomSquare.ViewPoints.BottomRight.Y
	height := int(bottomY - topY)

	// Ensure positive height
	if height <= 0 {
		height = g.Rows * g.SqSize / 2 // Fallback calculation (isometric projection reduces height)
	}

	g.GridViewHeight = height
}

// GetSquareByRowCol returns the square at the given row/col (1-based indexing)
// Matches Lingo IsoGrid.getSquareByRowCol() behavior
func (g *IsoGrid) GetSquareByRowCol(row, col int) *IsoSquare {
	// Convert to 0-based indexing and validate bounds
	rowIndex := row - 1
	colIndex := col - 1

	if rowIndex < 0 || rowIndex >= g.Rows || colIndex < 0 || colIndex >= g.Cols {
		return nil
	}

	return g.Squares[rowIndex][colIndex]
}

// GetSquareByScreenPoint returns the square at the given screen coordinates
// This is the key function for mouse-to-tile conversion
func (g *IsoGrid) GetSquareByScreenPoint(screenX, screenY int) *IsoSquare {
	// Convert screen coordinates to grid row/col
	row, col, valid := g.math2d.ScreenToGridCoordinates(screenX, screenY, g.XOff, g.YOff, g.SqSize)

	if !valid {
		return nil
	}

	return g.GetSquareByRowCol(row, col)
}

// GetSquareByXZ returns the square at the given grid coordinates
// Matches Lingo IsoGrid.getSquareByXZ() behavior
func (g *IsoGrid) GetSquareByXZ(gridX, gridZ float64) *IsoSquare {
	// Convert grid coordinates to row/col
	col := int(gridX/float64(g.SqSize)) + 1  // 1-based indexing
	row := int(-gridZ/float64(g.SqSize)) + 1 // 1-based indexing, Z is negative

	return g.GetSquareByRowCol(row, col)
}

// CenterGrid centers the grid on the screen
// Matches Lingo IsoGrid.centerGrid() behavior
func (g *IsoGrid) CenterGrid(screenWidth, screenHeight int) {
	screenCenterX := screenWidth / 2
	screenCenterY := screenHeight / 2

	// Position grid so its center aligns with screen center
	newX := screenCenterX
	newY := screenCenterY - g.GridViewHeight/2

	g.XOff = newX
	g.YOff = newY

	// Recalculate all squares with new offset
	g.recalculateSquares()

	log.Printf("IsoGrid: centered grid at (%d,%d)", g.XOff, g.YOff)
}

// recalculateSquares recalculates all square positions after offset changes
func (g *IsoGrid) recalculateSquares() {
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			if g.Squares[row][col] != nil {
				g.Squares[row][col].doGridCalc()
			}
		}
	}
}

// SetSquareWeight sets the pathfinding weight for a square
// Used for loading PathMap data
func (g *IsoGrid) SetSquareWeight(row, col, weight int) {
	square := g.GetSquareByRowCol(row, col)
	if square != nil {
		square.SetWeight(weight)
	}
}

// SetSquareOriginalWeight sets the original pathfinding weight for a square
// Used for storing PathMap data that can be restored later
func (g *IsoGrid) SetSquareOriginalWeight(row, col, weight int) {
	square := g.GetSquareByRowCol(row, col)
	if square != nil {
		square.SetOriginalWeight(weight)
	}
}

// GetWalkableSquares returns all squares that can be walked on
func (g *IsoGrid) GetWalkableSquares() []*IsoSquare {
	var walkable []*IsoSquare

	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			square := g.Squares[row][col]
			if square != nil && square.IsWalkable() {
				walkable = append(walkable, square)
			}
		}
	}

	return walkable
}

// EnableDebugGrid enables/disables the debug grid overlay
func (g *IsoGrid) EnableDebugGrid(enabled bool) {
	g.ShowDebugGrid = enabled
	if enabled {
		if g.DebugMode == 0 {
			g.DebugMode = 1 // default to dots when enabling
		}
		if !g.GridDrawn {
			g.createDebugGridImage()
		}
	} else {
		g.DebugMode = 0
	}
}

// CycleDebugMode cycles through 0=off, 1=dots, 2=tiles
func (g *IsoGrid) CycleDebugMode() {
	g.DebugMode = (g.DebugMode + 1) % 4
	g.ShowDebugGrid = g.DebugMode != 0
}

// createDebugGridImage creates a debug overlay showing the grid
func (g *IsoGrid) createDebugGridImage() {
	// Ensure we have valid dimensions
	width := g.GridViewWidth
	height := g.GridViewHeight

	if width <= 0 || height <= 0 {
		// Use fallback dimensions if calculations failed
		width = 800
		height = 600
		log.Printf("IsoGrid: using fallback dimensions for debug grid (%dx%d)", width, height)
	}

	// Create an image for the debug grid
	g.GridSprite = ebiten.NewImage(width, height)

	// Draw grid lines (simplified - in full implementation would draw diamond shapes)
	// For now, just mark that we've created the debug image
	g.GridDrawn = true

	log.Printf("IsoGrid: created debug grid image (%dx%d)", width, height)
}

// DrawDebugGrid draws the debug grid overlay if enabled
func (g *IsoGrid) DrawDebugGrid(screen *ebiten.Image) {
	if !g.ShowDebugGrid {
		return
	}

	switch g.DebugMode {
	case 1:
		// Dots at tile centers
		g.drawGridSquares(screen)
	case 2:
		// Translucent full-tile overlays
		g.drawGridOverlays(screen)
	case 3:
		// Translucent overlays with distinct tile borders
		g.drawGridOverlaysDelineated(screen)
	default:
		// Off
	}

	// Always draw per-tile coordinate labels (client row,col | server mapped r,c)
	g.drawTileCoords(screen)
}

// drawGridSquares draws each grid square as a colored rectangle
func (g *IsoGrid) drawGridSquares(screen *ebiten.Image) {
	// Create small colored dots for walkable (green) and blocked (red) tiles
	walkableDot := ebiten.NewImage(4, 4)
	walkableDot.Fill(color.RGBA{0, 255, 0, 255})

	blockedDot := ebiten.NewImage(4, 4)
	blockedDot.Fill(color.RGBA{255, 0, 0, 255})

	// Draw each square
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			square := g.Squares[row][col]
			if square == nil {
				continue
			}

			// Get square center in screen coordinates
			center := square.GetViewCenter()

			// Choose dot based on walkability
			var dot *ebiten.Image
			if square.IsWalkable() {
				dot = walkableDot
			} else {
				dot = blockedDot
			}

			// Draw a single dot at the exact center of the tile
			opts := &ebiten.DrawImageOptions{}
			opts.GeoM.Translate(center.X-2, center.Y-2) // Center the 4x4 dot
			screen.DrawImage(dot, opts)
		}
	}
}

// drawGridOverlays draws translucent green/red diamond overlays for each tile
func (g *IsoGrid) drawGridOverlays(screen *ebiten.Image) {
	// 1x1 white source image for triangle fills
	src := ebiten.NewImage(1, 1)
	src.Fill(color.White)

	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			square := g.Squares[row][col]
			if square == nil {
				continue
			}

			corners := square.GetViewPoints()

			// Build vertices for diamond (two triangles)
			var r, gcol, b, a float32
			if square.IsWalkable() {
				r, gcol, b, a = 0.0, 1.0, 0.0, 0.35 // translucent green
			} else {
				r, gcol, b, a = 1.0, 0.0, 0.0, 0.35 // translucent red
			}

			verts := []ebiten.Vertex{
				{DstX: float32(corners.TopLeft.X), DstY: float32(corners.TopLeft.Y), SrcX: 0, SrcY: 0, ColorR: r, ColorG: gcol, ColorB: b, ColorA: a},
				{DstX: float32(corners.TopRight.X), DstY: float32(corners.TopRight.Y), SrcX: 0, SrcY: 0, ColorR: r, ColorG: gcol, ColorB: b, ColorA: a},
				{DstX: float32(corners.BottomRight.X), DstY: float32(corners.BottomRight.Y), SrcX: 0, SrcY: 0, ColorR: r, ColorG: gcol, ColorB: b, ColorA: a},
				{DstX: float32(corners.BottomLeft.X), DstY: float32(corners.BottomLeft.Y), SrcX: 0, SrcY: 0, ColorR: r, ColorG: gcol, ColorB: b, ColorA: a},
			}

			idx := []uint16{0, 1, 2, 0, 2, 3}
			screen.DrawTriangles(verts, idx, src, nil)
		}
	}
}

// drawGridOverlaysDelineated draws translucent overlays plus clear borders per tile
func (g *IsoGrid) drawGridOverlaysDelineated(screen *ebiten.Image) {
	// Source image for triangle fills
	src := ebiten.NewImage(1, 1)
	src.Fill(color.White)

	// Border color (white, mostly opaque)
	br, bg, bb, ba := float32(1.0), float32(1.0), float32(1.0), float32(0.85)

	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			square := g.Squares[row][col]
			if square == nil {
				continue
			}

			corners := square.GetViewPoints()

			// Overlay color
			var r, gcol, b, a float32
			if square.IsWalkable() {
				r, gcol, b, a = 0.0, 1.0, 0.0, 0.35
			} else {
				r, gcol, b, a = 1.0, 0.0, 0.0, 0.35
			}

			// Draw diamond overlay (two triangles)
			verts := []ebiten.Vertex{
				{DstX: float32(corners.TopLeft.X), DstY: float32(corners.TopLeft.Y), SrcX: 0, SrcY: 0, ColorR: r, ColorG: gcol, ColorB: b, ColorA: a},
				{DstX: float32(corners.TopRight.X), DstY: float32(corners.TopRight.Y), SrcX: 0, SrcY: 0, ColorR: r, ColorG: gcol, ColorB: b, ColorA: a},
				{DstX: float32(corners.BottomRight.X), DstY: float32(corners.BottomRight.Y), SrcX: 0, SrcY: 0, ColorR: r, ColorG: gcol, ColorB: b, ColorA: a},
				{DstX: float32(corners.BottomLeft.X), DstY: float32(corners.BottomLeft.Y), SrcX: 0, SrcY: 0, ColorR: r, ColorG: gcol, ColorB: b, ColorA: a},
			}
			idx := []uint16{0, 1, 2, 0, 2, 3}
			screen.DrawTriangles(verts, idx, src, nil)

			// Draw edges for delineation with thin lines
			g.drawThickLine(screen, corners.TopLeft.X, corners.TopLeft.Y, corners.TopRight.X, corners.TopRight.Y, br, bg, bb, ba, 1.25, src)
			g.drawThickLine(screen, corners.TopRight.X, corners.TopRight.Y, corners.BottomRight.X, corners.BottomRight.Y, br, bg, bb, ba, 1.25, src)
			g.drawThickLine(screen, corners.BottomRight.X, corners.BottomRight.Y, corners.BottomLeft.X, corners.BottomLeft.Y, br, bg, bb, ba, 1.25, src)
			g.drawThickLine(screen, corners.BottomLeft.X, corners.BottomLeft.Y, corners.TopLeft.X, corners.TopLeft.Y, br, bg, bb, ba, 1.25, src)
		}
	}
}

// drawThickLine draws a thin rectangle between two points using triangles
func (g *IsoGrid) drawThickLine(screen *ebiten.Image, x1, y1, x2, y2 float64, r, gr, b, a float32, thickness float64, src *ebiten.Image) {
	dx := x2 - x1
	dy := y2 - y1
	length := math.Hypot(dx, dy)
	if length == 0 {
		return
	}
	nx := -dy / length
	ny := dx / length
	half := thickness / 2.0

	// Four corners of the thick line quad
	x1a := float32(x1 + nx*half)
	y1a := float32(y1 + ny*half)
	x1b := float32(x1 - nx*half)
	y1b := float32(y1 - ny*half)
	x2a := float32(x2 + nx*half)
	y2a := float32(y2 + ny*half)
	x2b := float32(x2 - nx*half)
	y2b := float32(y2 - ny*half)

	verts := []ebiten.Vertex{
		{DstX: x1a, DstY: y1a, SrcX: 0, SrcY: 0, ColorR: r, ColorG: gr, ColorB: b, ColorA: a},
		{DstX: x2a, DstY: y2a, SrcX: 0, SrcY: 0, ColorR: r, ColorG: gr, ColorB: b, ColorA: a},
		{DstX: x2b, DstY: y2b, SrcX: 0, SrcY: 0, ColorR: r, ColorG: gr, ColorB: b, ColorA: a},
		{DstX: x1b, DstY: y1b, SrcX: 0, SrcY: 0, ColorR: r, ColorG: gr, ColorB: b, ColorA: a},
	}
	idx := []uint16{0, 1, 2, 0, 2, 3}
	screen.DrawTriangles(verts, idx, src, nil)
}

// drawTileCoords renders small labels per tile: client [row,col] and mapped server coords
// Font size requested: 8pt. We use the UI font manager to get a small face.
func (g *IsoGrid) drawTileCoords(screen *ebiten.Image) {
	fm := ui.GetGlobalFontManager()
	var face = fm.GetFont("Verdana", "plain", 8)
	if face == nil {
		return
	}
	// Pink for client coords
	pink := color.RGBA{255, 105, 180, 255}
	yellow := color.RGBA{255, 255, 0, 255}

	for row := 1; row <= g.Rows; row++ {
		for col := 1; col <= g.Cols; col++ {
			sq := g.GetSquareByRowCol(row, col)
			if sq == nil {
				continue
			}
			center := sq.GetViewCenter()
			// Client coords: [row,col]
			client := fmt.Sprintf("%d,%d", row, col)
			text.Draw(screen, client, face, int(center.X)-10, int(center.Y)-2, pink)

			// Server-mapped coords (reverse of ApplyPathMap mapping used):
			// ApplyPathMap: mappedRow = node.Col; mappedCol = pathMap.Cols - node.Row + 1
			// Invert mapping for display: serverRow = g.Cols - col + 1; serverCol = row
			serverRow := g.Cols - col + 1
			serverCol := row
			server := fmt.Sprintf("%d,%d", serverRow, serverCol)
			text.Draw(screen, server, face, int(center.X)-10, int(center.Y)+8, yellow)
		}
	}
}

// doDepthCalcs reproduces Lingo's diagonal base depth assignment and per-square layer depths
func (g *IsoGrid) doDepthCalcs() {
	index := 1
	// First diagonal sweep
	for baseRow := 1; baseRow <= g.Rows; baseRow++ {
		col := 0
		for r := baseRow; r >= 1; r-- {
			col++
			sq := g.GetSquareByRowCol(r, col)
			if sq == nil {
				continue
			}
			baseDepth := index * sq.Layers
			sq.SetBaseDepth(baseDepth)
			sq.CalcLayerDepths(baseDepth)
			index++
		}
	}
	// Second diagonal sweep
	for baseCol := 2; baseCol <= g.Cols; baseCol++ {
		row := g.Rows + 1
		for c := baseCol; c <= g.Cols; c++ {
			row--
			sq := g.GetSquareByRowCol(row, c)
			if sq == nil {
				continue
			}
			baseDepth := index * sq.Layers
			sq.SetBaseDepth(baseDepth)
			sq.CalcLayerDepths(baseDepth)
			index++
		}
	}
}

// GetGridInfo returns debug information about the grid
func (g *IsoGrid) GetGridInfo() string {
	walkableCount := len(g.GetWalkableSquares())
	totalCount := g.Rows * g.Cols

	return fmt.Sprintf("Grid: %dx%d (%d squares), %d walkable, Size: %d, Offset: (%d,%d)",
		g.Rows, g.Cols, totalCount, walkableCount, g.SqSize, g.XOff, g.YOff)
}

// ValidateCoordinates checks if row/col coordinates are within grid bounds
func (g *IsoGrid) ValidateCoordinates(row, col int) bool {
	return row >= 1 && row <= g.Rows && col >= 1 && col <= g.Cols
}

// GetGridBounds returns the min/max row/col values for the grid
func (g *IsoGrid) GetGridBounds() (minRow, maxRow, minCol, maxCol int) { return 1, g.Rows, 1, g.Cols }

// GetConstants returns the constants used by this grid
func (g *IsoGrid) GetConstants() *IsoConstants { return g.constants }

// GetMath2d returns the math2d instance used by this grid
func (g *IsoGrid) GetMath2d() *Math2d { return g.math2d }

// getNeighbors returns the neighbors of a given square for pathfinding.
// This matches Lingo's createDirections() and getSuccessors() logic.
func (g *IsoGrid) getNeighbors(square *IsoSquare) []*IsoSquare {
	neighbors := make([]*IsoSquare, 0, 8)

	// Lingo createDirections order:
	// aDirections.append(point(0, -1))   // North
	// if me.bUseDiagonals: aDirections.append(point(1, -1))  // NE
	// aDirections.append(point(1, 0))    // East
	// if me.bUseDiagonals: aDirections.append(point(1, 1))   // SE
	// aDirections.append(point(0, 1))    // South
	// if me.bUseDiagonals: aDirections.append(point(-1, 1))  // SW
	// aDirections.append(point(-1, 0))   // West
	// if me.bUseDiagonals: aDirections.append(point(-1, -1)) // NW

	// Note: Lingo point(x,y) maps to (col, row) in our system
	// So point(0,-1) means col+0, row-1 (north)

	type direction struct {
		dCol, dRow int
	}

	// Build direction list based on whether diagonals are enabled
	// For CokeStudios, diagonals are typically enabled (bUseDiagonals = true)
	directions := []direction{
		{0, -1},  // North (point(0, -1))
		{1, -1},  // NE (point(1, -1)) - diagonal
		{1, 0},   // East (point(1, 0))
		{1, 1},   // SE (point(1, 1)) - diagonal
		{0, 1},   // South (point(0, 1))
		{-1, 1},  // SW (point(-1, 1)) - diagonal
		{-1, 0},  // West (point(-1, 0))
		{-1, -1}, // NW (point(-1, -1)) - diagonal
	}

	for _, dir := range directions {
		// Lingo: nextPt = nPt + dirPt
		checkRow := square.Row + dir.dRow
		checkCol := square.Col + dir.dCol

		if !g.ValidateCoordinates(checkRow, checkCol) {
			continue
		}

		neighborSquare := g.GetSquareByRowCol(checkRow, checkCol)
		if neighborSquare != nil {
			neighbors = append(neighbors, neighborSquare)
		}
	}

	// Lingo also adds pointer nodes from the map (teleporters, etc.)
	// This would be handled by checking square.p (pointer list) in the map data
	// TODO: Add pointer node support when map data is fully loaded

	return neighbors
}
