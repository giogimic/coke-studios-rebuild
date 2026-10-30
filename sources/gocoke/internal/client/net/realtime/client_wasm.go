//go:build js && wasm

package realtime

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	clientcrypto "gocoke/internal/client/crypto"
	studioscene "gocoke/internal/client/scene/studio"
	"html"
	"log"
	"strconv"
	"strings"
	"sync"
	"syscall/js"
	"time"
)

// Client implements the legacy Shockwave XMLSocket protocol over a browser WebSocket bridge.
// It frames messages as: sha1(xml + trip + secret) + " " + trip(7-width, space-padded) + " " + xml + "\x00".
// The server proxy at /ws/realtime relays to the TCP XMLSocket server.
type Client struct {
	mu     sync.Mutex
	trip   uint64
	msgID  uint64
	secret string
	closed bool

	// JS WebSocket
	ws         js.Value
	onOpenRef  js.Func
	onCloseRef js.Func
	onMsgRef   js.Func
	onErrRef   js.Func

	openCh chan struct{}
	errCh  chan error

	// Handshake state
	policySeen bool
	// Queue of XML payloads to send after policy is received
	pending []string

	// Ack tracking: map of msgID -> waiter channel
	pendingAcks map[uint64]chan struct{}

	logger *log.Logger

	// Navigation state: lobby -> studio
	navStep   int    // 0=idle, 1=awaiting exit accepted, 2=awaiting enter accepted
	navAwait  uint64 // msgID we are waiting for
	navTarget string // target studio roomID

	// Current room layout (from ST_EN). Type C observed as layout=3
	layout int

	// Current room/studio id for outgoing actions
	roomID string

	// Buffer POS packets until studio scene is active
	pendingPOS   []posPayload
	roomInbox      roomInbox
	dispatchPacket func([]byte)
	handlePacket   func([]byte) any

	// Movement send throttling
	lastMoveSend    time.Time
	minMoveInterval time.Duration
}

// posPayload holds a single POS element's attributes
type posPayload struct {
	posID  string
	prodID string
	x      int
	y      int
	z      int
	ho     int
	dir    int
	state  int
	iaot   bool
}

// New constructs a Client. Secret is the session Secret from loginDenizen.jsp.
func New(secret string, logger *log.Logger) *Client {
	return &Client{secret: secret, logger: logger, trip: 0, minMoveInterval: 650 * time.Millisecond}
}

