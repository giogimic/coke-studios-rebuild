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

func TestPlaceFromServerAppliesServerGridYAndServerHO(t *testing.T) {
	cat, err := catalog.LoadFile("data/Catalogue_English.txt")
	if err != nil {
		t.Fatal(err)
	}
	s := snapshotScene(cat)
	item := cat.Items[0]

	// 1. Initial placement atop a table (serverY=1, serverHO=32)
	if !s.placement.PlaceFromServer("test_item_1", item.ProdID, 5, 1, 10, 32, 2, 0, false) {
		t.Fatal("initial placement failed")
	}
	if len(s.placedFurni) != 1 {
		t.Fatalf("expected 1 placed item, got %d", len(s.placedFurni))
	}
	pf := s.placedFurni[0]
	if pf.GridY != 10 {
		t.Errorf("expected GridY=10 from serverY=1, got %d", pf.GridY)
	}
	if pf.ServerHO != 32 {
		t.Errorf("expected ServerHO=32 from serverHO=32, got %d", pf.ServerHO)
	}

	// 2. Move item to floor (serverY=0, serverHO=0): must reset stack height and HO
	if !s.placement.PlaceFromServer("test_item_1", item.ProdID, 6, 0, 9, 0, 2, 0, false) {
		t.Fatal("move to floor failed")
	}
	pf = s.placedFurni[0]
	if pf.GridY != 0 {
		t.Errorf("expected GridY=0 on floor, got %d", pf.GridY)
	}
	if pf.ServerHO != 0 {
		t.Errorf("expected ServerHO=0 on floor, got %d", pf.ServerHO)
	}
}

func TestDirectionOffsetIdempotency(t *testing.T) {
	cat, err := catalog.LoadFile("data/Catalogue_English.txt")
	if err != nil {
		t.Fatal(err)
	}
	s := snapshotScene(cat)
	for _, item := range cat.Items {
		pf := PlacedFurni{
			ProdID: item.ProdID,
			Base:   item.ImageBase,
			Width:  item.Width,
			Depth:  item.Depth,
		}
		for _, dir := range []int{0, 2, 4, 6} {
			dx, dy := s.CalculateDirectionOffset(&pf, dir)
			// Calling it multiple times must return the identical result
			dx2, dy2 := s.CalculateDirectionOffset(&pf, dir)
			if dx != dx2 || dy != dy2 {
				t.Errorf("CalculateDirectionOffset non-deterministic for %s dir %d: (%d,%d) vs (%d,%d)",
					item.ProdID, dir, dx, dy, dx2, dy2)
			}
		}
	}
}

func TestRollbackPlacementRestoresBackpack(t *testing.T) {
	cat, err := catalog.LoadFile("data/Catalogue_English.txt")
	if err != nil {
		t.Fatal(err)
	}
	s := snapshotScene(cat)
	s.backpackManager = &BackpackManager{}
	item := cat.Items[0]

	// Place item
	if !s.placement.PlaceFromServer("pos_rollback_1", item.ProdID, 5, 0, 10, 0, 2, 0, false) {
		t.Fatal("placement failed")
	}
	if len(s.placedFurni) != 1 {
		t.Fatalf("expected 1 placed item, got %d", len(s.placedFurni))
	}

	// Trigger rollback (e.g. from server rejection <POS_REJECT>)
	s.placement.RollbackPlacement("pos_rollback_1", item.ProdID)

	// Item must be removed from placedFurni
	if len(s.placedFurni) != 0 {
		t.Fatalf("expected 0 placed items after rollback, got %d", len(s.placedFurni))
	}

	// Item must be restored to backpack
	if len(s.backpackManager.items) != 1 {
		t.Fatalf("expected 1 item in backpack, got %d", len(s.backpackManager.items))
	}
	bpItem := s.backpackManager.items[0]
	if bpItem.PosID != "pos_rollback_1" || bpItem.ProdID != item.ProdID {
		t.Errorf("unexpected backpack item: %+v", bpItem)
	}
}
