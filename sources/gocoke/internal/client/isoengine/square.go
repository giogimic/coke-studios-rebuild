package isoengine

import (
	"fmt"
	"gocoke/internal/client/core"
)

// IsoSquare represents a single square in the isometric grid
// Replicates the Lingo IsoSquare.ls behavior with 1:1 parity
type IsoSquare struct {
	// Grid position
	Row    int
	Col    int
	Height int
	Type   int
	Dir    string

	// Grid reference
	grid *IsoGrid

	// 3D bounding box in grid coordinates
	MinX float64
	MaxX float64
	MinY float64
	MaxY float64
	MinZ float64
	MaxZ float64

	// Calculated points
	GridCenter Vector3D      // Center point in grid coordinates
	ViewCenter core.Point2D  // Center point in screen coordinates
	GridPoints SquareCorners // Corner points in grid coordinates (as 2D for simplicity)
	ViewPoints SquareCorners // Corner points in screen coordinates

	// Pathfinding properties
	Weight         int // 0 = walkable, 10 = blocked, other values for different costs
	OriginalWeight int // Original weight from PathMap data, used to restore when furniture is removed

	// Dynamic occupancy
	OccupiedByAvatar bool // If true, this square is temporarily blocked by an avatar

	// Layer and depth information
	Layers          int
	LayerDepthOff   int
	LayerStartDepth int
	LayerDepths     []int
	BaseDepth       int
}

// NewIsoSquare creates a new IsoSquare instance
// Matches Lingo IsoSquare.new() behavior
func NewIsoSquare(grid *IsoGrid, row, col, height, squareType int, dir string) *IsoSquare {
	square := &IsoSquare{
		Row:            row,
		Col:            col,
		Height:         height,
		Type:           squareType,
		Dir:            dir,
		grid:           grid,
		Weight:         0, // Default to walkable
		OriginalWeight: 0, // Default to walkable

		// Layer configuration (matches Lingo defaults)
		Layers:          grid.constants.DefaultLayers,
		LayerDepthOff:   grid.constants.DefaultStartDepth,
		LayerStartDepth: grid.constants.DefaultDepthOff,
		LayerDepths:     make([]int, 0),
	}

	// Calculate all coordinate information
	square.doGridCalc()

	return square
}

// doGridCalc performs all grid-related calculations
// Matches Lingo IsoSquare.doGridCalc() behavior
func (s *IsoSquare) doGridCalc() {
	// Calculate 3D bounding box
	s.calcBoundingBox()

	// Calculate grid points and center
	s.calcGridPoints()
	s.calcGridCenter()

	// Calculate view (screen) points and center
	s.calcViewPoints()
	s.calcViewCenter()
}

// SetBaseDepth sets the precomputed diagonal base depth for this square
// Matches Lingo IsoSquare.setBaseDepth()
func (s *IsoSquare) SetBaseDepth(base int) {
	s.BaseDepth = base
}

// CalcLayerDepths fills the aLayerDepths array for this square
// Matches Lingo IsoSquare.calcLayerDepths()
func (s *IsoSquare) CalcLayerDepths(base int) {
	layers := s.Layers
	if layers <= 0 {
		layers = 1
	}
	// 1-based indexing like Lingo: allocate layers+1, leave index 0 unused
	s.LayerDepths = make([]int, layers+1)
	for i := 1; i <= layers; i++ {
		s.LayerDepths[i] = base + (i - 1)
	}
}

// calcBoundingBox calculates the 3D bounding box for this square
// Matches Lingo IsoSquare calc methods
func (s *IsoSquare) calcBoundingBox() {
	sqSize := float64(s.grid.SqSize)

	// X coordinates (columns)
	s.MinX = sqSize * float64(s.Col-1) // 1-based to 0-based
	s.MaxX = s.MinX + sqSize

	// Y coordinates (height)
	s.MinY = 0.0
	s.MaxY = 0.0 // Floor level for now

	// Z coordinates (rows) - negative because rows increase "into" the screen
	s.MaxZ = -sqSize * float64(s.Row-1) // 1-based to 0-based
	s.MinZ = s.MaxZ - sqSize
}