// Connect opens a WebSocket to the bridge: /ws/realtime?host=HOST&port=PORT
// Uses window.location by default, but can be overridden via global bridgeEndpoint variable
func (c *Client) Connect(ctx context.Context, host string, port string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ws.Truthy() {
		return nil
	}

	// Check for custom bridge endpoint (allows hosting WASM on different domain than bridge)
	var url string
	if bridgeEndpoint := js.Global().Get("bridgeEndpoint"); !bridgeEndpoint.IsUndefined() {
		// Custom bridge endpoint provided (e.g., "wss://decibel.fun")
		baseURL := bridgeEndpoint.String()
		url = fmt.Sprintf("%s/ws/realtime?host=%s&port=%s", baseURL, host, port)
	} else {
		// Default: use same host as the WASM is served from
		win := js.Global().Get("window")
		loc := win.Get("location")
		protocol := loc.Get("protocol").String() // "http:" or "https:"
		wsScheme := "ws:"
		if protocol == "https:" {
			wsScheme = "wss:"
		}
		hostPort := loc.Get("host").String()
		url = fmt.Sprintf("%s//%s/ws/realtime?host=%s&port=%s", wsScheme, hostPort, host, port)
	}

	ws := js.Global().Get("WebSocket").New(url)
	ws.Set("binaryType", "arraybuffer")

	// init channels used to synchronize open/error
	c.openCh = make(chan struct{}, 1)
	c.errCh = make(chan error, 1)
	// reset handshake state
	c.policySeen = false
	c.pending = nil
	c.pendingAcks = make(map[uint64]chan struct{})
	c.trip = 0
	c.msgID = 0

	// onopen: signal readiness
	c.onOpenRef = js.FuncOf(func(this js.Value, args []js.Value) any {
		select {
		case c.openCh <- struct{}{}:
		default:
		}
		return nil
	})
	ws.Set("onopen", c.onOpenRef)

	// Internal packet dispatcher: parses and applies XML packets directly without deferral checks.
	c.dispatchPacket = func(b []byte) {
		// Detect cross-domain policy and flush any queued outbound messages
		if !c.policySeen {
			if containsSubstr(b, []byte("<cross-domain-policy")) {
				c.mu.Lock()
				c.policySeen = true
				queued := make([]string, len(c.pending))
				copy(queued, c.pending)
				c.pending = nil
				c.mu.Unlock()
				for _, xml := range queued {
					_ = c.sendFramed(xml)
				}
			}
		}
		// Detect <Accepted msgID="n"/> and resolve waiters
		if id, ok := parseAcceptedMsgID(b); ok {
			c.mu.Lock()
			ch := c.pendingAcks[id]
			if ch != nil {
				delete(c.pendingAcks, id)
				// signal outside of lock to avoid potential deadlocks
				c.mu.Unlock()
				select {
				case ch <- struct{}{}:
				default:
				}
			} else {
				c.mu.Unlock()
			}
			// After login is accepted (msgID==1), auto-enter the lobby with next msgID (expected 2)
			if id == 1 {
				nextID := c.nextMsgID()
				xml := fmt.Sprintf("<Room.Enter msgID=\"%d\" groupID=\"participants\" roomID=\"$LOBBY$\"/>", nextID)
				_ = c.sendFramed(xml)
			}

			// Navigation state machine: if we are performing lobby->studio transition
			c.mu.Lock()
			nextStep := c.navStep
			expected := c.navAwait
			target := c.navTarget
			c.mu.Unlock()
			if expected != 0 && id == expected {
				if nextStep == 1 {
					// Exit accepted; send Room.Enter for target
					enterID := c.nextMsgID()
					enterXML := fmt.Sprintf("<Room.Enter msgID=\"%d\" groupID=\"participants\" roomID=\"%s\"/>", enterID, target)
					_ = c.sendFramed(enterXML)
					c.mu.Lock()
					c.navStep = 2
					c.navAwait = enterID
					c.mu.Unlock()
				} else if nextStep == 2 {
					// Enter accepted; send AV_EN action and clear state
					actionXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV_EN/></Room.Action>", target)
					_ = c.sendFramed(actionXML)
					c.mu.Lock()
					c.roomID = target
					c.navStep = 0
					c.navAwait = 0
					c.navTarget = ""
					c.mu.Unlock()
				}
			}
		}

		// ST_EN received (room entered)
		if containsSubstr(b, []byte("<ST_EN")) {
			if c.logger != nil {
				c.logger.Printf("Realtime ST_EN received")
			}
			// Parse layout attribute if present
			if lt, ok := parseXMLAttrInt(b, "layout"); ok {
				c.layout = lt
				if c.logger != nil {
					c.logger.Printf("Realtime ST_EN: layout=%d", lt)
				}
			}
			// Remember current roomID if not already set
			c.mu.Lock()
			if c.roomID == "" && c.navTarget != "" {
				c.roomID = c.navTarget
			}
			c.mu.Unlock()
		}

		// Decode the same escaped profile used for initial spawn and late joins.
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<AV ")) {
			avatars, err := DecodeAvatarPackets(b)
			if err != nil {
				if c.logger != nil {
					c.logger.Printf("Realtime invalid avatar packet: %v", err)
				}
			} else if sc := studioscene.GetActiveScene(); sc != nil {
				for _, av := range avatars {
					if strings.TrimSpace(av.Name) == "" {
						continue
					}
					if av.X != nil && av.Z != nil {
						row, col := c.mapServerToClient(*av.X, *av.Z)
						sc.EnqueueAV(av.Name, av.Definition, av.Mission, av.Gender, av.Direction, row, col, false)
					} else if strings.HasPrefix(av.Action, "dnc") || av.Action == "spk" || av.Action == "" || av.Action == "drk" || av.Action == "mcp" {
						sc.EnqueueAVAction(av.Name, av.Action, av.Direction)
					}
				}
			}
		}
		// If this is a furniture POS payload, parse and enqueue placement
		if containsSubstr(b, []byte("<POS ")) {
			// Extract all <POS ...> start tags and enqueue each
			parseAttrIntIn := func(seg []byte, attr string) (int, bool) {
				key := []byte(attr + "=\"")
				for i := 0; i+len(key) <= len(seg); i++ {
					// Ensure attribute name starts at a boundary (start, space, or '<')
					if !(i == 0 || seg[i-1] == ' ' || seg[i-1] == '<') {
						continue
					}
					if string(seg[i:i+len(key)]) == string(key) {
						j := i + len(key)
						start := j
						for j < len(seg) && seg[j] != '"' {
							j++
						}
						if j > start {
							if n, err := strconv.Atoi(string(seg[start:j])); err == nil {
								return n, true
							}
						}
						break
					}
				}
				return 0, false
			}
			parseAttrStrIn := func(seg []byte, attr string) string {
				key := []byte(attr + "=\"")
				for i := 0; i+len(key) <= len(seg); i++ {
					// Ensure attribute name starts at a boundary (start, space, or '<')
					if !(i == 0 || seg[i-1] == ' ' || seg[i-1] == '<') {
						continue
					}
					if string(seg[i:i+len(key)]) == string(key) {
						j := i + len(key)
						start := j
						for j < len(seg) && seg[j] != '"' {
							j++
						}
						if j > start {
							return string(seg[start:j])
						}
						break
					}
				}
				return ""
			}
			// scan
			for i := 0; i+5 < len(b); {
				if b[i] == '<' && i+5 < len(b) && b[i+1] == 'P' && b[i+2] == 'O' && b[i+3] == 'S' && b[i+4] == ' ' {
					// find end of start tag '>'
					j := i
					for j < len(b) && b[j] != '>' {
						j++
					}
					if j >= len(b) {
						break
					}
					seg := b[i:j]
					posID := parseAttrStrIn(seg, "id")
					cid := parseAttrStrIn(seg, "cid")
					x, okX := parseAttrIntIn(seg, "x")
					y, _ := parseAttrIntIn(seg, "y")
					z, okZ := parseAttrIntIn(seg, "z")
					ho, _ := parseAttrIntIn(seg, "ho")
					d, _ := parseAttrIntIn(seg, "d")
					st, _ := parseAttrIntIn(seg, "st")
					iaotStr := parseAttrStrIn(seg, "iaot")
					ia := strings.EqualFold(strings.TrimSpace(iaotStr), "true")

					// If this POS carries a Lingo property list body, extract it now: [#texture: N, #color: M]
					// Find end tag for this POS element
					endTag := []byte("</POS>")
					bodyStart := j + 1
					bodyEnd := -1
					if k := bytes.Index(b[bodyStart:], endTag); k != -1 {
						bodyEnd = bodyStart + k
					}

					if okX && okZ && strings.TrimSpace(cid) != "" {
						// Handle wallpaper/floor update packets (cid 1 = wallpaper, cid 2 = floor)
						if cid == "1" || cid == "2" {
							textureIdx, colorIdx := 0, 0
							if bodyEnd != -1 {
								// Extract body between > and </POS>
								body := b[bodyStart:bodyEnd]
								// crude parse: look for #texture: N and #color: N
								if m := bytes.Index(body, []byte("#texture:")); m != -1 {
									p := m + len("#texture:")
									for p < len(body) && (body[p] == ' ' || body[p] == '\t') {
										p++
									}
									q := p
									for q < len(body) && body[q] >= '0' && body[q] <= '9' {
										q++
									}
									if v, err := strconv.Atoi(string(body[p:q])); err == nil {
										textureIdx = v
									}
								}
								if m := bytes.Index(body, []byte("#color:")); m != -1 {
									p := m + len("#color:")
									for p < len(body) && (body[p] == ' ' || body[p] == '\t') {
										p++
									}
									q := p
									for q < len(body) && body[q] >= '0' && body[q] <= '9' {
										q++
									}
									if v, err := strconv.Atoi(string(body[p:q])); err == nil {
										colorIdx = v
									}
								}
							}
							if sc := studioscene.GetActiveScene(); sc != nil {
								if c.logger != nil {
									c.logger.Printf("Realtime POS style: cid=%s texture=%d color=%d", cid, textureIdx, colorIdx)
								}
								fmt.Printf("Realtime POS style: cid=%s texture=%d color=%d\n", cid, textureIdx, colorIdx)
								if cid == "1" {
									sc.ApplyWallpaperFromNetwork(textureIdx, colorIdx)
								} else {
									sc.ApplyFloorFromNetwork(textureIdx, colorIdx)
								}
							}
						} else {
							payload := posPayload{posID: posID, prodID: cid, x: x, y: y, z: z, ho: ho, dir: d, state: st, iaot: ia}
							if c.logger != nil {
								c.logger.Printf("Realtime POS parsed: id=%s cid=%s x=%d y=%d z=%d ho=%d d=%d st=%d iaot=%t", posID, cid, x, y, z, ho, d, st, ia)
							}
							if sc := studioscene.GetActiveScene(); sc != nil {
								sc.EnqueuePOS(payload.posID, payload.prodID, payload.x, payload.y, payload.z, payload.ho, payload.dir, payload.state, payload.iaot)
							} else {
								// buffer until scene becomes active
								c.mu.Lock()
								c.pendingPOS = append(c.pendingPOS, payload)
								c.mu.Unlock()
							}
						}
					}
					i = j + 1
					continue
				}
				i++
			}
		}

		// If scene is active, flush any buffered POS
		if sc := studioscene.GetActiveScene(); sc != nil {
			c.mu.Lock()
			pending := c.pendingPOS
			c.pendingPOS = nil
			c.mu.Unlock()
			for _, p := range pending {
				sc.EnqueuePOS(p.posID, p.prodID, p.x, p.y, p.z, p.ho, p.dir, p.state, p.iaot)
			}
		}
		// If this is a chat message payload (CH), decode and emit chat bubble
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<CH ")) {
			// Parse CH packet: <CH av="Jesus" sm="1" m="7465737432"/>
			// sm="1" = speak, sm="2" = shout
			// m is binhex encoded message

			// Use same local parsing approach as AV parsing
			parseAttrStrIn := func(seg []byte, attr string) string {
				key := []byte(attr + "=\"")
				for i := 0; i+len(key) <= len(seg); i++ {
					if !(i == 0 || seg[i-1] == ' ' || seg[i-1] == '<') {
						continue
					}
					if string(seg[i:i+len(key)]) == string(key) {
						j := i + len(key)
						start := j
						for j < len(seg) && seg[j] != '"' {
							j++
						}
						if j > start {
							return string(seg[start:j])
						}
						break
					}
				}
				return ""
			}

			// Scan for <CH tags
			for i := 0; i+4 < len(b); {
				if b[i] == '<' && b[i+1] == 'C' && b[i+2] == 'H' && b[i+3] == ' ' {
					// Find end of the start tag
					j := i
					for j < len(b) && b[j] != '>' && b[j] != '/' {
						j++
					}
					if j >= len(b) {
						break
					}
					seg := b[i:j]

					name := parseAttrStrIn(seg, "av")
					sm := parseAttrStrIn(seg, "sm")
					to := parseAttrStrIn(seg, "to")
					encodedMsg := parseAttrStrIn(seg, "m")

					if c.logger != nil {
						c.logger.Printf("Realtime CH raw: name=%q sm=%s encoded=%q", name, sm, encodedMsg)
					}

					if name != "" && encodedMsg != "" {
						// Decode binhex message (hex string to bytes)
						decoded, err := hex.DecodeString(encodedMsg)
						if err != nil {
							if c.logger != nil {
								c.logger.Printf("Realtime CH decode error: %v", err)
							}
						} else {
							message := string(decoded)
							if sc := studioscene.GetActiveScene(); sc != nil {
								speechMode := 1 // default to speak
								if sm == "2" {
									speechMode = 2 // shout
								} else if sm == "0" || sm == "whisper" {
									speechMode = 0 // whisper
									if to != "" && !strings.EqualFold(name, sc.GetCurrentUserName()) {
										message = "(whisper): " + message
									} else if to != "" {
										message = "(to " + to + "): " + message
									}
								} else if sm == "3" || sm == "sing" {
									speechMode = 3 // sing
								}
								sc.EnqueueChat(name, message, speechMode)
								if c.logger != nil {
									c.logger.Printf("Realtime CH parsed: name=%q sm=%s msg=%q", name, sm, message)
								}
							}
						}
					}
					i = j + 1
					continue
				}
				i++
			}
		}

		// If this is a CD play payload (CD_P), parse and trigger studio playback
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<CD_P ")) {
			// Example:
			// <Room.Action roomID="ADAM$0"><CD_P av="Jesus" a="MajorTom77" sn="Get Daft" s="..." catId="89" cdPlId="526"/></Room.Action>
			avatar := parseXMLAttrString(b, "av")
			artist := parseXMLAttrString(b, "a")
			songName := parseXMLAttrString(b, "sn")
			edl := parseXMLAttrString(b, "s")
			if c.logger != nil {
				c.logger.Printf("Realtime IN CD_P: av=%q artist=%q song=%q edlLen=%d", avatar, artist, songName, len(edl))
			}
			if sc := studioscene.GetActiveScene(); sc != nil {
				offsetMS, _ := parseXMLAttrInt(b, "offset")
				sc.HandleCDPlayAt(avatar, artist, songName, edl, time.Duration(offsetMS)*time.Millisecond)
				// Record player avatar for self-button hiding BEFORE showing
				if sc.InfoStand() != nil {
					sc.InfoStand().SetNowPlayingPlayer(avatar)
				}
				// Always show Now Playing vote infostand during a performance
				sc.ShowNowPlayingInfo(songName, artist)
			}
		}

		// If this is a thumbs up payload (CD_TU), parse avatar and emit overlay
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<CD_TU")) {
			avatar := parseXMLAttrString(b, "av")
			if c.logger != nil {
				c.logger.Printf("Realtime IN CD_TU: av=%q", avatar)
			}
			if sc := studioscene.GetActiveScene(); sc != nil {
				sc.EmitThumbOverlay(avatar, true)
			}
		}

		// If this is a thumbs down payload (CD_TD), parse avatar and emit overlay
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<CD_TD")) {
			avatar := parseXMLAttrString(b, "av")
			if c.logger != nil {
				c.logger.Printf("Realtime IN CD_TD: av=%q", avatar)
			}
			if sc := studioscene.GetActiveScene(); sc != nil {
				sc.EmitThumbOverlay(avatar, false)
			}
		}

		// If this is a performance vote result payload (CD_VR), parse counts and show alert
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<CD_VR")) {
			thumbsUp, _ := parseXMLAttrInt(b, "tur")
			thumbsDown, _ := parseXMLAttrInt(b, "tdr")
			pointsAwarded, _ := parseXMLAttrInt(b, "pa")
			if c.logger != nil {
				c.logger.Printf("Realtime IN CD_VR: tur=%d tdr=%d pa=%d", thumbsUp, thumbsDown, pointsAwarded)
			}
			if sc := studioscene.GetActiveScene(); sc != nil {
				sc.ShowPerformanceResults(thumbsUp, thumbsDown, pointsAwarded)
			}
		}

		// If this is a performance reaction summary (CD_PRS), parse and play reaction sample
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<CD_PRS")) {
			thumbsUp, _ := parseXMLAttrInt(b, "tur")
			thumbsDown, _ := parseXMLAttrInt(b, "tdr")
			if c.logger != nil {
				c.logger.Printf("Realtime IN CD_PRS: tur=%d tdr=%d", thumbsUp, thumbsDown)
			}
			if sc := studioscene.GetActiveScene(); sc != nil {
				sc.HandleCDPerformanceResult(thumbsUp, thumbsDown)
			}
		}

		// If this is a CD stop/leave payload (CD_S or CD_E), stop audio and hide now playing infostand
		if containsSubstr(b, []byte("<Room.Action")) && (containsSubstr(b, []byte("<CD_S")) || containsSubstr(b, []byte("<CD_E"))) {
			if sc := studioscene.GetActiveScene(); sc != nil {
				sc.StopStereo()
				sc.HideNowPlayingInfo()
			}
		}
		// If this is a movement payload (AV_MV), enqueue a move
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<AV_MV")) {
			serverX, okX := parseXMLAttrInt(b, "x")
			serverZ, okZ := parseXMLAttrInt(b, "z")
			name := parseXMLAttrString(b, "av")
			if okX && okZ && name != "" {
				cx, cy := c.mapServerToClient(serverX, serverZ)
				if sc := studioscene.GetActiveScene(); sc != nil {
					// Swap to match engine's expectations observed in logs: row=cx, col=cy
					sc.EnqueueMove(name, cx, cy)
				}
			}
		}
		// If this is an avatar remove (AV_RM), enqueue removal
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<AV_RM")) {
			name := parseXMLAttrString(b, "av")
			if name != "" {
				if sc := studioscene.GetActiveScene(); sc != nil {
					sc.EnqueueRemoveAvatar(name)
				}
			}
		}
		// If this is a trade display update (TD_DISPL)
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<TD_DISPL")) {
			// Parse: <TD_DISPL tdr="Jesus" tde="MajorTom77" tdr_a="false" tde_a="false" tdr_i="" tde_i="45"/>
			traderName := parseXMLAttrString(b, "tdr")
			tradeeName := parseXMLAttrString(b, "tde")
			traderItems := parseXMLAttrString(b, "tdr_i")
			tradeeItems := parseXMLAttrString(b, "tde_i")
			traderAccepts := parseXMLAttrString(b, "tdr_a") == "true"
			tradeeAccepts := parseXMLAttrString(b, "tde_a") == "true"

			if sc := studioscene.GetActiveScene(); sc != nil {
				sc.UpdateTradeDisplay(traderName, tradeeName, traderItems, tradeeItems, traderAccepts, tradeeAccepts)
			}
		}
		// If this is a drink dispenser response or avatar action (AV_DC)
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<AV_DC")) {
			// Parse: <Room.Action roomID="ADAM$4"><AV_DC av="Adam" pts="10"/></Room.Action>
			avatarName := parseXMLAttrString(b, "av")
			ptsStr := parseXMLAttrString(b, "pts")
			actStr := parseXMLAttrString(b, "act")
			itemStr := parseXMLAttrString(b, "item")
			decibels := 0
			if pts, err := strconv.Atoi(ptsStr); err == nil {
				decibels = pts
			}

			if c.logger != nil {
				c.logger.Printf("Realtime IN AV_DC: avatar=%s act=%s item=%s decibels=%d", avatarName, actStr, itemStr, decibels)
			}

			if sc := studioscene.GetActiveScene(); sc != nil {
				if actStr != "" {
					sc.ApplyAvatarAction(avatarName, actStr, -1)
				}
				if decibels > 0 || itemStr == "coke_can" || actStr == "drk" {
					sc.HandleDrinkDispenserResponse(avatarName, decibels)
				}
			}
		}
		if containsSubstr(b, []byte("<Alert ")) {
			if sc := studioscene.GetActiveScene(); sc != nil {
				sc.ShowServerAlert(parseXMLAttrString(b, "msg"))
			}
		}
		// If this is a CD request accepted (CD_RA), open choose song window
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<CD_RA")) {
			if c.logger != nil {
				c.logger.Printf("Realtime IN CD_RA: request accepted")
			}
			if sc := studioscene.GetActiveScene(); sc != nil {
				sc.HandleCDRequestAccepted()
			}
		}
		// If this is a trade cancellation (TD_CN or TD_CNCL)
		if containsSubstr(b, []byte("<TD_CN")) || containsSubstr(b, []byte("<TD_CNCL")) {
			if sc := studioscene.GetActiveScene(); sc != nil {
				sc.CancelTradeFromNetwork()
			}
		}

		// If this is a trade completion (TD_CPLT)
		if containsSubstr(b, []byte("<Room.Action")) && containsSubstr(b, []byte("<TD_CPLT")) {
			// Parse: <TD_CPLT><P id="349" cid="45">[:]</P>...</TD_CPLT>
			// Extract all P elements for received items
			receivedItems := make([]map[string]string, 0)

			// Find all <P> elements
			data := b
			for {
				pStart := bytes.Index(data, []byte("<P "))
				if pStart == -1 {
					break
				}
				pEnd := bytes.Index(data[pStart:], []byte("/>"))
				if pEnd == -1 {
					pEnd = bytes.Index(data[pStart:], []byte(">"))
					if pEnd == -1 {
						break
					}
				}
				pElement := data[pStart : pStart+pEnd+1]

				// Extract id and cid attributes
				id := parseXMLAttrString(pElement, "id")
				cid := parseXMLAttrString(pElement, "cid")

				if id != "" && cid != "" {
					receivedItems = append(receivedItems, map[string]string{
						"id":  id,
						"cid": cid,
					})
				}

				data = data[pStart+pEnd+1:]
			}

			if sc := studioscene.GetActiveScene(); sc != nil {
				sc.CompleteTradeFromNetwork(receivedItems)
			}
		}

		// Messenger events (friend requests, friend accept, instant messages)
		if containsSubstr(b, []byte("<MSGR_INV")) || containsSubstr(b, []byte("<MMS_REQ")) {
			sn := parseXMLAttrString(b, "sn")
			if sn == "" {
				sn = parseXMLAttrString(b, "from")
			}
			if sn != "" {
				if sc := studioscene.GetActiveScene(); sc != nil {
					sc.HandleIncomingFriendRequest(sn)
				}
			}
		}
		if containsSubstr(b, []byte("<MSGR_AD_F")) || containsSubstr(b, []byte("<MMS_ACC")) {
			sn := parseXMLAttrString(b, "sn")
			if sn == "" {
				sn = parseXMLAttrString(b, "from")
			}
			if sn != "" {
				if sc := studioscene.GetActiveScene(); sc != nil {
					sc.HandleFriendAccepted(sn)
				}
			}
		}
		if containsSubstr(b, []byte("<MSGR_SND_M")) || containsSubstr(b, []byte("<MMS_MSG")) {
			snd := parseXMLAttrString(b, "snd")
			if snd == "" {
				snd = parseXMLAttrString(b, "from")
			}
			msg := parseXMLAttrString(b, "msg")
			if snd != "" && msg != "" {
				if sc := studioscene.GetActiveScene(); sc != nil {
					sc.HandleIncomingMessengerMessage(snd, msg)
				}
			}
		}
		if c.logger != nil {
			c.logger.Printf("Realtime IN: %s", string(b))
		}
	}

	// Message handler: wraps dispatchPacket with panic recovery and room action deferral for loading scenes.
	c.handlePacket = func(b []byte) any {
		defer func() {
			if r := recover(); r != nil {
				if c.logger != nil {
					c.logger.Printf("Realtime onmessage recovered from panic: %v", r)
				}
			}
		}()
		if containsSubstr(b, []byte("<Room.Action")) {
			room := parseXMLAttrString(b, "roomID")
			active := ""
			if sc := studioscene.GetActiveScene(); sc != nil && sc.GetStudio() != nil {
				active = sc.GetStudio().StudioID
			}
			if room != "" && c.roomInbox.deferPacket(room, active, b) {
				return nil
			}
		}
		c.dispatchPacket(b)
		return nil
	}
	c.onMsgRef = js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			return nil
		}
		ev := args[0]
		data := ev.Get("data")
		// Convert ArrayBuffer → Uint8Array → Go []byte
		uint8 := js.Global().Get("Uint8Array").New(data)
		b := make([]byte, uint8.Get("length").Int())
		js.CopyBytesToGo(b, uint8)
		return c.handlePacket(b)
	})
	ws.Set("onmessage", c.onMsgRef)

	// onerror: surface error
	c.onErrRef = js.FuncOf(func(this js.Value, args []js.Value) any {
		var err error = fmt.Errorf("websocket error")
		select {
		case c.errCh <- err:
		default:
		}
		return nil
	})
	ws.Set("onerror", c.onErrRef)

	c.onCloseRef = js.FuncOf(func(this js.Value, args []js.Value) any {
		c.mu.Lock()
		c.closed = true
		c.ws = js.Undefined()
		c.mu.Unlock()
		return nil
	})
	ws.Set("onclose", c.onCloseRef)

	c.ws = ws
	c.closed = false

	// wait until open or context done
	c.mu.Unlock()
	defer c.mu.Lock()
	select {
	case <-c.openCh:
		return nil
	case err := <-c.errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close closes the underlying WebSocket.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.ws.Truthy() {
		c.ws.Call("close")
		if c.onMsgRef.IsUndefined() == false {
			c.onMsgRef.Release()
		}
		if c.onCloseRef.IsUndefined() == false {
			c.onCloseRef.Release()
		}
		if c.onOpenRef.IsUndefined() == false {
			c.onOpenRef.Release()
		}
		if c.onErrRef.IsUndefined() == false {
			c.onErrRef.Release()
		}
		c.ws = js.Undefined()
	}
	return nil
}

