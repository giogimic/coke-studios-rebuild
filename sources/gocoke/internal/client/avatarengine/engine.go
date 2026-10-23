package avatarengine

import (
	"fmt"
	"log"
	"time"

	"image/color"

	"gocoke/internal/client/core"

	"github.com/hajimehoshi/ebiten/v2"
)

// AvatarEngine represents a single avatar instance with all its properties and state
// Matches the Lingo AvatarEngine behavior with 1:1 parity
type AvatarEngine struct {
	// Core configuration
	Width      int
	Height     int
	Scale      int // 50, 100, or 150 (percentage)
	Direction  int // 0=North, 1=NE, 2=East, 3=SE, 7=NW (isometric)
	ColorDepth int // 16 for Windows, 32 for others

	// Visual effects
	DefaultInk            ebiten.CompositeMode // Default ink effect
	DoFloodFill           bool                 // Flood fill background
	FloodFillColor        color.Color          // Color for flood fill
	SpriteBackgroundColor color.Color          // Background color for sprites

	// Avatar appearance attributes
	Attributes map[string]BodyPartAppearance // Body part -> appearance (style + color)
	DrawOrder  []string                      // Rendering order for body parts

	// Animation state
	AnimStep    int    // Current animation frame (0-3)
	AnimStepMax int    // Maximum animation frames (usually 7 for compatibility)
	BodyAction  string // "std", "wlk", "sit"
	FaceAction  string // "std", "spk", "sml", "agr", "srp", "sad"
	EyeAction   string // "std", "eyb" (blink) – independent of face
	ItemAction  string // Action for carried items

	// Behavioral timers (matching Lingo timing)
	FPS                    int
	FPSTimer               time.Time
	FPSTimerLength         time.Duration // ~67ms for 15 FPS
	WalkAnimScale          float64       // Multiplier for walk anim frame interval
	previousAnimScale      float64       // Store previous anim scale when overriding
	BlinkTimer             time.Time
	BlinkTimerLength       time.Duration // randomized around 3s
	ChatTimer              time.Time
	ChatTimerLength        time.Duration // 1 second
	EmoteTimer             time.Time
	EmoteTimerLength       time.Duration // 5 seconds
	CarryTimer             time.Time
	CarryTimerLength       time.Duration // 5 minutes
	DrinkTimer             time.Time
	DrinkWaitTimerLength   time.Duration // 5 seconds
	DrinkActionTimerLength time.Duration // 400ms
	DrinkTimerLength       time.Duration // Current drink timer length
	SleepTimer             time.Time
	SleepTimerLength       time.Duration // 5 minutes

	// Behavioral state flags
	IsBlink              bool
	blinkWasSetThisFrame bool // internal: ensure blink lasts one frame
	IsChatting           bool
	IsEmoting            bool
	IsDancing            bool
	IsCarrying           bool
	IsDrinking           bool
	IsSleeping           bool

	// Rendering
	Buffer       *ebiten.Image            // Composed avatar image
	ShadowBuffer *ebiten.Image            // Shadow buffer
	SourceImages map[string]*ebiten.Image // Cached body part images
	AssetManager *AssetManager            // Asset loading and caching

	// Configuration
	Config *AvatarConfig

	// Position and registration
	RegistrationPoint core.Point // Registration point for positioning
	HeightOffset      int        // Vertical offset adjustment
	screenPos         core.Point2D
	lastFrameTime     time.Time

	// Optional global draw offset (screen-space) to tweak sit alignment
	GlobalDrawOffsetX int
	GlobalDrawOffsetY int
}

