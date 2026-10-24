package studio

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"net/http"

	mrand "math/rand"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/colorm"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"

	"gocoke/internal/client/assets/catalog"
	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/studioroom"
	"gocoke/internal/client/assets/studios"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/avatarengine"
	"gocoke/internal/client/isoengine"
	"gocoke/internal/client/net/gateway"
	"gocoke/internal/client/patterns"
	"gocoke/internal/client/ui"
)

// Scene represents a studio room scene
type Scene struct {
	tm         *textmgr.Manager
	studio     *studios.Studio
	roomDef    *studioroom.RoomDefinition
	memberReg  *studioroom.MemberRegistry
	studioList *studios.StudioList // optional: used to populate navigator lists

	// Isometric engine
	isoScene   *isoengine.IsoScene
	studioXML  *isoengine.StudioXML
	pathMapXML *isoengine.PathMapXML

	// Assets
	assets map[string]*ebiten.Image

	// Pattern system
	patternMgr     *patterns.Manager
	patternConsole *patterns.Console

	// HUD system
	hudManager *HUDManager

	// Chat system
	chatManager *ChatManager

	// InfoStand overlay (plaque, preview, description, buttons)
	infoStand *InfoStand
	// Now Playing flag
	nowPlayingActive bool

	// Permissions
	currentUserName string // local player's screen name (for ownership checks and InfoStand)

	// Credentials for gateway calls
	userCredentials struct {
		screenName string
		secret     string
		gatewayURL string
	}

	// Exit control
	kicked bool // set true when the local user is kicked

	// Backpack manager
	backpackManager *BackpackManager

	// State
	last            time.Time
	debugPrinted    bool                 // Flag to prevent spammy debug output
	lastMouseSquare *isoengine.IsoSquare // Track last mouse square for debug output
	lastF2Press     time.Time            // For F2 key debouncing
	lastF3Press     time.Time            // Grid debug toggle
	lastF5Press     time.Time            // Chat
	lastF6Press     time.Time            // Emote cycle
	lastF7Press     time.Time            // Carry toggle
	lastF8Press     time.Time            // Drink toggle
	lastF9Press     time.Time            // Lighting toggle

	// Avatar engine
	playerAvatar   *isoengine.IsoAvatar
	avatars        []*isoengine.IsoAvatar
	avatarsByName  map[string]*isoengine.IsoAvatar
	playerHasMoved bool // Controls initial layering (draw behind walls until first move)
	carryingOn     bool
	drinkingOn     bool
	emoteIndex     int

	// Micro-adjustments
	heightDepthAdjust int // depth offset per pixel of stacked furniture height to prevent occlusion
	avatarDepthBoost  int // global avatar depth boost (positive or negative)

	// Per-PosID slice depth boosts: posID -> slice (a/b/c/d) -> delta
	depthBoostsBySlicePerPos map[string]map[string]int

	// Mouse and hiliter state (now managed by isoengine)
	showHiliter bool
	hiliterX    float64
	hiliterY    float64

	// Room positioning
	roomRect [4]int // Room bounds
	offsetX  int    // X offset for positioning
	offsetY  int    // Y offset for positioning

	// Cached element sorting (optimization)
	sortedElements     []studioroom.RoomElement
	elementsSortedOnce bool

	// Background image for the room (from Studio XML or PathMap)
	backgroundImg *ebiten.Image
	bgPos         [2]int // x, y

	// Data directory for loading shared data like Catalogue_English.txt
	dataDir string

	// Catalog and assets
	catalog     *catalog.Catalog
	smallAssets map[string]*ebiten.Image

	// Optional hook provided by app to send AV_GSP over realtime
	sendMovePathIfAvailable func(roomID string, points [][2]int)

	// Optional hook provided by app to send CH chat messages over realtime
	sendChatIfAvailable func(roomID string, message string, speechMode int) error

	// Optional hook provided by app to send POS_STD furniture placement over realtime
	sendFurnitureIfAvailable func(roomID string, posID string, prodID string, x, y, z, dir, state, ho int) error

	// Optional hook provided by app to send POS_BKPK furniture pickup over realtime
	sendPickupIfAvailable func(roomID string, posID string) error

	// Optional hook provided by app to send POS furniture rotation over realtime
	sendRotateIfAvailable func(roomID string, posID string, x, y, z, dir, state, ho int) error

	// Optional hook provided by app to send AV_DC drink dispenser request over realtime
	sendDrinkDispenserIfAvailable func(roomID string) error

	// Optional hook provided by app to send CD_R (stereo choose song) over realtime
	sendCDRequestIfAvailable func(roomID string, posID string) error

	// Optional hook to send CD_P play request with selected CD posID
	sendCDPlayIfAvailable func(roomID string, posID string) error

	// Optional hook to send thumbs up/down (CD_TU/CD_TD)
	sendThumbIfAvailable func(up bool) error

	// Optional hook to send CD_E (end performance)
	sendCDEndIfAvailable func(roomID string) error

	// Decibels display state
	decibelsWindow    *DecibelsWindow
	showingDecibels   bool
	decibelsShowUntil time.Time

	// Coke holding state for player avatar
	playerHoldingCoke bool
	cokeHoldUntil     time.Time
	lastCokeDrinkTime time.Time
	// Mic performance random "drk" cycle state
	lastMicDrinkTime time.Time

	// Placement state
	placement   *PlacementHelper
	placedFurni []PlacedFurni

	// Trading
	tradeManager *TradeManager

	// FPS counter
	fpsCounter *ui.FPSCounter

	// Lighting system
	lighting *LightingSystem
	// one-time debug print for lights count
	lightsDebugPrinted bool

	// Dim overlay for lighting mode
	dimOverlay *ebiten.Image

	// On-screen lighting toggle button (for mobile/touch use)
	lightBtnRect      [4]int        // x, y, w, h
	lightBtnBg        *ebiten.Image // cached background image
	lastLightBtnPress time.Time     // debounce for touch/click toggle

	// Studio loading overlay
	showStudioLoader bool
	loaderStart      time.Time
	loaderStep       int
	loaderSteps      int
	loaderMinMS      int
	loaderTip        string
	loaderFP         *studioFramePlayer

	// Random source for loader tip selection
	rng *mrand.Rand

	// Stereo/sequencer playback
	stereo *StereoPlayer

	// Pending network-driven avatar events (processed in Update on the main thread)
	pendingMu  sync.Mutex
	pendingAVs []avEvent
	pendingMV  []mvEvent
	pendingACT []actEvent
	pendingCH  []chatEvent
	pendingRM  []string

	// Pending furniture placement events from server (POS)
	pendingPOS []posEvent

	// Pending drink dispenser interaction: move to target then request
	pendingDrinkActive    bool
	pendingDrinkTargetRow int
	pendingDrinkTargetCol int
	pendingDrinkExpire    time.Time

	// Pending stereo interaction: move to target then send CD_R with posID
	pendingStereoActive    bool
	pendingStereoAlert     bool
	pendingStereoTargetRow int
	pendingStereoTargetCol int
	pendingStereoExpire    time.Time
	pendingStereoPosID     string
}

// GetCurrentUserName returns the local player's screen name.
func (s *Scene) GetCurrentUserName() string {
	return s.currentUserName
}

// Active scene registry for network updates
var activeScene *Scene

func SetActiveScene(s *Scene) { activeScene = s }
func GetActiveScene() *Scene  { return activeScene }

// SetMoveSender allows the app to inject a network sender for AV_GSP paths.
// The callback will be invoked with the current roomID and the ordered client grid points.
func (s *Scene) SetMoveSender(fn func(roomID string, points [][2]int)) {
	if s != nil {
		s.sendMovePathIfAvailable = fn
		//log.Printf("Studio: move sender injected")
	}
}

// SetChatSender allows the app to inject a network sender for CH chat messages.
// The callback will be invoked with the current roomID, message text, and speech mode (1=speak, 2=shout).
func (s *Scene) SetChatSender(fn func(roomID string, message string, speechMode int) error) {
	if s != nil {
		s.sendChatIfAvailable = fn
		//log.Printf("Studio: chat sender injected")
	}
}

// SetFurnitureSender allows the app to inject a network sender for POS_STD furniture placement.
// The callback will be invoked with the current roomID, posID, prodID, and placement coordinates/state.
func (s *Scene) SetFurnitureSender(fn func(roomID string, posID string, prodID string, x, y, z, dir, state, ho int) error) {
	if s != nil {
		s.sendFurnitureIfAvailable = fn
		//log.Printf("Studio: furniture sender injected")
	}
}

// SetPickupSender allows the app to inject a network sender for POS_BKPK furniture pickup.
// The callback will be invoked with the current roomID and posID of the furniture to pick up.
func (s *Scene) SetPickupSender(fn func(roomID string, posID string) error) {
	if s != nil {
		s.sendPickupIfAvailable = fn
		//log.Printf("Studio: pickup sender injected")
	}
}

// SetRotateSender allows the app to inject a network sender for POS furniture rotation.
// The callback will be invoked with the current roomID, posID, and updated rotation parameters.
func (s *Scene) SetRotateSender(fn func(roomID string, posID string, x, y, z, dir, state, ho int) error) {
	if s != nil {
		s.sendRotateIfAvailable = fn
		//log.Printf("Studio: rotate sender injected")
	}
}

// SetDrinkDispenserSender allows the app to inject a network sender for AV_DC drink dispenser requests.
// The callback will be invoked with the current roomID.
func (s *Scene) SetDrinkDispenserSender(fn func(roomID string) error) {
	if s != nil {
		s.sendDrinkDispenserIfAvailable = fn
	}
}

// SetCDRequestSender allows the app to inject a network sender for CD_R stereo requests.
// The callback will be invoked with the current roomID and the PosID of the stereo clicked.
func (s *Scene) SetCDRequestSender(fn func(roomID string, posID string) error) {
	if s != nil {
		s.sendCDRequestIfAvailable = fn
	}
}

// SetCDPlaySender allows the app to inject a network sender for CD_P play with selected CD posID.
func (s *Scene) SetCDPlaySender(fn func(roomID string, posID string) error) {
	if s != nil {
		s.sendCDPlayIfAvailable = fn
	}
}

// SetThumbSender allows the app to inject a network sender for CD_TU/CD_TD votes.
// The callback receives true for thumbs up, false for thumbs down.

// SetCDEndSender allows the app to inject a network sender for CD_E end performance.
func (s *Scene) SetCDEndSender(fn func(roomID string) error) {
	if s != nil {
		s.sendCDEndIfAvailable = fn
	}
}

func (s *Scene) SetThumbSender(fn func(up bool) error) {
	if s != nil {
		s.sendThumbIfAvailable = fn
	}
}

// EmitThumbOverlay shows a transient thumbs icon 10px above the named avatar's head.
func (s *Scene) EmitThumbOverlay(avatarName string, up bool) {
	if s == nil || s.chatManager == nil {
		return
	}
	if s.assets == nil {
		s.assets = make(map[string]*ebiten.Image)
	}
	av, ok := s.avatarsByName[avatarName]
	if !ok || av == nil {
		return
	}
	// Ensure assets present
	if _, ok := s.assets["cc.thumbicon.up"]; !ok {
		// Load directly via lingo assets
		if mm, _ := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_interface[1]")); mm != nil {
			if p, ok2 := mm.PathFor("cc.thumbicon.up"); ok2 {
				if img, err := lingoassets.LoadImage(p); err == nil {
					s.assets["cc.thumbicon.up"] = img
				}
			}
		}
	}
	if _, ok := s.assets["cc.thumbicon.down"]; !ok {
		if mm, _ := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_interface[1]")); mm != nil {
			if p, ok2 := mm.PathFor("cc.thumbicon.down"); ok2 {
				if img, err := lingoassets.LoadImage(p); err == nil {
					s.assets["cc.thumbicon.down"] = img
				}
			}
		}
	}
	var img *ebiten.Image
	if up {
		img = s.assets["cc.thumbicon.up"]
	} else {
		img = s.assets["cc.thumbicon.down"]
	}
	if img == nil {
		return
	}
	pos := av.GetScreenPos()
	x := pos.X - float64(img.Bounds().Dx())/2
	y := pos.Y - float64(img.Bounds().Dy()) - 100
	b := ChatBubble{img: img, posX: x, posY: y, created: time.Now(), lifetime: 10 * time.Second}
	s.chatManager.bubbles = append(s.chatManager.bubbles, b)
}

// HandleDrinkDispenserResponse processes the server response to a drink dispenser request.
// Called when we receive <Room.Action><AV_DC av="username" pts="10"/></Room.Action>
func (s *Scene) HandleDrinkDispenserResponse(avatarName string, decibels int) {
	if s == nil {
		return
	}

	log.Printf("HandleDrinkDispenserResponse: avatar=%s, decibels=%d", avatarName, decibels)

	// Show the decibels window for 6 seconds
	if s.decibelsWindow == nil {
		s.decibelsWindow = NewDecibelsWindow(s.tm)
		if err := s.decibelsWindow.LoadAssets(); err != nil {
			log.Printf("Failed to load decibels window assets: %v", err)
			return
		}
	}
	s.decibelsWindow.SetDecibels(decibels)
	// Ensure the cap window is visible for each response
	if !s.decibelsWindow.IsVisible() {
		s.decibelsWindow.Show()
	}
	s.showingDecibels = true
	s.decibelsShowUntil = time.Now().Add(6 * time.Second)

	// If this is the player's avatar, start holding coke for 5 minutes
	if s.playerAvatar != nil && s.currentUserName == avatarName {
		s.playerHoldingCoke = true
		s.cokeHoldUntil = time.Now().Add(5 * time.Minute) // 5 minutes, not 5000ms
		// Don't set lastCokeDrinkTime yet - wait for first random drink
		s.lastCokeDrinkTime = time.Time{} // Zero time means no drink yet
		// Set the holding item on the avatar (just hold, don't drink yet)
		s.playerAvatar.Engine.SetHoldingItem("coke_hold") // Use special flag to not trigger drink
	}
}

// HandleCDRequestAccepted opens the choose_song window upon CD_RA from server.
func (s *Scene) HandleCDRequestAccepted() {
	if s == nil || s.hudManager == nil {
		return
	}
	s.hudManager.OpenChooseSongWindow()
}

// UpdateTradeDisplay handles incoming TD_DISPL packets from the network
func (s *Scene) UpdateTradeDisplay(traderName, tradeeName, traderItems, tradeeItems string, traderAccepts, tradeeAccepts bool) {
	if s == nil || s.tradeManager == nil {
		return
	}
	s.tradeManager.UpdateFromNetwork(traderName, tradeeName, traderItems, tradeeItems, traderAccepts, tradeeAccepts)
}

// CompleteTradeFromNetwork handles incoming TD_CPLT packets from the network
func (s *Scene) CompleteTradeFromNetwork(receivedItems []map[string]string) {
	if s == nil || s.tradeManager == nil {
		return
	}
	s.tradeManager.CompleteTradeFromNetwork(receivedItems)
}

// CancelTradeFromNetwork handles incoming TD_CN or TD_CNCL packets from the network
func (s *Scene) CancelTradeFromNetwork() {
	if s == nil || s.tradeManager == nil {
		return
	}
	s.tradeManager.CancelIfPending()
}

// ShowPerformanceResults displays an alert with performance voting results and decibels awarded.
func (s *Scene) ShowPerformanceResults(thumbsUp, thumbsDown, decibels int) {
	if s == nil || s.hudManager == nil {
		return
	}
	s.hudManager.OpenPerformanceResultsDialog(thumbsUp, thumbsDown, decibels)
}

// SetCurrentUserName sets the local player's screen name for self-targeting logic
func (s *Scene) SetCurrentUserName(name string) {
	s.currentUserName = strings.TrimSpace(name)
	// Keep InfoStand's self-detection in sync
	if s.infoStand != nil {
		s.infoStand.SetCurrentPlayerName(s.currentUserName)
	}
}

// GetTradeManager returns the trade manager for setting up network callbacks
func (s *Scene) GetTradeManager() *TradeManager {
	if s != nil {
		return s.tradeManager
	}
	return nil
}

// avEvent represents an incoming avatar placement/update request from the network thread
type avEvent struct {
	name    string
	def     string
	mission string
	gender  int
	dir     int
	row     int
	col     int
	isBot   bool
}

type mvEvent struct {
	name string
	row  int
	col  int
}

// actEvent represents an avatar action update (e.g., dance start/stop)
type actEvent struct {
	name string
	act  string
	dir  int
}

// posEvent represents a server-driven furniture placement (POS) to be applied
// using the same placement rules as local placement.
type posEvent struct {
	posID  string
	prodID string // cid == catalog ProdId
	x      int    // server x
	y      int    // server y (unused for floor furni; reserved)
	z      int    // server z
	ho     int    // server height offset in pixels (vertical offset)
	dir    int    // 0,2,4,6
	state  int    // 0/1 active
	iaot   bool   // items allowed on top
}

// EnqueuePOS queues a furniture POS event to be processed on the main thread.
func (s *Scene) EnqueuePOS(posID, prodID string, x, y, z, ho, dir, state int, iaot bool) {
	if s == nil {
		return
	}
	s.pendingMu.Lock()
	s.pendingPOS = append(s.pendingPOS, posEvent{posID: posID, prodID: prodID, x: x, y: y, z: z, ho: ho, dir: dir, state: state, iaot: iaot})
	s.pendingMu.Unlock()
}

// EnqueueAV queues an avatar create/update to be processed on the main thread in Update()
func (s *Scene) EnqueueAV(name, def, mission string, gender, dir, row, col int, isBot bool) {
	if s == nil {
		return
	}
	s.pendingMu.Lock()
	s.pendingAVs = append(s.pendingAVs, avEvent{name: name, def: def, mission: mission, gender: gender, dir: dir, row: row, col: col, isBot: isBot})
	s.pendingMu.Unlock()
}

// EnqueueMove queues a movement event to be processed on the main thread
func (s *Scene) EnqueueMove(name string, row, col int) {
	if s == nil {
		return
	}
	s.pendingMu.Lock()
	s.pendingMV = append(s.pendingMV, mvEvent{name: name, row: row, col: col})
	s.pendingMu.Unlock()
}

// EnqueueAVAction queues an avatar action update (e.g., dance start/stop)
func (s *Scene) EnqueueAVAction(name, act string, dir int) {
	if s == nil {
		return
	}
	s.pendingMu.Lock()
	s.pendingACT = append(s.pendingACT, actEvent{name: name, act: act, dir: dir})
	s.pendingMu.Unlock()
}

// EnqueueRemoveAvatar queues an avatar removal to be processed on the main thread
func (s *Scene) EnqueueRemoveAvatar(name string) {
	if s == nil {
		return
	}
	s.pendingMu.Lock()
	s.pendingRM = append(s.pendingRM, strings.TrimSpace(name))
	s.pendingMu.Unlock()
}

// chatEvent represents an incoming chat message to be displayed as a bubble
type chatEvent struct {
	name       string
	message    string
	speechMode int // 1=speak, 2=shout
}

// EnqueueChat queues a chat message to be displayed on the main thread
func (s *Scene) EnqueueChat(name, message string, speechMode int) {
	if s == nil {
		return
	}
	s.pendingMu.Lock()
	s.pendingCH = append(s.pendingCH, chatEvent{name: name, message: message, speechMode: speechMode})
	s.pendingMu.Unlock()
}

// UpsertAvatarAtGrid creates or updates an avatar at a specific grid row/col.
// dir is server iso direction (0..7). gender: 1=male, 2=female.
func (s *Scene) UpsertAvatarAtGrid(name, def, mission string, gender, dir, row, col int, isBot bool) {
	if s == nil || s.isoScene == nil {
		return
	}
	if s.avatarsByName == nil {
		s.avatarsByName = make(map[string]*isoengine.IsoAvatar)
	}

	square := s.isoScene.GetSquareByRowCol(row, col)
	if square == nil {
		return
	}

	// Use server-provided direction directly
	engDir := dir

	if av, ok := s.avatarsByName[name]; ok && av != nil && av.Engine != nil {
		// Update engine attributes from definition by building a temp engine
		if def != "" {
			if eng, err := avatarengine.NewAvatarEngine(def, 100, false, s.dataDir); err == nil && eng != nil {
				for k, v := range eng.Attributes {
					av.Engine.Attributes[k] = v
				}
			}
		}
		av.Engine.SetDirection(engDir)
		av.SetMission(mission)
		// Snap to target square
		av.Square = square
		center := square.GetViewCenter()
		av.SetScreenPos(center)
		av.SetTargetScreenPos(center)
		// Public rooms: scale movement speed and animation speed to match 50% size
		if s.studio != nil && s.studio.Owner == "$SYSTEM$" {
			// Movement speed scale ~0.5 (from 84px/s to ~42px/s)
			av.SetSpeedScale(0.5)
			// Animation speed: make walking animation slower; 1.3 was previous slow factor
			av.Engine.WalkAnimScale = 1.3 * 0.8
		} else {
			// Private rooms: restore defaults
			av.SetSpeed(84.0)
			av.Engine.WalkAnimScale = 1.3
		}
		// Bind local player avatar if this is the current user
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(s.currentUserName)) {
			if s.playerAvatar != av {
				s.playerAvatar = av
				log.Printf("Studio: bound playerAvatar to %s (update)", name)
			}
		}
		return
	}

	// Create new engine from definition
	eng, err := avatarengine.NewAvatarEngine(def, 100, false, s.dataDir)
	if err != nil || eng == nil {
		return
	}
	eng.SetDirection(engDir)
	avatar := isoengine.NewIsoAvatar(eng, square, name, mission, isBot)
	if avatar == nil {
		return
	}
	avatar.SetMission(mission)
	// Place at square center
	center := square.GetViewCenter()
	avatar.SetScreenPos(center)
	avatar.SetTargetScreenPos(center)
	// Public rooms: slow movement and animation
	if s.studio != nil && s.studio.Owner == "$SYSTEM$" {
		avatar.SetSpeedScale(0.5)
		eng.WalkAnimScale = 1.3 * 0.8
	}

	s.avatars = append(s.avatars, avatar)
	s.avatarsByName[name] = avatar
	// Bind local player avatar if this is the current user
	if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(s.currentUserName)) {
		s.playerAvatar = avatar
		log.Printf("Studio: bound playerAvatar to %s (create)", name)
	}
}

