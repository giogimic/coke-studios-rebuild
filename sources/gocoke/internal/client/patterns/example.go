package patterns

// This file contains example usage of the pattern system
// It shows how to integrate patterns into your existing Go code without refactoring

/*

Example usage in your studio scene:

// 1. Apply a floor pattern to a specific floor tile
err := scene.ApplyFloorPattern("floortexture1", "floor_basic", 0)
if err != nil {
    log.Printf("Failed to apply floor pattern: %v", err)
}

// 2. Apply a wall pattern to a wall tile
err = scene.ApplyWallPattern("walltile_left_1", "wall_tile", 2)
if err != nil {
    log.Printf("Failed to apply wall pattern: %v", err)
}

// 3. Get available patterns for UI
floorPatterns := scene.GetAvailableFloorPatterns()
wallPatterns := scene.GetAvailableWallPatterns()

for _, pattern := range floorPatterns {
    colorCount := scene.GetFloorPatternColorCount(pattern)
    fmt.Printf("Floor pattern: %s (%d colors)\n", pattern, colorCount)
}

// 4. Example of applying patterns based on room elements
for _, element := range scene.roomDef.Elements {
    // Apply patterns to floor texture elements
    if strings.HasPrefix(element.ID, "floortexture") {
        // Apply different patterns to different tiles
        patternIndex := rand.Intn(len(floorPatterns))
        colorIndex := rand.Intn(scene.GetFloorPatternColorCount(floorPatterns[patternIndex]))

        err := scene.ApplyFloorPattern(element.ID, floorPatterns[patternIndex], colorIndex)
        if err != nil {
            log.Printf("Failed to apply pattern to %s: %v", element.ID, err)
        }
    }
}

// 5. Example of creating a pattern selection UI
type PatternUI struct {
    scene *Scene
    selectedFloorPattern string
    selectedFloorColor   int
    selectedWallPattern  string
    selectedWallColor    int
}

func (ui *PatternUI) Update() {
    // Handle user input for pattern selection
    if ebiten.IsKeyPressed(ebiten.KeyF1) {
        // Cycle through floor patterns
        patterns := ui.scene.GetAvailableFloorPatterns()
        // ... cycle logic
    }

    if ebiten.IsKeyPressed(ebiten.KeyF2) {
        // Cycle through floor colors
        colorCount := ui.scene.GetFloorPatternColorCount(ui.selectedFloorPattern)
        ui.selectedFloorColor = (ui.selectedFloorColor + 1) % colorCount

        // Apply to all floor tiles
        for _, element := range ui.scene.roomDef.Elements {
            if strings.HasPrefix(element.ID, "floortexture") {
                ui.scene.ApplyFloorPattern(element.ID, ui.selectedFloorPattern, ui.selectedFloorColor)
            }
        }
    }
}

// 6. Example of preview system (similar to Lingo's confirm_texture.window)
type PatternPreview struct {
    patternMgr *Manager
    previewImg *ebiten.Image
}

func (p *PatternPreview) GenerateFloorPreview(patternName string, colorIndex int) *ebiten.Image {
    // Create a preview image showing the pattern
    previewImg := ebiten.NewImage(100, 100)

    // Get pattern properties
    if patterns, exists := p.patternMgr.floorPatterns[patternName]; exists && colorIndex < len(patterns) {
        props := patterns[colorIndex]

        // Create preview using pattern properties
        // This would involve rendering the pattern similar to Lingo's preview system
        _ = props // Use properties to generate preview
    }

    return previewImg
}

*/
