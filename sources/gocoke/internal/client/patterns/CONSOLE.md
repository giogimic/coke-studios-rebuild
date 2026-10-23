# Pattern Console Usage Guide

The pattern console is an interactive debug interface that lets you apply and test patterns in real-time without needing to build a full UI.

## Opening the Console

Press **F10** to toggle the console open/closed.

## Console Interface

```
┌─────────────────────────────────────────────────────────────┐
│ Pattern Console - Type 'help' for commands                 │
│ > list floor                                                │
│ Floor Patterns:                                             │
│   floor_basic (24 colors)                                  │
│   floor_woodswirl (13 colors)                              │
│   floor_orgun (15 colors)                                  │
│ > floor floortexture1 floor_basic 0                        │
│ Applied floor pattern 'floor_basic' (color 0) to 'floortexture1' │
│ > _                                                         │
├─────────────────────────────────────────────────────────────┤
│ F10: Toggle Console | ESC: Close | Enter: Execute | Up/Down: History │
└─────────────────────────────────────────────────────────────┘
```

## Available Commands

### `help`
Shows all available commands with usage examples.

### `list [floor|wall]`
Lists available patterns:
- `list` - Show all patterns (floor and wall)
- `list floor` - Show only floor patterns
- `list wall` - Show only wall patterns

### `floor <element> <pattern> <color>`
Apply a floor pattern to a specific element:
```
floor floortexture1 floor_basic 0
floor floortexture2 floor_woodswirl 2
```

### `wall <element> <pattern> <color>`
Apply a wall pattern to a specific element:
```
wall walltile_1 wall_tile 1
wall walltile_left_1 wall_graythreedo 0
```

### `apply <type> <pattern> <color> all`
Apply a pattern to all elements of that type:
```
apply floor floor_basic 0 all
apply wall wall_tile 2 all
```

### `info <pattern>`
Show information about a specific pattern:
```
info floor_basic
info wall_tile
```

### `show [assignments]`
Show current pattern assignments (displays internal state).

### `remove <element>`
Remove pattern from a specific element (not yet implemented).

### `clear`
Clear the console output.

## Element IDs

The console works with element IDs from your room definition files. Common element IDs include:

**Floor Elements:**
- `floortexture1`, `floortexture2`, etc. (individual floor tiles)
- `doorfloortexture` (door floor tile)

**Wall Elements:**
- `walltile_1`, `walltile_2`, etc. (individual wall tiles)
- `walltile_left_1`, `walltile_right_1` (directional wall tiles)

## Usage Examples

### Quick Pattern Testing
```
> list floor
> floor floortexture1 floor_basic 0
> floor floortexture2 floor_woodswirl 1
> floor floortexture3 floor_orgun 2
```

### Apply Pattern to All Floor Tiles
```
> apply floor floor_basic 0 all
Applied floor pattern 'floor_basic' (color 0) to 15 elements
```

### Pattern Information
```
> info floor_basic
Floor Pattern: floor_basic
Color variants: 24
Type: Floor pattern with gradient and texture effects
```

### Command History
- **Up Arrow**: Navigate to previous commands
- **Down Arrow**: Navigate to next commands
- **Enter**: Execute current command
- **Backspace**: Delete characters
- **ESC**: Close console

## Pattern Effects

When you apply patterns through the console:

1. **Immediate Visual Effect**: Patterns are applied instantly during the next draw cycle
2. **Persistent**: Pattern assignments remain until changed or removed
3. **Non-Destructive**: Your base tile positioning and rendering is unchanged
4. **Stackable**: You can apply different patterns to different tiles

## Troubleshooting

### "Pattern not found" Error
- Use `list` to see available patterns
- Check spelling (patterns are case-sensitive)
- Pattern names use underscores: `floor_basic`, not `floor basic`

### "Color index out of range" Error
- Use `info <pattern>` to see how many colors are available
- Color indices start at 0 (so pattern with 5 colors uses indices 0-4)

### "Element not found" Error
- Element IDs must match exactly with your room definition
- Use the exact element ID from the room file (case-sensitive)

## Integration with Your Code

The console integrates seamlessly:
- **F10 Toggle**: Added to `Scene.Update()`
- **Console Rendering**: Added to `Scene.Draw()`
- **Pattern Application**: Uses your existing pattern system
- **No Performance Impact**: Console only processes input when visible

The console is perfect for:
- **Testing Patterns**: Quickly try different pattern combinations
- **Development**: Debug pattern loading and application
- **Content Creation**: Design room layouts with different patterns
- **User Testing**: Allow users to customize their studios

Press F10 in your application to start using the pattern console!
