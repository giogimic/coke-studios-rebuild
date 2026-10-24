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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gocoke/internal/client/minigames/sumo"
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

type PacketWriter interface {
	WriteBinary(p []byte) error
	Close() error
}

type Player struct {
	ID            string
	ScreenName    string
	Authenticated bool
	RoomID        string
	X             int
	Z             int
	Dir           int
	Gender        int
	Mission       string
	Definition    string
	Action        string
	Conn          PacketWriter
	mu            sync.Mutex
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



type Server struct {
	mu           sync.RWMutex
	Rooms        map[string]*Room
	Players      map[string]*Player
	webRoot      string
	host         string
	port         string
	store        *Store
	nextPosID    int
	catalogMu    sync.RWMutex
	catalog      map[int]*CatalogItem
	catalogList  []*CatalogItem
	tradeManager *TradeManager
	sumoManager  *SumoManager
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

	s.sumoManager = NewSumoManager(s)

	s.tradeManager = NewTradeManager(s.store, func(roomID, xml string) {
		if rm := s.GetOrCreateRoom(roomID); rm != nil {
			rm.Broadcast(xml, "")
		}
	}, func(screenName, xml string) error {
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, p := range s.Players {
			if strings.EqualFold(p.ScreenName, screenName) {
				return p.Send(xml)
			}
		}
		return nil
	})

	// Initialize default rooms in memory from store
	s.Rooms["$LOBBY$"] = NewRoom("$LOBBY$", "Lobby", 0)
	studios, _ := store.GetStudios()
	for _, st := range studios {
		r := NewRoom(st.ID, st.Name, st.Layout)
		r.OwnerName = st.OwnerName
		r.Wallpaper = st.Wallpaper
		r.Floor = st.Floor
		r.Doorbell = st.Doorbell
		r.LoadPathMap(s.webRoot)
		s.Rooms[st.ID] = r
	}

	// Initialize authoritative catalogue and evidenced prices
	s.InitCatalog()

	return s
}

func main() {
	_ = mime.AddExtensionType(".wasm", "application/wasm")
	_ = mime.AddExtensionType(".br", "application/x-brotli")
	_ = mime.AddExtensionType(".ls", "text/plain; charset=utf-8")
	_ = mime.AddExtensionType(".csv", "text/csv; charset=utf-8")
	_ = mime.AddExtensionType(".txt", "text/plain; charset=utf-8")
	_ = mime.AddExtensionType(".htm", "text/html; charset=utf-8")

	defaultPort := os.Getenv("PORT")
	if defaultPort == "" {
		defaultPort = "8080"
	}
	defaultAddr := "0.0.0.0:" + defaultPort
	addr := flag.String("addr", defaultAddr, "listen address (default 0.0.0.0:8080 or $PORT)")
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

	log.Printf("==========================================================")
	log.Printf(" Coke Studios Authoritative Server (Debian / Linux First)")
	log.Printf(" Listening on http://%s (Serving %s)", *addr, *root)
	log.Printf(" SQLite Store:              %s", *dbPath)
	log.Printf(" WebSocket Realtime Bridge: ws://%s/ws/realtime", *addr)
	log.Printf(" AMF0 Flash Gateway:        http://%s/sf/gateway", *addr)
	log.Printf("==========================================================")

	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
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
	cleanPass := strings.TrimSpace(req.Password)
	w.Header().Set("Content-Type", "application/json")

	if cleanName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "Screen name is required"})
		return
	}
	if cleanPass == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "Password is required"})
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

	d, err := s.store.RegisterDenizen(cleanName, cleanPass, genderStr, req.AvatarDef, req.Mission)
	if err != nil {
		if errors.Is(err, ErrDenizenExists) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "Screen name already taken"})
			return
		}
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

	// Create authoritative session token
	sess, err := s.store.CreateSession(d.ID, d.ScreenName, defaultSessionTTL)
	sw2 := d.Secret
	if err == nil && sess != nil {
		sw2 = sess.Token
	}

	// Generate Base64 SW1
	sw1 := base64.StdEncoding.EncodeToString([]byte(cleanName))

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "coke_session",
		Value:    sw2,
		Path:     "/",
		Expires:  time.Now().Add(defaultSessionTTL),
		HttpOnly: true,
	})

	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "success",
		"screenName": cleanName,
		"gender":     d.Gender,
		"avatarDef":  d.AvatarDef,
		"mission":    d.Mission,
		"decibels":   d.Decibels,
		"vcoins":     d.VCoins,
		"studioId":   studioID,
		"studioType": req.StudioType,
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
	pass := r.FormValue("password")
	if pass == "" {
		pass = r.URL.Query().Get("password")
	}

	d, err := s.store.VerifyCredentials(user, pass)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err != nil || d == nil {
		fmt.Fprintf(w, "rCode=LOGINFAILED\nerror=Invalid screen name or password\n")
		return
	}

	sess, _ := s.store.CreateSession(d.ID, d.ScreenName, defaultSessionTTL)
	token := d.Secret
	if sess != nil {
		token = sess.Token
	}

	fmt.Fprintf(w, "rCode=CMLOGGEDIN\n")
	fmt.Fprintf(w, "avatar_str=%s\n", d.AvatarDef)
	fmt.Fprintf(w, "screen_name=%s\n", d.ScreenName)
	fmt.Fprintf(w, "db_total=%d\n", d.Decibels)
	fmt.Fprintf(w, "person_id=%s\n", d.ID)
	fmt.Fprintf(w, "sid=%s\n", token)
	fmt.Fprintf(w, "SACStatus=1\n")
	fmt.Fprintf(w, "daysToExpire=100\n")
	fmt.Fprintf(w, "refresh=no\n")
	fmt.Fprintf(w, "showStore=yes\n")
}

