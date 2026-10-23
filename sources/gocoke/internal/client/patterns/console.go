package patterns

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"gocoke/internal/client/isoengine"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Console provides an interactive debug console for pattern management
type Console struct {
	manager     *Manager
	visible     bool
	input       string
	output      []string
	maxOutput   int
	cursorBlink int

	// Console state
	historyIndex int
	history      []string

	// Scrolling state
	scrollOffset int // Current scroll position (0 = bottom/latest)
	maxScroll    int // Maximum scroll offset

	// Available commands
	commands map[string]func([]string) string
}

// NewConsole creates a new debug console
func NewConsole(manager *Manager) *Console {
	console := &Console{
		manager:   manager,
		visible:   false,
		input:     "",
		output:    []string{"Pattern Console - Type 'help' for commands"},
		maxOutput: 20,
		history:   []string{},
		commands:  make(map[string]func([]string) string),
	}

	// Register commands
	console.registerCommands()

	return console
}

// registerCommands sets up all available console commands
func (c *Console) registerCommands() {
	c.commands["help"] = c.cmdHelp
	c.commands["list"] = c.cmdList
	c.commands["floor"] = c.cmdFloor
	c.commands["wall"] = c.cmdWall
	c.commands["clear"] = c.cmdClear
	c.commands["info"] = c.cmdInfo
	c.commands["apply"] = c.cmdApply
	c.commands["remove"] = c.cmdRemove
	c.commands["show"] = c.cmdShow
	c.commands["debug"] = c.cmdDebug
	c.commands["reload"] = c.cmdReload
	c.commands["test"] = c.cmdTest
	c.commands["elements"] = c.cmdElements
	c.commands["single"] = c.cmdSingle
	c.commands["hidetextures"] = c.cmdHideTextures
	c.commands["showtextures"] = c.cmdShowTextures
	c.commands["assets"] = c.cmdAssets
	c.commands["depths"] = c.cmdDepths
	c.commands["boost"] = c.cmdBoost
	c.commands["aboost"] = c.cmdABoost

}

// Update handles console input and state
func (c *Console) Update() {
	// Toggle console with F10
	if inpututil.IsKeyJustPressed(ebiten.KeyF10) {
		c.visible = !c.visible
		if c.visible {
			c.addOutput("Console opened - Type 'help' for commands")
		}
	}

	if !c.visible {
		return
	}

	// Update cursor blink
	c.cursorBlink++

	// Handle scrolling
	c.handleScrolling()

	// Handle input
	c.handleInput()
}

// handleScrolling processes scroll input for the console
func (c *Console) handleScrolling() {
	// Scroll up with Page Up or Up Arrow
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) || inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		c.scrollOffset++
		if c.scrollOffset > c.maxScroll {
			c.scrollOffset = c.maxScroll
		}
	}

	// Scroll down with Page Down or Down Arrow
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) || inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		c.scrollOffset--
		if c.scrollOffset < 0 {
			c.scrollOffset = 0
		}
	}

	// Home key - scroll to top
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) {
		c.scrollOffset = c.maxScroll
	}

	// End key - scroll to bottom (auto-scroll)
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) {
		c.scrollOffset = 0
	}

	// Mouse wheel scrolling
	_, wheelY := ebiten.Wheel()
	if wheelY > 0 {
		// Scroll up
		c.scrollOffset += 3
		if c.scrollOffset > c.maxScroll {
			c.scrollOffset = c.maxScroll
		}
	} else if wheelY < 0 {
		// Scroll down
		c.scrollOffset -= 3
		if c.scrollOffset < 0 {
			c.scrollOffset = 0
		}
	}
}

// handleInput processes keyboard input for the console
func (c *Console) handleInput() {
	// Handle character input
	for _, r := range ebiten.AppendInputChars(nil) {
		if r >= 32 && r <= 126 { // Printable ASCII
			c.input += string(r)
		}
	}

	// Handle special keys
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		c.executeCommand()
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(c.input) > 0 {
		c.input = c.input[:len(c.input)-1]
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		c.visible = false
	}

	// History navigation
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) && len(c.history) > 0 {
		if c.historyIndex > 0 {
			c.historyIndex--
		}
		if c.historyIndex < len(c.history) {
			c.input = c.history[c.historyIndex]
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) && len(c.history) > 0 {
		c.historyIndex++
		if c.historyIndex >= len(c.history) {
			c.historyIndex = len(c.history)
			c.input = ""
		} else {
			c.input = c.history[c.historyIndex]
		}
	}
}