// MoveAvatarToGrid moves an existing avatar to the given row/col using pathfinding if available.
func (s *Scene) MoveAvatarToGrid(name string, row, col int) {
	if s == nil || s.isoScene == nil || s.isoScene.GetGrid() == nil {
		return
	}
	av, ok := s.avatarsByName[name]
	if !ok || av == nil {
		return
	}
	goal := s.isoScene.GetSquareByRowCol(row, col)
	if goal == nil {
		return
	}
	av.MoveDirectToSquare(goal)
}

// Provide a small alias to reuse boot's framePlayer without exposing internals.
type FramePlayerAlias = studioFramePlayer

// GenerateUniquePosID returns a PosID unique across placed furniture and backpack items
func (s *Scene) GenerateUniquePosID(base string) string {
	if base == "" {
		base = "pos"
	}
	candidate := base
	counter := 1
	exists := func(id string) bool {
		// Check placed furniture
		for _, pf := range s.placedFurni {
			if strings.TrimSpace(pf.PosID) == strings.TrimSpace(id) {
				return true
			}
		}
		// Check backpack items
		if s.backpackManager != nil {
			for _, it := range s.backpackManager.items {
				if strings.TrimSpace(it.PosID) == strings.TrimSpace(id) {
					return true
				}
			}
		}
		return false
	}
	for exists(candidate) {
		counter++
		candidate = fmt.Sprintf("%s-%d", base, counter)
	}
	return candidate
}

// cumulativeIAOTHeightBelow returns the sum of heights (in pixels) of all IAOT items strictly below the given seat furniture
// at the avatar's current seat position. It excludes the seat's own height.
func (s *Scene) cumulativeIAOTHeightBelow(seat PlacedFurni) int {
	if s.placement == nil || s.catalog == nil {
		return 0
	}
	// Compute all squares covered by the seat
	seatedSquares := s.placement.coverageSquares(seat.Row, seat.Col, seat.Width, seat.Depth, seat.Dir)
	// For each placed item, if it overlaps any of the seat squares and is strictly lower GridY and IAOT=true, add its height
	total := 0
	for _, pf := range s.placedFurni {
		if pf.GridY >= seat.GridY { // only items below the seat
			continue
		}
		// Must allow items on top (IAOT=true)
		if !s.allowsItemsOnTop(pf) {
			continue
		}
		// Overlap check
		overlapped := false
		covered := s.placement.coverageSquares(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
		for _, sqSeat := range seatedSquares {
			if sqSeat == nil {
				continue
			}
			r := sqSeat.GetRow()
			c := sqSeat.GetCol()
			for _, sqPF := range covered {
				if sqPF != nil && sqPF.GetRow() == r && sqPF.GetCol() == c {
					overlapped = true
					break
				}
			}
			if overlapped {
				break
			}
		}
		if !overlapped {
			continue
		}
		// Add height from catalog
		if ci := s.catalog.GetItemByID(pf.ProdID); ci != nil {
			total += ci.Height
		} else {
			// Fallback to stored height if catalog entry missing
			total += pf.Height
		}
	}
	return total
}

// SetAvatarDepthBoost sets a global depth boost applied to avatar body and shadow depths
func (s *Scene) SetAvatarDepthBoost(amount int) string {
	s.avatarDepthBoost = amount
	return fmt.Sprintf("Avatar depth boost set to %d", amount)
}

// SetBackpackItems injects the user's possessions into the studio scene (replaces seeds)
func (s *Scene) SetBackpackItems(items []BackpackItem) {
	if s.backpackManager != nil {
		s.backpackManager.SetItems(items)
	}
}

// LoadPossessionsFromGateway fetches the user's possessions from the gateway and populates the backpack
func (s *Scene) LoadPossessionsFromGateway(screenName, secret, gatewayURL string) {
	// Validate inputs
	if screenName == "" || secret == "" {
		log.Printf("Studio: cannot load possessions - no credentials provided")
		return
	}

	if gatewayURL == "" {
		log.Printf("Studio: cannot load possessions - no gateway URL provided")
		return
	}

	// Store credentials for future use
	s.userCredentials.screenName = screenName
	s.userCredentials.secret = secret
	s.userCredentials.gatewayURL = gatewayURL

	log.Printf("Studio: LoadPossessionsFromGateway called for user=%s, gateway=%s", screenName, gatewayURL)

	// Create HTTP client
	httpClient := &http.Client{Timeout: 15 * time.Second}

	// Fetch possessions in background to avoid blocking scene init
	go func() {
		log.Printf("Studio: Starting gateway call to fetch possessions for %s...", screenName)
		possessions, err := gateway.GetPossessionsInBackpack(httpClient, gatewayURL, screenName, secret)
		if err != nil {
			log.Printf("Studio: ERROR - failed to fetch possessions: %v", err)
			return
		}

		log.Printf("Studio: SUCCESS - received %d possessions from gateway", len(possessions))

		// Convert to BackpackItem format
		var items []BackpackItem
		for i, poss := range possessions {
			item := BackpackItem{
				Attributes: make(map[string]interface{}),
			}

			// Extract prodID
			if prodID, ok := poss["prodID"]; ok && prodID != nil {
				switch v := prodID.(type) {
				case int:
					item.ProdID = fmt.Sprintf("%d", v)
				case float64:
					item.ProdID = fmt.Sprintf("%d", int(v))
				case string:
					item.ProdID = v
				}
			}

			// Extract posID
			if posID, ok := poss["posID"]; ok && posID != nil {
				switch v := posID.(type) {
				case int:
					item.PosID = fmt.Sprintf("%d", v)
				case float64:
					item.PosID = fmt.Sprintf("%d", int(v))
				case string:
					item.PosID = v
				}
			}

			// Extract attributes
			if attrs, ok := poss["attributes"].(map[string]interface{}); ok {
				item.Attributes = attrs
			}

			// Log first few items to debug conversion
			if i < 3 {
				log.Printf("Studio: Item %d - ProdID: %s, PosID: %s (from prodID=%v, posID=%v)",
					i, item.ProdID, item.PosID, poss["prodID"], poss["posID"])
			}

			// Only add valid items
			if item.ProdID != "" && item.PosID != "" {
				items = append(items, item)
			} else if i < 3 {
				log.Printf("Studio: Skipping item %d - missing ProdID or PosID", i)
			}
		}

		// Update backpack on main thread
		log.Printf("Studio: setting %d items in backpack", len(items))
		s.SetBackpackItems(items)
	}()
}

// New creates a new studio scene
func New(dataDir string, studio *studios.Studio) (*Scene, error) {
	tm := textmgr.New()
	path := filepath.Join(dataDir, "Text_English.txt")
	if err := tm.LoadFile(path); err != nil {
		// Silently continue if text file fails to load
	}

	patternMgr := patterns.New()
	scene := &Scene{
		tm:             tm,
		studio:         studio,
		assets:         make(map[string]*ebiten.Image),
		patternMgr:     patternMgr,
		patternConsole: patterns.NewConsole(patternMgr),

		dataDir:                  dataDir,
		smallAssets:              make(map[string]*ebiten.Image),
		heightDepthAdjust:        100, // 3 depth units per pixel of furniture height
		avatarDepthBoost:         0,
		depthBoostsBySlicePerPos: make(map[string]map[string]int),
	}

	// Initialize isometric engine
	if err := scene.initIsoEngine(dataDir); err != nil {
		return nil, fmt.Errorf("failed to initialize iso engine: %w", err)
	}

	// Load room definition based on studio layout (legacy support)
	if err := scene.loadRoomDefinition(dataDir); err != nil {
		return nil, fmt.Errorf("failed to load room definition: %w", err)
	}

	// Load member registry for registration points
	if err := scene.loadMemberRegistry(dataDir); err != nil {
		return nil, fmt.Errorf("failed to load member registry: %w", err)
	}

	// Load room assets
	if err := scene.loadRoomAssets(dataDir); err != nil {
		return nil, fmt.Errorf("failed to load room assets: %w", err)
	}

	// Load pattern data
	if err := scene.patternMgr.LoadPatterns(dataDir); err != nil {
		// Don't fail initialization if patterns can't be loaded
	}

	// Pass floor shape asset and position to pattern manager for floor pattern rendering
	scene.setupFloorShapeForPatterns()

	// Initialize HUD manager (required by InfoStand, chat, and other UI systems)
	scene.hudManager = NewHUDManager(scene.tm)
	if err := scene.hudManager.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize HUD manager: %w", err)
	}
	// Provide providers to navigator for current user and gateway URL
	if scene.hudManager != nil {
		scene.hudManager.SetNavigatorProviders(
			func() string { return scene.userCredentials.screenName },
			func() string { return scene.userCredentials.gatewayURL },
		)
		// Wire catalogue gateway providers and post-purchase hook
		if scene.hudManager.catalogueManager != nil {
			scene.hudManager.catalogueManager.SetGatewayProviders(
				func() string { return scene.userCredentials.screenName },
				func() string { return scene.userCredentials.secret },
				func() string { return scene.userCredentials.gatewayURL },
			)
			scene.hudManager.catalogueManager.SetOnPurchaseSucceeded(func(prodID int, posID string) {
				// Open backpack immediately and insert at first slot
				if scene.backpackManager != nil {
					scene.backpackManager.ToggleOpen()
					if prodID > 0 {
						scene.backpackManager.InsertItemAtFront(BackpackItem{
							ProdID:     fmt.Sprintf("%d", prodID),
							PosID:      posID,
							Attributes: map[string]interface{}{},
						})
					}
				}
				// Refresh balance and backpack from gateway
				sn := scene.userCredentials.screenName
				sec := scene.userCredentials.secret
				url := scene.userCredentials.gatewayURL
				if sn != "" && sec != "" && url != "" {
					go func(sn, sec, url string) {
						httpClient := &http.Client{Timeout: 15 * time.Second}
						if bal, err := gateway.GetDenizenBalance(httpClient, url, sn, sec); err == nil {
							scene.hudManager.SetBalance(bal)
							scene.hudManager.RefreshDecibelsWindow()
						}
						// Reload possessions (server truth will replace local insertion)
						scene.LoadPossessionsFromGateway(sn, sec, url)
					}(sn, sec, url)
				}
			})
		}
	}
	// Initialize chat manager (after HUD init creates font manager)
	scene.chatManager = NewChatManager(scene.hudManager.GetFontManager(), scene.tm)
	scene.chatManager.SetFieldRect(scene.hudManager.GetChatFieldRect())
	scene.chatManager.SetSpeechSwitchRect(scene.hudManager.GetSpeechSwitchRect())
	if err := scene.chatManager.LoadAssets(); err != nil {
		// Continue without chat assets if loading fails
	}

	// Initialize InfoStand overlay
	scene.infoStand = NewInfoStand(scene.hudManager.GetFontManager(), scene.tm, dataDir)
	// Set current user name; it can be overridden by caller
	scene.currentUserName = ""
	// Provide current player name for self-button logic
	scene.infoStand.SetCurrentPlayerName(scene.currentUserName)
	// Permissions bridge
	scene.infoStand.IsRoomOwner = scene.IsRoomOwner

	// Wire furniture rotate handler
	scene.infoStand.OnRotateFurni = scene.rotatePlacedFurni
	// Wire furniture move handler
	scene.infoStand.OnMoveFurni = scene.movePlacedFurni
	// Wire furniture pickup handler
	scene.infoStand.OnPickupFurni = scene.pickupPlacedFurni
	// Wire kick handler
	scene.infoStand.OnKick = scene.kickAvatarByName
	// Wire trade handler
	scene.infoStand.OnTrade = func(targetName string) {
		if scene.tradeManager != nil {
			scene.tradeManager.StartTrade(targetName)
		}
	}

	// Load catalogue for mapping prodId -> imageBase (needed for backpack previews)
	catPath := filepath.Join(dataDir, "Catalogue_English.txt")
	if c, err := catalog.LoadFile(catPath); err == nil {
		scene.catalog = c
	}

	// Init placement helper
	scene.placement = NewPlacementHelper(scene)

	// Initialize square walkability for any existing furniture
	scene.placement.updateAllSquareWalkability()

	// Initialize FPS counter
	scene.fpsCounter = ui.NewFPSCounter()

	// Initialize backpack manager
	scene.backpackManager = NewBackpackManager(scene.hudManager.GetWindowManager(), scene.catalog, scene.smallAssets, scene.tm, scene.hudManager.GetFontManager())

	// Set up the callback for loading possessions when backpack opens
	scene.backpackManager.SetLoadPossessionsFunc(func() {
		if scene.userCredentials.screenName != "" && scene.userCredentials.secret != "" && scene.userCredentials.gatewayURL != "" {
			scene.LoadPossessionsFromGateway(scene.userCredentials.screenName, scene.userCredentials.secret, scene.userCredentials.gatewayURL)
		} else {
			log.Printf("Studio: Cannot load possessions - credentials not available")
		}
	})

	// Initialize trade manager
	scene.tradeManager = NewTradeManager(scene)

	// Wire purchase handler: deduct balance and add item to backpack
	scene.hudManager.SetPurchaseHandler(func(imageBase string, price int) bool {
		// Verify balance
		if scene.hudManager.GetBalance() < price {
			return false
		}
		// Resolve ProdID from imageBase using catalog
		var prodID string
		if scene.catalog != nil {
			for _, it := range scene.catalog.Items {
				if strings.EqualFold(strings.TrimSpace(it.ImageBase), strings.TrimSpace(imageBase)) {
					prodID = it.ProdID
					break
				}
			}
		}
		if strings.TrimSpace(prodID) == "" {
			// Fallback: try to parse numeric from alias/imageBase if possible
			prodID = imageBase
		}
		// Deduct balance
		scene.hudManager.AdjustBalance(-price)
		// Add to backpack at front with globally-unique PosID and open if needed
		if scene.backpackManager != nil {
			if strings.TrimSpace(prodID) == "26" {
				scene.backpackManager.blankCDs += 10
				scene.backpackManager.ToggleOpen()
			} else {
				posID := scene.GenerateUniquePosID("pos")
				item := BackpackItem{ProdID: prodID, PosID: posID, Attributes: map[string]interface{}{}}
				scene.backpackManager.InsertItemAtFront(item)
				scene.backpackManager.ToggleOpen()
			}
		}
		return true
	})

	// Initialize lighting system (shader-based neon/glow overlay)
	if ls, err := NewLightingSystem(); err == nil {
		scene.lighting = ls
	}

	// Precreate dim overlay (scene fixed size 960x540)
	dim := ebiten.NewImage(1920, 1080)
	dim.Fill(color.Black)
	scene.dimOverlay = dim

	// Initialize on-screen lighting toggle button (top-right corner)
	// Button size ~90x24 with 10px margin
	scene.lightBtnRect = [4]int{960 - 90 - 10 - 200, 10, 90, 24}
	scene.lightBtnBg = ebiten.NewImage(scene.lightBtnRect[2], scene.lightBtnRect[3])
	// Fill with black; we'll apply alpha during draw
	scene.lightBtnBg.Fill(color.Black)

	// Initialize studio loading overlay
	scene.showStudioLoader = true
	scene.loaderStart = time.Now()
	scene.loaderSteps = 11
	scene.loaderStep = 1
	scene.loaderMinMS = 1200
	// Seed RNG for tip selection
	scene.rng = mrand.New(mrand.NewSource(time.Now().UnixNano()))
	// Load boot frames for consistent visuals
	if fp, err := loadStudioFramePlayer(filepath.Join("assets", "loading", "frames")); err == nil {
		scene.loaderFP = fp
	}
	// Pick a tip similar to entry scene safety tips
	if scene.tm != nil {
		all := scene.tm.All("SAFETY_TIP")
		if len(all) > 0 {
			// Exclude two boot-only tips from studio entry
			ex1 := "Welcome to Coke Studios. Take a minute to review our safety tips."
			ex2 := "If you encounter any problems, please use the Call for Assistance button to gain the assistance of an online monitor."
			filtered := make([]string, 0, len(all))
			for _, t := range all {
				tv := strings.TrimSpace(t)
				if strings.EqualFold(tv, strings.TrimSpace(ex1)) || strings.EqualFold(tv, strings.TrimSpace(ex2)) {
					continue
				}
				filtered = append(filtered, t)
			}
			candidates := filtered
			if len(candidates) == 0 {
				candidates = all
			}
			if scene.rng == nil {
				scene.rng = mrand.New(mrand.NewSource(time.Now().UnixNano()))
			}
			idx := scene.rng.Intn(len(candidates))
			scene.loaderTip = candidates[idx]
		}
	}

	// Disable test avatar now that realtime wiring is active
	// (Leave stub here for offline testing if needed.)
	// if err := scene.initTestAvatar(dataDir); err != nil { }

	// Apply default room patterns (floor and walls) so the room has a style if no server POS arrives
	scene.applyDefaultPatterns()

	// Don't re-initialize HUD and chat managers - they're already initialized above
	// Note: LoadPossessionsFromGateway should be called after scene creation with credentials
	return scene, nil
}

// ApplyFloorPattern applies a floor pattern to a specific element
func (s *Scene) ApplyFloorPattern(elementID, patternName string, colorIndex int) error {
	return s.patternMgr.ApplyFloorPattern(elementID, patternName, colorIndex)
}

// ApplyWallPattern applies a wall pattern to a specific element
func (s *Scene) ApplyWallPattern(elementID, patternName string, colorIndex int) error {
	return s.patternMgr.ApplyWallPattern(elementID, patternName, colorIndex)
}

// ApplyWallpaperFromNetwork applies a wallpaper pattern across all wall elements from
// a numeric texture index (1-based) and color slot (1-based), matching server semantics.
func (s *Scene) ApplyWallpaperFromNetwork(textureIndex, colorIndex int) {
	if s == nil || s.patternMgr == nil {
		return
	}
	name, err := s.patternMgr.GetWallPatternNameByTextureIndex(textureIndex)
	if err != nil {
		fmt.Printf("ApplyWallpaperFromNetwork: invalid texture index %d: %v\n", textureIndex, err)
		return
	}
	// Convert to 0-based color index, clamp to available range
	idx := colorIndex - 1
	if idx < 0 {
		idx = 0
	}
	fmt.Printf("ApplyWallpaperFromNetwork: pattern=%s color=%d (cid=1)\n", name, colorIndex)
	// Apply to all wall ids we render
	wallIDs := []string{
		"walltile_left_1",
		"walltile_right_1",
		"walltile_left_texture",
		"walltile_right_texture",
		"walltile_door_texture",
	}
	for _, id := range wallIDs {
		if err := s.patternMgr.ApplyWallPattern(id, name, idx); err != nil {
			fmt.Printf("ApplyWallpaperFromNetwork: failed for %s: %v\n", id, err)
		}
	}
}

// ApplyFloorFromNetwork applies a floor pattern across all floor texture elements from
// a numeric texture index (1-based) and color slot (1-based), matching server semantics.
func (s *Scene) ApplyFloorFromNetwork(textureIndex, colorIndex int) {
	if s == nil || s.patternMgr == nil || s.roomDef == nil {
		return
	}
	name, err := s.patternMgr.GetFloorPatternNameByTextureIndex(textureIndex)
	if err != nil {
		fmt.Printf("ApplyFloorFromNetwork: invalid texture index %d: %v\n", textureIndex, err)
		return
	}
	idx := colorIndex - 1
	if idx < 0 {
		idx = 0
	}
	fmt.Printf("ApplyFloorFromNetwork: pattern=%s color=%d (cid=2)\n", name, colorIndex)
	for _, element := range s.roomDef.Elements {
		if strings.HasPrefix(element.ID, "floortexture") || element.ID == "doorfloortexture" {
			if err := s.patternMgr.ApplyFloorPattern(element.ID, name, idx); err != nil {
				fmt.Printf("ApplyFloorFromNetwork: failed for %s: %v\n", element.ID, err)
			}
		}
	}
}

// ResetDebugState resets debug flags for fresh debugging
func (s *Scene) ResetDebugState() {
	s.debugPrinted = false
	s.patternMgr.ResetAllDebugStates()
}

// IsRoomOwner returns true if the current local user is the owner of the room.
func (s *Scene) IsRoomOwner() bool {
	if s == nil || s.studio == nil {
		return false
	}
	if strings.TrimSpace(s.currentUserName) == "" {
		return false
	}
	// Compare case-insensitively with studio owner
	return strings.EqualFold(strings.TrimSpace(s.studio.Owner), strings.TrimSpace(s.currentUserName))
}

// GetAvailableFloorPatterns returns all available floor pattern names
func (s *Scene) GetAvailableFloorPatterns() []string {
	return s.patternMgr.GetFloorPatternNames()
}

// GetAvailableWallPatterns returns all available wall pattern names
func (s *Scene) GetAvailableWallPatterns() []string {
	return s.patternMgr.GetWallPatternNames()
}

// GetFloorPatternColorCount returns the number of color variants for a floor pattern
func (s *Scene) GetFloorPatternColorCount(patternName string) int {
	return s.patternMgr.GetFloorPatternColorCount(patternName)
}

// GetWallPatternColorCount returns the number of color variants for a wall pattern
func (s *Scene) GetWallPatternColorCount(patternName string) int {
	return s.patternMgr.GetWallPatternColorCount(patternName)
}