func (s *Server) handleLoginDenizen(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var denizen *DenizenRecord
	var err error

	// 1. Try session token from request
	token := extractRequestToken(r)
	if token != "" {
		sess, sessErr := s.store.GetSession(token)
		if sessErr == nil && sess != nil {
			if time.Now().After(sess.ExpiresAt) {
				_ = s.store.DeleteSession(token)
				w.Header().Set("Content-Type", "text/xml;charset=UTF-8")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<LoginResult>
  <ErrorCode>1</ErrorCode>
  <ErrorMessage>Authentication failed: session has expired</ErrorMessage>
</LoginResult>`))
				return
			}
			denizen, err = s.store.GetDenizenByID(sess.DenizenID)
		}
		if denizen == nil {
			denizen, _ = s.store.GetDenizenBySecret(token)
		}
	}

	// 2. Try username & password
	if denizen == nil {
		user := q.Get("user")
		if user == "" { user = q.Get("screenName") }
		pass := q.Get("password")
		if pass == "" { pass = q.Get("pass") }
		if user != "" && pass != "" {
			denizen, err = s.store.VerifyCredentials(user, pass)
		}
	}

	// 3. Fallback for test fixtures / prototype accounts without password
	if denizen == nil {
		user := q.Get("user")
		if user == "" { user = q.Get("screenName") }
		if enc := q.Get("sw1"); enc != "" {
			if dec, decErr := base64.StdEncoding.DecodeString(enc); decErr == nil && len(dec) > 0 {
				user = string(dec)
			}
		}
		sid := q.Get("sid")
		if sid == "" { sid = q.Get("sidempty") }
		if sid != "" {
			parts := strings.Split(sid, "_")
			if len(parts) > 0 && parts[0] != "" {
				user = parts[0]
			}
		}
		if user != "" {
			if d, findErr := s.store.GetDenizenByName(user); findErr == nil && d != nil {
				if d.PasswordHash == "" {
					denizen = d
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/xml;charset=UTF-8")

	if denizen == nil {
		xmlErr := `<?xml version="1.0" encoding="UTF-8"?>
<LoginResult>
  <ErrorCode>1</ErrorCode>
  <ErrorMessage>Authentication failed: invalid credentials or session</ErrorMessage>
</LoginResult>`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(xmlErr))
		return
	}

	// Create or refresh session
	sess, err := s.store.CreateSession(denizen.ID, denizen.ScreenName, defaultSessionTTL)
	sessionToken := denizen.Secret
	if err == nil && sess != nil {
		sessionToken = sess.Token
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
</LoginResult>`, gStr, xmlDef, denizen.ScreenName, denizen.Mission, denizen.LastSeenIn, sessionToken)

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
	user := strings.TrimSpace(q.Get("user"))
	if user == "" {
		user = strings.TrimSpace(q.Get("screenName"))
	}
	if user == "" {
		if authD, err := s.authenticatedDenizen(r); err == nil && authD != nil {
			user = authD.ScreenName
		}
	}
	if user == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "user parameter required"})
		return
	}

	d, err := s.store.GetDenizenByName(user)
	if err != nil || d == nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "denizen not found"})
		return
	}

	studioID := fmt.Sprintf("studio_%s", strings.ToLower(d.ScreenName))
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":      true,
		"screen_name":  d.ScreenName,
		"gender":       d.Gender,
		"mission":      d.Mission,
		"avatar_def":   d.AvatarDef,
		"decibels":     d.Decibels,
		"vcoins":       d.VCoins,
		"last_seen_in": d.LastSeenIn,
		"studio_id":    studioID,
	})
}

func (s *Server) handleUpdateDenizen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	authD, err := s.authenticatedDenizen(r)
	if err != nil || authD == nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "Unauthorized: missing or invalid session"})
		return
	}

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

	if user != "" && !strings.EqualFold(user, authD.ScreenName) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "Forbidden: cannot update other users"})
		return
	}

	gender := authD.Gender
	if genderStr == "F" || genderStr == "2" || strings.ToLower(genderStr) == "female" {
		gender = 2
	} else if genderStr == "M" || genderStr == "1" || strings.ToLower(genderStr) == "male" {
		gender = 1
	}
	if avatarDef == "" {
		avatarDef = authD.AvatarDef
	}
	if mission == "" {
		mission = authD.Mission
	}

	updated, err := s.store.UpdateDenizenProfile(authD.ScreenName, gender, avatarDef, mission)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
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

func extractAMFCredentials(bodyVal any) (string, string) {
	if arr, ok := bodyVal.([]any); ok {
		var sn, sec string
		if len(arr) > 0 {
			if s, ok := arr[0].(string); ok {
				sn = strings.TrimSpace(s)
			}
		}
		if len(arr) > 1 {
			if s, ok := arr[1].(string); ok {
				sec = strings.TrimSpace(s)
			}
		}
		return sn, sec
	}
	return "", ""
}

