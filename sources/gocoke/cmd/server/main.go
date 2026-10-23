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
	"strconv"
	"strings"
	"sync"
	"time"

	"gocoke/internal/client/net/amf"
)

// Standalone Coke Studios Server
// Implements:
// 1. Static asset server (WebAssembly client, Lingo assets, sounds, fonts)
// 2. HTTP Flash Remoting AMF0 gateway (/sf/gateway)
// 3. Denizen XML authentication (/sf/ssldocs/loginDenizen.jsp)
// 4. WebSocket realtime multiplayer game engine (/ws/realtime)

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

type Room struct {
	ID        string
	Name      string
	Layout    int
	mu        sync.RWMutex
	Players   map[string]*Player
	Furniture []string // POS elements
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
	nextPosID int
}

func NewServer(webRoot, host, port string) *Server {
	s := &Server{
		Rooms:     make(map[string]*Room),
		Players:   make(map[string]*Player),
		webRoot:   webRoot,
		host:      host,
		port:      port,
		nextPosID: 2000,
	}

	// Initialize default public rooms
	s.Rooms["$LOBBY$"] = NewRoom("$LOBBY$", "Lobby", 0)
	s.Rooms["VIS$0"] = NewRoom("VIS$0", "Central Park Studio", 3)
	s.Rooms["VIS$1"] = NewRoom("VIS$1", "San Francisco Studio", 3)
	s.Rooms["VIS$2"] = NewRoom("VIS$2", "London Studio", 3)
	s.Rooms["studio_1"] = NewRoom("studio_1", "Private Studio #1", 3)
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
	flag.Parse()

	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		host = "127.0.0.1"
		port = "8080"
	}
	if host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}

	srv := NewServer(*root, host, port)

	mux := http.NewServeMux()

	// 1. Denizen login JSP
	mux.HandleFunc("/sf/ssldocs/loginDenizen.jsp", srv.handleLoginDenizen)

	// 2. Flash Remoting AMF0 gateway
	mux.HandleFunc("/sf/gateway", srv.handleAMFGateway)

	// 3. WebSocket Realtime Game Server
	mux.HandleFunc("/ws/realtime", srv.handleWebSocketRealtime)

	// 4. Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","mode":"standalone-server","project":"gocoke"}`))
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
	log.Printf(" Coke Studios Standalone Server (Debian / Linux First)")
	log.Printf(" Listening on http://%s (Serving %s)", *addr, *root)
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
		avatarDef = "ch=01/01/01&amp;fa=01/01/01&amp;cl=01/01/01"
	} else {
		avatarDef = strings.ReplaceAll(avatarDef, "&", "&amp;")
	}
	mission := q.Get("mission")
	if mission == "" {
		mission = "Coke Music Reborn"
	}

	secret := "coke_secret_" + strconv.FormatInt(time.Now().Unix(), 16)

	xmlReply := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<LoginResult>
  <ErrorCode>0</ErrorCode>
  <ErrorMessage></ErrorMessage>
  <Gender>%s</Gender>
  <AvatarDefinition>%s</AvatarDefinition>
  <ScreenName>%s</ScreenName>
  <MissionStatement>%s</MissionStatement>
  <LastSeenIn>VIS$0</LastSeenIn>
  <Banned>0</Banned>
  <Secret>%s</Secret>
