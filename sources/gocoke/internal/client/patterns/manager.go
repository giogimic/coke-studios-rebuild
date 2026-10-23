package patterns

import (
	"bufio"
	"fmt"
	"image/color"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gocoke/internal/client/fs"
	"gocoke/internal/client/isoengine"

	"github.com/hajimehoshi/ebiten/v2"
)

// SceneReporter interface for getting wall tile information from the scene
// This breaks the circular dependency between patterns and isoengine
type SceneReporter interface {
	GetWallTiles() []isoengine.WallTileInfo
	GetWallAssetsReport() isoengine.WallAssetsReport
	GetDrawDepthReport() isoengine.DrawDepthReport
}

// (continued imports below removed - de-dupe)

// FloorPatternProperties represents properties for a floor pattern color variant
type FloorPatternProperties struct {
	FloorStyle   int         // Pattern style type
	StartColor   color.Color // Gradient start color
	EndColor     color.Color // Gradient end color
	ShapeBlend   int         // Blend percentage for shape layer
	Palette      string      // Pattern texture name
	TextureColor color.Color // Color for texture overlay
	TextureBlend int         // Blend percentage for texture
	IndexNum     int         // Unique identifier
}

// WallPatternProperties represents properties for a wall pattern color variant
type WallPatternProperties struct {
	DirtStyle    int         // Wall style type (1 or 2)
	Color        color.Color // Wall color
	BGColor      color.Color // Wall background color
	DirtBlend    int         // Blend percentage for wall layer
	Palette      string      // Pattern texture name
	TextureBlend int         // Blend percentage for texture
}

// PatternIndex represents an entry in a pattern index file
type PatternIndex struct {
	Field string // Field name for pattern data
	Name  string // Display name for pattern
}

// FloorPatternAssignment represents an applied floor pattern to a specific element
type FloorPatternAssignment struct {
	PatternName string
	ColorIndex  int
	Properties  FloorPatternProperties
}

// WallPatternAssignment represents an applied wall pattern to a specific element
type WallPatternAssignment struct {
	PatternName string
	ColorIndex  int
	Properties  WallPatternProperties
}

// Manager handles pattern loading and application
type Manager struct {
	floorPatterns map[string][]FloorPatternProperties // patternName -> color variants
	wallPatterns  map[string][]WallPatternProperties  // patternName -> color variants
	floorIndex    []PatternIndex                      // floor pattern index
	wallIndex     []PatternIndex                      // wall pattern index

	// Pattern assignments by position ID
	floorAssignments map[string]FloorPatternAssignment // positionID -> pattern assignment
	wallAssignments  map[string]WallPatternAssignment  // positionID -> pattern assignment

	// Asset cache for pattern textures
	patternAssets map[string]*ebiten.Image

	// Floor shape positioning
	floorShapeX float64 // Floor shape X position
	floorShapeY float64 // Floor shape Y position

	// Debug tracking
	debuggedPatterns  map[string]bool // Track which patterns have been debugged
	debuggedThisFrame bool            // Track if we've debugged this frame

	// Rendering control
	textureLayerVisible bool // Whether to render the texture layer

	// Debug info
	lastLoadError string // Last loading error for debugging
	loadAttempted bool   // Whether loading was attempted
	foundPath     string // Path where patterns were found

	// Scene reporter for getting wall tile information
	sceneReporter SceneReporter

	// Canonical ordering loaded from index files (for #texture mapping)
	canonicalWallNames  []string
	canonicalFloorNames []string
}

// New creates a new pattern manager
func New() *Manager {
	return &Manager{
		floorPatterns:       make(map[string][]FloorPatternProperties),
		wallPatterns:        make(map[string][]WallPatternProperties),
		floorAssignments:    make(map[string]FloorPatternAssignment),
		wallAssignments:     make(map[string]WallPatternAssignment),
		patternAssets:       make(map[string]*ebiten.Image),
		debuggedPatterns:    make(map[string]bool),
		textureLayerVisible: true,
	}
}

// AttachSceneReporter connects the manager to a scene reporter for cross-module reporting
func (m *Manager) AttachSceneReporter(reporter SceneReporter) {
	m.sceneReporter = reporter
}