// executeCommand processes and runs a console command
func (c *Console) executeCommand() {
	if c.input == "" {
		return
	}

	// Add to history
	c.history = append(c.history, c.input)
	c.historyIndex = len(c.history)

	// Parse command
	parts := strings.Fields(c.input)
	if len(parts) == 0 {
		c.input = ""
		return
	}

	command := strings.ToLower(parts[0])
	args := parts[1:]

	// Add command to output
	c.addOutput("> " + c.input)

	// Execute command
	if cmd, exists := c.commands[command]; exists {
		result := cmd(args)
		if result != "" {
			c.addOutput(result)
		}
	} else {
		c.addOutput(fmt.Sprintf("Unknown command: %s (type 'help' for commands)", command))
	}

	c.input = ""
}

// addOutput adds a line to the console output
func (c *Console) addOutput(line string) {
	c.output = append(c.output, line)
	if len(c.output) > c.maxOutput {
		c.output = c.output[1:]
	}

	// Update max scroll based on output length
	c.updateScrollLimits()

	// Auto-scroll to bottom when new output is added (unless user is scrolled up)
	if c.scrollOffset == 0 {
		c.scrollOffset = 0 // Stay at bottom
	}
}

// updateScrollLimits calculates the maximum scroll offset
func (c *Console) updateScrollLimits() {
	// Calculate how many lines can fit in the console display area
	consoleHeight := 300
	lineHeight := 14
	displayLines := (consoleHeight - 40) / lineHeight // Reserve space for input line

	// Max scroll is the number of lines that extend beyond the display area
	c.maxScroll = len(c.output) - displayLines
	if c.maxScroll < 0 {
		c.maxScroll = 0
	}
}

// Draw renders the console if visible
func (c *Console) Draw(screen *ebiten.Image) {
	if !c.visible {
		return
	}

	// Console background
	consoleHeight := 300
	consoleY := screen.Bounds().Dy() - consoleHeight

	// Draw semi-transparent background
	bg := ebiten.NewImage(screen.Bounds().Dx(), consoleHeight)
	bg.Fill(color.RGBA{0, 0, 0, 200})

	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(0, float64(consoleY))
	screen.DrawImage(bg, opts)

	// Draw output lines with scrolling
	lineHeight := 14
	startY := consoleY + 10
	maxDisplayLines := (consoleHeight - 40) / lineHeight

	// Calculate which lines to display based on scroll offset
	totalLines := len(c.output)
	startLine := totalLines - maxDisplayLines - c.scrollOffset
	if startLine < 0 {
		startLine = 0
	}
	endLine := startLine + maxDisplayLines
	if endLine > totalLines {
		endLine = totalLines
	}

	// Draw the visible lines
	for i := startLine; i < endLine; i++ {
		y := startY + (i-startLine)*lineHeight
		if y+lineHeight > screen.Bounds().Dy()-30 {
			break
		}
		ebitenutil.DebugPrintAt(screen, c.output[i], 10, y)
	}

	// Draw scroll indicator if there's more content
	if c.maxScroll > 0 {
		scrollInfo := fmt.Sprintf("Scroll: %d/%d (↑↓/PgUp/PgDn/Wheel)", c.scrollOffset, c.maxScroll)
		ebitenutil.DebugPrintAt(screen, scrollInfo, screen.Bounds().Dx()-250, consoleY+5)
	}

	// Draw input line
	inputY := screen.Bounds().Dy() - 25
	prompt := "> " + c.input

	// Add blinking cursor
	if (c.cursorBlink/30)%2 == 0 {
		prompt += "_"
	}

	ebitenutil.DebugPrintAt(screen, prompt, 10, inputY)

	// Draw help text
	helpText := "F10: Toggle Console | ESC: Close | Enter: Execute | Up/Down: History"
	ebitenutil.DebugPrintAt(screen, helpText, 10, consoleY-15)
}