// initIsoEngine initializes the isometric engine for this studio
func (s *Scene) initIsoEngine(dataDir string) error {
	// Load studio XML and PathMap data first (supports public layout 5)
	studioXML, pathMapXML, err := isoengine.LoadStudioData(dataDir, s.studio.Layout)
	if err != nil {
		return fmt.Errorf("failed to load studio data: %w", err)
	}

	s.studioXML = studioXML
	s.pathMapXML = pathMapXML

	// Create iso scene
	s.isoScene = isoengine.NewIsoScene()
	// Provide a bridge so isoengine can use our pattern manager during Draw
	ownerBridge := isoengine.NewSceneOwnerBridge(s.patternMgr)
	s.isoScene.AttachOwner(ownerBridge)
	// Attach the iso scene back on the bridge so it can report tile info
	ownerBridge.AttachIsoScene(s.isoScene)
	// Attach draw depth provider for console reporting (also used for boost control via type-assert)
	ownerBridge.AttachDrawDepthProvider(s)
	// Connect the manager back to the bridge for asset reports
	s.patternMgr.AttachSceneReporter(ownerBridge)

	// Convert studio layout to iso layout
	// Public rooms (layout 5) -> layout 1 (30x30, smaller tiles)
	// Private studios (1-4)   -> layout 2 (15x15, larger tiles)
	isoLayout := isoengine.ConvertStudioLayoutToIsoLayout(s.studio.Layout)
	s.isoScene.SetLayout(isoLayout)

	// Override the grid offsets with the actual Studio XML offsets
	s.isoScene.SetCustomOffsets(studioXML.XOff, studioXML.YOff)

	s.isoScene.Init()


		// Load room background image (Studio XML preferred; fallback to PathMap)
		if err := s.loadStudioBackground(); err != nil {
			log.Printf("Studio: background load: %v", err)
		}

	// Apply PathMap data to the iso scene
	err = isoengine.ApplyPathMapToScene(s.isoScene, pathMapXML)
	if err != nil {
		return fmt.Errorf("failed to apply PathMap to scene: %w", err)
	}

	// Load wall data from Studio XML
	err = s.isoScene.LoadWallData(studioXML)
	if err != nil {
		return fmt.Errorf("failed to load wall data: %w", err)

	}

	// Leave iso debug grid off by default; toggle with F2/F3
	s.isoScene.EnableDebug(false)

	// For public rooms, use smaller avatar scale (50%) to match original Lingo
	// Note: Avatar scaling is applied at draw time for public rooms

	return nil
}

// loadStudioBackground loads the room background image and position from Studio XML (preferred)
// and falls back to PathMap bgimage if needed.
func (s *Scene) loadStudioBackground() error {
	// Determine asset directory based on layout
	studioDir := filepath.Join("Lingo", "cc_studio")
	if s.studio != nil && s.studio.Layout == 5 {
		studioDir = filepath.Join("Lingo", "london")
	}

	// Prefer Studio XML Background member (maps via Members.csv to e.g., 3_cc.london.bg.png)
	if s.studioXML != nil && strings.TrimSpace(s.studioXML.Background.Member) != "" {
		if mm, err := lingoassets.LoadMemberMapCached(studioDir); err == nil && mm != nil {
			member := strings.TrimSpace(s.studioXML.Background.Member)
			if p, ok := mm.PathFor(member); ok {
				if img, err2 := lingoassets.LoadImage(p); err2 == nil {
					s.backgroundImg = img
					s.bgPos = [2]int{s.studioXML.Background.X, s.studioXML.Background.Y}
					return nil
				}
			}
		}
	}

	// Fallback: PathMap bgimage and position
	if s.pathMapXML != nil && strings.TrimSpace(s.pathMapXML.BGImage) != "" {
		p := filepath.Join(studioDir, strings.TrimSpace(s.pathMapXML.BGImage))
		if img, err := lingoassets.LoadImage(p); err == nil {
			s.backgroundImg = img
			s.bgPos = [2]int{s.pathMapXML.BGX, s.pathMapXML.BGY}
			return nil
		}
	}

	return fmt.Errorf("background not found")
}


// SetDepthBoostSlicePerPos sets a per-slice depth boost for a specific position id
func (s *Scene) SetDepthBoostSlicePerPos(posID string, slice string, amount int) string {
	posID = strings.TrimSpace(posID)
	slice = strings.ToLower(strings.TrimSpace(slice))
	if posID == "" {
		return "posId required"
	}
	if slice != "a" && slice != "b" && slice != "c" && slice != "d" {
		return "slice must be one of a,b,c,d"
	}
	if s.depthBoostsBySlicePerPos == nil {
		s.depthBoostsBySlicePerPos = make(map[string]map[string]int)
	}
	if s.depthBoostsBySlicePerPos[posID] == nil {
		s.depthBoostsBySlicePerPos[posID] = make(map[string]int)
	}
	s.depthBoostsBySlicePerPos[posID][slice] = amount
	return fmt.Sprintf("Depth boost set: posId=%s slice=%s amount=%d", posID, slice, amount)
}

// ClearDepthBoostSlicePerPos clears all slice depth boosts for a specific position id
func (s *Scene) ClearDepthBoostSlicePerPos(posID string) {
	posID = strings.TrimSpace(posID)
	if posID == "" {
		return
	}
	if s.depthBoostsBySlicePerPos != nil {
		delete(s.depthBoostsBySlicePerPos, posID)
	}
}

// applySofaSliceBoosts applies automatic slice depth boosting for sofa items based on direction
func (s *Scene) applySofaSliceBoosts(pf PlacedFurni) {
	// Check if this is a sofa
	isSofa := false


	if s.catalog != nil {
		if ci := s.catalog.GetItemByID(pf.ProdID); ci != nil && strings.EqualFold(ci.Type, "sofa") {
			isSofa = true
		}
	}
	// Fallback check by base name
	if !isSofa {
		b := strings.ToLower(pf.Base)
		isSofa = strings.Contains(b, "sofa") || strings.Contains(b, "couch")
	}

	if !isSofa {
		return
	}

	// Clear any existing slice boosts for this specific PosID first
	s.ClearDepthBoostSlicePerPos(pf.PosID)

	// Apply direction-specific slice boosts
	switch pf.Dir {
	case 2:
		// dir 2: do nothing (no boosts)
		s.SetDepthBoostSlicePerPos(pf.PosID, "d", 10000)
		s.SetDepthBoostSlicePerPos(pf.PosID, "b", 10000)
		s.SetDepthBoostSlicePerPos(pf.PosID, "a", 10000)
		s.SetDepthBoostSlicePerPos(pf.PosID, "c", 10000)

	case 4:
		// dir 4: boost slice b 10002, boost slice a 10000
		s.SetDepthBoostSlicePerPos(pf.PosID, "b", 10002)
		s.SetDepthBoostSlicePerPos(pf.PosID, "a", 10000)

	case 6:
		// dir 6: boost slice a 1, boost slice b 6, boost slice d 6
		s.SetDepthBoostSlicePerPos(pf.PosID, "a", 1)
		s.SetDepthBoostSlicePerPos(pf.PosID, "b", 10006)
		s.SetDepthBoostSlicePerPos(pf.PosID, "d", 10006)

	case 0:
		// dir 0: boost slice d 1, boost slice b 10002, boost slice a 10000
		s.SetDepthBoostSlicePerPos(pf.PosID, "d", 1)
		s.SetDepthBoostSlicePerPos(pf.PosID, "b", 10002)
		s.SetDepthBoostSlicePerPos(pf.PosID, "a", 10000)

	default:
		// Unknown direction, no boosts applied
	}
}

// applyPinballSliceBoosts applies slice depth boosting for Pinball items
// Current rule: for direction 2, boost slice "a" by 10000.
func (s *Scene) applyPinballSliceBoosts(pf PlacedFurni) {
	// Ensure we have catalog data to check type; also allow fallback by base name
	isPinball := false
	if s.catalog != nil {
		if ci := s.catalog.GetItemByID(strings.TrimSpace(pf.ProdID)); ci != nil && strings.EqualFold(ci.Type, "Pinball") {
			isPinball = true
		}
	}
	if !isPinball {
		b := strings.ToLower(pf.Base)
		if strings.Contains(b, "pinball") {
			isPinball = true
		}
	}

	if !isPinball {
		return
	}

	// Do not clear all existing boosts; we only set the one we need so it can coexist
	if pf.Dir == 2 {
		s.SetDepthBoostSlicePerPos(pf.PosID, "a", 10000)
		s.SetDepthBoostSlicePerPos(pf.PosID, "c", 50)
	}
	if pf.Dir == 4 {
		s.SetDepthBoostSlicePerPos(pf.PosID, "a", -25)
		s.SetDepthBoostSlicePerPos(pf.PosID, "c", -10025)
	}
}

// applyVanillaCokeTableSliceBoosts applies slice depth boosting for Vanilla Coke Table items
// Rule: always boost slice "a" by 10000 so its top slice renders above neighbors consistently.
func (s *Scene) applyVanillaCokeTableSliceBoosts(pf PlacedFurni) {
	// Identify by catalog image base first, then fallback to base name substring
	isVCT := false
	if s.catalog != nil {
		if ci := s.catalog.GetItemByID(strings.TrimSpace(pf.ProdID)); ci != nil && strings.EqualFold(strings.TrimSpace(ci.ImageBase), "vanillacoketable") {
			isVCT = true
		}
	}
	if !isVCT {
		b := strings.ToLower(strings.TrimSpace(pf.Base))
		if strings.Contains(b, "vanillacoketable") {
			isVCT = true
		}
	}
	if !isVCT {
		return
	}
	// Apply per-PosID slice boost for layer 'a'
	s.SetDepthBoostSlicePerPos(pf.PosID, "a", 9993)
}

// loadRoomDefinition loads the room definition file based on studio layout
func (s *Scene) loadRoomDefinition(dataDir string) error {
	// Public rooms (layout 5) use location-specific room files
	var roomPath string
	if s.studio != nil && s.studio.Layout == 5 {
		roomPath = filepath.Join("Lingo", "london", "1_london.room.txt")
	} else {
		// Map layout numbers to model letters for private studios
		layoutMap := map[int]string{
			1: "a",
			2: "b",
			3: "c",
			4: "d",
		}

		modelLetter, exists := layoutMap[s.studio.Layout]
		if !exists {
			return fmt.Errorf("unsupported studio layout: %d", s.studio.Layout)
		}

		roomFile := fmt.Sprintf("%d_model_%s.room.txt", s.studio.Layout, modelLetter)
		roomPath = filepath.Join("Lingo", "cc_studio", roomFile)
	}

	roomDef, err := studioroom.ParseRoomFile(roomPath)
	if err != nil {
		return fmt.Errorf("failed to parse room file %s: %w", roomPath, err)
	}

	s.roomDef = roomDef
	s.roomRect = roomDef.Rect
	s.offsetX = roomDef.RoomData.OffsetX
	s.offsetY = roomDef.RoomData.OffsetY

	return nil
}

// loadMemberRegistry loads the Members.csv file for registration points
func (s *Scene) loadMemberRegistry(dataDir string) error {
	membersPath := filepath.Join("Lingo", "cc_studio", "Members.csv")
	if s.studio != nil && s.studio.Layout == 5 {
		membersPath = filepath.Join("Lingo", "london", "Members.csv")
	}

	memberReg, err := studioroom.LoadMemberRegistry(membersPath)
	if err != nil {
		return fmt.Errorf("failed to load member registry: %w", err)
	}

	s.memberReg = memberReg
	return nil
}

// loadRoomAssets loads all assets needed for the room
func (s *Scene) loadRoomAssets(dataDir string) error {
	studioDir := filepath.Join("Lingo", "cc_studio")
	if s.studio != nil && s.studio.Layout == 5 {
		studioDir = filepath.Join("Lingo", "london")
	}

	// Track unique assets to load
	assetsToLoad := make(map[string]bool)

	// Collect all member names from room elements
	roomMembers := make(map[string]bool)
	for _, element := range s.roomDef.Elements {
		if element.Member != "" {
			assetsToLoad[element.Member] = true
			roomMembers[element.Member] = true
		}
	}

	// Also ensure hiliter member from Studio XML is loaded (public rooms like London)
	if s.studioXML != nil && s.studioXML.Hiliter.Member != "" {
		assetsToLoad[s.studioXML.Hiliter.Member] = true
	}

	// For public rooms, include StaticItems from Studio XML that are not already in the .room file
	if s.studio != nil && s.studio.Layout == 5 && s.studioXML != nil {
		for _, it := range s.studioXML.StaticItems.Items {
			if it.Member == "" { continue }
			if !roomMembers[it.Member] {
				assetsToLoad[it.Member] = true
			}
		}
	}

	// Prepare member map once
	mm, err := lingoassets.LoadMemberMapCached(studioDir)
	if err != nil {
		// Continue without member map
	}

	// For public rooms, also include Furniture from Studio XML (e.g., london_chair/table/cokecase)
	if s.studio != nil && s.studio.Layout == 5 && s.studioXML != nil && mm != nil {
		cols := 30
		if s.pathMapXML != nil && s.pathMapXML.Cols > 0 {
			cols = s.pathMapXML.Cols
		}
		log.Printf("London: Studio XML Furniture count = %d (grid cols=%d)", len(s.studioXML.Furniture.Items), cols)
		addIfExists := func(name string) {
			if name == "" { return }
			if p, ok := mm.PathFor(name); ok {
				assetsToLoad[name] = true
				log.Printf("London: will load furniture member %q -> %s", name, p)
			}
		}
		mirrorEven := func(d int) int {
			switch d {
			case 2:
				return 4
			case 4:
				return 2
			case 0:
				return 6
			case 6:
				return 0
			default:
				return d
			}
		}
		for _, f := range s.studioXML.Furniture.Items {
			base := strings.TrimSpace(f.ImageBase)
			if base == "" { continue }
			serverX, serverZ := f.GridX, f.GridZ
			clientRow := serverX
			clientCol := (cols + 1) - serverZ
			log.Printf("London: Furniture base=%s dir=%d server(x=%d,z=%d) -> client(row=%d,col=%d)", base, f.Dir, serverX, serverZ, clientRow, clientCol)
			// Attempt to preload slices 'a' and 'b' for dir and its mirrored-even fallback
			w, d := 1, 1
			dirs := []int{f.Dir, mirrorEven(f.Dir)}
			for _, dirVal := range dirs {
				for _, o := range []string{"a", "b"} {
					cand := fmt.Sprintf("%s_%s_%d_%d_%d_%d_%d", base, o, 0, w, d, dirVal, 0)
					addIfExists(cand)
				}
			}
		}
	}
	if err != nil {
		// Continue without member map
	}

	// Load each asset
	for assetName := range assetsToLoad {
		if err := s.loadAsset(studioDir, assetName, mm); err != nil {
			// Continue loading other assets even if one fails
		}
	}
	return nil
}

// loadAsset loads a single asset by name
func (s *Scene) loadAsset(studioDir, assetName string, mm *lingoassets.MemberMap) error {
	// Resolve via Members.csv (handles numeric prefixes)
	var assetPath string
	if mm != nil {
		if p, ok := mm.PathFor(assetName); ok {
			assetPath = p
		}
	}
	// Fallback to direct filename when not listed in member map
	if assetPath == "" {
		assetPath = filepath.Join(studioDir, assetName+".png")
	}

	// Attempt to load
	img, err := lingoassets.LoadImage(assetPath)
	if err == nil {
		s.assets[assetName] = img
		return nil
	}

	// As a final fallback, try to find a numbered file that ends with the asset name
	// (only on desktop builds; not reliable on WASM due to lack of directory listing)
	if strings.HasPrefix(assetName, "studiofloor_") || strings.HasPrefix(assetName, "studio_model_") || strings.HasPrefix(assetName, "floor_shape_") {
		// Try common prefixes 0..300 to minimize requests
		for i := 0; i < 300; i++ {
			candidate := filepath.Join(studioDir, fmt.Sprintf("%d_%s.png", i, assetName))
			if img2, err2 := lingoassets.LoadImage(candidate); err2 == nil {
				s.assets[assetName] = img2
				return nil
			}
		}
	}

	return fmt.Errorf("asset file not found for %s", assetName)
}

// containsAssetName checks if a filename contains the asset name
func containsAssetName(filename, assetName string) bool {
	// Remove file extension
	name := filename
	if idx := len(filename) - 4; idx > 0 && filename[idx:] == ".png" {
		name = filename[:idx]
	}

	// Check for exact match or if the asset name is contained in the filename
	// This handles cases like "22_studio_model_b.png" for asset "studio_model_b"
	return name == assetName ||
		(len(name) > len(assetName) && name[len(name)-len(assetName):] == assetName)
}