// SetDepthBoost forwards a depth boost command to the scene, if available
func (m *Manager) SetDepthBoost(prodID string, amount int) string {
	if m.sceneReporter == nil {
		return "Scene reporter not attached"
	}
	// Call through the reporter if it exposes SetDepthBoost (iso bridge does)
	type depthBoost interface{ SetDepthBoost(string, int) string }
	if db, ok := any(m.sceneReporter).(depthBoost); ok {
		return db.SetDepthBoost(prodID, amount)
	}
	return "Depth boost not supported by scene"
}

// SetDepthBoostSlice forwards a per-slice depth boost command to the scene, if available
func (m *Manager) SetDepthBoostSlice(prodID string, slice string, amount int) string {
	if m.sceneReporter == nil {
		return "Scene reporter not attached"
	}
	if bridge, ok := any(m.sceneReporter).(interface {
		SetDepthBoostSlice(string, string, int) string
	}); ok {
		return bridge.SetDepthBoostSlice(prodID, slice, amount)
	}
	return "Per-slice depth boost not supported by scene"
}

// SetAvatarDepthBoost forwards a global avatar depth boost command to the scene, if available
func (m *Manager) SetAvatarDepthBoost(amount int) string {
	if m.sceneReporter == nil {
		return "Scene reporter not attached"
	}
	if bridge, ok := any(m.sceneReporter).(interface{ SetAvatarDepthBoost(int) string }); ok {
		return bridge.SetAvatarDepthBoost(amount)
	}
	return "Avatar depth boost not supported by scene"
}

// LoadPatterns loads all pattern data from the Lingo files
func (m *Manager) LoadPatterns(dataDir string) error {
	m.loadAttempted = true

	// Prefer the project Lingo path first for WASM, then try data/
	var studioDir string
	possiblePaths := []string{
		filepath.Join("Lingo", "cc_studio"),                // Lingo/cc_studio (served at /web/Lingo/cc_studio)
		filepath.Join(dataDir, "Lingo", "cc_studio"),       // data/Lingo/cc_studio
		filepath.Join("..", "Lingo", "cc_studio"),          // ../Lingo/cc_studio
		filepath.Join(dataDir, "..", "Lingo", "cc_studio"), // data/../Lingo/cc_studio
	}

	// Find the first path that is accessible
	for _, path := range possiblePaths {
		if f, err := fs.Open(filepath.Join(path, "122_floorpattern_patterns_index.txt")); err == nil {
			f.Close()
			studioDir = path
			m.foundPath = path
			break
		}
	}

	if studioDir == "" {
		m.lastLoadError = fmt.Sprintf("could not find Lingo/cc_studio directory in any of: %v", possiblePaths)
		return fmt.Errorf("could not find Lingo/cc_studio directory in any of: %v", possiblePaths)
	}

	// Load floor patterns
	if err := m.loadFloorPatterns(studioDir); err != nil {
		m.lastLoadError = fmt.Sprintf("failed to load floor patterns: %v", err)
		return fmt.Errorf("failed to load floor patterns: %w", err)
	}

	// Load wall patterns
	if err := m.loadWallPatterns(studioDir); err != nil {
		m.lastLoadError = fmt.Sprintf("failed to load wall patterns: %v", err)
		return fmt.Errorf("failed to load wall patterns: %w", err)
	}

	m.lastLoadError = ""
	return nil
}

// Default canonical orders as a last-resort fallback when index files are unavailable
var defaultWallOrder = []string{
	"wall_tile",
	"wall_graythreedo",
	"wall_threegrid",
	"wall_zalmiag",
	"wall_plottr",
	"wall_disnej",
	"wall_lilacgrandma",
	"wall_plates",
	"wall_bighstripe",
	"wall_dithastriperh",
}

var defaultFloorOrder = []string{
	"floor_orgun",
	"floor_karpyt",
	"floor_indjun",
	"floor_ditlines",
	"floor_ditgrid",
	"floor_basic",
	"floor_twirlee",
	"floor_phunk",
	"floor_wooddither",
	"floor_oceanliner",
	"floor_cahya",
	"floor_purplecarpetswirl",
	"floor_bluecarpet",
	"floor_drija",
	"floor_woodswirl",
}

