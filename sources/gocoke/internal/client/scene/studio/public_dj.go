package studio

import (
	"fmt"
	"image"
	"strconv"
)

// publicDJBounds uses the very same member and registration point as the room
// renderer. Supports authentic DJ booths across all public rooms.
func (s *Scene) publicDJBounds() (image.Rectangle, bool) {
	if s.studioXML == nil || s.studio == nil || s.studio.Layout != 5 {
		return image.Rectangle{}, false
	}
	djMembers := []string{
		"london_djtable_a_0_0_0",
		"reykjavik_turntables",
		"miami_mixer1",
		"miami_mixer2",
		"goa_stage_pink",
		"mosmixa",
		"mom_mixah_1_anim0",
		"dj",
	}
	for _, item := range s.studioXML.StaticItems.Items {
		isDJ := false
		for _, dj := range djMembers {
			if item.Member == dj {
				isDJ = true
				break
			}
		}
		if !isDJ {
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
		reqID := "public:london:dj"
		switch s.studio.StudioID {
		case "$SYSTEM$1", "$SYSTEM$2", "london", "VIS$2":
			reqID = "public:london:dj"
		default:
			reqID = fmt.Sprintf("public:%s:dj", s.studio.StudioID)
		}
		if err := s.sendCDRequestIfAvailable(s.studio.StudioID, reqID); err != nil {
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

// HandlePlacementRejected handles server placement rejection (<POS_REJECT>), rolling back any optimistic placement.
func (s *Scene) HandlePlacementRejected(posID, prodID string) {
	if s == nil || s.placement == nil {
		return
	}
	s.placement.RollbackPlacement(posID, prodID)
}

// HandleCDMinted adds the newly burned CD directly into the player's backpack and updates decibels.
func (s *Scene) HandleCDMinted(posID, cid, title, edl, dbStr string) {
	if s == nil {
		return
	}
	if s.backpackManager != nil {
		props := map[string]interface{}{
			"songName": title,
			"edl":      edl,
		}
		s.backpackManager.InsertItemAtFront(BackpackItem{
			ProdID:     cid,
			PosID:      posID,
			Attributes: props,
		})
	}
	if db, err := strconv.Atoi(dbStr); err == nil && s.hudManager != nil {
		s.hudManager.SetBalance(db)
	}
	s.ShowServerAlert(fmt.Sprintf("Your mix %q has been burned to a CD!", title))
}
