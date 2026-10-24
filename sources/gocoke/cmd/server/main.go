package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	Wallpaper   int
	Floor       int
	Doorbell    int
	mu          sync.RWMutex
	Players     map[string]*Player
	Performance *PerformanceSession
}

func NewRoom(id, name string, layout int) *Room {
	return &Room{
		ID:        id,
		Name:      name,
		Layout:    layout,
		Wallpaper: 1,
		Floor:     1,
		Doorbell:  1,
		Players:   make(map[string]*Player),
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
		r := NewRoom(st.ID, st.Name, st.Layout)
		r.Wallpaper = st.Wallpaper
		r.Floor = st.Floor
		r.Doorbell = st.Doorbell
		s.Rooms[st.ID] = r
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
	_ = mime.AddExtensionType(".ls", "text/plain; charset=utf-8")
	_ = mime.AddExtensionType(".csv", "text/csv; charset=utf-8")
	_ = mime.AddExtensionType(".txt", "text/plain; charset=utf-8")
	_ = mime.AddExtensionType(".htm", "text/html; charset=utf-8")

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

	// 0. Denizen Registration & Legacy Login APIs
	mux.HandleFunc("/api/denizen/register", srv.handleRegisterDenizen)
	mux.HandleFunc("/sf/ssldocs/registerDenizen.jsp", srv.handleRegisterDenizen)
	mux.HandleFunc("/login/Login", srv.handleLegacyLogin)
	mux.HandleFunc("/login/LoginPerson", srv.handleLegacyLogin)

	// 1. Denizen login JSP
	mux.HandleFunc("/sf/ssldocs/loginDenizen.jsp", srv.handleLoginDenizen)

	// 2. Flash Remoting AMF0 gateway
	mux.HandleFunc("/sf/gateway", srv.handleAMFGateway)

	// 3. WebSocket Realtime Game Server
	mux.HandleFunc("/ws/realtime", srv.handleWebSocketRealtime)

	// 4. Red Cap PIN code redemption HTTP API
	mux.HandleFunc("/sf/ssldocs/redeemCap.jsp", srv.handleRedeemCap)

	// 5. Denizen Profile & Customizer APIs
	mux.HandleFunc("/api/denizen/profile", srv.handleDenizenProfile)
	mux.HandleFunc("/api/denizen/update", srv.handleUpdateDenizen)
	mux.HandleFunc("/sf/ssldocs/updateDenizen.jsp", srv.handleUpdateDenizen)

	// 6. Messenger & Social APIs
	mux.HandleFunc("/api/messenger/friends", srv.handleMessengerFriends)
	mux.HandleFunc("/api/messenger/send", srv.handleMessengerSend)
	mux.HandleFunc("/api/messenger/messages", srv.handleMessengerMessages)
	mux.HandleFunc("/api/messenger/friend/add", srv.handleMessengerAddFriend)
	mux.HandleFunc("/api/messenger/friend/remove", srv.handleMessengerRemoveFriend)

	// 7. Studio Decor API
	mux.HandleFunc("/api/studio/decor", srv.handleStudioDecor)

	// 8. In-Game CD Burning API
	mux.HandleFunc("/api/mixer/burn", srv.handleMixerBurn)

	// 9. Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","mode":"standalone-server","project":"gocoke","storage":"%s"}`, srv.store.driver)
	})

	// 10. Sound library resolver (maps un-prefixed WAV names to Director prefixed files)
	mux.HandleFunc("/Lingo/soundlibrary/", srv.handleSoundLibrary)
	mux.HandleFunc("/sounds/", srv.handleSoundLibrary)
	mux.HandleFunc("/soundlibrary/", srv.handleSoundLibrary)

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

type RegisterRequest struct {
	ScreenName string `json:"screenName"`
	Password   string `json:"password"`
	Email      string `json:"email"`
	Gender     int    `json:"gender"`
	AvatarDef  string `json:"avatarDef"`
	Mission    string `json:"mission"`
	StudioType string `json:"studioType"`
	PromoCode  string `json:"promoCode"`
}

