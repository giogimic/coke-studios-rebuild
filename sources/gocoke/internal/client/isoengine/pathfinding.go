package isoengine

import (
	"container/heap"
	"math"
)

// node is an internal representation for A* pathfinding.
type node struct {
	parent  *node
	square  *IsoSquare
	g, h, f float64
	index   int // The index of the item in the priority queue.
}

// astar implements the A* pathfinding algorithm matching Lingo's AStar.ls implementation.
func astar(start, goal *IsoSquare, grid *IsoGrid) []*IsoSquare {
	openSet := make(priorityQueue, 0)
	heap.Init(&openSet)

	closedSet := make(map[*IsoSquare]bool)

	// Lingo: s.g = 0, s.h = getHeuristic(pt1, pt2), s.f = s.g + s.h
	startNode := &node{square: start, g: 0, h: heuristic(start, goal)}
	startNode.f = startNode.g + startNode.h
	heap.Push(&openSet, startNode)

	for openSet.Len() > 0 {
		// Lingo: n = me.aOpen[1], me.aOpen.deleteAt(1)
		current := heap.Pop(&openSet).(*node)

		if current.square == goal {
			// Path found - construct and optionally smooth it
			return reconstructPath(current)
		}

		// Mark as closed (Lingo: n.s = n.closed)
		closedSet[current.square] = true

		// Get successors (neighbors + pointers from the map node)
		for _, neighborSquare := range grid.getNeighbors(current.square) {
			if closedSet[neighborSquare] {
				continue
			}

			// Check passability (matches Lingo's getPassable logic)
			if !isPassable(neighborSquare, neighborSquare == goal, grid) {
				continue
			}

			// Cost is always 1 in Lingo (getCost returns 1)
			stepCost := 1.0
			// Lingo: newg = n.g + me.getCost(n, nn)
			gScore := current.g + stepCost

			foundInOpen := false
			var neighborNode *node
			for _, n := range openSet {
				if n.square == neighborSquare {
					neighborNode = n
					foundInOpen = true
					break
				}
			}

			if !foundInOpen {
				// Create new node and add to open set
				neighborNode = &node{square: neighborSquare}
				neighborNode.parent = current
				neighborNode.g = gScore
				neighborNode.h = heuristic(neighborSquare, goal)
				neighborNode.f = neighborNode.g + neighborNode.h
				heap.Push(&openSet, neighborNode)
			} else if gScore < neighborNode.g {
				// Update existing node with better path
				neighborNode.parent = current
				neighborNode.g = gScore
				neighborNode.f = neighborNode.g + neighborNode.h
				heap.Fix(&openSet, neighborNode.index)
			}
		}
	}

	return nil // No path found
}

// isPassable checks if a square can be traversed, matching Lingo's getPassable logic.
// This includes checking for blocked squares, avatars, furniture, etc.
func isPassable(square *IsoSquare, isEndNode bool, grid *IsoGrid) bool {
	// Basic walkability check
	if !square.IsWalkable() {
		return false
	}

	// Lingo checks for various weights:
	// W_BLOCKED = 255 - never passable
	// W_QUEUE = 155 - only passable as end node
	// W_STAGE = 175, W_PERFORM = 180, W_EXITSTAGE = 185 - not passable
	// W_AVATAR = 50 - check for avatars at square
	// W_ITEM = 100 - check for furniture items

	// For now, we use the basic walkability check
	// Additional checks for avatars and furniture would go here
	// when those systems are fully integrated

	return true
}

// FindPath returns a path from start to goal using the scene's A* implementation.
// The returned slice includes both the start and goal squares when a path exists.
// If no path is found or inputs are invalid, it returns nil.
func FindPath(start, goal *IsoSquare, grid *IsoGrid) []*IsoSquare {
	if start == nil || goal == nil || grid == nil {
		return nil
	}
	return astar(start, goal, grid)
}

func reconstructPath(n *node) []*IsoSquare {
	// Build path from goal to start
	path := make([]*IsoSquare, 0)
	for current := n; current != nil; current = current.parent {
		path = append([]*IsoSquare{current.square}, path...)
	}

	// Apply path smoothing if enabled (Lingo: me.bSmoothPath = 1 by default)
	// Smoothing reduces waypoints by finding direct paths
	if len(path) > 2 {
		path = smoothPath(path)
	}

	return path
}

// smoothPath implements Lingo's path smoothing algorithm.
// It tries to find direct paths between points to reduce waypoints.
func smoothPath(path []*IsoSquare) []*IsoSquare {
	if len(path) <= 2 {
		return path
	}

	// Start from the beginning and try to find direct paths
	newPath := make([]*IsoSquare, 0, len(path))
	newPath = append(newPath, path[0]) // Always include start

	current := 0
	for current < len(path)-1 {
		// Try to find the furthest point we can reach directly
		furthest := current + 1
		for i := len(path) - 1; i > current+1; i-- {
			if hasDirectPath(path[current], path[i]) {
				furthest = i
				break
			}
		}

		// Add the furthest reachable point
		newPath = append(newPath, path[furthest])
		current = furthest
	}

	return newPath
}

// hasDirectPath checks if there's a clear direct path between two squares.
// This is a simplified version - Lingo checks if all intermediate squares are walkable.
func hasDirectPath(from, to *IsoSquare) bool {
	// For now, only allow direct paths along straight lines (horizontal/vertical)
	// Lingo's getDirectPath checks along straight lines only
	if from.Row != to.Row && from.Col != to.Col {
		return false // Not a straight line
	}

	// TODO: Check if all squares between from and to are walkable
	// For now, assume no direct paths to keep movement accurate
	return false
}

func heuristic(a, b *IsoSquare) float64 {
	// Lingo getHeuristic: dx = pt1.locH - pt2.locH, dy = pt1.locV - pt2.locV
	// return sqrt((dx * dx) + (dy * dy))
	// Note: In Lingo, point.locH = Col, point.locV = Row
	dx := float64(a.Col - b.Col)
	dy := float64(a.Row - b.Row)
	return math.Sqrt(dx*dx + dy*dy)
}

// priorityQueue implements heap.Interface and holds Nodes.
type priorityQueue []*node

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	return pq[i].f < pq[j].f
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*node)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}