// Update implements the Scene interface
func (s *Scene) Update() error {
	prev := s.last
	now := time.Now()
	s.last = now

	// If showing studio loader, update animation, simulate progress and block interactions
	if s.showStudioLoader {
		if s.loaderFP != nil && !prev.IsZero() {
			s.loaderFP.Update(now.Sub(prev))
		}
		// Advance step every ~90ms up to loaderSteps
		elapsedMS := time.Since(s.loaderStart).Milliseconds()
		maxStep := int(elapsedMS/90) + 1
		if maxStep > s.loaderSteps {
			maxStep = s.loaderSteps
		}
		if s.loaderStep < maxStep {
			s.loaderStep = maxStep
		}
		// Keep loader for at least loaderMinMS
		if elapsedMS >= int64(s.loaderMinMS) && s.loaderStep >= s.loaderSteps {
			s.showStudioLoader = false
		}
		return nil
	}

	// Drain pending AV events (queued from websocket thread)
	if len(s.pendingAVs) > 0 {
		s.pendingMu.Lock()
		events := make([]avEvent, len(s.pendingAVs))
		copy(events, s.pendingAVs)
		s.pendingAVs = s.pendingAVs[:0]
		s.pendingMu.Unlock()
		for _, ev := range events {
			s.UpsertAvatarAtGrid(ev.name, ev.def, ev.mission, ev.gender, ev.dir, ev.row, ev.col, ev.isBot)
		}
	}

	// Drain pending POS events (queued from websocket thread)
	if len(s.pendingPOS) > 0 {
		s.pendingMu.Lock()
		items := make([]posEvent, len(s.pendingPOS))
		copy(items, s.pendingPOS)
		s.pendingPOS = s.pendingPOS[:0]
		s.pendingMu.Unlock()
		// Ensure placement order: base items first (ho=0), then stacked (ho>0)
		sort.SliceStable(items, func(i, j int) bool { return items[i].ho < items[j].ho })
		for _, ev := range items {
			fmt.Printf("[POS->Scene] enqueue id=%s prod=%s x=%d y=%d z=%d ho=%d d=%d st=%d iaot=%t\n", ev.posID, ev.prodID, ev.x, ev.y, ev.z, ev.ho, ev.dir, ev.state, ev.iaot)
			if s.placement != nil {
				_ = s.placement.PlaceFromServer(ev.posID, ev.prodID, ev.x, ev.y, ev.z, ev.ho, ev.dir, ev.state, ev.iaot)
			}
		}
	}
	// Drain pending movement events
	if len(s.pendingMV) > 0 {
		s.pendingMu.Lock()
		moves := make([]mvEvent, len(s.pendingMV))
		copy(moves, s.pendingMV)
		s.pendingMV = s.pendingMV[:0]
		s.pendingMu.Unlock()
		for _, mv := range moves {
			s.MoveAvatarToGrid(mv.name, mv.row, mv.col)
		}
	}

	// Arrival-gated drink dispenser trigger (handled after avatar updates below)

	// Drain pending action events (dance start/stop, speaking)
	if len(s.pendingACT) > 0 {
		s.pendingMu.Lock()
		acts := make([]actEvent, len(s.pendingACT))
		copy(acts, s.pendingACT)
		s.pendingACT = s.pendingACT[:0]
		s.pendingMu.Unlock()
		for _, ev := range acts {
			s.ApplyAvatarAction(ev.name, ev.act, ev.dir)
		}
	}

	// Drain pending avatar removals
	if len(s.pendingRM) > 0 {
		s.pendingMu.Lock()
		names := make([]string, len(s.pendingRM))
		copy(names, s.pendingRM)
		s.pendingRM = s.pendingRM[:0]
		s.pendingMu.Unlock()
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			// Do not remove the local player
			if strings.EqualFold(name, strings.TrimSpace(s.currentUserName)) {
				continue
			}
			// Find avatar and remove from arrays/maps
			if av, ok := s.avatarsByName[name]; ok && av != nil {
				// Remove from slice
				idx := -1
				for i := range s.avatars {
					if s.avatars[i] == av {
						idx = i
						break
					}
				}
				if idx >= 0 {
					s.avatars = append(s.avatars[:idx], s.avatars[idx+1:]...)
				}
				// Remove from map
				delete(s.avatarsByName, name)
			}
		}
	}

	// Drain pending chat messages
	if len(s.pendingCH) > 0 {
		s.pendingMu.Lock()
		chats := make([]chatEvent, len(s.pendingCH))
		copy(chats, s.pendingCH)
		s.pendingCH = s.pendingCH[:0]
		s.pendingMu.Unlock()
		log.Printf("Studio: processing %d pending chat messages", len(chats))
		for _, ev := range chats {
			log.Printf("Studio: chat event for %q: %q (mode=%d)", ev.name, ev.message, ev.speechMode)
			// Find avatar by name and emit chat bubble
			if av, ok := s.avatarsByName[ev.name]; ok && av != nil {
				log.Printf("Studio: found avatar %q, emitting bubble", ev.name)
				if s.chatManager != nil {
					// Set speech mode based on sm value
					prevMode := s.chatManager.GetSpeechMode()
					if ev.speechMode == 2 {
						s.chatManager.speechMode = SpeechModeShout
					} else {
						s.chatManager.speechMode = SpeechModeSpeak
					}
					s.chatManager.EmitChatBubble(ev.message, av)
					// Restore previous mode
					s.chatManager.speechMode = prevMode
				} else {
					log.Printf("Studio: chatManager is nil!")
				}
			} else {
				log.Printf("Studio: avatar %q not found in avatarsByName", ev.name)
			}
		}
	}

	// Update FPS counter
	if s.fpsCounter != nil {
		s.fpsCounter.Update()
	}

	// Update pattern console
	s.patternConsole.Update()

	// Update HUD system
	if s.hudManager != nil {
		if err := s.hudManager.Update(); err != nil {
			return err
		}
	}

	// Update backpack manager
	if s.backpackManager != nil {
		if err := s.backpackManager.Update(); err != nil {
			return err
		}
	}

	// Handle F2 key for debug cycle (just-pressed)
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) && s.isoScene != nil {
		grid := s.isoScene.GetGrid()
		if grid != nil {
			grid.CycleDebugMode()
		}
		// Toggle PosID labels on backpack and placed items
		if s.backpackManager != nil {
			s.backpackManager.showPosLabels = !s.backpackManager.showPosLabels
		}
	}

	// Handle F3 key specifically for grid debug toggle (just-pressed)
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) && s.isoScene != nil {
		grid := s.isoScene.GetGrid()
		if grid != nil {
			if !grid.ShowDebugGrid {
				grid.EnableDebugGrid(true)
				grid.DebugMode = 3 // overlays with delineation by default
			} else {
				grid.EnableDebugGrid(false)
			}
		}
	}

	// Toggle lighting with F9 (debounced)
	if ebiten.IsKeyPressed(ebiten.KeyF9) {
		if time.Since(s.lastF9Press) > 300*time.Millisecond {
			if s.lighting != nil {
				s.lighting.SetEnabled(!s.lighting.Enabled())
				fmt.Printf("[lighting] toggled: %v\n", s.lighting.Enabled())
			}
			s.lastF9Press = time.Now()
		}
	}

	// Touch/click lighting toggle button (for mobile or mouse)
	if s.handleLightingTouchToggle() {
		return nil // consume click to avoid click-through
	}

	// Keybinds for avatar testing (debounced ~200ms)
	if s.playerAvatar != nil {
		debounce := 200 * time.Millisecond

		if ebiten.IsKeyPressed(ebiten.KeyF5) {
			if time.Since(s.lastF5Press) > debounce {
				// Chat (speaking)
				s.playerAvatar.Engine.StartChatting()
				s.lastF5Press = time.Now()
			}
		}

		if ebiten.IsKeyPressed(ebiten.KeyF6) {
			if time.Since(s.lastF6Press) > debounce {
				// Cycle emotes
				emotes := []string{"sml", "agr", "srp", "sad"}
				s.emoteIndex = (s.emoteIndex + 1) % len(emotes)
				s.playerAvatar.Engine.StartEmote(emotes[s.emoteIndex])
				s.lastF6Press = time.Now()
			}
		}

		if ebiten.IsKeyPressed(ebiten.KeyF7) {
			if time.Since(s.lastF7Press) > debounce {
				// Toggle carry
				s.carryingOn = !s.carryingOn
				if s.carryingOn {
					s.playerAvatar.Engine.StartCarrying()
				} else {
					s.playerAvatar.Engine.StopCarrying()
				}
				s.lastF7Press = time.Now()
			}
		}

		if ebiten.IsKeyPressed(ebiten.KeyF8) {
			if time.Since(s.lastF8Press) > debounce {
				// Toggle drink (only meaningful when carrying)
				s.drinkingOn = !s.drinkingOn
				if s.drinkingOn {
					s.playerAvatar.Engine.StartDrinking()
				} else {
					s.playerAvatar.Engine.StopDrinking()
				}
				s.lastF8Press = time.Now()
			}
		}

		// Block click-through when any UI window is under the mouse cursor
		allowStudioClick := true
		if s.hudManager != nil && s.hudManager.GetWindowManager() != nil && s.hudManager.GetWindowManager().IsMouseOverAnyWindow() {
			allowStudioClick = false
		}
		// Prevent movement input and stop ongoing movement while trading
		if s.tradeManager != nil && s.tradeManager.IsOpen() {
			allowStudioClick = false
			if s.playerAvatar != nil {
				s.playerAvatar.StopMovement()
			}
		}
		holdingActive := s.backpackManager != nil && s.backpackManager.IsHoldingActive()
		// Debug: print raw click state once when mouse just pressed
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			isWalk := s.isoScene != nil && s.isoScene.IsMouseOverWalkableTile()
			mx, my := ebiten.CursorPosition()
			log.Printf("Studio click: mouseDown at (%d,%d) allow=%t holding=%t tradeOpen=%t walkable=%t",
				mx, my, allowStudioClick, holdingActive, s.tradeManager != nil && s.tradeManager.IsOpen(), isWalk)
		}
		// Block movement when a window is under mouse cursor
		canMoveClick := allowStudioClick
		if canMoveClick && !holdingActive && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			if s.isoScene.IsMouseOverWalkableTile() {
				if targetSquare := s.isoScene.GetMouseSquare(); targetSquare != nil {
					// If clicking the tile we're already on, do nothing (do not start walking)
					if s.playerAvatar != nil && s.playerAvatar.Square != nil {
						cr, cc := s.playerAvatar.Square.GetRow(), s.playerAvatar.Square.GetCol()
						if cr == targetSquare.GetRow() && cc == targetSquare.GetCol() {
							// No-op: already on this tile
						} else {
							// Get the starting square for path calculation
							// If currently moving, use the next committed square (where we'll be after current step)
							// Otherwise use the current square
							start := s.playerAvatar.GetNextCommittedSquare()
							grid := s.isoScene.GetGrid()
							path := isoengine.FindPath(start, targetSquare, grid)
							if len(path) > 0 {
								log.Printf("Studio click: start [%d,%d] -> target [%d,%d], pathLen=%d", start.GetRow(), start.GetCol(), targetSquare.GetRow(), targetSquare.GetCol(), len(path))
								// Convert to client row/col points (including start and end)
								pts := make([][2]int, 0, len(path))
								for _, sq := range path {
									pts = append(pts, [2]int{sq.GetRow(), sq.GetCol()})
								}
								// Defer actual network send to net layer via studio Scene callback
								if s.sendMovePathIfAvailable != nil {
									log.Printf("Studio click: sending AV_GSP with %d points for room %s", len(pts), s.studio.StudioID)
									s.sendMovePathIfAvailable(s.studio.StudioID, pts)
								} else {
									// Fallback: no network sender wired; perform local move for interactivity
									log.Printf("Studio click: no sender wired; moving locally as fallback")
									s.playerAvatar.Engine.StartWalking()
									s.playerAvatar.MoveToSquare(targetSquare, s.isoScene.GetGrid())
									s.playerHasMoved = true
								}
							}
							// If sender exists, only stop if not already moving
							// If already moving, MoveToSquare will queue as pending
							if s.sendMovePathIfAvailable != nil {
								// Don't stop movement - let the avatar continue and queue the new move
								// The server will be updated with the new path starting from next tile
							}
						}
					} else {
						// Fallback if player or square missing; proceed with move
						// No server send because we don't know start square reliably; move locally
						log.Printf("Studio click: missing player or square; moving locally")
						if s.playerAvatar != nil {
							s.playerAvatar.Engine.StartWalking()
							s.playerAvatar.MoveToSquare(targetSquare, s.isoScene.GetGrid())
							s.playerHasMoved = true
						}
					}
				}
			}
		}

		// Handle InfoStand interactions and selection on click
		mouseX, mouseY := ebiten.CursorPosition()
		mouseClicked := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
		mouseReleased := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)
		mousePressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
		if s.infoStand != nil {
			if s.infoStand.HandleMouse(mouseX, mouseY, mousePressed, mouseClicked, mouseReleased) {
				// If InfoStand consumed the click, skip room interactions this frame
				return nil
			}
		}
		// Handle furniture clicks first
		if mouseClicked && allowStudioClick && !holdingActive {
			mx, my := ebiten.CursorPosition()
			furnitureClicked := false

			// Check if clicking on furniture
			if s.placement != nil {
				if hit, ok := s.placement.HitTestAt(mx, my); ok {
					furnitureClicked = true

					// Check for DRINK_DISPENSER action first (single click)
					if s.catalog != nil {
						if ci := s.catalog.GetItemByID(strings.TrimSpace(hit.ProdID)); ci != nil {
							log.Printf("Clicked furniture %s with action: %s", hit.ProdID, ci.Action)
							if ci.Action == "DRINK_DISPENSER" || ci.Action == "DRINK_DESPENSER" { // Note: typo in original data
								// Compute adjacent target and move there; upon arrival, request drink
								s.startDrinkDispenserApproach(hit)
							} else if ci.Action == "STEREO" {
								// Move adjacent to the stereo and, upon arrival, send CD_R with PosID
								s.startStereoApproach(hit)
							}
						}
					}

					// Also show info stand for the furniture
					if s.infoStand != nil {
						s.infoStand.ShowForFurni(hit)
						// Populate name/desc if catalog available
						if s.catalog != nil {
							if ci := s.catalog.GetItemByID(strings.TrimSpace(hit.ProdID)); ci != nil {
								s.infoStand.PopulateFurniText(ci.Name, ci.RoomDesc)
							}
						}
					}
				}
			}

			// If not clicking on furniture, check for avatar clicks
			if !furnitureClicked {
				selected := false
				for _, av := range s.avatars {
					if TryGetAvatarHit(av, mouseX, mouseY) {
						if s.infoStand != nil {
							s.infoStand.ShowForAvatar(av)
						}
						selected = true
						break
					}
				}

				// Clear info stand if clicking on empty space
				if !selected && s.infoStand != nil {
					if s.nowPlayingActive {
						// Restore Now Playing
						s.infoStand.nowPlayingActive = true
						// Clear target contexts so draw uses custom title/body/icon
						s.infoStand.selectedAvatar = nil
						s.infoStand.selectedFurni = nil
						s.infoStand.previewAvatar = nil
						s.infoStand.visible = true
						// Rebuild vote buttons based on nowPlayingPlayer vs current user
						s.infoStand.rebuildButtonsForAvatar()
					} else {
						s.infoStand.Clear()
					}
				}
			}
		}

		// Double-click toggles furniture active animation (approximate Lingo ACTION_TOGGLE_*)
		if allowStudioClick && !holdingActive && mouseClicked {
			if s.isoScene != nil && s.isoScene.IsDoubleClick() {
				mx, my := ebiten.CursorPosition()
				var target *PlacedFurni
				if s.placement != nil {
					if hit, ok := s.placement.HitTestAt(mx, my); ok {
						target = hit
					}
				}
				if target != nil {
					if target.State == 0 {
						target.State = 1
						// Start active loop at first on-frame
						target.Frame = 1
						target.Animate = target.NumFrames > 0
						target.LastFrameTS = time.Now().UnixMilli()
					} else {
						// Trigger a one-shot power-off frame (e.g., frame 8) if available, then deactivate
						if target.AnimFrames != nil && target.AnimFrames[target.DriverSlice] >= 8 {
							// Show frame 8 briefly (~150ms)
							target.OneShotFrame = 8
							target.OneShotUntil = time.Now().Add(150 * time.Millisecond).UnixMilli()
						}
						target.State = 0
						target.Animate = false
						// Keep last shown oneshot for a short time; frame will reset to 0 after window
					}
				}
			}
		}
	}

	// Recompute avatar occupancy blocking before updating avatars/pathfinding
	if s.isoScene != nil {
		grid := s.isoScene.GetGrid()
		if grid != nil {
			// Clear previous occupancy flags
			for r := 1; r <= grid.Rows; r++ {
				for c := 1; c <= grid.Cols; c++ {
					if sq := grid.GetSquareByRowCol(r, c); sq != nil {
						sq.SetAvatarOccupied(false)
					}
				}
			}
			// Mark current avatar positions as occupied (players and bots)
			for _, av := range s.avatars {
				if av != nil && av.GetSquare() != nil {
					av.GetSquare().SetAvatarOccupied(true)
				}
			}
		}
	}

	// Update all avatars
	for _, avatar := range s.avatars {
		avatar.Update()
	}

	// Arrival-gated drink dispenser trigger: run after avatar.Update() so squares are committed on arrival
	if s.pendingDrinkActive && s.playerAvatar != nil {
		// Use next committed square if mid-step, else current square
		var sq *isoengine.IsoSquare
		if s.playerAvatar.IsMoving() && s.playerAvatar.GetNextCommittedSquare() != nil {
			sq = s.playerAvatar.GetNextCommittedSquare()
		} else {
			sq = s.playerAvatar.Square
		}
		if sq != nil {
			cr, cc := sq.GetRow(), sq.GetCol()
			log.Printf("DrinkApproach: arrival check sq=[%d,%d] target=[%d,%d] moving=%t", cr, cc, s.pendingDrinkTargetRow, s.pendingDrinkTargetCol, s.playerAvatar.IsMoving())
			if cr == s.pendingDrinkTargetRow && cc == s.pendingDrinkTargetCol {
				// Arrived at target; send drink request once
				s.pendingDrinkActive = false
				if s.sendDrinkDispenserIfAvailable != nil && !s.playerHoldingCoke {
					_ = s.sendDrinkDispenserIfAvailable(s.studio.StudioID)
				}
			} else if time.Now().After(s.pendingDrinkExpire) {
				// Failed to reach in time or movement stopped; cancel
				s.pendingDrinkActive = false
			}
		}
	}

	// Arrival-gated stereo trigger: on arrival send CD_R with pendingStereoPosID
	if s.pendingStereoActive && s.playerAvatar != nil {
		var sq *isoengine.IsoSquare
		if s.playerAvatar.IsMoving() && s.playerAvatar.GetNextCommittedSquare() != nil {
			sq = s.playerAvatar.GetNextCommittedSquare()
		} else {
			sq = s.playerAvatar.Square
		}
		if sq != nil {
			cr, cc := sq.GetRow(), sq.GetCol()
			log.Printf("StereoApproach: arrival check sq=[%d,%d] target=[%d,%d] moving=%t", cr, cc, s.pendingStereoTargetRow, s.pendingStereoTargetCol, s.playerAvatar.IsMoving())
			if cr == s.pendingStereoTargetRow && cc == s.pendingStereoTargetCol {
				// Arrived at target
				s.pendingStereoActive = false
				if s.nowPlayingActive {
					if s.hudManager != nil {
						s.hudManager.OpenGeneralAlert("Attention", "There's a song already playing. Wait for your turn.")
					}
				} else {
					if s.sendCDRequestIfAvailable != nil && strings.TrimSpace(s.pendingStereoPosID) != "" {
						_ = s.sendCDRequestIfAvailable(s.studio.StudioID, s.pendingStereoPosID)
					}
				}
			} else if time.Now().After(s.pendingStereoExpire) {
				// Timed out; cancel
				s.pendingStereoActive = false
			}
		}
	}

	// Handle decibels window timeout
	if s.showingDecibels && time.Now().After(s.decibelsShowUntil) {
		s.showingDecibels = false
		if s.decibelsWindow != nil {
			s.decibelsWindow.Hide()
		}
	}

	// Handle player coke holding state
	if s.playerHoldingCoke {
		// Check if 5 minutes has passed
		if time.Now().After(s.cokeHoldUntil) {
			s.playerHoldingCoke = false
			if s.playerAvatar != nil {
				s.playerAvatar.Engine.SetHoldingItem("")
				// Make sure we stop drinking if we were
				if s.playerAvatar.Engine.IsDrinking {
					s.playerAvatar.Engine.StopDrinking()
				}
			}
		} else if s.playerAvatar != nil {
			// Handle drinking animation cycles
			isDrinking := s.playerAvatar.Engine.IsDrinking

			// If currently drinking, check if 500ms has passed to stop
			if isDrinking {
				if time.Since(s.lastCokeDrinkTime) >= 500*time.Millisecond {
					// Stop drinking
					s.playerAvatar.Engine.StopDrinking()
					log.Printf("Coke drink animation ended")
				}
			} else {
				// Not currently drinking - check if we should start a drink cycle
				// Wait at least 5 seconds between drinks
				timeSinceLastDrink := time.Since(s.lastCokeDrinkTime)
				if s.lastCokeDrinkTime.IsZero() {
					// First drink - wait 3 seconds after getting the coke
					timeSinceLastDrink = time.Since(s.cokeHoldUntil.Add(-5 * time.Minute))
				}

				if timeSinceLastDrink >= 5*time.Second {
					// Use a better random check - roughly 30% chance per second
					// This means we'll drink roughly every 3-10 seconds
					if s.rng.Intn(100) < 2 { // 2% chance per frame at 60fps = ~30% per second
						// Start drinking animation
						s.playerAvatar.Engine.StartDrinking()
						s.lastCokeDrinkTime = time.Now()
						log.Printf("Starting coke drink animation")
					}
				}

			}
		}
	}

	// Handle performer microphone hold random "drk" cycles (mirror coke behavior)
	if s.nowPlayingActive && s.infoStand != nil {
		performer := strings.TrimSpace(s.infoStand.nowPlayingPlayer)
		if performer != "" && s.avatarsByName != nil {
			if av, ok := s.avatarsByName[performer]; ok && av != nil && av.Engine != nil {
				if ap, ok2 := av.Engine.Attributes["ri"]; ok2 && strings.TrimSpace(ap.Style) == "100" {
					if av.Engine.IsDrinking {
						if time.Since(s.lastMicDrinkTime) >= 500*time.Millisecond {
							av.Engine.StopDrinking()
						}
					} else {
						if time.Since(s.lastMicDrinkTime) >= 5*time.Second {
							if s.rng.Intn(100) < 2 { // ~30%/sec
								av.Engine.StartDrinking()
								s.lastMicDrinkTime = time.Now()
							}
						}
					}
				}
			}
		}
	}

	// Seat detection for all avatars: sit when standing on a seatable furniture tile
	for _, av := range s.avatars {
		s.updateSeatingForAvatar(av)
	}

	// Update isometric engine
	if s.isoScene != nil {
		s.isoScene.Update()
		// Step furniture animations each frame (mirrors Lingo FurnitureItem.stepFrame)
		if s.placement != nil {
			s.placement.tickFurnitureAnimations()
		}

		// Update hiliter state based on iso engine
		s.updateHiliterFromIsoEngine()
	}

	// Handle backpack input (selection always allowed)
	if s.backpackManager != nil {
		s.backpackManager.HandleInput()
		// Suppress room placement while trading is open
		if s.tradeManager == nil || !s.tradeManager.IsOpen() {

			s.backpackManager.HandleHoldingInput(s.placement, s.hudManager.GetWindowManager())
		}
	}

	// Trading: if open, consume held-item drops over trade slots and update trade state
	if s.tradeManager != nil && s.tradeManager.IsOpen() {
		mx, my := ebiten.CursorPosition()
		if s.tradeManager.HandleHoldingDrop(mx, my) {
			// Remove the exact held item from backpack and clear holding
			if s.backpackManager != nil {
				_, prePosID, _, _, _, _ := s.backpackManager.GetHoldingInfo()
				if strings.TrimSpace(prePosID) != "" {
					_ = s.backpackManager.RemoveItemByPosID(prePosID)
				}
				s.backpackManager.ClearHolding()
			}
		}
		// Update trade manager (checkbox/cancel/finalize)
		s.tradeManager.Update()
		// If partner has left the room, cancel the trade like a normal cancel
		partner := strings.TrimSpace(s.tradeManager.partnerName)
		if partner != "" {
			present := false
			for _, av := range s.avatars {
				if av != nil && strings.EqualFold(strings.TrimSpace(av.GetScreenName()), partner) {
					present = true
					break
				}
			}
			if !present {
				s.tradeManager.CancelIfPending()
			}
		}
	}
	// Detect if trading window was closed externally and return items if pending
	if s.tradeManager != nil && !s.tradeManager.IsOpen() {
		s.tradeManager.CancelIfPending()
	}

	// Handle Studio HUD input (mouse over/click for buttons)
	if s.hudManager != nil {
		clickedButton, err := s.hudManager.HandleInput()
		if err != nil {
			return err
		}

		// Handle specific button actions
		if clickedButton != "" {
			switch clickedButton {
			case "cc.bottombtn.navi":
				s.hudManager.OpenNavigatorWindow()
			case "cc.bottombtn.mms":
				if s.hudManager != nil {
					// Open/toggle Messenger window
					s.hudManager.OpenMessenger()
					// Populate My Info contents
					name := s.currentUserName
					mission := ""
					if s.playerAvatar != nil {
						mission = s.playerAvatar.GetMission()
					}
					// Build a static preview facing SE, like InfoStand
					var buf *ebiten.Image
					if s.playerAvatar != nil && s.playerAvatar.Engine != nil {
						if eng, err := avatarengine.NewAvatarEngine("", 100, true, s.dataDir); err == nil && eng != nil {
							for k, v := range s.playerAvatar.Engine.Attributes {
								eng.Attributes[k] = v
							}
							eng.SetDirection(2)
							eng.SetBodyAction("std")
							eng.SetFaceAction("std")
							eng.EyeAction = "std"
							eng.IsDancing = false
							eng.IsEmoting = false
							_ = eng.Draw()
							buf = eng.GetBuffer()
						}
					}
					// Placeholder counts until wired to backend
					s.hudManager.UpdateMessengerMyInfo(name, mission, 1, 0, buf)
				}
			case "cc.bottombtn.bag":
				// Toggle backpack
				if s.backpackManager != nil {
					s.backpackManager.ToggleWindow()
				}
			case "cc.bottombtn.catalogue":
				if s.hudManager != nil && s.hudManager.catalogueManager != nil {
					s.hudManager.catalogueManager.OpenCatalogue()
					// Fetch latest product prices from gateway and apply to catalogue
					if s.userCredentials.gatewayURL != "" {
						go func(url string) {
							httpClient := &http.Client{Timeout: 15 * time.Second}
							prices, err := gateway.GetProductList(httpClient, url)
							if err != nil {
								log.Printf("Studio: failed to fetch product list: %v", err)
								return
							}
							// Apply prices to UI
							s.hudManager.catalogueManager.ApplyPriceList(prices)
						}(s.userCredentials.gatewayURL)
					}
				}
			case "cc.bottombtn.sound":
				// Toggle sound mute state and swap button art
				if s.hudManager != nil {
					muted := s.hudManager.ToggleSound()
					// Apply to stereo master volume
					if s.stereo != nil {
						s.stereo.SetMuted(muted)
					}
				}
			case "cc.bottombtn.dbs":
				// Toggle Decibels window; if opening, fetch fresh balance from gateway
				if s.hudManager == nil {
					break
				}
				wm := s.hudManager.GetWindowManager()
				if wm != nil && wm.GetWindow("cc_balance") != nil {
					wm.CloseWindow("cc_balance")
					break
				}
				// Open immediately with current balance, then refresh after fetch
				s.hudManager.OpenDecibelsWindow()
				if s.userCredentials.screenName != "" && s.userCredentials.secret != "" && s.userCredentials.gatewayURL != "" {
					go func(sn, sec, url string) {
						httpClient := &http.Client{Timeout: 15 * time.Second}
						bal, err := gateway.GetDenizenBalance(httpClient, url, sn, sec)
						if err != nil {
							log.Printf("Studio: failed to fetch denizen balance: %v", err)
							return
						}
						s.hudManager.SetBalance(bal)
						s.hudManager.RefreshDecibelsWindow()
					}(s.userCredentials.screenName, s.userCredentials.secret, s.userCredentials.gatewayURL)
				} else {
					log.Printf("Studio: cannot fetch denizen balance - credentials not available")
				}
			default:
				// Other buttons clicked
			}
		}

		// Handle chat input
		if s.chatManager != nil {
			mouseX, mouseY := ebiten.CursorPosition()
			mouseClicked := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
			mouseReleased := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)

			// Get HUD offsets for chat positioning
			offsetX, offsetY := s.hudManager.GetHUDOffsets()

			s.chatManager.HandleInput(mouseX, mouseY, mouseClicked, offsetX, offsetY, s.hudManager.GetWindowManager().IsMouseOverAnyWindow())

			// Check if message should be sent
			if s.chatManager.ShouldSendMessage() {
				message := strings.TrimSpace(s.chatManager.GetInput())
				if message != "" && s.playerAvatar != nil {
					// Get current speech mode (1=speak, 2=shout)
					speechMode := 1
					if s.chatManager.GetSpeechMode() == SpeechModeShout {
						speechMode = 2
					}

					// Send the chat message over network if sender is available
					if s.sendChatIfAvailable != nil {
						if err := s.sendChatIfAvailable(s.studio.StudioID, message, speechMode); err != nil {
							log.Printf("Studio: failed to send chat: %v", err)
						} else {
							log.Printf("Studio: sent chat message: %q (mode=%d)", message, speechMode)
						}
					}

					// Don't emit local bubble - we'll receive it back from the server
				}
				s.chatManager.ClearInput()
			}

			// Handle speech switch
			s.chatManager.HandleSpeechSwitch(mouseX, mouseY, mouseClicked, mouseReleased, offsetX, offsetY)
		}
	}
	return nil
}

