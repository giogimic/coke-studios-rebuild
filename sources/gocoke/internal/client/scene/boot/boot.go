// Boot scene renders Lingo general_loader.window using ripped interface slices
// and shows text from Text_English.txt. Fonts should come from C:\GoCoke\fonts
// where available for parity.

package boot

import (
	"fmt"
	"image/color"
	"log"
	mrand "math/rand"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"gocoke/internal/client/assets/catalog"
	"gocoke/internal/client/assets/features"
	"gocoke/internal/client/assets/studios"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/net/gateway"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"

	"gocoke/internal/client/ui"

	"context"
	"gocoke/internal/client/net/realtime"
	"gocoke/internal/client/session"
)

type loadState int

const (
	stateInit loadState = iota
	stateLoadingTip
	stateTipLoaded
	stateLoadingFeatures
	stateFeaturesLoaded
	stateLoadingCatalog
	stateCatalogLoaded
	stateLoadingStudios
	stateStudiosLoaded
	stateError
	stateComplete
)

type Scene struct {
	tm      *textmgr.Manager
	lw      *loaderWindow
	fp      *framePlayer
	last    time.Time
	dataDir string

	// Fonts
	fm *ui.FontManager

	// Network state
	state      loadState
	httpClient *http.Client
	gatewayURL string
	tip        string
	errorMsg   string

	// Parsed data
	featureSet *features.FeatureSet
	catalog    *catalog.Catalog
	studioList *studios.StudioList

	// Scripted status sequence for initial entry
	script        []bootScriptStep
	scriptIdx     int
	scriptProg    int
	scriptStart   time.Time
	currentStatus string
	tipActive     bool
	tipDisplay    string
	rng           *mrand.Rand

	// Realtime login state
	rtStarted bool
}

type bootScriptStep struct {
	kind  string // "progress" or "text"
	label string
	total int // for progress steps
}

func New(dataDir string, httpClient *http.Client, gatewayURL string) *Scene {
	tm := textmgr.New()
	path := filepath.Join(dataDir, "Text_English.txt")
	if err := tm.LoadFile(path); err != nil {
		log.Printf("Boot: failed to load text file %s: %v", path, err)
	}
	// Load ripped interface parts from Lingo assets
	lingoDir := filepath.Join("Lingo", "cc_interface[1]")
	lw := newLoaderWindow(lingoDir)
	// Load pre-rendered frames if available
	fp, _ := loadFramePlayer(filepath.Join("assets", "loading", "frames"))

	s := &Scene{
		tm:         tm,
		lw:         lw,
		fp:         fp,
		dataDir:    dataDir,
		state:      stateInit,
		httpClient: httpClient,
		gatewayURL: gatewayURL,
	}

	// Fonts
	s.fm = ui.NewFontManager()
	// Seed a per-scene RNG for tip selection
	s.rng = mrand.New(mrand.NewSource(time.Now().UnixNano()))
	// Initialize scripted status sequence
	s.script = []bootScriptStep{
		{kind: "progress", label: "Loading Coke Studios", total: 3},
		{kind: "progress", label: "Loading Coke Studios", total: 4},
		{kind: "text", label: "Loading Coke Studios"},
		{kind: "text", label: "Checking Status"}, // show tip here
		{kind: "text", label: "Loading Coke Studios"},
		{kind: "progress", label: "Loading Coke Studios", total: 4},
		{kind: "text", label: "Connecting.."},
		{kind: "text", label: "Entering: Front Door"},
	}
	s.scriptIdx = 0
	s.scriptProg = 0
	s.scriptStart = time.Now()

	return s
}

// NewLobby creates a boot scene configured for the lobby entry flow used on studio exit.
func NewLobby(dataDir string, httpClient *http.Client, gatewayURL string) *Scene {
	s := New(dataDir, httpClient, gatewayURL)
	// Override the scripted status sequence to the lobby variant
	s.script = []bootScriptStep{
		{kind: "text", label: "Entering Lobby"},
		{kind: "text", label: "Connecting.."},
		{kind: "text", label: "Entering: Front Door"},
	}
	// Activate tip immediately and pick one (server tip or SAFETY_TIP fallback)
	s.tipActive = true
	s.tipDisplay = s.pickTip()
	return s
}