// SendLogin sends a System.Login request with binhex-encoded username and password (secret).
func (c *Client) SendLogin(username string, secret string) error {
	encUser := strings.ToUpper(hex.EncodeToString([]byte(username)))
	encPass := strings.ToUpper(hex.EncodeToString([]byte(secret)))
	id := c.nextMsgID()
	xml := fmt.Sprintf("<System.Login msgID=\"%d\"><Username encoding=\"text/binhex\">%s</Username><Password encoding=\"text/binhex\">%s</Password></System.Login>", id, encUser, encPass)
	return c.SendXML(xml)
}

// SendLoginAndWait sends System.Login and waits for <Accepted msgID> or context cancellation.
func (c *Client) SendLoginAndWait(ctx context.Context, username string, secret string) error {
	encUser := strings.ToUpper(hex.EncodeToString([]byte(username)))
	encPass := strings.ToUpper(hex.EncodeToString([]byte(secret)))
	id := c.nextMsgID()
	xml := fmt.Sprintf("<System.Login msgID=\"%d\"><Username encoding=\"text/binhex\">%s</Username><Password encoding=\"text/binhex\">%s</Password></System.Login>", id, encUser, encPass)
	return c.sendAndWait(ctx, xml, id)
}

// EnterRoom transitions from $LOBBY$ to the given room using proper sequence:
// Exit($LOBBY$) → Accepted → Enter(room) → Accepted → AV_EN.
func (c *Client) EnterRoom(roomID string) error {
	return c.BeginEnterStudioFromLobby(roomID)
}

