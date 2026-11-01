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
// Replicates the Lingo Math2d.ls and 1_IsoScene.ls behavior with 1:1 parity
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
// Matches Lingo 1_IsoScene.ls calcViewX and calcViewY exactly:
// on calcViewX me, o3dPoint: return integer(me.iXOff + ((o3dPoint.z * me.fSinY) + (o3dPoint.x * me.fCosY)))
// on calcViewY me, o3dPoint: return integer(me.iYOff + ((o3dPoint.y * me.fCosX) - (((o3dPoint.z * me.fCosY) - (o3dPoint.x * me.fSinY)) / 2)))
func (m *Math2d) ProjectToScreen(gridPoint Vector3D, xOff, yOff int) core.Point2D {
	finalX := float64(xOff) + ((gridPoint.Z * m.constants.SinY) + (gridPoint.X * m.constants.CosY))
	finalY := float64(yOff) + ((gridPoint.Y * m.constants.CosX) - (((gridPoint.Z * m.constants.CosY) - (gridPoint.X * m.constants.SinY)) / 2.0))

	return core.Point2D{X: finalX, Y: finalY}
}

// ProjectToGrid converts 2D screen coordinates to 3D grid coordinates
// Matches Lingo 1_IsoScene.ls calcViewToGridPoint exactly:
// iViewX = oViewPoint.locH - me.iXOff
// iViewY = oViewPoint.locV - me.iYOff
// iZ = float(((iViewX / me.fCosY) - (iViewY / (me.fSinY * me.fSinX))) * (1 / ((me.fCosY / me.fSinY) + (me.fSinY / me.fCosY))))
// iX = float(1 / me.fCosY * (iViewX - (iZ * me.fSinY)))
// return vector(iX, 0, iZ)
func (m *Math2d) ProjectToGrid(screenPoint core.Point2D, xOff, yOff int, height float64) Vector3D {
	iViewX := screenPoint.X - float64(xOff)
	iViewY := (screenPoint.Y - float64(yOff)) - (height * m.constants.CosX)

	iZ := (((iViewX / m.constants.CosY) - (iViewY / (m.constants.SinY * m.constants.SinX))) * (1.0 / ((m.constants.CosY / m.constants.SinY) + (m.constants.SinY / m.constants.CosY))))
	iX := (1.0 / m.constants.CosY * (iViewX - (iZ * m.constants.SinY)))

	return Vector3D{X: iX, Y: height, Z: iZ}
}

// ScreenToGridCoordinates converts screen mouse coordinates to grid row/col
// Matches Lingo 5_IsoGrid.ls getSquareByXZ behavior exactly:
// iCol = ceil(abs(iX) / sqSize)
// iRow = ceil(abs(iZ) / sqSize)
func (m *Math2d) ScreenToGridCoordinates(screenX, screenY, xOff, yOff, sqSize int) (row, col int, valid bool) {
	gridPoint := m.ProjectToGrid(core.Point2D{X: float64(screenX), Y: float64(screenY)}, xOff, yOff, 0)

	// Lingo parity: X must be >= 0, Z must be <= 0
	if gridPoint.X < 0 || gridPoint.Z > 0 {
		return 0, 0, false
	}

	col = int(math.Ceil(gridPoint.X / float64(sqSize)))
	row = int(math.Ceil(math.Abs(gridPoint.Z) / float64(sqSize)))

	if col <= 0 || row <= 0 || row > 30 || col > 30 {
		return 0, 0, false
	}

	return row, col, true
}

// GridToScreenCoordinates converts grid row/col to screen coordinates
// Used for positioning elements at specific grid locations
func (m *Math2d) GridToScreenCoordinates(row, col, xOff, yOff, sqSize int) core.Point2D {
	gridX := float64((col - 1) * sqSize)
	gridZ := float64(-(row - 1) * sqSize)
	gridY := 0.0

	return m.ProjectToScreen(Vector3D{X: gridX, Y: gridY, Z: gridZ}, xOff, yOff)
}

// GetSquareCenter returns the center point of a grid square in screen coordinates
// Matches Lingo IsoSquare.calcViewCenter() behavior
func (m *Math2d) GetSquareCenter(row, col, xOff, yOff, sqSize int) core.Point2D {
	minX := float64((col - 1) * sqSize)
	minZ := -float64((row-1)*sqSize) - float64(sqSize)
	centerX := minX + float64(sqSize)/2.0
	centerZ := minZ + float64(sqSize)/2.0

	return m.ProjectToScreen(Vector3D{X: centerX, Y: 0, Z: centerZ}, xOff, yOff)
}

// GetSquareCorners returns the four corner points of a grid square in screen coordinates
// Matches Lingo IsoSquare corner calculations (Quad tl, tr, br, bl)
func (m *Math2d) GetSquareCorners(row, col, xOff, yOff, sqSize int) SquareCorners {
	minX := float64((col - 1) * sqSize)
	maxX := minX + float64(sqSize)
	maxZ := -float64((row - 1) * sqSize)
	minZ := maxZ - float64(sqSize)
	gridY := 0.0

	tl := m.ProjectToScreen(Vector3D{X: minX, Y: gridY, Z: maxZ}, xOff, yOff)
	tr := m.ProjectToScreen(Vector3D{X: maxX, Y: gridY, Z: maxZ}, xOff, yOff)
	br := m.ProjectToScreen(Vector3D{X: maxX, Y: gridY, Z: minZ}, xOff, yOff)
	bl := m.ProjectToScreen(Vector3D{X: minX, Y: gridY, Z: minZ}, xOff, yOff)

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
	// Diamond formed by: TopLeft -> TopRight -> BottomRight -> BottomLeft -> TopLeft
	polygon := []core.Point2D{
		corners.TopLeft,
		corners.TopRight,
		corners.BottomRight,
		corners.BottomLeft,
		corners.TopLeft,
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
