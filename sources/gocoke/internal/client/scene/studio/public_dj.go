package studio

import (
	"image"
)

// publicDJBounds uses the very same member and registration point as the room
// renderer. Only London's recovered cast is currently implemented here.
func (s *Scene) publicDJBounds() (image.Rectangle, bool) {
	if s.studioXML == nil || s.studio == nil {
		return image.Rectangle{}, false
	}
	switch s.studio.StudioID {
	case "$SYSTEM$1", "$SYSTEM$2", "london", "VIS$2":
	default:
		return image.Rectangle{}, false
	}
	for _, item := range s.studioXML.StaticItems.Items {
		if item.Member != "london_djtable_a_0_0_0" {
			continue
		}
		asset := s.assets[item.Member]
		if asset == nil {
			return image.Rectangle{}, false
		}
		x, y := item.X, item.Y
		if s.memberReg != nil {
			if reg, ok := s.memberReg.GetRegistrationPoint(item.Member); ok {
				x -= reg[0]
				y -= reg[1]
			}
		}
		return image.Rect(x, y, x+asset.Bounds().Dx(), y+asset.Bounds().Dy()), true
	}
	return image.Rectangle{}, false
}

func (s *Scene) handlePublicDJClick(x, y int) bool {
	bounds, ok := s.publicDJBounds()
	if !ok || !image.Pt(x, y).In(bounds) {
		return false
	}
	if s.playerAvatar == nil {
		s.ShowServerAlert("Your character is still entering the studio.")
	} else if s.sendCDRequestIfAvailable != nil {
		if err := s.sendCDRequestIfAvailable(s.studio.StudioID, "public:london:dj"); err != nil {
			s.ShowServerAlert("The CD player could not be reached. Please try again.")
		}
	}
	return true
}

func (s *Scene) ShowServerAlert(message string) {
	if s.hudManager != nil {
		s.hudManager.OpenGeneralAlert("Coke Studios", message)
	}
}
