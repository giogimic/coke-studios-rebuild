package isoengine

import (
	"math"

	"gocoke/internal/client/core"
)

// Vector3D represents a 3D point in grid space
type Vector3D struct {
	X float64
	Y float64
	Z float64
}

// Math2d handles 2D/3D coordinate transformations for the isometric engine
// Replicates the Lingo Math2d.ls behavior with 1:1 parity
type Math2d struct {
	constants *IsoConstants
}

// NewMath2d creates a new Math2d instance
func NewMath2d(constants *IsoConstants) *Math2d {
	return &Math2d{
		constants: constants,
	}
}

// ProjectToScreen converts 3D grid coordinates to 2D screen coordinates
// Matches Lingo's isometric projection calculations exactly
func (m *Math2d) ProjectToScreen(gridPoint Vector3D, xOff, yOff int) core.Point2D {
	// Apply isometric transformation matrix
	// This matches the Lingo calculation using sin/cos values

	// Rotate around Y axis (45 degrees)
	rotatedX := gridPoint.X*m.constants.CosY - gridPoint.Z*m.constants.SinY
	rotatedZ := gridPoint.X*m.constants.SinY + gridPoint.Z*m.constants.CosY

	// Rotate around X axis (30 degrees)
	screenX := rotatedX
	screenY := gridPoint.Y*m.constants.CosX - rotatedZ*m.constants.SinX

	// Apply screen offsets
	finalX := screenX + float64(xOff)
	finalY := screenY + float64(yOff)

	return core.Point2D{X: finalX, Y: finalY}
}

// ProjectToGrid converts 2D screen coordinates to 3D grid coordinates
// Inverse of ProjectToScreen - matches Lingo's reverse calculation
func (m *Math2d) ProjectToGrid(screenPoint core.Point2D, xOff, yOff int, height float64) Vector3D {
	// Remove screen offsets
	adjustedX := screenPoint.X - float64(xOff)
	adjustedY := screenPoint.Y - float64(yOff)

	// Reverse the X rotation (30 degrees)
	// Assuming Y = height for the reverse calculation
	rotatedZ := (height*m.constants.CosX - adjustedY) / m.constants.SinX
	rotatedX := adjustedX

	// Reverse the Y rotation (45 degrees)
	gridX := rotatedX*m.constants.CosY + rotatedZ*m.constants.SinY
	gridZ := -rotatedX*m.constants.SinY + rotatedZ*m.constants.CosY

	return Vector3D{X: gridX, Y: height, Z: gridZ}
}

// ScreenToGridCoordinates converts screen mouse coordinates to grid row/col
// This is the key function for mouse-to-tile conversion
func (m *Math2d) ScreenToGridCoordinates(screenX, screenY, xOff, yOff, sqSize int) (row, col int, valid bool) {
	// Convert screen coordinates to grid space
	gridPoint := m.ProjectToGrid(core.Point2D{X: float64(screenX), Y: float64(screenY)}, xOff, yOff, 0)

	// Convert grid coordinates to row/col indices
	// Lingo parity: use Ceil on magnitudes to avoid off-by-one at boundaries
	col = int(math.Ceil(gridPoint.X / float64(sqSize)))
	row = int(math.Ceil((-gridPoint.Z) / float64(sqSize)))
	// Ensure 1-based indexing
	if col < 1 {
		col = 1
	}
	if row < 1 {
		row = 1
	}

	// Validate coordinates are within reasonable bounds
	if row < 1 || col < 1 || row > 30 || col > 30 {
		return 0, 0, false
	}

	return row, col, true
}

// GridToScreenCoordinates converts grid row/col to screen coordinates
// Used for positioning elements at specific grid locations
func (m *Math2d) GridToScreenCoordinates(row, col, xOff, yOff, sqSize int) core.Point2D {
	// Convert row/col to grid coordinates (1-based to 0-based)
	gridX := float64((col - 1) * sqSize)
	gridZ := float64(-(row - 1) * sqSize) // Z is negative for rows
	gridY := 0.0                          // Floor level

	// Project to screen coordinates
	return m.ProjectToScreen(Vector3D{X: gridX, Y: gridY, Z: gridZ}, xOff, yOff)
}