// calcGridCenter calculates the center point in grid coordinates
// Matches Lingo IsoSquare.calcGridCenter() behavior
func (s *IsoSquare) calcGridCenter() {
	s.GridCenter = Vector3D{
		X: s.MinX + (s.MaxX-s.MinX)/2.0,
		Y: s.MaxY - (s.MaxY-s.MinY)/2.0,
		Z: s.MinZ + (s.MaxZ-s.MinZ)/2.0,
	}
}

// calcGridPoints calculates the four corner points in grid coordinates
func (s *IsoSquare) calcGridPoints() {
	// Store as 2D points for simplicity (X, Z coordinates, Y is always 0 for floor)
	s.GridPoints = SquareCorners{
		TopLeft:     core.Point2D{X: s.MinX, Y: s.MaxZ}, // Top-left in isometric view
		TopRight:    core.Point2D{X: s.MaxX, Y: s.MaxZ}, // Top-right in isometric view
		BottomLeft:  core.Point2D{X: s.MinX, Y: s.MinZ}, // Bottom-left in isometric view
		BottomRight: core.Point2D{X: s.MaxX, Y: s.MinZ}, // Bottom-right in isometric view
	}
}

// calcViewPoints calculates the four corner points in screen coordinates
// Matches Lingo IsoSquare view point calculations
func (s *IsoSquare) calcViewPoints() {
	math2d := s.grid.math2d

	// Convert each grid corner to screen coordinates
	s.ViewPoints = SquareCorners{
		TopLeft: math2d.ProjectToScreen(
			Vector3D{X: s.MinX, Y: s.MinY, Z: s.MaxZ},
			s.grid.XOff, s.grid.YOff,
		),
		TopRight: math2d.ProjectToScreen(
			Vector3D{X: s.MaxX, Y: s.MinY, Z: s.MaxZ},
			s.grid.XOff, s.grid.YOff,
		),
		BottomLeft: math2d.ProjectToScreen(
			Vector3D{X: s.MinX, Y: s.MinY, Z: s.MinZ},
			s.grid.XOff, s.grid.YOff,
		),
		BottomRight: math2d.ProjectToScreen(
			Vector3D{X: s.MaxX, Y: s.MinY, Z: s.MinZ},
			s.grid.XOff, s.grid.YOff,
		),
	}
}

// calcViewCenter calculates the center point in screen coordinates
// Matches Lingo IsoSquare.calcViewCenter() behavior
func (s *IsoSquare) calcViewCenter() {
	math2d := s.grid.math2d

	s.ViewCenter = math2d.ProjectToScreen(
		s.GridCenter,
		s.grid.XOff, s.grid.YOff,
	)
}

// GetRow returns the row index (1-based)
func (s *IsoSquare) GetRow() int { return s.Row }

// GetCol returns the column index (1-based)
func (s *IsoSquare) GetCol() int { return s.Col }

// GetGridCenter returns the center point in grid coordinates
func (s *IsoSquare) GetGridCenter() Vector3D { return s.GridCenter }

// GetViewCenter returns the center point in screen coordinates
func (s *IsoSquare) GetViewCenter() core.Point2D { return s.ViewCenter }

// GetViewPoints returns the corner points in screen coordinates
func (s *IsoSquare) GetViewPoints() SquareCorners { return s.ViewPoints }

// IsWalkable returns true if this square can be walked on
// Walkable if base weight allows it AND the square is not occupied by an avatar
func (s *IsoSquare) IsWalkable() bool { return s.Weight == 0 && !s.OccupiedByAvatar }

// SetWeight sets the pathfinding weight for this square
// Matches PathMap node weight system
func (s *IsoSquare) SetWeight(weight int) { s.Weight = weight }

