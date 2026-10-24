package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ErrRoomNotFound = errors.New("room not found")

type Room struct {
	ID             string
	Name           string
	Layout         int
	OwnerName      string
	Wallpaper      int
	Floor          int
	Doorbell       int
	IsRedRoom      bool
	EntranceX      int
	EntranceZ      int
	mu             sync.RWMutex
	Players        map[string]*Player
	Performance    *PerformanceSession
	walkableGrid   map[[2]int]bool
	furnitureSolid map[[2]int]string
}

func NewRoom(id, name string, layout int) *Room {
	isRed := strings.Contains(strings.ToLower(id), "red") || strings.Contains(strings.ToLower(name), "red")
	entX, entZ := 4, 8
	switch layout {
	case 1:
		entX, entZ = 4, 8
	case 2:
		entX, entZ = 4, 10
	case 3:
		entX, entZ = 8, 10
	case 4:
		entX, entZ = 4, 8
	case 5:
		entX, entZ = 5, 12
	}
	return &Room{
		ID:             id,
		Name:           name,
		Layout:         layout,
		Wallpaper:      1,
		Floor:          1,
		Doorbell:       1,
		IsRedRoom:      isRed,
		EntranceX:      entX,
		EntranceZ:      entZ,
		Players:        make(map[string]*Player),
		walkableGrid:   make(map[[2]int]bool),
		furnitureSolid: make(map[[2]int]string),
	}
}

func (r *Room) Broadcast(msg string, excludeID string) {
	r.mu.RLock()
	var targets []*Player
	for id, p := range r.Players {
		if id != excludeID && p != nil {
			targets = append(targets, p)
		}
	}
	r.mu.RUnlock()

	for _, p := range targets {
		_ = p.Send(msg)
	}
}

func (r *Room) SetFurnitureSolid(posID string, x, z int, solid bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := [2]int{x, z}
	if solid {
		r.furnitureSolid[key] = posID
	} else {
		delete(r.furnitureSolid, key)
	}
}

func (r *Room) RemoveFurnitureSolid(posID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range r.furnitureSolid {
		if v == posID {
			delete(r.furnitureSolid, k)
		}
	}
}

func (r *Room) IsWalkable(x, z int) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Entrance tile is always walkable
	if x == r.EntranceX && z == r.EntranceZ {
		return true
	}

	// 2. Check placed solid furniture
	if _, blocked := r.furnitureSolid[[2]int{x, z}]; blocked {
		return false
	}

	// 3. Check base walkable grid
	if len(r.walkableGrid) > 0 {
		return r.walkableGrid[[2]int{x, z}]
	}

	// If no grid loaded, allow default reasonable bounds [0, 30]
	return x >= 0 && x <= 30 && z >= 0 && z <= 30
}

type pathMapNode struct {
	Row    int `xml:"r,attr"`
	Col    int `xml:"c,attr"`
	Weight int `xml:"w,attr"`
}

type pathMapXML struct {
	XMLName xml.Name      `xml:"PathMap"`
	Layout  int           `xml:"layout,attr"`
	Nodes   []pathMapNode `xml:"Node"`
}

var (
	pathMapCacheMu sync.RWMutex
	pathMapCache   = make(map[int]map[[2]int]bool)
)

func (r *Room) LoadPathMap(webRoot string) {
	if r.Layout <= 0 {
		return
	}

	pathMapCacheMu.RLock()
	cached, ok := pathMapCache[r.Layout]
	pathMapCacheMu.RUnlock()

	if ok {
		r.mu.Lock()
		r.walkableGrid = make(map[[2]int]bool, len(cached))
		for k, v := range cached {
			r.walkableGrid[k] = v
		}
		r.mu.Unlock()
		return
	}

	var relPath string
	switch r.Layout {
	case 1:
		relPath = filepath.Join("Lingo", "isoengine", "142_Studio_A_MapXml.txt")
	case 2:
		relPath = filepath.Join("Lingo", "isoengine", "143_Studio_B_MapXml.txt")
	case 3:
		relPath = filepath.Join("Lingo", "isoengine", "144_Studio_C_MapXml.txt")
	case 4:
		relPath = filepath.Join("Lingo", "isoengine", "145_Studio_D_MapXml.txt")
	case 5:
		relPath = filepath.Join("Lingo", "london", "59_MapXml.txt")
	default:
		relPath = filepath.Join("Lingo", "isoengine", "144_Studio_C_MapXml.txt")
	}

	fullPath := filepath.Join(webRoot, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		repoRoot := findRepoRoot()
		data, err = os.ReadFile(filepath.Join(repoRoot, "sources", "gocoke", "web", relPath))
	}

	walkable := make(map[[2]int]bool)
	if err == nil {
		var pm pathMapXML
		if xmlErr := xml.Unmarshal(data, &pm); xmlErr == nil {
			for _, n := range pm.Nodes {
				if n.Weight == 0 || n.Weight == 10 || n.Weight == 150 || n.Weight == 155 || n.Weight == 110 || n.Weight == 200 {
					walkable[[2]int{n.Row, n.Col}] = true
				}
			}
		}
	}

	if len(walkable) == 0 {
		if r.Layout == 1 {
			walkable[[2]int{4, 8}] = true
			for row := 5; row <= 14; row++ {
				for col := 7; col <= 12; col++ {
					walkable[[2]int{row, col}] = true
				}
			}
		} else {
			for row := 4; row <= 14; row++ {
				for col := 4; col <= 14; col++ {
					walkable[[2]int{row, col}] = true
				}
			}
		}
	}

	pathMapCacheMu.Lock()
	pathMapCache[r.Layout] = walkable
	pathMapCacheMu.Unlock()

	r.mu.Lock()
	r.walkableGrid = make(map[[2]int]bool, len(walkable))
	for k, v := range walkable {
		r.walkableGrid[k] = v
	}
	r.mu.Unlock()
}

