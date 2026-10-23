# Pattern System for Coke Studios

This package implements the wall and floor pattern system from the original Lingo Coke Studios implementation, integrated into your existing Go codebase **without requiring any refactoring**.

## Overview

The pattern system adds visual customization to floor and wall tiles by applying:
- **Floor patterns**: Gradient colors, texture overlays, and blend effects
- **Wall patterns**: Wall colors, background colors, and texture patterns
- **Position-based application**: Patterns applied to specific elements by ID

## Key Features

✅ **Zero Refactoring**: Your existing tile placement and rendering code is unchanged  
✅ **Post-Processing**: Patterns applied during the draw phase as visual effects  
✅ **Lingo Compatible**: Loads original pattern data files from `Lingo/cc_studio/`  
✅ **Position Targeting**: Apply different patterns to individual tiles  
✅ **Real-time Changes**: Update patterns without restarting the application  

## Integration

The pattern system integrates seamlessly into your existing studio scene:

### 1. Automatic Loading
```go
// Added to Scene.New() - loads pattern data automatically
scene.patternMgr.LoadPatterns(dataDir)
```

### 2. Automatic Rendering
```go
// Added to Scene.drawElement() - applies patterns during rendering
if s.patternMgr.HasPatternForElement(element.ID) {
    s.patternMgr.ApplyPatternToElement(screen, element.ID, asset, x, y)
} else {
    screen.DrawImage(asset, opts) // Your existing rendering
}
```

### 3. Public API
```go
// Apply patterns to specific elements
scene.ApplyFloorPattern("floortexture1", "floor_basic", 0)
scene.ApplyWallPattern("walltile_1", "wall_tile", 2)

// Get available patterns for UI
floorPatterns := scene.GetAvailableFloorPatterns()
wallPatterns := scene.GetAvailableWallPatterns()

// Get color variant counts
colorCount := scene.GetFloorPatternColorCount("floor_basic")
```

## Pattern Data Structure

### Floor Patterns
Loaded from `Lingo/cc_studio/122_floorpattern_patterns_index.txt` and individual pattern files:

```lingo
[#floorStyle:1, #startcolor: rgb("#33ff33"), #endcolor: rgb("#009900"), 
 #shapeBlend: 70, #palette: "basic", #texturecolor: rgb("#333366"), 
 #textureBlend: 55, #indexNum: 1001]
```

**Properties:**
- `startcolor`/`endcolor`: Gradient colors for base shape
- `shapeBlend`: Blend percentage for shape layer
- `palette`: Texture pattern name (e.g., "basic", "orgun", "woodswirl")
- `texturecolor`: Color tint for texture overlay
- `textureBlend`: Blend percentage for texture layer

### Wall Patterns
Loaded from `Lingo/cc_studio/109_wallpattern_patterns_index.txt` and pattern files:

```lingo
[#dirtStyle: 1, #color: rgb("#FFFFFF"), #bgColor: rgb("#000000"),
 #dirtBlend: 50, #palette: "tile", #textureBlend: 30]
```

**Properties:**
- `dirtStyle`: Wall style type (1 or 2)
- `color`: Primary wall color
- `bgColor`: Background wall color
- `dirtBlend`: Blend percentage for wall layer
- `palette`: Texture pattern name
- `textureBlend`: Blend percentage for texture

## File Handling

The system handles numeric prefixes in pattern files automatically:

- **Index files**: `122_floorpattern_patterns_index.txt`, `109_wallpattern_patterns_index.txt`
- **Pattern files**: `128_floorpattern_basic.txt`, `137_floorpattern_woodswirl.txt`, etc.
- **Lookup logic**: Finds files by pattern name with or without numeric prefix

## Rendering Pipeline

### Floor Rendering
1. **Base tile**: Draw existing floor tile (your code unchanged)
2. **Shape layer**: Apply gradient colors with blend mode
3. **Texture layer**: Apply pattern texture with color tint and blend

### Wall Rendering  
1. **Base tile**: Draw existing wall tile with color effects
2. **Texture layer**: Apply pattern texture with blend mode

## Usage Examples

### Basic Pattern Application
```go
// Apply floor pattern to specific tile
err := scene.ApplyFloorPattern("floortexture1", "floor_basic", 0)
if err != nil {
    log.Printf("Failed to apply floor pattern: %v", err)
}

// Apply wall pattern
err = scene.ApplyWallPattern("walltile_left_1", "wall_tile", 2)
```

### Pattern Selection UI
```go
// Get available patterns
floorPatterns := scene.GetAvailableFloorPatterns()

// Display in UI
for i, pattern := range floorPatterns {
    colorCount := scene.GetFloorPatternColorCount(pattern)
    fmt.Printf("%d. %s (%d colors)\n", i+1, pattern, colorCount)
}

// Apply user selection
scene.ApplyFloorPattern(selectedElementID, 
    floorPatterns[userPatternIndex], 
    userColorIndex)
```

### Batch Pattern Application
```go
// Apply patterns to all floor tiles
for _, element := range scene.roomDef.Elements {
    if strings.HasPrefix(element.ID, "floortexture") {
        scene.ApplyFloorPattern(element.ID, "floor_woodswirl", 1)
    }
}
```

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Your Existing │    │  Pattern System │    │  Lingo Pattern  │
│   Studio Scene  │───▶│    Manager      │───▶│   Data Files    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  drawElement()  │    │ ApplyPatternTo  │    │ *.txt pattern   │
│   (enhanced)    │───▶│   Element()     │───▶│   definitions   │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Benefits

1. **Non-Invasive**: No changes to your existing tile positioning logic
2. **Modular**: Pattern system can be disabled without affecting base functionality
3. **Performance**: Only processes patterns when assigned to elements
4. **Extensible**: Easy to add new pattern types or effects
5. **Compatible**: Uses original Lingo pattern data for authenticity

## Testing

Run the validation function to test pattern loading:

```go
err := patterns.ValidatePatternFiles(dataDir)
if err != nil {
    log.Printf("Pattern validation failed: %v", err)
}
```

This will:
- Load all pattern files
- Verify pattern data parsing
- Test pattern assignment
- Report any issues

The pattern system is now ready to use with your existing Go codebase!
