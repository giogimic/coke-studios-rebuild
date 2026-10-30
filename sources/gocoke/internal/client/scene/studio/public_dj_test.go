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
