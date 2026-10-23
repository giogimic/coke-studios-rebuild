package isoengine

import (
	"gocoke/internal/client/avatarengine"
	"gocoke/internal/client/core"
	"log"
	"math"
	"time"
)

// Directions now match server/iso: 0=N,1=NE,2=E,3=SE,4=S,5=SW,6=W,7=NW.

// Direction indices follow Lingo isoengine convention:
// 0=N, 1=NE, 2=E, 3=SE, 4=S, 5=SW, 6=W, 7=NW
// We avoid approximate vector matching and mirror Lingo's discrete
// grid offset mapping from IsoGrid.getDirectionToSquare().

// IsoAvatar represents an avatar in the isometric scene.
// It bridges the AvatarEngine with the IsoEngine.
type IsoAvatar struct {
	Engine *avatarengine.AvatarEngine

	// Position and movement
	Square *IsoSquare
	path   []*IsoSquare
	// speed is interpreted as pixels per second for time-based movement
	speed           float64
	screenPos       core.Point2D
	targetScreenPos core.Point2D
	isMovingToPoint bool
	nextSquare      *IsoSquare

	// Pending move - used when clicking during active movement
	pendingGoal *IsoSquare
	pendingGrid *IsoGrid

	// State
	screenName string
	mission    string
	isBot      bool
	isMoving   bool

	// Timing for time-based movement
	lastUpdate time.Time
}

// NewIsoAvatar creates a new IsoAvatar instance.
func NewIsoAvatar(engine *avatarengine.AvatarEngine, square *IsoSquare, screenName, mission string, isBot bool) *IsoAvatar {
	avatar := &IsoAvatar{
		Engine:     engine,
		Square:     square,
		screenName: screenName,
		mission:    mission,
		isBot:      isBot,
		// Legacy was ~4 px/tick at 30 TPS => ~120 px/sec. Slow ~20% for smoother remote movement.
		speed: 84.0,
	}
	// Initialize screen position
	if square != nil {
		avatar.screenPos = square.GetViewCenter()
		avatar.targetScreenPos = avatar.screenPos
	}
	return avatar
}

// SetSpeed sets the avatar's movement speed in pixels per second.
func (a *IsoAvatar) SetSpeed(pxPerSec float64) {
	if a == nil {
		return
	}
	a.speed = pxPerSec
}

// SetSpeedScale sets the avatar's movement speed as a multiple of the default.
// Default is 84 px/sec; e.g., scale 0.5 -> 42 px/sec.
func (a *IsoAvatar) SetSpeedScale(scale float64) {
	if a == nil {
		return
	}
	a.speed = 84.0 * scale
}

// SetMission updates the avatar's motto/mission string.
func (a *IsoAvatar) SetMission(m string) { a.mission = m }

// SetScreenPos sets the current on-screen position immediately.
func (a *IsoAvatar) SetScreenPos(pos core.Point2D) {
	a.screenPos = pos
	if a.Engine != nil {
		a.Engine.SetPosition(pos)
	}
}

// SetTargetScreenPos sets the target on-screen position without movement.
func (a *IsoAvatar) SetTargetScreenPos(pos core.Point2D) { a.targetScreenPos = pos }

// MoveToSquare calculates a path and initiates movement.
// If already moving, it queues the move as pending.
func (a *IsoAvatar) MoveToSquare(goal *IsoSquare, grid *IsoGrid) {
	if a.Square == goal {
		return
	}

	// If we're currently moving to a point, queue this as a pending move
	if a.isMovingToPoint && a.nextSquare != nil {
		log.Printf("IsoAvatar: queuing pending move to [%d,%d] (currently moving to [%d,%d])",
			valueOrZero(goal, func(s *IsoSquare) int { return s.Row }),
			valueOrZero(goal, func(s *IsoSquare) int { return s.Col }),
			valueOrZero(a.nextSquare, func(s *IsoSquare) int { return s.Row }),
			valueOrZero(a.nextSquare, func(s *IsoSquare) int { return s.Col }),
		)
		a.pendingGoal = goal
		a.pendingGrid = grid
		return
	}

	path := astar(a.Square, goal, grid)
	// Debug: print once per click
	log.Printf("IsoAvatar: start [%d,%d] goal [%d,%d] pathLen=%d path=%s",
		valueOrZero(a.Square, func(s *IsoSquare) int { return s.Row }),
		valueOrZero(a.Square, func(s *IsoSquare) int { return s.Col }),
		valueOrZero(goal, func(s *IsoSquare) int { return s.Row }),
		valueOrZero(goal, func(s *IsoSquare) int { return s.Col }),
		len(path),
		debugPathString(path),
	)

	if len(path) > 0 {
		// Ensure the final step reaches the exact goal square (Lingo parity)
		if path[len(path)-1] != goal && goal != nil && goal.IsWalkable() {
			path = append(path, goal)
		}

		if len(path) > 1 {
			a.path = path[1:] // Exclude the starting square
			a.isMoving = true
			// Clear any pending move since we're starting a new path
			a.pendingGoal = nil
			a.pendingGrid = nil
			// Immediately enter walking animation when movement starts (interrupt sit/std)
			if a.Engine != nil {
				a.Engine.Walk()
			}
		}
	}
}