// NewAvatarEngine creates a new avatar engine instance
// Matches Lingo AvatarEngine.new() behavior
func NewAvatarEngine(attributes string, scale int, previewOnly bool, dataDir string) (*AvatarEngine, error) {
	engine := &AvatarEngine{
		// Core dimensions (from Lingo)
		Width:  77,
		Height: 110,
		Scale:  100, // Default scale

		// Default state
		Direction:   2, // East facing
		AnimStep:    0,
		AnimStepMax: 7, // Lingo compatibility
		BodyAction:  "std",
		FaceAction:  "std",
		EyeAction:   "std",
		ItemAction:  "",

		// Visual effects (matching Lingo defaults)
		DefaultInk:            ebiten.CompositeModeSourceOver, // 36 is copy, 41 is blend
		DoFloodFill:           false,                          // Depends on studio type
		SpriteBackgroundColor: color.RGBA{R: 255, G: 255, B: 255, A: 255},
		FloodFillColor:        color.RGBA{R: 100, G: 100, B: 100, A: 255},

		// Timing configuration (matching Lingo exactly)
		FPS:                    15,
		FPSTimerLength:         time.Duration(1000/15) * time.Millisecond, // ~67ms
		WalkAnimScale:          1.0,
		BlinkTimerLength:       3000 * time.Millisecond,
		ChatTimerLength:        1000 * time.Millisecond,
		EmoteTimerLength:       5000 * time.Millisecond,
		CarryTimerLength:       300000 * time.Millisecond, // 5 minutes
		DrinkWaitTimerLength:   5000 * time.Millisecond,
		DrinkActionTimerLength: 400 * time.Millisecond,
		SleepTimerLength:       300000 * time.Millisecond, // 5 minutes

		// Initialize timers
		FPSTimer:   time.Now(),
		BlinkTimer: time.Now(),
		ChatTimer:  time.Now(),
		EmoteTimer: time.Now(),
		CarryTimer: time.Now(),
		DrinkTimer: time.Now(),
		SleepTimer: time.Now(),

		// State
		IsBlink:              true, // enable blinking behavior
		blinkWasSetThisFrame: false,
		IsChatting:           false,
		IsEmoting:            false,
		IsDancing:            false,
		IsCarrying:           false,
		IsDrinking:           false,
		IsSleeping:           false,

		// Timers
		lastFrameTime: time.Now(),

		// Registration point (from Lingo)
		RegistrationPoint: core.Point{X: 90, Y: 255},
		HeightOffset:      0,

		// Initialize collections
		Attributes:   make(map[string]BodyPartAppearance),
		SourceImages: make(map[string]*ebiten.Image),
	}

	// Set scale if provided
	if scale > 0 {
		engine.Scale = scale
	}

	// Set color depth based on platform (Lingo behavior)
	engine.ColorDepth = 32 // Default to 32-bit (Mac behavior)
	// TODO: Detect Windows and set to 16-bit if needed

	// Initialize asset manager
	var err error
	engine.AssetManager, err = NewAssetManager(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create asset manager: %w", err)
	}

	// Load configuration
	engine.Config, err = LoadAvatarConfig(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load avatar config: %w", err)
	}

	// Set direction-specific draw order (from Lingo)
	engine.updateDrawOrder()

	// Parse attributes if provided
	if attributes != "" {
		err = engine.ParseAttributeString(attributes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse attributes: %w", err)
		}
	} else {
		// Set default attributes
		engine.setDefaultAttributes()
	}

	// Set flood fill based on preview mode (matches Lingo's setFloodFill)
	engine.SetFloodFill(previewOnly)

	// Set drink timer length to wait length initially
	engine.DrinkTimerLength = engine.DrinkWaitTimerLength

	// Create initial buffer
	err = engine.createBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to create buffer: %w", err)
	}

	// Apply scale
	engine.setScale(engine.Scale)

	// Slow walking animation by 30% for better sync with movement speed
	engine.WalkAnimScale = 1.3

	// Initial draw
	err = engine.Draw()
	if err != nil {
		return nil, fmt.Errorf("failed to initial draw: %w", err)
	}
	// Initial shadow draw
	err = engine.DrawShadow()
	if err != nil {
		return nil, fmt.Errorf("failed to initial shadow draw: %w", err)
	}

	log.Printf("AvatarEngine: created avatar with scale %d%%, direction %d", engine.Scale, engine.Direction)
	return engine, nil
}