// Command implementations

func (c *Console) cmdHelp(args []string) string {
	help := `Available Commands:
help                    - Show this help
list [floor|wall]       - List available patterns
floor <element> <pattern> <color> - Apply floor pattern
wall <element> <pattern> <color>  - Apply wall pattern
apply <type> <pattern> <color> all - Apply to all elements
info <pattern>          - Show pattern information
show [assignments]      - Show current assignments
remove <element>        - Remove pattern from element
clear                   - Clear console output
debug                   - Show pattern loading debug info
elements                - Show which elements have patterns applied
single <type> <element> <pattern> <color> - Apply pattern to specific element
hidetextures            - Hide floor texture layer (show only shape gradient)
showtextures            - Show floor texture layer (normal rendering)

Examples:
  list floor
  floor floortexture1 floor_basic 0
  wall walltile_1 wall_tile 2
  apply floor floor_woodswirl 1 all
  info floor_basic
  hidetextures
  showtextures`

	return help
}

func (c *Console) cmdList(args []string) string {
	if len(args) == 0 {
		floorPatterns := c.manager.GetFloorPatternNames()
		wallPatterns := c.manager.GetWallPatternNames()

		result := fmt.Sprintf("Available Patterns (Floor: %d, Wall: %d):\n\n", len(floorPatterns), len(wallPatterns))

		if len(floorPatterns) == 0 {
			result += "Floor Patterns: None loaded\n"
		} else {
			result += "Floor Patterns:\n"
			for _, pattern := range floorPatterns {
				colorCount := c.manager.GetFloorPatternColorCount(pattern)
				result += fmt.Sprintf("  %s (%d colors)\n", pattern, colorCount)
			}
		}

		if len(wallPatterns) == 0 {
			result += "\nWall Patterns: None loaded\n"
		} else {
			result += "\nWall Patterns:\n"
			for _, pattern := range wallPatterns {
				colorCount := c.manager.GetWallPatternColorCount(pattern)
				result += fmt.Sprintf("  %s (%d colors)\n", pattern, colorCount)
			}
		}

		return result
	}

	switch strings.ToLower(args[0]) {
	case "floor":
		result := "Floor Patterns:\n"
		for _, pattern := range c.manager.GetFloorPatternNames() {
			colorCount := c.manager.GetFloorPatternColorCount(pattern)
			result += fmt.Sprintf("  %s (%d colors)\n", pattern, colorCount)
		}
		return result

	case "wall":
		result := "Wall Patterns:\n"
		for _, pattern := range c.manager.GetWallPatternNames() {
			colorCount := c.manager.GetWallPatternColorCount(pattern)
			result += fmt.Sprintf("  %s (%d colors)\n", pattern, colorCount)
		}
		return result

	default:
		return "Usage: list [floor|wall]"
	}
}

func (c *Console) cmdFloor(args []string) string {
	if len(args) < 3 {
		return "Usage: floor <element> <pattern> <color>\nExample: floor floortexture1 floor_basic 0"
	}

	element := args[0]
	pattern := args[1]
	colorStr := args[2]

	color, err := strconv.Atoi(colorStr)
	if err != nil {
		return "Error: Color must be a number"
	}

	err = c.manager.ApplyFloorPattern(element, pattern, color)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Applied floor pattern '%s' (color %d) to element '%s'", pattern, color, element)
}

func (c *Console) cmdWall(args []string) string {
	if len(args) < 3 {
		return "Usage: wall <element> <pattern> <color>\nExample: wall walltile_1 wall_tile 2"
	}

	element := args[0]
	pattern := args[1]
	colorStr := args[2]

	color, err := strconv.Atoi(colorStr)
	if err != nil {
		return "Error: Color must be a number"
	}

	err = c.manager.ApplyWallPattern(element, pattern, color)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Applied wall pattern '%s' (color %d) to element '%s'", pattern, color, element)
}

