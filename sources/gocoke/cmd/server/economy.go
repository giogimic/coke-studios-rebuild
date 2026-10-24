package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	ErrItemNotFound         = errors.New("catalogue item not found")
	ErrInsufficientFunds    = errors.New("insufficient decibels for purchase")
	ErrUnauthorizedPurchase = errors.New("unauthorized purchase attempt")
	ErrItemNotOwned         = errors.New("possession not owned in backpack")
	ErrNotStudioOwner       = errors.New("only studio owner may arrange furniture")
	ErrStudioNotFound       = errors.New("studio not found")
)

// CatalogItem represents an authenticated item in the Coke Studios catalogue.
type CatalogItem struct {
	ID                int            `json:"id"`
	Name              string         `json:"name"`
	Type              string         `json:"type"`
	Action            string         `json:"action"`
	CatID             int            `json:"catId"`
	CatName           string         `json:"catName"`
	CatDesc           string         `json:"catDesc"`
	RoomDesc          string         `json:"roomDesc"`
	ImageBase         string         `json:"imageBase"`
	Price             int            `json:"price"`
	Width             int            `json:"width"`
	Depth             int            `json:"depth"`
	Height            int            `json:"height"`
	ItemsAllowedOnTop bool           `json:"itemsAllowedOnTop"`
	Modifiable        bool           `json:"modifiable"`
	Attributes        map[string]any `json:"attributes"`
}

// CanonicalCatalogPrices records the evidenced historical prices in Decibels.
var CanonicalCatalogPrices = map[int]int{
	0:   500,  // Decibels
	1:   500,  // Wallpaper
	2:   500,  // Floor
	3:   500,  // Premier Chair
	4:   450,  // Premier Stool
	5:   625,  // Premier Sofa
	6:   450,  // Premier Coffee Table
	7:   950,  // Premier Table
	8:   500,  // Premier Stereo
	9:   500,  // Premier Minibar
	10:  450,  // Premier Light
	11:  500,  // 5001 Chair
	12:  450,  // 5001 Stool
	13:  625,  // 5001 Sofa
	14:  450,  // 5001 Coffee Table
	15:  950,  // 5001 Table
	16:  500,  // 5001 Sound System
	17:  500,  // 5001 Minibar
	18:  600,  // 5001 Cube Light
	19:  450,  // Equipment Rack
	20:  900,  // Sampler Rack
	21:  450,  // Wooden Speaker
	22:  100,  // Microphone
	23:  50,   // Cables
	24:  575,  // TV
	25:  650,  // Retro Speaker
	26:  50,   // Blank CD (10 pack / burn fee)
	27:  600,  // Acoustic Pad
	28:  600,  // Tinted Separator
	29:  350,  // Potted Plant
	30:  75,   // Tulip
	31:  450,  // Psychedelic Carpet
	32:  450,  // Straw Mat
	33:  600,  // Furry Rug
	34:  400,  // Corner Piece
	35:  225,  // White Separator
	36:  225,  // Beige Separator
	37:  225,  // Grey Separator
	38:  225,  // Yellow Separator
	39:  225,  // Steel Separator
	40:  225,  // Orange Separator
	41:  225,  // Green Separator
	42:  225,  // Blue Separator
	43:  500,  // Minibar
	44:  650,  // Leather Chair
	45:  200,  // Coke Cooler
	46:  800,  // Polar Bear
	47:  450,  // Coke CD Player
	48:  950,  // Lava Lamp
	49:  2000, // Coca-Cola Neon Sign
	50:  2500, // Coke Couch
	51:  225,  // Guitar
	52:  225,  // Bass
	53:  225,  // Trumpet
	54:  225,  // Acoustic Foam
	55:  225,  // Retro Keyboard
	56:  225,  // Disco Poster
	57:  225,  // Tropical Poster
	58:  500,  // Top 40 Chart
	74:  450,  // Old Fridge
	75:  450,  // Boom Box
	76:  500,  // Instant Bookshelf
	77:  450,  // Dorm Light
	78:  625,  // Instant Table
	79:  450,  // Funky Chair
	80:  625,  // Stylish Couch
	81:  625,  // Surf Board
	82:  500,  // Gold Record
	83:  500,  // Platinum Record
	84:  500,  // Coca-Cola Pinball Machine
	85:  500,  // Tatami Mat
	86:  500,  // Rice Paper Divider
	87:  500,  // Gong
	88:  500,  // Mixer
	89:  500,  // BurnedCD
	90:  500,  // Vanilla Coke Gumball Machine
	91:  500,  // Cow Pattern Bean Bag Chair
	92:  500,  // WallShelf
	93:  500,  // Vanilla Coke Machine
	94:  500,  // Vanilla Coke Shelf
	95:  500,  // Trash Can
	96:  500,  // Vanilla Coke Table
	97:  500,  // Eyeball Lamp
	98:  500,  // Shark Portal
	99:  500,  // Deluxe Aquarium
	100: 500,  // Diving Suit
	101: 500,  // Fish Porthole
	102: 500,  // Life Preserver
	103: 500,  // Lighthouse Buoy
	104: 500,  // Oyster Couch
	105: 500,  // Fishy
	106: 500,  // Placeholder
	107: 500,  // Lighthouse Aquarium
	108: 500,  // Treasure Chest
	109: 500,  // Promo Poster
	110: 500,  // Ocean Floor Table
	111: 500,  // Aquatic Divider
	112: 500,  // Sea Sponge Barrier
	113: 500,  // Molten Magma Lamp
	114: 500,  // Coral Mood Light
	115: 500,  // Scrap Metal Carpet
	116: 500,  // Robot Sculpture
	117: 500,  // Recycling Bin
	118: 500,  // Tire Chair
	119: 500,  // Gear Table
	120: 500,  // Comet Curtain
	121: 500,  // Pop Art Poster
	122: 500,  // Diva Marquee
	123: 500,  // Playa Marquee
	124: 500,  // Street Styles
	125: 500,  // Red Couch
	126: 500,  // El Vego Placeholder
	127: 500,  // Wardrobe Trunk
	128: 500,  // Trophy Case
	129: 500,  // Piano
	130: 500,  // Congas
	131: 500,  // Vintage Amp
	132: 500,  // Plasma Screen
	133: 500,  // Star Rug
	134: 500,  // Royal Throne
	135: 500,  // Res-O-Scanner
	136: 500,  // Item Dispenser
	137: 500,  // Hologram
	138: 500,  // Batman Begins Poster
	140: 500,  // Retro TV
	141: 500,  // Lawn Chair
	142: 500,  // Refrigerator
	143: 500,  // Oven
	144: 500,  // Kitchen Sink
	145: 500,  // Kitchen Cabinet
	146: 500,  // Bookshelf
	147: 500,  // Fireplace
	148: 500,  // Entertainment Center
	149: 500,  // Black Traffic Light
	150: 500,  // Heart Poster
	151: 500,  // Spider Web
	152: 500,  // Winter View
	153: 500,  // City View
	154: 500,  // Beach View
	155: 500,  // Curtain
	156: 500,  // Grey Traffic Light
	157: 500,  // Infinite Jukebox
	158: 500,  // Randomatic Number Podium
	159: 500,  // Randomatic Number Display
	160: 500,  // Randomatic Letter Podium
	161: 500,  // Randomatic Letter Display
	162: 500,  // DJ Stand
	163: 500,  // DJ Console
	164: 500,  // Robot Dog
	165: 500,  // Cushion
	166: 500,  // Dryer
	167: 500,  // Washer
	168: 500,  // Victorian Chair
	169: 500,  // Victorian Table
	170: 500,  // Arcade Sign
	171: 500,  // V-Ball Machine
}