// ensureCanonicalIndicesLoaded loads canonical wall/floor pattern name lists from index files
// so that numeric #texture values map correctly regardless of how patterns were grouped when
// loading from comprehensive test files.
func (m *Manager) ensureCanonicalIndicesLoaded() {
	// If already populated and non-empty, skip
	if len(m.canonicalWallNames) > 0 && len(m.canonicalFloorNames) > 0 {
		return
	}
	// Try multiple base directories
	var bases []string
	if strings.TrimSpace(m.foundPath) != "" {
		bases = append(bases, strings.TrimSpace(m.foundPath))
	}
	bases = append(bases,
		filepath.Join("Lingo", "cc_studio"),
		filepath.Join("web", "Lingo", "cc_studio"),
	)

	for _, base := range bases {
		if len(m.canonicalWallNames) == 0 {
			idxPath := filepath.Join(base, "109_wallpattern_patterns_index.txt")
			if list, err := m.loadPatternIndex(idxPath); err == nil && len(list) > 0 {
				names := make([]string, 0, len(list))
				for _, it := range list {
					names = append(names, it.Name)
				}
				m.canonicalWallNames = names
			}
		}
		if len(m.canonicalFloorNames) == 0 {
			idxPath := filepath.Join(base, "122_floorpattern_patterns_index.txt")
			if list, err := m.loadPatternIndex(idxPath); err == nil && len(list) > 0 {
				names := make([]string, 0, len(list))
				for _, it := range list {
					names = append(names, it.Name)
				}
				m.canonicalFloorNames = names
			}
		}
		if len(m.canonicalWallNames) > 0 && len(m.canonicalFloorNames) > 0 {
			break
		}
	}

	// Fallback to in-memory indexes if files couldn't be read
	if len(m.canonicalWallNames) == 0 && len(m.wallIndex) > 0 {
		names := make([]string, 0, len(m.wallIndex))
		for _, it := range m.wallIndex {
			names = append(names, it.Name)
		}
		m.canonicalWallNames = names
	}
	if len(m.canonicalFloorNames) == 0 && len(m.floorIndex) > 0 {
		names := make([]string, 0, len(m.floorIndex))
		for _, it := range m.floorIndex {
			names = append(names, it.Name)
		}
		m.canonicalFloorNames = names
	}

	// Final fallback or correction if lists are too short: use hardcoded defaults
	if len(m.canonicalWallNames) < len(defaultWallOrder) {
		m.canonicalWallNames = append([]string{}, defaultWallOrder...)
	}
	if len(m.canonicalFloorNames) < len(defaultFloorOrder) {
		m.canonicalFloorNames = append([]string{}, defaultFloorOrder...)
	}

	// Debug: report what we have for diagnosis
	fmt.Printf("Pattern: canonical index sizes -> wall=%d floor=%d (foundPath=%s)\n",
		len(m.canonicalWallNames), len(m.canonicalFloorNames), strings.TrimSpace(m.foundPath))
}

// GetWallPatternNameByTextureIndex returns pattern name (e.g. "wall_plates") for a 1-based texture index
func (m *Manager) GetWallPatternNameByTextureIndex(textureIndex int) (string, error) {
	m.ensureCanonicalIndicesLoaded()
	// Prefer canonical if it has enough entries
	if textureIndex > 0 && textureIndex <= len(m.canonicalWallNames) {
		return m.canonicalWallNames[textureIndex-1], nil
	}
	// Fallback: use in-memory wallIndex if long enough
	if textureIndex > 0 && textureIndex <= len(m.wallIndex) {
		return m.wallIndex[textureIndex-1].Name, nil
	}
	// Final fallback: hardcoded default order
	if textureIndex > 0 && textureIndex <= len(defaultWallOrder) {
		return defaultWallOrder[textureIndex-1], nil
	}
	return "", fmt.Errorf("wall texture index %d out of range", textureIndex)
}

// GetFloorPatternNameByTextureIndex returns pattern name (e.g. "floor_ditgrid") for a 1-based texture index
func (m *Manager) GetFloorPatternNameByTextureIndex(textureIndex int) (string, error) {
	m.ensureCanonicalIndicesLoaded()
	// Prefer canonical if it has enough entries
	if textureIndex > 0 && textureIndex <= len(m.canonicalFloorNames) {
		return m.canonicalFloorNames[textureIndex-1], nil
	}
	// Fallback: use in-memory floorIndex if long enough
	if textureIndex > 0 && textureIndex <= len(m.floorIndex) {
		return m.floorIndex[textureIndex-1].Name, nil
	}
	// Final fallback: hardcoded default order
	if textureIndex > 0 && textureIndex <= len(defaultFloorOrder) {
		return defaultFloorOrder[textureIndex-1], nil
	}
	return "", fmt.Errorf("floor texture index %d out of range", textureIndex)
}

