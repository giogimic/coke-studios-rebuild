package isoengine

import "math"

// IsoConstants holds all configuration constants for the isometric engine
// Replicates the Lingo IsoConstants.ls behavior with 1:1 parity
type IsoConstants struct {
	// Scene dimensions
	DefaultSceneWidth  int
	DefaultSceneHeight int
	DefaultMaxHeight   int
	DefaultHeight      int
	DefaultRow         int
	DefaultCol         int

	// Layout types and grid dimensions
	DefaultLayoutType int // Layout 1 (small grid)
	DefaultSqSize     int // Square size for layout 1
	DefaultRows       int // Grid rows for layout 1
	DefaultCols       int // Grid columns for layout 1
	DefaultXOff       int // X offset for layout 1
	DefaultYOff       int // Y offset for layout 1

	DefaultLGLayoutType int // Layout 2 (large grid)
	DefaultLGSqSize     int // Square size for layout 2
	DefaultLGRows       int // Grid rows for layout 2
	DefaultLGCols       int // Grid columns for layout 2
	DefaultLGXOff       int // X offset for layout 2
	DefaultLGYOff       int // Y offset for layout 2

	// Rotation angles (in degrees)
	DefaultYRot float64 // Y rotation (45 degrees)
	DefaultXRot float64 // X rotation (30 degrees)

	// Depth and layering
	DefaultGridDepth       int
	DefaultBackgroundDepth int
	DefaultHotspotDepth    int
	DefaultStartDepth      int
	DefaultDepthOff        int
	DefaultLayers          int
	DefaultLayer           int

	// Sprite management
	DefaultSpriteCounter    int
	DefaultSpriteCounterMax int

	// Performance
	DefaultFPS          int
	DefaultSequenceRate int

	// Special item types
	BurnedCD       int
	Teleporter     int
	GoldRecord     int
	PlatinumRecord int

	// Layer mapping for depth calculation
	LayerMap map[string]int

	// Calculated trigonometric values (cached for performance)
	SinY float64 // sin(YRot in radians)
	SinX float64 // sin(XRot in radians)
	CosY float64 // cos(YRot in radians)
	CosX float64 // cos(XRot in radians)

	// Maximum items
	DefaultMaxItems int
}

// NewIsoConstants creates a new IsoConstants instance with default values
// Matches the Lingo IsoConstants.new() behavior exactly
func NewIsoConstants() *IsoConstants {
	constants := &IsoConstants{
		// Scene dimensions (matches Lingo stage dimensions)
		DefaultSceneWidth:  960, // Standard game width
		DefaultSceneHeight: 540, // Standard game height
		DefaultMaxHeight:   20,
		DefaultHeight:      0,
		DefaultRow:         0,
		DefaultCol:         0,

		// Layout 1 (small grid) - original studio layout
		DefaultLayoutType: 1,
		DefaultSqSize:     34,
		DefaultRows:       30,
		DefaultCols:       30,
		DefaultXOff:       380,
		DefaultYOff:       -11,

		// Layout 2 (large grid) - used by private studios
		DefaultLGLayoutType: 2,
		DefaultLGSqSize:     51,
		DefaultLGRows:       15,
		DefaultLGCols:       15,
		DefaultLGXOff:       380,
		DefaultLGYOff:       -35,

		// Rotation angles (isometric projection)
		DefaultYRot: 45.0, // Y rotation in degrees
		DefaultXRot: 30.0, // X rotation in degrees

		// Depth and layering constants
		DefaultGridDepth:       5,
		DefaultBackgroundDepth: 6,
		DefaultHotspotDepth:    7,
		DefaultStartDepth:      10,
		DefaultDepthOff:        10,
		DefaultLayer:           1,

		// Sprite management
		DefaultSpriteCounter:    50,
		DefaultSpriteCounterMax: 750,

		// Performance settings
		DefaultFPS:          100,
		DefaultSequenceRate: 600,

		// Special item type constants
		BurnedCD:       89,
		Teleporter:     61,
		GoldRecord:     82,
		PlatinumRecord: 83,

		// Maximum items
		DefaultMaxItems: 100,
	}

	// Initialize layer mapping (matches Lingo aLayerMap)
	constants.LayerMap = map[string]int{
		"Base": 1,
		"00":   2,
		"sd":   3,
		"a":    4,
		"a2":   5,
		"avsd": 6,
		"av":   7,
		"b":    9,
		"c":    10,
		"d":    11,
		"e":    12,
		"f":    13,
		"g":    14,
		"h":    15,
	}

	// Calculate total layers (matches Lingo calculation)
	constants.DefaultLayers = len(constants.LayerMap) * (constants.DefaultMaxHeight + 1)

	// Pre-calculate trigonometric values for performance
	constants.updateTrigValues()

	return constants
}

// updateTrigValues calculates and caches trigonometric values
// Matches Lingo's sin/cos calculations for isometric projection
func (c *IsoConstants) updateTrigValues() {
	// Convert degrees to radians
	yRotRad := c.DefaultYRot * math.Pi / 180.0
	xRotRad := c.DefaultXRot * math.Pi / 180.0

	// Calculate and cache trigonometric values
	c.SinY = math.Sin(yRotRad)
	c.SinX = math.Sin(xRotRad)
	c.CosY = math.Cos(yRotRad)
	c.CosX = math.Cos(xRotRad)
}

// GetLayoutConfig returns the grid configuration for a specific layout type
// Matches Lingo IsoScene.setLayout() behavior
func (c *IsoConstants) GetLayoutConfig(layoutType int) (xOff, yOff, sqSize, rows, cols int) {
	switch layoutType {
	case 1:
		return c.DefaultXOff, c.DefaultYOff, c.DefaultSqSize, c.DefaultRows, c.DefaultCols
	case 2:
		return c.DefaultLGXOff, c.DefaultLGYOff, c.DefaultLGSqSize, c.DefaultLGRows, c.DefaultLGCols
	default:
		// Default to layout 1
		return c.DefaultXOff, c.DefaultYOff, c.DefaultSqSize, c.DefaultRows, c.DefaultCols
	}
}

// GetLayerDepth returns the depth value for a given layer name
// Matches Lingo layer mapping system
func (c *IsoConstants) GetLayerDepth(layerName string) int {
	if depth, exists := c.LayerMap[layerName]; exists {
		return depth
	}
	return c.DefaultLayer
}

// RadToDeg converts radians to degrees
func (c *IsoConstants) RadToDeg(rad float64) float64 {
	return rad * 180.0 / math.Pi
}

// DegToRad converts degrees to radians
func (c *IsoConstants) DegToRad(deg float64) float64 {
	return deg * math.Pi / 180.0
}