// pickTip returns the best available tip: server-provided if present,
// else a SAFETY_TIP from Text_English.txt with studio-style exclusions.
func (s *Scene) pickTip() string {
	if strings.TrimSpace(s.tip) != "" {
		return s.tip
	}
	if s.tm == nil {
		return ""
	}
	alts := s.tm.All("SAFETY_TIP")
	if len(alts) == 0 {
		return ""
	}
	// Exclude boot-only lines (same as studio)
	ex1 := "Welcome to Coke Studios. Take a minute to review our safety tips."
	ex2 := "If you encounter any problems, please use the Call for Assistance button to gain the assistance of an online monitor."
	filtered := make([]string, 0, len(alts))
	for _, t := range alts {
		tv := strings.TrimSpace(t)
		if strings.EqualFold(tv, strings.TrimSpace(ex1)) || strings.EqualFold(tv, strings.TrimSpace(ex2)) {
			continue
		}
		filtered = append(filtered, t)
	}
	candidates := filtered
	if len(candidates) == 0 {
		candidates = alts
	}
	if s.rng == nil {
		s.rng = mrand.New(mrand.NewSource(time.Now().UnixNano()))
	}
	idx := s.rng.Intn(len(candidates))
	return candidates[idx]
}

func (s *Scene) Update() error {
	// Advance frame player using elapsed time
	now := time.Now()
	if !s.last.IsZero() && s.fp != nil {
		s.fp.Update(now.Sub(s.last))
	}
	s.last = now

	// Advance scripted status sequence
	s.advanceScript(now)
	// If we've loaded studio data and finished the script, mark complete immediately
	if s.state == stateStudiosLoaded && s.scriptIdx >= len(s.script) {
		s.state = stateComplete
	}

	// State machine for loading sequence (mirrors original Lingo PreloadTextsBehavior)
	switch s.state {
	case stateInit:
		// Start getTip call
		s.state = stateLoadingTip
		go s.loadTip()

	case stateLoadingTip:
		// Waiting for getTip to complete (handled in goroutine)

	case stateTipLoaded:
		// Tip loaded, now load FeatureSet.txt
		log.Printf("Boot: Tip loaded: %s", s.tip)
		s.state = stateLoadingFeatures
		go s.loadFeatures()

	case stateLoadingFeatures:
		// Waiting for features to load

	case stateFeaturesLoaded:
		// Features loaded, now load Catalogue_English.txt
		log.Printf("Boot: Features loaded (%d features)", len(s.featureSet.GetAll()))
		s.state = stateLoadingCatalog
		go s.loadCatalog()

	case stateLoadingCatalog:
		// Waiting for catalog to load

	case stateCatalogLoaded:
		// Catalog loaded, now load PublicStudios_English.xml
		log.Printf("Boot: Catalog loaded (%d pages, %d items)", len(s.catalog.Pages), len(s.catalog.Items))
		s.state = stateLoadingStudios
		go s.loadStudios()

	case stateLoadingStudios:
		// Waiting for studios to load

	case stateStudiosLoaded:
		// All data loaded successfully
		log.Printf("Boot: Studios loaded (%d studios)", len(s.studioList.Studios))
		// Completion is driven by the scripted status sequence

	case stateError:
		// Error occurred, could retry or show error
		log.Printf("Boot: Error: %s", s.errorMsg)

	case stateComplete:
		// Loading complete, ready to transition to navigator
	}

	return nil
}

