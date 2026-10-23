package patterns

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"path/filepath"

	lingoassets "gocoke/internal/client/assets/lingo"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/colorm"
)

// Configurable radial gradient parameters - TWEAK THESE VALUES
const (
	// Gradient center position as percentage of image size (0.0 to 1.0)
	RADIAL_CENTER_X = 0.5 // 0.5 = center horizontally
	RADIAL_CENTER_Y = 0.3 // 0.3 = 30% from top (adjust this!)

	// Gradient radius as percentage of image diagonal (0.0 to 1.0)
	RADIAL_RADIUS = 0.8 // 0.8 = 80% of image diagonal (adjust this!)

	// Inner radius where start color begins (0.0 to 1.0, relative to RADIAL_RADIUS)
	RADIAL_INNER_RADIUS = 0.0 // 0.0 = start color at center
)

// RenderOptions contains options for rendering patterns
type RenderOptions struct {
	BaseImage      *ebiten.Image
	ShapeImage     *ebiten.Image // For floor shape overlay
	TextureImage   *ebiten.Image // For pattern texture
	X, Y           float64       // Position to render at
	ShapeX, ShapeY float64       // Position to render shape at (if different from texture)
	Width, Height  int           // Size to render
	Direction      string        // For walls: "left" or "right" to choose side-specific texture
	IsTextureLayer bool          // For walls: true if rendering the "a" texture layer
	IsDoorFloor    bool          // For floors: true if this element is the door floor tile
}

