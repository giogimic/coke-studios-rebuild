package avatarengine

import (
	"container/list"
	"fmt"
	"image/color"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/fs"

	"github.com/hajimehoshi/ebiten/v2"
)

// Point represents a 2D coordinate for registration points
type Point struct {
	X, Y float64
}

// AssetManager handles loading and caching of avatar assets
// Uses the white-safe image loader to handle avatar PNGs with white backgrounds
type AssetManager struct {
	dataDir               string
	// LRU cache for images bounded by byte budget
	cacheLRU              *list.List                    // list of *cacheEntry (front = most-recent)
	cacheIndex            map[string]*list.Element      // key -> element in list
	cacheBytes            int64                         // total bytes in cache
	cacheBudget           int64                         // max allowed bytes
	cacheMutex            sync.RWMutex
	registrationPoints    map[string]Point // Registration points from Members.csv
	regPointsMutex        sync.RWMutex
	debuggedMissingPoints map[string]bool // Track which missing points we've already logged
	debuggedParseErrors   map[string]bool // Track which parse errors we've already logged
	debuggedLookup        bool            // Track if we've done diagnostic lookup
	debuggedLookupLogic   bool            // Track if we've debugged the lookup logic
	debugMutex            sync.RWMutex

	// Negative cache to suppress repeated 404s
	missingCounts map[string]int
	missingCutoff int
	missingMutex  sync.RWMutex
}

type cacheEntry struct {
	key   string
	img   *ebiten.Image
	bytes int64
}

// AssetKey uniquely identifies an avatar asset
type AssetKey struct {
	Gender    string // "h" for human
	Action    string // "std", "wlk", "sit", "crr", "drk", "spk", etc.
	BodyPart  string // "hd", "bd", "fc", "ey", "hr", "ch", "ls", "rs", "lg", "lh", "rh", "sh", "ri", "ht"
	Style     string // "001", "002", etc.
	Direction int    // 0, 1, 2, 3, 7
	Frame     int    // 0, 1, 2, 3
}

// String returns the asset key as a cache key string
func (k AssetKey) String() string {
	return fmt.Sprintf("%s_%s_%s_%s_%d_%d", k.Gender, k.Action, k.BodyPart, k.Style, k.Direction, k.Frame)
}

// ToFilenameBase returns the member base name without numeric prefix
func (k AssetKey) ToFilenameBase() string {
	// Base: h_<action>_<bodypart>_<style>_<direction>_<frame>
	return fmt.Sprintf("%s_%s_%s_%s_%d_%d", k.Gender, k.Action, k.BodyPart, k.Style, k.Direction, k.Frame)
}

// ToFilename converts an AssetKey to the expected filename format
func (k AssetKey) ToFilename() string {
	// Format: {ID}_{gender}_{action}_{bodypart}_{style}_{direction}_{frame}.png
	// We'll use a placeholder ID since we don't have the mapping yet
	return fmt.Sprintf("*_%s_%s_%s_%s_%d_%d.png", k.Gender, k.Action, k.BodyPart, k.Style, k.Direction, k.Frame)
}

// NewAssetManager creates a new asset manager
func NewAssetManager(dataDir string) (*AssetManager, error) {
	am := &AssetManager{
		dataDir:               dataDir,
		cacheLRU:             list.New(),
		cacheIndex:           make(map[string]*list.Element),
		cacheBudget:          256 << 20, // 256 MB default budget; adjust as needed
		registrationPoints:    make(map[string]Point),
		debuggedMissingPoints: make(map[string]bool),
		debuggedParseErrors:   make(map[string]bool),
		missingCounts:         make(map[string]int),
		missingCutoff:         2,
	}

	// Load registration points from Members.csv
	err := am.loadRegistrationPoints()
	if err != nil {
		log.Printf("AssetManager: failed to load registration points: %v", err)
		// Don't fail creation, just log the error
	}

	return am, nil
}