var (
	reCatalogProp     = regexp.MustCompile(`#(\w+):\s*"([^"]*)"`)
	reCatalogIntProp  = regexp.MustCompile(`#(\w+):\s*(\d+)`)
	reCatalogBoolProp = regexp.MustCompile(`#(\w+):\s*(true|false)`)
)

func (s *Server) InitCatalog() {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()

	s.catalog = make(map[int]*CatalogItem)
	s.catalogList = make([]*CatalogItem, 0)

	// Seed canonical items from price map
	for id, price := range CanonicalCatalogPrices {
		ci := &CatalogItem{
			ID:         id,
			Name:       fmt.Sprintf("Item %d", id),
			Price:      price,
			Attributes: make(map[string]any),
		}
		s.catalog[id] = ci
		s.catalogList = append(s.catalogList, ci)
	}

	// Try reading Catalogue_English.txt from candidate locations
	pathsToTry := []string{
		filepath.Join(s.webRoot, "data", "Catalogue_English.txt"),
		filepath.Join(findRepoRoot(), "sources", "gocoke", "web", "data", "Catalogue_English.txt"),
		filepath.Join("web", "data", "Catalogue_English.txt"),
		filepath.Join("..", "..", "web", "data", "Catalogue_English.txt"),
	}

	var dataFile string
	for _, p := range pathsToTry {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			dataFile = p
			break
		}
	}

	if dataFile != "" {
		if err := s.loadCatalogFile(dataFile); err != nil {
			log.Printf("[Economy] Warning: failed to parse %s: %v", dataFile, err)
		} else {
			log.Printf("[Economy] Loaded %d catalogue items from %s", len(s.catalog), dataFile)
		}
	} else {
		log.Printf("[Economy] Catalogue_English.txt not found on disk; using %d canonical definitions", len(s.catalog))
	}

	sort.Slice(s.catalogList, func(i, j int) bool {
		return s.catalogList[i].ID < s.catalogList[j].ID
	})
}

