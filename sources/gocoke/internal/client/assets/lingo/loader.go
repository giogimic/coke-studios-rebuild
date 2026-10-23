package lingoassets

import (
	"log"
	"path/filepath"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"gocoke/internal/client/assets/lingo/window"
)

// Caches for member maps, window definitions, and images
var (
	mmCache    sync.Map // key: dir -> *MemberMap
	winDefCache sync.Map // key: path -> *window.Definition
	imgCache   sync.Map // key: absolute path -> *ebiten.Image
)

// LoadMemberMapCached loads a MemberMap for a directory, with memoization.
func LoadMemberMapCached(dir string) (*MemberMap, error) {
	if v, ok := mmCache.Load(dir); ok {
		return v.(*MemberMap), nil
	}
	mm, err := LoadMemberMap(dir)
	if err != nil {
		return nil, err
	}
	mmCache.Store(dir, mm)
	return mm, nil
}

// ParseWindowCached parses a window definition with memoization.
func ParseWindowCached(path string) (*window.Definition, error) {
	if v, ok := winDefCache.Load(path); ok {
		return v.(*window.Definition), nil
	}
	def, err := window.ParseFile(path)
	if err != nil {
		return nil, err
	}
	winDefCache.Store(path, def)
	return def, nil
}

// loadImageCached loads an ebiten.Image from disk with memoization using LoadImage (alpha-safe).
func loadImageCached(path string) (*ebiten.Image, error) {
	if v, ok := imgCache.Load(path); ok {
		return v.(*ebiten.Image), nil
	}
	img, err := LoadImage(path)
	if err != nil {
		return nil, err
	}
	imgCache.Store(path, img)
	return img, nil
}

// findMemberPath searches provided Lingo directories for a member by name using their MemberMaps.
func findMemberPath(member string, dirs []string) (string, bool) {
	for _, dir := range dirs {
		mm, err := LoadMemberMapCached(dir)
		if err != nil {
			log.Printf("lingoassets: member map load failed for %s: %v", dir, err)
			continue
		}
		if p, ok := mm.PathFor(member); ok {
			return p, true
		}
	}
	return "", false
}

// LoadAssetsForDefinition loads all bitmap assets referenced by a window definition
// by resolving member names against provided Lingo directories (in order).
func LoadAssetsForDefinition(def *window.Definition, dirs ...string) map[string]*ebiten.Image {
	assets := make(map[string]*ebiten.Image)
	if def == nil {
		return assets
	}
	for _, el := range def.Elements {
		if el.Media != "bitmap" {
			continue
		}
		member := el.Member
		if member == "" {
			continue
		}
		if _, ok := assets[member]; ok {
			continue // already loaded
		}
		if p, found := findMemberPath(member, dirs); found {
			if img, err := loadImageCached(filepath.Clean(p)); err == nil {
				assets[member] = img
			} else {
				log.Printf("lingoassets: failed to load asset %s from %s: %v", member, p, err)
			}
		} else {
			log.Printf("lingoassets: asset not found for member %s in %v", member, dirs)
		}
	}
	return assets
}

// LoadAssetsByNames loads specific members by name from the provided Lingo directories (first hit wins).
func LoadAssetsByNames(names []string, dirs ...string) map[string]*ebiten.Image {
	assets := make(map[string]*ebiten.Image)
	for _, name := range names {
		if p, found := findMemberPath(name, dirs); found {
			if img, err := loadImageCached(filepath.Clean(p)); err == nil {
				assets[name] = img
			} else {
				log.Printf("lingoassets: failed to load asset %s from %s: %v", name, p, err)
			}
		} else {
			log.Printf("lingoassets: asset not found for member %s in %v", name, dirs)
		}
	}
	return assets
}