// MoveDirectToSquare moves directly to the target square with walking animation, without pathfinding.
// This is used for rendering remote avatars' movement based on server-provided tiles.
func (a *IsoAvatar) MoveDirectToSquare(goal *IsoSquare) {
	if a == nil || goal == nil {
		return
	}
	// Use the square we're currently moving towards (if any) as the starting point for direction calculation
	// This fixes the bug where rapid AV_MV messages calculate direction from the wrong starting position
	var fromSquare *IsoSquare
	if a.nextSquare != nil && a.isMovingToPoint {
		// Avatar is already moving to a square, use that as the starting point
		fromSquare = a.nextSquare
		log.Printf("IsoAvatar: chained movement from pending [%d,%d] to [%d,%d]",
			fromSquare.Row, fromSquare.Col, goal.Row, goal.Col)
	} else {
		// Avatar is stationary, use current square
		fromSquare = a.Square
	}

	a.nextSquare = goal
	a.targetScreenPos = goal.GetViewCenter()
	a.isMovingToPoint = true
	a.isMoving = true
	if a.Engine != nil {
		a.Engine.Walk()
	}
	// Update facing direction to match movement
	if dir, ok := getDirectionToSquare(fromSquare, goal); ok {
		// Use direction directly (server/iso convention)
		a.Engine.SetDirection(dir)
		log.Printf("AvatarEngine: set direction to %d", dir)
	}
	a.lastUpdate = time.Now()
}

// Update processes avatar movement and animations.
func (a *IsoAvatar) Update() {
	// Initialize timing on first update to avoid large dt
	now := time.Now()
	if a.lastUpdate.IsZero() {
		a.lastUpdate = now
	}
	dt := now.Sub(a.lastUpdate).Seconds()
	if dt < 0 {
		dt = 0
	}
	if a.isMovingToPoint {
		// Vector from current to target
		dx := a.targetScreenPos.X - a.screenPos.X
		dy := a.targetScreenPos.Y - a.screenPos.Y
		dist := math.Sqrt(dx*dx + dy*dy)

		// Calculate speed adjustment for diagonal movement
		// The Lingo implementation calculates speed based on grid distance but
		// the isometric transformation makes screen distance shorter for diagonals.
		// This creates the visual effect of faster diagonal movement.
		speedMultiplier := 1.0
		if a.Square != nil && a.nextSquare != nil {
			dRow := a.nextSquare.Row - a.Square.Row
			dCol := a.nextSquare.Col - a.Square.Col

			// Check if this is a diagonal movement
			if math.Abs(float64(dRow)) > 0 && math.Abs(float64(dCol)) > 0 {
				// Diagonal movement detected
				// In the original Lingo code:
				// - Speed is calculated as: iDistance / iNumFrames
				// - iDistance uses grid distance (sqrt(dx^2 + dy^2))
				// - But visual distance is affected by isometric transform:
				//   calcViewY divides by 2: (y * cosX - ((z * cosY - x * sinY) / 2))
				// This makes diagonal movements (especially SE/NW) appear faster

				// Left-to-right diagonal (SE direction: +row, +col) appears fastest
				// due to how the isometric transformation compresses this direction
				if dRow > 0 && dCol > 0 {
					// Southeast diagonal (left-to-right, moving down-right on screen)
					speedMultiplier = 1.8
				} else if dRow < 0 && dCol < 0 {
					// Northwest diagonal (right-to-left, moving up-left on screen)
					speedMultiplier = 1.8
				} else {
					// NE or SW diagonals
					speedMultiplier = 1.2
				}
			}
		}

		// Move distance allowed this frame based on real time
		moveDist := a.speed * speedMultiplier * dt

		if dist > moveDist {
			// Move towards target
			ratio := moveDist / dist
			a.screenPos.X += dx * ratio
			a.screenPos.Y += dy * ratio
			// Keep engine position in sync with rendered position
			a.Engine.SetPosition(a.screenPos)
		} else {
			// Reached target point
			a.screenPos = a.targetScreenPos
			a.isMovingToPoint = false
			// Commit square change upon arrival (Lingo parity)
			if a.nextSquare != nil {
				log.Printf("IsoAvatar: arrived at [%d,%d]",
					a.nextSquare.Row, a.nextSquare.Col,
				)
				a.Square = a.nextSquare
				a.nextSquare = nil
			}
			a.Engine.SetPosition(a.screenPos)

			// Check if we have a pending move to process
			if a.pendingGoal != nil && a.pendingGrid != nil {
				log.Printf("IsoAvatar: processing pending move from [%d,%d] to [%d,%d]",
					valueOrZero(a.Square, func(s *IsoSquare) int { return s.Row }),
					valueOrZero(a.Square, func(s *IsoSquare) int { return s.Col }),
					valueOrZero(a.pendingGoal, func(s *IsoSquare) int { return s.Row }),
					valueOrZero(a.pendingGoal, func(s *IsoSquare) int { return s.Col }),
				)
				// Clear current path and calculate new one from current position
				a.path = nil
				goal := a.pendingGoal
				grid := a.pendingGrid
				a.pendingGoal = nil
				a.pendingGrid = nil
				// Start new path from current square
				a.MoveToSquare(goal, grid)
			} else {
				a.processPath() // Move to next path segment
			}
		}
	} else if a.isMoving {
		a.processPath()
	}
	// Update last timestamp after processing
	a.lastUpdate = now
}