func (s *Server) handleRegisterDenizen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req RegisterRequest
	if r.Method == http.MethodPost {
		ct := r.Header.Get("Content-Type")
		if strings.Contains(ct, "application/json") {
			_ = json.NewDecoder(r.Body).Decode(&req)
		} else {
			_ = r.ParseForm()
			req.ScreenName = r.FormValue("screenName")
			req.Password = r.FormValue("password")
			req.Email = r.FormValue("email")
			g, _ := strconv.Atoi(r.FormValue("gender"))
			req.Gender = g
			req.AvatarDef = r.FormValue("avatarDef")
			req.Mission = r.FormValue("mission")
			req.StudioType = r.FormValue("studioType")
			req.PromoCode = r.FormValue("promoCode")
		}
	} else {
		q := r.URL.Query()
		req.ScreenName = q.Get("screenName")
		req.Password = q.Get("password")
		req.Email = q.Get("email")
		g, _ := strconv.Atoi(q.Get("gender"))
		req.Gender = g
		req.AvatarDef = q.Get("avatarDef")
		req.Mission = q.Get("mission")
		req.StudioType = q.Get("studioType")
		req.PromoCode = q.Get("promoCode")
	}

	cleanName := strings.TrimSpace(req.ScreenName)
	if cleanName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "Screen name is required"})
		return
	}

	if req.Gender <= 0 {
		req.Gender = 1
	}
	genderStr := fmt.Sprintf("%d", req.Gender)
	if req.AvatarDef == "" {
		req.AvatarDef = "hr=001/255,194,115&hd=001/245,226,203&ey=001/245,226,203&fc=001/245,226,203&bd=001/245,226,203&lh=001/245,226,203&rh=001/245,226,203&ch=001/255,194,115&ls=002/238,238,238&rs=002/238,238,238&lg=001/127,102,64&sh=001/194,123,56"
	}
	if req.Mission == "" {
		req.Mission = "Coke Music Legend"
	}
	if req.StudioType == "" {
		req.StudioType = "studio_model_c"
	}

	d, err := s.store.GetOrCreateDenizen(cleanName, genderStr, req.AvatarDef, req.Mission)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": err.Error()})
		return
	}

	// Create user's personal Studio
	layoutMap := map[string]int{
		"studio_model_a":       1,
		"studio_model_b":       2,
		"studio_model_c":       3,
		"studio_model_d":       4,
		"studio_model_rooftop": 5,
	}
	layout := layoutMap[req.StudioType]
	if layout == 0 {
		layout = 3
	}
	studioID := fmt.Sprintf("studio_%s", strings.ToLower(cleanName))
	studio, _ := s.store.CreateStudio(studioID, d.ID, cleanName, fmt.Sprintf("%s's Studio", cleanName), req.StudioType, layout)
	if studio != nil {
		s.mu.Lock()
		s.Rooms[studio.ID] = NewRoom(studio.ID, studio.Name, studio.Layout)
		s.mu.Unlock()
	}

	// Handle promo voucher bonus if provided
	if req.PromoCode != "" {
		_, _, _ = s.store.RedeemRedCap(cleanName, req.PromoCode)
	}

	// Generate Base64 SW1 and SW2
	sw1 := base64.StdEncoding.EncodeToString([]byte(cleanName))
	sw2 := d.Secret

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "success",
		"screenName": cleanName,
		"gender":     d.Gender,
		"avatarDef":  d.AvatarDef,
		"mission":    d.Mission,
		"decibels":   d.Decibels,
		"vcoins":     d.VCoins,
		"studioId":   studioID,
		"sw1":        sw1,
		"sw2":        sw2,
		"gameUrl":    fmt.Sprintf("/game/?user=%s&sw1=%s&sw2=%s", url.QueryEscape(cleanName), url.QueryEscape(sw1), url.QueryEscape(sw2)),
	})
}

func (s *Server) handleLegacyLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = r.ParseForm()
	user := r.FormValue("screenName")
	if user == "" {
		user = r.FormValue("user")
	}
	if user == "" {
		user = "Adam"
	}
	d, _ := s.store.GetOrCreateDenizen(user, "1", "", "")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "rCode=CMLOGGEDIN\n")
	fmt.Fprintf(w, "avatar_str=%s\n", d.AvatarDef)
	fmt.Fprintf(w, "screen_name=%s\n", d.ScreenName)
	fmt.Fprintf(w, "db_total=%d\n", d.Decibels)
	fmt.Fprintf(w, "person_id=%s\n", d.ID)
	fmt.Fprintf(w, "SACStatus=1\n")
	fmt.Fprintf(w, "daysToExpire=100\n")
	fmt.Fprintf(w, "refresh=no\n")
	fmt.Fprintf(w, "showStore=yes\n")
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

