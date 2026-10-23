package isoengine

import (
	"encoding/xml"
	"fmt"
	"gocoke/internal/client/fs"
	"log"
	"path/filepath"
)

// StudioXML represents the root studio XML structure
// Matches the Lingo Studio XML format exactly
type StudioXML struct {
	XMLName    xml.Name `xml:"Studio"`
	XOff       int      `xml:"xOff,attr"`
	YOff       int      `xml:"yOff,attr"`
	LayoutType int      `xml:"layoutType,attr"`

	// Optional: avatar draw offsets (pixels). If omitted, 0. Useful for public rooms.
	AvatarXOff int `xml:"avatarXOff,attr,omitempty"`
	AvatarYOff int `xml:"avatarYOff,attr,omitempty"`

	Background  Background  `xml:"Background"`
	Hiliter     Hiliter     `xml:"Hiliter"`
	Floor       Floor       `xml:"Floor"`
	Door        Door        `xml:"Door"`
	Wall        Wall        `xml:"Wall"`
	Window      Window      `xml:"Window"`
	Furniture   Furniture   `xml:"Furniture"`
	WallItems   WallItems   `xml:"WallItems"`
	StaticItems StaticItems `xml:"StaticItems"`
}

// Background represents the studio background image
type Background struct {
	Member string `xml:"member,attr"`
	X      int    `xml:"x,attr"`
	Y      int    `xml:"y,attr"`
	Ink    int    `xml:"ink,attr"`
}

// Hiliter represents the hiliter configuration
type Hiliter struct {
	Member string `xml:"member,attr"`
	Ink    int    `xml:"ink,attr"`
}

// Floor represents the floor configuration
type Floor struct {
	Shape      FloorShape `xml:"Shape"`
	FloorTiles FloorTiles `xml:"FloorTiles"`
}

// FloorShape represents the floor shape overlay
type FloorShape struct {
	Member string `xml:"member,attr"`
	X      int    `xml:"x,attr"`
	Y      int    `xml:"y,attr"`
	Ink    int    `xml:"ink,attr"`
	Blend  int    `xml:"blend,attr"`
}

// FloorTiles represents the collection of floor tiles
type FloorTiles struct {
	Member  string      `xml:"member,attr"`
	Ink     int         `xml:"ink,attr"`
	Blend   int         `xml:"blend,attr"`
	Palette string      `xml:"palette,attr"`
	Tiles   []FloorTile `xml:"FloorTile"`
}

// FloorTile represents an individual floor tile
type FloorTile struct {
	Row     int    `xml:"row,attr"`
	Col     int    `xml:"col,attr"`
	XOff    int    `xml:"xoff,attr,omitempty"`
	YOff    int    `xml:"yoff,attr,omitempty"`
	Member  string `xml:"member,attr,omitempty"`  // For door tiles
	Ink     int    `xml:"ink,attr,omitempty"`     // For door tiles
	Blend   int    `xml:"blend,attr,omitempty"`   // For door tiles
	Palette string `xml:"palette,attr,omitempty"` // For door tiles
}

// Door represents the door/exit configuration
type Door struct {
	Row       int       `xml:"row,attr"`
	Col       int       `xml:"col,attr"`
	FloorTile FloorTile `xml:"FloorTile"`
	Exit      Exit      `xml:"Exit"`
	DoorMask  DoorMask  `xml:"DoorMask"`
}

// Exit represents the exit hotspot
type Exit struct {
	Member string `xml:"member,attr"`
	Ink    int    `xml:"ink,attr"`
	Blend  int    `xml:"blend,attr"`
}

// DoorMask represents the door mask
type DoorMask struct {
	XOff  int `xml:"xoff,attr"`
	YOff  int `xml:"yoff,attr"`
	Blend int `xml:"blend,attr"`
}

// Wall represents the wall configuration
type Wall struct {
	WallTiles []WallTile `xml:"WallTile"`
}

// WallTile represents an individual wall tile
type WallTile struct {
	Row    int    `xml:"row,attr"`
	Col    int    `xml:"col,attr"`
	Dir    string `xml:"dir,attr"`
	XOff   int    `xml:"xoff,attr,omitempty"`
	YOff   int    `xml:"yoff,attr,omitempty"`
	Corner bool   `xml:"corner,attr,omitempty"`
}

// Window represents a window configuration
type Window struct {
	Row       int    `xml:"row,attr"`
	Col       int    `xml:"col,attr"`
	ImageBase string `xml:"imageBase,attr"`
}

