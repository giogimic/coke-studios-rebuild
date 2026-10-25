// Font loading system that replicates Lingo font management
// Loads actual fonts from fonts/ directory to match original typography exactly

package ui

import (
	"log"
	"path/filepath"
	"strconv"

	"gocoke/internal/client/fs"
	"os"
	"runtime"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
)

// FontManager handles loading and caching of fonts exactly as Lingo did
type FontManager struct {
	fonts       map[string]font.Face      // Cache: "fontName_style_size" -> font.Face
	loadedFonts map[string]*opentype.Font // Raw font data cache
}

var globalFM *FontManager

// GetGlobalFontManager returns a shared font manager for subsystems that need text without direct HUD access
func GetGlobalFontManager() *FontManager {
	if globalFM == nil {
		globalFM = NewFontManager()
		globalFM.PreloadCommonFonts()
	}
	return globalFM
}

// NewFontManager creates a new font manager and loads fonts from fonts/ directory
func NewFontManager() *FontManager {
	fm := &FontManager{
		fonts:       make(map[string]font.Face),
		loadedFonts: make(map[string]*opentype.Font),
	}

	// Load fonts from fonts/ directory
	fm.loadFontsFromDirectory()

	return fm
}

// loadFontsFromDirectory loads all TTF fonts from the fonts/ directory
func (fm *FontManager) loadFontsFromDirectory() {
	fontsDir := "fonts"

	// WASM environment: avoid os.Stat/os.ReadDir which are not implemented
	if runtime.GOOS == "js" || runtime.GOARCH == "wasm" {
		for _, name := range []string{"CokeStudioschat.ttf", "CokeStudioschatb.ttf", "Verdana.ttf", "CgAlpinGthNo1.ttf", "CokeMIRGothic.ttf"} {
			fontPath := filepath.Join(fontsDir, name)
			if err := fm.loadFont(fontPath); err == nil {
				log.Printf("FontManager: loaded font %s (wasm)", name)
			} else {
				log.Printf("FontManager: font %s not found (%v)", name, err)
			}
		}
		return
	}

	// Check if fonts directory exists
	if _, err := os.Stat(fontsDir); os.IsNotExist(err) {
		log.Printf("FontManager: fonts directory not found: %s", fontsDir)
		return
	}

	// Read fonts directory
	entries, err := os.ReadDir(fontsDir)
	if err != nil {
		log.Printf("FontManager: failed to read fonts directory: %v", err)
		// WASM fallback: try loading known font filenames directly
		for _, name := range []string{"CokeStudioschat.ttf", "CokeStudioschatb.ttf", "Verdana.ttf", "CgAlpinGthNo1.ttf", "CokeMIRGothic.ttf"} {
			fontPath := filepath.Join(fontsDir, name)
			if err := fm.loadFont(fontPath); err == nil {
				log.Printf("FontManager: loaded font %s (fallback)", name)
			}
		}
		return
	}

	// Load each TTF file
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if filepath.Ext(filename) != ".ttf" {
			continue
		}

		fontPath := filepath.Join(fontsDir, filename)
		if err := fm.loadFont(fontPath); err != nil {
			log.Printf("FontManager: failed to load font %s: %v", filename, err)
		} else {
			log.Printf("FontManager: loaded font %s", filename)
		}
	}
}

// loadFont loads a single TTF font file
func (fm *FontManager) loadFont(fontPath string) error {
	fontData, err := fs.ReadFile(fontPath)
	if err != nil {
		return err
	}

	font, err := opentype.Parse(fontData)
	if err != nil {
		return err
	}

	// Extract font name from filename (e.g., "CokeStudioschat.ttf" -> "CokeStudioschat")
	fontName := filepath.Base(fontPath)
	fontName = fontName[:len(fontName)-len(filepath.Ext(fontName))]

	fm.loadedFonts[fontName] = font
	return nil
}

// GetFont returns a font face for the given parameters, matching Lingo's font system
func (fm *FontManager) GetFont(fontName, fontStyle string, fontSize int) font.Face {
	// Create cache key
	key := fontName + "_" + fontStyle + "_" + strconv.Itoa(fontSize)

	// Return cached font if available
	if face, exists := fm.fonts[key]; exists {
		return face
	}

	// Try to create font face from loaded fonts
	face := fm.createFontFace(fontName, fontStyle, fontSize)
	if face != nil {
		fm.fonts[key] = face
		return face
	}

	// Fallback to basic font
	log.Printf("FontManager: using fallback font for %s %s %d", fontName, fontStyle, fontSize)
	face = basicfont.Face7x13
	fm.fonts[key] = face
	return face
}

// createFontFace creates a font face from loaded font data
func (fm *FontManager) createFontFace(fontName, fontStyle string, fontSize int) font.Face {
	// Map Lingo font names to actual font files
	var actualFontName string
	switch fontName {
	case "Verdana":
		if _, ok := fm.loadedFonts["Verdana"]; ok {
			actualFontName = "Verdana"
		} else if fontStyle == "bold" {
			actualFontName = "CokeStudioschatb" // Bold version
		} else {
			actualFontName = "CokeStudioschat" // Regular version
		}
	case "Arial":
		// Fallback to CokeStudios font for Arial requests
		actualFontName = "CokeStudioschat"
	default:
		// Try the font name as-is
		actualFontName = fontName
	}

	// Get the loaded font
	loadedFont, exists := fm.loadedFonts[actualFontName]
	if !exists {
		return nil
	}

	// Create font face with exact size matching Lingo
	face, err := opentype.NewFace(loadedFont, &opentype.FaceOptions{
		Size:    float64(fontSize),
		DPI:     72, // Match Director's DPI
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Printf("FontManager: failed to create font face: %v", err)
		return nil
	}

	return face
}

// GetAvailableFonts returns a list of loaded font names for debugging
func (fm *FontManager) GetAvailableFonts() []string {
	var fonts []string
	for fontName := range fm.loadedFonts {
		fonts = append(fonts, fontName)
	}
	return fonts
}

// PreloadCommonFonts preloads commonly used font combinations for better performance
func (fm *FontManager) PreloadCommonFonts() {
	// Common Lingo font combinations used in CokeStudios
	commonFonts := []struct {
		name  string
		style string
		size  int
	}{
		{"Verdana", "bold", 11},  // Navigator text
		{"Verdana", "plain", 10}, // Regular UI text
		{"Verdana", "bold", 20},  // Headers
		{"Arial", "plain", 12},   // Loader text
		{"Arial", "bold", 14},    // Button text
	}

	for _, f := range commonFonts {
		fm.GetFont(f.name, f.style, f.size)
	}

	log.Printf("FontManager: preloaded %d common font combinations", len(commonFonts))
}