func (c *Console) cmdApply(args []string) string {
	if len(args) < 4 || args[3] != "all" {
		return "Usage: apply <floor|wall> <pattern> <color> all\nExample: apply floor floor_basic 0 all"
	}

	patternType := strings.ToLower(args[0])
	pattern := args[1]
	colorStr := args[2]

	color, err := strconv.Atoi(colorStr)
	if err != nil {
		return "Error: Color must be a number"
	}

	count := 0
	if patternType == "floor" {
		// Clear debug state for fresh debugging
		c.manager.ClearDebugState()

		c.addOutput(fmt.Sprintf("=== FLOOR PATTERN APPLICATION DEBUG ==="))
		c.addOutput(fmt.Sprintf("Pattern: %s, Color Index: %d", pattern, color))

		// Get pattern properties for debugging
		if patterns, exists := c.manager.floorPatterns[pattern]; exists && color < len(patterns) {
			props := patterns[color]
			sr, sg, sb, _ := props.StartColor.RGBA()
			er, eg, eb, _ := props.EndColor.RGBA()
			tr, tg, tb, _ := props.TextureColor.RGBA()

			c.addOutput(fmt.Sprintf("Pattern Properties:"))
			c.addOutput(fmt.Sprintf("  StartColor: #%02x%02x%02x", sr>>8, sg>>8, sb>>8))
			c.addOutput(fmt.Sprintf("  EndColor: #%02x%02x%02x", er>>8, eg>>8, eb>>8))
			c.addOutput(fmt.Sprintf("  TextureColor: #%02x%02x%02x", tr>>8, tg>>8, tb>>8))
			c.addOutput(fmt.Sprintf("  ShapeBlend: %d%%", props.ShapeBlend))
			c.addOutput(fmt.Sprintf("  TextureBlend: %d%%", props.TextureBlend))
			c.addOutput(fmt.Sprintf("  Palette: %s", props.Palette))
		}

		c.addOutput(fmt.Sprintf("Applying to floor elements:"))

		// Apply to common floor element IDs
		for i := 1; i <= 21; i++ {
			elementID := fmt.Sprintf("floortexture%d", i)
			err := c.manager.ApplyFloorPattern(elementID, pattern, color)
			if err == nil {
				count++
				c.addOutput(fmt.Sprintf("  ✓ %s", elementID))
			} else {
				c.addOutput(fmt.Sprintf("  ✗ %s (error: %v)", elementID, err))
			}
		}
		// Also apply to door floor texture
		if err := c.manager.ApplyFloorPattern("doorfloortexture", pattern, color); err == nil {
			count++
			c.addOutput(fmt.Sprintf("  ✓ doorfloortexture"))
		} else {
			c.addOutput(fmt.Sprintf("  ✗ doorfloortexture (error: %v)", err))
		}

		c.addOutput(fmt.Sprintf("=== PATTERN APPLICATION COMPLETE ==="))
		c.addOutput(fmt.Sprintf("Total elements assigned: %d", count))

		// Debug: Show which elements actually exist in the scene
		c.addOutput(fmt.Sprintf("Checking which elements exist in room definition..."))

		return fmt.Sprintf("Applied floor pattern '%s' (color %d) to %d elements", pattern, color, count)
	} else if patternType == "wall" {
		// Clear debug state for fresh debugging
		c.manager.ClearDebugState()

		c.addOutput(fmt.Sprintf("=== WALL PATTERN APPLICATION DEBUG ==="))
		c.addOutput(fmt.Sprintf("Pattern: %s, Color Index: %d", pattern, color))

		// Show pattern properties for debugging
		if patterns, exists := c.manager.wallPatterns[pattern]; exists && color < len(patterns) {
			props := patterns[color]

			cr, cg, cb, _ := props.Color.RGBA()
			br, bg, bb, _ := props.BGColor.RGBA()
			c.addOutput(fmt.Sprintf("Pattern Properties:"))
			c.addOutput(fmt.Sprintf("  Color: #%02x%02x%02x", cr>>8, cg>>8, cb>>8))
			c.addOutput(fmt.Sprintf("  BGColor: #%02x%02x%02x", br>>8, bg>>8, bb>>8))
			c.addOutput(fmt.Sprintf("  DirtBlend: %d%%", props.DirtBlend))
			c.addOutput(fmt.Sprintf("  TextureBlend: %d%%", props.TextureBlend))
			c.addOutput(fmt.Sprintf("  Palette: %s", props.Palette))
		}

		c.addOutput("Applying to wall elements:")

		// Apply to both left and right, color and texture layer identifiers, plus door mask
		wallElements := []string{
			"walltile_left_1", "walltile_right_1",
			"walltile_left_texture", "walltile_right_texture",
			"walltile_door_texture", // Include door mask in pattern application
		}
		for _, elementID := range wallElements {
			err := c.manager.ApplyWallPattern(elementID, pattern, color)
			if err == nil {
				count++
				c.addOutput(fmt.Sprintf("  ✓ %s", elementID))
			} else {
				c.addOutput(fmt.Sprintf("  ✗ %s (error: %v)", elementID, err))
			}
		}

		c.addOutput(fmt.Sprintf("=== PATTERN APPLICATION COMPLETE ==="))
		c.addOutput(fmt.Sprintf("Total elements assigned: %d", count))
		return fmt.Sprintf("Applied wall pattern '%s' (color %d) to %d elements", pattern, color, count)
	}

	return "Error: Type must be 'floor' or 'wall'"
}