// loadFloorPatterns loads floor pattern data
func (m *Manager) loadFloorPatterns(studioDir string) error {
	// Try to load from the comprehensive single file first
	comprehensivePath := filepath.Join(studioDir, "121_floorpattern_testi.txt")
	if f, err := fs.Open(comprehensivePath); err == nil {
		f.Close()
		fmt.Printf("Pattern: Loading all floor patterns from comprehensive file: %s\n", comprehensivePath)
		return m.loadAllFloorPatternsFromSingleFile(comprehensivePath)
	}

	// Fallback to individual pattern files approach
	fmt.Printf("Pattern: Comprehensive file not found, falling back to individual pattern files\n")

	// Load floor pattern index
	indexPath := filepath.Join(studioDir, "122_floorpattern_patterns_index.txt")
	floorIndex, err := m.loadPatternIndex(indexPath)
	if err != nil {
		return fmt.Errorf("failed to load floor pattern index: %w", err)
	}
	m.floorIndex = floorIndex
	fmt.Printf("Pattern: loaded %d floor patterns from index\n", len(floorIndex))

	// Load each floor pattern's color variants
	for _, index := range floorIndex {
		patternPath := m.findPatternFile(studioDir, index.Field)
		if patternPath == "" {
			fmt.Printf("Pattern: skipping pattern %s - file not found\n", index.Field)
			continue // Skip if pattern file not found
		}

		properties, err := m.loadFloorPatternProperties(patternPath)
		if err != nil {
			return fmt.Errorf("failed to load floor pattern %s: %w", index.Field, err)
		}

		m.floorPatterns[index.Name] = properties
		fmt.Printf("Pattern: loaded %s with %d color variants from %s\n", index.Name, len(properties), patternPath)
	}

	return nil
}

// loadAllFloorPatternsFromSingleFile loads all floor patterns from the comprehensive test file
func (m *Manager) loadAllFloorPatternsFromSingleFile(filePath string) error {
	properties, err := m.loadFloorPatternProperties(filePath)
	if err != nil {
		return fmt.Errorf("failed to load comprehensive floor patterns: %w", err)
	}

	// Group patterns by palette name
	patternGroups := make(map[string][]FloorPatternProperties)
	for _, prop := range properties {
		patternGroups[prop.Palette] = append(patternGroups[prop.Palette], prop)
	}

	// Store patterns with "floor_" prefix for consistency
	for palette, props := range patternGroups {
		patternName := "floor_" + palette
		m.floorPatterns[patternName] = props
		fmt.Printf("Pattern: loaded %s with %d color variants\n", patternName, len(props))
	}

	// Create a simple index for console listing
	m.floorIndex = make([]PatternIndex, 0, len(patternGroups))
	for palette := range patternGroups {
		m.floorIndex = append(m.floorIndex, PatternIndex{
			Field: "floorpattern_" + palette,
			Name:  "floor_" + palette,
		})
	}
	// Prefer canonical order when available on disk
	if strings.TrimSpace(m.foundPath) != "" {
		idxPath := filepath.Join(m.foundPath, "122_floorpattern_patterns_index.txt")
		if list, err := m.loadPatternIndex(idxPath); err == nil && len(list) > 0 {
			m.floorIndex = list
		}
	}

	fmt.Printf("Pattern: loaded %d total floor patterns from comprehensive file\n", len(m.floorPatterns))
	return nil
}