func (s *Scene) advanceScript(now time.Time) {
	if s.scriptIdx >= len(s.script) {
		return
	}
	step := s.script[s.scriptIdx]
	elapsed := now.Sub(s.scriptStart)
	progressInterval := 90 * time.Millisecond
	textHold := 500 * time.Millisecond
	if step.kind == "progress" {
		if s.scriptProg == 0 {
			s.scriptProg = 1
			s.scriptStart = now
		}
		s.currentStatus = fmt.Sprintf("%s %d of %d", step.label, s.scriptProg, step.total)
		if s.scriptProg < step.total {
			if elapsed >= progressInterval {
				s.scriptProg++
				s.scriptStart = now
			}
		} else {
			// Hold briefly on final count, then move to next
			if elapsed >= 200*time.Millisecond {
				s.scriptIdx++
				s.scriptProg = 0
				s.scriptStart = now
			}
		}
		return
	}
	// text step
	if s.scriptProg == 0 {
		s.scriptProg = 1
		s.scriptStart = now
	}
	s.currentStatus = step.label
	if step.label == "Checking Status" {
		// Activate tip display and choose a tip once
		if !s.tipActive {
			s.tipActive = true
			s.tipDisplay = s.pickTip()
		}
	}
	if step.label == "Connecting.." {
		if !s.rtStarted {
			s.rtStarted = true
			go s.startRealtimeLogin()
		}
	}
	if elapsed >= textHold {
		s.scriptIdx++
		s.scriptProg = 0
		s.scriptStart = now
	}
}

// startRealtimeLogin connects to the game server (lobby) and performs System.Login.
func (s *Scene) startRealtimeLogin() {
	// Resolve game server for lobby (VIS$0)
	srv, err := gateway.GetGameServerByStudioID(s.httpClient, s.gatewayURL, "VIS$0")
	if err != nil {
		log.Printf("Boot: getGameServerByStudioID failed: %v", err)
		return
	}
	m, ok := srv.(map[string]any)
	if !ok {
		log.Printf("Boot: unexpected GS response: %T", srv)
		return
	}
	host := fmt.Sprint(m["server"])
	if strings.TrimSpace(host) == "" || host == "<nil>" {
		host = fmt.Sprint(m["dns"])
	}
	port := fmt.Sprint(m["port"])
	sn, sec := session.GetCredentials()
	if strings.TrimSpace(sn) == "" || strings.TrimSpace(sec) == "" {
		log.Printf("Boot: missing credentials; skipping realtime login")
		return
	}
	rc := realtime.New(sec, log.Default())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := rc.Connect(ctx, host, port); err != nil {
		log.Printf("Boot: realtime connect failed: %v", err)
		return
	}
	// Send login without waiting for Accepted; some servers may not ack
	if err := rc.SendLogin(sn, sec); err != nil {
		log.Printf("Boot: realtime login send failed: %v", err)
		return
	}
	// Expose client immediately for reuse on room entry
	session.SetRealtimeClient(rc)
	session.SetRealtimeEndpoint(host, port)
	session.SetRealtimeReady(true)
	log.Printf("Boot: realtime login sent (ready)")
}

func (s *Scene) loadTip() {
	// Try to fetch a tip from the server, but don't block boot if offline.
	tip, err := gateway.GetTip(s.httpClient, s.gatewayURL)
	if err != nil {
		// Fallback: proceed without a tip (no on-screen error)
		log.Printf("Boot: getTip failed, continuing without tip: %v", err)
		s.tip = "" // ensures no tip text is drawn
		s.state = stateTipLoaded
		return
	}
	s.tip = tip
	s.state = stateTipLoaded
}

func (s *Scene) loadFeatures() {
	path := filepath.Join(s.dataDir, "FeatureSet.txt")
	featureSet, err := features.LoadFile(path)
	if err != nil {
		s.errorMsg = fmt.Sprintf("FeatureSet.txt load failed: %v", err)
		s.state = stateError
		return
	}
	s.featureSet = featureSet
	s.state = stateFeaturesLoaded
}

func (s *Scene) loadCatalog() {
	path := filepath.Join(s.dataDir, "Catalogue_English.txt")
	catalog, err := catalog.LoadFile(path)
	if err != nil {
		s.errorMsg = fmt.Sprintf("Catalogue_English.txt load failed: %v", err)
		s.state = stateError
		return
	}
	s.catalog = catalog
	s.state = stateCatalogLoaded
}