func (s *Server) handleDenizenProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	user := q.Get("user")
	if user == "" {
		user = q.Get("screenName")
	}
	if user == "" {
		user = "Adam"
	}

	d, err := s.store.GetDenizenByName(user)
	if err != nil || d == nil {
		d, err = s.store.GetOrCreateDenizen(user, "M", "hr=001/1&hd=001/1&ch=001/1&lg=001/1&sh=001/1&fa=001/1&ey=001/1&g=1", "Coke Music Reborn")
	}

	if d != nil {
		json.NewEncoder(w).Encode(map[string]any{
			"success":      true,
			"screen_name":  d.ScreenName,
			"gender":       d.Gender,
			"mission":      d.Mission,
			"avatar_def":   d.AvatarDef,
			"decibels":     d.Decibels,
			"vcoins":       d.VCoins,
			"last_seen_in": d.LastSeenIn,
		})
	} else {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "denizen not found"})
	}
}

func (s *Server) handleUpdateDenizen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	user := r.URL.Query().Get("user")
	genderStr := r.URL.Query().Get("gender")
	avatarDef := r.URL.Query().Get("avatarDef")
	mission := r.URL.Query().Get("mission")

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		if u := r.FormValue("user"); u != "" {
			user = u
		}
		if g := r.FormValue("gender"); g != "" {
			genderStr = g
		}
		if ad := r.FormValue("avatarDef"); ad != "" {
			avatarDef = ad
		}
		if m := r.FormValue("mission"); m != "" {
			mission = m
		}
	}

	if user == "" {
		user = "Adam"
	}
	gender := 1
	if genderStr == "F" || genderStr == "2" || strings.ToLower(genderStr) == "female" {
		gender = 2
	}
	if avatarDef == "" {
		avatarDef = "hr=001/1&hd=001/1&ch=001/1&lg=001/1&sh=001/1&fa=001/1&ey=001/1&g=1"
	}
	if mission == "" {
		mission = "Coke Music Reborn"
	}

	updated, err := s.store.UpdateDenizenProfile(user, gender, avatarDef, mission)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	// Update active Player session in memory and broadcast AV update to room occupants
	s.mu.RLock()
	for _, room := range s.Rooms {
		room.mu.Lock()
		for _, p := range room.Players {
			if strings.EqualFold(p.ScreenName, user) {
				p.Gender = updated.Gender
				p.Definition = updated.AvatarDef
				p.Mission = updated.Mission
				avXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV av=\"%s\" x=\"%d\" z=\"%d\" d=\"%d\" g=\"%d\" m=\"%s\" def=\"%s\" act=\"%s\"/></Room.Action>",
					room.ID, p.ScreenName, p.X, p.Z, p.Dir, p.Gender, p.Mission, p.Definition, p.Action)
				go room.Broadcast(avXML, "")
			}
		}
		room.mu.Unlock()
	}
	s.mu.RUnlock()

	log.Printf("[HTTP] Updated profile for %s: gender=%d def=%s", updated.ScreenName, updated.Gender, updated.AvatarDef)

	json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"screen_name": updated.ScreenName,
		"gender":      updated.Gender,
		"mission":     updated.Mission,
		"avatar_def":  updated.AvatarDef,
		"decibels":    updated.Decibels,
		"vcoins":      updated.VCoins,
	})
}