// RenderFloorPattern renders a floor pattern with the given properties
func (m *Manager) RenderFloorPattern(screen *ebiten.Image, assignment FloorPatternAssignment, opts RenderOptions) {
	// Create debug key to track if we've already debugged this pattern
	debugKey := fmt.Sprintf("%s_%d", assignment.PatternName, assignment.ColorIndex)
	shouldDebug := !m.debuggedPatterns[debugKey] && !m.debuggedThisFrame

	if shouldDebug {
		log.Printf("PATTERN RENDER DEBUG: Rendering element with pattern")
		log.Printf("  Pattern: %s, Color Index: %d", assignment.PatternName, assignment.ColorIndex)
		log.Printf("  Texture Position: (%.0f, %.0f)", opts.X, opts.Y)
		log.Printf("PATTERN RENDER DEBUG: First time rendering this pattern - showing full details")
		m.debuggedPatterns[debugKey] = true
		m.debuggedThisFrame = true
	}

	// Layer 1: Floor Shape with gradient coloring (if provided)
	if opts.ShapeImage != nil {
		if shouldDebug {
			sr, sg, sb, _ := assignment.Properties.StartColor.RGBA()
			er, eg, eb, _ := assignment.Properties.EndColor.RGBA()

			log.Printf("PATTERN RENDER DEBUG: Layer 1 - Floor Shape")
			log.Printf("  Shape Image: %dx%d", opts.ShapeImage.Bounds().Dx(), opts.ShapeImage.Bounds().Dy())
			log.Printf("  Gradient: #%02x%02x%02x → #%02x%02x%02x", sr>>8, sg>>8, sb>>8, er>>8, eg>>8, eb>>8)
			log.Printf("  Shape Blend: %d%%", assignment.Properties.ShapeBlend)
		}

		// Create gradient-colored version of the floor shape
		gradientShape := m.RenderGradientToImage(
			opts.ShapeImage,
			assignment.Properties.StartColor,
			assignment.Properties.EndColor,
		)

		// Render the gradient shape with shapeBlend opacity using ColorM
		shapeOpts := &colorm.DrawImageOptions{}
		// Use shape-specific position if provided, otherwise use texture position
		shapeX, shapeY := opts.ShapeX, opts.ShapeY
		if shapeX == 0 && shapeY == 0 {
			shapeX, shapeY = opts.X, opts.Y
		}
		shapeOpts.GeoM.Translate(shapeX, shapeY)

		if shouldDebug {
			log.Printf("  Shape Position: (%.0f, %.0f)", shapeX, shapeY)
		}

		// Use ColorM for proper alpha blending
		var cm colorm.ColorM
		alpha := float64(assignment.Properties.ShapeBlend) / 100.0
		cm.Scale(1, 1, 1, alpha)

		if shouldDebug {
			log.Printf("  Shape Opacity: %.2f", alpha)
			log.Printf("  Blend Mode: ColorM with alpha scaling")
		}

		colorm.DrawImage(screen, gradientShape, cm, shapeOpts)

		if shouldDebug {
			log.Printf("  ✓ Floor Shape Layer Rendered")
		}
	} else if shouldDebug {
		log.Printf("PATTERN RENDER DEBUG: No floor shape provided - skipping Layer 1")
	}

	// Layer 2: Floor Texture with palette (if base image provided and texture layer is visible)
	if opts.BaseImage != nil && m.textureLayerVisible {
		if shouldDebug {
			log.Printf("PATTERN RENDER DEBUG: Layer 2 - Floor Texture")
			log.Printf("  Base Image: %dx%d", opts.BaseImage.Bounds().Dx(), opts.BaseImage.Bounds().Dy())
			log.Printf("  Palette: %s", assignment.Properties.Palette)
			log.Printf("  Texture Blend: %d%%", assignment.Properties.TextureBlend)
		}

		// Load the texture image based on the palette name
		var textureImage *ebiten.Image
		if opts.IsDoorFloor {
			textureImage = m.getDoorFloorTextureImage(assignment.Properties.Palette)
		} else {
			textureImage = m.getFloorTextureImage(assignment.Properties.Palette)
		}
		if textureImage == nil {
			if shouldDebug {
				log.Printf("  ⚠ Texture not loaded, using base image fallback")
			}
			// Fallback to base image if texture can't be loaded, using ColorM for consistency
			fallbackOpts := &colorm.DrawImageOptions{}
			fallbackOpts.GeoM.Translate(opts.X, opts.Y)

			var cm colorm.ColorM
			alpha := float64(assignment.Properties.TextureBlend) / 100.0
			cm.Scale(1, 1, 1, alpha)

			colorm.DrawImage(screen, opts.BaseImage, cm, fallbackOpts)
			if shouldDebug {
				log.Printf("  Texture Position: (%.0f, %.0f)", opts.X, opts.Y)
				log.Printf("  ✓ Base Image Fallback Rendered with ColorM")
			}
			return
		}

		if shouldDebug {
			log.Printf("  Texture Image: %dx%d", textureImage.Bounds().Dx(), textureImage.Bounds().Dy())
			log.Printf("  Texture Position: (%.0f, %.0f)", opts.X, opts.Y)
		}

		// Render the texture image with textureBlend opacity using ColorM
		textureOpts := &colorm.DrawImageOptions{}
		textureOpts.GeoM.Translate(opts.X, opts.Y)

		// Use ColorM for proper alpha blending
		var cm colorm.ColorM
		alpha := float64(assignment.Properties.TextureBlend) / 100.0
		cm.Scale(1, 1, 1, alpha)

		if shouldDebug {
			log.Printf("  Texture Opacity: %.2f", alpha)
			log.Printf("  Blend Mode: ColorM with alpha scaling")
		}

		colorm.DrawImage(screen, textureImage, cm, textureOpts)

		if shouldDebug {
			log.Printf("  ✓ Floor Texture Layer Rendered")
		}
	} else if shouldDebug {
		if opts.BaseImage != nil && !m.textureLayerVisible {
			log.Printf("PATTERN RENDER DEBUG: Texture layer hidden - skipping Layer 2")
		} else {
			log.Printf("PATTERN RENDER DEBUG: No base image provided - skipping Layer 2")
		}
	}

	if shouldDebug {
		log.Printf("PATTERN RENDER DEBUG: Floor pattern render complete")
	}
}