func (c *Console) cmdInfo(args []string) string {
	if len(args) < 1 {
		return "Usage: info <pattern>\nExample: info floor_basic"
	}

	pattern := args[0]

	// Check floor patterns
	if floorPatterns, exists := c.manager.floorPatterns[pattern]; exists {
		result := fmt.Sprintf("Floor Pattern: %s\nColor variants: %d\n\n", pattern, len(floorPatterns))

		for i, props := range floorPatterns {
			startR, startG, startB, _ := props.StartColor.RGBA()
			endR, endG, endB, _ := props.EndColor.RGBA()
			textureR, textureG, textureB, _ := props.TextureColor.RGBA()

			result += fmt.Sprintf("Color %d:\n", i)
			result += fmt.Sprintf("  Start: #%02x%02x%02x\n", startR>>8, startG>>8, startB>>8)
			result += fmt.Sprintf("  End: #%02x%02x%02x\n", endR>>8, endG>>8, endB>>8)
			result += fmt.Sprintf("  Texture: #%02x%02x%02x\n", textureR>>8, textureG>>8, textureB>>8)
			result += fmt.Sprintf("  Blend: %d%%, Texture Blend: %d%%\n\n", props.ShapeBlend, props.TextureBlend)
		}

		return result
	}

	// Check wall patterns
	if colorCount := c.manager.GetWallPatternColorCount(pattern); colorCount > 0 {
		return fmt.Sprintf("Wall Pattern: %s\nColor variants: %d\nType: Wall pattern with color and texture effects", pattern, colorCount)
	}

	return fmt.Sprintf("Pattern '%s' not found", pattern)
}

func (c *Console) cmdShow(args []string) string {
	// For now, just show that assignments exist
	// In a full implementation, you'd iterate through assignments
	return "Pattern assignments are stored internally. Use 'list' to see available patterns."
}

func (c *Console) cmdRemove(args []string) string {
	if len(args) < 1 {
		return "Usage: remove <element>\nExample: remove floortexture1"
	}

	// This would need to be implemented in the manager
	return fmt.Sprintf("Remove functionality not yet implemented for element: %s", args[0])
}

func (c *Console) cmdClear(args []string) string {
	c.output = []string{"Console cleared"}
	return ""
}