func (s *Server) handleSoundLibrary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	name := filepath.Base(r.URL.Path)
	soundDir := filepath.Join(s.webRoot, "Lingo", "soundlibrary")
	targetPath := filepath.Join(soundDir, name)

	if _, err := os.Stat(targetPath); err != nil {
		entries, err := os.ReadDir(soundDir)
		if err == nil {
			lowerName := strings.ToLower(name)
			for _, entry := range entries {
				fn := entry.Name()
				if strings.EqualFold(fn, name) {
					targetPath = filepath.Join(soundDir, fn)
					break
				}
				idx := strings.Index(fn, "_")
				if idx != -1 && strings.EqualFold(fn[idx+1:], lowerName) {
					targetPath = filepath.Join(soundDir, fn)
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "audio/wav")
	http.ServeFile(w, r, targetPath)
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

	target, respURI, bodyVal, err := amf.ParseRemotingMessage(body)
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
		// Load from persistent database
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
		screenName := "Adam"
		title := "Coke Beats Vol. 1"
		edl := "^50,rockOne_guitar00{2,0{4,4{^50,danceOne_beatz00{4,0{^"
		if arr, ok := bodyVal.([]any); ok {
			if len(arr) > 0 {
				if s, ok := arr[0].(string); ok && s != "" {
					screenName = s
				}
			}
			if len(arr) > 1 {
				if s, ok := arr[1].(string); ok && s != "" {
					title = s
				}
			}
			if len(arr) > 2 {
				if s, ok := arr[2].(string); ok && s != "" {
					edl = s
				}
			}
		}
		// Deduct 10 dB and mint a new playable CD (catalogItem 89) into backpack
		_, _ = s.store.AdjustBalance(screenName, -10, 0)
		cdProps := map[string]any{
			"songName":  title,
			"ownerName": screenName,
			"edl":       edl,
		}
		newID, _ := s.store.AddBackpackItem(screenName, 89, cdProps)
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
					"wallpaper": float64(st.Wallpaper),
					"floor":     float64(st.Floor),
					"doorbell":  float64(st.Doorbell),
				})
			}
		}
		responseValue = resList

	case strings.Contains(target, "getGameServerByStudioID"):
		hostName := s.host
		if h := r.Header.Get("X-Forwarded-Host"); h != "" {
			hostName = strings.Split(h, ":")[0]
		} else if r.Host != "" {
			hostName = strings.Split(r.Host, ":")[0]
		}
		responseValue = map[string]any{
			"server": hostName,
			"port":   s.port,
			"dns":    hostName,
		}

	// --- Messenger & Social AMF Endpoints ---
	case strings.Contains(target, "getFriends"):
		screenName := "Adam"
		if arr, ok := bodyVal.([]any); ok && len(arr) > 0 {
			if s, ok := arr[0].(string); ok && s != "" {
				screenName = s
			}
		}
		friends, err := s.store.GetFriends(screenName)
		if err != nil {
			log.Printf("[AMF] GetFriends error: %v", err)
		}
		var list []any
		s.mu.RLock()
		for _, f := range friends {
			isOnline := false
			for _, p := range s.Players {
				if strings.EqualFold(p.ScreenName, f.ScreenName) {
					isOnline = true
					break
				}
			}
			list = append(list, map[string]any{
				"screenName": f.ScreenName,
				"gender":     float64(f.Gender),
				"mission":    f.Mission,
				"avatarDef":  f.AvatarDef,
				"online":     isOnline,
				"lastSeen":   f.LastSeenIn,
				"status":     f.Status,
			})
		}
		s.mu.RUnlock()
		responseValue = list

	case strings.Contains(target, "getOnlineFriends"):
		screenName := "Adam"
		if arr, ok := bodyVal.([]any); ok && len(arr) > 0 {
			if s, ok := arr[0].(string); ok && s != "" {
				screenName = s
			}
		}
		friends, _ := s.store.GetFriends(screenName)
		var list []any
		s.mu.RLock()
		for _, f := range friends {
			for _, p := range s.Players {
				if strings.EqualFold(p.ScreenName, f.ScreenName) {
					list = append(list, map[string]any{
						"screenName": f.ScreenName,
						"online":     true,
						"location":   p.RoomID,
					})
					break
				}
			}
		}
		s.mu.RUnlock()
		responseValue = list

	case strings.Contains(target, "sendMessage"):
		from := "Adam"
		to := "Adam"
		msg := ""
		if arr, ok := bodyVal.([]any); ok {
			if len(arr) > 0 {
				if s, ok := arr[0].(string); ok {
					from = s
				}
			}
			if len(arr) > 1 {
				switch v := arr[1].(type) {
				case string:
					to = v
				case []any:
					if len(v) > 0 {
						if s, ok := v[0].(string); ok {
							to = s
						}
					}
				}
			}
			if len(arr) > 2 {
				if s, ok := arr[2].(string); ok {
					msg = s
				}
			}
		}
		msgID, _ := s.store.SendMessage(from, to, msg)
		s.notifyMessengerRecipient(to, from, msg)
		responseValue = float64(msgID)

	case strings.Contains(target, "getMessages"):
		screenName := "Adam"
		if arr, ok := bodyVal.([]any); ok && len(arr) > 0 {
			if s, ok := arr[0].(string); ok && s != "" {
				screenName = s
			}
		}
		msgs, _ := s.store.GetMessages(screenName, false)
		var list []any
		for _, m := range msgs {
			list = append(list, map[string]any{
				"id":        float64(m.ID),
				"sender":    m.SenderName,
				"recipient": m.RecipientName,
				"message":   m.Message,
				"sentAt":    m.SentAt,
				"isRead":    m.IsRead,
			})
		}
		responseValue = list

	case strings.Contains(target, "removeMessage") || strings.Contains(target, "deleteMessage"):
		if arr, ok := bodyVal.([]any); ok && len(arr) > 0 {
			if idFloat, ok := arr[0].(float64); ok {
				_ = s.store.MarkMessageRead(int(idFloat))
			}
		}
		responseValue = true

	case strings.Contains(target, "acceptInvitation"):
		u1, u2 := "Adam", "Adam"
		if arr, ok := bodyVal.([]any); ok && len(arr) >= 2 {
			if s, ok := arr[0].(string); ok {
				u1 = s
			}
			if s, ok := arr[1].(string); ok {
				u2 = s
			}
		}
		_ = s.store.AddFriend(u1, u2)
		s.notifyFriendEvent(u1, u2, "accepted")
		responseValue = true

	case strings.Contains(target, "rejectInvitation") || strings.Contains(target, "removeFriend"):
		u1, u2 := "Adam", "Adam"
		if arr, ok := bodyVal.([]any); ok && len(arr) >= 2 {
			if s, ok := arr[0].(string); ok {
				u1 = s
			}
			if s, ok := arr[1].(string); ok {
				u2 = s
			}
		}
		_ = s.store.RemoveFriend(u1, u2)
		responseValue = true

	// --- Studio Decor AMF Endpoint ---
	case strings.Contains(target, "updateStudio"):
		studioID := "1"
		wp, fl, db := 1, 1, 1
		if arr, ok := bodyVal.([]any); ok {
			if len(arr) > 0 {
				if s, ok := arr[0].(string); ok {
					studioID = s
				} else if f, ok := arr[0].(float64); ok {
					studioID = strconv.Itoa(int(f))
				}
			}
			if len(arr) > 1 {
				if f, ok := arr[1].(float64); ok {
					wp = int(f)
				}
			}
			if len(arr) > 2 {
				if f, ok := arr[2].(float64); ok {
					fl = int(f)
				}
			}
			if len(arr) > 3 {
				if f, ok := arr[3].(float64); ok {
					db = int(f)
				}
			}
		}
		_ = s.store.UpdateStudioDecor(studioID, wp, fl, db)
		rm := s.GetOrCreateRoom(studioID)
		rm.mu.Lock()
		rm.Wallpaper = wp
		rm.Floor = fl
		rm.Doorbell = db
		rm.mu.Unlock()
		decorXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><ST_DECOR wp=\"%d\" fl=\"%d\" db=\"%d\"/></Room.Action>",
			studioID, wp, fl, db)
		go rm.Broadcast(decorXML, "")
		responseValue = true

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

		// Send ST_EN room layout with live decor attributes
		stEnXML := fmt.Sprintf("<ST_EN layout=\"%d\" roomID=\"%s\" name=\"%s\" owner=\"Coke Studios\" wp=\"%d\" fl=\"%d\" db=\"%d\"/>",
			room.Layout, room.ID, room.Name, room.Wallpaper, room.Floor, room.Doorbell)
		_ = player.Send(stEnXML)
		decorXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><ST_DECOR wp=\"%d\" fl=\"%d\" db=\"%d\"/></Room.Action>",
			room.ID, room.Wallpaper, room.Floor, room.Doorbell)
		_ = player.Send(decorXML)

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

	// --- Live Studio Decor (Wall, Floor, Doorbell) ---
	if strings.Contains(xml, "<ST_DECOR") {
		roomID := player.RoomID
		if roomID != "" && roomID != "$LOBBY$" {
			wpStr := extractAttr(xml, "wp")
			flStr := extractAttr(xml, "fl")
			dbStr := extractAttr(xml, "db")
			wp, _ := strconv.Atoi(wpStr)
			fl, _ := strconv.Atoi(flStr)
			db, _ := strconv.Atoi(dbStr)
			if wp <= 0 {
				wp = 1
			}
			if fl <= 0 {
				fl = 1
			}
			if db <= 0 {
				db = 1
			}

			room := s.GetOrCreateRoom(roomID)
			room.mu.Lock()
			room.Wallpaper = wp
			room.Floor = fl
			room.Doorbell = db
			room.mu.Unlock()

			_ = s.store.UpdateStudioDecor(roomID, wp, fl, db)
			decorXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><ST_DECOR wp=\"%d\" fl=\"%d\" db=\"%d\"/></Room.Action>",
				room.ID, wp, fl, db)
			room.Broadcast(decorXML, "")
			log.Printf("[WS] Studio %s decor updated: wallpaper=%d, floor=%d, doorbell=%d", roomID, wp, fl, db)
		}
		return
	}

	// --- Live Messenger / Instant Messaging ---
	if strings.Contains(xml, "<MMS_SND") || strings.Contains(xml, "<MMS_MSG") {
		to := extractAttr(xml, "to")
		msgHex := extractAttr(xml, "m")
		msgText := extractAttr(xml, "msg")
		if msgText == "" && msgHex != "" {
			if dec, err := hex.DecodeString(msgHex); err == nil {
				msgText = string(dec)
			}
		}
		if to != "" && msgText != "" {
			_, _ = s.store.SendMessage(player.ScreenName, to, msgText)
			s.notifyMessengerRecipient(to, player.ScreenName, msgText)
			log.Printf("[MESSENGER] %s -> %s: %s", player.ScreenName, to, msgText)
		}
		return
	}

	// --- Friend Request ---
	if strings.Contains(xml, "<MMS_REQ") {
		to := extractAttr(xml, "to")
		if to != "" && !strings.EqualFold(to, player.ScreenName) {
			s.notifyMessengerEvent(to, fmt.Sprintf("<MMS_REQ from=\"%s\"/>", player.ScreenName))
			log.Printf("[MESSENGER] Friend request: %s -> %s", player.ScreenName, to)
		}
		return
	}

	// --- Friend Accept ---
	if strings.Contains(xml, "<MMS_ACC") {
		from := extractAttr(xml, "from")
		if from != "" {
			_ = s.store.AddFriend(player.ScreenName, from)
			s.notifyFriendEvent(player.ScreenName, from, "accepted")
			log.Printf("[MESSENGER] Friend accepted: %s and %s", player.ScreenName, from)
		}
		return
	}

	// --- In-Game CD Burning ---
	if strings.Contains(xml, "<CD_BURN") {
		title := extractAttr(xml, "title")
		if title == "" {
			title = "My Burned Beat"
		}
		edl := extractAttr(xml, "edl")
		if edl == "" {
			edl = "^50,rockOne_guitar00{2,0{4,4{^"
		}
		newDB, err := s.store.AdjustBalance(player.ScreenName, -10, 0)
		if err == nil {
			cdProps := map[string]any{
				"songName":  title,
				"ownerName": player.ScreenName,
				"edl":       edl,
			}
			newID, _ := s.store.AddBackpackItem(player.ScreenName, 89, cdProps)
			mintXML := fmt.Sprintf("<POS_MINT id=\"%d\" cid=\"89\" title=\"%s\" edl=\"%s\" db=\"%d\"/>",
				newID, title, edl, newDB)
			_ = player.Send(mintXML)
			log.Printf("[MIXER] Player %s burned CD #%d (%q)", player.ScreenName, newID, title)
		}
		return
	}

	// --- Studio Doorbell Knock ---
	if strings.Contains(xml, "<ST_KNOCK") {
		roomID := extractAttr(xml, "roomID")
		if roomID != "" {
			room := s.GetOrCreateRoom(roomID)
			knockXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><ST_KNOCK av=\"%s\"/></Room.Action>",
				room.ID, player.ScreenName)
			room.Broadcast(knockXML, player.ID)
			log.Printf("[DOORBELL] %s knocked on room %s", player.ScreenName, roomID)
		}
		return
	}

	// --- Studio Doorbell Response (Admit / Deny) ---
	if strings.Contains(xml, "<ST_KNOCK_RES") {
		target := extractAttr(xml, "target")
		allow := extractAttr(xml, "allow")
		roomID := player.RoomID
		if target != "" && roomID != "" {
			s.mu.RLock()
			for _, p := range s.Players {
				if strings.EqualFold(p.ScreenName, target) {
					if allow == "1" || allow == "true" {
						_ = p.Send(fmt.Sprintf("<Accepted roomID=\"%s\"/>", roomID))
					} else {
						_ = p.Send("<Alert msg=\"The studio owner declined your knock.\"/>")
					}
					break
				}
			}
			s.mu.RUnlock()
		}
		return
	}

	// --- Studio Moderation Kick ---
	if strings.Contains(xml, "<ST_KICK") {
		target := extractAttr(xml, "target")
		roomID := player.RoomID
		if target != "" && roomID != "" && roomID != "$LOBBY$" {
			room := s.GetOrCreateRoom(roomID)
			var kickedPlayer *Player
			room.mu.Lock()
			for pid, p := range room.Players {
				if strings.EqualFold(p.ScreenName, target) {
					kickedPlayer = p
					delete(room.Players, pid)
					break
				}
			}
			room.mu.Unlock()

			if kickedPlayer != nil {
				kickedPlayer.RoomID = "$LOBBY$"
				_ = kickedPlayer.Send(fmt.Sprintf("<KICKED roomID=\"%s\" by=\"%s\"/>", roomID, player.ScreenName))
				rmXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV_RM av=\"%s\"/></Room.Action>", roomID, target)
				room.Broadcast(rmXML, "")
				log.Printf("[MODERATION] %s kicked %s from %s", player.ScreenName, target, roomID)
			}
		}
		return
	}
}