// loadWallPatterns loads wall pattern data
func (m *Manager) loadWallPatterns(studioDir string) error {
	// Try to load from the comprehensive single file first
	comprehensivePath := filepath.Join(studioDir, "108_wallpattern_testi.txt")
	if f, err := fs.Open(comprehensivePath); err == nil {
		f.Close()
		fmt.Printf("Pattern: Loading all wall patterns from comprehensive file: %s\n", comprehensivePath)
		return m.loadAllWallPatternsFromSingleFile(comprehensivePath)
	}

	// Fallback to individual pattern files approach
	fmt.Printf("Pattern: Comprehensive wall file not found, falling back to individual wall pattern files\n")

	// Load wall pattern index
	indexPath := filepath.Join(studioDir, "109_wallpattern_patterns_index.txt")
	wallIndex, err := m.loadPatternIndex(indexPath)
	if err != nil {
		return fmt.Errorf("failed to load wall pattern index: %w", err)
	}
	m.wallIndex = wallIndex

	// Load each wall pattern's color variants
	for _, index := range wallIndex {
		patternPath := m.findPatternFile(studioDir, index.Field)
		if patternPath == "" {
			continue // Skip if pattern file not found
		}

		properties, err := m.loadWallPatternProperties(patternPath)
		if err != nil {
			return fmt.Errorf("failed to load wall pattern %s: %w", index.Field, err)
		}

		m.wallPatterns[index.Name] = properties
	}

	return nil
}

// loadAllWallPatternsFromSingleFile loads all wall patterns from the comprehensive test file
func (m *Manager) loadAllWallPatternsFromSingleFile(filePath string) error {
	properties, err := m.loadWallPatternProperties(filePath)
	if err != nil {
		return fmt.Errorf("failed to load comprehensive wall patterns: %w", err)
	}

	// Group patterns by palette name
	patternGroups := make(map[string][]WallPatternProperties)
	for _, prop := range properties {
		palette := strings.TrimSpace(prop.Palette)
		if palette == "" {
			continue
		}
		patternGroups[palette] = append(patternGroups[palette], prop)
	}

	// Store grouped patterns under names wall_<palette>
	if m.wallPatterns == nil {
		m.wallPatterns = make(map[string][]WallPatternProperties)
	}
	for palette, props := range patternGroups {
		patternName := "wall_" + palette
		m.wallPatterns[patternName] = props
		fmt.Printf("Pattern: loaded %s with %d color variants\n", patternName, len(props))
	}

	// Create a simple index for console listing
	m.wallIndex = make([]PatternIndex, 0, len(patternGroups))
	for palette := range patternGroups {
		m.wallIndex = append(m.wallIndex, PatternIndex{
			Field: "wallpattern_" + palette,
			Name:  "wall_" + palette,
		})
	}
	// Prefer canonical order when available on disk
	if strings.TrimSpace(m.foundPath) != "" {
		idxPath := filepath.Join(m.foundPath, "109_wallpattern_patterns_index.txt")
		if list, err := m.loadPatternIndex(idxPath); err == nil && len(list) > 0 {
			m.wallIndex = list
		}
	}

	fmt.Printf("Pattern: loaded %d total wall patterns from comprehensive file\n", len(m.wallPatterns))
	return nil
}

// findPatternFile finds a pattern file by field name, handling numeric prefixes (WASM-safe)
func (m *Manager) findPatternFile(studioDir, fieldName string) string {
	// Try exact match in primary dir
	exactPath := filepath.Join(studioDir, fieldName+".txt")
	if f, err := fs.Open(exactPath); err == nil {
		f.Close()
		return exactPath
	}
	// Probe numeric prefixes without directory listing
	for i := 0; i < 300; i++ {
		candidate := filepath.Join(studioDir, fmt.Sprintf("%d_%s.txt", i, fieldName))
		if f, err := fs.Open(candidate); err == nil {
			f.Close()
			return candidate
		}
	}
	return ""
}

