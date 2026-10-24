package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gocoke/internal/client/net/amf"
)

// Standalone Authoritative Coke Studios Server (Debian 12 First)
// Implements:
// 1. Static asset server (WebAssembly client, Lingo assets, sounds, fonts)
// 2. HTTP Flash Remoting AMF0 gateway (/sf/gateway)
// 3. Denizen XML authentication (/sf/ssldocs/loginDenizen.jsp)
// 4. WebSocket realtime multiplayer game engine (/ws/realtime)
// 5. Persistent SQLite state store (denizens, balances, rooms, furniture, songs)
// 6. DJ Performance voting & Decibel meter battle loop (CD_P, CD_TU, CD_TD, CD_VR, CD_PRS)
// 7. Interactive furniture (Coke vending machine drinks, seating, lighting toggles)

type Player struct {
	ID         string
	ScreenName string
	RoomID     string
	X          int
	Z          int
	Dir        int
	Gender     int
	Mission    string
	Definition string
	Action     string
	Conn       *upgradedConn
	mu         sync.Mutex
}

func (p *Player) Send(msg string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Conn == nil {
		return errors.New("closed")
	}
	payload := append([]byte(msg), 0x00)
	return p.Conn.WriteBinary(payload)
}

type PerformanceSession struct {
	DJ        string
	SongTitle string
	EDL       string
	StartedAt time.Time
	Duration  time.Duration
	Cheers    int
	Boos      int
	Voters    map[string]bool
	Done      bool
	cancelCh  chan struct{}
}

type Room struct {
	ID          string
	Name        string
	Layout      int
	mu          sync.RWMutex
	Players     map[string]*Player
	Performance *PerformanceSession
}

func NewRoom(id, name string, layout int) *Room {
	return &Room{
		ID:      id,
		Name:    name,
		Layout:  layout,
		Players: make(map[string]*Player),
	}
}

func (r *Room) Broadcast(msg string, excludeID string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.Players {
		if p.ID != excludeID {
			_ = p.Send(msg)
		}
	}
}

type Server struct {
	mu        sync.RWMutex
	Rooms     map[string]*Room
	Players   map[string]*Player
	webRoot   string
	host      string
	port      string
	store     *Store
	nextPosID int
}

func NewServer(webRoot, host, port string, store *Store) *Server {
	if store == nil {
		var err error
		store, err = OpenStore(":memory:")
		if err != nil {
			log.Fatalf("Failed to open fallback in-memory store: %v", err)
		}
	}

	s := &Server{
		Rooms:     make(map[string]*Room),
		Players:   make(map[string]*Player),
		webRoot:   webRoot,
		host:      host,
		port:      port,
		store:     store,
		nextPosID: 2000,
	}

	// Initialize default rooms in memory from store
	s.Rooms["$LOBBY$"] = NewRoom("$LOBBY$", "Lobby", 0)
	studios, _ := store.GetStudios()
	for _, st := range studios {
		s.Rooms[st.ID] = NewRoom(st.ID, st.Name, st.Layout)
	}

	return s
}

func (s *Server) GetOrCreateRoom(roomID string) *Room {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.Rooms[roomID]
	if !ok {
		name := "Studio " + roomID
		r = NewRoom(roomID, name, 3)
		s.Rooms[roomID] = r
	}
	return r
}

