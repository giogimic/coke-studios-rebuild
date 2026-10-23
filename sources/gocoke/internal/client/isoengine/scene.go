package isoengine

import (
	"fmt"
	"log"
	"time"

	"gocoke/internal/client/core"

	"github.com/hajimehoshi/ebiten/v2"
)

// IsoScene manages the complete isometric scene
// Replicates the Lingo IsoScene.ls behavior with 1:1 parity
type IsoScene struct {
	// Core components
	constants *IsoConstants
	math2d    *Math2d
	grid      *IsoGrid
	wall      *WallSystem

	// Back-reference to owning high-level scene (for pattern manager)
	sceneOwner *SceneOwnerBridge

	// Scene configuration
	layoutType int // 1 or 2
	xOff       int
	yOff       int
	sqSize     int
	rows       int
	cols       int

	// Mouse and interaction
	mouseSquare      *IsoSquare // Current square under mouse
	mouseGridPoint   Vector3D   // Current mouse position in grid coordinates
	lastClickTime    int64      // For click timing
	allowedClickTime int64      // Minimum time between clicks

	// Debug settings
	debugEnabled bool

	// Mouse state
	mouseX int
	mouseY int
}

// NewIsoScene creates a new IsoScene instance
// Matches Lingo IsoScene.new() behavior
func NewIsoScene() *IsoScene {
	scene := &IsoScene{
		constants:        NewIsoConstants(),
		allowedClickTime: 650, // Matches Lingo default
		debugEnabled:     false,
	}

	scene.math2d = NewMath2d(scene.constants)

	log.Printf("IsoScene: created new isometric scene")
	return scene
}

// SetLayout configures the scene for a specific layout type
// Matches Lingo IsoScene.setLayout() behavior
func (s *IsoScene) SetLayout(layoutType int) {
	s.layoutType = layoutType

	// Get layout configuration from constants
	s.xOff, s.yOff, s.sqSize, s.rows, s.cols = s.constants.GetLayoutConfig(layoutType)

	log.Printf("IsoScene: set layout %d - Grid: %dx%d, SqSize: %d, Offset: (%d,%d)",
		layoutType, s.rows, s.cols, s.sqSize, s.xOff, s.yOff)
}

// SetCustomOffsets overrides the default offsets with custom values from Studio XML
func (s *IsoScene) SetCustomOffsets(xOff, yOff int) {
	s.xOff = xOff
	s.yOff = yOff

	log.Printf("IsoScene: set custom offsets to (%d,%d)", s.xOff, s.yOff)
}

// Init initializes the scene with the current layout
// Matches Lingo IsoScene.Init() behavior
func (s *IsoScene) Init() {
	if s.layoutType == 0 {
		// Default to layout 2 (large grid) for private studios
		s.SetLayout(2)
	}

	// Create the grid with current offsets (may be custom offsets from Studio XML)
	s.grid = NewIsoGrid(s.rows, s.cols, s.sqSize, s.xOff, s.yOff, s.constants)

	// Initialize wall system
	s.wall = NewWallSystem(s)

	log.Printf("IsoScene: initialized scene with layout %d, final offsets: (%d,%d)", s.layoutType, s.xOff, s.yOff)
}

// Update updates the scene state
// Should be called every frame
func (s *IsoScene) Update() {
	if s.grid == nil {
		return
	}

	// Update mouse coordinates
	s.mouseX, s.mouseY = ebiten.CursorPosition()

	// Update the square under the mouse
	s.mouseSquare = s.grid.GetSquareByScreenPoint(s.mouseX, s.mouseY)

	// Update mouse tracking
	s.updateMouseTracking()
}

// updateMouseTracking updates mouse position and current square
func (s *IsoScene) updateMouseTracking() {
	// Get the square under the mouse
	newMouseSquare := s.grid.GetSquareByScreenPoint(s.mouseX, s.mouseY)

	// Update mouse square if it changed
	if newMouseSquare != s.mouseSquare {
		s.mouseSquare = newMouseSquare

		if s.mouseSquare != nil {
			s.mouseGridPoint = s.mouseSquare.GetGridCenter()

			if s.debugEnabled {
				log.Printf("IsoScene: mouse over square [%d,%d] at grid (%.1f,%.1f,%.1f)",
					s.mouseSquare.GetRow(), s.mouseSquare.GetCol(),
					s.mouseGridPoint.X, s.mouseGridPoint.Y, s.mouseGridPoint.Z)
			}
		} else {
			s.mouseGridPoint = Vector3D{} // Reset to zero
		}
	}
}

// Draw renders the scene
func (s *IsoScene) Draw(screen *ebiten.Image) {
	if s.grid == nil {
		return
	}

	// Draw wall tiles first (behind other elements)
	if s.wall != nil {
		s.wall.Draw(screen)
	}

	// Draw debug grid if enabled (on top of walls)

	if s.debugEnabled {
		s.grid.DrawDebugGrid(screen)
	}
}

// AttachOwner attaches a scene owner bridge so isoengine can call pattern manager
func (s *IsoScene) AttachOwner(owner *SceneOwnerBridge) {
	s.sceneOwner = owner
}

// GetMouseSquare returns the square currently under the mouse
func (s *IsoScene) GetMouseSquare() *IsoSquare {
	return s.mouseSquare
}

// GetMouseGridPoint returns the current mouse position in grid coordinates
func (s *IsoScene) GetMouseGridPoint() Vector3D {
	return s.mouseGridPoint
}