// ApplyAvatarAction applies an action to a named avatar. "dnc" starts dance, "spk" starts speaking, empty stops.
// If a direction is provided (0..7), face the avatar that way.
func (s *Scene) ApplyAvatarAction(name, act string, dir int) {
	if s == nil || name == "" {
		return
	}
	av, ok := s.avatarsByName[name]
	if !ok || av == nil || av.Engine == nil {
		return
	}
	// Set direction if within 0..7; use raw server direction
	if dir >= 0 && dir <= 7 {
		av.Engine.SetDirection(dir)
	}
	// Handle different actions
	switch act {
	case "dnc":
		// Toggle dance state. When dancing and not walking, keep body action "std" for dance overrides
		av.Engine.SetDance(true)
		if av.Engine.GetBodyAction() == "wlk" {
			// let movement control walking animation; dance will be visible once standing
		} else {
			av.Engine.SetBodyAction("std")
		}
	case "spk":
		// Start speaking animation with 2 second duration (2000ms) and 30% slower speed
		av.Engine.StartChattingWithDuration(2000*time.Millisecond, 1.3)
	case "drk":
		// Remote drink action: trigger a 500ms drink animation
		// Ensure avatar has a coke item visible in hand
		if av.Engine != nil {
			// If not carrying yet, set holding item for visuals without starting drink timer conflicts
			if !av.Engine.IsCarrying {
				av.Engine.SetHoldingItem("coke_hold")
			}
			// Start a short drink action
			av.Engine.IsDrinking = true
			av.Engine.DrinkTimer = time.Now()
			av.Engine.DrinkTimerLength = 500 * time.Millisecond
		}
	case "mcp":
		// Microphone carry: replace any existing carried item (e.g., coke) with microphone
		if av.Engine != nil {
			av.Engine.StartCarrying()
			_ = av.Engine.SetBodyPartStyle("ri", "100") // microphone (crr_ri_100)
			// If transitioning from an active coke sip, stop it to avoid a truncated first mic sip
			if av.Engine.IsDrinking {
				av.Engine.StopDrinking()
			}
			// If this is the local player, disable the coke-hold cycle so it doesn't interfere
			if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(s.currentUserName)) && s.playerHoldingCoke {
				s.playerHoldingCoke = false
				s.lastCokeDrinkTime = time.Time{}
				s.cokeHoldUntil = time.Time{}
			}
			// Do NOT force-drink; we want the same random drk cycle as coke. Start fresh wait window.
			s.lastMicDrinkTime = time.Now()
		}
	case "":
		// Stop all actions
		av.Engine.SetDance(false)
		if av.Engine.GetBodyAction() != "wlk" {
			av.Engine.SetBodyAction("std")
		}
	}
}

// updateHiliterFromIsoEngine updates hiliter state using the iso engine
func (s *Scene) updateHiliterFromIsoEngine() {
	// Check if mouse is over a walkable tile using iso engine
	s.showHiliter = s.isoScene.IsMouseOverWalkableTile()

	// Debug: Print mouse and grid coordinates when square changes
	mouseSquare := s.isoScene.GetMouseSquare()

	s.lastMouseSquare = mouseSquare

	if s.showHiliter {
		// Get the current mouse square from iso engine
		if mouseSquare != nil {
			// Position hiliter at the bottom-left corner of the grid square
			bottomLeft := mouseSquare.GetViewPoints().BottomLeft
			s.hiliterX = bottomLeft.X
			s.hiliterY = bottomLeft.Y
		}
	}
}

// Draw implements the Scene interface
func (s *Scene) Draw(screen *ebiten.Image) {
	// Clear screen with pure black background
	screen.Fill(color.RGBA{0x00, 0x00, 0x00, 0xFF}) // Pure black

	// Draw studio loading overlay if active (blocks scene render)
	if s.showStudioLoader {
		// Centered animation and text positions identical to boot
		sW, sH := screen.Bounds().Dx(), screen.Bounds().Dy()
		centerX, centerY := float64(sW/2), float64(sH/2)
		if s.loaderFP != nil {
			// animation ~317x127
			animX := centerX - 158.5
			animY := centerY - 63.5
			s.loaderFP.Draw(screen, animX, animY)
		}
		studioName := "Studio"
		if s.studio != nil && strings.TrimSpace(s.studio.Name) != "" {
			studioName = s.studio.Name
		}
		status := fmt.Sprintf("Loading %s %d of %d", studioName, s.loaderStep, s.loaderSteps)
		// Use the same text drawer style as boot for exact placement and crispness
		s.drawText(screen, status, centerX, centerY-90, color.RGBA{255, 255, 255, 255}, true, 10)
		if strings.TrimSpace(s.loaderTip) != "" {
			// Tip in bold Verdana, below animation like boot
			s.drawText(screen, s.loaderTip, centerX, centerY+90, color.RGBA{255, 255, 255, 255}, true, 10)

		}
		return
	}

	// Determine if any avatar is on the door tile (controls door layering)
	doorOccupied := s.anyAvatarOnDoorTile()


		// Draw room background image (behind everything)
		if s.backgroundImg != nil {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(s.bgPos[0]), float64(s.bgPos[1]))
			screen.DrawImage(s.backgroundImg, op)
		}

	// 1) Draw low-Z room elements first (Z < 1000)
	s.drawRoomElementsBelow(screen, 1000)

	if doorOccupied {


		// 3a) Avatar is in doorway: draw door avatars first
		s.drawAvatarsOnDoorTiles(screen)
		// 4a) Draw overlay, then non-door walls, then door mask tiles
		if s.isoScene != nil && s.isoScene.GetWallSystem() != nil {
			ws := s.isoScene.GetWallSystem()
			if s.studio != nil && s.studio.Layout == 5 {
				// Public room: hide private studio door overlays and door masks
				ws.DrawNonDoorTiles(screen)
			} else {
				ws.DrawDoorOverlayTiles(screen)
				ws.DrawNonDoorTiles(screen)
				ws.DrawDoorTiles(screen)
			}
		}
	} else {
		// 3b) No avatar in doorway: still draw the door overlay, then non-door walls, then door masks
		if s.isoScene != nil && s.isoScene.GetWallSystem() != nil {
			ws := s.isoScene.GetWallSystem()
			if s.studio != nil && s.studio.Layout == 5 {
				// Public room: hide private studio door overlays and door masks
				ws.DrawNonDoorTiles(screen)
			} else {
				// Keep the 'd' doormask overlay always on
				ws.DrawDoorOverlayTiles(screen)
				ws.DrawNonDoorTiles(screen)
				ws.DrawDoorTiles(screen)
			}
		}
	}

	// 6) Draw high-Z room elements (e.g., windows)
	s.drawRoomElementsAbove(screen, 1000)

		// Draw tile hiliter overlay for public rooms (above room elems, below furniture/avatars)
		s.drawPublicHiliter(screen)


			// Draw static furniture-like items from Studio XML for public rooms
			s.drawPublicStaticItems(screen)


	// Draw furniture and avatars (non-door) combined with simple row/col depth
	ghostDrawn := false
	if s.placement != nil {
		ghostDrawn = s.drawFurnitureAndAvatarsSimple(screen)
	}

	// Slightly darken scene when lighting is enabled to make glows pop
	if s.lighting != nil && s.lighting.Enabled() && s.dimOverlay != nil {
		op := &ebiten.DrawImageOptions{}
		// 20% darkening
		op.ColorM.Scale(1, 1, 1, 0.6)
		screen.DrawImage(s.dimOverlay, op)
	}

	// Lighting overlay: render glows for active items and wall neons before HUD/UI
	if s.lighting != nil && s.lighting.Enabled() {
		lights := s.lighting.BuildLights(s)
		// Debug: quick counter to verify lights are being built
		if !s.lightsDebugPrinted {
			fmt.Printf("[lighting] built %d lights\n", len(lights))
			s.lightsDebugPrinted = true
		}
		s.lighting.Render(screen, lights)
	}

	// Draw grid debug overlay only (avoid redrawing walls on top of furniture)
	if s.isoScene != nil && s.isoScene.GetGrid() != nil {
		if s.isoScene.GetGrid().ShowDebugGrid {
			s.isoScene.GetGrid().DrawDebugGrid(screen)
		}
	}

	// Draw Studio HUD (bottom control bar) on top of room/avatars
	if s.hudManager != nil {
		s.hudManager.Draw(screen, s.chatManager)
	}

	// Draw decibels window if visible
	if s.showingDecibels && s.decibelsWindow != nil {
		s.decibelsWindow.Draw(screen)
	}

	// Draw InfoStand overlay on top of studio assets but under overlay windows
	if s.infoStand != nil {
		s.infoStand.Draw(screen)
	}

	// Draw InfoStand overlay on top of studio assets but under overlay windows
	if s.infoStand != nil {
		s.infoStand.Draw(screen)
	}

	// Draw chat bubbles on top of studio assets but under overlay windows
	if s.chatManager != nil {
		s.chatManager.DrawChatBubbles(screen)
	}

	// Draw overlay windows (navigator/backpack) on top of scene and avatars, with a hook after backpack
	if s.hudManager != nil && s.hudManager.GetWindowManager() != nil {
		wm := s.hudManager.GetWindowManager()
		wm.DrawWithHook(screen, "cc_backpack", func() {
			if s.backpackManager != nil {
				s.backpackManager.DrawItems(screen)
				s.backpackManager.DrawFrontPanel(screen)
			}
		})
		// After windows are drawn, render trade overlay labels (PosIDs) when debug is enabled
		if s.tradeManager != nil && s.tradeManager.IsOpen() {
			s.tradeManager.Draw(screen)
		}
	}

	// Draw holding preview
	if s.backpackManager != nil {
		s.backpackManager.DrawHoldingPreview(screen, ghostDrawn)
	}

	// Draw on-screen lighting toggle button (small utility control)
	// Always visible to give mobile users access; harmless on desktop
	if s.lightBtnBg != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(s.lightBtnRect[0]), float64(s.lightBtnRect[1]))
		// Semi-transparent background
		op.ColorM.Scale(1, 1, 1, 0.55)
		screen.DrawImage(s.lightBtnBg, op)
		// Label
		label := "Lights: Off"
		if s.lighting != nil && s.lighting.Enabled() {
			label = "Lights: On"
		}
		if s.hudManager != nil {
			fm := s.hudManager.GetFontManager()
			if fm != nil {
				face := fm.GetFont("Verdana", "bold", 11)
				if face != nil {
					bx := s.lightBtnRect[0]
					by := s.lightBtnRect[1]
					bw := s.lightBtnRect[2]
					bh := s.lightBtnRect[3]
					bounds := text.BoundString(face, label)
					tx := bx + (bw-bounds.Dx())/2
					ty := by + (bh+bounds.Dy())/2 - 2
					text.Draw(screen, label, face, tx, ty, color.RGBA{255, 255, 255, 255})
				}
			}
		}
	}

	// 5) Debug and UI overlays
	s.drawDebugInfo(screen)
	// Pattern console (always draw last)
	s.patternConsole.Draw(screen)

	// Draw FPS counter in top-left corner (very last so it's always visible)
	if s.fpsCounter != nil {
		s.fpsCounter.Draw(screen)
	}
}