// LoadAsset loads an avatar asset by key, using cache if available
func (am *AssetManager) LoadAsset(key AssetKey) (*ebiten.Image, error) {
	cacheKey := key.String()
	baseName := key.ToFilenameBase()

	// Check LRU cache first
	if img := am.cacheGet(cacheKey); img != nil {
		return img, nil
	}

	// Suppress repeated 404s
	if am.shouldSkipMissing(baseName) {
		return nil, fmt.Errorf("asset not found (suppressed): %s", baseName)
	}

	// Resolve the actual path using Lingo/people Members.csv (WASM-safe)
	pathToImage, err := am.findAssetPath(key)
	if err != nil {
		am.noteMissing(baseName)
		return nil, fmt.Errorf("asset not found: %s", key.String())
	}

	// Load using white-safe image loader
	img, err := lingoassets.LoadImage(pathToImage)
	if err != nil {
		am.noteMissing(baseName)
		return nil, fmt.Errorf("failed to load asset %s: %w", pathToImage, err)
	}

	// Apply avatar-specific white->transparent processing
	img = am.processAvatarImage(img)

	// Success: clear missing counter
	am.clearMissing(baseName)

	// Cache in LRU with budget enforcement
	am.cachePut(cacheKey, img)

	log.Printf("AssetManager: loaded and cached asset %s", key.ToFilenameBase())
	return img, nil
}

// processAvatarImage applies avatar-specific image processing
// Avatar PNGs have white backgrounds that need to be made transparent
func (am *AssetManager) processAvatarImage(img *ebiten.Image) *ebiten.Image {
	// For avatar assets, we need to make pure white pixels transparent
	// This is similar to the aggressive processing in the lingo image loader
	// but specifically tuned for avatar assets

	// Get the image as RGBA for processing
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Create a new image for the processed result
	processed := ebiten.NewImage(width, height)

	// Process pixel by pixel to make white transparent
	// This is expensive but necessary for avatar assets
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, a := img.At(x, y).RGBA()

			// Convert from 16-bit to 8-bit values
			r8 := uint8(r >> 8)
			g8 := uint8(g >> 8)
			b8 := uint8(b >> 8)
			a8 := uint8(a >> 8)

			// Make pure white (or near-white) pixels transparent
			if r8 >= 250 && g8 >= 250 && b8 >= 250 {
				// Set to transparent
				processed.Set(x, y, &color.RGBA{0, 0, 0, 0})
			} else {
				// Keep original pixel
				processed.Set(x, y, &color.RGBA{r8, g8, b8, a8})
			}
		}
	}

	return processed
}

// findAssetPath finds the actual absolute path for an asset key via Members.csv
func (am *AssetManager) findAssetPath(key AssetKey) (string, error) {
	peopleDir := filepath.Join("Lingo", "people")
	mm, err := lingoassets.LoadMemberMapCached(peopleDir)
	if err != nil {
		return "", err
	}
	baseName := fmt.Sprintf("%s", key.ToFilenameBase())
	if p, ok := mm.PathFor(baseName); ok {
		return p, nil
	}
	return "", fmt.Errorf("no matching entry for %s", baseName)
}

// ParseAssetFilename parses an avatar asset filename into an AssetKey
func ParseAssetFilename(filename string) (*AssetKey, error) {
	// Remove .png extension
	name := strings.TrimSuffix(filename, ".png")

	// Split by underscore: {ID}_{gender}_{action}_{bodypart}_{style}_{direction}_{frame}
	parts := strings.Split(name, "_")
	if len(parts) != 7 {
		return nil, fmt.Errorf("invalid filename format: %s (expected 7 parts)", filename)
	}

	// Parse direction
	direction, err := strconv.Atoi(parts[5])
	if err != nil {
		return nil, fmt.Errorf("invalid direction in filename %s: %w", filename, err)
	}

	// Parse frame
	frame, err := strconv.Atoi(parts[6])
	if err != nil {
		return nil, fmt.Errorf("invalid frame in filename %s: %w", filename, err)
	}

	return &AssetKey{
		Gender:    parts[1],
		Action:    parts[2],
		BodyPart:  parts[3],
		Style:     parts[4],
		Direction: direction,
		Frame:     frame,
	}, nil
}

// GetAssetForBodyPart loads the appropriate asset for a body part given current avatar state
func (am *AssetManager) GetAssetForBodyPart(bodyPart string, style string, action string, direction int, frame int) (*ebiten.Image, error) {
	key := AssetKey{
		Gender:    "h", // Human
		Action:    action,
		BodyPart:  bodyPart,
		Style:     style,
		Direction: direction,
		Frame:     frame,
	}

	return am.LoadAsset(key)
}