// ParseAttributeString parses a Lingo-style attribute string and applies it to the engine.
func (e *AvatarEngine) ParseAttributeString(s string) error {
	appearances, err := ParseAttributeString(s)
	if err != nil {
		return err
	}
	e.Attributes = appearances
	log.Printf("AvatarEngine: parsed and set attributes for %d body parts", len(e.Attributes))
	return nil
}

// createBuffer creates the rendering buffer for the avatar
func (e *AvatarEngine) createBuffer() error {
	// Lingo creates an unscaled buffer of 77x(110+20)
	baseWidth := e.Width
	baseHeight := e.Height + 20
	e.Buffer = ebiten.NewImage(baseWidth, baseHeight)
	e.ShadowBuffer = ebiten.NewImage(baseWidth, baseHeight)

	log.Printf("AvatarEngine: created buffer %dx%d (unscaled base)", baseWidth, baseHeight)
	return nil
}

// setScale applies scaling to the avatar
func (e *AvatarEngine) setScale(scale int) {
	e.Scale = scale
	// In Lingo, scaling is applied when copying the composed buffer to the display member,
	// not during composition. We therefore keep the composition buffer at base resolution.
	log.Printf("AvatarEngine: set scale to %d%% (composition remains unscaled)", scale)
}

// setDefaultAttributes sets up default avatar attributes
func (e *AvatarEngine) setDefaultAttributes() {
	// Corresponds to Lingo's sDefaultString
	defaultString := "hr=017/238,238,238&hd=001/115,92,69&ey=001/238,238,238&fc=001/115,92,69&bd=001/115,92,69&lh=001/115,92,69&rh=001/115,92,69&ch=027/255,255,255&ls=012/255,255,255&rs=012/255,255,255&lg=014/255,255,255&sh=005/238,238,238"
	err := e.ParseAttributeString(defaultString)
	if err != nil {
		// This should not happen with a hardcoded default string
		log.Fatalf("FATAL: could not parse default attribute string: %v", err)
	}
	log.Printf("AvatarEngine: set default attributes")
}

// GetFPS returns the current FPS setting
func (e *AvatarEngine) GetFPS() int {
	return e.FPS
}

// GetBuffer returns the current rendered avatar buffer
func (e *AvatarEngine) GetBuffer() *ebiten.Image {
	return e.Buffer
}

// GetShadowBuffer returns the current rendered shadow buffer
func (e *AvatarEngine) GetShadowBuffer() *ebiten.Image {
	return e.ShadowBuffer
}

// GetScale returns the current scale percentage
func (e *AvatarEngine) GetScale() int {
	return e.Scale
}

// GetHeightOffset returns the vertical offset for the avatar
func (e *AvatarEngine) GetHeightOffset() int {
	return e.HeightOffset
}

// GetDirection returns the current facing direction
func (e *AvatarEngine) GetDirection() int {
	return e.Direction
}

// SetDirection sets the avatar's facing direction
func (e *AvatarEngine) SetDirection(direction int) {
	// Validate direction (0-7). Unsupported asset directions are mirrored via getDirectionAndFlip.
	validDirections := map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true}
	if !validDirections[direction] {
		log.Printf("AvatarEngine: invalid direction %d, keeping %d", direction, e.Direction)
		return
	}

	e.Direction = direction

	// Update draw order for new direction
	e.updateDrawOrder()
	e.wakeUp()

	log.Printf("AvatarEngine: set direction to %d", direction)
}

