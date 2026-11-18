package studio

import (
	"github.com/hajimehoshi/ebiten/v2"
	lingo "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/studios"
	"gocoke/internal/client/isoengine"
	"testing"
)

func TestLondonDJClickUsesRenderedCastMember(t *testing.T) {
	xml, err := isoengine.ParseStudioXML("Lingo/london/58_SceneXml.txt")
	if err != nil {
		t.Fatal(err)
	}
	members, err := lingo.LoadMemberMapCached("Lingo/london")
	if err != nil {
		t.Fatal(err)
	}
	member := "london_djtable_a_0_0_0"
	path, ok := members.PathFor(member)
	if !ok {
		t.Fatal("DJ table absent from recovered cast")
	}
	asset, err := lingo.LoadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	s := &Scene{studio: &studios.Studio{StudioID: "$SYSTEM$1", Layout: 5}, studioXML: xml, assets: map[string]*ebiten.Image{member: asset}, playerAvatar: &isoengine.IsoAvatar{}}
	if err := s.loadMemberRegistry("data"); err != nil {
		t.Fatal(err)
	}
	bounds, ok := s.publicDJBounds()
	if !ok || bounds.Empty() {
		t.Fatal("rendered DJ table has no click target")
	}
	calls := 0
	s.sendCDRequestIfAvailable = func(room, id string) error {
		if room != "$SYSTEM$1" || id != "public:london:dj" {
			t.Fatalf("wrong request %q %q", room, id)
		}
		calls++
		return nil
	}
	if !s.handlePublicDJClick(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y+bounds.Dy()/2) || calls != 1 {
		t.Fatal("table click did not request song chooser")
	}
	if s.handlePublicDJClick(bounds.Max.X+1, bounds.Max.Y+1) {
		t.Fatal("click outside table consumed")
	}
}

func TestHandleCDMintedAddsToBackpackAndUpdatesBalance(t *testing.T) {
	s := &Scene{
		backpackManager: &BackpackManager{},
		hudManager:      &HUDManager{},
	}

	s.HandleCDMinted("9876", "89", "Summer Anthem", "^50,danceOne_bass00{4,0{^", "490")

	if len(s.backpackManager.items) != 1 {
		t.Fatalf("expected 1 backpack item, got %d", len(s.backpackManager.items))
	}
	item := s.backpackManager.items[0]
	if item.PosID != "9876" || item.ProdID != "89" {
		t.Errorf("unexpected minted item: %+v", item)
	}
	if s.hudManager.GetBalance() != 490 {
		t.Errorf("expected balance 490, got %d", s.hudManager.GetBalance())
	}
}

func TestPublicDJBoundsForVenues(t *testing.T) {
	venues := []struct {
		roomID   string
		location int
		member   string
	}{
		{"$SYSTEM$19", 10, "reykjavik_turntables"},
		{"$SYSTEM$3", 2, "miami_mixer1"},
		{"$SYSTEM$27", 14, "dj"},
		{"$SYSTEM$17", 9, "goa_stage_pink"},
		{"$SYSTEM$21", 11, "mosmixa"},
		{"$SYSTEM$5", 3, "mom_mixah_1_anim0"},
	}

	dummyImg := ebiten.NewImage(32, 32)
	for _, v := range venues {
		s := &Scene{
			studio: &studios.Studio{
				StudioID: v.roomID,
				Layout:   5,
				Location: v.location,
			},
			studioXML: &isoengine.StudioXML{
				StaticItems: isoengine.StaticItems{
					Items: []isoengine.StaticItem{
						{Member: v.member, X: 100, Y: 150},
					},
				},
			},
			assets: map[string]*ebiten.Image{
				v.member: dummyImg,
			},
		}

		bounds, ok := s.publicDJBounds()
		if !ok || bounds.Empty() {
			t.Fatalf("failed to find DJ booth bounds for venue %s member %s", v.roomID, v.member)
		}
		if bounds.Min.X != 100 || bounds.Min.Y != 150 {
			t.Errorf("unexpected bounds for %s: got %v", v.roomID, bounds)
		}
	}
}