// GetAssetByName loads a generic asset by its filename (e.g., for shadows)
func (am *AssetManager) GetAssetByName(filename string) (*ebiten.Image, error) {
	// Check LRU cache first
	if img := am.cacheGet(filename); img != nil {
		return img, nil
	}

	// Try to resolve via Members.csv using the base name without numeric prefix
	peopleDir := filepath.Join("Lingo", "people")
	mm, err := lingoassets.LoadMemberMapCached(peopleDir)
	if err == nil {
		base := strings.TrimSuffix(filepath.Base(filename), ".png")
		// Remove possible numeric prefix
		if idx := strings.Index(base, "_"); idx > 0 {
			base = base[idx+1:]
		}
		if am.shouldSkipMissing(base) {
			return nil, fmt.Errorf("asset not found (suppressed): %s", base)
		}
		if p, ok := mm.PathFor(base); ok {
			if img, err := lingoassets.LoadImage(p); err == nil {
				img = am.processAvatarImage(img)
				am.cachePut(filename, img)
				log.Printf("AssetManager: loaded and cached asset %s", base)
				am.clearMissing(base)
				return img, nil
			} else {
				am.noteMissing(base)
			}
		}
	}

	// Fallback: attempt direct path under Lingo/people
	fullPath := filepath.Join("Lingo", "people", filename)
	img, err := lingoassets.LoadImage(fullPath)
	if err != nil {
		base := strings.TrimSuffix(filepath.Base(filename), ".png")
		if idx := strings.Index(base, "_"); idx > 0 {
			base = base[idx+1:]
		}
		am.noteMissing(base)
		return nil, fmt.Errorf("failed to load asset %s: %w", filename, err)
	}

	img = am.processAvatarImage(img)
	am.cachePut(filename, img)
	log.Printf("AssetManager: loaded and cached asset %s", filename)
	return img, nil
}

// ClearCache clears the asset cache to free memory
func (am *AssetManager) ClearCache() {
	am.ClearLRU()
	log.Printf("AssetManager: cleared asset cache")
}

// GetCacheSize returns the current number of cached assets (LRU length)
func (am *AssetManager) GetCacheSize() int {
	am.cacheMutex.RLock()
	defer am.cacheMutex.RUnlock()
	if am.cacheLRU == nil {
		return 0
	}
	return am.cacheLRU.Len()
}

// PreloadAssets preloads commonly used assets to improve performance
func (am *AssetManager) PreloadAssets(bodyParts []string, styles []string, actions []string, directions []int) error {
	log.Printf("AssetManager: preloading assets for %d body parts, %d styles, %d actions, %d directions",
		len(bodyParts), len(styles), len(actions), len(directions))

	loadCount := 0
	errorCount := 0

	for _, bodyPart := range bodyParts {
		for _, style := range styles {
			for _, action := range actions {
				for _, direction := range directions {
					// Load frame 0 for each combination
					key := AssetKey{
						Gender:    "h",
						Action:    action,
						BodyPart:  bodyPart,
						Style:     style,
						Direction: direction,
						Frame:     0,
					}

					_, err := am.LoadAsset(key)
					if err != nil {
						errorCount++
						// Don't log every error to avoid spam
						if errorCount <= 10 {
							log.Printf("AssetManager: failed to preload %s: %v", key.String(), err)
						}
					} else {
						loadCount++
					}
				}
			}
		}
	}

	log.Printf("AssetManager: preloaded %d assets, %d errors", loadCount, errorCount)
	return nil
}

// Negative-miss helpers
func (am *AssetManager) shouldSkipMissing(base string) bool {
	am.missingMutex.RLock()
	defer am.missingMutex.RUnlock()
	if c, ok := am.missingCounts[base]; ok && c >= am.missingCutoff {
		return true
	}
	return false
}

func (am *AssetManager) noteMissing(base string) {
	am.missingMutex.Lock()
	am.missingCounts[base] = am.missingCounts[base] + 1
	am.missingMutex.Unlock()
}

func (am *AssetManager) clearMissing(base string) {
	am.missingMutex.Lock()
	delete(am.missingCounts, base)
	am.missingMutex.Unlock()
}

