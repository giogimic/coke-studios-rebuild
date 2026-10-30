package studio

import "testing"

// Exercise the actual selection and button dispatch, without a graphics loop.
func TestFurnitureSelectionSurvivesListChanges(t *testing.T) {
	for _, action := range []string{"Rotate", "Move", "Pick up"} {
		for _, change := range []string{"append", "remove preceding", "remove selected"} {
			t.Run(action+"/"+change, func(t *testing.T) {
				s := &Scene{placedFurni: []PlacedFurni{{PosID: "before"}, {PosID: "selected", Dir: 2}, {PosID: "after"}}}
				i := &InfoStand{ResolveFurniture: s.findPlacedFurniByID}
				i.ShowForFurni(&s.placedFurni[1])
				switch change {
				case "append":
					// The initial literal has len==cap, so appending replaces the backing array.
					s.placedFurni = append(s.placedFurni, PlacedFurni{PosID: "new"})
				case "remove preceding":
					s.placedFurni = append(s.placedFurni[:0], s.placedFurni[1:]...)
				case "remove selected":
					s.placedFurni = append(s.placedFurni[:1], s.placedFurni[2:]...)
				}
				calls := 0
				callback := func(pf *PlacedFurni) {
					calls++
					if change == "remove selected" {
						t.Fatal("removed selection dispatched to another item")
					}
					if pf.PosID != "selected" {
						t.Fatalf("wrong possession targeted: %s", pf.PosID)
					}
					pf.Dir = 4
				}
				i.OnRotateFurni, i.OnMoveFurni, i.OnPickupFurni = callback, callback, callback
				i.onButtonClick(action)
				if change == "remove selected" {
					if calls != 0 || i.visible {
						t.Fatal("removed selection was not cleared")
					}
					return
				}
				if calls != 1 {
					t.Fatalf("expected one action, got %d", calls)
				}
				for _, pf := range s.placedFurni {
					if pf.PosID == "selected" && pf.Dir != 4 {
						t.Fatal("action mutated stale furniture copy instead of live item")
					}
				}
			})
		}
	}
}