// BeginEnterStudioFromLobby starts the lobby -> studio transition.
// It sends <Room.Exit roomID="$LOBBY$"/> and relies on onmessage Accepteds to proceed.
func (c *Client) BeginEnterStudioFromLobby(targetRoomID string) error {
	c.mu.Lock()
	if c.navStep != 0 {
		c.mu.Unlock()
		return fmt.Errorf("navigation already in progress")
	}
	c.roomInbox.begin(targetRoomID)
	c.navTarget = targetRoomID
	c.navStep = 1
	c.mu.Unlock()

	exitID := c.nextMsgID()
	c.mu.Lock()
	c.navAwait = exitID
	c.mu.Unlock()
	c.mu.Lock()
	previousRoom := c.roomID
	c.mu.Unlock()
	if previousRoom == "" {
		previousRoom = "$LOBBY$"
	}
	exitXML := fmt.Sprintf("<Room.Exit msgID=\"%d\" roomID=\"%s\"/>", exitID, previousRoom)
	return c.SendXML(exitXML)
}

// SendXML frames and writes a single XML message over WebSocket.
func (c *Client) SendXML(xml string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.ws.Truthy() {
		return fmt.Errorf("connection not open")
	}
	// ensure readyState == 1 (OPEN) to avoid 'CONNECTING' errors
	if rs := c.ws.Get("readyState").Int(); rs != 1 {
		return fmt.Errorf("websocket not open (readyState=%d)", rs)
	}
	// If policy not yet received, queue for later
	if !c.policySeen {
		c.pending = append(c.pending, xml)
		return nil
	}
	tripStr := fmt.Sprintf("%d", c.trip+1)
	if n := 7 - len(tripStr); n > 0 {
		tripStr = strings.Repeat(" ", n) + tripStr
	}
	sum := clientcrypto.Sha1(xml + tripStr + c.secret)
	framed := sum + " " + tripStr + " " + xml + "\x00"
	c.trip++

	// Send as binary Frame
	b := []byte(framed)
	uint8 := js.Global().Get("Uint8Array").New(len(b))
	_ = js.CopyBytesToJS(uint8, b)
	c.ws.Call("send", uint8)
	return nil
}