func (s *Server) handleMixerBurn(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user := r.FormValue("user")
	if user == "" {
		user = r.URL.Query().Get("user")
	}
	if user == "" {
		user = "Adam"
	}
	title := r.FormValue("title")
	if title == "" {
		title = r.URL.Query().Get("title")
	}
	if title == "" {
		title = "My Coke Mix"
	}
	edl := r.FormValue("edl")
	if edl == "" {
		edl = r.URL.Query().Get("edl")
	}
	if edl == "" {
		edl = "^50,rockOne_guitar00{2,0{4,4{^"
	}

	newDB, err := s.store.AdjustBalance(user, -10, 0)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	cdProps := map[string]any{
		"songName":  title,
		"ownerName": user,
		"edl":       edl,
	}
	newID, err := s.store.AddBackpackItem(user, 89, cdProps)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	// Notify active user session in-game
	s.mu.RLock()
	for _, p := range s.Players {
		if strings.EqualFold(p.ScreenName, user) {
			mintXML := fmt.Sprintf("<POS_MINT id=\"%d\" cid=\"89\" title=\"%s\" edl=\"%s\" db=\"%d\"/>",
				newID, title, edl, newDB)
			_ = p.Send(mintXML)
		}
	}
	s.mu.RUnlock()

	json.NewEncoder(w).Encode(map[string]any{
		"success":       true,
		"possession_id": newID,
		"catalog_item":  89,
		"song_name":     title,
		"owner":         user,
		"edl":           edl,
		"new_decibels":  newDB,
	})
}