// Furniture represents furniture items (usually empty in basic studios)
type Furniture struct {
	Items []FurnitureItem `xml:"Item"`
}

// FurnitureItem represents an individual furniture item
// Supports both private-studio (<Item row/col member/type>) and public-room (<Item gridX/gridZ imageBase/dir ...>) formats
type FurnitureItem struct {
	// Private studio attributes (rarely used in public rooms)
	Row    int    `xml:"row,attr"`
	Col    int    `xml:"col,attr"`
	Member string `xml:"member,attr"`
	Type   string `xml:"type,attr"`

	// Public room (London) attributes
	GridX     int    `xml:"gridX,attr"`
	GridZ     int    `xml:"gridZ,attr"`
	GridY     int    `xml:"gridY,attr"` // stacking height (unused for static render)
	ImageBase string `xml:"imageBase,attr"`
	Dir       int    `xml:"dir,attr"`
	Action    string `xml:"Action,attr"`
	Ink       int    `xml:"ink,attr"`
	CastLib   string `xml:"castLib,attr"`
}

// WallItems represents wall-mounted items (usually empty in basic studios)
type WallItems struct {
	Items []WallItem `xml:"Item"`
}

// WallItem represents an individual wall item
type WallItem struct {
	Row    int    `xml:"row,attr"`
	Col    int    `xml:"col,attr"`
	Type   string `xml:"type,attr"`
	Member string `xml:"member,attr"`
}

// StaticItems represents static bitmap items positioned in the scene (public rooms)
type StaticItems struct {
	Items []StaticItem `xml:"Item"`
}

// StaticItem represents an individual static item in the scene XML
// We only map the attributes we actually use for drawing
// Example: <Item ... member="london_bartable_a_0_2_1_0_0" x="279" y="238" ink="36" blend="100" .../>
type StaticItem struct {
	Member string `xml:"member,attr"`
	X      int    `xml:"x,attr"`
	Y      int    `xml:"y,attr"`
	Ink    int    `xml:"ink,attr"`
	Blend  int    `xml:"blend,attr"`
	Row    int    `xml:"row,attr"` // optional, may be 0
	Col    int    `xml:"col,attr"` // optional, may be 0
	Layer  int    `xml:"layer,attr"`
}

// PathMapXML represents the PathMap XML structure for walkable tiles
type PathMapXML struct {
	XMLName xml.Name   `xml:"PathMap"`
	Layout  int        `xml:"layout,attr"`
	X       int        `xml:"x,attr"`
	Y       int        `xml:"y,attr"`
	BGImage string     `xml:"bgimage,attr"`
	BGX     int        `xml:"bgx,attr"`
	BGY     int        `xml:"bgy,attr"`
	Cols    int        `xml:"cols,attr"`
	Rows    int        `xml:"rows,attr"`
	Nodes   []PathNode `xml:"Node"`
}

// PathNode represents a walkable/blocked tile in the PathMap
type PathNode struct {
	Row    int `xml:"r,attr"`
	Col    int `xml:"c,attr"`
	Weight int `xml:"w,attr"` // 0 = walkable, 10 = blocked
}