</LoginResult>`, gender, avatarDef, screenName, mission, secret)

	w.Header().Set("Content-Type", "text/xml;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(xmlReply))
	log.Printf("[HTTP] LoginDenizen authenticated: %s (Gender: %s, Secret: %s)", screenName, gender, secret)
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
		responseValue = "Welcome to Coke Studios! Click any tile to walk, open the catalogue to decorate, or open the mixer to create beats."

	case strings.Contains(target, "getDenizenBalance"):
		// Rich starting balance of 50,000 Decibels!
		responseValue = float64(50000)

	case strings.Contains(target, "getBackpack"):
		// Return backpack with starter items
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
					},
				},
			},
		}

	case strings.Contains(target, "prePurchaseItem"):
		responseValue = map[string]any{
			"balance":       float64(50000),
			"price":         float64(25),
			"catalogItemId": float64(1),
			"properties":    "[:]",
		}

	case strings.Contains(target, "purchaseItem"):
		s.mu.Lock()
		s.nextPosID++
		newID := s.nextPosID
		s.mu.Unlock()
		responseValue = []any{
			map[string]any{
				"id":          float64(newID),
				"catalogItem": float64(1),
				"properties":  map[string]any{},
			},
		}

	case strings.Contains(target, "getAllPrivateStudios") || strings.Contains(target, "getMyStudios") || strings.Contains(target, "getFeaturedStudios"):
		responseValue = []any{
			map[string]any{
				"studioId":   "VIS$0",
				"name":       "Central Park Studio",
				"layout":     float64(3),
				"owner":      "Coke Studios",
				"userCount":  float64(len(s.GetOrCreateRoom("VIS$0").Players)),
				"maxUsers":   float64(25),
				"type":       "studio_model_c",
			},
			map[string]any{
				"studioId":   "VIS$1",
				"name":       "San Francisco Studio",
				"layout":     float64(3),
				"owner":      "Coke Studios",
				"userCount":  float64(len(s.GetOrCreateRoom("VIS$1").Players)),
				"maxUsers":   float64(25),
				"type":       "studio_model_c",
			},
			map[string]any{
				"studioId":   "VIS$2",
				"name":       "London Studio",
				"layout":     float64(3),
				"owner":      "Coke Studios",
				"userCount":  float64(len(s.GetOrCreateRoom("VIS$2").Players)),
				"maxUsers":   float64(25),
				"type":       "studio_model_c",
			},
			map[string]any{
				"studioId":   "studio_1",
				"name":       "Private Studio #1",
				"layout":     float64(3),
				"owner":      "Adam",
				"userCount":  float64(len(s.GetOrCreateRoom("studio_1").Players)),
				"maxUsers":   float64(25),
				"type":       "studio_model_c",
			},
		}

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

		// Frame format: sha1(xml + trip + secret) + " " + trip + " " + xml + "\x00"
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
		}
		log.Printf("[WS] Player Login: %s (msgID: %s)", player.ScreenName, msgID)
		_ = player.Send(fmt.Sprintf("<Accepted msgID=\"%s\"/>", msgID))
		return
	}

	if strings.Contains(xml, "<Room.Enter") {
		msgID := extractAttr(xml, "msgID")
		roomID := extractAttr(xml, "roomID")
		log.Printf("[WS] Player %s entering room %s (msgID: %s)", player.ScreenName, roomID, msgID)

		// Acknowledge enter
		_ = player.Send(fmt.Sprintf("<Accepted msgID=\"%s\"/>", msgID))

		if roomID == "$LOBBY$" {
			player.RoomID = "$LOBBY$"
			return
		}

		// Studio room enter
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
				avXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV av=\"%s\" x=\"%d\" z=\"%d\" d=\"%d\" g=\"%d\" m=\"%s\" def=\"%s\" act=\"std\"/></Room.Action>",
					room.ID, other.ScreenName, other.X, other.Z, other.Dir, other.Gender, other.Mission, other.Definition)
				_ = player.Send(avXML)
			}
		}
		// Send existing furniture to this player
		for _, posXML := range room.Furniture {
			_ = player.Send(fmt.Sprintf("<Room.Action roomID=\"%s\">%s</Room.Action>", room.ID, posXML))
		}
		room.mu.RUnlock()

		room.mu.Lock()
		room.Players[player.ID] = player
		room.mu.Unlock()
		return
	}

	if strings.Contains(xml, "<AV_EN") {
		// Player entered and is ready to appear
		roomID := player.RoomID
		if roomID == "" || roomID == "$LOBBY$" {
			return
		}
		room := s.GetOrCreateRoom(roomID)
		avXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV av=\"%s\" x=\"%d\" z=\"%d\" d=\"%d\" g=\"%d\" m=\"%s\" def=\"%s\" act=\"std\"/></Room.Action>",
			room.ID, player.ScreenName, player.X, player.Z, player.Dir, player.Gender, player.Mission, player.Definition)
		room.Broadcast(avXML, "")
		log.Printf("[WS] Player %s spawned in room %s", player.ScreenName, roomID)
		return
	}

	if strings.Contains(xml, "<AV_GSP") {
		// Path movement: <AV_GSP><P z=".." x=".."/>...</AV_GSP>
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
		// Direct movement request
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
		// Chat packet: <CH sm="speak" m="HEX"/>
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

	if strings.Contains(xml, "<POS_STD") {
		// Furniture placement: <POS_STD st="0" d="2" ho="0" z="Z" y="Y" x="X" id="ID"/>
		posID := extractAttr(xml, "id")
		st := extractAttr(xml, "st")
		d := extractAttr(xml, "d")
		ho := extractAttr(xml, "ho")
		x := extractAttr(xml, "x")
		y := extractAttr(xml, "y")
		z := extractAttr(xml, "z")

		roomID := player.RoomID
		if roomID != "" {
			room := s.GetOrCreateRoom(roomID)
			posXML := fmt.Sprintf("<POS id=\"%s\" cid=\"45\" x=\"%s\" y=\"%s\" z=\"%s\" ho=\"%s\" d=\"%s\" st=\"%s\" iaot=\"false\"/>",
				posID, x, y, z, ho, d, st)
			room.mu.Lock()
			room.Furniture = append(room.Furniture, posXML)
			room.mu.Unlock()
			actionXML := fmt.Sprintf("<Room.Action roomID=\"%s\">%s</Room.Action>", room.ID, posXML)
			room.Broadcast(actionXML, "")
			log.Printf("[WS] Player %s placed furniture %s at (%s,%s,%s)", player.ScreenName, posID, x, y, z)
		}
		return
	}

	if strings.Contains(xml, "<POS_BKPK") {
		// Furniture pickup: <POS_BKPK id="ID"/>
		posID := extractAttr(xml, "id")
		roomID := player.RoomID
		if roomID != "" && posID != "" {
			room := s.GetOrCreateRoom(roomID)
			room.mu.Lock()
			var updated []string
			for _, f := range room.Furniture {
				if !strings.Contains(f, "id=\""+posID+"\"") {
					updated = append(updated, f)
				}
			}
			room.Furniture = updated
			room.mu.Unlock()

			rmXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><POS_RM id=\"%s\"/></Room.Action>", room.ID, posID)
			room.Broadcast(rmXML, "")
			log.Printf("[WS] Player %s picked up furniture %s", player.ScreenName, posID)
		}
		return
	}
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