// drawFurnitureAndAvatarsSimple builds furniture layers (and ghost if applicable) and
// non-door avatars into a single list, sorts by a simple row/column depth rule, and renders.
// Returns true if a full ghost preview was drawn.
func (s *Scene) drawFurnitureAndAvatarsSimple(screen *ebiten.Image) bool {
	if s.isoScene == nil || s.placement == nil {
		return false
	}

	// Collect furniture (and ghost) draws
	draws, ghostDrawn := s.placement.BuildDrawsSimple()

	// Helper for simple depth: higher row -> on top; if same row, lower col -> on top
	// Avatars use +1 per column (no multiplication) for finest positioning
	simpleDepth := func(row, col, weight int) int {
		return row*10000 - col*5
	}

	// Public room (London): add Studio XML Furniture (london_chair/table/cokecase) to the unified draw list
	if s.studio != nil && s.studio.Layout == 5 && s.studioXML != nil {
		mm, _ := lingoassets.LoadMemberMapCached(filepath.Join("Lingo", "london"))
		if mm != nil {
			mirrorEven := func(d int) int {
				switch d {
				case 2:
					return 4
				case 4:
					return 2
				case 0:
					return 6
				case 6:
					return 0
				default:
					return d
				}
			}
			grid := s.isoScene.GetGrid()
			for _, f := range s.studioXML.Furniture.Items {
				base := strings.TrimSpace(f.ImageBase)
				if base == "" || f.GridX <= 0 || f.GridZ <= 0 {
					continue
				}
				// London public room furniture uses server-side grid (x=GridX, z=GridZ).
				// Convert to client grid using the same mapping as movement:
				// clientRow = serverX
				// clientCol = (grid.Cols + 1) - serverZ
				clientRow := f.GridX
				clientCol := (grid.Cols + 1) - f.GridZ
				sq := grid.GetSquareByRowCol(clientRow, clientCol)
				if sq == nil {
					continue
				}
				anchor := sq.GetViewPoints().BottomLeft
				row, col := clientRow, clientCol
				for _, o := range []string{"a", "b"} {
					// Try exact dir first, then mirrored-even fallback
					dirCandidates := []int{f.Dir, mirrorEven(f.Dir)}
					var chosenName string
					flipH := false
					for i, dval := range dirCandidates {
						cand := fmt.Sprintf("%s_%s_%d_%d_%d_%d_%d", base, o, 0, 1, 1, dval, 0)
						if p, ok := mm.PathFor(cand); ok {
							// Ensure asset is loaded (white-safe loader)
							if _, ok2 := s.assets[cand]; !ok2 {
								if im, err := lingoassets.LoadImage(p); err == nil {
									s.assets[cand] = im
								}
							}
							chosenName = cand
							flipH = (i == 1) // mirrored fallback
							break
						}
					}
					if chosenName == "" {
						continue
					}
					img := s.assets[chosenName]
					if img == nil {
						continue
					}
					// Registration point
					offX, offY := 0, 0
					if rp, has := s.memberReg.GetRegistrationPoint(chosenName); has {
						offX, offY = rp[0], rp[1]
					}
					// If flipped, convert X registration to mirrored coordinate
					if flipH {
						imgW := img.Bounds().Dx()
						offX = imgW - offX
					}
					x := float64(anchor.X) - float64(offX)
					y := float64(anchor.Y) - float64(offY)
					z := simpleDepth(row, col, 0)
					// Slightly raise later slices over earlier ones to preserve intra-item order
					if o == "b" { z += 1 }
					draws = append(draws, layerDraw{img: img, x: x, y: y, depth: z, order: o, flipH: flipH})
				}
			}
		}
	}

	// Collect non-door avatars as layers (shadow + body)
	for _, avatar := range s.avatars {
		if avatar == nil || avatar.Engine == nil || avatar.Square == nil {
			continue
		}
		if s.isAvatarOnDoorTile(avatar) {
			continue
		}
		// Advance avatar animation for this frame
		if err := avatar.Engine.Update(); err != nil {
			continue
		}
		// Ensure current buffers are drawn
		if err := avatar.Engine.DrawShadow(); err != nil {
			continue
		}
		if err := avatar.Engine.Draw(); err != nil {
			continue
		}
		avatarBuffer := avatar.Engine.GetBuffer()
		avatarShadowBuffer := avatar.Engine.GetShadowBuffer()
		if avatarBuffer == nil || avatarShadowBuffer == nil {
			continue
		}
		pos := avatar.GetScreenPos()
		avatarBounds := avatarBuffer.Bounds()
		x := pos.X - float64(avatarBounds.Dx())/2.1
		y := pos.Y - float64(avatarBounds.Dy())/1.2
		// Public-room-only avatar draw offset (from Studio XML if provided)
		if s.studioXML != nil {
			x += float64(s.studioXML.AvatarXOff)
			y += float64(s.studioXML.AvatarYOff)
		}
		yOffset := avatar.Engine.GetHeightOffset()

		row := avatar.Square.GetRow()
		col := avatar.Square.GetCol()

		adj := 0

		// Check if the avatar is on a seat and calculate total height offset
		isOnSeat := false
		var seatHeight int
		var topSeatFurni *PlacedFurni

		// Find the topmost seatable furniture at this position
		for _, pf := range s.placedFurni {
			if s.isSeatableFurniture(pf) {
				covered := s.placement.coverageSquares(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
				for _, sq := range covered {
					if sq != nil && sq.GetRow() == row && sq.GetCol() == col {
						if !isOnSeat || pf.GridY > topSeatFurni.GridY {
							isOnSeat = true
							topSeatFurni = &pf
						}
						break
					}
				}
			}
		}

		// Calculate total height of stacked items below the seat
		if isOnSeat && topSeatFurni != nil {
			seatHeight = s.placement.supportStackHeightPx(*topSeatFurni)
			// Preserve floating offset if the seat is floating
			if topSeatFurni.Floating && topSeatFurni.FloatingOffsetPx > seatHeight {
				seatHeight = topSeatFurni.FloatingOffsetPx
			}
		}

		// Apply seat height offset for stacked furniture
		if isOnSeat && seatHeight > 0 {
			y -= float64(seatHeight)
		}

		// Depth offset for avatar when sitting. We rely on IAOT boost only to avoid occlusion issues.
		heightDepthOffset := 0

		// Calculate avatar depth - use furniture depth if sitting, otherwise normal depth
		var avatarBodyDepth, avatarShadowDepth int
		if isOnSeat && topSeatFurni != nil && s.placement != nil {
			// Sitting: apply cumulative IAOT depth boost (100 * IAOT height below)
			iaotHeight := s.cumulativeIAOTHeightBelow(*topSeatFurni)
			if topSeatFurni.Floating && topSeatFurni.FloatingIAOTHeight > iaotHeight {
				iaotHeight = topSeatFurni.FloatingIAOTHeight
			}
			if topSeatFurni.Floating && topSeatFurni.FloatingOffsetPx > iaotHeight {
				iaotHeight = topSeatFurni.FloatingOffsetPx
			}
			iaotBoost := iaotHeight * 100
			avatarBodyDepth = simpleDepth(row, col, 2) + adj + s.avatarDepthBoost + iaotBoost
			avatarShadowDepth = simpleDepth(row, col, 6) + adj - 11 + s.avatarDepthBoost + iaotBoost
		} else {
			// Standing: apply same global avatar boost
			avatarBodyDepth = simpleDepth(row, col, 2) + adj + heightDepthOffset + s.avatarDepthBoost
			avatarShadowDepth = simpleDepth(row, col, 6) + adj + heightDepthOffset - 11 + s.avatarDepthBoost
		}

		// Shadow below body at same row/col
		draws = append(draws, layerDraw{img: avatarShadowBuffer, x: x, y: y, depth: avatarShadowDepth, order: "avatar_shadow"})
		draws = append(draws, layerDraw{img: avatarBuffer, x: x, y: y - float64(yOffset), depth: avatarBodyDepth, order: "avatar_body"})
	}

	// Sort so smaller depth draws first (further back), larger depth draws later (on top)
	sort.SliceStable(draws, func(i, j int) bool { return draws[i].depth < draws[j].depth })
	for _, d := range draws {
		if d.order == "sd" || d.order == "avatar_shadow" {
			// Shadow layers use 40% opacity as per Lingo blend setting
			op := &colorm.DrawImageOptions{}
			if d.flipH {
				w := d.img.Bounds().Dx()
				op.GeoM.Scale(-1, 1)
				op.GeoM.Translate(d.x+float64(w), d.y)
			} else {
				// Public rooms: scale avatars to 50%
				if s.studio != nil && s.studio.Owner == "$SYSTEM$" && d.order == "avatar_shadow" {
					scale := 0.5
					bw := float64(d.img.Bounds().Dx())
					bh := float64(d.img.Bounds().Dy())
					ax := (bw * (1.0 - scale)) / 2.0
					ay := (bh * (1.0 - scale)) / 2.0
					op.GeoM.Scale(scale, scale)
					op.GeoM.Translate(d.x+ax, d.y+ay)
				} else {
					op.GeoM.Translate(d.x, d.y)
				}
			}
			var cm colorm.ColorM
			cm.Scale(1, 1, 1, 0.4) // 40% opacity for shadows
			colorm.DrawImage(screen, d.img, cm, op)
		} else {
			// Regular layers use full opacity
			op := &ebiten.DrawImageOptions{}
			if d.flipH {
				w := d.img.Bounds().Dx()
				op.GeoM.Scale(-1, 1)
				op.GeoM.Translate(d.x+float64(w), d.y)
			} else {
				// Public rooms: scale avatars to 50%
				if s.studio != nil && s.studio.Owner == "$SYSTEM$" && d.order == "avatar_body" {
					scale := 0.5
					bw := float64(d.img.Bounds().Dx())
					bh := float64(d.img.Bounds().Dy())
					ax := (bw * (1.0 - scale)) / 2.0
					ay := (bh * (1.0 - scale)) / 2.0
					op.GeoM.Scale(scale, scale)
					op.GeoM.Translate(d.x+ax, d.y+ay)
				} else {
					op.GeoM.Translate(d.x, d.y)
				}
			}
			screen.DrawImage(d.img, op)

			// Debug labels when F2 debug is enabled
			if s.backpackManager != nil && s.backpackManager.showPosLabels && d.order != "avatar_body" && d.order != "avatar_shadow" {
				// We need access to the associated PlacedFurni for furniture layers; in this simple draw, only furniture entries from placement.BuildDrawsSimple include pf
				if d.pf != nil {
					fm := s.hudManager.GetFontManager()
					if fm != nil {
						face := fm.GetFont("Verdana", "bold", 11)
						if face != nil {
							label := strings.TrimSpace(d.pf.PosID)
							if label != "" {
								bounds := d.img.Bounds()
								tx := d.x + float64(bounds.Dx())/2 - float64(text.BoundString(face, label).Dx())/2
								ty := d.y - 4
								text.Draw(screen, label, face, int(tx), int(ty), color.RGBA{0, 255, 0, 255})
								// Also draw current depth value in yellow just above the PosID
								depthStr := strconv.Itoa(d.depth)
								db := text.BoundString(face, depthStr)
								dtx := d.x + float64(bounds.Dx())/2 - float64(db.Dx())/2
								dty := d.y - 16
								text.Draw(screen, depthStr, face, int(dtx), int(dty), color.RGBA{255, 255, 0, 255})
							}
						}
					}
				}
			}

			// Avatar depth debug: draw depth for avatar body layer in yellow
			if s.backpackManager != nil && s.backpackManager.showPosLabels && d.order == "avatar_body" {
				fm := s.hudManager.GetFontManager()
				if fm != nil {
					face := fm.GetFont("Verdana", "bold", 11)
					if face != nil {
						depthStr := strconv.Itoa(d.depth)
						bounds := d.img.Bounds()
						tx := d.x + float64(bounds.Dx())/2 - float64(text.BoundString(face, depthStr).Dx())/2
						ty := d.y - 4
						text.Draw(screen, depthStr, face, int(tx), int(ty), color.RGBA{255, 255, 0, 255})
					}
				}
			}
		}
	}

	return ghostDrawn
}

// CollectDrawDepths provides current furniture and avatar depth values for console reporting
func (s *Scene) CollectDrawDepths() []isoengine.DrawDepthEntry {
	var entries []isoengine.DrawDepthEntry
	if s.isoScene == nil {
		return entries
	}
	// Furniture depths (approximate to simple draw rule)
	if s.placement != nil {
		// Removed per-layer ordering; report uses only stacking and zshift
		simpleDepth := func(row, col, weight int) int {
			return row*10000 - col*5
		}
		mirrorEven := func(d int) int {
			switch d {
			case 2:
				return 4
			case 4:
				return 2
			case 0:
				return 6
			case 6:
				return 0
			default:
				return d
			}
		}
		for _, pf := range s.placedFurni {
			// Shadow layer depth
			depthRow := pf.Row
			depthCol := pf.Col
			zsd := simpleDepth(depthRow, depthCol, 0)
			// Rule boosts for shadow as well
			if s.catalog != nil {
				if ci := s.catalog.GetItemByID(pf.ProdID); ci != nil {

					if strings.EqualFold(ci.Type, "separator") && ci.ItemsAllowedOnTop && (pf.Dir == 2 || pf.Dir == 6) {
						zsd += 10000
					}
				}
			}

			// Apply z-shift from .zshift.txt file for shadow layer
			if s.placement != nil {
				zshift := s.placement.parseZShift(pf.Base, "sd")
				zsd += zshift
			}

			// Skip shadow layer entry for rugs - they don't have shadow layers
			if !s.isRugFurniture(pf) {
				entries = append(entries, isoengine.DrawDepthEntry{Kind: "furni", Name: pf.Base, Layer: "sd", Depth: zsd})
			}

			aw := max1(pf.Width)
			ad := max1(pf.Depth)
			if aw < ad {
				aw, ad = ad, aw
			}
			w := strconv.Itoa(aw)
			d := strconv.Itoa(ad)
			for _, o := range []string{"a", "b", "c", "d"} {
				// Only include if an asset exists for this direction or its mirror
				dirS := strconv.Itoa(pf.Dir)
				name := pf.Base + "_" + o + "_0_" + w + "_" + d + "_" + dirS + "_0"
				has := false
				if s.placement.mm != nil {
					if _, ok := s.placement.mm.PathFor(name); ok {
						has = true
					}
					if !has {
						alt := pf.Base + "_" + o + "_0_" + w + "_" + d + "_" + strconv.Itoa(mirrorEven(pf.Dir)) + "_0"
						if _, ok := s.placement.mm.PathFor(alt); ok {
							has = true
						}
					}
				} else {
					has = true // if no member map, include
				}
				if !has {
					continue
				}

				// Compute per-layer depth using the same rules as BuildDrawsSimple()
				// Base row/col from furniture origin with special 2x1 horizontal correction
				depthRow := pf.Row
				depthCol := pf.Col
				if pf.Width == 2 && pf.Depth == 1 && (pf.Dir == 2 || pf.Dir == 6) {
					depthRow += 1
				}
				if pf.Width == 3 && pf.Depth == 3 {
					depthRow += 1
					depthCol += 1
				}
				// Apply stacking influence identical to BuildDrawsSimple (keep scaling consistent)
				weight := 0
				// weight -= pf.GridY * 1 / 2
				z := simpleDepth(depthRow, depthCol, weight)

				// Apply per-PosID slice boosts only
				if m, ok := s.depthBoostsBySlicePerPos[strings.TrimSpace(pf.PosID)]; ok && m != nil {
					if sdelta, ok2 := m[strings.ToLower(o)]; ok2 {
						z += sdelta
					}
				}

				// Apply z-shift from .zshift.txt file for this specific layer
				if s.placement != nil {
					zshift := s.placement.parseZShift(pf.Base, o)
					z += zshift
				}

				// Rule boosts (constant depth adjustments independent of row/col)
				if s.catalog != nil {
					if ci := s.catalog.GetItemByID(pf.ProdID); ci != nil {
						if strings.EqualFold(ci.Type, "separator") && ci.ItemsAllowedOnTop && (pf.Dir == 2 || pf.Dir == 6) {
							z += 10000
						}
					}
				}

				entries = append(entries, isoengine.DrawDepthEntry{Kind: "furni", Name: pf.Base + "_" + o, Layer: o, Depth: z})
			}
		}
	}

	// Avatar depths (mirror drawFurnitureAndAvatarsSimple logic)
	for _, avatar := range s.avatars {
		if avatar == nil || avatar.Engine == nil || avatar.Square == nil {
			continue
		}
		if s.isAvatarOnDoorTile(avatar) {
			continue
		}
		simpleDepth := func(row, col, weight int) int { return row*10000 - col }
		row := avatar.Square.GetRow()
		col := avatar.Square.GetCol()

		// Seat detection and stacked-seat height handling
		adj := 0
		isOnSeat := false
		var seatHeight int
		var topSeatFurni *PlacedFurni
		for _, pf := range s.placedFurni {
			if s.isSeatableFurniture(pf) {
				covered := s.placement.coverageSquares(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
				for _, sq := range covered {
					if sq != nil && sq.GetRow() == row && sq.GetCol() == col {
						if !isOnSeat || (topSeatFurni != nil && pf.GridY > topSeatFurni.GridY) {
							isOnSeat = true
							topSeatFurni = &pf
						} else if !isOnSeat {
							isOnSeat = true
							topSeatFurni = &pf
						}
						break
					}
				}
			}
		}
		if isOnSeat && topSeatFurni != nil {
			seatHeight = s.placement.supportStackHeightPx(*topSeatFurni)
		}
		heightDepthOffset := 0
		if isOnSeat && seatHeight > 0 {
			heightDepthOffset = -seatHeight * s.heightDepthAdjust
		}

		var zBody, zShadow int
		if isOnSeat && topSeatFurni != nil {
			furnitureDepth := s.placement.getFurnitureDepth(*topSeatFurni)
			iaotHeight := s.cumulativeIAOTHeightBelow(*topSeatFurni)
			if topSeatFurni.Floating && topSeatFurni.FloatingIAOTHeight > iaotHeight {
				iaotHeight = topSeatFurni.FloatingIAOTHeight
			}
			if topSeatFurni.Floating && topSeatFurni.FloatingOffsetPx > iaotHeight {
				iaotHeight = topSeatFurni.FloatingOffsetPx
			}
			iaotBoost := iaotHeight * 100
			zBody = furnitureDepth + s.avatarDepthBoost + iaotBoost
			zShadow = zBody - 9
		} else {
			zBody = simpleDepth(row, col, 2) + adj + heightDepthOffset + s.avatarDepthBoost
			zShadow = simpleDepth(row, col, 6) + adj + heightDepthOffset - 11 + s.avatarDepthBoost
		}

		entries = append(entries, isoengine.DrawDepthEntry{Kind: "avatar", Name: avatar.GetScreenName(), Layer: "shadow", Depth: zShadow})
		entries = append(entries, isoengine.DrawDepthEntry{Kind: "avatar", Name: avatar.GetScreenName(), Layer: "body", Depth: zBody})
	}

	// Sort entries by depth to reflect exact on-screen draw order
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Depth < entries[j].Depth })

	return entries
}

// Removed isOccludedByLeftFurniture and isOccludedByRightFurniture functions

// isSeatableFurniture reports whether a placed furniture is a chair-type (Chair/Sofa/Stool/carfront)
func (s *Scene) isSeatableFurniture(pf PlacedFurni) bool {
	if s.catalog != nil {
		if ci := s.catalog.GetItemByID(pf.ProdID); ci != nil {
			t := ci.Type
			if t == "Chair" || t == "Sofa" || t == "Stool" || t == "carfront" {
				return true
			}
		}
	}
	// Fallback by base name heuristic
	b := strings.ToLower(pf.Base)
	return strings.Contains(b, "sofa") || strings.Contains(b, "couch") || strings.Contains(b, "chair") || strings.Contains(b, "stool")
}

// isRugFurniture reports whether a placed furniture is a rug-type (walkable and IAOT)
func (s *Scene) isRugFurniture(pf PlacedFurni) bool {
	if s.catalog != nil {
		if ci := s.catalog.GetItemByID(pf.ProdID); ci != nil {
			if strings.EqualFold(ci.Type, "rug") || strings.EqualFold(ci.Type, "hardwood") {
				return true
			}
		}
	}
	// Fallback by base name heuristic
	b := strings.ToLower(pf.Base)
	return strings.Contains(b, "rug") || strings.Contains(b, "hardwood")
}

// allowsItemsOnTop reports whether a placed furniture item allows other items to be stacked on top of it (IAOT)
func (s *Scene) allowsItemsOnTop(pf PlacedFurni) bool {
	if s.catalog == nil {
		return false
	}
	prodID := strings.TrimSpace(pf.ProdID)
	if prodID == "" {
		return false
	}
	// If server provided IAOT on this item, honor it
	if pf.IAOTProvided {
		return pf.IAOT
	}
	ci := s.catalog.GetItemByID(prodID)
	if ci == nil {
		return false
	}
	// Rugs always allow items on top, regardless of catalog setting
	if s.isRugFurniture(pf) {
		return true
	}
	return ci.ItemsAllowedOnTop
}

// updateSeatingState checks if the player avatar is on a seatable furniture and applies sit/stand.
func (s *Scene) updateSeatingState() {
	if s.playerAvatar == nil || s.playerAvatar.Square == nil {
		return
	}
	// If moving, force non-sit state and return early
	if s.playerAvatar.IsMoving() {
		if s.playerAvatar.Engine.GetBodyAction() != "wlk" {
			s.playerAvatar.Engine.StartWalking()
		}
		return
	}
	row := s.playerAvatar.Square.GetRow()
	col := s.playerAvatar.Square.GetCol()

	// Determine if any placed furniture covers this square and is seatable
	// Find the topmost seatable furniture at this position and capture its direction
	seatableHere := false
	seatDir := -1
	var topSeatLevel int = -1
	for _, pf := range s.placedFurni {
		// Compute coverage based on dir (use same logic as placement)
		covered := s.placement.coverageSquares(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
		for _, sq := range covered {
			if sq != nil && sq.GetRow() == row && sq.GetCol() == col {
				// Lookup catalog type to decide seatability
				if s.catalog != nil {
					if ci := s.catalog.GetItemByID(pf.ProdID); ci != nil {
						if ci.Type == "Chair" || ci.Type == "Sofa" || ci.Type == "Stool" || ci.Type == "carfront" {
							// Only use the topmost seatable furniture
							if pf.GridY > topSeatLevel {
								seatableHere = true
								topSeatLevel = pf.GridY
								seatDir = pf.Dir
							}
							break
						}
					}
				}
			}
		}
	}

	if seatableHere {
		// Apply sit state and align direction with seat direction
		if s.playerAvatar.Engine.GetBodyAction() != "sit" {
			s.playerAvatar.Engine.SetBodyAction("sit")
			if seatDir >= 0 {
				s.playerAvatar.Engine.SetDirection(seatDir)
			}
		} else {
			// Already sitting: keep direction synced with the seat's current direction
			if seatDir >= 0 && s.playerAvatar.Engine.GetDirection() != seatDir {
				s.playerAvatar.Engine.SetDirection(seatDir)
			}
		}
	} else {
		// Return to std when not on a seat
		if s.playerAvatar.Engine.GetBodyAction() == "sit" {
			s.playerAvatar.Engine.SetBodyAction("std")
		}
	}
}

// updateSeatingForAvatar applies sit/stand and seat-aligned direction for any avatar.
func (s *Scene) updateSeatingForAvatar(avatar *isoengine.IsoAvatar) {
	if avatar == nil || avatar.Square == nil || avatar.Engine == nil {
		return
	}
	// If moving, do not sit
	if avatar.IsMoving() {
		if avatar.Engine.GetBodyAction() != "wlk" {
			avatar.Engine.StartWalking()
		}
		return
	}
	row := avatar.Square.GetRow()
	col := avatar.Square.GetCol()

	seatableHere := false
	seatDir := -1
	var topSeatLevel int = -1
	for _, pf := range s.placedFurni {
		covered := s.placement.coverageSquares(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
		for _, sq := range covered {
			if sq != nil && sq.GetRow() == row && sq.GetCol() == col {
				if s.catalog != nil {
					if ci := s.catalog.GetItemByID(pf.ProdID); ci != nil {
						if ci.Type == "Chair" || ci.Type == "Sofa" || ci.Type == "Stool" || ci.Type == "carfront" {
							if pf.GridY > topSeatLevel {
								seatableHere = true
								topSeatLevel = pf.GridY
								seatDir = pf.Dir
							}
							break
						}
					}
				}
			}
		}
	}

	if seatableHere {
		if avatar.Engine.GetBodyAction() != "sit" {
			avatar.Engine.SetBodyAction("sit")
			if seatDir >= 0 {
				avatar.Engine.SetDirection(seatDir)
			}
		} else {
			if seatDir >= 0 && avatar.Engine.GetDirection() != seatDir {
				avatar.Engine.SetDirection(seatDir)
			}
		}
	} else {
		if avatar.Engine.GetBodyAction() == "sit" {
			avatar.Engine.SetBodyAction("std")
		}
	}
}

// drawRoomElements draws all room elements in proper Z-order (kept for compatibility)
func (s *Scene) drawRoomElements(screen *ebiten.Image) {
	s.drawRoomElementsBelow(screen, 999999) // Draw all elements
}

// drawRoomElementsBelow draws room elements with Z < threshold
func (s *Scene) drawRoomElementsBelow(screen *ebiten.Image, threshold int) {
	s.drawRoomElementsFiltered(screen, threshold, true)
}

// drawRoomElementsAbove draws room elements with Z >= threshold
func (s *Scene) drawRoomElementsAbove(screen *ebiten.Image, threshold int) {
	s.drawRoomElementsFiltered(screen, threshold, false)
}

// drawRoomElementsFiltered draws room elements based on Z-threshold
func (s *Scene) drawRoomElementsFiltered(screen *ebiten.Image, threshold int, below bool) {
	// Sort elements only once (cached for performance)
	if !s.elementsSortedOnce {
		s.sortedElements = make([]studioroom.RoomElement, len(s.roomDef.Elements))
		copy(s.sortedElements, s.roomDef.Elements)

		// Simple bubble sort by LocZ (lowest Z values first)
		for i := 0; i < len(s.sortedElements)-1; i++ {
			for j := 0; j < len(s.sortedElements)-i-1; j++ {
				if s.sortedElements[j].LocZ > s.sortedElements[j+1].LocZ {
					s.sortedElements[j], s.sortedElements[j+1] = s.sortedElements[j+1], s.sortedElements[j]
				}
			}
		}
		s.elementsSortedOnce = true

		s.debugPrinted = true
	}

	// Draw filtered elements using cached sorted list
	for _, element := range s.sortedElements {
		// Skip floor shape elements - they're only used by pattern system
		if element.ID == "floor_shape_b" {
			continue
		}

		// Filter by Z-threshold
		if below && element.LocZ >= threshold {
			continue // Skip high-Z elements when drawing below
		}
		if !below && element.LocZ < threshold {
			continue // Skip low-Z elements when drawing above
		}

		s.drawElement(screen, element)
	}
}

// drawPublicHiliter draws the tile hiliter overlay for public rooms (London)
func (s *Scene) drawPublicHiliter(screen *ebiten.Image) {
	if s.studio == nil || s.studio.Layout != 5 {
		return
	}
	if !s.showHiliter || s.isoScene == nil {
		return
	}

	// Resolve member name: prefer Studio XML <Hiliter member="...">
	memberName := ""
	if s.studioXML != nil && s.studioXML.Hiliter.Member != "" {
		memberName = s.studioXML.Hiliter.Member
	}
	// Fallback: search room elements for type=="hiliter"
	if memberName == "" {
		for _, el := range s.roomDef.Elements {
			if strings.ToLower(el.Type) == "hiliter" && el.Member != "" {
				memberName = el.Member
				break
			}
		}
	}
	if memberName == "" {
		return
	}

	asset, ok := s.assets[memberName]
	if !ok || asset == nil {
		return
	}
	// Position from iso grid (bottom-left of tile), then apply registration point
	x := s.hiliterX
	y := s.hiliterY
	if rp, has := s.memberReg.GetRegistrationPoint(memberName); has {
		x -= float64(rp[0])
		y -= float64(rp[1])
	}
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(x, y)
	// Additive blend to emulate Lingo ink 36 glow
	opts.CompositeMode = ebiten.CompositeModeLighter
	screen.DrawImage(asset, opts)
}


// drawPublicStaticItems draws StaticItems from Studio XML that are not in the .room file
// These include bartable/djtable pieces in London
func (s *Scene) drawPublicStaticItems(screen *ebiten.Image) {
	if s.studio == nil || s.studio.Layout != 5 || s.studioXML == nil {
		return
	}
	// Build a set of room element members to avoid duplicates
	roomMembers := make(map[string]bool)
	for _, el := range s.roomDef.Elements {
		if el.Member != "" {
			roomMembers[el.Member] = true
		}
	}
	for _, it := range s.studioXML.StaticItems.Items {
		if it.Member == "" || roomMembers[it.Member] {
			continue
		}
		asset, ok := s.assets[it.Member]
		if !ok || asset == nil {
			continue
		}
		x := float64(it.X)
		y := float64(it.Y)
		if rp, has := s.memberReg.GetRegistrationPoint(it.Member); has {
			x -= float64(rp[0])
			y -= float64(rp[1])
		}
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(x, y)
		if it.Ink == 36 { // additive glow
			opts.CompositeMode = ebiten.CompositeModeLighter
		}
		screen.DrawImage(asset, opts)
	}
}


// InvalidateElementSorting forces re-sorting of room elements on next draw
// Call this when room elements are added, removed, or their Z-order changes
func (s *Scene) InvalidateElementSorting() {
	s.elementsSortedOnce = false
	s.sortedElements = nil
}

// drawElement draws a single room element
func (s *Scene) drawElement(screen *ebiten.Image, element studioroom.RoomElement) {
	// Skip non-visual elements
	if element.Media != "bitmap" {
		return
	}

	// In public rooms, draw the hiliter via a dedicated overlay pass instead
	if element.Type == "hiliter" && s.studio != nil && s.studio.Layout == 5 {
		return
	}

	// Skip hiliter if it shouldn't be shown (matches Lingo Hiliter.ls hide/show logic)
	if element.Type == "hiliter" && !s.showHiliter {
		return
	}

	// Get the asset
	asset, exists := s.assets[element.Member]
	if !exists {
		return
	}

	// Calculate position with registration point offset
	x := float64(element.LocH)
	y := float64(element.LocV)

	// For hiliter, use grid-snapped positioning from iso engine
	if element.Type == "hiliter" && s.showHiliter {
		// Use the grid-snapped position from iso engine
		x = s.hiliterX
		y = s.hiliterY
	}

	// Apply registration point offset if available
	regPoint := [2]int{0, 0}
	if regPointFound, hasRegPoint := s.memberReg.GetRegistrationPoint(element.Member); hasRegPoint {
		regPoint = regPointFound
		x -= float64(regPoint[0])
		y -= float64(regPoint[1])
	}

	// Create draw options
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(x, y)

	// For hiliter, use additive blending so it "glows" like Lingo ink 36
	if element.Type == "hiliter" {
		opts.CompositeMode = ebiten.CompositeModeLighter
	}

	// Apply blend mode if needed (simplified for now)
	if element.Blend < 100 {
		alpha := float64(element.Blend) / 100.0
		opts.ColorM.Scale(1, 1, 1, alpha)
	}

	// Skip floor shape elements entirely - they should never be rendered directly
	if element.ID == "floor_shape_b" {
		return
	}

	// Check if this element has patterns assigned
	if s.patternMgr.HasPatternForElement(element.ID) {
		// Use pattern manager to render with effects
		s.patternMgr.ApplyPatternToElement(screen, element.ID, asset, x, y)
	} else {
		// Draw the asset normally
		screen.DrawImage(asset, opts)
	}
}

// setupFloorShapeForPatterns finds and passes the floor shape asset and position to pattern manager
func (s *Scene) setupFloorShapeForPatterns() {
	// Find the floor shape element in room definition
	var floorShapeElement *studioroom.RoomElement
	for i := range s.roomDef.Elements {
		if s.roomDef.Elements[i].ID == "floorshape" {
			floorShapeElement = &s.roomDef.Elements[i]
			break
		}
	}

	if floorShapeElement == nil {
		return
	}

	// Find the corresponding asset
	floorShapeAsset, exists := s.assets[floorShapeElement.Member]
	if !exists {
		return
	}

	// Calculate position with registration point offset (same as drawElement)
	x := float64(floorShapeElement.LocH)
	y := float64(floorShapeElement.LocV)

	// Apply registration point offset if available
	if regPointFound, hasRegPoint := s.memberReg.GetRegistrationPoint(floorShapeElement.Member); hasRegPoint {
		regPoint := regPointFound
		x -= float64(regPoint[0])
		y -= float64(regPoint[1])
	}

	// Pass to pattern manager
	s.patternMgr.SetFloorShapeAsset(floorShapeAsset)
	s.patternMgr.SetFloorShapePosition(x, y)
}

// applyDefaultPatterns sets default floor and wall patterns for the room
func (s *Scene) applyDefaultPatterns() {
	// Default floor: floor_orgun color 4
	for _, element := range s.roomDef.Elements {
		if strings.HasPrefix(element.ID, "floortexture") || element.ID == "doorfloortexture" {
			s.patternMgr.ApplyFloorPattern(element.ID, "floor_orgun", 4)
		}
	}

	// Default walls: wall_tile color 0
	wallIDs := []string{
		"walltile_left_1",
		"walltile_right_1",
		"walltile_left_texture",
		"walltile_right_texture",
		"walltile_door_texture", // door mask texture
	}
	for _, id := range wallIDs {
		s.patternMgr.ApplyWallPattern(id, "wall_tile", 0)
	}
}

// drawAvatars draws all avatars in the scene
func (s *Scene) drawAvatars(screen *ebiten.Image) {
	for _, avatar := range s.avatars {
		if avatar == nil || avatar.Engine == nil {
			continue
		}

		// Update avatar animation
		err := avatar.Engine.Update()
		if err != nil {
			continue
		}

		// Redraw avatar shadow and buffer
		if err := avatar.Engine.DrawShadow(); err != nil {
			continue
		}
		if err := avatar.Engine.Draw(); err != nil {
			continue
		}

		avatarBuffer := avatar.Engine.GetBuffer()
		avatarShadowBuffer := avatar.Engine.GetShadowBuffer()

		if avatarBuffer == nil || avatarShadowBuffer == nil {
			continue
		}

		// Position avatar based on its screen position, which is interpolated
		pos := avatar.GetScreenPos()
		avatarBounds := avatarBuffer.Bounds()
		x := pos.X - float64(avatarBounds.Dx())/2.1
		y := pos.Y - float64(avatarBounds.Dy())/1.2
		// Public-room-only avatar draw offset (from Studio XML if provided)
		if s.studioXML != nil {
			x += float64(s.studioXML.AvatarXOff)
			y += float64(s.studioXML.AvatarYOff)
		}
		if avatar.Engine != nil && avatar.Engine.GetBodyAction() == "sit" {
			x -= 5
			y -= 5
		}
		yOffset := avatar.Engine.GetHeightOffset()

		// Draw shadow and avatar
		shadowOpts := &ebiten.DrawImageOptions{}
		shadowOpts.GeoM.Translate(x, y)
		screen.DrawImage(avatarShadowBuffer, shadowOpts)

		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(x, y-float64(yOffset))
		screen.DrawImage(avatarBuffer, opts)
	}
}

// drawAvatarsOnDoorTiles draws only avatars standing on studiofloor_door_1 tiles
func (s *Scene) drawAvatarsOnDoorTiles(screen *ebiten.Image) {
	for _, avatar := range s.avatars {
		if avatar == nil || avatar.Engine == nil || avatar.Square == nil {
			continue
		}

		// Check if avatar is on a door tile (studiofloor_door_1)
		if !s.isAvatarOnDoorTile(avatar) {
			continue
		}

		// Ensure avatar animation advances each frame
		if err := avatar.Engine.Update(); err != nil {
			continue
		}

		s.drawSingleAvatar(screen, avatar)
	}
}

// anyAvatarOnDoorTile returns true if any avatar is currently on the door tile
func (s *Scene) anyAvatarOnDoorTile() bool {
	for _, avatar := range s.avatars {
		if avatar == nil || avatar.Square == nil {
			continue
		}
		if s.isAvatarOnDoorTile(avatar) {
			return true
		}
	}
	return false
}

// drawAvatarsNotOnDoorTiles draws avatars not standing on studiofloor_door_1 tiles
func (s *Scene) drawAvatarsNotOnDoorTiles(screen *ebiten.Image) {
	for _, avatar := range s.avatars {
		if avatar == nil || avatar.Engine == nil || avatar.Square == nil {
			continue
		}

		// Check if avatar is NOT on a door tile
		if s.isAvatarOnDoorTile(avatar) {
			continue
		}

		// Ensure avatar animation advances each frame
		if err := avatar.Engine.Update(); err != nil {
			continue
		}
		s.drawSingleAvatar(screen, avatar)
	}
}

// isAvatarOnDoorTile checks if avatar is standing on the studio door floor tile
func (s *Scene) isAvatarOnDoorTile(avatar *isoengine.IsoAvatar) bool {
	if s.isoScene == nil || s.studioXML == nil || avatar == nil || avatar.Square == nil {
		return false
	}
	grid := s.isoScene.GetGrid()
	if grid == nil {
		return false
	}
	door := s.studioXML.Door
	if door.Row == 0 && door.Col == 0 {
		return false
	}
	// Transform Studio XML coords -> grid coords (same mapping used during placement)
	mappedRow := door.Col
	mappedCol := grid.Cols - door.Row + 1
	return avatar.Square.GetRow() == mappedRow && avatar.Square.GetCol() == mappedCol
}

// drawDebugInfo draws debug information
func (s *Scene) drawDebugInfo(screen *ebiten.Image) {
	// For now, just log some basic info periodically
	// TODO: Add debug overlay with room info, element count, etc.
}

// Layout implements the Scene interface
func (s *Scene) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 960, 540 // Standard game resolution
}

// GetStudio returns the studio this scene represents
func (s *Scene) GetStudio() *studios.Studio {
	return s.studio
}

// ShouldExit returns true if the scene should exit (e.g., user clicked exit)
func (s *Scene) ShouldExit() bool {
	if s == nil || s.isoScene == nil {
		return false
	}
	// Exit if kicked
	if s.kicked {
		// Ensure any pending trade is cancelled before exiting
		if s.tradeManager != nil {
			s.tradeManager.CancelIfPending()
		}
		return true
	}
	// Do not allow exit click-through when any window is under mouse (nav, backpack, catalog, etc.)
	if s.hudManager != nil && s.hudManager.GetWindowManager() != nil && s.hudManager.GetWindowManager().IsMouseOverAnyWindow() {
		return false
	}
	// Detect click on the exit shape (studio_exitshape)
	mouseClicked := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	if !mouseClicked {
		return false
	}
	wall := s.isoScene.GetWallSystem()
	if wall == nil {
		return false
	}
	mx, my := ebiten.CursorPosition()
	tiles := wall.GetWallTiles()
	for _, t := range tiles {
		if t == nil || t.Image == nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(t.MemberName), "studio_exitshape") {
			continue
		}
		b := t.Image.Bounds()
		x0 := int(t.Position.X)
		y0 := int(t.Position.Y)
		rect := image.Rect(x0, y0, x0+b.Dx(), y0+b.Dy())
		if mx >= rect.Min.X && mx < rect.Max.X && my >= rect.Min.Y && my < rect.Max.Y {
			// Pixel alpha hit-test for precision
			px := mx - x0
			py := my - y0
			if px >= 0 && py >= 0 && px < b.Dx() && py < b.Dy() {
				_, _, _, a := t.Image.At(px, py).RGBA()
				if a>>8 > 0 {
					return true
				}
			}
		}
	}
	return false
}