// ParseStudioXML parses a studio XML file
func ParseStudioXML(filePath string) (*StudioXML, error) {
	file, err := fs.Open(filepath.Clean(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to open studio XML file %s: %w", filePath, err)
	}
	defer file.Close()

	var studio StudioXML
	decoder := xml.NewDecoder(file)

	err = decoder.Decode(&studio)
	if err != nil {
		return nil, fmt.Errorf("failed to parse studio XML: %w", err)
	}

	return &studio, nil
}

// ParsePathMapXML parses a PathMap XML file
func ParsePathMapXML(filePath string) (*PathMapXML, error) {
	file, err := fs.Open(filepath.Clean(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to open PathMap XML file %s: %w", filePath, err)
	}
	defer file.Close()

	var pathMap PathMapXML
	decoder := xml.NewDecoder(file)

	err = decoder.Decode(&pathMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PathMap XML: %w", err)
	}

	return &pathMap, nil
}

// LoadStudioData loads both studio and PathMap data for a given studio layout
func LoadStudioData(dataDir string, layoutType int) (*StudioXML, *PathMapXML, error) {
	// Public rooms (layout 5) use location-specific files (London)
	if layoutType == 5 {
		studioPath := filepath.Join("Lingo", "london", "58_SceneXml.txt")
		pathMapPath := filepath.Join("Lingo", "london", "59_MapXml.txt")

		studio, err := ParseStudioXML(studioPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load public room studio data: %w", err)
		}
		pathMap, err := ParsePathMapXML(pathMapPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load public room PathMap data: %w", err)
		}
		return studio, pathMap, nil
	}

	// Map layout types to file numbers and letters for private studios
	studioFileMap := map[int]string{
		1: "138_Studio_A.txt",
		2: "139_Studio_B.txt",
		3: "140_Studio_C.txt",
		4: "141_Studio_D.txt",
	}

	pathMapFileMap := map[int]string{
		1: "142_Studio_A_MapXml.txt",
		2: "143_Studio_B_MapXml.txt",
		3: "144_Studio_C_MapXml.txt",
		4: "145_Studio_D_MapXml.txt",
	}

	studioFile, exists := studioFileMap[layoutType]
	if !exists {
		return nil, nil, fmt.Errorf("unsupported layout type: %d", layoutType)
	}

	pathMapFile, exists := pathMapFileMap[layoutType]
	if !exists {
		return nil, nil, fmt.Errorf("unsupported layout type for PathMap: %d", layoutType)
	}

	// Load studio XML (files are in root Lingo directory, not data subdirectory)
	studioPath := filepath.Join("Lingo", "isoengine", studioFile)

	studio, err := ParseStudioXML(studioPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load studio data: %w", err)
	}

	// Load PathMap XML
	pathMapPath := filepath.Join("Lingo", "isoengine", pathMapFile)

	pathMap, err := ParsePathMapXML(pathMapPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load PathMap data: %w", err)
	}

	return studio, pathMap, nil
}

// ApplyPathMapToScene applies PathMap data to an IsoScene
func ApplyPathMapToScene(scene *IsoScene, pathMap *PathMapXML) error {
	if scene == nil || pathMap == nil {
		return fmt.Errorf("scene or pathMap is nil")
	}

	// Set all squares to blocked by default
	grid := scene.GetGrid()
	if grid == nil {
		return fmt.Errorf("scene grid is not initialized")
	}

	// First, mark all squares as blocked (weight 10)
	for row := 1; row <= grid.Rows; row++ {
		for col := 1; col <= grid.Cols; col++ {
			scene.SetSquareWeight(row, col, 10)         // Blocked
			scene.SetSquareOriginalWeight(row, col, 10) // Store as original weight too
		}
	}

	// Then, mark walkable squares from PathMap data
	walkableCount := 0
	for _, node := range pathMap.Nodes {
		// TEST: Try flipping row coordinates to see if that fixes the mapping
		// PathMap shows walkable tiles at high row numbers (5-14)
		// If they should appear at the top of the screen, we might need to flip

		// FIXED: Grid needs to be rotated 270 degrees total - swap row/col and flip col
		mappedRow := node.Col                    // PathMap col becomes grid row
		mappedCol := pathMap.Cols - node.Row + 1 // PathMap row becomes flipped grid col

		scene.SetSquareWeight(mappedRow, mappedCol, node.Weight)
		scene.SetSquareOriginalWeight(mappedRow, mappedCol, node.Weight)

		if node.Weight == 0 {
			walkableCount++
			// Debug: Log first few walkable tiles
			if walkableCount <= 5 {
				log.Printf("PathMap: Node r=%d c=%d -> Grid[%d,%d] (270° rotated: col->row, flipped row->col)",
					node.Row, node.Col, mappedRow, mappedCol)
			}
		}
	}

	log.Printf("PathMap: Applied %d walkable tiles", walkableCount)

	return nil
}

// GetFloorTilePositions returns all floor tile positions from studio data
func GetFloorTilePositions(studio *StudioXML) []FloorTile {
	var positions []FloorTile

	// Add regular floor tiles
	positions = append(positions, studio.Floor.FloorTiles.Tiles...)

	// Add door floor tile
	if studio.Door.FloorTile.Row > 0 && studio.Door.FloorTile.Col > 0 {
		positions = append(positions, studio.Door.FloorTile)
	}

	return positions
}

// ConvertStudioLayoutToIsoLayout converts studio layout number to iso layout
// Studio layouts 1-4 (A-D) all use iso layout 2 (large grid)
func ConvertStudioLayoutToIsoLayout(studioLayout int) int {
	// Public rooms (layout 5) use small grid (layout 1 = 30x30)
	if studioLayout == 5 {
		return 1
	}
	// Private studios (1-4) use large grid (layout 2 = 15x15)
	return 2
}