// StopMovement immediately cancels any ongoing or pending movement and returns the avatar to standing.
func (a *IsoAvatar) StopMovement() {
	if a == nil {
		return
	}
	a.path = nil
	a.isMoving = false
	a.isMovingToPoint = false
	a.nextSquare = nil
	a.pendingGoal = nil
	a.pendingGrid = nil
	// Keep position where it is and ensure engine shows standing animation
	if a.Engine != nil {
		a.Engine.Stand()
	}
}

func (a *IsoAvatar) processPath() {
	if len(a.path) == 0 {
		if a.isMoving {
			a.isMoving = false
			a.Engine.Stand()
		}
		return
	}

	// Set next target square; commit to a.Square when we arrive
	targetSquare := a.path[0]
	a.path = a.path[1:]

	fromSquare := a.Square

	a.nextSquare = targetSquare
	a.targetScreenPos = targetSquare.GetViewCenter()
	a.isMovingToPoint = true
	// Ensure walking animation is active for each movement segment
	if a.Engine != nil {
		a.Engine.Walk()
	}
	// Reset timing to avoid carrying over a long idle dt into the first movement step
	a.lastUpdate = time.Now()

	// Update direction to face movement, matching Lingo logic
	if dir, ok := getDirectionToSquare(fromSquare, targetSquare); ok {
		// Use direction directly (server/iso convention)
		a.Engine.SetDirection(dir)
	}
}

// SetSquare sets the avatar's current square and screen position.
func (a *IsoAvatar) SetSquare(s *IsoSquare) {
	a.Square = s
	a.screenPos = s.GetViewCenter()
	a.targetScreenPos = a.screenPos
	a.Engine.SetPosition(a.screenPos)
}

// IsMoving returns true if the avatar is currently moving.
func (a *IsoAvatar) IsMoving() bool {
	return a.isMoving
}