func (s *Scene) loadStudios() {
	path := filepath.Join(s.dataDir, "PublicStudios_English.xml")
	studioList, err := studios.LoadFile(path)
	if err != nil {
		s.errorMsg = fmt.Sprintf("PublicStudios_English.xml load failed: %v", err)
		s.state = stateError
		return
	}

	// Fetch real private studios via AMF and merge
	if ps, err := s.fetchPrivateStudios(); err == nil && len(ps) > 0 {
		studioList.Studios = append(studioList.Studios, ps...)
		log.Printf("Boot: merged %d private studios from backend (total now %d)", len(ps), len(studioList.Studios))
	} else if err != nil {
		log.Printf("Boot: getAllPrivateStudios failed (continuing with public only): %v", err)
	}

	s.studioList = studioList
	s.state = stateStudiosLoaded
}

func (s *Scene) fetchPrivateStudios() ([]studios.Studio, error) {
	v, err := gateway.GetAllPrivateStudios(s.httpClient, s.gatewayURL)
	if err != nil {
		return nil, err
	}
	// Handle StrictArray ([]any) and ECMAArray (map[string]any with numeric keys)
	var elems []any
	switch x := v.(type) {
	case []any:
		elems = x
	case map[string]any:
		// ECMA array: expect keys "0","1",..., and possibly "length"
		length := 0
		if l, ok := x["length"].(float64); ok {
			length = int(l)
		}
		if length == 0 {
			length = len(x)
		}
		for i := 0; i < length; i++ {
			key := fmt.Sprintf("%d", i)
			if el, ok := x[key]; ok {
				elems = append(elems, el)
			}
		}
	default:
		return nil, fmt.Errorf("unexpected private studios type: %T", v)
	}
	out := make([]studios.Studio, 0, len(elems))
	for _, el := range elems {
		m, ok := el.(map[string]any)
		if !ok {
			continue
		}
		st := studios.Studio{}
		// Case-insensitive get helper
		get := func(mm map[string]any, key string) (any, bool) {
			if v, ok := mm[key]; ok {
				return v, true
			}
			lk := strings.ToLower(key)
			for k, v := range mm {
				if strings.ToLower(k) == lk {
					return v, true
				}
			}
			return nil, false
		}
		// Strings
		if v, ok := get(m, "studioId"); ok {
			if s1, ok := v.(string); ok {
				st.StudioID = s1
			}
		}
		if v, ok := get(m, "name"); ok {
			if s1, ok := v.(string); ok {
				st.Name = s1
			}
		}
		if v, ok := get(m, "description"); ok {
			if s1, ok := v.(string); ok {
				st.Description = s1
			}
		}
		if v, ok := get(m, "ownerName"); ok {
			if s1, ok := v.(string); ok {
				st.Owner = s1
			}
		} else if v, ok := get(m, "owner"); ok {
			if s1, ok := v.(string); ok {
				st.Owner = s1
			}
		}
		if v, ok := get(m, "server"); ok {
			if s1, ok := v.(string); ok {
				st.Server = s1
			}
		}
		// Ints
		if v, ok := get(m, "location"); ok {
			switch t := v.(type) {
			case float64:
				st.Location = int(t)
			case int:
				st.Location = t
			}
		}
		if v, ok := get(m, "layout"); ok {
			switch t := v.(type) {
			case float64:
				lt := int(t)
				if lt == 2 {
					lt = 4
				} else if lt == 4 {
					lt = 2
				}
				st.Layout = lt
			case int:
				lt := t
				if lt == 2 {
					lt = 4
				} else if lt == 4 {
					lt = 2
				}
				st.Layout = lt
			}
		}
		if v, ok := get(m, "currentOccupancy"); ok {
			switch t := v.(type) {
			case float64:
				st.UserCount = int(t)
			case int:
				st.UserCount = t
			}
		}
		if v, ok := get(m, "maxOccupancy"); ok {
			switch t := v.(type) {
			case float64:
				st.Capacity = int(t)
			case int:
				st.Capacity = t
			}
		}
		if st.StudioID != "" && st.Name != "" {
			out = append(out, st)
		}
	}
	return out, nil
}