// MarkKicked marks the local user as kicked so the game can transition back to entry
func (s *Scene) MarkKicked() {
	s.kicked = true
	// If trading, cancel and close as if cancelled
	if s.tradeManager != nil {
		s.tradeManager.CancelIfPending()
	}
}

// GetIsoScene returns the isometric scene for external access
func (s *Scene) GetIsoScene() *isoengine.IsoScene {
	return s.isoScene
}

// getKeysFromMap returns a slice of keys from a map[string]bool
func getKeysFromMap(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

// initTestAvatar initializes a test avatar for development
func (s *Scene) initTestAvatar(dataDir string) error {
	// Lingo-style avatar string with color information
	completeAvatarString := "hr=002/238,238,238&hd=001/255,224,196&ey=003/245,226,203&fc=002/255,224,196&bd=001/255,224,196&lh=001/255,224,196&rh=001/255,224,196&ch=002/204,153,102&ls=002/204,153,102&rs=002/204,153,102&lg=008/115,142,191&sh=009/238,238,238"

	engine, err := avatarengine.NewAvatarEngine(completeAvatarString, 100, false, dataDir)
	if err != nil {
		return fmt.Errorf("failed to create test avatar engine: %w", err)
	}

	// Place avatar on the door floor tile (studiofloor_door_1) facing west
	grid := s.isoScene.GetGrid()
	door := s.studioXML.Door
	// Transform Studio XML coords -> grid coords (same as walls/PathMap):
	// mappedRow = XML col, mappedCol = grid.Cols - XML row + 1
	mappedRow := door.Col
	mappedCol := grid.Cols - door.Row + 1

	startSquare := s.isoScene.GetSquareByRowCol(mappedRow, mappedCol)
	// Place exactly on the door tile even if it's not walkable
	// Only fallback if the square lookup failed (shouldn't happen on valid XML)
	if startSquare == nil {
		// Try immediate neighbors as a safety net
		for dr := -1; dr <= 1 && startSquare == nil; dr++ {
			for dc := -1; dc <= 1 && startSquare == nil; dc++ {
				if dr == 0 && dc == 0 {
					continue
				}
				if sq := s.isoScene.GetSquareByRowCol(mappedRow+dr, mappedCol+dc); sq != nil {
					startSquare = sq
				}
			}
		}
		// Absolute fallback to any square
		if startSquare == nil {
			for _, sq := range grid.GetWalkableSquares() {
				startSquare = sq
				break
			}
		}
	}

	if startSquare == nil {
		return fmt.Errorf("no walkable squares found for avatar placement (door at XML[%d,%d] -> Grid[%d,%d])", door.Row, door.Col, mappedRow, mappedCol)
	}

	s.playerAvatar = isoengine.NewIsoAvatar(engine, startSquare, "Adam", "hehe", false)
	s.avatars = append(s.avatars, s.playerAvatar)

	// Face West per avatarengine mapping (0=E,1=SE,2=S,3=SW,4=W,5=NW,6=N,7=NE)
	s.playerAvatar.Engine.SetDirection(4) // West facing

	// Initial draw
	err = s.playerAvatar.Engine.Draw()
	if err != nil {
		return fmt.Errorf("failed to draw test avatar: %w", err)
	}

	return nil
}

// SetStudioList allows passing the studio list so the navigator in studio can populate lists
func (s *Scene) SetStudioList(list *studios.StudioList) {
	if s.hudManager != nil {
		s.hudManager.SetStudioList(list)
	}
	// Keep reference for return-to-entry flow
	s.studioList = list
}

// SetCatalog injects the parsed catalog for IAOT and other lookups
func (s *Scene) SetCatalog(c *catalog.Catalog) {
	s.catalog = c
}

// GetStudioList returns the studio list (for returning to entry scene)
func (s *Scene) GetStudioList() *studios.StudioList {
	return s.studioList
}

// GetCatalog returns the catalog (for returning to entry scene)
func (s *Scene) GetCatalog() *catalog.Catalog {
	return s.catalog
}

// handleLightingTouchToggle processes mouse/touch input for the on-screen lighting button.
// Returns true if the input was consumed.
func (s *Scene) handleLightingTouchToggle() bool {
	// Debounce to avoid rapid toggles
	if time.Since(s.lastLightBtnPress) < 300*time.Millisecond {
		// still allow other inputs
	}

	// Mouse click
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		if pointInRect(mx, my, s.lightBtnRect) {
			if s.lighting != nil && time.Since(s.lastLightBtnPress) >= 300*time.Millisecond {
				s.lighting.SetEnabled(!s.lighting.Enabled())
				fmt.Printf("[lighting] toggled (button): %v\n", s.lighting.Enabled())
				s.lastLightBtnPress = time.Now()
			}
			return true
		}
	}

	// Touch taps
	var ids []ebiten.TouchID
	ids = inpututil.AppendJustPressedTouchIDs(ids)
	for _, id := range ids {
		x, y := ebiten.TouchPosition(id)
		if pointInRect(x, y, s.lightBtnRect) {
			if s.lighting != nil && time.Since(s.lastLightBtnPress) >= 300*time.Millisecond {
				s.lighting.SetEnabled(!s.lighting.Enabled())
				fmt.Printf("[lighting] toggled (touch): %v\n", s.lighting.Enabled())
				s.lastLightBtnPress = time.Now()
			}
			return true
		}
	}
	return false
}

