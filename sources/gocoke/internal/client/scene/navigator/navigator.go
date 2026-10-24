// Navigator scene renders the original Lingo nav_public_start.window using ripped interface assets
// Provides 1:1 visual parity with the original Director/Shockwave navigator

package navigator

import (
	"image/color"
	"log"
	"path/filepath"
	"time"

	"gocoke/internal/client/assets/features"
	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/lingo/window"
	"gocoke/internal/client/assets/studios"
	textmgr "gocoke/internal/client/assets/text"

	"github.com/hajimehoshi/ebiten/v2"
)

type tabState int

const (
	tabPublic tabState = iota
	tabPrivate
	tabSearch
)

type Scene struct {
	tm         *textmgr.Manager
	featureSet *features.FeatureSet
	studioList *studios.StudioList

	// Window definition and assets
	windowDef *window.Definition
	assets    map[string]*ebiten.Image

	// State
	currentTab       tabState
	selectedLocation int
	selectedStudio   string
	last             time.Time

	// UI positioning (from original window definition)
	windowRect [4]int // [434, 66, 721, 427] from nav_public_start.window.txt
}

func New(dataDir string, featureSet *features.FeatureSet, studioList *studios.StudioList) *Scene {
	tm := textmgr.New()
	path := filepath.Join(dataDir, "Text_English.txt")
	if err := tm.LoadFile(path); err != nil {
		log.Printf("Navigator: failed to load text file %s: %v", path, err)
	}

	scene := &Scene{
		tm:               tm,
		featureSet:       featureSet,
		studioList:       studioList,
		currentTab:       tabPublic,
		selectedLocation: -1,
		windowRect:       [4]int{434, 66, 721, 427}, // Original position
		assets:           make(map[string]*ebiten.Image),
	}

	// Load window definition (Lingo files are in current directory, not in dataDir)
	windowPath := filepath.Join("Lingo", "cc_navigator[1]", "59_nav_public_start.window.txt")
	windowDef, err := window.ParseFile(windowPath)
	if err != nil {
		log.Printf("Navigator: failed to load window definition: %v", err)
	} else {
		scene.windowDef = windowDef
		log.Printf("Navigator: loaded window definition with %d elements", len(windowDef.Elements))
	}

	// Load assets based on window definition (search both navigator and interface dirs)
	navigatorDir := filepath.Join("Lingo", "cc_navigator[1]")
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	scene.assets = lingoassets.LoadAssetsForDefinition(scene.windowDef, navigatorDir, interfaceDir)

	return scene
}


func (s *Scene) Update() error {
	now := time.Now()
	s.last = now

	// Handle input for tab switching, location selection, studio selection
	// TODO: Implement input handling

	return nil
}

func (s *Scene) Draw(screen *ebiten.Image) {
	// Clear background
	screen.Fill(color.RGBA{0x00, 0x00, 0x00, 0xFF})

	// Calculate window position (centered on screen)
	screenW, screenH := screen.Bounds().Dx(), screen.Bounds().Dy()
	windowW := s.windowRect[2] - s.windowRect[0] // 287 pixels wide
	windowH := s.windowRect[3] - s.windowRect[1] // 361 pixels tall

	// Center the window on screen
	offsetX := float64(screenW-windowW) / 2
	offsetY := float64(screenH-windowH) / 2

	// Draw all elements using exact coordinates from window definition
	s.drawElementsFromDefinition(screen, offsetX, offsetY)
}

func (s *Scene) drawElementsFromDefinition(screen *ebiten.Image, offsetX, offsetY float64) {
	if s.windowDef == nil {
		return
	}

	// Draw each element from the window definition using exact coordinates
	for _, element := range s.windowDef.Elements {
		if element.Media != "bitmap" {
			continue // Skip text fields for now
		}

		// Get the asset
		img := s.assets[element.Member]
		if img == nil {
			continue
		}

		// Draw at exact position from window definition
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(offsetX+float64(element.LocH), offsetY+float64(element.LocV))
		screen.DrawImage(img, op)
	}
}

// IsComplete returns true when navigator is ready to transition to studio
func (s *Scene) IsComplete() bool {
	return s.selectedStudio != ""
}

// GetSelectedStudio returns the selected studio ID
func (s *Scene) GetSelectedStudio() string {
	return s.selectedStudio
}

// Layout implements the Scene interface
func (s *Scene) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 760, 520 // Standard Director stage resolution
}