func getString(m map[string]any, key string) (string, bool) {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

func getInt(m map[string]any, key string) (int, bool) {
	if v, ok := m[key]; ok {
		switch x := v.(type) {
		case float64:
			return int(x), true
		case int:
			return x, true
		}
	}
	return 0, false
}

func (s *Scene) drawText(screen *ebiten.Image, str string, centerX, centerY float64, clr color.RGBA, bold bool, size int) {
	face := s.fm.GetFont("Verdana", "plain", size)
	if bold {
		face = s.fm.GetFont("Verdana", "bold", size)
	}
	if face == nil {
		return
	}
	b := text.BoundString(face, str)
	x := int(centerX) - b.Dx()/2
	y := int(centerY) + b.Dy()/2
	text.Draw(screen, str, face, x, y, clr)
}

func (s *Scene) getStatusText() string {
	// Mirror original Lingo loading sequence text
	switch s.state {
	case stateInit, stateLoadingTip:
		return "Loading Coke Studios..."
	case stateTipLoaded, stateLoadingFeatures:
		return "Loading Session Beans..."
	case stateFeaturesLoaded, stateLoadingCatalog:
		return "Loading Catalog..."
	case stateCatalogLoaded, stateLoadingStudios:
		return "Loading Studio Data..."
	case stateStudiosLoaded, stateComplete:
		return "Ready!"
	case stateError:
		return "Error Loading"
	default:
		return "Loading..."
	}
}

// GetFeatureSet returns the loaded feature set (for other scenes)
func (s *Scene) GetFeatureSet() *features.FeatureSet {
	return s.featureSet
}

// GetCatalog returns the loaded catalog (for other scenes)
func (s *Scene) GetCatalog() *catalog.Catalog {
	return s.catalog
}

// GetStudioList returns the loaded studio list (for other scenes)
func (s *Scene) GetStudioList() *studios.StudioList {
	return s.studioList
}

// IsComplete returns true when all loading is finished
func (s *Scene) IsComplete() bool {
	return s.state == stateComplete
}

// CreateNavigatorScene creates a navigator scene with the loaded data
func (s *Scene) CreateNavigatorScene(dataDir string) interface{} {
	// Import the navigator package dynamically to avoid circular imports
	// This will be called by the scene manager when boot is complete
	return struct {
		DataDir    string
		FeatureSet interface{}
		StudioList interface{}
	}{
		DataDir:    dataDir,
		FeatureSet: s.featureSet,
		StudioList: s.studioList,
	}
}

func (s *Scene) Draw(screen *ebiten.Image) {
	// Clear background to match original (black)
	screen.Fill(color.RGBA{0x00, 0x00, 0x00, 0xFF})

	// Calculate center position for loader (original was around 222,197 to 538,323 in 1520x1040 stage)
	screenW, screenH := screen.Bounds().Dx(), screen.Bounds().Dy()
	centerX, centerY := float64(screenW/2), float64(screenH/2)

	// Draw loading animation in center (this is the main visual element)
	if s.fp != nil {
		// Position animation in center (317x127 from manifest)
		animX := centerX - 158.5 // 317/2
		animY := centerY - 63.5  // 127/2
		s.fp.Draw(screen, animX, animY)
	}

	// Draw loading status text above the animation (bold like studio)
	status := s.currentStatus
	if status == "" {
		status = s.getStatusText()
	}
	s.drawText(screen, status, centerX, centerY-90, color.RGBA{255, 255, 255, 255}, true, 10)

	// Draw tip from Checking Status onward until sequence completion
	if s.tipActive && s.tipDisplay != "" {
		s.drawText(screen, s.tipDisplay, centerX, centerY+90, color.RGBA{255, 255, 255, 255}, true, 10)
	} else if s.errorMsg != "" {
		s.drawText(screen, s.errorMsg, centerX, centerY+90, color.RGBA{255, 100, 100, 255}, false, 10)
	}
}

func (s *Scene) Layout(ow, oh int) (int, int) { return 960, 540 }