// Update updates the avatar's animation and behavioral state
// Should be called every frame to maintain proper timing
func (e *AvatarEngine) Update() error {
	now := time.Now()
	delta := now.Sub(e.lastFrameTime)

	// Advance animation frame if enough time has passed for 15 FPS
	frameLen := e.FPSTimerLength
	if e.BodyAction == "wlk" {
		frameLen = time.Duration(float64(frameLen) * e.WalkAnimScale)
	}
	// Also apply animation scale when speaking (for face animation)
	if e.IsChatting && e.FaceAction == "spk" {
		frameLen = time.Duration(float64(frameLen) * e.WalkAnimScale)
	}
	if delta >= frameLen {
		e.lastFrameTime = now

		// Advance a single shared animation step each tick
		e.AdvanceAnimation()

		// Clear one-frame blink flag after frame advance
		if e.blinkWasSetThisFrame {
			e.blinkWasSetThisFrame = false
			// After the blink frame, return eyes to std
			if e.EyeAction == "eyb" {
				e.EyeAction = "std"
			}
		}
	}

	// Update behavioral timers
	e.updateBehaviors(now)

	return nil
}

// updateBehaviors updates behavioral state based on timers
func (e *AvatarEngine) updateBehaviors(now time.Time) {
	// Blinking
	if e.IsBlink && now.Sub(e.BlinkTimer) >= e.BlinkTimerLength {
		e.BlinkTimer = now
		// Trigger a one-frame blink (ey -> eyb)
		e.EyeAction = "eyb"
		e.blinkWasSetThisFrame = true
		// Randomize next blink between 2-4 seconds like Lingo behavior
		e.BlinkTimerLength = time.Duration(2000+int64(time.Now().UnixNano()%2000)) * time.Millisecond
	}

	// Chatting timeout
	if e.IsChatting && now.Sub(e.ChatTimer) >= e.ChatTimerLength {
		e.IsChatting = false
		e.SetFaceAction("std")
		// Restore previous animation scale if it was overridden
		if e.previousAnimScale > 0 {
			e.WalkAnimScale = e.previousAnimScale
			e.previousAnimScale = 0
		}
		log.Printf("AvatarEngine: chat timeout, returning to standard face")
	}

	// Emote timeout
	if e.IsEmoting && now.Sub(e.EmoteTimer) >= e.EmoteTimerLength {
		e.IsEmoting = false
		e.SetFaceAction("std")
		log.Printf("AvatarEngine: emote timeout, returning to standard face")
	}

	// Carry timeout
	if e.IsCarrying && now.Sub(e.CarryTimer) >= e.CarryTimerLength {
		e.IsCarrying = false
		log.Printf("AvatarEngine: carry timeout")
	}

	// Drink timeout
	if e.IsDrinking && now.Sub(e.DrinkTimer) >= e.DrinkTimerLength {
		e.IsDrinking = false
		log.Printf("AvatarEngine: drink timeout")
	}

	// Sleep timeout
	if !e.IsSleeping && now.Sub(e.SleepTimer) >= e.SleepTimerLength {
		e.IsSleeping = true
		e.EyeAction = "eyb" // close eyes while sleeping
		log.Printf("AvatarEngine: entering sleep mode")
	}
}

// StartChatting starts the speaking animation with default duration (1 second)
func (e *AvatarEngine) StartChatting() {
	e.IsChatting = true
	e.ChatTimer = time.Now()
	e.ChatTimerLength = 1000 * time.Millisecond
	e.SetFaceAction("spk")
	e.wakeUp()
	log.Printf("AvatarEngine: started chatting")
}

// StartChattingWithDuration starts the speaking animation with custom duration and animation speed
func (e *AvatarEngine) StartChattingWithDuration(duration time.Duration, animSpeedScale float64) {
	e.IsChatting = true
	e.ChatTimer = time.Now()
	e.ChatTimerLength = duration
	e.SetFaceAction("spk")
	// Store previous animation scale and apply new one
	e.previousAnimScale = e.WalkAnimScale
	e.WalkAnimScale = animSpeedScale // Reuse walk scale for general animation speed
	e.wakeUp()
	log.Printf("AvatarEngine: started chatting with duration %v and speed scale %.2f", duration, animSpeedScale)
}