func (s *Server) GetRoom(roomID string) (*Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return nil, ErrRoomNotFound
	}

	if rm, ok := s.Rooms[roomID]; ok {
		return rm, nil
	}

	switch roomID {
	case "VIS$0":
		rm := NewRoom("VIS$0", "Central Park Studio", 5)
		rm.OwnerName = "Coke Studios"
		rm.LoadPathMap(s.webRoot)
		s.Rooms["VIS$0"] = rm
		return rm, nil
	case "VIS$1":
		rm := NewRoom("VIS$1", "San Francisco Studio", 3)
		rm.OwnerName = "Coke Studios"
		rm.LoadPathMap(s.webRoot)
		s.Rooms["VIS$1"] = rm
		return rm, nil
	case "VIS$2":
		rm := NewRoom("VIS$2", "London Studio", 5)
		rm.OwnerName = "Coke Studios"
		rm.LoadPathMap(s.webRoot)
		s.Rooms["VIS$2"] = rm
		return rm, nil
	}

	if s.store != nil {
		st, err := s.store.GetStudio(roomID)
		if err == nil && st != nil {
			rm := NewRoom(st.ID, st.Name, st.Layout)
			rm.OwnerName = st.OwnerName
			rm.Wallpaper = st.Wallpaper
			rm.Floor = st.Floor
			rm.Doorbell = st.Doorbell
			rm.LoadPathMap(s.webRoot)

			furniList, _ := s.store.GetRoomFurniture(rm.ID)
			for _, pf := range furniList {
				rm.SetFurnitureSolid(pf.PosID, pf.X, pf.Z, true)
			}

			s.Rooms[st.ID] = rm
			return rm, nil
		}
	}

	return nil, ErrRoomNotFound
}

func (s *Server) GetOrCreateRoom(roomID string) *Room {
	rm, err := s.GetRoom(roomID)
	if err != nil {
		log.Printf("[ROOM] GetOrCreateRoom fallback requested for unknown room %q: %v", roomID, err)
		return nil
	}
	return rm
}

func (s *Server) removePlayerFromRoom(player *Player, roomID string) {
	if roomID == "" || roomID == "$LOBBY$" {
		return
	}
	s.mu.RLock()
	rm, ok := s.Rooms[roomID]
	s.mu.RUnlock()
	if ok && rm != nil {
		rm.mu.Lock()
		delete(rm.Players, player.ID)
		rm.mu.Unlock()
		leaveXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV_RM av=\"%s\"/></Room.Action>", rm.ID, player.ScreenName)
		rm.Broadcast(leaveXML, player.ID)
		log.Printf("[ROOM] Player %s removed from room %s", player.ScreenName, rm.ID)
	}
}