// sendFramed computes frame and sends without touching the pending queue.
func (c *Client) sendFramed(xml string) error {
	// No lock here; caller must ensure not racing with Close.
	if c.closed || !c.ws.Truthy() {
		return fmt.Errorf("connection not open")
	}
	tripStr := fmt.Sprintf("%d", c.trip+1)
	if n := 7 - len(tripStr); n > 0 {
		tripStr = strings.Repeat(" ", n) + tripStr
	}
	sum := clientcrypto.Sha1(xml + tripStr + c.secret)
	framed := sum + " " + tripStr + " " + xml + "\x00"
	c.trip++
	b := []byte(framed)
	uint8 := js.Global().Get("Uint8Array").New(len(b))
	_ = js.CopyBytesToJS(uint8, b)
	c.ws.Call("send", uint8)
	return nil
}

// containsSubstr checks if haystack contains needle as literal bytes
func containsSubstr(haystack []byte, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func (c *Client) nextMsgID() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgID++
	return c.msgID
}

// sendAndWait enqueues xml (respecting policy gate) and waits for <Accepted msgID="id">.
func (c *Client) sendAndWait(ctx context.Context, xml string, id uint64) error {
	// register waiter before sending
	c.mu.Lock()
	if c.pendingAcks == nil {
		c.pendingAcks = make(map[uint64]chan struct{})
	}
	ch := make(chan struct{}, 1)
	c.pendingAcks[id] = ch
	c.mu.Unlock()

	if err := c.SendXML(xml); err != nil {
		c.mu.Lock()
		delete(c.pendingAcks, id)
		c.mu.Unlock()
		return err
	}

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pendingAcks, id)
		c.mu.Unlock()
		return ctx.Err()
	}
}

// parseAcceptedMsgID extracts the msgID from an <Accepted msgID="n"/> response.
func parseAcceptedMsgID(b []byte) (uint64, bool) {
	needle := []byte("<Accepted msgID=\"")
	// simple scan
	for i := 0; i+len(needle) < len(b); i++ {
		if b[i] == '<' && i+len(needle) <= len(b) {
			match := true
			for j := 0; j < len(needle); j++ {
				if b[i+j] != needle[j] {
					match = false
					break
				}
			}
			if match {
				// parse digits until closing quote
				k := i + len(needle)
				var id uint64 = 0
				for ; k < len(b); k++ {
					c := b[k]
					if c >= '0' && c <= '9' {
						id = id*10 + uint64(c-'0')
						continue
					}
					if c == '"' {
						return id, true
					}
					break
				}
			}
		}
	}
	return 0, false
}