// GetSquareCenter returns the center point of a grid square in screen coordinates
// Matches Lingo IsoSquare.calcViewCenter() behavior
func (m *Math2d) GetSquareCenter(row, col, xOff, yOff, sqSize int) core.Point2D {
	// Get the base position
	basePoint := m.GridToScreenCoordinates(row, col, xOff, yOff, sqSize)

	// Add half square size to get center
	// The isometric projection affects how we calculate the center
	centerOffsetX := float64(sqSize) * m.constants.CosY / 2.0
	centerOffsetY := float64(sqSize) * m.constants.SinX / 2.0

	return core.Point2D{
		X: basePoint.X + centerOffsetX,
		Y: basePoint.Y + centerOffsetY,
	}
}

// GetSquareCorners returns the four corner points of a grid square in screen coordinates
// Matches Lingo IsoSquare corner calculations
func (m *Math2d) GetSquareCorners(row, col, xOff, yOff, sqSize int) SquareCorners {
	// Calculate the four corners of the square in grid space
	gridX := float64((col - 1) * sqSize)
	gridZ := float64(-(row - 1) * sqSize)
	gridY := 0.0

	// Top-left corner
	tl := m.ProjectToScreen(Vector3D{X: gridX, Y: gridY, Z: gridZ}, xOff, yOff)

	// Top-right corner
	tr := m.ProjectToScreen(Vector3D{X: gridX + float64(sqSize), Y: gridY, Z: gridZ}, xOff, yOff)

	// Bottom-left corner
	bl := m.ProjectToScreen(Vector3D{X: gridX, Y: gridY, Z: gridZ - float64(sqSize)}, xOff, yOff)

	// Bottom-right corner
	br := m.ProjectToScreen(Vector3D{X: gridX + float64(sqSize), Y: gridY, Z: gridZ - float64(sqSize)}, xOff, yOff)

	return SquareCorners{
		TopLeft:     tl,
		TopRight:    tr,
		BottomLeft:  bl,
		BottomRight: br,
	}
}

// SquareCorners holds the four corner points of a grid square
type SquareCorners struct {
	TopLeft     core.Point2D
	TopRight    core.Point2D
	BottomLeft  core.Point2D
	BottomRight core.Point2D
}

// IsPointInSquare checks if a screen point is within a grid square
// Uses the four corner points to determine if point is inside the diamond shape
func (m *Math2d) IsPointInSquare(point core.Point2D, corners SquareCorners) bool {
	// For isometric squares, we need to check if the point is within the diamond shape
	// This uses a point-in-polygon algorithm for the diamond formed by the four corners

	// Create polygon points in order: TL -> TR -> BR -> BL -> TL
	polygon := []core.Point2D{
		corners.TopLeft,
		corners.TopRight,
		corners.BottomRight,
		corners.BottomLeft,
		corners.TopLeft, // Close the polygon
	}

	return m.pointInPolygon(point, polygon)
}

// pointInPolygon uses ray casting algorithm to determine if point is inside polygon
func (m *Math2d) pointInPolygon(point core.Point2D, polygon []core.Point2D) bool {
	inside := false
	j := len(polygon) - 1

	for i := 0; i < len(polygon); i++ {
		if ((polygon[i].Y > point.Y) != (polygon[j].Y > point.Y)) &&
			(point.X < (polygon[j].X-polygon[i].X)*(point.Y-polygon[i].Y)/(polygon[j].Y-polygon[i].Y)+polygon[i].X) {
			inside = !inside
		}
		j = i
	}

	return inside
}

// Distance calculates the distance between two 2D points
func (m *Math2d) Distance(p1, p2 core.Point2D) float64 {
	dx := p2.X - p1.X
	dy := p2.Y - p1.Y
	return math.Sqrt(dx*dx + dy*dy)
}

// Distance3D calculates the distance between two 3D points
func (m *Math2d) Distance3D(p1, p2 Vector3D) float64 {
	dx := p2.X - p1.X
	dy := p2.Y - p1.Y
	dz := p2.Z - p1.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// Rad converts degrees to radians
func (m *Math2d) Rad(degrees float64) float64 {
	return degrees * math.Pi / 180.0
}

// Deg converts radians to degrees
func (m *Math2d) Deg(radians float64) float64 {
	return radians * 180.0 / math.Pi
}