// IsMouseOverWalkableTile returns true if mouse is over a walkable tile
// This is used by the hiliter system
func (s *IsoScene) IsMouseOverWalkableTile() bool {
	return s.mouseSquare != nil && s.mouseSquare.IsWalkable()
}

// GetSquareByRowCol returns a square by its row/col coordinates
func (s *IsoScene) GetSquareByRowCol(row, col int) *IsoSquare {
	if s.grid == nil {
		return nil
	}
	return s.grid.GetSquareByRowCol(row, col)
}

// GetSquareByScreenPoint returns a square by screen coordinates
func (s *IsoScene) GetSquareByScreenPoint(screenX, screenY int) *IsoSquare {
	if s.grid == nil {
		return nil
	}
	return s.grid.GetSquareByScreenPoint(screenX, screenY)
}

// SetSquareWeight sets the pathfinding weight for a square
// Used for loading PathMap data
func (s *IsoScene) SetSquareWeight(row, col, weight int) {
	if s.grid != nil {
		s.grid.SetSquareWeight(row, col, weight)
	}
}

// SetSquareOriginalWeight sets the original pathfinding weight for a square
// Used for storing PathMap data that can be restored later
func (s *IsoScene) SetSquareOriginalWeight(row, col, weight int) {
	if s.grid != nil {
		s.grid.SetSquareOriginalWeight(row, col, weight)
	}
}

// LoadPathMapData loads pathfinding data for the scene
// This will be expanded when we implement the XML parser
func (s *IsoScene) LoadPathMapData(pathMapData map[string]interface{}) {
	// TODO: Implement PathMap XML parsing
	// For now, this is a placeholder
	log.Printf("IsoScene: LoadPathMapData called (not yet implemented)")
}

// CenterGrid centers the grid on the screen
func (s *IsoScene) CenterGrid(screenWidth, screenHeight int) {
	if s.grid != nil {
		s.grid.CenterGrid(screenWidth, screenHeight)

		// Update scene offsets to match grid
		s.xOff = s.grid.XOff
		s.yOff = s.grid.YOff
	}
}

// EnableDebug enables/disables debug mode
func (s *IsoScene) EnableDebug(enabled bool) {
	s.debugEnabled = enabled
	if s.grid != nil {
		s.grid.EnableDebugGrid(enabled)
	}

	log.Printf("IsoScene: debug mode %t", enabled)
}

// GetGridToScreenCoordinates converts grid row/col to screen coordinates
// Useful for positioning elements at specific grid locations
func (s *IsoScene) GetGridToScreenCoordinates(row, col int) core.Point2D {
	if s.grid == nil {
		return core.Point2D{}
	}
	return s.math2d.GridToScreenCoordinates(row, col, s.xOff, s.yOff, s.sqSize)
}

// GetScreenToGridCoordinates converts screen coordinates to grid row/col
// Returns row, col, and whether the coordinates are valid
func (s *IsoScene) GetScreenToGridCoordinates(screenX, screenY int) (row, col int, valid bool) {
	if s.grid == nil {
		return 0, 0, false
	}
	return s.math2d.ScreenToGridCoordinates(screenX, screenY, s.xOff, s.yOff, s.sqSize)
}

// GetSquareCenter returns the center point of a square in screen coordinates
func (s *IsoScene) GetSquareCenter(row, col int) core.Point2D {
	if s.grid == nil {
		return core.Point2D{}
	}
	return s.math2d.GetSquareCenter(row, col, s.xOff, s.yOff, s.sqSize)
}

// GetLayoutType returns the current layout type
func (s *IsoScene) GetLayoutType() int {
	return s.layoutType
}

// GetGrid returns the grid instance
func (s *IsoScene) GetGrid() *IsoGrid {
	return s.grid
}

// GetConstants returns the constants instance
func (s *IsoScene) GetConstants() *IsoConstants {
	return s.constants
}

// GetMath2d returns the math2d instance
func (s *IsoScene) GetMath2d() *Math2d {
	return s.math2d
}

// GetSceneInfo returns debug information about the scene
func (s *IsoScene) GetSceneInfo() string {
	if s.grid == nil {
		return "IsoScene: not initialized"
	}

	mouseInfo := "none"
	if s.mouseSquare != nil {
		mouseInfo = fmt.Sprintf("[%d,%d]", s.mouseSquare.GetRow(), s.mouseSquare.GetCol())
	}

	return fmt.Sprintf("IsoScene: Layout %d, %s, Mouse: %s",
		s.layoutType, s.grid.GetGridInfo(), mouseInfo)
}

// Cleanup cleans up scene resources
func (s *IsoScene) Cleanup() {
	if s.wall != nil {
		s.wall.Destroy()
		s.wall = nil
	}
	s.grid = nil
	s.mouseSquare = nil
	log.Printf("IsoScene: cleaned up")
}

// LoadWallData loads wall data from Studio XML
func (s *IsoScene) LoadWallData(studioXML *StudioXML) error {
	if s.wall == nil {
		return fmt.Errorf("wall system not initialized")
	}
	return s.wall.Init(studioXML)
}

// GetWallSystem returns the wall system for external access
func (s *IsoScene) GetWallSystem() *WallSystem {
	return s.wall
}

// IsDoubleClick returns true if a second click occurred within the allowed window
func (s *IsoScene) IsDoubleClick() bool {
	now := time.Now().UnixMilli()
	if s.lastClickTime == 0 {
		s.lastClickTime = now
		return false
	}
	if now-s.lastClickTime <= s.allowedClickTime {
		s.lastClickTime = 0
		return true
	}
	s.lastClickTime = now
	return false
}