// parseXMLAttrInt is a minimal parser to extract integer attribute values from a short XML snippet.
// Example: parseXMLAttrInt([]byte(`<AV x="10" z="14"/>`), "x") => 10
func parseXMLAttrInt(b []byte, attr string) (int, bool) {
	key := []byte(attr + "=\"")
	for i := 0; i+len(key) <= len(b); i++ {
		if i+len(key) <= len(b) && string(b[i:i+len(key)]) == string(key) {
			j := i + len(key)
			start := j
			for j < len(b) && b[j] != '"' {
				j++
			}
			if j > start {
				valStr := string(b[start:j])
				if n, err := strconv.Atoi(valStr); err == nil {
					return n, true
				}
			}
			// do not break; allow further matches if any
		}
	}
	return 0, false
}

// parseXMLAttrString extracts a string attribute value from a short XML snippet.
func parseXMLAttrString(b []byte, attr string) string {
	key := []byte(attr + "=\"")
	for i := 0; i+len(key) < len(b); i++ {
		if i+len(key) <= len(b) && string(b[i:i+len(key)]) == string(key) {
			j := i + len(key)
			start := j
			for j < len(b) && b[j] != '"' {
				j++
			}
			if j > start {
				return html.UnescapeString(string(b[start:j]))
			}
			break
		}
	}
	return ""
}

// mapServerToClient maps server (x,z) to client (x,y) grid coordinates
func (c *Client) mapServerToClient(serverX, serverZ int) (clientX, clientY int) {
	// Public rooms: layout 5 uses a 30x30 grid with a 270° rotation mapping
	// Observed mapping (from London PathMap and server telemetry):
	//   clientRow = serverX
	//   clientCol = 31 - serverZ   (since grid is 30 cols)
	if c.layout == 5 {
		clientX = serverX
		clientY = 31 - serverZ
		return
	}
	// Layout 3: direct table for Studio type C (server uses z,x in 9..14)
	if c.layout == 3 {
		switch serverZ {
		case 9:
			switch serverX {
			case 9:
				return 9, 7
			case 10:
				return 10, 7
			case 11:
				return 11, 7
			case 12:
				return 12, 7
			case 13:
				return 13, 7
			case 14:
				return 14, 7
			}
		case 10:
			switch serverX {
			case 9:
				return 9, 6
			case 10:
				return 10, 6
			case 11:
				return 11, 6
			case 12:
				return 12, 6
			case 13:
				return 13, 6
			case 14:
				return 14, 6
			}
		case 11:
			switch serverX {
			case 9:
				return 9, 5
			case 10:
				return 10, 5
			case 11:
				return 11, 5
			case 12:
				return 12, 5
			case 13:
				return 13, 5
			case 14:
				return 14, 5
			}
		case 12:
			switch serverX {
			case 9:
				return 9, 4
			case 10:
				return 10, 4
			case 11:
				return 11, 4
			case 12:
				return 12, 4
			case 13:
				return 13, 4
			case 14:
				return 14, 4
			}
		case 13:
			switch serverX {
			case 9:
				return 9, 3
			case 10:
				return 10, 3
			case 11:
				return 11, 3
			case 12:
				return 12, 3
			case 13:
				return 13, 3
			case 14:
				return 14, 3
			}
		case 14:
			switch serverX {
			case 9:
				return 9, 2
			case 10:
				return 10, 2
			case 11:
				return 11, 2
			case 12:
				return 12, 2
			case 13:
				return 13, 2
			case 14:
				return 14, 2
			}
		}
	}
	// Fallback heuristic (legacy 15x15-like): clientRow=serverX, clientCol=16-serverZ
	clientX = serverX
	clientY = 16 - serverZ
	return
}

// mapClientToServer maps client grid (row,col) to server (x,z) for layout C.
// For layout 3 (Studio type C), inverse of mapServerToClient:
//
//	client (x=row, y=col) -> server (x=clientX, z=16-clientY)
//
// This matches the direct table where serverZ 9..14 map to clientY 7..2.
func (c *Client) mapClientToServer(clientRow, clientCol int) (serverX, serverZ int) {
	if c.layout == 5 {
		// Inverse of layout 5 mapping
		serverX = clientRow
		serverZ = 31 - clientCol
		return
	}
	if c.layout == 3 {
		// From the observed mapping table: clientY = 16 - serverZ
		// and clientX == serverX for the walkable band 9..14
		serverX = clientRow
		serverZ = 16 - clientCol
		return
	}
	// Fallback heuristic mirrors inverse of fallback mapping
	serverX = clientRow
	serverZ = 16 - clientCol
	return
}

// SendChat sends a Room.Action CH packet with a chat message
// The message will be encoded as binhex. speechMode should be 1 for speak, 2 for shout.
func (c *Client) SendChat(roomID string, message string, speechMode int) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}
	// Encode message as binhex (uppercase hex)
	encodedMsg := strings.ToUpper(hex.EncodeToString([]byte(message)))

	// Build XML: <Room.Action roomID="..."><CH sm="speak" m="..."/></Room.Action>
	var b strings.Builder
	b.Grow(128 + len(encodedMsg))
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><CH sm=\"")
	if speechMode == 2 {
		b.WriteString("shout")
	} else if speechMode == 0 {
		b.WriteString("whisper")
	} else if speechMode == 3 {
		b.WriteString("sing")
	} else {
		b.WriteString("speak")
	}
	b.WriteString("\" m=\"")
	b.WriteString(encodedMsg)
	b.WriteString("\"/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT CH: room=%s mode=%d msg=%q", roomID, speechMode, message)
	}
	return c.SendXML(b.String())
}

// SendWhisper sends a private Room.Action CH packet targeted to a specific denizen.
func (c *Client) SendWhisper(roomID, target, message string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	encodedMsg := strings.ToUpper(hex.EncodeToString([]byte(message)))
	var b strings.Builder
	b.Grow(128 + len(encodedMsg))
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><CH sm=\"whisper\" to=\"")
	b.WriteString(target)
	b.WriteString("\" m=\"")
	b.WriteString(encodedMsg)
	b.WriteString("\"/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT CH (whisper): room=%s to=%s msg=%q", roomID, target, message)
	}
	return c.SendXML(b.String())
}

// SendKickAvatar sends an authoritative Room.Action ST_KICK packet to expel a visitor from a room.
func (c *Client) SendKickAvatar(roomID, target string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	var b strings.Builder
	b.Grow(128)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><ST_KICK target=\"")
	b.WriteString(target)
	b.WriteString("\"/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT ST_KICK: room=%s target=%s", roomID, target)
	}
	return c.SendXML(b.String())
}