// RenderWallPattern renders a wall pattern with the given properties
func (m *Manager) RenderWallPattern(screen *ebiten.Image, assignment WallPatternAssignment, opts RenderOptions) {
	// Allow door masks to proceed even if BaseImage is nil (we'll select a base by style)
	if opts.BaseImage == nil && opts.Direction != "door" {
		return
	}

	// Choose the correct base image: for door masks, pick style-specific doormask base
	baseImg := opts.BaseImage
	if opts.Direction == "door" {
		if dm := m.getDoorMaskBaseImage(assignment.Properties.DirtStyle); dm != nil {
			baseImg = dm
		}
	}
	if baseImg == nil {
		return
	}

	// Decide whether to draw the base (two-tone) for this element
	// - For regular walls: draw base only on the color layer (IsTextureLayer == false)
	// - For door masks: we have a single tile; draw base even though IsTextureLayer == true
	drawBase := !opts.IsTextureLayer || opts.Direction == "door"

	if drawBase {
		// Build two-tone (BGColor + Color) image from the base
		var twoTone *ebiten.Image
		if assignment.Properties.Color != nil || assignment.Properties.BGColor != nil {
			fg := assignment.Properties.Color
			bg := assignment.Properties.BGColor
			// Fallbacks
			if fg == nil {
				fg = color.RGBA{255, 255, 255, 255}
			}
			if bg == nil {
				bg = color.RGBA{0, 0, 0, 255}
			}
			twoTone = m.RenderTwoToneToImage(baseImg, fg, bg)
		} else {
			// No colors provided; use the base image as-is
			twoTone = ebiten.NewImageFromImage(baseImg)
		}

		// Draw the composed result with DirtBlend as overall opacity
		finalOpts := &colorm.DrawImageOptions{}
		finalOpts.GeoM.Translate(opts.X, opts.Y)
		var finalCM colorm.ColorM
		finalCM.Scale(1, 1, 1, float64(assignment.Properties.DirtBlend)/100.0)
		colorm.DrawImage(screen, twoTone, finalCM, finalOpts)
	}

	// Step 2: Apply texture overlay (palette) only on the texture layer, and only when visible
	textureImg := opts.TextureImage
	shouldDrawTexture := opts.IsTextureLayer && m.textureLayerVisible
	if textureImg == nil && assignment.Properties.Palette != "" && shouldDrawTexture {
		textureImg = m.getWallTextureImage(assignment.Properties.Palette, opts.Direction)
	}
	if shouldDrawTexture && textureImg != nil {
		// Match floor texture opacity method: use ColorM with colorm.DrawImage
		textureOpts := &colorm.DrawImageOptions{}
		textureOpts.GeoM.Translate(opts.X, opts.Y)
		var cm colorm.ColorM
		alpha := float64(assignment.Properties.TextureBlend) / 100.0
		cm.Scale(1, 1, 1, alpha)
		colorm.DrawImage(screen, textureImg, cm, textureOpts)
	}
}

// RenderTwoToneToImage maps the source image luminance to a blend between bg (dark) and fg (light)
// preserving the source alpha. This approximates Director's sprite.bgColor+sprite.color effect.
func (m *Manager) RenderTwoToneToImage(sourceImage *ebiten.Image, fg, bg color.Color) *ebiten.Image {
	bounds := sourceImage.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Read source pixels
	src := make([]byte, 4*w*h)
	sourceImage.ReadPixels(src)

	// Extract fg/bg components
	fr, fgG, fb, fa := fg.RGBA()
	br, bgG, bb, ba := bg.RGBA()
	// Convert to 0..255
	fr8, fg8, fb8, fa8 := uint8(fr>>8), uint8(fgG>>8), uint8(fb>>8), uint8(fa>>8)
	br8, bg8, bb8, ba8 := uint8(br>>8), uint8(bgG>>8), uint8(bb>>8), uint8(ba>>8)

	out := make([]byte, 4*w*h)
	for i := 0; i < w*h; i++ {
		r := float64(src[4*i+0])
		g := float64(src[4*i+1])
		b := float64(src[4*i+2])
		a := src[4*i+3]

		// Luminance 0..1 (Rec. 601)
		lum := (0.299*r + 0.587*g + 0.114*b) / 255.0
		// Blend bg -> fg by luminance
		R := uint8(float64(br8)*(1.0-lum) + float64(fr8)*lum)
		G := uint8(float64(bg8)*(1.0-lum) + float64(fg8)*lum)
		B := uint8(float64(bb8)*(1.0-lum) + float64(fb8)*lum)
		A := uint8(float64(ba8)*(1.0-lum) + float64(fa8)*lum) // allow alpha bias by colors; commonly 255

		// Modulate by source alpha
		out[4*i+0] = R
		out[4*i+1] = G
		out[4*i+2] = B
		out[4*i+3] = uint8(float64(a) / 255.0 * float64(A))
	}

	img := ebiten.NewImage(w, h)
	img.WritePixels(out)
	return img
}

// createColorM creates a ColorM for applying a specific color
func (m *Manager) createColorM(c color.Color) colorm.ColorM {
	r, g, b, a := c.RGBA()

	var cm colorm.ColorM
	cm.Scale(
		float64(r)/0xFFFF,
		float64(g)/0xFFFF,
		float64(b)/0xFFFF,
		float64(a)/0xFFFF,
	)

	return cm
}

// createGradientColorM creates a ColorM for gradient effects (simplified)
func (m *Manager) createGradientColorM(startColor, endColor color.Color) colorm.ColorM {
	// For now, use the start color. Full gradient implementation would require
	// more complex shader or multiple draw calls
	return m.createColorM(startColor)
}