func (s *Server) notifyMessengerRecipient(to, from, message string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hexMsg := hex.EncodeToString([]byte(message))
	mmsXML := fmt.Sprintf("<MMS_MSG from=\"%s\" m=\"%s\" msg=\"%s\"/>", from, hexMsg, message)
	for _, p := range s.Players {
		if strings.EqualFold(p.ScreenName, to) {
			_ = p.Send(mmsXML)
		}
	}
}

func (s *Server) notifyMessengerEvent(to, xml string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.Players {
		if strings.EqualFold(p.ScreenName, to) {
			_ = p.Send(xml)
		}
	}
}

func (s *Server) notifyFriendEvent(u1, u2, status string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	xml1 := fmt.Sprintf("<MMS_ACC from=\"%s\" status=\"%s\"/>", u2, status)
	xml2 := fmt.Sprintf("<MMS_ACC from=\"%s\" status=\"%s\"/>", u1, status)
	for _, p := range s.Players {
		if strings.EqualFold(p.ScreenName, u1) {
			_ = p.Send(xml1)
		} else if strings.EqualFold(p.ScreenName, u2) {
			_ = p.Send(xml2)
		}
	}
}

func (s *Server) handleMessengerFriends(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user := r.URL.Query().Get("user")
	if user == "" {
		user = "Adam"
	}
	friends, err := s.store.GetFriends(user)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}
	s.mu.RLock()
	for i := range friends {
		for _, p := range s.Players {
			if strings.EqualFold(p.ScreenName, friends[i].ScreenName) {
				friends[i].Online = true
				break
			}
		}
	}
	s.mu.RUnlock()
	json.NewEncoder(w).Encode(map[string]any{"success": true, "friends": friends})
}