// SendMovePathGSP sends a single Room.Action AV_GSP packet containing the full path
// of points from start to end. The server expects a single packet with multiple P nodes.
// The points slice should be ordered from start to end and expressed in client grid
// coordinates (row,col). They will be mapped to server (x,z) using layout inverse mapping.
func (c *Client) SendMovePathGSP(roomID string, points [][2]int) error {
	if len(points) == 0 {
		return nil
	}
	now := time.Now()
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	// Throttle movement sends to at most once per allowed interval
	if c.minMoveInterval > 0 {
		if !c.lastMoveSend.IsZero() && now.Sub(c.lastMoveSend) < c.minMoveInterval {
			c.mu.Unlock()
			if c.logger != nil {
				c.logger.Printf("Throttle: skipping AV_GSP send (%.0fms since last; min=%dms)", now.Sub(c.lastMoveSend).Seconds()*1000, c.minMoveInterval/time.Millisecond)
			}
			return nil
		}
		c.lastMoveSend = now
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}
	// Build flattened XML: <Room.Action roomID="..."><AV_GSP><P z=".." x=".."/>...</AV_GSP></Room.Action>
	var b strings.Builder
	b.Grow(64 + len(points)*24)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><AV_GSP>")
	for _, rc := range points {
		serverX, serverZ := c.mapClientToServer(rc[0], rc[1])
		b.WriteString("<P z=\"")
		b.WriteString(strconv.Itoa(serverZ))
		b.WriteString("\" x=\"")
		b.WriteString(strconv.Itoa(serverX))
		b.WriteString("\"/>")
	}
	b.WriteString("</AV_GSP></Room.Action>")
	return c.SendXML(b.String())
}

// SendFurniturePlacement sends a Room.Action POS_STD packet for furniture placement.
// The coordinates (clientRow, clientCol) are in client grid space and will be mapped to server (x,z).
// dir is 0,2,4,6 for the 4 cardinal directions. state is 0 or 1 for animated items. y is height (usually 0).
// Special case: when values look like screen pixels (large), treat as wall-item placement and bypass grid mapping,
// sending raw x=clientRow, y=clientCol, z=y as observed in Shockwave wall item POS packets.
func (c *Client) SendFurniturePlacement(roomID string, posID string, prodID string, clientRow, clientCol, y, dir, state, ho int) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	// Heuristic: pixel-like values indicate wall-item placement; bypass grid mapping
	isPixelMode := clientRow > 100 || clientCol > 100 || y > 50
	var serverX, serverZ int
	if !isPixelMode {
		// Map client coordinates to server coordinates (floor furniture)
		serverX, serverZ = c.mapClientToServer(clientRow, clientCol)
		if c.logger != nil {
			c.logger.Printf("SendFurniturePlacement: Mapping client[%d,%d] -> server[x=%d,z=%d]",
				clientRow, clientCol, serverX, serverZ)
		}
	} else {
		// For wall items, preserve raw pixel coordinates: x=clientRow, y=clientCol, z=y
		serverX = clientRow
		serverZ = y // keep z as-provided (pixel depth hint)
		if c.logger != nil {
			c.logger.Printf("SendFurniturePlacement (wall): using raw pixels x=%d y=%d z=%d", clientRow, clientCol, y)
		}
	}

	// Build XML: <Room.Action roomID="..."><POS_STD st="0" d="2" ho="HO" z="Z" y="Y" x="X" id="11427"/></Room.Action>
	var b strings.Builder
	b.Grow(128)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><POS_STD st=\"")
	b.WriteString(strconv.Itoa(state))
	b.WriteString("\" d=\"")
	b.WriteString(strconv.Itoa(dir))
	b.WriteString("\" ho=\"")
	b.WriteString(strconv.Itoa(ho))
	b.WriteString("\" z=\"")
	if !isPixelMode {
		b.WriteString(strconv.Itoa(serverZ))
	} else {
		b.WriteString(strconv.Itoa(y))
	}
	b.WriteString("\" y=\"")
	if !isPixelMode {
		b.WriteString(strconv.Itoa(y))
	} else {
		b.WriteString(strconv.Itoa(clientCol))
	}
	b.WriteString("\" x=\"")
	if !isPixelMode {
		b.WriteString(strconv.Itoa(serverX))
	} else {
		b.WriteString(strconv.Itoa(clientRow))
	}
	b.WriteString("\" id=\"")
	b.WriteString(posID)
	b.WriteString("\"/></Room.Action>")

	if c.logger != nil {
		if !isPixelMode {
			c.logger.Printf("Realtime OUT POS_STD: room=%s pos=%s prod=%s client[%d,%d] -> server[x=%d,z=%d] dir=%d state=%d ho=%d",
				roomID, posID, prodID, clientRow, clientCol, serverX, serverZ, dir, state, ho)
		} else {
			c.logger.Printf("Realtime OUT POS_STD (wall): room=%s pos=%s prod=%s x=%d y=%d z=%d dir=%d state=%d ho=%d",
				roomID, posID, prodID, clientRow, clientCol, y, dir, state, ho)
		}
	}
	return c.SendXML(b.String())
}

// SendFurniturePickup sends a Room.Action POS_BKPK packet to pick up furniture back to backpack.
func (c *Client) SendFurniturePickup(roomID string, posID string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	// Build XML: <Room.Action roomID="..."><POS_BKPK id="..."/></Room.Action>
	var b strings.Builder
	b.Grow(128)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><POS_BKPK id=\"")
	b.WriteString(posID)
	b.WriteString("\"/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT POS_BKPK: room=%s pos=%s (pickup to backpack)", roomID, posID)
	}
	return c.SendXML(b.String())
}

// SendFurnitureRotation sends a Room.Action POS packet to rotate furniture in place.
// Unlike POS_STD for placement, rotation uses POS with the same coordinates but new direction.
// ho is the cumulative vertical pixel offset of supports below the item (if any).
func (c *Client) SendFurnitureRotation(roomID string, posID string, clientRow, clientCol, y, dir, state, ho int) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	// Map client coordinates to server coordinates
	serverX, serverZ := c.mapClientToServer(clientRow, clientCol)

	// Build XML: <Room.Action roomID="..."><POS st="0" d="2" ho="0" z="14" y="0" x="10" id="11362"/></Room.Action>
	var b strings.Builder
	b.Grow(128)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><POS st=\"")
	b.WriteString(strconv.Itoa(state))
	b.WriteString("\" d=\"")
	b.WriteString(strconv.Itoa(dir))
	b.WriteString("\" ho=\"")
	b.WriteString(strconv.Itoa(ho))
	b.WriteString("\" z=\"")
	b.WriteString(strconv.Itoa(serverZ))
	b.WriteString("\" y=\"")
	b.WriteString(strconv.Itoa(y))
	b.WriteString("\" x=\"")
	b.WriteString(strconv.Itoa(serverX))
	b.WriteString("\" id=\"")
	b.WriteString(posID)
	b.WriteString("\"/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT POS: room=%s pos=%s rotate to dir=%d at server[x=%d,z=%d] ho=%d",
			roomID, posID, dir, serverX, serverZ, ho)
	}
	return c.SendXML(b.String())
}

// SendAvatarAction sends a Room.Action AV_ACT packet to perform an avatar action (dance, drink, etc.)
func (c *Client) SendAvatarAction(roomID string, act string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	var b strings.Builder
	b.Grow(80)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><AV_ACT act=\"")
	b.WriteString(act)
	b.WriteString("\"/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT AV_ACT: room=%s act=%s", roomID, act)
	}
	return c.SendXML(b.String())
}

// SendDrinkDispenser sends a Room.Action AV_DC packet to request decibels from a drink dispenser.
func (c *Client) SendDrinkDispenser(roomID string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	// Build XML: <Room.Action roomID="..."><AV_DC/></Room.Action>
	var b strings.Builder
	b.Grow(80)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><AV_DC/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT AV_DC: room=%s (drink dispenser request)", roomID)
	}
	return c.SendXML(b.String())
}

