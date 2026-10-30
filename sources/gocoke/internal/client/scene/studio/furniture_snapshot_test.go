package studio

import (
	"fmt"
	"gocoke/internal/client/assets/catalog"
	"gocoke/internal/client/isoengine"
	"testing"
)

func snapshotScene(cat *catalog.Catalog) *Scene {
	s := &Scene{catalog: cat, isoScene: isoengine.NewIsoScene()}
	s.isoScene.SetLayout(2)
	s.isoScene.Init()
	s.placement = &PlacementHelper{scene: s}
	return s
}

func TestTwoByTwoFurnitureSnapshotAndMoveKeepAnchor(t *testing.T) {
	cat, err := catalog.LoadFile("data/Catalogue_English.txt")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range cat.Items {
		if item.Width != 2 || item.Depth != 2 {
			continue
		}
		count++
		for _, dir := range []int{0, 2, 4, 6} {
			t.Run(fmt.Sprintf("%s/dir%d", item.ProdID, dir), func(t *testing.T) {
				s := snapshotScene(cat)
				// Existing outbound placement/rotation sends row unchanged and col+1;
				// realtime converts col to 16-col. (8,5) therefore arrives as (8,10).
				if !s.placement.PlaceFromServer("owned", item.ProdID, 8, 0, 10, 0, dir, 0, false) {
					t.Fatal("snapshot rejected")
				}
				pf := s.placedFurni[0]
				if pf.Row != 8 || pf.Col != 5 {
					t.Errorf("rejoin changed anchor to (%d,%d), want (8,5)", pf.Row, pf.Col)
				}
				// Another player sees the owner's move through the same POS update.
				if !s.placement.PlaceFromServer("owned", item.ProdID, 9, 0, 8, 0, dir, 0, false) {
					t.Fatal("move update rejected")
				}
				pf = s.placedFurni[0]
				if len(s.placedFurni) != 1 || pf.Row != 9 || pf.Col != 7 {
					t.Errorf("move ignored or duplicated: count=%d anchor=(%d,%d)", len(s.placedFurni), pf.Row, pf.Col)
				}
			})
		}
	}
	if count == 0 {
		t.Fatal("catalogue fixture has no 2x2 items")
	}
}
