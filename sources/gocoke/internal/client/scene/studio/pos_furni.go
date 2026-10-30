package studio

import (
	"fmt"
	"strings"
	"time"
)

// contains checks if a slice contains a specific integer value
func contains(slice []int, val int) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// PlaceFromServer places or updates furniture coming from a server POS packet.
// It reuses the same placement/offset/stacking logic as manual placement.
func (ph *PlacementHelper) PlaceFromServer(posID, prodID string, serverX, serverY, serverZ, serverHO, dir, state int, iaot bool) bool {
	if ph == nil || ph.scene == nil || ph.scene.isoScene == nil || ph.scene.catalog == nil {
		return false
	}
	clientRow := serverX
	clientCol := 16 - serverZ

	fmt.Printf("[POS] PlaceFromServer: server[x=%d,y=%d,z=%d,ho=%d] -> client[row=%d,col=%d] for pos=%s prod=%s\n",
		serverX, serverY, serverZ, serverHO, clientRow, clientCol, posID, prodID)

	ci := ph.scene.catalog.GetItemByID(strings.TrimSpace(prodID))
	if ci == nil {
		return false
	}

	// Special handling for wall items (posters): server sends pixel coordinates (x,y,z)
	if strings.EqualFold(ci.Type, "poster") && (serverX > 100 || serverY > 100) {
		// Reverse the Shockwave placement offsets so we render at the original mouse location
		// These must mirror placement.go (px := mx + 3, py := my + 61, pz := my + 14)
		drawX := serverX - 3
		drawY := serverY - 61
		// Determine wall side from the corrected pixel position (dir from server is 0 for posters)
		wallSide := "right"
		if ws, ok := ph.getWallUnderMouse(drawX, drawY); ok {
			wallSide = ws
		} else {
			// Fallback to numeric dir if detection fails
			if dir == 6 {
				wallSide = "left"
			} else if dir == 2 {
				wallSide = "right"
			}
		}
		// If we already have this PosID locally (from optimistic place), update it instead of appending
		trimPos := strings.TrimSpace(posID)
		for i := range ph.scene.placedFurni {
			ex := &ph.scene.placedFurni[i]
			if strings.TrimSpace(ex.PosID) == trimPos {
				ex.ProdID = strings.TrimSpace(prodID)
				ex.Base = strings.TrimSpace(ci.ImageBase)
				ex.IsWallItem = true
				// Preserve the original wall side chosen at placement; server d is 0 for posters
				ex.WallX = float64(serverX - 3)
				ex.WallY = float64(serverY - 61)
				ex.State = state
				ex.Dir = 0 // posters use dir=0 for rendering logic
				ex.FPS = 8
				ex.Height = ci.Height
				if serverHO > 0 {
					ex.ServerHO = serverHO
				}
				ex.IAOT = iaot
				ex.IAOTProvided = true
				return true
			}
		}

		// Otherwise, create it fresh
		pf := PlacedFurni{
			ProdID:        strings.TrimSpace(prodID),
			PosID:         strings.TrimSpace(posID),
			Base:          strings.TrimSpace(ci.ImageBase),
			Row:           0,
			Col:           0,
			Width:         1,
			Depth:         1,
			Dir:           0,
			GridY:         0,
			IsWallItem:    true,
			State:         state,
			Animate:       false,
			WallDirection: wallSide,
			WallX:         float64(drawX),
			WallY:         float64(drawY),
		}
		pf.FPS = 8
		pf.Height = ci.Height
		if serverHO > 0 {
			pf.ServerHO = serverHO
		}
		pf.IAOT = iaot
		pf.IAOTProvided = true
		ph.scene.placedFurni = append(ph.scene.placedFurni, pf)
		// Wall items don't affect floor walkability; just place and return
		return true
	}

	base := strings.TrimSpace(ci.ImageBase)
	width := ci.Width
	depth := ci.Depth
	if width <= 0 || depth <= 0 {
		if w2, d2, ok := ph.inferDimensionsFromMembers(base); ok {
			width, depth = w2, d2
		}
	}
	// Ensure orientation parity with local rotation logic: starting from dir 2 as baseline,
	// apply a dimension swap for odd 90° rotations (dir 4 or dir 0). Keep as-is for dir 2 or 6.
	if width != depth && (dir == 4 || dir == 0) {
		width, depth = depth, width
	}

	// Keep server-provided origin as authoritative; manual placement already stores top-left.
	if width <= 0 {
		width = 1
	}
	if depth <= 0 {
		depth = 1
	}

	if dir != 0 && dir != 2 && dir != 4 && dir != 6 {
		dir = 2
	}

	// Outbound placement and rotation add one column for a 2x2 item in every
	// orientation. Invert that once, for both snapshots and updates; rotating
	// must not shift the saved row or make later owner moves invisible.
	if width == 2 && depth == 2 {
		clientCol--
	}

	// Trust server-provided coordinates for placed furniture; do not apply hover-time
	// origin heuristics (rugs/2x2 center) that are intended for manual placement.

	stackLevel := 0
	isRug := strings.Contains(strings.ToLower(base), "rug") || strings.Contains(strings.ToLower(base), "hardwood")
	computedHO := 0
	if !isRug && len(ph.scene.placedFurni) > 0 {
		newCovered := ph.coverageSquares(clientRow, clientCol, width, depth, dir)
		topGY := -1
		for _, existing := range ph.scene.placedFurni {
			existingCovered := ph.coverageSquares(existing.Row, existing.Col, existing.Width, existing.Depth, existing.Dir)
			if squaresOverlap(newCovered, existingCovered) && ph.scene.allowsItemsOnTop(existing) {
				if existing.GridY+10 > stackLevel {
					stackLevel = existing.GridY + 10
				}
				if existing.GridY > topGY {
					topGY = existing.GridY
					computedHO = existing.Height
				}
			}
		}
	}

	trimPos := strings.TrimSpace(posID)
	if trimPos != "" {
		for i := range ph.scene.placedFurni {
			ex := &ph.scene.placedFurni[i]
			if strings.TrimSpace(ex.PosID) == trimPos {
				oldRow, oldCol := ex.Row, ex.Col
				oldW, oldD := ex.Width, ex.Depth
				oldDir := ex.Dir
				oldCovered := ph.coverageSquares(oldRow, oldCol, oldW, oldD, oldDir)

				ex.ProdID = strings.TrimSpace(prodID)
				ex.Base = base
				ex.Row = clientRow
				ex.Col = clientCol
				ex.Width = width
				ex.Depth = depth
				ex.Dir = dir
				ex.State = state
				ex.Height = ci.Height
				ex.FPS = 8
				ph.computeNumFrames(ex)
				// Cache server-provided height offset (ho) for consistent usage
				if serverHO > 0 {
					ex.ServerHO = serverHO
				}
				// Preserve server IAOT flag for this item
				ex.IAOT = iaot
				ex.IAOTProvided = true
				if ex.State == 1 && ex.NumFrames > 0 {
					ex.Animate = true
					if ex.Frame < 1 || ex.Frame > ex.NumFrames {
						ex.Frame = 1
					}
					ex.LastFrameTS = time.Now().UnixMilli()
				} else {
					ex.Animate = false
					ex.Frame = 0
				}
				ph.scene.applySofaSliceBoosts(*ex)
				ph.scene.applyPinballSliceBoosts(*ex)
				ph.scene.applyVanillaCokeTableSliceBoosts(*ex)

				// Minimal delta offset parity (sofas)
				isSofa := false
				if ph.scene.catalog != nil {
					if ci2 := ph.scene.catalog.GetItemByID(strings.TrimSpace(ex.ProdID)); ci2 != nil && strings.EqualFold(ci2.Type, "Sofa") {
						isSofa = true
					}
				}
				if !isSofa {
					b := strings.ToLower(ex.Base)
					isSofa = strings.Contains(b, "sofa") || strings.Contains(b, "couch")
				}
				hasDir0 := ph.scene.hasAssetsForDirection(ex, 0)
				hasDir2 := ph.scene.hasAssetsForDirection(ex, 2)
				hasDir4 := ph.scene.hasAssetsForDirection(ex, 4)
				hasDir6 := ph.scene.hasAssetsForDirection(ex, 6)
				fmt.Printf("[POS] ASSET CHECK: base=%s hasDir0=%t hasDir2=%t hasDir4=%t hasDir6=%t\n",
					ex.Base, hasDir0, hasDir2, hasDir4, hasDir6)
				stepOffset := func(od, nd int) (int, int) {
					fmt.Printf("[POS] stepOffset check: od=%d nd=%d hasDir2=%t hasDir4=%t hasDir0=%t hasDir6=%t\n",
						od, nd, hasDir2, hasDir4, hasDir0, hasDir6)
					if (ex.Width == 2 || ex.Depth == 2) && isSofa {
						switch {
						case od == 2 && nd == 4:
							return 72, 0
						case od == 4 && nd == 6:
							return 0, 0
						case od == 6 && nd == 0:
							return -72, 0
						case od == 0 && nd == 2:
							return 0, 0
						}
						return 0, 0
					} else if ex.Width == 2 && ex.Depth == 2 {
						switch {
						case od == 2 && nd == 4:
							return -72, 0
						case od == 4 && nd == 6:
							return 72, 0
						case od == 6 && nd == 0:
							return -72, 0
						case od == 0 && nd == 2:
							return 72, 0
						}
						return 0, 0
					} else if hasDir0 && hasDir2 && hasDir4 && hasDir6 {
						return 0, 0
					} else if (ex.Width == 2 || ex.Depth == 2) && hasDir2 && hasDir0 && !hasDir6 {
						// coke machine style (2,0 but not 6)
						switch {
						case od == 2 && nd == 4:
							return 0, 0
						case od == 4 && nd == 6:
							return 72, 0
						case od == 6 && nd == 0:
							return -72, 0
						case od == 0 && nd == 2:
							return 0, 0
						}
						return 0, 0
					} else if (ex.Width == 2 || ex.Depth == 2) && hasDir2 && hasDir6 && !hasDir0 {
						// 2 & 6 but not 0
						switch {
						case od == 2 && nd == 4:
							return 108, -18
						case od == 4 && nd == 6:
							return -108, 18
						case od == 6 && nd == 0:
							return 108, -18
						case od == 0 && nd == 2:
							return -108, 18
						}
						return 0, 0
					} else if ex.Width == 3 && ex.Depth == 3 && ph.scene.isRugFurniture(*ex) {
						switch {
						case od == 0 && nd == 6:
							return 144, -36
						case od == 6 && nd == 0:
							return -144, 36
						}
						return 0, 0
					} else if hasDir0 && !hasDir6 {
						// 1x1 has 2,0; hflip 2->4 and 0->6
						switch {
						case od == 2 && nd == 4:
							return 72, 0
						case od == 4 && nd == 6:
							return 0, 0
						case od == 6 && nd == 0:
							return -72, 0
						case od == 0 && nd == 2:
							return 0, 0
						}
						return 0, 0
					} else if hasDir6 && !hasDir0 {
						// 1x1 has 2,6; hflip 2->4 and 6->0
						switch {
						case od == 2 && nd == 4:
							return 72, 0
						case od == 4 && nd == 6:
							return -72, 0
						case od == 6 && nd == 0:
							return 72, 0
						case od == 0 && nd == 2:
							return -72, 0
						}
						return 0, 0
					} else if (ex.Width == 2 || ex.Depth == 2) && hasDir2 && hasDir4 && !hasDir0 && !hasDir6 {
						// only 2 and 4 assets - no offset needed for 2-unit furniture
						return 0, 0
					} else if hasDir2 && hasDir4 && !hasDir0 && !hasDir6 {
						// 1x1 furniture with only dir 2 and dir 4 assets
						switch {
						case od == 2 && nd == 4:
							return 72, 0
						case od == 4 && nd == 2:
							return -72, 0
						}
						return 0, 0
					} else if hasDir2 && !hasDir0 && !hasDir6 && !hasDir4 {
						// only dir 2 assets (hflipped for other directions)
						switch {
						case od == 2 && nd == 4:
							return 72, 0
						case od == 4 && nd == 2:
							return -72, 0
						}
						return 0, 0
					}
					return 0, 0
				}
				if oldDir != ex.Dir {
					accX, accY := 0, 0
					curr := oldDir
					fmt.Printf("[POS] offset calc: base=%s dir %d->%d hasDir0=%t hasDir2=%t hasDir4=%t hasDir6=%t\n",
						ex.Base, oldDir, ex.Dir, hasDir0, hasDir2, hasDir4, hasDir6)

					// Special case: direct 4->2 or 2->4 for items with only dir 2 assets
					if hasDir2 && !hasDir0 && !hasDir4 && !hasDir6 {
						if (oldDir == 4 && ex.Dir == 2) || (oldDir == 2 && ex.Dir == 4) {
							dx, dy := stepOffset(oldDir, ex.Dir)
							fmt.Printf("[POS] direct step %d->%d offset=(%d,%d)\n", oldDir, ex.Dir, dx, dy)
							accX += dx
							accY += dy
						}
					} else {
						// Normal step-by-step rotation
						for curr != ex.Dir {
							next := (curr + 2) % 8
							dx, dy := stepOffset(curr, next)
							fmt.Printf("[POS] step %d->%d offset=(%d,%d)\n", curr, next, dx, dy)
							accX += dx
							accY += dy
							curr = next
						}
					}
					if accX != 0 || accY != 0 {
						ex.PixelOffsetX += accX
						ex.PixelOffsetY += accY
						fmt.Printf("[POS] applied total offset (%d,%d) new total=(%d,%d)\n",
							accX, accY, ex.PixelOffsetX, ex.PixelOffsetY)
					} else {
						fmt.Printf("[POS] no offset applied\n")
					}
				}

				newCovered := ph.coverageSquares(ex.Row, ex.Col, ex.Width, ex.Depth, ex.Dir)
				affected := make(map[[2]int]bool)
				for _, sq := range oldCovered {
					if sq != nil {
						affected[[2]int{sq.GetRow(), sq.GetCol()}] = true
					}
				}
				for _, sq := range newCovered {
					if sq != nil {
						affected[[2]int{sq.GetRow(), sq.GetCol()}] = true
					}
				}
				for rc := range affected {
					ph.updateSquareWalkability(rc[0], rc[1])
				}

				fmt.Printf("[POS] update id=%s prod=%s base=%s server(%d,%d,%d) -> client(row=%d,col=%d) dir %d->%d\n",
					strings.TrimSpace(posID), strings.TrimSpace(prodID), base, serverX, serverY, serverZ, clientRow, clientCol, oldDir, ex.Dir)
				return true
			}
		}
	}

	pf := PlacedFurni{
		ProdID:     strings.TrimSpace(prodID),
		PosID:      strings.TrimSpace(posID),
		Base:       base,
		Row:        clientRow,
		Col:        clientCol,
		Width:      width,
		Depth:      depth,
		Dir:        dir,
		GridY:      stackLevel,
		IsWallItem: ph.isWallItem(prodID),
		State:      state,
		Animate:    false,
	}
	pf.FPS = 8
	ph.computeNumFrames(&pf)
	if pf.State == 1 && pf.NumFrames > 0 {
		pf.Animate = true
		pf.Frame = 1
		pf.LastFrameTS = time.Now().UnixMilli()
	}
	pFHeight := ci.Height
	pf.Height = pFHeight
	if computedHO > 0 {
		pf.ServerHO = computedHO
	}
	if strings.TrimSpace(pf.PosID) == "" {
		pf.PosID = ph.makeUniquePosID("pos")
	} else {
		exists := false
		for _, e := range ph.scene.placedFurni {
			if strings.TrimSpace(e.PosID) == strings.TrimSpace(pf.PosID) {
				exists = true
				break
			}
		}
		if exists {
			pf.PosID = ph.makeUniquePosID(pf.PosID)
		}
	}

	ph.scene.applySofaSliceBoosts(pf)
	ph.scene.applyPinballSliceBoosts(pf)
	ph.scene.applyVanillaCokeTableSliceBoosts(pf)
	if ph.scene != nil {
		ph.scene.applyInitialDirectionOffsets(&pf)
	}

	// Cache server-provided height offset (ho) for new items
	if serverHO > 0 {
		pf.ServerHO = serverHO
	}
	pf.IAOT = iaot
	pf.IAOTProvided = true
	fmt.Printf("[POS] place id=%s prod=%s base=%s server(%d,%d,%d,ho=%d) -> client(row=%d,col=%d) dir=%d state=%d\n",
		strings.TrimSpace(posID), strings.TrimSpace(prodID), base, serverX, serverY, serverZ, serverHO, clientRow, clientCol, dir, state)

	ph.scene.placedFurni = append(ph.scene.placedFurni, pf)
	covered := ph.coverageSquares(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
	for _, csq := range covered {
		if csq != nil {
			ph.updateSquareWalkability(csq.GetRow(), csq.GetCol())
		}
	}
	return true
}

// applyInitialDirectionOffsets accumulates per-step rotation offsets starting from dir 2
// to match rotation-time pixel corrections for items created at a non-2 direction.
func (s *Scene) applyInitialDirectionOffsets(pf *PlacedFurni) {
	if pf == nil {
		return
	}
	target := pf.Dir
	if target != 0 && target != 2 && target != 4 && target != 6 {
		return
	}
	if target == 2 {
		return
	}

	isSofaType := false
	if s.catalog != nil {
		if ci := s.catalog.GetItemByID(strings.TrimSpace(pf.ProdID)); ci != nil && strings.EqualFold(ci.Type, "Sofa") {
			isSofaType = true
		}
	}
	if !isSofaType {
		b := strings.ToLower(pf.Base)
		isSofaType = strings.Contains(b, "sofa") || strings.Contains(b, "couch")
	}

	hasDir0 := s.hasAssetsForDirection(pf, 0)
	hasDir2 := s.hasAssetsForDirection(pf, 2)
	hasDir4 := s.hasAssetsForDirection(pf, 4)
	hasDir6 := s.hasAssetsForDirection(pf, 6)

	stepOffset := func(od, nd int) (int, int) {
		if (pf.Width == 2 || pf.Depth == 2) && isSofaType {
			switch {
			case od == 2 && nd == 4:
				return 72, 0
			case od == 4 && nd == 6:
				return 0, 0
			case od == 6 && nd == 0:
				return -72, 0
			case od == 0 && nd == 2:
				return 0, 0
			}
			return 0, 0
		} else if pf.Width == 2 && pf.Depth == 2 {
			switch {
			case od == 2 && nd == 4:
				return 0, 0
			case od == 4 && nd == 6:
				return 0, 0
			case od == 6 && nd == 0:
				return 0, 0
			case od == 0 && nd == 2:
				return 0, 0
			}
			return 0, 0
		} else if hasDir0 && hasDir2 && hasDir4 && hasDir6 {
			return 0, 0
		} else if (pf.Width == 2 || pf.Depth == 2) && hasDir2 && hasDir0 && !hasDir6 {
			switch {
			case od == 2 && nd == 4:
				return 0, 0
			case od == 4 && nd == 6:
				return 72, 0
			case od == 6 && nd == 0:
				return -72, 0
			case od == 0 && nd == 2:
				return 0, 0
			}
			return 0, 0
		} else if (pf.Width == 2 || pf.Depth == 2) && hasDir2 && hasDir6 && !hasDir0 {
			switch {
			case od == 2 && nd == 4:
				return 108, -18
			case od == 4 && nd == 6:
				return -108, 18
			case od == 6 && nd == 0:
				return 108, -18
			case od == 0 && nd == 2:
				return -108, 18
			}
			return 0, 0
		} else if pf.Width == 3 && pf.Depth == 3 && s.isRugFurniture(*pf) {
			switch {
			case od == 0 && nd == 6:
				return 144, -36
			case od == 6 && nd == 0:
				return -144, 36
			}
			return 0, 0
		} else if hasDir0 && !hasDir6 {
			switch {
			case od == 2 && nd == 4:
				return 72, 0
			case od == 4 && nd == 6:
				return 0, 0
			case od == 6 && nd == 0:
				return -72, 0
			case od == 0 && nd == 2:
				return 0, 0
			}
			return 0, 0
		} else if hasDir6 && !hasDir0 {
			switch {
			case od == 2 && nd == 4:
				return 72, 0
			case od == 4 && nd == 6:
				return -72, 0
			case od == 6 && nd == 0:
				return 72, 0
			case od == 0 && nd == 2:
				return -72, 0
			}
			return 0, 0
		} else if (pf.Width == 2 || pf.Depth == 2) && hasDir2 && hasDir4 && !hasDir0 && !hasDir6 {
			// 2-unit furniture with only dir 2 and dir 4 assets - no offset needed
			return 0, 0
		} else if hasDir2 && hasDir4 && !hasDir0 && !hasDir6 {
			// 1x1 furniture with only dir 2 and dir 4 assets
			switch {
			case od == 2 && nd == 4:
				return 72, 0
			case od == 4 && nd == 2:
				return -72, 0
			}
			return 0, 0
		} else if hasDir2 && !hasDir0 && !hasDir6 && !hasDir4 {
			// only dir 2 assets (hflipped for other directions)
			switch {
			case od == 2 && nd == 4:
				return 72, 0
			case od == 4 && nd == 2:
				return -72, 0
			}
			return 0, 0
		}
		return 0, 0
	}

	accumX, accumY := 0, 0
	curr := 2
	for curr != target {
		next := (curr + 2) % 8
		dx, dy := stepOffset(curr, next)
		accumX += dx
		accumY += dy
		curr = next
	}
	if accumX != 0 || accumY != 0 {
		pf.PixelOffsetX += accumX
		pf.PixelOffsetY += accumY
	}
}