func (c *Console) cmdDebug(args []string) string {
	result := "Pattern System Debug Info:\n\n"

	// Check loading status
	result += "Loading Status:\n"
	result += fmt.Sprintf("  Load attempted: %t\n", c.manager.loadAttempted)
	if c.manager.foundPath != "" {
		result += fmt.Sprintf("  Found path: %s\n", c.manager.foundPath)
	}
	if c.manager.lastLoadError != "" {
		result += fmt.Sprintf("  Last error: %s\n", c.manager.lastLoadError)
	} else {
		result += "  Last error: None\n"
	}

	// Check internal state
	floorCount := len(c.manager.floorPatterns)
	wallCount := len(c.manager.wallPatterns)
	floorIndexCount := len(c.manager.floorIndex)
	wallIndexCount := len(c.manager.wallIndex)

	result += "\nInternal Counts:\n"
	result += fmt.Sprintf("  Floor patterns loaded: %d\n", floorCount)
	result += fmt.Sprintf("  Wall patterns loaded: %d\n", wallCount)
	result += fmt.Sprintf("  Floor index entries: %d\n", floorIndexCount)
	result += fmt.Sprintf("  Wall index entries: %d\n", wallIndexCount)

	// Show index entries
	if floorIndexCount > 0 {
		result += "\nFloor Index Entries:\n"
		for i, index := range c.manager.floorIndex {
			result += fmt.Sprintf("  %d. Field:'%s' Name:'%s'\n", i+1, index.Field, index.Name)
			if i >= 5 { // Limit output
				result += fmt.Sprintf("  ... and %d more\n", floorIndexCount-6)
				break
			}
		}
	}

	if wallIndexCount > 0 {
		result += "\nWall Index Entries:\n"
		for i, index := range c.manager.wallIndex {
			result += fmt.Sprintf("  %d. Field:'%s' Name:'%s'\n", i+1, index.Field, index.Name)
			if i >= 5 { // Limit output
				result += fmt.Sprintf("  ... and %d more\n", wallIndexCount-6)
				break
			}
		}
	}

	return result
}

func (c *Console) cmdReload(args []string) string {
	// This would need access to the dataDir - for now just show what would happen
	return "Reload command would re-load pattern files from disk.\nUse 'debug' to see current loading status."
}

func (c *Console) cmdTest(args []string) string {
	return "Test command would check if pattern files exist on disk.\nUse 'debug' to see loading status and counts."
}

func (c *Console) cmdElements(args []string) string {
	result := "Floor Elements Being Targeted by Apply Command:\n\n"

	// Show what the apply command tries to target
	for i := 1; i <= 20; i++ {
		elementID := fmt.Sprintf("floortexture%d", i)
		hasPattern := c.manager.HasPatternForElement(elementID)
		status := "no pattern"
		if hasPattern {
			status = "HAS PATTERN"
		}
		result += fmt.Sprintf("  %s - %s\n", elementID, status)
	}

	result += fmt.Sprintf("  doorfloortexture - ")
	if c.manager.HasPatternForElement("doorfloortexture") {
		result += "HAS PATTERN\n"
	} else {
		result += "no pattern\n"
	}

	return result
}

func (c *Console) cmdSingle(args []string) string {
	if len(args) < 4 {
		return "Usage: single <floor|wall> <element> <pattern> <color>\nExample: single floor floortexture1 floor_orgun 0"
	}

	patternType := strings.ToLower(args[0])
	elementID := args[1]
	pattern := args[2]
	colorStr := args[3]

	color, err := strconv.Atoi(colorStr)
	if err != nil {
		return "Error: Color must be a number"
	}

	if patternType == "floor" {
		err := c.manager.ApplyFloorPattern(elementID, pattern, color)
		if err != nil {
			return fmt.Sprintf("Error applying floor pattern: %v", err)
		}
		return fmt.Sprintf("Applied floor pattern '%s' (color %d) to element '%s'", pattern, color, elementID)
	} else if patternType == "wall" {
		err := c.manager.ApplyWallPattern(elementID, pattern, color)
		if err != nil {
			return fmt.Sprintf("Error applying wall pattern: %v", err)
		}
		return fmt.Sprintf("Applied wall pattern '%s' (color %d) to element '%s'", pattern, color, elementID)
	}

	return "Error: Type must be 'floor' or 'wall'"
}