func (s *Server) resolveAMFDenizen(r *http.Request, bodyVal any) *DenizenRecord {
	sn, sec := extractAMFCredentials(bodyVal)
	if sn != "" && sec != "" {
		if d, err := s.authenticateRealtime(sn, sec); err == nil && d != nil {
			return d
		}
	}
	if sn != "" {
		if d, err := s.store.GetDenizenByName(sn); err == nil && d != nil {
			if d.PasswordHash == "" {
				return d
			}
		}
	}
	if d, err := s.authenticatedDenizen(r); err == nil && d != nil {
		return d
	}
	return nil
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
		d := s.resolveAMFDenizen(r, bodyVal)
		if d == nil {
			responseValue = float64(0)
		} else {
			decibels, _, _ := s.store.GetBalance(d.ScreenName)
			responseValue = float64(decibels)
		}

	case strings.Contains(target, "getProductList"):
		responseValue = s.GetProductListAMF()

	case strings.Contains(target, "getBackpack") || strings.Contains(target, "getPossessionsInBackpack"):
		d := s.resolveAMFDenizen(r, bodyVal)
		if d == nil {
			responseValue = map[string]any{"possessionlist": []any{}}
		} else {
			items, err := s.store.GetBackpack(d.ScreenName)
			var list []any
			if err == nil && len(items) > 0 {
				for _, it := range items {
					list = append(list, map[string]any{
						"id":          float64(it.ID),
						"catalogItem": float64(it.CatalogItemID),
						"prodId":      float64(it.CatalogItemID),
						"properties":  it.Properties,
					})
				}
			}
			responseValue = map[string]any{"possessionlist": list}
		}

	case strings.Contains(target, "getPossessionList"):
		d := s.resolveAMFDenizen(r, bodyVal)
		if d == nil {
			responseValue = []any{}
		} else {
			items, err := s.store.GetBackpack(d.ScreenName)
			var list []any
			if err == nil && len(items) > 0 {
				for _, it := range items {
					list = append(list, map[string]any{
						"id":          float64(it.ID),
						"catalogItem": float64(it.CatalogItemID),
						"prodId":      float64(it.CatalogItemID),
						"properties":  it.Properties,
					})
				}
			}
			responseValue = list
		}

	case strings.Contains(target, "prePurchaseItem"):
		d := s.resolveAMFDenizen(r, bodyVal)
		if d == nil {
			responseValue = map[string]any{"__amf_error_type": "SecurityException", "__amf_error_message": "Unauthorized"}
			break
		}
		catID := 1
		attrs := "[:]"
		if arr, ok := bodyVal.([]any); ok {
			if len(arr) > 2 {
				switch v := arr[2].(type) {
				case float64:
					catID = int(v)
				case int:
					catID = v
				case string:
					catID, _ = strconv.Atoi(v)
				}
			}
			if len(arr) > 3 {
				if s, ok := arr[3].(string); ok {
					attrs = s
				}
			}
		}
		res, err := s.PrePurchase(d.ScreenName, catID, attrs)
		if err != nil {
			responseValue = map[string]any{"__amf_error_type": "CatalogException", "__amf_error_message": err.Error()}
		} else {
			responseValue = res
		}

	case strings.Contains(target, "purchaseItem"):
		d := s.resolveAMFDenizen(r, bodyVal)
		if d == nil {
			responseValue = map[string]any{"__amf_error_type": "SecurityException", "__amf_error_message": "Unauthorized"}
			break
		}
		catID := 0
		var attrs map[string]any
		idempotencyKey := ""
		if arr, ok := bodyVal.([]any); ok {
			if len(arr) > 2 {
				switch v := arr[2].(type) {
				case float64:
					catID = int(v)
				case int:
					catID = v
				case string:
					catID, _ = strconv.Atoi(v)
				}
			}
			if len(arr) > 3 {
				if m, ok := arr[3].(map[string]any); ok {
					attrs = m
					if k, ok := m["requestId"].(string); ok {
						idempotencyKey = k
					} else if k, ok := m["idempotencyKey"].(string); ok {
						idempotencyKey = k
					}
				}
			}
		}
		res, err := s.Purchase(d.ScreenName, catID, attrs, idempotencyKey)
		if err != nil {
			responseValue = map[string]any{"__amf_error_type": "PurchaseException", "__amf_error_message": err.Error()}
		} else {
			responseValue = res
		}

	case strings.Contains(target, "burnCD"):
		d := s.resolveAMFDenizen(r, bodyVal)
		if d == nil {
			responseValue = map[string]any{"__amf_error_type": "SecurityException", "__amf_error_message": "Unauthorized"}
			break
		}
		screenName := d.ScreenName
		title := "My Track"
		edl := ""
		if arr, ok := bodyVal.([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok && strings.HasPrefix(s, "^") {
					edl = s
				} else if s, ok := item.(string); ok && s != screenName && s != "" && !strings.Contains(s, "secret") {
					title = s
				}
			}
		}
		if edl == "" {
			responseValue = map[string]any{"__amf_error_type": "MusicException", "__amf_error_message": "Empty EDL sequence"}
			break
		}
		songID, err := s.store.SaveSong(screenName, title, edl)
		if err != nil {
			responseValue = map[string]any{"__amf_error_type": "DatabaseException", "__amf_error_message": err.Error()}
			break
		}
		newDB, _ := s.store.AdjustBalance(screenName, -10, 0)
		cdProps := map[string]any{
			"songId":    float64(songID),
			"songName":  title,
			"ownerName": screenName,
			"artist":    screenName,
			"edl":       edl,
		}
		newID, _ := s.store.AddBackpackItem(screenName, 89, cdProps)
		responseValue = map[string]any{
			"success":     true,
			"possession":  float64(newID),
			"catalogItem": float64(89),
			"properties":  cdProps,
			"balance":     float64(newDB),
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
		screenName := ""
		if arr, ok := bodyVal.([]any); ok && len(arr) > 0 {
			if s, ok := arr[0].(string); ok && s != "" {
				screenName = s
			}
		}
		if screenName == "" {
			if authD, _ := s.authenticatedDenizen(r); authD != nil {
				screenName = authD.ScreenName
			}
		}
		if screenName == "" {
			responseValue = []any{}
			break
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
		screenName := ""
		if arr, ok := bodyVal.([]any); ok && len(arr) > 0 {
			if s, ok := arr[0].(string); ok && s != "" {
				screenName = s
			}
		}
		if screenName == "" {
			if authD, _ := s.authenticatedDenizen(r); authD != nil {
				screenName = authD.ScreenName
			}
		}
		if screenName == "" {
			responseValue = []any{}
			break
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
		from := ""
		if authD, _ := s.authenticatedDenizen(r); authD != nil {
			from = authD.ScreenName
		}
		to := ""
		msg := ""
		if arr, ok := bodyVal.([]any); ok {
			if len(arr) > 0 && from == "" {
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
		if from != "" && to != "" && msg != "" {
			msgID, _ := s.store.SendMessage(from, to, msg)
			s.notifyMessengerRecipient(to, from, msg)
			responseValue = float64(msgID)
		} else {
			responseValue = float64(0)
		}

	case strings.Contains(target, "getMessages"):
		screenName := ""
		if arr, ok := bodyVal.([]any); ok && len(arr) > 0 {
			if s, ok := arr[0].(string); ok && s != "" {
				screenName = s
			}
		}
		if screenName == "" {
			if authD, _ := s.authenticatedDenizen(r); authD != nil {
				screenName = authD.ScreenName
			}
		}
		if screenName == "" {
			responseValue = []any{}
			break
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
		u1, u2 := "", ""
		if arr, ok := bodyVal.([]any); ok && len(arr) >= 2 {
			if s, ok := arr[0].(string); ok {
				u1 = s
			}
			if s, ok := arr[1].(string); ok {
				u2 = s
			}
		}
		if authD, _ := s.authenticatedDenizen(r); authD != nil {
			if u1 == "" {
				u1 = authD.ScreenName
			}
		}
		if u1 != "" && u2 != "" {
			_ = s.store.AcceptFriendRequest(u1, u2)
			s.notifyFriendEvent(u1, u2, "accepted")
			responseValue = true
		} else {
			responseValue = false
		}

	case strings.Contains(target, "rejectInvitation") || strings.Contains(target, "removeFriend"):
		u1, u2 := "", ""
		if arr, ok := bodyVal.([]any); ok && len(arr) >= 2 {
			if s, ok := arr[0].(string); ok {
				u1 = s
			}
			if s, ok := arr[1].(string); ok {
				u2 = s
			}
		}
		if authD, _ := s.authenticatedDenizen(r); authD != nil {
			if u1 == "" {
				u1 = authD.ScreenName
			}
		}
		if u1 != "" && u2 != "" {
			_ = s.store.RejectFriendRequest(u1, u2)
			_ = s.store.RemoveFriend(u1, u2)
			responseValue = true
		} else {
			responseValue = false
		}

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
		if rm != nil {
			rm.mu.Lock()
			rm.Wallpaper = wp
			rm.Floor = fl
			rm.Doorbell = db
			rm.mu.Unlock()
			decorXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><ST_DECOR wp=\"%d\" fl=\"%d\" db=\"%d\"/></Room.Action>",
				studioID, wp, fl, db)
			go rm.Broadcast(decorXML, "")
		}
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

func extractTagContent(xml, tag string) string {
	open := "<" + tag
	idx := strings.Index(xml, open)
	if idx == -1 {
		return ""
	}
	closeBracket := strings.Index(xml[idx:], ">")
	if closeBracket == -1 {
		return ""
	}
	start := idx + closeBracket + 1
	endTag := "</" + tag + ">"
	endIdx := strings.Index(xml[start:], endTag)
	if endIdx == -1 {
		return ""
	}
	return strings.TrimSpace(xml[start : start+endIdx])
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
		ID:            playerID,
		ScreenName:    "",
		Authenticated: false,
		Action:        "std",
		Dir:           2,
		X:             0,
		Z:             0,
		Conn:          conn,
	}

	s.mu.Lock()
	s.Players[playerID] = player
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.Players, playerID)
		s.mu.Unlock()

		if s.tradeManager != nil && player.ScreenName != "" {
			s.tradeManager.AbortTradeForPlayer(player.ScreenName)
		}

		if player.RoomID != "" && player.ScreenName != "" {
			s.removePlayerFromRoom(player, player.RoomID)
		}
		if player.ScreenName != "" {
			log.Printf("[WS] Player %s disconnected", player.ScreenName)
		}
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
	if strings.Contains(xml, "<System.Login") || strings.Contains(xml, "<Login") {
		msgID := extractAttr(xml, "msgID")
		if msgID == "" {
			msgID = "1"
		}
		sn := extractTagContent(xml, "Username")
		if sn == "" {
			sn = extractAttr(xml, "username")
		}
		if sn == "" {
			sn = extractAttr(xml, "screenName")
		}
		if dec, err := hex.DecodeString(sn); err == nil && len(dec) > 0 {
			sn = string(dec)
		}

		sec := extractTagContent(xml, "Password")
		if sec == "" {
			sec = extractAttr(xml, "password")
		}
		if sec == "" {
			sec = extractAttr(xml, "secret")
		}
		if dec, err := hex.DecodeString(sec); err == nil && len(dec) > 0 {
			sec = string(dec)
		}

		def := extractAttr(xml, "def")

		denizen, err := s.authenticateRealtime(sn, sec)
		if err != nil {
			// Compatibility check: prototype accounts with empty password
			if strings.HasPrefix(sec, sn+"_") || strings.EqualFold(sn, "Adam") {
				if d, findErr := s.store.GetDenizenByName(sn); findErr == nil && d != nil && d.PasswordHash == "" {
					denizen = d
					err = nil
				}
			}
		}

		if err != nil || denizen == nil {
			log.Printf("[WS] Realtime login REJECTED for user %q: %v", sn, err)
			_ = player.Send(fmt.Sprintf("<LoginFailed msgID=\"%s\" reason=\"Invalid credentials\"/>", msgID))
			return
		}

		player.ScreenName = denizen.ScreenName
		player.ID = denizen.ID
		player.Gender = denizen.Gender
		player.Mission = denizen.Mission
		player.Definition = denizen.AvatarDef
		if def != "" {
			player.Definition = def
		}
		player.Authenticated = true

		log.Printf("[WS] Player Realtime Login: %s (msgID: %s)", player.ScreenName, msgID)
		_ = player.Send(fmt.Sprintf("<Accepted msgID=\"%s\"/>", msgID))
		return
	}

	if !player.Authenticated {
		log.Printf("[WS] Dropped message from unauthenticated socket: %s", xml)
		return
	}

	if strings.Contains(xml, "<Room.Enter") {
		msgID := extractAttr(xml, "msgID")
		roomID := extractAttr(xml, "roomID")
		log.Printf("[WS] Player %s entering room %s (msgID: %s)", player.ScreenName, roomID, msgID)
		_ = s.enterPlayerRoom(player, roomID, msgID)
		return
	}

	if strings.Contains(xml, "<Room.Exit") {
		msgID := extractAttr(xml, "msgID")
		roomID := extractAttr(xml, "roomID")
		s.exitPlayerRoom(player, roomID, msgID)
		return
	}

	if strings.Contains(xml, "<AV_EN") {
		def := extractAttr(xml, "def")
		s.spawnAvatarInRoom(player, def)
		return
	}

	if strings.Contains(xml, "<AV_GSP") {
		pts := parsePPoints(xml)
		if len(pts) > 0 {
			_ = s.movePlayerInRoom(player, pts)
		}
		return
	}

	if strings.Contains(xml, "<AV_MV") {
		xStr := extractAttr(xml, "x")
		zStr := extractAttr(xml, "z")
		x, _ := strconv.Atoi(xStr)
		z, _ := strconv.Atoi(zStr)
		_ = s.movePlayerInRoom(player, [][2]int{{x, z}})
		return
	}

	if strings.Contains(xml, "<CH ") {
		sm := extractAttr(xml, "sm")
		if sm == "" || sm == "speak" {
			sm = "1"
		} else if sm == "shout" {
			sm = "2"
		} else if sm == "sing" {
			sm = "3"
		}
		mHex := extractAttr(xml, "m")
		roomID := player.RoomID
		if roomID != "" && mHex != "" {
			room := s.GetOrCreateRoom(roomID)
			decoded, _ := hex.DecodeString(mHex)
			plainMsg := cleanProfanity(string(decoded))

			if sm == "2" || sm == "3" {
				cleanHex := hex.EncodeToString([]byte(plainMsg))
				chatXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CH av=\"%s\" sm=\"%s\" m=\"%s\"/></Room.Action>",
					room.ID, player.ScreenName, sm, cleanHex)
				room.Broadcast(chatXML, "")
			} else {
				room.mu.RLock()
				for _, other := range room.Players {
					dx := player.X - other.X
					if dx < 0 { dx = -dx }
					dz := player.Z - other.Z
					if dz < 0 { dz = -dz }
					dist := dx
					if dz > dist { dist = dz }

					var msgForOther string
					if dist <= 4 {
						msgForOther = plainMsg
					} else if dist <= 7 {
						msgForOther = obscureVowels(plainMsg)
					} else {
						msgForOther = "..."
					}
					otherHex := hex.EncodeToString([]byte(msgForOther))
					otherXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CH av=\"%s\" sm=\"%s\" m=\"%s\"/></Room.Action>",
						room.ID, player.ScreenName, sm, otherHex)
					_ = other.Send(otherXML)
				}
				room.mu.RUnlock()
			}
			log.Printf("[CHAT] [%s] %s: %s (mode: %s)", roomID, player.ScreenName, plainMsg, sm)
		}
		return
	}

	// --- DJ Performance & Cheer/Boo Voting System ---
	if strings.Contains(xml, "<CD_P") {
		// Player triggered CD playback or DJ booth performance
		sEDL := extractAttr(xml, "s")
		title := extractAttr(xml, "title")
		if title == "" {
			title = "My Mix"
		}
		if sEDL != "" {
			roomID := player.RoomID
			if roomID != "" {
				room := s.GetOrCreateRoom(roomID)
				s.startPerformance(room, player.ScreenName, title, sEDL)
			}
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
			count, awarded, _ := s.store.RecordDailyCokeDrink(player.ScreenName)
			drinkXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV_DC av=\"%s\" act=\"drk\" item=\"coke_can\"/></Room.Action>",
				room.ID, player.ScreenName)
			room.Broadcast(drinkXML, "")
			if awarded {
				log.Printf("[WS] Player %s received a Coke drink #%d today in %s (+10 dB)", player.ScreenName, count, roomID)
			} else {
				log.Printf("[WS] Player %s received a Coke drink #%d today in %s (daily cap reached)", player.ScreenName, count, roomID)
			}
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

	// --- Furniture Placement, Rotation, and Pickup (Persistent in SQLite/MariaDB) ---
	if strings.Contains(xml, "<POS_STD") {
		posID := extractAttr(xml, "id")
		cidStr := extractAttr(xml, "cid")
		cid, _ := strconv.Atoi(cidStr)
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
			rm, err := s.GetRoom(roomID)
			if err == nil && rm != nil {
				// Authoritatively place or move/rotate furniture from backpack
				if err := s.store.PlaceBackpackItem(rm.ID, player.ScreenName, posID, cid, x, y, z, ho, d, st, false, nil); err != nil {
					log.Printf("[WS] Player %s unauthorized or failed to place %s in %s: %v", player.ScreenName, posID, rm.ID, err)
					return
				}
				rm.SetFurnitureSolid(posID, x, z, true)

				posXML := fmt.Sprintf("<POS id=\"%s\" cid=\"%d\" x=\"%d\" y=\"%d\" z=\"%d\" ho=\"%d\" d=\"%d\" st=\"%d\" iaot=\"false\"/>",
					posID, cid, x, y, z, ho, d, st)
				actionXML := fmt.Sprintf("<Room.Action roomID=\"%s\">%s</Room.Action>", rm.ID, posXML)
				rm.Broadcast(actionXML, "")
				log.Printf("[WS] Player %s placed furniture %s (cid: %d) at (%d,%d,%d) in %s", player.ScreenName, posID, cid, x, y, z, roomID)
			}
		}
		return
	}

	if strings.Contains(xml, "<POS_BKPK") {
		posID := extractAttr(xml, "id")
		roomID := player.RoomID
		if roomID != "" && posID != "" {
			rm, err := s.GetRoom(roomID)
			if err == nil && rm != nil {
				if err := s.store.PickupFurnitureToBackpack(rm.ID, player.ScreenName, posID); err != nil {
					log.Printf("[WS] Player %s failed to pickup %s from %s: %v", player.ScreenName, posID, roomID, err)
					return
				}
				rm.RemoveFurnitureSolid(posID)

				rmXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><POS_RM id=\"%s\"/></Room.Action>", rm.ID, posID)
				rm.Broadcast(rmXML, "")
				log.Printf("[WS] Player %s picked up furniture %s from %s", player.ScreenName, posID, roomID)
			}
		}
		return
	}

	// --- Live Studio Decor (Wall, Floor, Doorbell) ---
	if strings.Contains(xml, "<ST_DECOR") {
		wp, _ := strconv.Atoi(extractAttr(xml, "wp"))
		fl, _ := strconv.Atoi(extractAttr(xml, "fl"))
		db, _ := strconv.Atoi(extractAttr(xml, "db"))
		s.applyRealtimeStudioDecor(player, wp, fl, db)
		return
	}

	// --- Live Messenger / Instant Messaging ---
	if strings.Contains(xml, "<MMS_SND") || strings.Contains(xml, "<MMS_MSG") || strings.Contains(xml, "<MSGR_SND_M") {
		to := extractAttr(xml, "to")
		if to == "" {
			to = extractAttr(xml, "rcv")
		}
		msgText := extractAttr(xml, "msg")
		if msgText == "" {
			msgHex := extractAttr(xml, "m")
			if msgHex != "" {
				if dec, err := hex.DecodeString(msgHex); err == nil {
					msgText = string(dec)
				}
			}
		}
		s.deliverRealtimeMessengerMessage(player, to, msgText)
		return
	}

	// --- Friend Request ---
	if strings.Contains(xml, "<MMS_REQ") || strings.Contains(xml, "<MSGR_INV") {
		to := extractAttr(xml, "to")
		if to == "" {
			to = extractAttr(xml, "sn")
		}
		s.handleMessengerRequest(player, to)
		return
	}

	// --- Friend Accept ---
	if strings.Contains(xml, "<MMS_ACC") || strings.Contains(xml, "<MSGR_AC_INV") {
		from := extractAttr(xml, "from")
		if from == "" {
			from = extractAttr(xml, "inviter")
		}
		s.handleMessengerAccept(player, from)
		return
	}

	// --- Friend Reject / Refuse ---
	if strings.Contains(xml, "<MMS_REJ") || strings.Contains(xml, "<MSGR_RJ_INV") {
		from := extractAttr(xml, "from")
		if from == "" {
			from = extractAttr(xml, "inviter")
		}
		s.handleMessengerReject(player, from)
		return
	}

	// --- Call For Help / Report ---
	if strings.Contains(xml, "<CFH") {
		msg := extractAttr(xml, "msg")
		suspect := extractAttr(xml, "av")
		s.handleCallForHelp(player, suspect, msg)
		return
	}

	// --- In-Game CD Burning ---
	if strings.Contains(xml, "<CD_BURN") {
		title := extractAttr(xml, "title")
		if title == "" {
			title = "My Burned Beat"
		}
		edl := extractAttr(xml, "edl")
		if edl != "" {
			songID, _ := s.store.SaveSong(player.ScreenName, title, edl)
			newDB, err := s.store.AdjustBalance(player.ScreenName, -10, 0)
			if err == nil {
				cdProps := map[string]any{
					"songId":    float64(songID),
					"songName":  title,
					"ownerName": player.ScreenName,
					"artist":    player.ScreenName,
					"edl":       edl,
				}
				newID, _ := s.store.AddBackpackItem(player.ScreenName, 89, cdProps)
				mintXML := fmt.Sprintf("<POS_MINT id=\"%d\" cid=\"89\" title=\"%s\" edl=\"%s\" db=\"%d\"/>",
					newID, title, edl, newDB)
				_ = player.Send(mintXML)
				log.Printf("[MIXER] Player %s burned CD #%d (%q)", player.ScreenName, newID, title)
			}
		}
		return
	}

	// --- Studio Doorbell Knock ---
	if strings.Contains(xml, "<ST_KNOCK") {
		roomID := extractAttr(xml, "roomID")
		s.handleDoorbellKnock(player, roomID)
		return
	}

	// --- Studio Doorbell Response (Admit / Deny) ---
	if strings.Contains(xml, "<ST_KNOCK_RES") {
		target := extractAttr(xml, "target")
		allow := extractAttr(xml, "allow")
		s.handleDoorbellResponse(player, target, allow == "1" || allow == "true")
		return
	}

	// --- Studio Moderation Kick ---
	if strings.Contains(xml, "<ST_KICK") || strings.Contains(xml, "<AV_K") {
		target := extractAttr(xml, "target")
		if target == "" {
			target = extractAttr(xml, "av")
		}
		s.handleStudioKick(player, target)
		return
	}

	// --- V-ego San Sumo Game Actions ---
	if strings.Contains(xml, "<Sumo.MoveSelected") {
		moveStr := extractAttr(xml, "move")
		moveInt, _ := strconv.Atoi(moveStr)
		if s.sumoManager != nil {
			state, res, err := s.sumoManager.SubmitMove(player.ScreenName, sumo.Move(moveInt))
			if err == nil && state != nil {
				respXML := fmt.Sprintf("<Sumo.RoundResult round=\"%d\" p1=\"%s\" p2=\"%s\" p1_score=\"%d\" p2_score=\"%d\" res=\"%d\" over=\"%v\" winner=\"%s\"/>",
					state.CurrentRound, state.Player1Name, state.Player2Name, state.Player1Score, state.Player2Score, int(res), state.GameOver, state.WinnerName)
				_ = player.Send(respXML)
			}
		}
		return
	}

	if strings.Contains(xml, "<Lobby.StartGame") || strings.Contains(xml, "<Sumo.JoinQueue") {
		if s.sumoManager != nil {
			sess := s.sumoManager.JoinQueue(player)
			if sess != nil {
				startXML := fmt.Sprintf("<Room.StartGameAccepted roomID=\"%s\" p1=\"%s\" p2=\"%s\"/>",
					sess.ID, sess.Match.Player1Name, sess.Match.Player2Name)
				if sess.P1 != nil {
					_ = sess.P1.Send(startXML)
				}
				if sess.P2 != nil {
					_ = sess.P2.Send(startXML)
				}
			} else {
				_ = player.Send("<Sumo.QueueWaiting/>")
			}
		}
		return
	}

	if strings.Contains(xml, "<Sumo.GetDecibels") {
		if d, _ := s.store.GetDenizenByName(player.ScreenName); d != nil {
			_ = player.Send(fmt.Sprintf("<Sumo.Decibels val=\"%d\"/>", d.Decibels))
		}
		return
	}

	// --- Trading System (TD_ST, TD_AD, TD_AG, TD_DAG, TD_CNCL, TD_CN) ---
	if strings.Contains(xml, "<TD_ST") {
		target := extractAttr(xml, "tde")
		if target == "" {
			target = extractAttr(xml, "t")
		}
		if target == "" {
			target = extractAttr(xml, "tdr")
		}
		if s.tradeManager != nil && target != "" {
			_, err := s.tradeManager.StartTrade(player.RoomID, player.ScreenName, target)
			if err != nil {
				log.Printf("[TRADE] StartTrade failed: %v", err)
			}
		}
		return
	}

	if strings.Contains(xml, "<TD_AD") {
		idStr := extractAttr(xml, "id")
		posID, _ := strconv.Atoi(idStr)
		if s.tradeManager != nil && posID > 0 {
			err := s.tradeManager.AddItem(player.ScreenName, posID)
			if err != nil {
				log.Printf("[TRADE] AddItem failed: %v", err)
			}
		}
		return
	}

	if strings.Contains(xml, "<TD_AG") {
		if s.tradeManager != nil {
			_, err := s.tradeManager.AgreeTrade(player.ScreenName)
			if err != nil {
				log.Printf("[TRADE] AgreeTrade failed: %v", err)
			}
		}
		return
	}

	if strings.Contains(xml, "<TD_DAG") {
		if s.tradeManager != nil {
			err := s.tradeManager.DisagreeTrade(player.ScreenName)
			if err != nil {
				log.Printf("[TRADE] DisagreeTrade failed: %v", err)
			}
		}
		return
	}

	if strings.Contains(xml, "<TD_CN") || strings.Contains(xml, "<TD_CNCL") {
		if s.tradeManager != nil {
			_ = s.tradeManager.CancelTrade(player.ScreenName)
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
		if d, _ := s.authenticatedDenizen(r); d != nil {
			user = d.ScreenName
		}
	}
	if user == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "user identity required"})
		return
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
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "EDL sequence required"})
		return
	}

	songID, err := s.store.SaveSong(user, title, edl)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	newDB, err := s.store.AdjustBalance(user, -10, 0)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	cdProps := map[string]any{
		"songId":    float64(songID),
		"songName":  title,
		"ownerName": user,
		"artist":    user,
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
		"song_id":       songID,
		"song_name":     title,
		"owner":         user,
		"artist":        user,
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
	authD, _ := s.authenticatedDenizen(r)
	user := r.FormValue("user")
	if user == "" {
		user = r.URL.Query().Get("user")
	}
	if user == "" && authD != nil {
		user = authD.ScreenName
	}
	if authD != nil && user != "" && !strings.EqualFold(authD.ScreenName, user) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "Forbidden: cannot impersonate other sender"})
		return
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
	authD, _ := s.authenticatedDenizen(r)
	user := r.URL.Query().Get("user")
	if user == "" && authD != nil {
		user = authD.ScreenName
	}
	if user == "" {
		user = "Adam"
	}
	if authD != nil && !strings.EqualFold(authD.ScreenName, user) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "Forbidden: cannot view other denizen's inbox"})
		return
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
	authD, _ := s.authenticatedDenizen(r)
	studioID := r.FormValue("studioId")
	if studioID == "" {
		studioID = r.URL.Query().Get("studioId")
	}
	if studioID == "" {
		studioID = "1"
	}

	// Verify studio ownership if auth is present and studio is private
	if authD != nil && strings.HasPrefix(studioID, "studio_") {
		st, err := s.store.GetStudio(studioID)
		if err == nil && st != nil && !st.IsPublic {
			if !strings.EqualFold(st.OwnerName, authD.ScreenName) {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "Forbidden: you do not own this studio"})
				return
			}
		}
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
	if rm != nil {
		rm.mu.Lock()
		rm.Wallpaper = wp
		rm.Floor = fl
		rm.Doorbell = db
		rm.mu.Unlock()
		decorXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><ST_DECOR wp=\"%d\" fl=\"%d\" db=\"%d\"/></Room.Action>",
			studioID, wp, fl, db)
		go rm.Broadcast(decorXML, "")
	}

	json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"studio_id": studioID,
		"wallpaper": wp,
		"floor":     fl,
		"doorbell":  db,
	})
}

