package main

import (
	"fmt"
	"strconv"
	"strings"
)

// requestCDPlayer implements the request/accept exchange used by the Go UI.
func (s *Server) requestCDPlayer(player *Player, posID string) error {
	room, err := s.GetRoom(player.RoomID)
	if err != nil {
		return err
	}
	valid := posID == "public:london:dj" && (room.ID == "$SYSTEM$1" || room.ID == "$SYSTEM$2" || room.ID == "london" || room.ID == "VIS$2")
	if !valid {
		furniture, err := s.store.GetRoomFurniture(room.ID)
		if err != nil {
			return err
		}
		for _, item := range furniture {
			if item.PosID != posID {
				continue
			}
			catalog, err := s.GetCatalogItem(item.CatalogItemID)
			valid = err == nil && catalog != nil && strings.EqualFold(catalog.Action, "STEREO")
			break
		}
	}
	if !valid {
		return fmt.Errorf("select a CD player in this studio")
	}
	room.mu.RLock()
	busy := room.Performance != nil && !room.Performance.Done
	room.mu.RUnlock()
	if busy {
		return fmt.Errorf("a song is already playing")
	}
	return player.Send(fmt.Sprintf(`<Room.Action roomID="%s"><CD_RA/></Room.Action>`, room.ID))
}

// playOwnedCD resolves title, author and sequence from committed ownership.
// The browser supplies only an owned CD instance ID, never an arbitrary EDL.
func (s *Server) playOwnedCD(player *Player, cdID string) error {
	id, err := strconv.Atoi(cdID)
	if err != nil || id < 1 {
		return fmt.Errorf("select a CD from your backpack")
	}
	cd, err := s.store.GetBackpackItem(player.ScreenName, id)
	if err != nil || cd.CatalogItemID != 89 {
		return fmt.Errorf("CD is not in your backpack")
	}
	songID, err := strconv.Atoi(fmt.Sprint(cd.Properties["songId"]))
	if err != nil {
		return fmt.Errorf("CD has no saved composition")
	}
	song, err := s.store.GetSong(songID)
	if err != nil {
		return fmt.Errorf("saved composition unavailable")
	}
	room, err := s.GetRoom(player.RoomID)
	if err != nil {
		return err
	}
	_, err = s.StartPerformance(room, player.ScreenName, song.Artist, song.Title, song.EDLString)
	return err
}