func (c *Console) cmdHideTextures(args []string) string {
	c.manager.SetTextureLayerVisible(false)
	return "Texture layers hidden - showing only base/color layers (floor shape + wall color)"
}

func (c *Console) cmdShowTextures(args []string) string {
	c.manager.SetTextureLayerVisible(true)
	return "Texture layers visible - showing normal rendering (floor + wall textures)"
}

// IsVisible returns whether the console is currently visible
func (c *Console) IsVisible() bool {
	return c.visible
}

func (c *Console) cmdAssets(args []string) string {
	if len(args) < 1 {
		return "Usage: assets <wall>\nExample: assets wall"
	}
	typ := strings.ToLower(args[0])
	if typ != "wall" {
		return "Only 'assets wall' is supported currently"
	}

	// The manager implements WallAssetsProvider
	report := c.manager.GetWallAssetsReport()
	text := formatWallAssetsReport(report)
	// Print once to terminal (stdout) and also to console output
	fmt.Print(text)
	return text
}

// cmdDepths prints the current furniture and avatar asset depth values being drawn
func (c *Console) cmdDepths(args []string) string {
	report := c.manager.GetDrawDepthReport()
	if len(report.Entries) == 0 {
		return "No draw depth data available"
	}
	out := "=== DRAW DEPTHS ===\n"
	for _, e := range report.Entries {
		out += fmt.Sprintf("  - %-6s name=%-20s layer=%-8s depth=%d\n", e.Kind, e.Name, e.Layer, e.Depth)
	}
	return out
}

// cmdBoost adjusts depth for all furniture with a given product ID by an amount (can be negative)
// Usage: boost <prodId> <amount>
func (c *Console) cmdBoost(args []string) string {
	// Variants:
	// boost <prodId> <amount>
	// boost <prodId> <slice> <amount>   where slice in {a,b,c,d}
	if len(args) < 2 {
		return "Usage:\n  boost <prodId> <amount>\n  boost <prodId> <slice> <amount>\nExamples:\n  boost 50 1000\n  boost 50 a 1000"
	}
	prod := strings.TrimSpace(args[0])
	if len(args) == 2 {
		amt, err := strconv.Atoi(strings.TrimSpace(args[1]))
		if err != nil {
			return "Invalid amount: must be integer"
		}
		return c.manager.SetDepthBoost(prod, amt)
	}
	slice := strings.ToLower(strings.TrimSpace(args[1]))
	if slice != "a" && slice != "b" && slice != "c" && slice != "d" {
		return "Invalid slice: must be one of a,b,c,d"
	}
	amt, err := strconv.Atoi(strings.TrimSpace(args[2]))
	if err != nil {
		return "Invalid amount: must be integer"
	}
	return c.manager.SetDepthBoostSlice(prod, slice, amt)
}

// cmdABoost adjusts global avatar depth by an amount (can be negative)
// Usage: aboost <amount>
func (c *Console) cmdABoost(args []string) string {
	if len(args) < 1 {
		return "Usage: aboost <amount>\nExample: aboost 1000"
	}
	amt, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil {
		return "Invalid amount: must be integer"
	}
	return c.manager.SetAvatarDepthBoost(amt)
}

// formatWallAssetsReport returns a human-readable summary for terminal/console
func formatWallAssetsReport(r isoengine.WallAssetsReport) string {
	out := "=== WALL ASSETS REPORT ===\n"
	out += fmt.Sprintf("Cached assets: %d\n", len(r.Cached))
	for _, a := range r.Cached {
		out += fmt.Sprintf("  - %s (%dx%d) ~%d KB\n", a.Member, a.Width, a.Height, (a.Bytes+1023)/1024)
	}
	out += fmt.Sprintf("Total cached ~%d KB\n", (r.TotalBytes+1023)/1024)
	out += "\nActive tiles:\n"
	for _, t := range r.Tiles {
		has := "no"
		if t.HasImage {
			has = "yes"
		}
		out += fmt.Sprintf("  - %-24s type=%-7s dir=%-5s pos=(%.0f,%.0f) image=%s\n",
			t.Member, t.AssetType, t.Direction, t.X, t.Y, has)
	}
	return out
}