// RenderGradientToImage creates a radial gradient-colored version of the source image
// Only applies gradient to non-transparent pixels
func (m *Manager) RenderGradientToImage(sourceImage *ebiten.Image, startColor, endColor color.Color) *ebiten.Image {
	bounds := sourceImage.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	// Create a new image for the gradient result
	gradientImage := ebiten.NewImage(width, height)

	// Get color components
	sr, sg, sb, _ := startColor.RGBA()
	er, eg, eb, _ := endColor.RGBA()

	// Convert to 0-255 range
	sr, sg, sb = sr>>8, sg>>8, sb>>8
	er, eg, eb = er>>8, eg>>8, eb>>8

	// Read source pixels
	sourcePixels := make([]byte, 4*width*height)
	sourceImage.ReadPixels(sourcePixels)

	// Create gradient pixels
	gradientPixels := make([]byte, 4*width*height)

	// Calculate gradient center and radius
	centerX := float64(width) * RADIAL_CENTER_X
	centerY := float64(height) * RADIAL_CENTER_Y
	diagonal := math.Sqrt(float64(width*width + height*height))
	maxRadius := diagonal * RADIAL_RADIUS
	innerRadius := maxRadius * RADIAL_INNER_RADIUS

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixelIndex := (y*width + x) * 4

			// Check if source pixel is transparent
			sourceAlpha := sourcePixels[pixelIndex+3]
			if sourceAlpha == 0 {
				// Keep transparent
				gradientPixels[pixelIndex] = 0   // R
				gradientPixels[pixelIndex+1] = 0 // G
				gradientPixels[pixelIndex+2] = 0 // B
				gradientPixels[pixelIndex+3] = 0 // A
			} else {
				// Calculate distance from gradient center
				dx := float64(x) - centerX
				dy := float64(y) - centerY
				distance := math.Sqrt(dx*dx + dy*dy)

				// Calculate gradient factor based on radial distance
				var factor float64
				if distance <= innerRadius {
					factor = 0.0 // Start color
				} else if distance >= maxRadius {
					factor = 1.0 // End color
				} else {
					// Interpolate between inner and outer radius
					factor = (distance - innerRadius) / (maxRadius - innerRadius)
				}

				// Interpolate colors
				r := uint8(float64(sr)*(1-factor) + float64(er)*factor)
				g := uint8(float64(sg)*(1-factor) + float64(eg)*factor)
				b := uint8(float64(sb)*(1-factor) + float64(eb)*factor)

				// Apply gradient color, preserve source alpha
				gradientPixels[pixelIndex] = r             // R
				gradientPixels[pixelIndex+1] = g           // G
				gradientPixels[pixelIndex+2] = b           // B
				gradientPixels[pixelIndex+3] = sourceAlpha // A (preserve original)
			}
		}
	}

	// Write pixels to gradient image
	gradientImage.WritePixels(gradientPixels)

	return gradientImage
}

// getFloorTextureImage loads a floor texture image by palette name
func (m *Manager) getFloorTextureImage(paletteName string) *ebiten.Image {
	// Check cache first
	cacheKey := "floor_" + paletteName
	if img, exists := m.patternAssets[cacheKey]; exists {
		return img
	}

	// Load texture image from Lingo/cc_studio via member map (WASM-safe)
	ccStudioDir := filepath.Join("Lingo", "cc_studio")
	mm, err := lingoassets.LoadMemberMapCached(ccStudioDir)
	if err != nil {
		log.Printf("Pattern: failed to load member map: %v", err)
		return nil
	}
	// Palette files may be referenced without numeric prefix
	texturePath, ok := mm.PathFor(paletteName)
	if !ok {
		texturePath = filepath.Join(ccStudioDir, paletteName+".png")
	}
	ebitenImg, err := lingoassets.LoadImage(texturePath)
	if err != nil {
		log.Printf("Pattern: failed to load texture %s: %v", texturePath, err)
		return nil
	}

	// Cache for future use
	m.patternAssets[cacheKey] = ebitenImg

	log.Printf("Pattern: loaded floor texture %s", texturePath)
	return ebitenImg
}

// getDoorFloorTextureImage loads a door floor texture image by palette name
func (m *Manager) getDoorFloorTextureImage(paletteName string) *ebiten.Image {
	// Check cache first
	cacheKey := "doorfloor_" + paletteName
	if img, exists := m.patternAssets[cacheKey]; exists {
		return img
	}

	// Load via member map
	ccStudioDir := filepath.Join("Lingo", "cc_studio")
	mm, err := lingoassets.LoadMemberMapCached(ccStudioDir)
	if err != nil {
		log.Printf("Pattern: failed to load member map: %v", err)
		return nil
	}
	name := fmt.Sprintf("%s_doorfloor", paletteName)
	texturePath, ok := mm.PathFor(name)
	if !ok {
		texturePath = filepath.Join(ccStudioDir, name+".png")
	}
	ebitenImg, err := lingoassets.LoadImage(texturePath)
	if err != nil {
		log.Printf("Pattern: failed to load door floor texture %s: %v", texturePath, err)
		return nil
	}

	m.patternAssets[cacheKey] = ebitenImg
	log.Printf("Pattern: loaded door floor texture %s", texturePath)
	return ebitenImg
}