func (s *Server) loadCatalogFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var buf strings.Builder
	inItem := false

	flushItem := func() {
		raw := strings.TrimSpace(buf.String())
		buf.Reset()
		inItem = false
		if raw == "" {
			return
		}
		if idx := strings.Index(raw, "["); idx >= 0 {
			raw = raw[idx+1:]
		}
		if strings.HasSuffix(raw, "]") {
			raw = raw[:len(raw)-1]
		}
		raw = strings.TrimSpace(raw)

		ci := parseCatalogItemProps(raw)
		if ci == nil || ci.ID < 0 {
			return
		}

		// Apply evidenced canonical price
		if p, ok := CanonicalCatalogPrices[ci.ID]; ok && p > 0 {
			ci.Price = p
		} else if ci.Price == 0 {
			ci.Price = 500
		}

		if existing, ok := s.catalog[ci.ID]; ok {
			*existing = *ci
		} else {
			s.catalog[ci.ID] = ci
			s.catalogList = append(s.catalogList, ci)
		}
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "# ") {
			continue
		}
		if !inItem {
			if strings.HasPrefix(line, "ITEM=[") {
				inItem = true
				buf.WriteString(line)
				if strings.HasSuffix(line, "]") {
					flushItem()
				}
			}
			continue
		}
		buf.WriteString(" ")
		buf.WriteString(line)
		if strings.HasSuffix(line, "]") {
			flushItem()
		}
	}

	return scanner.Err()
}

func parseCatalogItemProps(props string) *CatalogItem {
	ci := &CatalogItem{
		ID:         -1,
		Attributes: make(map[string]any),
	}

	for _, match := range reCatalogProp.FindAllStringSubmatch(props, -1) {
		k, v := match[1], match[2]
		switch k {
		case "prodId":
			ci.ID, _ = strconv.Atoi(v)
		case "name":
			ci.Name = v
		case "type":
			ci.Type = v
		case "action":
			ci.Action = v
		case "catName":
			ci.CatName = v
		case "catId":
			ci.CatID, _ = strconv.Atoi(v)
		case "catDesc":
			ci.CatDesc = v
		case "roomDesc":
			ci.RoomDesc = v
		case "imageBase":
			ci.ImageBase = v
		}
	}

	for _, match := range reCatalogIntProp.FindAllStringSubmatch(props, -1) {
		k, vStr := match[1], match[2]
		v, _ := strconv.Atoi(vStr)
		switch k {
		case "width":
			ci.Width = v
		case "depth":
			ci.Depth = v
		case "height":
			ci.Height = v
		}
	}

	for _, match := range reCatalogBoolProp.FindAllStringSubmatch(props, -1) {
		k, vStr := match[1], match[2]
		b := (vStr == "true")
		switch k {
		case "itemsAllowedOnTop":
			ci.ItemsAllowedOnTop = b
		case "modifiable":
			ci.Modifiable = b
		}
	}

	return ci
}

func (s *Server) GetCatalogItem(id int) (*CatalogItem, error) {
	s.catalogMu.RLock()
	defer s.catalogMu.RUnlock()

	if item, ok := s.catalog[id]; ok {
		return item, nil
	}
	return nil, ErrItemNotFound
}

func (s *Server) GetAllCatalogItems() []*CatalogItem {
	s.catalogMu.RLock()
	defer s.catalogMu.RUnlock()

	items := make([]*CatalogItem, len(s.catalogList))
	copy(items, s.catalogList)
	return items
}

func (s *Server) GetProductListAMF() []any {
	items := s.GetAllCatalogItems()
	list := make([]any, 0, len(items))
	for _, it := range items {
		list = append(list, map[string]any{
			"id":     float64(it.ID),
			"prodId": float64(it.ID),
			"price":  float64(it.Price),
			"name":   it.Name,
		})
	}
	return list
}

func (s *Server) PrePurchase(screenName string, catID int, attributes string) (map[string]any, error) {
	item, err := s.GetCatalogItem(catID)
	if err != nil {
		return nil, ErrItemNotFound
	}

	bal, _, err := s.store.GetBalance(screenName)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"balance":       float64(bal),
		"price":         float64(item.Price),
		"catalogItemId": float64(item.ID),
		"prodId":        float64(item.ID),
		"properties":    attributes,
	}, nil
}

func (s *Server) Purchase(screenName string, catID int, attributes map[string]any, idempotencyKey string) ([]any, error) {
	item, err := s.GetCatalogItem(catID)
	if err != nil {
		return nil, ErrItemNotFound
	}

	posID, _, err := s.store.PurchaseItemAtomic(screenName, catID, item.Price, attributes, idempotencyKey)
	if err != nil {
		return nil, err
	}

	if attributes == nil {
		attributes = make(map[string]any)
	}

	return []any{
		map[string]any{
			"id":          float64(posID),
			"catalogItem": float64(catID),
			"prodID":      float64(catID),
			"properties":  attributes,
		},
	}, nil
}