// loadPatternIndex loads a pattern index file
func (m *Manager) loadPatternIndex(indexPath string) ([]PatternIndex, error) {
	file, err := fs.Open(indexPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var patterns []PatternIndex
	scanner := bufio.NewScanner(file)

	// Regex to parse Lingo property lists
	lineRe := regexp.MustCompile(`\[#field:\s*"([^"]+)",\s*#name:\s*"([^"]+)"\]`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		matches := lineRe.FindStringSubmatch(line)
		if len(matches) == 3 {
			patterns = append(patterns, PatternIndex{
				Field: matches[1],
				Name:  matches[2],
			})
		}
	}

	return patterns, scanner.Err()
}

// loadFloorPatternProperties loads floor pattern color variants
func (m *Manager) loadFloorPatternProperties(patternPath string) ([]FloorPatternProperties, error) {
	file, err := fs.Open(patternPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var properties []FloorPatternProperties
	scanner := bufio.NewScanner(file)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		rawLine := scanner.Text()
		// Split on both \r and \n to handle different line ending formats
		segments := strings.FieldsFunc(rawLine, func(c rune) bool {
			return c == '\r' || c == '\n'
		})

		for _, segment := range segments {
			line := strings.TrimSpace(segment)
			if line == "" {
				continue
			}

			props, err := m.parseFloorPatternLine(line)
			if err != nil {
				continue // Skip invalid lines
			}

			properties = append(properties, props)
		}
	}

	return properties, scanner.Err()
}

// loadWallPatternProperties loads wall pattern color variants
func (m *Manager) loadWallPatternProperties(patternPath string) ([]WallPatternProperties, error) {
	file, err := fs.Open(patternPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var properties []WallPatternProperties
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		rawLine := scanner.Text()

		// Split on both \r and \n to handle different line ending formats
		segments := strings.FieldsFunc(rawLine, func(c rune) bool {
			return c == '\r' || c == '\n'
		})

		for _, segment := range segments {
			line := strings.TrimSpace(segment)
			if line == "" {
				continue
			}

			props, err := m.parseWallPatternLine(line)
			if err != nil {
				continue // Skip invalid lines
			}

			properties = append(properties, props)
		}
	}

	return properties, scanner.Err()
}

// parseFloorPatternLine parses a floor pattern property line
func (m *Manager) parseFloorPatternLine(line string) (FloorPatternProperties, error) {
	props := FloorPatternProperties{}

	// Parse Lingo property list format
	// [#floorStyle:1, #startcolor: rgb("#33ff33"), #endcolor: rgb("#009900"), ...]

	// Extract properties using regex
	floorStyleRe := regexp.MustCompile(`#floorStyle:\s*(\d+)`)
	startColorRe := regexp.MustCompile(`#startcolor:\s*rgb\("([^"]+)"\)`)
	endColorRe := regexp.MustCompile(`#endcolor:\s*rgb\("([^"]+)"\)`)
	shapeBlendRe := regexp.MustCompile(`#shapeBlend:\s*(\d+)`)
	paletteRe := regexp.MustCompile(`#palette:\s*"([^"]+)"`)
	textureColorRe := regexp.MustCompile(`#texturecolor:\s*rgb\s*\("([^"]+)"\)`)
	textureBlendRe := regexp.MustCompile(`#textureBlend:\s*(\d+)`)
	indexNumRe := regexp.MustCompile(`#indexNum:\s*(\d+)`)

	// Parse floorStyle
	if matches := floorStyleRe.FindStringSubmatch(line); len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			props.FloorStyle = val
		}
	}

	// Parse startcolor
	if matches := startColorRe.FindStringSubmatch(line); len(matches) > 1 {
		props.StartColor = parseColor(matches[1])
	}

	// Parse endcolor
	if matches := endColorRe.FindStringSubmatch(line); len(matches) > 1 {
		props.EndColor = parseColor(matches[1])
	}

	// Parse shapeBlend
	if matches := shapeBlendRe.FindStringSubmatch(line); len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			props.ShapeBlend = val
		}
	}

	// Parse palette
	if matches := paletteRe.FindStringSubmatch(line); len(matches) > 1 {
		props.Palette = matches[1]
	}

	// Parse texturecolor
	if matches := textureColorRe.FindStringSubmatch(line); len(matches) > 1 {
		props.TextureColor = parseColor(matches[1])
	}

	// Parse textureBlend
	if matches := textureBlendRe.FindStringSubmatch(line); len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			props.TextureBlend = val
		}
	}

	// Parse indexNum
	if matches := indexNumRe.FindStringSubmatch(line); len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			props.IndexNum = val
		}
	}

	return props, nil
}