// startPerformance initiates an authoritative DJ performance using calculated sequence duration
func (s *Server) startPerformance(room *Room, dj, title, edl string) {
	_, _ = s.StartPerformance(room, dj, dj, title, edl)
}

func (s *Server) stopPerformance(room *Room) {
	_, _ = s.SettlePerformance(room)
}

func (s *Server) recordVote(room *Room, voter string, delta int) {
	if err := s.RecordPerformanceVote(room, voter, delta); err != nil {
		log.Printf("[VOTE] Vote by %s in %s rejected: %v", voter, room.ID, err)
		return
	}

	room.mu.RLock()
	session := room.Performance
	if session == nil {
		room.mu.RUnlock()
		return
	}
	cheers := session.Cheers
	boos := session.Boos
	room.mu.RUnlock()

	var voteXML string
	if delta > 0 {
		voteXML = fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_TU av=\"%s\"/></Room.Action>", room.ID, voter)
	} else {
		voteXML = fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_TD av=\"%s\"/></Room.Action>", room.ID, voter)
	}
	room.Broadcast(voteXML, "")

	vrXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><CD_VR tu=\"%d\" td=\"%d\"/></Room.Action>",
		room.ID, cheers, boos)
	room.Broadcast(vrXML, "")
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

func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "sources", "cokephase")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir || parent == "" {
			break
		}
		dir = parent
	}
	return "."
}

