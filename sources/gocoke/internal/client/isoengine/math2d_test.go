package isoengine

import (
	"math"
	"testing"

	"gocoke/internal/client/core"
)

func TestMath2d_ParityWithLingo(t *testing.T) {
	constants := NewIsoConstants()
	m := NewMath2d(constants)

	xOff := constants.DefaultXOff // 380
	yOff := constants.DefaultYOff // -11
	sqSize := constants.DefaultSqSize // 34

	// Test 1: Corner projection of (Row 1, Col 1)
	corners := m.GetSquareCorners(1, 1, xOff, yOff, sqSize)
	// TL (Point 1 in Lingo) is at (0, 0, 0) in grid:
	// Screen X = 380, Screen Y = -11
	if math.Abs(corners.TopLeft.X-380.0) > 0.01 || math.Abs(corners.TopLeft.Y-(-11.0)) > 0.01 {
		t.Errorf("TopLeft corner mismatch: got (%f, %f), expected (380.0, -11.0)", corners.TopLeft.X, corners.TopLeft.Y)
	}

	// Width of the diamond: TR.X - BL.X should be 48 pixels (factorx)
	diamondWidth := corners.TopRight.X - corners.BottomLeft.X
	if math.Abs(diamondWidth-48.08) > 0.1 {
		t.Errorf("Diamond width mismatch: got %f, expected ~48.08", diamondWidth)
	}

	// Height of the diamond: BR.Y - TL.Y should be 24 pixels (factory)
	diamondHeight := corners.BottomRight.Y - corners.TopLeft.Y
	if math.Abs(diamondHeight-24.04) > 0.1 {
		t.Errorf("Diamond height mismatch: got %f, expected ~24.04", diamondHeight)
	}

	// Aspect ratio must be 2.0 (standard 2:1 isometric projection)
	aspectRatio := diamondWidth / diamondHeight
	if math.Abs(aspectRatio-2.0) > 0.01 {
		t.Errorf("Aspect ratio not 2:1: got %f", aspectRatio)
	}

	// Test 2: Center of (Row 1, Col 1)
	center := m.GetSquareCenter(1, 1, xOff, yOff, sqSize)
	// Center in Lingo: bl.locH + width/2 = 356 + 24 = 380; tl.locV + height/2 = -11 + 12 = 1
	if math.Abs(center.X-380.0) > 0.1 || math.Abs(center.Y-1.0) > 0.1 {
		t.Errorf("Center mismatch: got (%f, %f), expected (380.0, 1.0)", center.X, center.Y)
	}

	// Test 3: ScreenToGridCoordinates round-trip
	row, col, valid := m.ScreenToGridCoordinates(int(center.X), int(center.Y), xOff, yOff, sqSize)
	if !valid {
		t.Fatalf("Expected valid screen-to-grid for center point (%f, %f)", center.X, center.Y)
	}
	if row != 1 || col != 1 {
		t.Errorf("ScreenToGrid mapped center to [%d, %d], expected [1, 1]", row, col)
	}

	// Test 4: Inverse projection round-trip for various tiles across the room
	for testRow := 1; testRow <= 20; testRow++ {
		for testCol := 1; testCol <= 20; testCol++ {
			c := m.GetSquareCenter(testRow, testCol, xOff, yOff, sqSize)
			r, cl, ok := m.ScreenToGridCoordinates(int(math.Round(c.X)), int(math.Round(c.Y)), xOff, yOff, sqSize)
			if !ok {
				t.Errorf("Tile [%d, %d] center (%f, %f) marked invalid", testRow, testCol, c.X, c.Y)
				continue
			}
			if r != testRow || cl != testCol {
				t.Errorf("Tile [%d, %d] round-trip failed: got [%d, %d]", testRow, testCol, r, cl)
			}
		}
	}

	// Test 5: Point in Square check
	if !m.IsPointInSquare(center, corners) {
		t.Errorf("Center point should be inside square corners")
	}
	// Point way outside should not be in square
	if m.IsPointInSquare(core.Point2D{X: 100, Y: 100}, corners) {
		t.Errorf("Outside point (100, 100) incorrectly considered inside square [1, 1]")
	}
}