func (s *Server) enterPlayerRoom(player *Player, roomID string, msgID string) error {
	if player.RoomID != "" && player.RoomID != roomID {
		s.removePlayerFromRoom(player, player.RoomID)
	}

	if roomID == "$LOBBY$" {
		player.RoomID = "$LOBBY$"
		_ = player.Send(fmt.Sprintf("<Accepted msgID=\"%s\"/>", msgID))
		return nil
	}

	rm, err := s.GetRoom(roomID)
	if err != nil {
		log.Printf("[ROOM] Room %q not found for %s: %v", roomID, player.ScreenName, err)
		_ = player.Send(fmt.Sprintf("<Room.NotFound roomID=\"%s\" msgID=\"%s\"/>", roomID, msgID))
		_ = player.Send("<Alert msg=\"The requested studio was not found.\"/>")
		return err
	}

	player.RoomID = rm.ID
	player.X = rm.EntranceX
	player.Z = rm.EntranceZ
	player.Dir = 2
	player.Action = "std"

	_ = player.Send(fmt.Sprintf("<Accepted msgID=\"%s\"/>", msgID))
	stEnXML := fmt.Sprintf("<ST_EN layout=\"%d\" roomID=\"%s\" name=\"%s\" owner=\"%s\" wp=\"%d\" fl=\"%d\" db=\"%d\"/>",
		rm.Layout, rm.ID, rm.Name, rm.OwnerName, rm.Wallpaper, rm.Floor, rm.Doorbell)
	_ = player.Send(stEnXML)
	decorXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><ST_DECOR wp=\"%d\" fl=\"%d\" db=\"%d\"/></Room.Action>",
		rm.ID, rm.Wallpaper, rm.Floor, rm.Doorbell)
	_ = player.Send(decorXML)

	rm.mu.RLock()
	for _, other := range rm.Players {
		if other.ID != player.ID && other.ScreenName != "" {
			avXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV av=\"%s\" x=\"%d\" z=\"%d\" d=\"%d\" g=\"%d\" m=\"%s\" def=\"%s\" act=\"%s\"/></Room.Action>",
				rm.ID, other.ScreenName, other.X, other.Z, other.Dir, other.Gender, other.Mission, other.Definition, other.Action)
			_ = player.Send(avXML)
		}
	}

	if s.store != nil {
		furniList, _ := s.store.GetRoomFurniture(rm.ID)
		for _, pf := range furniList {
			posXML := fmt.Sprintf("<POS id=\"%s\" cid=\"%d\" x=\"%d\" y=\"%d\" z=\"%d\" ho=\"%d\" d=\"%d\" st=\"%d\" iaot=\"%t\"/>",
				pf.PosID, pf.CatalogItemID, pf.X, pf.Y, pf.Z, pf.HO, pf.Dir, pf.State, pf.IAOT)
			_ = player.Send(fmt.Sprintf("<Room.Action roomID=\"%s\">%s</Room.Action>", rm.ID, posXML))
		}
	}

	if rm.Performance != nil && !rm.Performance.Done {
		elapsed := time.Since(rm.Performance.StartedAt)
		if elapsed < rm.Performance.Duration {
			offsetMS := int(elapsed.Milliseconds())
			durSec := int(rm.Performance.Duration.Seconds())
			cdpXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_P av=\"%s\" s=\"%s\" title=\"%s\" dur=\"%d\" offset=\"%d\"/></Room.Action>",
				rm.ID, rm.Performance.DJ, rm.Performance.EDL, rm.Performance.SongTitle, durSec, offsetMS)
			_ = player.Send(cdpXML)
			meterXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_VR tu=\"%d\" td=\"%d\"/></Room.Action>",
				rm.ID, rm.Performance.Cheers, rm.Performance.Boos)
			_ = player.Send(meterXML)
		}
	}
	rm.mu.RUnlock()

	rm.mu.Lock()
	rm.Players[player.ID] = player
	rm.mu.Unlock()

	log.Printf("[ROOM] Player %s entered %s at (%d, %d)", player.ScreenName, rm.ID, rm.EntranceX, rm.EntranceZ)
	return nil
}

func (s *Server) exitPlayerRoom(player *Player, roomID string, msgID string) {
	if player.RoomID != "" && player.RoomID != "$LOBBY$" {
		s.removePlayerFromRoom(player, player.RoomID)
	}
	player.RoomID = "$LOBBY$"
	_ = player.Send(fmt.Sprintf("<Accepted msgID=\"%s\"/>", msgID))
	log.Printf("[ROOM] Player %s exited to lobby (msgID: %s)", player.ScreenName, msgID)
}

func (s *Server) spawnAvatarInRoom(player *Player, def string) {
	if player.RoomID == "" || player.RoomID == "$LOBBY$" {
		return
	}
	if def != "" {
		player.Definition = def
	}
	rm, err := s.GetRoom(player.RoomID)
	if err != nil {
		return
	}
	avXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV av=\"%s\" x=\"%d\" z=\"%d\" d=\"%d\" g=\"%d\" m=\"%s\" def=\"%s\" act=\"%s\"/></Room.Action>",
		rm.ID, player.ScreenName, player.X, player.Z, player.Dir, player.Gender, player.Mission, player.Definition, player.Action)
	rm.Broadcast(avXML, "")
	log.Printf("[ROOM] Player %s spawned in %s (%d, %d)", player.ScreenName, rm.ID, player.X, player.Z)
}

func (s *Server) movePlayerInRoom(player *Player, pts [][2]int) error {
	if len(pts) == 0 {
		return errors.New("empty path")
	}
	if player.RoomID == "" || player.RoomID == "$LOBBY$" {
		return errors.New("player not in room")
	}
	rm, err := s.GetRoom(player.RoomID)
	if err != nil {
		return err
	}
	dest := pts[len(pts)-1]
	if !rm.IsWalkable(dest[0], dest[1]) {
		log.Printf("[ROOM] Move rejected for %s: destination (%d, %d) blocked or out of bounds", player.ScreenName, dest[0], dest[1])
		return fmt.Errorf("tile (%d, %d) is blocked", dest[0], dest[1])
	}
	player.X = dest[0]
	player.Z = dest[1]
	mvXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV_MV av=\"%s\" x=\"%d\" z=\"%d\"/></Room.Action>",
		rm.ID, player.ScreenName, player.X, player.Z)
	rm.Broadcast(mvXML, "")
	log.Printf("[ROOM] Player %s moved to (%d, %d) in %s", player.ScreenName, player.X, player.Z, rm.ID)
	return nil
}