// StartEmote starts an emote animation
func (e *AvatarEngine) StartEmote(emote string) {
	validEmotes := map[string]bool{
		"sml": true, // Smile
		"agr": true, // Angry
		"srp": true, // Surprised
		"sad": true, // Sad
	}

	if !validEmotes[emote] {
		log.Printf("AvatarEngine: invalid emote: %s", emote)
		return
	}

	e.IsEmoting = true
	e.EmoteTimer = time.Now()
	e.SetFaceAction(emote)
	e.wakeUp()
	log.Printf("AvatarEngine: started emote %s", emote)
}

// StartWalking starts the walking animation
func (e *AvatarEngine) StartWalking() {
	e.SetBodyAction("wlk")
	e.ResetAnimation()
	e.wakeUp()
	log.Printf("AvatarEngine: started walking")
}

// StopWalking stops the walking animation
func (e *AvatarEngine) StopWalking() {
	e.SetBodyAction("std")
	e.ResetAnimation()
	e.wakeUp()
	log.Printf("AvatarEngine: stopped walking")
}

// StartCarrying starts carrying an item
func (e *AvatarEngine) StartCarrying() {
	e.IsCarrying = true
	e.CarryTimer = time.Now()
	e.wakeUp()
	log.Printf("AvatarEngine: started carrying")
}

// StopCarrying stops carrying an item
func (e *AvatarEngine) StopCarrying() {
	e.IsCarrying = false
	e.wakeUp()
	log.Printf("AvatarEngine: stopped carrying")
}

// StartDrinking starts the drinking animation
func (e *AvatarEngine) StartDrinking() {
	e.IsDrinking = true
	e.DrinkTimer = time.Now()
	e.DrinkTimerLength = e.DrinkWaitTimerLength
	e.wakeUp()
	log.Printf("AvatarEngine: started drinking")
}

// StopDrinking stops the drinking animation
func (e *AvatarEngine) StopDrinking() {
	e.IsDrinking = false
	e.wakeUp()
	log.Printf("AvatarEngine: stopped drinking")
}

// wakeUp clears sleeping state and resets its timer; also opens eyes
func (e *AvatarEngine) wakeUp() {
	if e.IsSleeping {
		e.IsSleeping = false
		e.ResetFace()
	}
	e.SleepTimer = time.Now()
}

// SetFloodFill sets whether to apply flood fill based on studio type
func (e *AvatarEngine) SetFloodFill(previewOnly bool) {
	// In Lingo, this is determined by whether the studio is private.
	// For now, we'll base it on the previewOnly flag.
	if previewOnly {
		e.DoFloodFill = false
	} else {
		// In a real implementation, we'd check `oStudioMap.isPrivate()`
		// and enable flood fill for public studios.
		e.DoFloodFill = true // Default to on for non-previews
	}
}

// updateDrawOrder sets the draw order based on current direction (from Lingo setDrawOrder)
func (e *AvatarEngine) updateDrawOrder() {
	switch e.Direction {
	case 1:
		e.DrawOrder = []string{"bd", "sh", "lg", "lh", "ls", "ch", "hd", "fc", "ey", "ri", "rh", "rs", "hr"}
	case 5:
		e.DrawOrder = []string{"bd", "sh", "lg", "lh", "ls", "ch", "hd", "fc", "ey", "ri", "rh", "rs", "hr"}
	case 4, 2:
		// Match Lingo: during dance and not walking, bring hands/shoes front
		if e.IsDancing && e.BodyAction != "wlk" {
			e.DrawOrder = []string{"lh", "ls", "bd", "sh", "lg", "ch", "ri", "rh", "rs", "hd", "fc", "ey", "hr", "ht"}
		} else {
			e.DrawOrder = []string{"bd", "sh", "lg", "lh", "ls", "ch", "hd", "fc", "ey", "ri", "rh", "rs", "hr"}
		}
	case 3:
		e.DrawOrder = []string{"bd", "sh", "lg", "ch", "lh", "ls", "hd", "fc", "ey", "ri", "rh", "rs", "hr"}
	case 7:
		if e.IsDancing && e.BodyAction != "wlk" {
			e.DrawOrder = []string{"lh", "ls", "bd", "sh", "lg", "ch", "ri", "rh", "rs", "hd", "fc", "ey", "hr", "ht"}
		} else {
			e.DrawOrder = []string{"lh", "ls", "bd", "sh", "lg", "ch", "ri", "rh", "rs", "hd", "hr", "ht"}
		}
	case 6, 0:
		if e.IsDancing && e.BodyAction != "wlk" {
			e.DrawOrder = []string{"lh", "ls", "bd", "sh", "lg", "ch", "ri", "rh", "rs", "hd", "fc", "ey", "hr", "ht"}
		} else {
			e.DrawOrder = []string{"lh", "ls", "bd", "sh", "lg", "ch", "ri", "rh", "rs", "hd", "hr", "ht"}
		}
	default:
		// Default draw order
		e.DrawOrder = []string{"lh", "ls", "bd", "sh", "lg", "ch", "ri", "rh", "rs", "hd", "fc", "ey", "hr", "ht"}
	}
}