func (s *Server) handleTop40(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "daily"
	}
	list, err := s.store.GetTop40(period)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleVegoSanMove(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	user := r.URL.Query().Get("user")
	if user == "" {
		user = "Adam"
	}
	move := r.URL.Query().Get("move")
	if move == "" && r.Method == http.MethodPost {
		_ = r.ParseForm()
		move = r.FormValue("move")
		if u := r.FormValue("user"); u != "" {
			user = u
		}
	}
	res, err := s.store.ProcessVegoSanMove(user, move)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleJsonRoom(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = r.ParseForm()
	idStr := r.FormValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	roomNames := []string{"Studio 1", "Studio 2", "Studio 3", "Studio 4", "Studio Rooftop", "Wayne Enterprise", "Pokemon"}
	roomTypes := []string{"studio_model_a", "studio_model_b", "studio_model_c", "studio_model_d", "studio_model_rooftop", "studio_wayne", "pokemon"}
	id, _ := strconv.Atoi(idStr)
	if id < 1 || id > len(roomNames) {
		id = 2
	}
	resp := map[string]string{
		"room_id":    strconv.Itoa(id),
		"room_name":  roomNames[id-1],
		"room_owner": "Coke Studios",
		"room_type":  roomTypes[id-1],
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleJsonCatalog(cokephaseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_ = r.ParseForm()
		if r.FormValue("page-number") != "" || r.URL.Query().Get("page-number") != "" {
			w.Write([]byte(`{"pages":15}`))
			return
		}
		page := r.FormValue("page")
		if page == "" {
			page = r.URL.Query().Get("page")
		}
		if page == "" {
			page = "1"
		}
		catFile := filepath.Join(cokephaseDir, "jsonreply", "catalog", fmt.Sprintf("page-%s.json", page))
		content, err := os.ReadFile(catFile)
		if err != nil {
			w.Write([]byte(`{}`))
			return
		}
		w.Write(content)
	}
}

func cleanProfanity(text string) string {
	badWords := []string{"fuck", "shit", "bitch", "cunt", "asshole", "dick"}
	res := text
	for _, bad := range badWords {
		re := regexp.MustCompile("(?i)\\b" + regexp.QuoteMeta(bad) + "\\b")
		res = re.ReplaceAllString(res, "blah")
	}
	return res
}

func obscureVowels(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch r {
		case 'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U':
			b.WriteRune('.')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