// loadRegistrationPoints loads registration points from Members.csv
func (am *AssetManager) loadRegistrationPoints() error {
	csvPath := filepath.Join("Lingo", "people", "Members.csv")

	file, err := fs.Open(csvPath)
	if err != nil {
		return fmt.Errorf("failed to open Members.csv: %w", err)
	}
	defer file.Close()

	// Read the CSV file
	content, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read Members.csv: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	loadedCount := 0

	// Skip header line
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Parse CSV line: Number,Type,Name,Registration Point,Filename
		// Handle quoted fields properly - CSV might have commas inside quotes
		parts := am.parseCSVLine(line)
		if len(parts) < 4 {
			continue
		}

		name := strings.Trim(parts[2], `"`)
		regPointStr := strings.Trim(parts[3], `"`)

		// Parse registration point: "(-23, 62)"
		regPoint, err := am.parseRegistrationPoint(regPointStr)
		if err != nil {
			// Debug: Show the problematic line (only once per error type)
			errorKey := fmt.Sprintf("%s:%v", name, err)
			if !am.debuggedParseErrors[errorKey] {
				am.debuggedParseErrors[errorKey] = true
				log.Printf("AssetManager: failed to parse registration point for %s: %v (regPointStr='%s')", name, err, regPointStr)
			}
			continue
		}

		am.regPointsMutex.Lock()
		am.registrationPoints[name] = regPoint
		am.regPointsMutex.Unlock()

		// Debug: Log first few registration points (only once)
		if loadedCount < 3 {
			log.Printf("AssetManager: loaded registration point %s -> (%.1f, %.1f)", name, regPoint.X, regPoint.Y)
		}

		loadedCount++
	}

	log.Printf("AssetManager: loaded %d registration points from %d lines", loadedCount, len(lines))

	// Debug: Show a few loaded registration points (only once)
	if loadedCount > 0 {
		count := 0
		for name, point := range am.registrationPoints {
			if count < 2 {
				log.Printf("AssetManager: example registration point: %s -> (%.1f, %.1f)", name, point.X, point.Y)
				count++
			} else {
				break
			}
		}
	}

	return nil
}

// parseRegistrationPoint parses a registration point string like "(-23, 62)"
func (am *AssetManager) parseRegistrationPoint(regPointStr string) (Point, error) {
	// Remove parentheses and quotes
	regPointStr = strings.Trim(regPointStr, `"()`)

	// Split by comma
	parts := strings.Split(regPointStr, ",")
	if len(parts) != 2 {
		return Point{}, fmt.Errorf("invalid registration point format: %s", regPointStr)
	}

	// Parse X and Y
	x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return Point{}, fmt.Errorf("invalid X coordinate: %w", err)
	}

	y, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return Point{}, fmt.Errorf("invalid Y coordinate: %w", err)
	}

	return Point{X: x, Y: y}, nil
}

// GetRegistrationPoint returns the registration point for an asset
// Handles both prefixed and non-prefixed asset names
func (am *AssetManager) GetRegistrationPoint(assetName string) (Point, bool) {
	// Run diagnostic analysis once
	am.analyzeRegistrationPoints(assetName)

	am.regPointsMutex.RLock()
	defer am.regPointsMutex.RUnlock()

	// Try direct lookup first
	if point, exists := am.registrationPoints[assetName]; exists {
		// Debug lookup success (only once)
		am.debugMutex.Lock()
		if !am.debuggedLookupLogic {
			am.debuggedLookupLogic = true
			log.Printf("LOOKUP SUCCESS: Found '%s' -> (%.1f, %.1f)", assetName, point.X, point.Y)
		}
		am.debugMutex.Unlock()
		return point, true
	}

	// If assetName has a prefix (like "7_h_std_bd_001_2_0"), try without prefix
	if strings.Contains(assetName, "_") {
		parts := strings.Split(assetName, "_")
		if len(parts) > 1 {
			// Check if first part is numeric (indicating a prefix)
			if _, err := strconv.Atoi(parts[0]); err == nil {
				// Remove the numeric prefix
				nameWithoutPrefix := strings.Join(parts[1:], "_")
				if point, exists := am.registrationPoints[nameWithoutPrefix]; exists {
					return point, true
				}
			}
		}
	}

	// If no prefix, try to find a match with any prefix
	for regName, point := range am.registrationPoints {
		if strings.HasSuffix(regName, assetName) || strings.HasSuffix(assetName, regName) {
			return point, true
		}
	}

	// Debug: Log missing registration point (only once per asset)
	am.debugMutex.Lock()
	if !am.debuggedMissingPoints[assetName] {
		am.debuggedMissingPoints[assetName] = true
		log.Printf("LOOKUP FAILED: '%s' not found in %d registration points", assetName, len(am.registrationPoints))

		// Show if we have similar keys (only for first failure)
		if len(am.debuggedMissingPoints) == 1 {
			for key := range am.registrationPoints {
				if strings.Contains(key, "std_bd_001") {
					log.Printf("  Similar key found: '%s'", key)
					break
				}
			}
		}
	}
	am.debugMutex.Unlock()

	return Point{}, false
}

