package isoengine

import (
	"testing"
)

func TestLoadStudioDataForPublicVenues(t *testing.T) {
	venues := []struct {
		name     string
		location int
	}{
		{"London", 1},
		{"Miami", 2},
		{"Mombasa", 3},
		{"Goa", 9},
		{"Alaska", 10},
		{"Moscow", 11},
		{"Central Park", 14},
	}

	for _, v := range venues {
		t.Run(v.name, func(t *testing.T) {
			studio, pathMap, err := LoadStudioDataForStudio("", 5, v.location)
			if err != nil {
				t.Fatalf("failed to load %s (loc %d): %v", v.name, v.location, err)
			}
			if studio == nil {
				t.Fatalf("nil studio for %s", v.name)
			}
			if pathMap == nil {
				t.Fatalf("nil pathMap for %s", v.name)
			}
			if len(pathMap.Nodes) == 0 {
				t.Fatalf("no nodes loaded in pathMap for %s", v.name)
			}

			// Verify scene integration
			scene := NewIsoScene()
			scene.SetLayout(1)
			scene.Init()
			if err := ApplyPathMapToScene(scene, pathMap); err != nil {
				t.Fatalf("failed to apply pathmap for %s: %v", v.name, err)
			}
		})
	}
}

func TestLoadStudioDataForPrivateStudios(t *testing.T) {
	layouts := []int{1, 2, 3, 4, 6, 7, 8}
	for _, l := range layouts {
		studio, pathMap, err := LoadStudioDataForStudio("", l, 1)
		if err != nil {
			t.Fatalf("failed to load private studio layout %d: %v", l, err)
		}
		if studio == nil || pathMap == nil {
			t.Fatalf("nil studio or pathMap for layout %d", l)
		}
		if len(pathMap.Nodes) == 0 {
			t.Fatalf("no nodes loaded for layout %d", l)
		}
	}
}