// SetDance turns dance mode on/off (matches Lingo dance(bState))
func (e *AvatarEngine) SetDance(state bool) {
	e.IsDancing = state
	// Redetermine draw order as dance affects layering
	e.updateDrawOrder()
	// Reset animation step to start a clean cycle
	e.ResetAnimation()
}

// Walk sets the avatar's body action to walking
func (e *AvatarEngine) Walk() {
	if e.BodyAction != "wlk" {
		e.SetBodyAction("wlk")
		e.ResetAnimation()
		e.wakeUp()
		log.Printf("AvatarEngine: action set to walk")
	}
}

// Stand sets the avatar's body action to standing
func (e *AvatarEngine) Stand() {
	if e.BodyAction != "std" {
		e.SetBodyAction("std")
		e.ResetAnimation()
		e.wakeUp()
		log.Printf("AvatarEngine: action set to stand")
	}
}

// SetPosition sets the on-screen position of the avatar
func (e *AvatarEngine) SetPosition(pos core.Point2D) {
	e.screenPos = pos
}

// GetPosition returns the on-screen position of the avatar
func (e *AvatarEngine) GetPosition() core.Point2D {
	return e.screenPos
}

// GetBodyAction returns the current body action
func (e *AvatarEngine) GetBodyAction() string {
	return e.BodyAction
}

// SetHoldingItem sets the item the avatar is holding (e.g., "coke")
func (e *AvatarEngine) SetHoldingItem(item string) {
	if item == "" {
		// Clear holding state
		e.IsCarrying = false
		e.ItemAction = ""
		e.CarryTimer = time.Time{}
		e.IsDrinking = false
		// Remove "ri" from attributes when not carrying
		delete(e.Attributes, "ri")
	} else {
		// Set holding state
		e.IsCarrying = true
		e.CarryTimer = time.Now()

		// Handle different item types
		if item == "coke_hold" {
			// Just hold the coke, don't start drinking
			e.ItemAction = "coke"
			e.IsDrinking = false
			// Add "ri" (right item) with coke style 001
			if e.Attributes == nil {
				e.Attributes = make(map[string]BodyPartAppearance)
			}
			e.Attributes["ri"] = BodyPartAppearance{Style: "001", Color: nil}
		} else if item == "coke" {
			// Old behavior for compatibility - start drinking immediately
			e.ItemAction = item
			e.IsDrinking = true
			e.DrinkTimer = time.Now()
			// Add "ri" (right item) with coke style 001
			if e.Attributes == nil {
				e.Attributes = make(map[string]BodyPartAppearance)
			}
			e.Attributes["ri"] = BodyPartAppearance{Style: "001", Color: nil}
		} else {
			// Other items - use item name as style
			e.ItemAction = item
			e.IsDrinking = false
			// Add "ri" (right item) to attributes so it gets rendered
			if e.Attributes == nil {
				e.Attributes = make(map[string]BodyPartAppearance)
			}
			e.Attributes["ri"] = BodyPartAppearance{Style: item, Color: nil}
		}
	}
}