// parseCSVLine properly parses a CSV line handling quoted fields
func (am *AssetManager) parseCSVLine(line string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false

	for _, char := range line {
		switch char {
		case '"':
			inQuotes = !inQuotes
			current.WriteRune(char)
		case ',':
			if inQuotes {
				current.WriteRune(char)
			} else {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}

	// Add the last part
	parts = append(parts, current.String())

	return parts
}

// analyzeRegistrationPoints performs one-time diagnostic analysis
func (am *AssetManager) analyzeRegistrationPoints(requestedAsset string) {
	am.debugMutex.Lock()
	defer am.debugMutex.Unlock()

	if am.debuggedLookup {
		return // Already analyzed
	}
	am.debuggedLookup = true

	log.Printf("=== REGISTRATION POINT DIAGNOSTIC ===")
	log.Printf("Requested asset: '%s'", requestedAsset)

	// Show sample of what we actually have
	log.Printf("Sample registration points available:")
	count := 0
	for name := range am.registrationPoints {
		if count < 5 {
			log.Printf("  - %s", name)
			count++
		}
	}

	// Analyze what we have for the specific body parts being requested
	bodyParts := []string{"bd", "hd", "hr", "lg", "fc", "ey", "ch", "ls", "rs", "lh", "rh", "sh", "ri", "ht"}
	log.Printf("Checking availability for standard assets (style 001, direction 2, frame 0):")

	for _, bodyPart := range bodyParts {
		key := fmt.Sprintf("h_std_%s_001_2_0", bodyPart)
		if _, exists := am.registrationPoints[key]; exists {
			log.Printf("  ✅ %s", key)
		} else {
			log.Printf("  ❌ %s", key)
		}
	}

	// Check if we have any assets with style 001
	log.Printf("Assets with style 001:")
	count = 0
	for name := range am.registrationPoints {
		if strings.Contains(name, "_001_") && count < 10 {
			log.Printf("  - %s", name)
			count++
		}
	}

	log.Printf("=== END DIAGNOSTIC ===")
}

// getSampleKeys returns a sample of keys for debugging
func getSampleKeys(m map[string]Point, limit int) []string {
	var keys []string
	count := 0
	for k := range m {
		if count >= limit {
			break
		}
		keys = append(keys, k)
		count++
	}
	return keys
}
// ---- LRU helpers ----

// cacheGet returns an image and moves it to MRU.
func (am *AssetManager) cacheGet(key string) *ebiten.Image {
	am.cacheMutex.Lock()
	defer am.cacheMutex.Unlock()
	if am.cacheIndex == nil || am.cacheLRU == nil {
		return nil
	}
	if el, ok := am.cacheIndex[key]; ok {
		am.cacheLRU.MoveToFront(el)
		return el.Value.(*cacheEntry).img
	}
	return nil
}

// cachePut inserts or updates an image under LRU with budget enforcement.
func (am *AssetManager) cachePut(key string, img *ebiten.Image) {
	am.cacheMutex.Lock()
	defer am.cacheMutex.Unlock()
	if am.cacheIndex == nil || am.cacheLRU == nil {
		am.cacheLRU = list.New()
		am.cacheIndex = make(map[string]*list.Element)
	}
	w, h := img.Size()
	// Estimate bytes: 4 bytes per pixel
	bytes := int64(w*h) * 4
	if el, ok := am.cacheIndex[key]; ok {
		ce := el.Value.(*cacheEntry)
		am.cacheBytes -= ce.bytes
		ce.img = img
		ce.bytes = bytes
		am.cacheBytes += bytes
		am.cacheLRU.MoveToFront(el)
	} else {
		ce := &cacheEntry{key: key, img: img, bytes: bytes}
		el := am.cacheLRU.PushFront(ce)
		am.cacheIndex[key] = el
		am.cacheBytes += bytes
	}
	am.enforceBudget()
}

func (am *AssetManager) enforceBudget() {
	budget := am.cacheBudget
	if budget <= 0 {
		budget = 256 << 20
	}
	for am.cacheBytes > budget && am.cacheLRU.Len() > 0 {
		// Pop LRU from back
		el := am.cacheLRU.Back()
		ce := el.Value.(*cacheEntry)
		am.cacheLRU.Remove(el)
		delete(am.cacheIndex, ce.key)
		am.cacheBytes -= ce.bytes
		// Let GC reclaim the image; Ebiten images are GPU-backed but freed when dereferenced
		ce.img = nil
	}
}

// ClearLRU clears all cached images.
func (am *AssetManager) ClearLRU() {
	am.cacheMutex.Lock()
	defer am.cacheMutex.Unlock()
	am.cacheLRU = list.New()
	am.cacheIndex = make(map[string]*list.Element)
	am.cacheBytes = 0
}

