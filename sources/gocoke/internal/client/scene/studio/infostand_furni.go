package studio

import (
	"image"
	"path/filepath"
	"strings"

	lingoassets "gocoke/internal/client/assets/lingo"

	"github.com/hajimehoshi/ebiten/v2"
)

// ShowForFurni populates the InfoStand for a placed furniture item
func (i *InfoStand) ShowForFurni(pf *PlacedFurni) {
	if pf == nil {
		return
	}
	// Clear avatar context and set furniture context
	i.selectedAvatar = nil
	i.previewAvatar = nil
	i.selectedFurniID = strings.TrimSpace(pf.PosID)
	// Disable now playing mode when explicitly showing a furni (but keep stored custom title/body)
	i.nowPlayingActive = false
	i.visible = true

	// Title/body from catalog if available via scene (will be provided by caller)
	// Fallback to base if missing
	i.furniTitle = pf.Base
	i.furniBody = ""

	// Build preview image for furniture from its base name (full-size if available)
	// Try to resolve from cc_furniture[1] first; fallback to small
	var img *ebiten.Image
	if path, ok := i.resolveFurnitureImagePath(pf.Base); ok {
		if im, err := lingoassets.LoadImage(path); err == nil {
			img = im
		}
	}
	i.furniPreview = img

	// Build default button set for furniture (placeholder actions)
	i.rebuildButtonsForFurni()
}

// Resolve at action time: the room's furniture slice can move or compact between
// selection and click. A pointer saved into that slice can target a different item.
func (i *InfoStand) resolveSelectedFurniture() *PlacedFurni {
	if i.selectedFurniID != "" && i.ResolveFurniture != nil {
		if pf := i.ResolveFurniture(i.selectedFurniID); pf != nil {
			return pf
		}
	}
	i.Clear()
	return nil
}

func (s *Scene) findPlacedFurniByID(posID string) *PlacedFurni {
	if s == nil || strings.TrimSpace(posID) == "" {
		return nil
	}
	for idx := range s.placedFurni {
		if strings.TrimSpace(s.placedFurni[idx].PosID) == strings.TrimSpace(posID) {
			return &s.placedFurni[idx]
		}
	}
	return nil
}

// resolveFurnitureImagePath tries to find a path for a furniture preview based on base name
func (i *InfoStand) resolveFurnitureImagePath(base string) (string, bool) {
	// Always use small furniture thumbnail
	smallDir := filepath.Join("Lingo", "cc_furniture_small")
	mm, _ := lingoassets.LoadMemberMapCached(smallDir)
	if mm != nil {
		if p, ok := mm.PathFor(base + "_small"); ok {
			return p, true
		}
	}
	return "", false
}

// RenderFurniPreview draws furniture preview in the same preview box position used for avatars
func (i *InfoStand) RenderFurniPreview(screen *ebiten.Image) {
	if i.furniPreview == nil {
		return
	}
	b := i.furniPreview.Bounds()
	w := b.Dx()
	h := b.Dy()
	// Scale down large previews to fit approx 64x64 area
	maxW := 64
	maxH := 64
	sx := 1.0
	sy := 1.0
	if w > maxW || h > maxH {
		sx = float64(maxW) / float64(w)
		sy = float64(maxH) / float64(h)
	}
	x := prevCX - int(float64(w)*sx)/2
	// Move the small furniture preview up by ~15px for better alignment
	y := prevCY - int(float64(h)*sy) - 15
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(sx, sy)
	op.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(i.furniPreview, op)
}

// PopulateFurniText sets the title/body for the furni based on catalog data provided by caller
func (i *InfoStand) PopulateFurniText(name, roomDesc string) {
	if strings.TrimSpace(name) != "" {
		i.furniTitle = name
	}
	i.furniBody = roomDesc
}

// rebuildButtonsForFurni sets the button list for furniture based on ownership rules (placeholder)
func (i *InfoStand) rebuildButtonsForFurni() {
	// Only room owner can manipulate furniture
	actions := []string{}
	if i.IsRoomOwner != nil && i.IsRoomOwner() {
		actions = []string{"Move", "Rotate", "Pick up", "Delete"}
	}
	i.buttons = nil
	curRight := descX + descW
	for _, label := range actions {
		active := i.composeButton(label, false)
		pressed := i.composeButton(label, true)
		if active == nil || pressed == nil {
			continue
		}
		b := active.Bounds()
		rect := image.Rect(curRight-b.Dx(), descY+descH, curRight, descY+descH+b.Dy())
		curRight = rect.Min.X - 3
		i.buttons = append(i.buttons, infoStandButton{ID: label, Rect: rect, Active: active, PressedI: pressed})
	}
}