// SetOriginalWeight sets the original pathfinding weight from PathMap data
func (s *IsoSquare) SetOriginalWeight(weight int) { s.OriginalWeight = weight }

// GetOriginalWeight returns the original pathfinding weight from PathMap data
func (s *IsoSquare) GetOriginalWeight() int { return s.OriginalWeight }

// GetWeight returns the pathfinding weight for this square
func (s *IsoSquare) GetWeight() int { return s.Weight }

// SetAvatarOccupied sets the dynamic avatar occupancy flag for this square
func (s *IsoSquare) SetAvatarOccupied(occupied bool) { s.OccupiedByAvatar = occupied }

// IsAvatarOccupied reports whether an avatar currently occupies this square
func (s *IsoSquare) IsAvatarOccupied() bool { return s.OccupiedByAvatar }

// ContainsPoint checks if a screen point is within this square
// Uses the diamond-shaped isometric square bounds
func (s *IsoSquare) ContainsPoint(screenPoint core.Point2D) bool {
	return s.grid.math2d.IsPointInSquare(screenPoint, s.ViewPoints)
}

// GetDepth calculates the rendering depth for this square
// Matches Lingo depth calculation system
func (s *IsoSquare) GetDepth(layer string) int {
	layerDepth := s.grid.constants.GetLayerDepth(layer)
	positionDepth := s.Row*s.grid.constants.DefaultCols + s.Col
	return s.grid.constants.DefaultStartDepth + layerDepth*s.grid.constants.DefaultDepthOff + positionDepth
}

// GetDepthFromMap calculates depth using the Lingo getDepthFromMap method
// Matches Lingo IsoSquare.getDepthFromMap(sMapProp, iGridY) behavior
func (s *IsoSquare) GetDepthFromMap(layerName string, gridY int) int {
	// Get base layer depth index (1-based)
	layerDepth := s.grid.constants.GetLayerDepth(layerName)
	if layerDepth <= 0 {
		layerDepth = 1
	}
	// Calculate height layer: iLayer + (layersPerMap * gridY)
	layersPerMap := len(s.grid.constants.LayerMap)
	heightLayer := layerDepth + (layersPerMap * gridY)
	if s.LayerDepths == nil || heightLayer < 1 || heightLayer >= len(s.LayerDepths) {
		// Fallback to simple calculation if layer depths not initialized
		return s.BaseDepth + (heightLayer - 1)
	}
	return s.LayerDepths[heightLayer]
}

// ToString returns a string representation for debugging
func (s *IsoSquare) ToString() string {
	return fmt.Sprintf("Square[%d,%d] Weight:%d Center:(%.1f,%.1f) ViewCenter:(%.1f,%.1f)",
		s.Row, s.Col, s.Weight,
		s.GridCenter.X, s.GridCenter.Z,
		s.ViewCenter.X, s.ViewCenter.Y)
}

// PrintSquare outputs debug information about this square
// Matches Lingo IsoSquare.printSquare() behavior
func (s *IsoSquare) PrintSquare() {
	fmt.Printf("[SQUARE]\n")
	fmt.Printf("Row: %d, Col: %d\n", s.Row, s.Col)
	fmt.Printf("MinX: %.1f, MaxX: %.1f\n", s.MinX, s.MaxX)
	fmt.Printf("MinY: %.1f, MaxY: %.1f\n", s.MinY, s.MaxY)
	fmt.Printf("MinZ: %.1f, MaxZ: %.1f\n", s.MinZ, s.MaxZ)
	fmt.Printf("GridCenter: (%.1f, %.1f, %.1f)\n", s.GridCenter.X, s.GridCenter.Y, s.GridCenter.Z)
	fmt.Printf("ViewCenter: (%.1f, %.1f)\n", s.ViewCenter.X, s.ViewCenter.Y)
	fmt.Printf("Weight: %d (walkable: %t)\n", s.Weight, s.IsWalkable())
}