// rotatePlacedFurni rotates a placed furniture item 90 degrees (4-way even directions)
func (s *Scene) rotatePlacedFurni(pf *PlacedFurni) {
	if pf == nil || s.placement == nil || s.isoScene == nil {
		return
	}
	// Disable rotation for 3x3 rugs
	if pf.Width == 3 && pf.Depth == 3 && s.isRugFurniture(*pf) {
		return
	}
	// Disable rotation for furniture that only has dir 4 assets
	if s.hasOnlyDir4Assets(pf.Base, pf.Width, pf.Depth) {
		return
	}
	// Capture old state
	oldRow, oldCol := pf.Row, pf.Col
	oldW, oldD := pf.Width, pf.Depth
	oldDir := pf.Dir

	// Capture squares covered by the old orientation (before rotation)
	oldCoveredSquares := s.placement.coverageSquares(oldRow, oldCol, oldW, oldD, oldDir)

	// Determine allowed directions from assets and advance to next allowed direction
	newDir := s.nextAllowedDirectionFor(pf)
	newW, newD := oldW, oldD
	if oldW != oldD {
		newW, newD = oldD, oldW
	}

	// Lingo behavior: keep origin (top-left) constant and attempt placement at same row/col
	if !s.placement.ValidateRotate(oldRow, oldCol, newW, newD, newDir, pf) {
		return
	}

	// Debug: log rotation intent
	fmt.Printf("[rotate] base=%s size=%dx%d dir %d -> %d\n", pf.Base, newW, newD, oldDir, newDir)
	// Apply new state; origin unchanged
	pf.Width, pf.Depth = newW, newD
	pf.Dir = newDir
	// Recompute available frames for new orientation/size
	if s.placement != nil {
		s.placement.computeNumFrames(pf)
	}

	// Apply special pixel offsets for specific rotations
	// Different offsets depending on which directions are being hflipped
	var offsetX, offsetY int

	// Check if this furniture has assets for different directions
	hasDir0Assets := s.hasAssetsForDirection(pf, 0)
	hasDir2Assets := s.hasAssetsForDirection(pf, 2)
	hasDir4Assets := s.hasAssetsForDirection(pf, 4)
	hasDir6Assets := s.hasAssetsForDirection(pf, 6)

	// If furniture has all direction assets, no offsets needed (no hflipping)
	// Generalized handling for Sofa-type furniture (2x1 / 1x2) which may have partial even-direction assets.
	// Determine sofa type using catalog first, with a base-name fallback.
	isSofaType := false
	if s.catalog != nil {
		if ci := s.catalog.GetItemByID(strings.TrimSpace(pf.ProdID)); ci != nil && strings.EqualFold(ci.Type, "Sofa") {
			isSofaType = true
		}
	}
	if !isSofaType {
		b := strings.ToLower(pf.Base)
		isSofaType = strings.Contains(b, "sofa") || strings.Contains(b, "couch")
	}
	if (pf.Width == 2 || pf.Depth == 2) && isSofaType {
		// Apply stable offsets for even-direction rotations to keep the registration anchor consistent
		// when some layers are mirrored (2<>4, 0<>6) and others exist natively.
		switch {
		case oldDir == 2 && newDir == 4:
			offsetX, offsetY = 72, 0
		case oldDir == 4 && newDir == 6:
			offsetX, offsetY = 0, 0
		case oldDir == 6 && newDir == 0:
			offsetX, offsetY = -72, 0
		case oldDir == 0 && newDir == 2:
			offsetX, offsetY = 0, 0
		}
	} else if pf.Width == 2 && pf.Depth == 2 {
		// Explicit 2x2 furniture rotation offsets for all even-direction transitions (2,4,6,0).
		// These are set to zero by default to maintain center-anchored stability; adjust if specific
		// assets exhibit registration drift on rotation.
		switch {
		case oldDir == 2 && newDir == 4:
			offsetX, offsetY = -76, 0
		case oldDir == 4 && newDir == 6:
			offsetX, offsetY = 76, 0
		case oldDir == 6 && newDir == 0:
			offsetX, offsetY = -76, 0
		case oldDir == 0 && newDir == 2:
			offsetX, offsetY = 76, 0
		}
	} else if hasDir0Assets && hasDir2Assets && hasDir4Assets && hasDir6Assets {
		// Has all directions, no offsets needed
	} else if (pf.Width == 2 || pf.Depth == 2) && hasDir2Assets && hasDir0Assets && !hasDir6Assets {
		// 2-unit furniture (2x1 or 1x2) with 2,0 assets (but not 6) e.g. coke machine
		switch {
		case oldDir == 2 && newDir == 4:
			offsetX, offsetY = 0, 0
		case oldDir == 4 && newDir == 6:
			offsetX, offsetY = 72, 0
		case oldDir == 6 && newDir == 0:
			offsetX, offsetY = -72, 0
		case oldDir == 0 && newDir == 2:
			offsetX, offsetY = 0, 0
		}
	} else if (pf.Width == 2 || pf.Depth == 2) && hasDir2Assets && hasDir6Assets && !hasDir0Assets {
		// 2-unit furniture (2x1 or 1x2) with 2,6 assets (but not 0)
		switch {
		case oldDir == 2 && newDir == 4:
			offsetX, offsetY = 108, -18
		case oldDir == 4 && newDir == 6:
			offsetX, offsetY = -108, 18
		case oldDir == 6 && newDir == 0:
			offsetX, offsetY = 108, -18
		case oldDir == 0 && newDir == 2:
			offsetX, offsetY = -108, 18
		}
	} else if pf.Width == 3 && pf.Depth == 3 && s.isRugFurniture(*pf) {
		// 3x3 rug with single directional asset - apply offsets for proper centering
		switch {
		case oldDir == 0 && newDir == 6:
			// Common 0->6 toggle for rugs; adjust to keep ghost/placement center stable
			offsetX, offsetY = 144, -36
		case oldDir == 6 && newDir == 0:
			offsetX, offsetY = -144, 36
		}
	} else if hasDir0Assets && !hasDir6Assets {
		// 1x1 furni case: has 2,0 assets, hflips 2->4 and 0->6
		switch {
		case oldDir == 2 && newDir == 4:
			offsetX, offsetY = 72, 0
		case oldDir == 4 && newDir == 6:
			offsetX, offsetY = 0, 0
		case oldDir == 6 && newDir == 0:
			offsetX, offsetY = -72, 0
		case oldDir == 0 && newDir == 2:
			offsetX, offsetY = 0, 0
		}
	} else if hasDir6Assets && !hasDir0Assets {
		// 1x1 furni case: has 2,6 assets, hflips 2->4 and 6->0
		switch {
		case oldDir == 2 && newDir == 4:
			offsetX, offsetY = 72, 0
		case oldDir == 4 && newDir == 6:
			offsetX, offsetY = -72, 0
		case oldDir == 6 && newDir == 0:
			offsetX, offsetY = 72, 0
		case oldDir == 0 && newDir == 2:
			offsetX, offsetY = -72, 0
		}
	} else if (pf.Width == 2 || pf.Depth == 2) && hasDir2Assets && hasDir4Assets && !hasDir0Assets && !hasDir6Assets {
		// 2-unit furniture (2x1 or 1x2) with only dir 2 and dir 4 assets
		switch {
		case oldDir == 2 && newDir == 4:
			offsetX, offsetY = 0, 0
		case oldDir == 4 && newDir == 2:
			offsetX, offsetY = 0, 0
		}
	} else if hasDir2Assets && !hasDir0Assets && !hasDir6Assets {
		// Direction 2 only case: only has dir 2 assets, only needs 2<->4 hflip offset
		switch {
		case oldDir == 2 && newDir == 4:
			offsetX, offsetY = 72, 0
		case oldDir == 4 && newDir == 2:
			offsetX, offsetY = -72, 0
		}
	} else if pf.Width == 3 && pf.Depth == 3 && s.isRugFurniture(*pf) {
		// 3x3 rug with single directional asset - apply offsets for proper centering
		switch {
		case oldDir == 0 && newDir == 6:
			offsetX, offsetY = 108, 108 // 2x standard offset for 3x3
		case oldDir == 6 && newDir == 0:
			offsetX, offsetY = 108, 108
		}
	}

	if offsetX != 0 || offsetY != 0 {
		pf.PixelOffsetX += offsetX
		pf.PixelOffsetY += offsetY
		fmt.Printf("[rotate] applied pixel offset %+d,%+d total=%+d,%+d\n", offsetX, offsetY, pf.PixelOffsetX, pf.PixelOffsetY)
	} else {
		fmt.Printf("[rotate] no offset applied\n")
	}

	// Apply automatic sofa slice depth boosting for the new direction
	s.applySofaSliceBoosts(*pf)
	// Apply Pinball slice depth boosting for the new direction
	s.applyPinballSliceBoosts(*pf)
	// Apply Vanilla Coke Table slice depth boosting for the new direction
	s.applyVanillaCokeTableSliceBoosts(*pf)

	// Recompute walkability for all affected squares (old and new coverage)
	affectedSquares := make(map[[2]int]bool)
	// Add squares that were previously covered (before rotation)
	for _, sq := range oldCoveredSquares {
		if sq != nil {
			affectedSquares[[2]int{sq.GetRow(), sq.GetCol()}] = true
		}
	}
	// Add squares that are currently covered (after rotation)
	newCoveredSquares := s.placement.coverageSquares(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
	for _, sq := range newCoveredSquares {
		if sq != nil {
			affectedSquares[[2]int{sq.GetRow(), sq.GetCol()}] = true
		}
	}
	// Update walkability for all affected squares
	for coords := range affectedSquares {
		s.placement.updateSquareWalkability(coords[0], coords[1])
	}

	// If rotating a seatable item, align any seated avatars on it to the new direction
	if s.isSeatableFurniture(*pf) {
		covered := s.placement.coverageSquares(pf.Row, pf.Col, pf.Width, pf.Depth, pf.Dir)
		for _, av := range s.avatars {
			if av == nil || av.Engine == nil || av.Square == nil {
				continue
			}
			if av.Engine.GetBodyAction() != "sit" {
				continue
			}
			ar := av.Square.GetRow()
			ac := av.Square.GetCol()
			for _, sq := range covered {
				if sq != nil && sq.GetRow() == ar && sq.GetCol() == ac {
					av.Engine.SetDirection(pf.Dir)
					break
				}
			}
		}
		// Also check the player avatar if not included above (guarded already by the slice loop)
		if s.playerAvatar != nil && s.playerAvatar.Engine != nil && s.playerAvatar.Square != nil && s.playerAvatar.Engine.GetBodyAction() == "sit" {
			ar := s.playerAvatar.Square.GetRow()
			ac := s.playerAvatar.Square.GetCol()
			for _, sq := range covered {
				if sq != nil && sq.GetRow() == ar && sq.GetCol() == ac {
					s.playerAvatar.Engine.SetDirection(pf.Dir)
					break
				}
			}
		}
	}

	// Send rotation packet over network
	if s.sendRotateIfAvailable != nil {
		// Y is the stack level (GridY / 10 for server compatibility, as server uses 1 per level, we use 10)
		y := pf.GridY / 10
		ho := pf.ServerHO
		// For 2x2 furniture, stored origin is shifted left by 1 for client rendering.
		// Server expects the unshifted column; adjust when sending rotation.
		sendRow := pf.Row
		sendCol := pf.Col
		if pf.Width == 2 && pf.Depth == 2 {
			sendCol = pf.Col + 1
		}
		log.Printf("Studio: Sending POS for rotation - pos=%s dir=%d at client[row=%d,col=%d] ho=%d",
			pf.PosID, pf.Dir, sendRow, sendCol, ho)
		if err := s.sendRotateIfAvailable(s.studio.StudioID, pf.PosID, sendRow, sendCol, y, pf.Dir, pf.State, ho); err != nil {
			log.Printf("Studio: Failed to send furniture rotation: %v", err)
		}
	}
}

// movePlacedFurni puts a placed furniture item back into ghost preview to be moved elsewhere.
// It retains the current rotation and size, and excludes the item from collisions by removing it first.
func (s *Scene) movePlacedFurni(pf *PlacedFurni) {
	if pf == nil || s.placement == nil || s.backpackManager == nil || s.catalog == nil {
		return
	}
	// Only room owner can move furniture
	if !s.IsRoomOwner() {
		return
	}
	// Find index by pointer equality and copy the value BEFORE removal to preserve identity
	idx := -1
	for i := range s.placedFurni {
		if &s.placedFurni[i] == pf {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}
	orig := s.placedFurni[idx] // make a safe copy before slice mutation

	// Before removal, mark any items stacked on top of this one as floating so they retain height/depth
	if s.placement != nil {
		origCovered := s.placement.coverageSquares(orig.Row, orig.Col, orig.Width, orig.Depth, orig.Dir)
		for i := range s.placedFurni {
			if i == idx {
				continue
			}
			top := &s.placedFurni[i]
			if top.GridY <= orig.GridY {
				continue
			}
			// If this top item overlaps any of the squares covered by the original (support) item
			existingCovered := s.placement.coverageSquares(top.Row, top.Col, top.Width, top.Depth, top.Dir)
			if squaresOverlap(origCovered, existingCovered) {
				// Cache its current stacked pixel offset and IAOT height to preserve while floating
				fh := s.placement.supportStackHeightPx(*top)
				top.Floating = true
				top.FloatingOffsetPx = fh
				ia := s.cumulativeIAOTHeightBelow(*top)
				top.FloatingIAOTHeight = ia
			}
		}
	}

	// Before removal, update walkability for squares previously covered by the item
	covered := s.placement.coverageSquares(orig.Row, orig.Col, orig.Width, orig.Depth, orig.Dir)
	// Remove the item from the list
	s.placedFurni = append(s.placedFurni[:idx], s.placedFurni[idx+1:]...)
	// Recompute walkability for those squares and neighbors impacted
	for _, sq := range covered {
		if sq != nil {
			s.placement.updateSquareWalkability(sq.GetRow(), sq.GetCol())
		}
	}

	// Start holding state using the original item's properties, preserving current rotation
	s.backpackManager.StartHoldingFromPlaced(orig)

	// Clear InfoStand selection to avoid stale references
	if s.infoStand != nil {
		s.infoStand.Clear()
	}
}

// pickupPlacedFurni removes a placed furniture item and returns it to the backpack
func (s *Scene) pickupPlacedFurni(pf *PlacedFurni) {
	if s == nil || pf == nil || s.placement == nil {
		return
	}
	// Only room owner can pick up furniture
	if !s.IsRoomOwner() {
		return
	}
	// Find index of the placed item by PosID (stable key)
	idx := -1
	for i := range s.placedFurni {
		if strings.TrimSpace(s.placedFurni[i].PosID) == strings.TrimSpace(pf.PosID) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}
	removed := s.placedFurni[idx]

	// Before removal, mark any items stacked on top of this one as floating so they retain height/depth
	origCovered := s.placement.coverageSquares(removed.Row, removed.Col, removed.Width, removed.Depth, removed.Dir)
	for i := range s.placedFurni {
		if i == idx {
			continue
		}
		top := &s.placedFurni[i]
		if top.GridY <= removed.GridY {
			continue
		}
		// If this top item overlaps any of the squares covered by the original (support) item
		existingCovered := s.placement.coverageSquares(top.Row, top.Col, top.Width, top.Depth, top.Dir)
		if squaresOverlap(origCovered, existingCovered) {
			// Cache its current stacked pixel offset and IAOT height to preserve while floating
			fh := s.placement.supportStackHeightPx(*top)
			top.Floating = true
			top.FloatingOffsetPx = fh
			ia := s.cumulativeIAOTHeightBelow(*top)
			top.FloatingIAOTHeight = ia
		}
	}

	// Remove from placed list
	s.placedFurni = append(s.placedFurni[:idx], s.placedFurni[idx+1:]...)
	// Update walkability for squares previously covered by this furniture
	covered := s.placement.coverageSquares(removed.Row, removed.Col, removed.Width, removed.Depth, removed.Dir)
	for _, sq := range covered {
		if sq != nil {
			s.placement.updateSquareWalkability(sq.GetRow(), sq.GetCol())
		}
	}

	// Send pickup packet over network
	if s.sendPickupIfAvailable != nil {
		log.Printf("Studio: Sending POS_BKPK for pos=%s", removed.PosID)
		if err := s.sendPickupIfAvailable(s.studio.StudioID, removed.PosID); err != nil {
			log.Printf("Studio: Failed to send furniture pickup: %v", err)
		}
	}

	// Return to backpack preserving original ProdID and PosID
	if s.backpackManager != nil {
		// Ensure no duplicate of this PosID exists in backpack first
		_ = s.backpackManager.RemoveItemByPosID(removed.PosID)
		item := BackpackItem{ProdID: removed.ProdID, PosID: removed.PosID, Attributes: map[string]interface{}{}}
		s.backpackManager.InsertItemAtFront(item)
		// Optionally open/show backpack
		s.backpackManager.ToggleOpen()
	}
}

// applyInitialDirectionOffsets applies the same directional pixel-offset corrections
// used during rotation, but for items that are being instantiated directly at a
// non-baseline direction from server POS. We simulate rotating from baseline dir 2
// through 4->6->0 until we reach the target direction, accumulating per-step offsets.
// applyInitialDirectionOffsets moved to pos_furni.go

// nextAllowedDirectionFor returns the next available even direction for a placed item based on assets.
func (s *Scene) nextAllowedDirectionFor(pf *PlacedFurni) int {
	allowed := s.allowedDirectionsForBase(pf.Base, pf.Width, pf.Depth)
	if len(allowed) == 0 {
		return (pf.Dir + 2) % 8
	}
	set := map[int]bool{}
	for _, d := range allowed {
		set[d] = true
	}
	try := pf.Dir
	for i := 0; i < 4; i++ {
		try = (try + 2) % 8
		if set[try] {
			return try
		}
	}
	return (pf.Dir + 2) % 8
}

// hasAssetsForDirection checks if furniture has assets for a specific direction
func (s *Scene) hasAssetsForDirection(pf *PlacedFurni, dir int) bool {
	if s.placement == nil || s.placement.mm == nil {
		return false
	}

	aw := max1(pf.Width)
	ad := max1(pf.Depth)
	if aw < ad {
		aw, ad = ad, aw
	}
	wStr := strconv.Itoa(aw)
	dStr := strconv.Itoa(ad)

	// Check for both a and b layer assets
	nameA := pf.Base + "_a_0_" + wStr + "_" + dStr + "_" + strconv.Itoa(dir) + "_0"
	nameB := pf.Base + "_b_0_" + wStr + "_" + dStr + "_" + strconv.Itoa(dir) + "_0"

	if _, ok := s.placement.mm.PathFor(nameA); ok {
		return true
	}
	if _, ok := s.placement.mm.PathFor(nameB); ok {
		return true
	}

	return false
}

// hasOnlyDir4Assets checks if furniture only has assets for direction 4 (no hflipping)
func (s *Scene) hasOnlyDir4Assets(base string, width, depth int) bool {
	if s.placement == nil || s.placement.mm == nil {
		return false
	}

	aw := width
	ad := depth
	if aw < ad {
		aw, ad = ad, aw
	}
	wStr := strconv.Itoa(max1(aw))
	dStr := strconv.Itoa(max1(ad))

	lingoDir := filepath.Join("Lingo", "cc_furniture[1]")
	mm, err := lingoassets.LoadMemberMapCached(lingoDir)
	if err != nil || mm == nil {
		return false
	}

	// Check if only direction 4 has actual assets (no mirrored fallbacks)
	checkDir := func(dir int) bool {
		nameA := base + "_a_0_" + wStr + "_" + dStr + "_" + strconv.Itoa(dir) + "_0"
		if _, ok := mm.PathFor(nameA); ok {
			return true
		}
		nameB := base + "_b_0_" + wStr + "_" + dStr + "_" + strconv.Itoa(dir) + "_0"
		if _, ok := mm.PathFor(nameB); ok {
			return true
		}
		return false
	}

	// Only dir 4 should have assets
	hasDir0 := checkDir(0)
	hasDir2 := checkDir(2)
	hasDir4 := checkDir(4)
	hasDir6 := checkDir(6)

	return hasDir4 && !hasDir0 && !hasDir2 && !hasDir6
}

// allowedDirectionsForBase inspects cc_furniture[1] member map to determine which directions exist
func (s *Scene) allowedDirectionsForBase(base string, width, depth int) []int {
	aw := width
	ad := depth
	if aw < ad {
		aw, ad = ad, aw
	}
	wStr := strconv.Itoa(max1(aw))
	dStr := strconv.Itoa(max1(ad))
	lingoDir := filepath.Join("Lingo", "cc_furniture[1]")
	mm, err := lingoassets.LoadMemberMapCached(lingoDir)
	if err != nil || mm == nil {
		return nil
	}
	// A direction is considered allowed if its own assets exist, or if assets exist
	// for its mirrored even direction (2<>4, 0<>6) which we can render via hflip.
	mirrorEven := func(d int) int {
		switch d {
		case 2:
			return 4
		case 4:
			return 2
		case 0:
			return 6
		case 6:
			return 0
		default:
			return d
		}
	}
	check := func(dir int) bool {
		nameA := base + "_a_0_" + wStr + "_" + dStr + "_" + strconv.Itoa(dir) + "_0"
		if _, ok := mm.PathFor(nameA); ok {
			return true
		}
		nameB := base + "_b_0_" + wStr + "_" + dStr + "_" + strconv.Itoa(dir) + "_0"
		if _, ok := mm.PathFor(nameB); ok {
			return true
		}
		// Try mirrored direction for hflip rendering
		alt := mirrorEven(dir)
		nameAAlt := base + "_a_0_" + wStr + "_" + dStr + "_" + strconv.Itoa(alt) + "_0"
		if _, ok := mm.PathFor(nameAAlt); ok {
			return true
		}
		nameBAlt := base + "_b_0_" + wStr + "_" + dStr + "_" + strconv.Itoa(alt) + "_0"
		if _, ok := mm.PathFor(nameBAlt); ok {
			return true
		}
		return false
	}
	var dirs []int
	for _, d := range []int{0, 2, 4, 6} {
		if check(d) {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// kickAvatarByName removes an avatar from the current room client-side (placeholder).
// In a networked setup this should send a gateway message to kick the user.
func (s *Scene) kickAvatarByName(screenName string) {
	if !s.IsRoomOwner() || strings.TrimSpace(screenName) == "" {
		return
	}
	// If the target is the local user, mark kicked to exit
	if strings.EqualFold(strings.TrimSpace(screenName), strings.TrimSpace(s.currentUserName)) {
		s.MarkKicked()
		return
	}
	// Find and remove the avatar from the avatars slice
	idx := -1
	for i := range s.avatars {
		if s.avatars[i] != nil && strings.EqualFold(strings.TrimSpace(s.avatars[i].GetScreenName()), strings.TrimSpace(screenName)) {
			idx = i
			break
		}
	}
	if idx >= 0 {
		s.avatars = append(s.avatars[:idx], s.avatars[idx+1:]...)
		// Also remove from BotsManager if this was a bot

		// Clear InfoStand selection if it was showing this avatar
		if s.infoStand != nil {
			s.infoStand.Clear()
		}
		// Show a simple system message as a chat bubble near the player
		if s.chatManager != nil && s.playerAvatar != nil {
			msg := screenName + " was kicked by the room owner."
			// Emit as a normal chat bubble near the player
			s.chatManager.EmitChatBubble(msg, s.playerAvatar)
		}
	}
}

func (s *Scene) drawText(screen *ebiten.Image, textStr string, centerX, centerY float64, clr color.RGBA, bold bool, size int) {
	// Prefer HUD font manager for consistent rendering
	if s.hudManager != nil {
		fm := s.hudManager.GetFontManager()
		if fm != nil {
			style := "plain"
			if bold {
				style = "bold"
			}
			face := fm.GetFont("Verdana", style, size)
			if face != nil {
				b := text.BoundString(face, textStr)
				x := int(centerX) - b.Dx()/2
				y := int(centerY) + b.Dy()/2
				text.Draw(screen, textStr, face, x, y, clr)
				return
			}
		}
	}
	// Fallback: no-op image draw to avoid import of ebitenutil; in practice fm should exist
}

// startDrinkDispenserApproach computes the adjacent walk tile based on furni anchor and direction,
// moves the player there, and arms a pending trigger to request a drink upon arrival.
func (s *Scene) startDrinkDispenserApproach(pf *PlacedFurni) {
	if s == nil || s.isoScene == nil || s.playerAvatar == nil || pf == nil || s.placement == nil {
		return
	}
	// Determine anchor row/col the same way stacking uses it
	anchorR, anchorC := s.placement.anchorRowColFor(*pf)

	// Compute adjacent tile cardinally based on furni direction
	tr, tc := anchorR, anchorC
	switch pf.Dir {
	case 2: // East-facing: stand south of anchor (increase row)
		tr = anchorR + 1
		tc = anchorC
	case 4: // South-facing: stand east of anchor (increase col)
		tr = anchorR
		tc = anchorC + 1
	case 6: // West-facing: stand north of anchor (decrease row)
		tr = anchorR - 1
		tc = anchorC
	case 0: // North-facing: stand west of anchor (decrease col)
		tr = anchorR
		tc = anchorC - 1
	default:
		// Fallback to west of anchor
		tc = anchorC - 1
	}

	grid := s.isoScene.GetGrid()
	if grid == nil {
		return
	}
	target := grid.GetSquareByRowCol(tr, tc)
	if target == nil || !target.IsWalkable() {
		// If the ideal adjacent is blocked, do not auto-trigger remote drink
		log.Printf("DrinkApproach: target [%d,%d] not walkable; aborting", tr, tc)
		return
	}

	// Plan path from current or next committed square
	start := s.playerAvatar.GetNextCommittedSquare()
	path := isoengine.FindPath(start, target, grid)
	if len(path) == 0 {
		// Already there or unreachable
		if s.playerAvatar.Square != nil && s.playerAvatar.Square.GetRow() == tr && s.playerAvatar.Square.GetCol() == tc {
			// We're already at the spot; send immediately
			if s.sendDrinkDispenserIfAvailable != nil && !s.playerHoldingCoke {
				_ = s.sendDrinkDispenserIfAvailable(s.studio.StudioID)
			}
		}
		return
	}

	// Send movement to server if available; otherwise move locally
	if s.sendMovePathIfAvailable != nil {
		pts := make([][2]int, 0, len(path))
		for _, sq := range path {
			pts = append(pts, [2]int{sq.GetRow(), sq.GetCol()})
		}
		s.sendMovePathIfAvailable(s.studio.StudioID, pts)
	} else {
		s.playerAvatar.Engine.StartWalking()
		s.playerAvatar.MoveToSquare(target, grid)
		s.playerHasMoved = true
	}

	// Arm pending drink trigger with timeout
	s.pendingDrinkActive = true
	s.pendingDrinkTargetRow = tr
	s.pendingDrinkTargetCol = tc
	s.pendingDrinkExpire = time.Now().Add(6 * time.Second)
	log.Printf("DrinkApproach: armed AV_DC trigger for [%d,%d] (anchor [%d,%d] dir=%d)", tr, tc, anchorR, anchorC, pf.Dir)
}

// startStereoApproach computes the adjacent walk tile and moves the player there;
// upon arrival it sends CD_R with the furniture PosID.
func (s *Scene) startStereoApproach(pf *PlacedFurni) {
	if s == nil || s.isoScene == nil || s.playerAvatar == nil || pf == nil || s.placement == nil {
		return
	}
	// If a performance is already active, remember to show alert (but still move to stereo)
	pendingAlert := s.nowPlayingActive
	// Determine anchor row/col like stacking
	anchorR, anchorC := s.placement.anchorRowColFor(*pf)

	// Compute adjacent tile cardinally based on furni direction
	tr, tc := anchorR, anchorC
	switch pf.Dir {
	case 2: // East-facing: stand south of anchor (increase row)
		tr = anchorR + 1
		tc = anchorC - 1
	case 4: // South-facing: stand east of anchor (increase col)
		tr = anchorR
		tc = anchorC + 1
	case 6: // West-facing: stand north of anchor (decrease row)
		tr = anchorR - 1
		tc = anchorC
	case 0: // North-facing: stand west of anchor (decrease col)
		tr = anchorR
		tc = anchorC - 1
	default:
		tc = anchorC - 1
	}

	grid := s.isoScene.GetGrid()
	if grid == nil {
		return
	}
	target := grid.GetSquareByRowCol(tr, tc)
	if target == nil || !target.IsWalkable() {
		log.Printf("StereoApproach: target [%d,%d] not walkable; aborting", tr, tc)
		return
	}

	// Plan path from current or next committed square
	start := s.playerAvatar.GetNextCommittedSquare()
	path := isoengine.FindPath(start, target, grid)
	if len(path) == 0 {
		// Already there or unreachable
		if s.playerAvatar.Square != nil && s.playerAvatar.Square.GetRow() == tr && s.playerAvatar.Square.GetCol() == tc {
			// We're already at the spot; send immediately
			if !pendingAlert {
				if s.sendCDRequestIfAvailable != nil && strings.TrimSpace(pf.PosID) != "" {
					_ = s.sendCDRequestIfAvailable(s.studio.StudioID, pf.PosID)
				}
			} else if s.hudManager != nil {
				s.hudManager.OpenGeneralAlert("Attention", "There's a song already playing. Wait for your turn.")
			}
		}
		return
	}

	// Send movement to server if available; otherwise move locally
	if s.sendMovePathIfAvailable != nil {
		pts := make([][2]int, 0, len(path))
		for _, sq := range path {
			pts = append(pts, [2]int{sq.GetRow(), sq.GetCol()})
		}
		s.sendMovePathIfAvailable(s.studio.StudioID, pts)
	} else {
		s.playerAvatar.Engine.StartWalking()
		s.playerAvatar.MoveToSquare(target, grid)
		s.playerHasMoved = true
	}

	// Arm pending stereo trigger with timeout
	s.pendingStereoActive = true
	s.pendingStereoTargetRow = tr
	s.pendingStereoTargetCol = tc
	s.pendingStereoPosID = strings.TrimSpace(pf.PosID)
	s.pendingStereoExpire = time.Now().Add(6 * time.Second)
	log.Printf("StereoApproach: armed CD_R trigger for [%d,%d] (anchor [%d,%d] dir=%d posID=%s)", tr, tc, anchorR, anchorC, pf.Dir, s.pendingStereoPosID)
}

// InfoStand returns the InfoStand overlay pointer
func (s *Scene) InfoStand() *InfoStand {
	return s.infoStand
}