func main() {
	_ = mime.AddExtensionType(".wasm", "application/wasm")
	_ = mime.AddExtensionType(".br", "application/x-brotli")

	addr := flag.String("addr", "0.0.0.0:8080", "listen address (default 0.0.0.0:8080)")
	root := flag.String("root", "./web", "web directory to serve")
	defaultDB := os.Getenv("DATABASE_URL")
	if defaultDB == "" {
		defaultDB = os.Getenv("MARIADB_URL")
	}
	if defaultDB == "" {
		defaultDB = "coke.db"
	}
	dbPath := flag.String("db", defaultDB, "Database DSN (MariaDB 'user:pass@tcp(host:port)/db' or SQLite 'coke.db')")
	flag.Parse()

	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		host = "127.0.0.1"
		port = "8080"
	}
	if host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}

	store, err := OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database (%s): %v", *dbPath, err)
	}
	defer store.Close()

	srv := NewServer(*root, host, port, store)

	mux := http.NewServeMux()

	// 1. Denizen login JSP
	mux.HandleFunc("/sf/ssldocs/loginDenizen.jsp", srv.handleLoginDenizen)

	// 2. Flash Remoting AMF0 gateway
	mux.HandleFunc("/sf/gateway", srv.handleAMFGateway)

	// 3. WebSocket Realtime Game Server
	mux.HandleFunc("/ws/realtime", srv.handleWebSocketRealtime)

	// 4. Red Cap PIN code redemption HTTP API
	mux.HandleFunc("/sf/ssldocs/redeemCap.jsp", srv.handleRedeemCap)

	// 5. Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","mode":"standalone-server","project":"gocoke","storage":"%s"}`, srv.store.driver)
	})

	// 5. Static files
	fs := http.FileServer(http.Dir(*root))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fs.ServeHTTP(w, r)
	})

	log.Printf("==========================================================")
	log.Printf(" Coke Studios Authoritative Server (Debian / Linux First)")
	log.Printf(" Listening on http://%s (Serving %s)", *addr, *root)
	log.Printf(" SQLite Store:              %s", *dbPath)
	log.Printf(" WebSocket Realtime Bridge: ws://%s/ws/realtime", *addr)
	log.Printf(" AMF0 Flash Gateway:        http://%s/sf/gateway", *addr)
	log.Printf("==========================================================")

	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func (s *Server) handleLoginDenizen(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	screenName := q.Get("user")
	if screenName == "" {
		screenName = q.Get("screenName")
	}
	if screenName == "" {
		if enc := q.Get("sw1"); enc != "" {
			if dec, err := base64.StdEncoding.DecodeString(enc); err == nil && len(dec) > 0 {
				screenName = string(dec)
			}
		}
	}
	if screenName == "" {
		sid := q.Get("sidempty")
		if sid == "" {
			sid = q.Get("sid")
		}
		if sid != "" {
			parts := strings.Split(sid, "_")
			if len(parts) > 0 && parts[0] != "" {
				screenName = parts[0]
			}
		}
	}
	if screenName == "" {
		screenName = "Adam"
	}

	gender := q.Get("gender")
	if gender == "" {
		gender = "M"
	}
	avatarDef := q.Get("avatarDef")
	if avatarDef == "" {
		avatarDef = "ch=01/01/01&fa=01/01/01&cl=01/01/01"
	}
	mission := q.Get("mission")
	if mission == "" {
		mission = "Coke Music Reborn"
	}

	// Persistent store retrieval/creation
	denizen, err := s.store.GetOrCreateDenizen(screenName, gender, avatarDef, mission)
	if err != nil {
		log.Printf("[DB] LoginDenizen error: %v, using transient credentials", err)
		denizen = &DenizenRecord{
			ScreenName: screenName,
			Gender:     1,
			Mission:    mission,
			AvatarDef:  avatarDef,
			Secret:     "coke_secret_" + strconv.FormatInt(time.Now().Unix(), 16),
			LastSeenIn: "VIS$0",
		}
	}

	xmlDef := strings.ReplaceAll(denizen.AvatarDef, "&", "&amp;")
	gStr := "M"
	if denizen.Gender == 2 {
		gStr = "F"
	}

	xmlReply := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<LoginResult>
  <ErrorCode>0</ErrorCode>
  <ErrorMessage></ErrorMessage>
  <Gender>%s</Gender>
  <AvatarDefinition>%s</AvatarDefinition>
  <ScreenName>%s</ScreenName>
  <MissionStatement>%s</MissionStatement>
  <LastSeenIn>%s</LastSeenIn>
  <Banned>0</Banned>
  <Secret>%s</Secret>
</LoginResult>`, gStr, xmlDef, denizen.ScreenName, denizen.Mission, denizen.LastSeenIn, denizen.Secret)

	w.Header().Set("Content-Type", "text/xml;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(xmlReply))
	log.Printf("[HTTP] LoginDenizen authenticated: %s (Gender: %s, dB: %d)", denizen.ScreenName, gStr, denizen.Decibels)
}

func (s *Server) handleRedeemCap(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	user := q.Get("user")
	if user == "" {
		user = "Adam"
	}
	pin := q.Get("pin")
	if pin == "" && r.Method == http.MethodPost {
		_ = r.ParseForm()
		pin = r.FormValue("pin")
		if u := r.FormValue("user"); u != "" {
			user = u
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if pin == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success":false,"error":"Missing PIN code"}`))
		return
	}

	rewardVC, rewardItem, err := s.store.RedeemRedCap(user, pin)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"success":false,"error":%q}`, err.Error())
		return
	}

	_, newVC, _ := s.store.GetBalance(user)
	fmt.Fprintf(w, `{"success":true,"reward_vcoins":%d,"reward_item":%d,"new_vcoins":%d,"user":%q}`,
		rewardVC, rewardItem, newVC, user)
}

func (s *Server) handleAMFGateway(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) < 8 {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	target, respURI, err := amf.ParseRemotingTarget(body)
	if err != nil {
		log.Printf("[AMF] Parse target error: %v", err)
		target = "unknown"
	}

	responseTarget := respURI + "/onResult"
	var responseValue any = nil

	log.Printf("[AMF] RPC Target: %s (respURI: %s)", target, respURI)

	switch {
	case strings.Contains(target, "getTip"):
		tips := []string{
			"Welcome to Coke Studios! Click any tile to walk, open the catalogue to decorate, or open the mixer to create beats.",
			"Play your burned CDs in the studio stereo to host DJ battles and earn Decibels from audience cheers!",
			"Approach the Coke vending machine to grab an ice-cold can of Coca-Cola.",
			"Cheer for great tracks using thumbs-up to boost the room's Decibel meter!",
		}
		responseValue = tips[time.Now().Unix()%int64(len(tips))]

	case strings.Contains(target, "getDenizenBalance"):
		// Extract screenName if available, or return default balance
		decibels, _, _ := s.store.GetBalance("Adam")
		responseValue = float64(decibels)

	case strings.Contains(target, "getBackpack"):
		// Load from persistent SQLite database
		items, err := s.store.GetBackpack("Adam")
		if err != nil || len(items) == 0 {
			// Fallback starter backpack if empty
			responseValue = map[string]any{
				"possessionlist": []any{
					map[string]any{
						"id":          float64(1001),
						"catalogItem": float64(45), // Classic Sofa
						"properties":  map[string]any{},
					},
					map[string]any{
						"id":          float64(1002),
						"catalogItem": float64(52), // Coke Vending Machine
						"properties":  map[string]any{},
					},
					map[string]any{
						"id":          float64(1003),
						"catalogItem": float64(89), // Burned CD
						"properties": map[string]any{
							"ownerName": "DJ Adam",
							"songName":  "Coke Beats Vol. 1",
							"edl":       "^50,countryOne_guitar00{2,0{4,4{^50,danceOne_bass00{4,0{^",
						},
					},
				},
			}
		} else {
			var list []any
			for _, it := range items {
				list = append(list, map[string]any{
					"id":          float64(it.ID),
					"catalogItem": float64(it.CatalogItemID),
					"properties":  it.Properties,
				})
			}
			responseValue = map[string]any{"possessionlist": list}
		}

	case strings.Contains(target, "prePurchaseItem"):
		db, _, _ := s.store.GetBalance("Adam")
		responseValue = map[string]any{
			"balance":       float64(db),
			"price":         float64(25),
			"catalogItemId": float64(1),
			"properties":    "[:]",
		}

	case strings.Contains(target, "purchaseItem"):
		// Deduct 25 dB and insert into persistent backpack
		_, _ = s.store.AdjustBalance("Adam", -25, 0)
		newID, _ := s.store.AddBackpackItem("Adam", 45, map[string]any{})
		responseValue = []any{
			map[string]any{
				"id":          float64(newID),
				"catalogItem": float64(45),
				"properties":  map[string]any{},
			},
		}

	case strings.Contains(target, "burnCD"):
		// Deduct 10 dB and mint a new playable CD (catalogItem 89) into backpack
		_, _ = s.store.AdjustBalance("Adam", -10, 0)
		cdProps := map[string]any{
			"songName":  "Coke Anthem Reborn",
			"ownerName": "DJ Adam",
			"edl":       "^50,rockOne_guitar00{2,0{4,4{^50,danceOne_beatz00{4,0{^",
		}
		newID, _ := s.store.AddBackpackItem("Adam", 89, cdProps)
		responseValue = map[string]any{
			"success":     true,
			"possession":  float64(newID),
			"catalogItem": float64(89),
			"properties":  cdProps,
		}

	case strings.Contains(target, "getAllPrivateStudios") || strings.Contains(target, "getMyStudios") || strings.Contains(target, "getFeaturedStudios"):
		studios, err := s.store.GetStudios()
		var resList []any
		if err == nil && len(studios) > 0 {
			for _, st := range studios {
				userCount := float64(0)
				if rm := s.GetOrCreateRoom(st.ID); rm != nil {
					userCount = float64(len(rm.Players))
				}
				resList = append(resList, map[string]any{
					"studioId":  st.ID,
					"name":      st.Name,
					"layout":    float64(st.Layout),
					"owner":     st.OwnerName,
					"userCount": userCount,
					"maxUsers":  float64(25),
					"type":      st.RoomType,
				})
			}
		}
		responseValue = resList

	case strings.Contains(target, "getGameServerByStudioID"):
		responseValue = map[string]any{
			"server": s.host,
			"port":   s.port,
			"dns":    s.host,
		}

	default:
		log.Printf("[AMF] Default handler for: %s", target)
		responseValue = true
	}

	respBytes, err := amf.EncodeRemotingResponse(responseTarget, responseValue)
	if err != nil {
		log.Printf("[AMF] Encode response error: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-amf")
	w.WriteHeader(http.StatusOK)
	w.Write(respBytes)
}

func (s *Server) handleWebSocketRealtime(w http.ResponseWriter, r *http.Request) {
	conn, err := acceptRFC6455(w, r)
	if err != nil {
		log.Printf("[WS] Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	playerID := fmt.Sprintf("p_%d", time.Now().UnixNano())
	player := &Player{
		ID:         playerID,
		ScreenName: "Adam",
		Gender:     1,
		Mission:    "Coke Music",
		Definition: "ch=01/01/01&fa=01/01/01&cl=01/01/01",
		Action:     "std",
		Dir:        2,
		X:          0,
		Z:          0,
		Conn:       conn,
	}

	s.mu.Lock()
	s.Players[playerID] = player
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.Players, playerID)
		s.mu.Unlock()

		if player.RoomID != "" {
			if rm, ok := s.Rooms[player.RoomID]; ok {
				rm.mu.Lock()
				delete(rm.Players, playerID)
				rm.mu.Unlock()
				leaveXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV_RM av=\"%s\"/></Room.Action>", player.RoomID, player.ScreenName)
				rm.Broadcast(leaveXML, playerID)
			}
		}
		log.Printf("[WS] Player %s disconnected", player.ScreenName)
	}()

	// 1. Immediately send cross-domain policy
	policyXML := "<cross-domain-policy><allow-access-from domain=\"*\" to-ports=\"*\"/></cross-domain-policy>"
	_ = player.Send(policyXML)
	log.Printf("[WS] Sent cross-domain policy to player %s", playerID)

	// 2. Read incoming client packets
	for {
		raw, err := conn.ReadBinary()
		if err != nil {
			break
		}
		if len(raw) == 0 {
			continue
		}

		parts := strings.SplitN(string(raw), " ", 3)
		var xmlPayload string
		if len(parts) == 3 {
			xmlPayload = strings.TrimRight(parts[2], "\x00")
		} else {
			xmlPayload = strings.TrimRight(string(raw), "\x00")
		}

		s.handlePlayerMessage(player, xmlPayload)
	}
}

func (s *Server) handlePlayerMessage(player *Player, xml string) {
	if strings.Contains(xml, "<Login") {
		msgID := extractAttr(xml, "msgID")
		sn := extractAttr(xml, "username")
		if sn != "" {
			player.ScreenName = sn
			if d, err := s.store.GetDenizenByName(sn); err == nil && d != nil {
				player.Gender = d.Gender
				player.Mission = d.Mission
				player.Definition = d.AvatarDef
			}
		}
		log.Printf("[WS] Player Login: %s (msgID: %s)", player.ScreenName, msgID)
		_ = player.Send(fmt.Sprintf("<Accepted msgID=\"%s\"/>", msgID))
		return
	}

	if strings.Contains(xml, "<Room.Enter") {
		msgID := extractAttr(xml, "msgID")
		roomID := extractAttr(xml, "roomID")
		log.Printf("[WS] Player %s entering room %s (msgID: %s)", player.ScreenName, roomID, msgID)

		_ = player.Send(fmt.Sprintf("<Accepted msgID=\"%s\"/>", msgID))

		if roomID == "$LOBBY$" {
			player.RoomID = "$LOBBY$"
			return
		}

		room := s.GetOrCreateRoom(roomID)
		player.RoomID = roomID
		player.X = 0
		player.Z = 0

		// Send ST_EN room layout
		stEnXML := fmt.Sprintf("<ST_EN layout=\"%d\" roomID=\"%s\" name=\"%s\" owner=\"Coke Studios\"/>", room.Layout, room.ID, room.Name)
		_ = player.Send(stEnXML)

		// Send existing occupants to this player
		room.mu.RLock()
		for _, other := range room.Players {
			if other.ID != player.ID {
				avXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV av=\"%s\" x=\"%d\" z=\"%d\" d=\"%d\" g=\"%d\" m=\"%s\" def=\"%s\" act=\"%s\"/></Room.Action>",
					room.ID, other.ScreenName, other.X, other.Z, other.Dir, other.Gender, other.Mission, other.Definition, other.Action)
				_ = player.Send(avXML)
			}
		}

		// Send persistent placed furniture from SQLite
		furniList, _ := s.store.GetRoomFurniture(room.ID)
		for _, pf := range furniList {
			posXML := fmt.Sprintf("<POS id=\"%s\" cid=\"%d\" x=\"%d\" y=\"%d\" z=\"%d\" ho=\"%d\" d=\"%d\" st=\"%d\" iaot=\"%t\"/>",
				pf.PosID, pf.CatalogItemID, pf.X, pf.Y, pf.Z, pf.HO, pf.Dir, pf.State, pf.IAOT)
			_ = player.Send(fmt.Sprintf("<Room.Action roomID=\"%s\">%s</Room.Action>", room.ID, posXML))
		}

		// If a performance is currently active, sync track playback and vote meter to entering player!
		if room.Performance != nil && !room.Performance.Done {
			cdpXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_P av=\"%s\" s=\"%s\" title=\"%s\"/></Room.Action>",
				room.ID, room.Performance.DJ, room.Performance.EDL, room.Performance.SongTitle)
			_ = player.Send(cdpXML)
			meterXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_VR tu=\"%d\" td=\"%d\"/></Room.Action>",
				room.ID, room.Performance.Cheers, room.Performance.Boos)
			_ = player.Send(meterXML)
		}
		room.mu.RUnlock()

		room.mu.Lock()
		room.Players[player.ID] = player
		room.mu.Unlock()
		return
	}

	if strings.Contains(xml, "<AV_EN") {
		roomID := player.RoomID
		if roomID == "" || roomID == "$LOBBY$" {
			return
		}
		room := s.GetOrCreateRoom(roomID)
		avXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV av=\"%s\" x=\"%d\" z=\"%d\" d=\"%d\" g=\"%d\" m=\"%s\" def=\"%s\" act=\"%s\"/></Room.Action>",
			room.ID, player.ScreenName, player.X, player.Z, player.Dir, player.Gender, player.Mission, player.Definition, player.Action)
		room.Broadcast(avXML, "")
		log.Printf("[WS] Player %s spawned in room %s", player.ScreenName, roomID)
		return
	}

	if strings.Contains(xml, "<AV_GSP") {
		roomID := player.RoomID
		if roomID != "" {
			room := s.GetOrCreateRoom(roomID)
			pts := parsePPoints(xml)
			if len(pts) > 0 {
				dest := pts[len(pts)-1]
				player.X = dest[0]
				player.Z = dest[1]
				mvXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV_MV av=\"%s\" x=\"%d\" z=\"%d\"/></Room.Action>",
					room.ID, player.ScreenName, player.X, player.Z)
				room.Broadcast(mvXML, "")
				log.Printf("[WS] Player %s moved to (%d, %d) in %s", player.ScreenName, player.X, player.Z, roomID)
			}
		}
		return
	}

	if strings.Contains(xml, "<AV_MV") {
		xStr := extractAttr(xml, "x")
		zStr := extractAttr(xml, "z")
		x, _ := strconv.Atoi(xStr)
		z, _ := strconv.Atoi(zStr)
		player.X = x
		player.Z = z

		roomID := player.RoomID
		if roomID != "" {
			room := s.GetOrCreateRoom(roomID)
			mvXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV_MV av=\"%s\" x=\"%d\" z=\"%d\"/></Room.Action>",
				room.ID, player.ScreenName, player.X, player.Z)
			room.Broadcast(mvXML, "")
		}
		return
	}

	if strings.Contains(xml, "<CH ") {
		sm := extractAttr(xml, "sm")
		if sm == "" || sm == "speak" {
			sm = "1"
		} else if sm == "shout" {
			sm = "2"
		}
		mHex := extractAttr(xml, "m")
		roomID := player.RoomID
		if roomID != "" && mHex != "" {
			room := s.GetOrCreateRoom(roomID)
			chatXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CH av=\"%s\" sm=\"%s\" m=\"%s\"/></Room.Action>",
				room.ID, player.ScreenName, sm, mHex)
			room.Broadcast(chatXML, "")

			decoded, _ := hex.DecodeString(mHex)
			log.Printf("[CHAT] [%s] %s: %s", roomID, player.ScreenName, string(decoded))
		}
		return
	}

	// --- DJ Performance & Cheer/Boo Voting System ---
	if strings.Contains(xml, "<CD_P") {
		// Player triggered CD playback or DJ booth performance
		sEDL := extractAttr(xml, "s")
		title := extractAttr(xml, "title")
		if title == "" {
			title = "Coke Beats Vol. 1"
		}
		if sEDL == "" {
			sEDL = "^50,rockOne_guitar00{2,0{4,4{^50,danceOne_beatz00{4,0{^"
		}

		roomID := player.RoomID
		if roomID != "" {
			room := s.GetOrCreateRoom(roomID)
			s.startPerformance(room, player.ScreenName, title, sEDL)
		}
		return
	}

	if strings.Contains(xml, "<CD_TU") {
		// Thumbs Up / Cheer vote
		roomID := player.RoomID
		if roomID != "" {
			room := s.GetOrCreateRoom(roomID)
			s.recordVote(room, player.ScreenName, 1)
		}
		return
	}

	if strings.Contains(xml, "<CD_TD") {
		// Thumbs Down / Boo vote
		roomID := player.RoomID
		if roomID != "" {
			room := s.GetOrCreateRoom(roomID)
			s.recordVote(room, player.ScreenName, -1)
		}
		return
	}

	// --- Interactive Furniture & Vending Machine Drinks ---
	if strings.Contains(xml, "<DRK_REQ") || strings.Contains(xml, "coke_machine") {
		roomID := player.RoomID
		if roomID != "" {
			room := s.GetOrCreateRoom(roomID)
			drinkXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV_DC av=\"%s\" act=\"drk\" item=\"coke_can\"/></Room.Action>",
				room.ID, player.ScreenName)
			room.Broadcast(drinkXML, "")
			log.Printf("[WS] Player %s received a Coke drink from dispenser in %s", player.ScreenName, roomID)
		}
		return
	}

	// --- Avatar Gestures & Actions ---
	if strings.Contains(xml, "<AV_ACT") {
		act := extractAttr(xml, "act")
		roomID := player.RoomID
		if roomID != "" && act != "" {
			player.Action = act
			room := s.GetOrCreateRoom(roomID)
			actXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV_DC av=\"%s\" act=\"%s\"/></Room.Action>",
				room.ID, player.ScreenName, act)
			room.Broadcast(actXML, "")
		}
		return
	}

	// --- Furniture Placement, Rotation, and Pickup (Persistent in SQLite) ---
	if strings.Contains(xml, "<POS_STD") {
		posID := extractAttr(xml, "id")
		cidStr := extractAttr(xml, "cid")
		cid, _ := strconv.Atoi(cidStr)
		if cid == 0 {
			cid = 45 // fallback to sofa
		}
		stStr := extractAttr(xml, "st")
		st, _ := strconv.Atoi(stStr)
		dStr := extractAttr(xml, "d")
		d, _ := strconv.Atoi(dStr)
		hoStr := extractAttr(xml, "ho")
		ho, _ := strconv.Atoi(hoStr)
		xStr := extractAttr(xml, "x")
		x, _ := strconv.Atoi(xStr)
		yStr := extractAttr(xml, "y")
		y, _ := strconv.Atoi(yStr)
		zStr := extractAttr(xml, "z")
		z, _ := strconv.Atoi(zStr)

		roomID := player.RoomID
		if roomID != "" {
			room := s.GetOrCreateRoom(roomID)
			_ = s.store.PlaceFurniture(room.ID, posID, cid, x, y, z, ho, d, st, false, nil)

			posXML := fmt.Sprintf("<POS id=\"%s\" cid=\"%d\" x=\"%d\" y=\"%d\" z=\"%d\" ho=\"%d\" d=\"%d\" st=\"%d\" iaot=\"false\"/>",
				posID, cid, x, y, z, ho, d, st)
			actionXML := fmt.Sprintf("<Room.Action roomID=\"%s\">%s</Room.Action>", room.ID, posXML)
			room.Broadcast(actionXML, "")
			log.Printf("[WS] Player %s placed furniture %s (cid: %d) at (%d,%d,%d) in %s", player.ScreenName, posID, cid, x, y, z, roomID)
		}
		return
	}

	if strings.Contains(xml, "<POS_BKPK") {
		posID := extractAttr(xml, "id")
		roomID := player.RoomID
		if roomID != "" && posID != "" {
			room := s.GetOrCreateRoom(roomID)
			_ = s.store.RemoveFurniture(room.ID, posID)

			rmXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><POS_RM id=\"%s\"/></Room.Action>", room.ID, posID)
			room.Broadcast(rmXML, "")
			log.Printf("[WS] Player %s picked up furniture %s from %s", player.ScreenName, posID, roomID)
		}
		return
	}
}

// startPerformance manages the 60-second DJ voting battle loop
func (s *Server) startPerformance(room *Room, dj, title, edl string) {
	room.mu.Lock()
	if room.Performance != nil && !room.Performance.Done {
		close(room.Performance.cancelCh)
	}

	session := &PerformanceSession{
		DJ:        dj,
		SongTitle: title,
		EDL:       edl,
		StartedAt: time.Now(),
		Duration:  60 * time.Second,
		Cheers:    0,
		Boos:      0,
		Voters:    make(map[string]bool),
		cancelCh:  make(chan struct{}),
	}
	room.Performance = session
	room.mu.Unlock()

	// Broadcast song start and DJ animation to all room occupants
	cdpXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_P av=\"%s\" s=\"%s\" title=\"%s\"/></Room.Action>",
		room.ID, dj, edl, title)
	room.Broadcast(cdpXML, "")
	log.Printf("[MUSIC] DJ %s started track %q in %s", dj, title, room.ID)

	// Background ticker for 60-second song duration
	go func() {
		select {
		case <-time.After(session.Duration):
		case <-session.cancelCh:
			return
		}

		room.mu.Lock()
		if session.Done {
			room.mu.Unlock()
			return
		}
		session.Done = true
		cheers := session.Cheers
		boos := session.Boos
		room.mu.Unlock()

		// Calculate 1:1 reward formula: max(0, (Cheers * 2) - Boos) + 10 audience bonus
		rewardDB := (cheers * 2) - boos
		if rewardDB < 0 {
			rewardDB = 0
		}
		rewardDB += 10 // Participation bonus

		// Credit Decibels to DJ in persistent SQLite database
		newBalance, _ := s.store.AdjustBalance(dj, rewardDB, 0)
		log.Printf("[MUSIC] Performance ended for %s in %s. Cheers: %d, Boos: %d, Awarded: +%d dB (New Balance: %d)",
			dj, room.ID, cheers, boos, rewardDB, newBalance)

		// Broadcast official performance results dialog packet (CD_PRS)
		prsXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_PRS av=\"%s\" tu=\"%d\" td=\"%d\" db=\"%d\"/></Room.Action>",
			room.ID, dj, cheers, boos, rewardDB)
		room.Broadcast(prsXML, "")
	}()
}

func (s *Server) stopPerformance(room *Room) {
	room.mu.Lock()
	defer room.mu.Unlock()
	if room.Performance != nil && !room.Performance.Done {
		room.Performance.Done = true
		select {
		case <-room.Performance.cancelCh:
		default:
			close(room.Performance.cancelCh)
		}
	}
}

func (s *Server) recordVote(room *Room, voter string, delta int) {
	room.mu.Lock()
	defer room.mu.Unlock()

	if room.Performance == nil || room.Performance.Done {
		return
	}
	if room.Performance.Voters[voter] {
		// Already voted in this performance round
		return
	}
	room.Performance.Voters[voter] = true

	act := "cheer"
	if delta > 0 {
		room.Performance.Cheers++
	} else {
		room.Performance.Boos++
		act = "boo"
	}

	// 1. Broadcast real-time Decibel meter update (CD_VR)
	vrXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_VR tu=\"%d\" td=\"%d\"/></Room.Action>",
		room.ID, room.Performance.Cheers, room.Performance.Boos)
	room.Broadcast(vrXML, "")

	// 2. Broadcast voter avatar animation (cheer or boo)
	avXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV_DC av=\"%s\" act=\"%s\"/></Room.Action>",
		room.ID, voter, act)
	room.Broadcast(avXML, "")

	log.Printf("[VOTE] [%s] %s voted %s (Cheers: %d, Boos: %d)",
		room.ID, voter, act, room.Performance.Cheers, room.Performance.Boos)
}

func parsePPoints(xml string) [][2]int {
	var pts [][2]int
	data := xml
	for {
		idx := strings.Index(data, "<P ")
		if idx == -1 {
			break
		}
		end := strings.Index(data[idx:], "/>")
		if end == -1 {
			end = strings.Index(data[idx:], ">")
			if end == -1 {
				break
			}
		}
		seg := data[idx : idx+end+1]
		xStr := extractAttr(seg, "x")
		zStr := extractAttr(seg, "z")
		x, _ := strconv.Atoi(xStr)
		z, _ := strconv.Atoi(zStr)
		pts = append(pts, [2]int{x, z})
		data = data[idx+end+1:]
	}
	return pts
}

func extractAttr(xml, attr string) string {
	pattern := attr + "=\""
	i := strings.Index(xml, pattern)
	if i < 0 {
		return ""
	}
	i += len(pattern)
	j := strings.Index(xml[i:], "\"")
	if j < 0 {
		return ""
	}
	return xml[i : i+j]
}

// Minimal RFC6455 upgrader
type upgradedConn struct {
	netConn net.Conn
	rw      *bufio.ReadWriter
}

func (c *upgradedConn) Close() error { return c.netConn.Close() }

func (c *upgradedConn) WriteBinary(p []byte) error {
	hdr := []byte{0x82}
	l := len(p)
	var lenField []byte
	switch {
	case l < 126:
		lenField = []byte{byte(l)}
	case l <= 0xFFFF:
		lenField = []byte{126, byte(l >> 8), byte(l)}
	default:
		lenField = []byte{127, 0, 0, 0, 0, byte(l >> 24), byte(l >> 16), byte(l >> 8), byte(l)}
	}
	if _, err := c.rw.Write(hdr); err != nil {
		return err
	}
	if _, err := c.rw.Write(lenField); err != nil {
		return err
	}
	if _, err := c.rw.Write(p); err != nil {
		return err
	}
	return c.rw.Flush()
}

func (c *upgradedConn) ReadBinary() ([]byte, error) {
	b1, err := c.rw.ReadByte()
	if err != nil {
		return nil, err
	}
	b2, err := c.rw.ReadByte()
	if err != nil {
		return nil, err
	}
	opcode := b1 & 0x0F
	masked := (b2 & 0x80) != 0
	if opcode == 0x8 {
		return nil, io.EOF
	}
	payloadLen := int(b2 & 0x7F)
	switch payloadLen {
	case 126:
		b, _ := c.rw.Peek(2)
		_, _ = c.rw.Discard(2)
		payloadLen = int(b[0])<<8 | int(b[1])
	case 127:
		b, _ := c.rw.Peek(8)
		_, _ = c.rw.Discard(8)
		payloadLen = int(b[4])<<24 | int(b[5])<<16 | int(b[6])<<8 | int(b[7])
	}
	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.rw, maskKey[:]); err != nil {
			return nil, err
		}
	}
	buf := make([]byte, payloadLen)
	if _, err := io.ReadFull(c.rw, buf); err != nil {
		return nil, err
	}
	if masked {
		for i := 0; i < payloadLen; i++ {
			buf[i] ^= maskKey[i%4]
		}
	}
	return buf, nil
}

func acceptRFC6455(w http.ResponseWriter, r *http.Request) (*upgradedConn, error) {
	h, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("hijack not supported")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("missing Sec-WebSocket-Key")
	}
	hash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	accept := base64.StdEncoding.EncodeToString(hash[:])
	conn, rw, err := h.Hijack()
	if err != nil {
		return nil, err
	}
	rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	rw.WriteString("Upgrade: websocket\r\n")
	rw.WriteString("Connection: Upgrade\r\n")
	rw.WriteString("Sec-WebSocket-Accept: " + accept + "\r\n\r\n")
	if err := rw.Flush(); err != nil {
		conn.Close()
		return nil, err
	}
	return &upgradedConn{netConn: conn, rw: rw}, nil
}