// parseWallPatternLine parses a wall pattern property line
func (m *Manager) parseWallPatternLine(line string) (WallPatternProperties, error) {
	props := WallPatternProperties{}

	// Parse wall pattern properties (similar to floor but different fields)
	dirtStyleRe := regexp.MustCompile(`#dirtStyle:\s*(\d+)`)
	colorRe := regexp.MustCompile(`#color:\s*rgb\("([^"]+)"\)`)
	bgColorRe1 := regexp.MustCompile(`#bgColor:\s*rgb\("([^"]+)"\)`)
	bgColorRe2 := regexp.MustCompile(`#bgcolor:\s*rgb\("([^"]+)"\)`)
	dirtBlendRe := regexp.MustCompile(`#dirtBlend:\s*(\d+)`)
	paletteRe := regexp.MustCompile(`#palette:\s*"([^"]+)"`)
	textureBlendRe := regexp.MustCompile(`#textureBlend:\s*(\d+)`)

	// Parse dirtStyle
	if matches := dirtStyleRe.FindStringSubmatch(line); len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			props.DirtStyle = val
		}
	}

	// Parse color
	if matches := colorRe.FindStringSubmatch(line); len(matches) > 1 {
		props.Color = parseColor(matches[1])
	}

	// Parse bgColor (support both #bgColor and #bgcolor)
	if matches := bgColorRe1.FindStringSubmatch(line); len(matches) > 1 {
		props.BGColor = parseColor(matches[1])
	} else if matches := bgColorRe2.FindStringSubmatch(line); len(matches) > 1 {
		props.BGColor = parseColor(matches[1])
	}

	// Parse dirtBlend
	if matches := dirtBlendRe.FindStringSubmatch(line); len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			props.DirtBlend = val
		}
	}

	// Parse palette
	if matches := paletteRe.FindStringSubmatch(line); len(matches) > 1 {
		props.Palette = matches[1]
	}

	// Parse textureBlend
	if matches := textureBlendRe.FindStringSubmatch(line); len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			props.TextureBlend = val
		}
	}

	return props, nil
}

// parseColor parses a hex color string to color.Color
func parseColor(hexColor string) color.Color {
	// Remove # if present
	hexColor = strings.TrimPrefix(hexColor, "#")

	// Parse hex color
	if len(hexColor) != 6 {
		return color.RGBA{255, 255, 255, 255} // Default to white
	}

	r, _ := strconv.ParseUint(hexColor[0:2], 16, 8)
	g, _ := strconv.ParseUint(hexColor[2:4], 16, 8)
	b, _ := strconv.ParseUint(hexColor[4:6], 16, 8)

	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}

// ApplyFloorPattern assigns a floor pattern to a position
func (m *Manager) ApplyFloorPattern(positionID, patternName string, colorIndex int) error {
	patterns, exists := m.floorPatterns[patternName]
	if !exists {
		return fmt.Errorf("floor pattern %s not found", patternName)
	}

	if colorIndex < 0 || colorIndex >= len(patterns) {
		return fmt.Errorf("color index %d out of range for pattern %s", colorIndex, patternName)
	}

	m.floorAssignments[positionID] = FloorPatternAssignment{
		PatternName: patternName,
		ColorIndex:  colorIndex,
		Properties:  patterns[colorIndex],
	}

	return nil
}

// HasPatternForElement checks if an element has any pattern assigned
func (m *Manager) HasPatternForElement(elementID string) bool {
	_, hasFloor := m.floorAssignments[elementID]
	_, hasWall := m.wallAssignments[elementID]
	return hasFloor || hasWall
}

// ApplyPatternToElement renders an element with its assigned pattern
func (m *Manager) ApplyPatternToElement(screen *ebiten.Image, elementID string, baseImage *ebiten.Image, x, y float64) {
	// Check for floor pattern
	if assignment, exists := m.floorAssignments[elementID]; exists {

		opts := RenderOptions{
			BaseImage: baseImage,
			X:         x,
			Y:         y,
		}
		// Mark door floor so we use the *_doorfloor texture file
		if elementID == "doorfloortexture" {
			opts.IsDoorFloor = true
		}

		// For floor textures, only render the floor shape for the first element to avoid duplication
		if strings.HasPrefix(elementID, "floortexture") || elementID == "doorfloortexture" {
			// Only render floor shape for the first floor texture element (floortexture1)
			if elementID == "floortexture1" {
				// Get floor shape from assets if available
				if shapeImage, exists := m.patternAssets["floorshape"]; exists {
					opts.ShapeImage = shapeImage
					// Use the stored floor shape position for the shape layer
					opts.ShapeX = m.floorShapeX
					opts.ShapeY = m.floorShapeY
					// Texture position remains as passed in (x, y)
				}
			}
			// All other floor texture elements render without the shape layer
		}

		m.RenderFloorPattern(screen, assignment, opts)
		return
	}

	// Check for wall pattern
	if assignment, exists := m.wallAssignments[elementID]; exists {
		// Infer direction and layer from element ID
		dir := "right"
		isTexture := false
		lowerID := strings.ToLower(elementID)
		if strings.Contains(lowerID, "left") {
			dir = "left"
		}
		if strings.Contains(lowerID, "right") {
			dir = "right"
		}
		if strings.Contains(lowerID, "door") {
			dir = "door"
		} // Support door mask
		if strings.Contains(lowerID, "texture") {
			isTexture = true
		}

		opts := RenderOptions{
			BaseImage:      baseImage,
			X:              x,
			Y:              y,
			Direction:      dir,
			IsTextureLayer: isTexture,
		}
		m.RenderWallPattern(screen, assignment, opts)
		return
	}
}