// getDirectionToSquare mirrors Lingo IsoGrid.getDirectionToSquare()
// Returns (direction, true) or (0, false) if from==to or inputs are nil.
//
// The issue was that the avatar engine interprets directions differently than expected.
// After analyzing the user's report, there's a consistent pattern showing
// the avatar faces 90° clockwise from the intended direction.
// We need to rotate our direction indices 90° counter-clockwise to compensate.
func getDirectionToSquare(from, to *IsoSquare) (int, bool) {
	if from == nil || to == nil {
		return 0, false
	}

	rowOffset := clampToUnit(to.Row - from.Row)
	colOffset := clampToUnit(to.Col - from.Col)

	if rowOffset == 0 && colOffset == 0 {
		return 0, false
	}

	// Original Lingo mapping (what we had before):
	// North (row-1, col+0) → 0
	// East (row+0, col+1) → 2
	// South (row+1, col+0) → 4
	// West (row+0, col-1) → 6
	// NE (row-1, col+1) → 1
	// SE (row+1, col+1) → 3
	// SW (row+1, col-1) → 5
	// NW (row-1, col-1) → 7

	// But user reports avatars face 90° clockwise from expected, so we rotate 90° CCW:
	// Direction rotation: 0→6, 1→7, 2→0, 3→1, 4→2, 5→3, 6→4, 7→5

	if rowOffset == 0 {
		if colOffset == 1 {
			// East: was 2, rotate CCW → 0 (North)
			return 0, true
		}
		// West: was 6, rotate CCW → 4 (South)
		return 4, true
	}

	if colOffset == 0 {
		if rowOffset == 1 {
			// South: was 4, rotate CCW → 2 (East)
			return 2, true
		}
		// North: was 0, rotate CCW → 6 (West)
		return 6, true
	}

	if rowOffset == -1 { // moving northward
		if colOffset == 1 {
			// NE: was 1, rotate CCW → 7 (NW)
			return 7, true
		}
		// NW: was 7, rotate CCW → 5 (SW)
		return 5, true
	}

	// rowOffset == 1 (moving southward)
	if colOffset == 1 {
		// SE: was 3, rotate CCW → 1 (NE)
		return 1, true
	}
	// SW: was 5, rotate CCW → 3 (SE)
	return 3, true
}

func clampToUnit(v int) int {
	if v < 0 {
		return -1
	}
	if v > 0 {
		return 1
	}
	return 0
}

func (a *IsoAvatar) GetSquare() *IsoSquare {
	return a.Square
}

// GetNextCommittedSquare returns the square the avatar will be on after the current movement step.
// If currently moving to a square, returns that square. Otherwise returns the current square.
// This is used for calculating new paths during movement.
func (a *IsoAvatar) GetNextCommittedSquare() *IsoSquare {
	if a.nextSquare != nil && a.isMovingToPoint {
		return a.nextSquare
	}
	return a.Square
}

// HasPendingMove returns true if there's a pending move queued
func (a *IsoAvatar) HasPendingMove() bool {
	return a.pendingGoal != nil
}

func (a *IsoAvatar) GetScreenPos() core.Point2D {
	return a.screenPos
}

// GetScreenName returns the avatar's display name used in chat bubbles
func (a *IsoAvatar) GetScreenName() string {
	return a.screenName
}

// GetMission returns the avatar's mission/status string
func (a *IsoAvatar) GetMission() string {
	return a.mission
}

// GetDepth calculates the avatar's rendering depth based on its current square
// Matches Lingo IsoAvatar.calculateZOrder() behavior for "av" draw order
func (a *IsoAvatar) GetDepth() int {
	if a.Square == nil {
		return 0
	}

	// Simple depth calculation: avatars use layer 7 ("av" layer)
	baseDepth := (a.Square.GetRow() * 100) + a.Square.GetCol()
	avatarDepthOffset := 20000 // Adjust this value to tweak avatar depth relative to walls
	return baseDepth + 7 + avatarDepthOffset
}

func debugPathString(path []*IsoSquare) string {
	if len(path) == 0 {
		return "[]"
	}
	max := len(path)
	if max > 40 {
		max = 40
	}
	s := "["
	for i := 0; i < max; i++ {
		s += "(" + itoa(path[i].Row) + "," + itoa(path[i].Col) + ")"
		if i != max-1 {
			s += " -> "
		}
	}
	if max < len(path) {
		s += " -> ..."
	}
	s += "]"
	return s
}

func itoa(v int) string {
	return fmtInt(v)
}

// fmtInt is a tiny inline formatter to avoid extra imports beyond log.
func fmtInt(v int) string {
	// simple base-10 conversion
	if v == 0 {
		return "0"
	}
	n := v
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + (n % 10))
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// valueOrZero safely extracts an int from an *IsoSquare via accessor f, returning 0 if nil.
func valueOrZero(s *IsoSquare, f func(*IsoSquare) int) int {
	if s == nil {
		return 0
	}
	return f(s)
}