func (s *Server) handleMessengerSend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user := r.FormValue("user")
	if user == "" {
		user = r.URL.Query().Get("user")
	}
	to := r.FormValue("to")
	if to == "" {
		to = r.URL.Query().Get("to")
	}
	msg := r.FormValue("message")
	if msg == "" {
		msg = r.URL.Query().Get("message")
	}
	if user == "" || to == "" || msg == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "missing parameters"})
		return
	}
	msgID, err := s.store.SendMessage(user, to, msg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}
	s.notifyMessengerRecipient(to, user, msg)
	json.NewEncoder(w).Encode(map[string]any{"success": true, "id": msgID})
}

func (s *Server) handleMessengerMessages(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user := r.URL.Query().Get("user")
	if user == "" {
		user = "Adam"
	}
	unread := r.URL.Query().Get("unread") == "true"
	msgs, err := s.store.GetMessages(user, unread)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"success": true, "messages": msgs})
}

func (s *Server) handleMessengerAddFriend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user := r.FormValue("user")
	if user == "" {
		user = r.URL.Query().Get("user")
	}
	friend := r.FormValue("friend")
	if friend == "" {
		friend = r.URL.Query().Get("friend")
	}
	if user == "" || friend == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "missing user or friend"})
		return
	}
	err := s.store.AddFriend(user, friend)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}
	s.notifyFriendEvent(user, friend, "accepted")
	json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (s *Server) handleMessengerRemoveFriend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user := r.FormValue("user")
	if user == "" {
		user = r.URL.Query().Get("user")
	}
	friend := r.FormValue("friend")
	if friend == "" {
		friend = r.URL.Query().Get("friend")
	}
	if user == "" || friend == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "missing user or friend"})
		return
	}
	err := s.store.RemoveFriend(user, friend)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (s *Server) handleStudioDecor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	studioID := r.FormValue("studioId")
	if studioID == "" {
		studioID = r.URL.Query().Get("studioId")
	}
	if studioID == "" {
		studioID = "1"
	}

	wpStr := r.FormValue("wallpaper")
	if wpStr == "" {
		wpStr = r.URL.Query().Get("wallpaper")
	}
	flStr := r.FormValue("floor")
	if flStr == "" {
		flStr = r.URL.Query().Get("floor")
	}
	dbStr := r.FormValue("doorbell")
	if dbStr == "" {
		dbStr = r.URL.Query().Get("doorbell")
	}

	wp, _ := strconv.Atoi(wpStr)
	fl, _ := strconv.Atoi(flStr)
	db, _ := strconv.Atoi(dbStr)
	if wp <= 0 {
		wp = 1
	}
	if fl <= 0 {
		fl = 1
	}
	if db <= 0 {
		db = 1
	}

	err := s.store.UpdateStudioDecor(studioID, wp, fl, db)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	rm := s.GetOrCreateRoom(studioID)
	rm.mu.Lock()
	rm.Wallpaper = wp
	rm.Floor = fl
	rm.Doorbell = db
	rm.mu.Unlock()
	decorXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><ST_DECOR wp=\"%d\" fl=\"%d\" db=\"%d\"/></Room.Action>",
		studioID, wp, fl, db)
	go rm.Broadcast(decorXML, "")

	json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"studio_id": studioID,
		"wallpaper": wp,
		"floor":     fl,
		"doorbell":  db,
	})
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

	// Broadcast song start and DJ animation to all room occupants (with 1:1 client attributes: av, a, sn, s)
	cdpXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_P av=\"%s\" a=\"%s\" sn=\"%s\" title=\"%s\" s=\"%s\" catId=\"89\"/></Room.Action>",
		room.ID, dj, dj, title, title, edl)
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

		// Credit Decibels to DJ in persistent database
		newBalance, _ := s.store.AdjustBalance(dj, rewardDB, 0)
		log.Printf("[MUSIC] Performance ended for %s in %s. Cheers: %d, Boos: %d, Awarded: +%d dB (New Balance: %d)",
			dj, room.ID, cheers, boos, rewardDB, newBalance)

		// 1. Broadcast CD_VR with tur, tdr, pa for official performance results dialog
		vrXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_VR tur=\"%d\" tdr=\"%d\" pa=\"%d\"/></Room.Action>",
			room.ID, cheers, boos, rewardDB)
		room.Broadcast(vrXML, "")

		// 2. Broadcast CD_PRS with reaction summary for crowd cheers/boos audio
		prsXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_PRS av=\"%s\" tur=\"%d\" tdr=\"%d\" tu=\"%d\" td=\"%d\" db=\"%d\"/></Room.Action>",
			room.ID, dj, cheers, boos, cheers, boos, rewardDB)
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
	if room.Performance == nil || room.Performance.Done {
		room.mu.Unlock()
		return
	}
	if room.Performance.Voters[voter] {
		// Already voted in this performance round
		room.mu.Unlock()
		return
	}
	room.Performance.Voters[voter] = true

	var tuXML, tdXML string
	if delta > 0 {
		room.Performance.Cheers++
		tuXML = fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_TU av=\"%s\"/></Room.Action>", room.ID, voter)
	} else {
		room.Performance.Boos++
		tdXML = fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_TD av=\"%s\"/></Room.Action>", room.ID, voter)
	}

	cheers := room.Performance.Cheers
	boos := room.Performance.Boos
	vrXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_VR tu=\"%d\" td=\"%d\"/></Room.Action>",
		room.ID, cheers, boos)
	room.mu.Unlock()

	// Broadcast events after releasing lock
	if tuXML != "" {
		room.Broadcast(tuXML, "")
	} else if tdXML != "" {
		room.Broadcast(tdXML, "")
	}
	room.Broadcast(vrXML, "")

	log.Printf("[VOTE] [%s] %s voted (delta: %d, Cheers: %d, Boos: %d)",
		room.ID, voter, delta, cheers, boos)
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