// GetWallAssetsReport implements WallAssetsProvider, delegating to isoengine scene via a bridge if available
func (m *Manager) GetWallAssetsReport() isoengine.WallAssetsReport {
	// If we have a bridge attached, ask it; otherwise return what we can from pattern cache
	var report isoengine.WallAssetsReport
	// Cached pattern textures
	for key, img := range m.patternAssets {
		if !strings.HasPrefix(key, "wall_") {
			continue
		}
		b := img.Bounds()
		w := b.Dx()
		h := b.Dy()
		bytes := w * h * 4
		report.Cached = append(report.Cached, isoengine.WallAssetInfo{Member: key, Width: w, Height: h, Bytes: bytes})
		report.TotalBytes += bytes
	}
	// Tiles info is provided by isoengine; leave empty if not connected

	// Merge in isoengine tile info if a scene reporter is attached
	if m.sceneReporter != nil {
		isoReport := m.sceneReporter.GetWallAssetsReport()
		report.Tiles = isoReport.Tiles
	}

	return report
}

// GetDrawDepthReport proxies to the scene reporter to obtain current furniture/avatar depth values
func (m *Manager) GetDrawDepthReport() isoengine.DrawDepthReport {
	if m.sceneReporter == nil {
		return isoengine.DrawDepthReport{}
	}
	return m.sceneReporter.GetDrawDepthReport()
}

// ApplyWallPattern assigns a wall pattern to a position
func (m *Manager) ApplyWallPattern(positionID, patternName string, colorIndex int) error {
	patterns, exists := m.wallPatterns[patternName]
	if !exists {
		return fmt.Errorf("wall pattern %s not found", patternName)
	}

	if colorIndex < 0 || colorIndex >= len(patterns) {
		return fmt.Errorf("color index %d out of range for pattern %s", colorIndex, patternName)
	}

	m.wallAssignments[positionID] = WallPatternAssignment{
		PatternName: patternName,
		ColorIndex:  colorIndex,
		Properties:  patterns[colorIndex],
	}

	return nil
}

// GetFloorPattern returns the floor pattern assignment for a position
func (m *Manager) GetFloorPattern(positionID string) (FloorPatternAssignment, bool) {
	assignment, exists := m.floorAssignments[positionID]
	return assignment, exists
}

// GetWallPattern returns the wall pattern assignment for a position
func (m *Manager) GetWallPattern(positionID string) (WallPatternAssignment, bool) {
	assignment, exists := m.wallAssignments[positionID]
	return assignment, exists
}

// GetFloorPatternNames returns all available floor pattern names
func (m *Manager) GetFloorPatternNames() []string {
	var names []string
	for _, index := range m.floorIndex {
		names = append(names, index.Name)
	}
	return names
}

// GetWallPatternNames returns all available wall pattern names
func (m *Manager) GetWallPatternNames() []string {
	var names []string
	for _, index := range m.wallIndex {
		names = append(names, index.Name)
	}
	return names
}

// GetFloorPatternColorCount returns the number of color variants for a floor pattern
func (m *Manager) GetFloorPatternColorCount(patternName string) int {
	if patterns, exists := m.floorPatterns[patternName]; exists {
		return len(patterns)
	}
	return 0
}

// GetWallPatternColorCount returns the number of color variants for a wall pattern
func (m *Manager) GetWallPatternColorCount(patternName string) int {
	if patterns, exists := m.wallPatterns[patternName]; exists {
		return len(patterns)
	}
	return 0
}