// getWallTextureImage loads a wall texture image by palette name and direction
func (m *Manager) getWallTextureImage(paletteName, direction string) *ebiten.Image {
	var fileName string

	// Handle different directions
	if direction == "door" {
		// For door masks, use the door mask texture file
		fileName = fmt.Sprintf("%s_doormask.png", paletteName)
	} else {
		// Normalize direction for left/right walls
		if direction != "left" && direction != "right" {
			// Unknown direction; default to right
			direction = "right"
		}
		// Map to file names like tile_left.png / tile_right.png
		fileName = fmt.Sprintf("%s_%s.png", paletteName, direction)
	}

	// Check cache first
	cacheKey := "wall_" + fileName
	if img, exists := m.patternAssets[cacheKey]; exists {
		return img
	}

	// Load via member map
	ccStudioDir := filepath.Join("Lingo", "cc_studio")
	mm, err := lingoassets.LoadMemberMapCached(ccStudioDir)
	if err != nil {
		log.Printf("Pattern: failed to load member map: %v", err)
		return nil
	}
	name := fileName[:len(fileName)-len(filepath.Ext(fileName))]
	texturePath, ok := mm.PathFor(name)
	if !ok {
		texturePath = filepath.Join(ccStudioDir, fileName)
	}
	ebitenImg, err := lingoassets.LoadImage(texturePath)
	if err != nil {
		log.Printf("Pattern: failed to load wall texture %s: %v", texturePath, err)
		return nil
	}

	ebitenImg2 := ebitenImg
	m.patternAssets[cacheKey] = ebitenImg2
	log.Printf("Pattern: loaded wall texture %s", texturePath)
	return ebitenImg2
}

// LoadPatternAsset loads and caches a pattern texture asset
func (m *Manager) LoadPatternAsset(assetName string, image *ebiten.Image) {
	m.patternAssets[assetName] = image
}

// SetFloorShapeAsset stores the floor shape image for pattern rendering
func (m *Manager) SetFloorShapeAsset(shapeImage *ebiten.Image) {
	m.patternAssets["floorshape"] = shapeImage
}

// SetFloorShapePosition stores the floor shape position for pattern rendering
func (m *Manager) SetFloorShapePosition(x, y float64) {
	// Store position in a simple way - we can extend this later
	m.floorShapeX = x
	m.floorShapeY = y
}

// ClearDebugState clears the debug tracking to allow fresh debugging for new commands
func (m *Manager) ClearDebugState() {
	m.debuggedPatterns = make(map[string]bool)
	m.debuggedThisFrame = false
}

// StartFrameDebug marks the start of a new frame for debugging
func (m *Manager) StartFrameDebug() {
	m.debuggedThisFrame = false
}

// ResetAllDebugStates resets all debug tracking (for external scenes)
func (m *Manager) ResetAllDebugStates() {
	m.debuggedPatterns = make(map[string]bool)
	m.debuggedThisFrame = false
}

// SetTextureLayerVisible controls whether the texture layer is rendered

// getDoorMaskBaseImage selects the correct door mask base image by DirtStyle
// Style 1 -> use the "1_b" variant (to match wall base behavior)
// Style 2 -> use the "2_b" variant
func (m *Manager) getDoorMaskBaseImage(dirtStyle int) *ebiten.Image {
	var member string
	if dirtStyle == 2 {
		member = "29_wall_doormask_2_b_0_2_0.png"
	} else {
		// Default/fallback to style 1 base uses the 'b' variant for consistent luminance
		member = "9_wall_doormask_1_b_0_2_0.png"
	}

	cacheKey := "doorbase_" + member
	if img, ok := m.patternAssets[cacheKey]; ok {
		return img
	}

	path := filepath.Join("Lingo", "cc_studio", member)
	eimg, err := lingoassets.LoadImage(path)
	if err != nil {
		log.Printf("Pattern: failed to load door base %s: %v", path, err)
		return nil
	}
	m.patternAssets[cacheKey] = eimg
	return eimg
}

func (m *Manager) SetTextureLayerVisible(visible bool) {
	m.textureLayerVisible = visible
}