// SendCDPlay sends a Room.Action CD_P packet with the selected CD posID.
// Example: <Room.Action roomID="ADAM$0"><CD_P cdId="12011"/></Room.Action>
func (c *Client) SendCDPlay(roomID string, posID string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}
	if strings.TrimSpace(posID) == "" {
		return fmt.Errorf("posID is empty")
	}

	var b strings.Builder
	b.Grow(96)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><CD_P cdId=\"")
	b.WriteString(posID)
	b.WriteString("\"/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT CD_P: room=%s cdId=%s", roomID, posID)
	}
	return c.SendXML(b.String())
}

// SendCDRequest sends a Room.Action CD_R packet to request starting a stereo performance.
// Example: <Room.Action roomID="ADAM$0"><CD_R cdPlId="526"/></Room.Action>
func (c *Client) SendCDRequest(roomID string, posID string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}
	if strings.TrimSpace(posID) == "" {
		return fmt.Errorf("posID is empty")
	}

	var b strings.Builder
	b.Grow(96)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><CD_R cdPlId=\"")
	b.WriteString(posID)
	b.WriteString("\"/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT CD_R: room=%s pos=%s", roomID, posID)
	}
	return c.SendXML(b.String())
}

// SendThumbsUp sends a Room.Action CD_TU packet for the current room.
// XML: <Room.Action roomID="..."><CD_TU/></Room.Action>
func (c *Client) SendThumbsUp(roomID string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	var b strings.Builder
	b.Grow(64)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><CD_TU/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT CD_TU: room=%s", roomID)
	}
	return c.SendXML(b.String())
}

// SendThumbsDown sends a Room.Action CD_TD packet for the current room.
// XML: <Room.Action roomID="..."><CD_TD/></Room.Action>
func (c *Client) SendThumbsDown(roomID string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	var b strings.Builder
	b.Grow(64)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><CD_TD/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT CD_TD: room=%s", roomID)
	}
	return c.SendXML(b.String())
}

// SendCDEnd sends a Room.Action CD_E packet for the current room to end a performance.
// XML: <Room.Action roomID="..."><CD_E/></Room.Action>
func (c *Client) SendCDEnd(roomID string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	var b strings.Builder
	b.Grow(64)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><CD_E/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT CD_E: room=%s", roomID)
	}
	return c.SendXML(b.String())
}

// SendFriendRequest sends a Room.Action MMS_REQ packet to invite another denizen as friend.

func (c *Client) SendFriendRequest(roomID string, targetName string) error {

	c.mu.Lock()

	if roomID == "" {

		roomID = c.roomID

	}

	c.mu.Unlock()

	if strings.TrimSpace(roomID) == "" {

		return fmt.Errorf("roomID is empty")

	}

	var b strings.Builder

	b.Grow(80)

	b.WriteString("<Room.Action roomID=\"")

	b.WriteString(roomID)

	b.WriteString("\"><MMS_REQ to=\"")

	b.WriteString(targetName)

	b.WriteString("\"/></Room.Action>")

	if c.logger != nil {

		c.logger.Printf("Realtime OUT MMS_REQ: room=%s to=%s", roomID, targetName)

	}

	return c.SendXML(b.String())

}

// SendTradeStart sends a Room.Action TD_ST packet to initiate a trade with another player.
func (c *Client) SendTradeStart(roomID string, targetName string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	// Build XML: <Room.Action roomID="..."><TD_ST tde="targetname"/></Room.Action>
	var b strings.Builder
	b.Grow(128)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><TD_ST tde=\"")
	b.WriteString(targetName)
	b.WriteString("\"/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT TD_ST: room=%s target=%s (start trade)", roomID, targetName)
	}
	return c.SendXML(b.String())
}

// SendTradeAddItem sends a Room.Action TD_AD packet to add an item to the trade.
func (c *Client) SendTradeAddItem(roomID string, posID string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	// Build XML: <Room.Action roomID="..."><TD_AD id="posid"/></Room.Action>
	var b strings.Builder
	b.Grow(128)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><TD_AD id=\"")
	b.WriteString(posID)
	b.WriteString("\"/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT TD_AD: room=%s pos=%s (add to trade)", roomID, posID)
	}
	return c.SendXML(b.String())
}

// SendTradeAgree sends a Room.Action TD_AG packet to accept the current trade.
func (c *Client) SendTradeAgree(roomID string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	// Build XML: <Room.Action roomID="..."><TD_AG/></Room.Action>
	var b strings.Builder
	b.Grow(100)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><TD_AG/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT TD_AG: room=%s (agree to trade)", roomID)
	}
	return c.SendXML(b.String())
}

// SendTradeCancel sends a Room.Action TD_CN packet to cancel the trade.
func (c *Client) SendTradeCancel(roomID string) error {
	c.mu.Lock()
	if roomID == "" {
		roomID = c.roomID
	}
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roomID is empty")
	}

	// Build XML: <Room.Action roomID="..."><TD_CN/></Room.Action>
	var b strings.Builder
	b.Grow(100)
	b.WriteString("<Room.Action roomID=\"")
	b.WriteString(roomID)
	b.WriteString("\"><TD_CN/></Room.Action>")

	if c.logger != nil {
		c.logger.Printf("Realtime OUT TD_CN: room=%s (cancel trade)", roomID)
	}
	return c.SendXML(b.String())
}

// SendMessengerMessage sends a direct instant message to another denizen
func (c *Client) SendMessengerMessage(targetName, message string) error {
	c.mu.Lock()
	roomID := c.roomID
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		roomID = "$LOBBY$"
	}
	xml := fmt.Sprintf("<Room.Action roomID=\"%s\"><MMS_MSG to=\"%s\" msg=\"%s\"/></Room.Action>", roomID, targetName, message)
	return c.SendXML(xml)
}

// SendFriendAccept sends an authoritative acceptance of a friend request
func (c *Client) SendFriendAccept(fromUser string) error {
	c.mu.Lock()
	roomID := c.roomID
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		roomID = "$LOBBY$"
	}
	xml := fmt.Sprintf("<Room.Action roomID=\"%s\"><MMS_ACC from=\"%s\"/></Room.Action>", roomID, fromUser)
	return c.SendXML(xml)
}

// SendFriendReject declines a pending friend request
func (c *Client) SendFriendReject(fromUser string) error {
	c.mu.Lock()
	roomID := c.roomID
	c.mu.Unlock()
	if strings.TrimSpace(roomID) == "" {
		roomID = "$LOBBY$"
	}
	xml := fmt.Sprintf("<Room.Action roomID=\"%s\"><MMS_REJ from=\"%s\"/></Room.Action>", roomID, fromUser)
	return c.SendXML(xml)
}

// FlushRoomMessages delivers snapshots once the game loop installs their scene,
// even when no further socket packet arrives. Drained packets are dispatched directly
// without re-entering the inbox deferral filter.
func (c *Client) FlushRoomMessages() {
	if c.dispatchPacket == nil {
		return
	}
	if sc := studioscene.GetActiveScene(); sc != nil && sc.GetStudio() != nil {
		for _, packet := range c.roomInbox.drain(sc.GetStudio().StudioID) {
			c.dispatchPacket(packet)
		}
	}
}
