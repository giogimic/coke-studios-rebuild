package sumo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net/http"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"gocoke/internal/client/fs"
	rulesumo "gocoke/internal/minigames/sumo"
)

type Move = rulesumo.Move
type RoundResult = rulesumo.RoundResult
type MatchState = rulesumo.MatchState

const (
	MoveNone         = rulesumo.MoveNone
	MoveHeadHit      = rulesumo.MoveHeadHit
	MoveBodyBlaster  = rulesumo.MoveBodyBlaster
	MoveOverheadToss = rulesumo.MoveOverheadToss
	MoveFlyingKick   = rulesumo.MoveFlyingKick
	MoveFlyingPunch  = rulesumo.MoveFlyingPunch

	ResultDraw        = rulesumo.ResultDraw
	ResultPlayer1Wins = rulesumo.ResultPlayer1Wins
	ResultPlayer2Wins = rulesumo.ResultPlayer2Wins
)

var (
	NewMatch           = rulesumo.NewMatch
	RandomMove         = rulesumo.RandomMove
	GetRewardForStreak = rulesumo.GetRewardForStreak
)

type SumoState int

const (
	StateChoosing SumoState = iota
	StateResolving
	StateGameOver
)

type MoveButton struct {
	Move  Move
	X     float64
	Y     float64
	W     float64
	H     float64
	Hover bool
}

type Scene struct {
	match           *MatchState
	localPlayer     string
	opponent        string
	state           SumoState
	timerSec        float64
	resolveTimerSec float64
	selectedMove    Move
	lastResult      RoundResult
	lastP1Move      Move
	lastP2Move      Move
	streak          int
	rewardNotice    string
	awardedDB       int
	awardedFurni    string

	// Assets
	dojoBg    *ebiten.Image
	moveIcons map[Move]*ebiten.Image

	// Buttons
	buttons [5]MoveButton

	// Callbacks
	onExit    func()
	serverURL string
}

func NewScene(localPlayer, opponent string, serverURL string, initialStreak int, onExit func()) (*Scene, error) {
	if opponent == "" {
		opponent = "Master_San"
	}
	match := NewMatch(localPlayer, opponent)

	sc := &Scene{
		match:        match,
		localPlayer:  localPlayer,
		opponent:     opponent,
		state:        StateChoosing,
		timerSec:     10.0,
		streak:       initialStreak,
		moveIcons:    make(map[Move]*ebiten.Image),
		onExit:       onExit,
		serverURL:    serverURL,
		selectedMove: MoveNone,
	}

	sc.loadAssets()
	sc.initButtons()

	return sc, nil
}

func (sc *Scene) loadAssets() {
	loadImage := func(p string) *ebiten.Image {
		f, err := fs.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			return nil
		}
		return ebiten.NewImageFromImage(img)
	}

	sc.dojoBg = loadImage("assets/sumo/dojo/dojo_bg.png")
	if sc.dojoBg == nil {
		sc.dojoBg = loadImage("assets/sumo/dojo/dojo_bg.jpg")
	}

	// Load move icons if present
	sc.moveIcons[MoveHeadHit] = loadImage("assets/sumo/moveselection/img_6.png")
	sc.moveIcons[MoveBodyBlaster] = loadImage("assets/sumo/moveselection/img_12.png")
	sc.moveIcons[MoveOverheadToss] = loadImage("assets/sumo/moveselection/img_16.png")
	sc.moveIcons[MoveFlyingKick] = loadImage("assets/sumo/moveselection/img_20.png")
}

func (sc *Scene) initButtons() {
	startX := 105.0
	y := 445.0
	w := 100.0
	h := 50.0
	spacing := 10.0

	moves := []Move{
		MoveHeadHit,
		MoveBodyBlaster,
		MoveOverheadToss,
		MoveFlyingKick,
		MoveFlyingPunch,
	}

	for i, m := range moves {
		x := startX + float64(i)*(w+spacing)
		sc.buttons[i] = MoveButton{
			Move: m,
			X:    x,
			Y:    y,
			W:    w,
			H:    h,
		}
	}
}

func (sc *Scene) Update() error {
	dt := 1.0 / 60.0

	// Game over interaction
	if sc.state == StateGameOver {
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			mx, my := ebiten.CursorPosition()
			// Check Exit button (330..430, 360..390)
			if mx >= 330 && mx <= 430 && my >= 360 && my <= 390 {
				if sc.onExit != nil {
					sc.onExit()
				}
			}
			// Check Play Again button (330..430, 320..350)
			if mx >= 330 && mx <= 430 && my >= 320 && my <= 350 {
				sc.resetForNewMatch()
			}
		}
		return nil
	}

	// Resolving round animation
	if sc.state == StateResolving {
		sc.resolveTimerSec -= dt
		if sc.resolveTimerSec <= 0 {
			sc.finishRoundResolution()
		}
		return nil
	}

	// Choosing state: update hover and timer
	mx, my := ebiten.CursorPosition()
	for i := range sc.buttons {
		btn := &sc.buttons[i]
		btn.Hover = float64(mx) >= btn.X && float64(mx) <= btn.X+btn.W &&
			float64(my) >= btn.Y && float64(my) <= btn.Y+btn.H
	}

	sc.timerSec -= dt
	if sc.timerSec <= 0 {
		// Timeout -> fallback random move
		sc.selectedMove = RandomMove()
		sc.executeRound()
		return nil
	}

	// Player button click
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		for _, btn := range sc.buttons {
			if btn.Hover {
				sc.selectedMove = btn.Move
				sc.executeRound()
				break
			}
		}
	}

	return nil
}

func (sc *Scene) executeRound() {
	p1Move := sc.selectedMove
	if p1Move == MoveNone {
		p1Move = RandomMove()
	}
	p2Move := RandomMove()

	res, actualP1, actualP2 := sc.match.SubmitMoves(p1Move, p2Move)
	sc.lastResult = res
	sc.lastP1Move = actualP1
	sc.lastP2Move = actualP2

	sc.state = StateResolving
	sc.resolveTimerSec = 2.0

	// Sync with server if configured
	sc.syncServerRound(actualP1, actualP2)
}

func (sc *Scene) finishRoundResolution() {
	if sc.match.GameOver {
		sc.state = StateGameOver
		if sc.match.WinnerName == sc.localPlayer {
			sc.streak++
			tier := GetRewardForStreak(sc.streak)
			if tier != nil {
				sc.awardedDB = tier.RewardDB
				sc.awardedFurni = tier.RewardName
				sc.rewardNotice = fmt.Sprintf("Streak %d Bonus: +%d dB! Prize: %s", sc.streak, tier.RewardDB, tier.RewardName)
				sc.syncServerPrize(sc.streak)
			} else {
				sc.rewardNotice = fmt.Sprintf("Streak %d: Victory!", sc.streak)
			}
		} else {
			sc.streak = 0
			sc.rewardNotice = "Defeated! Win streak reset to 0."
		}
	} else {
		sc.state = StateChoosing
		sc.timerSec = 10.0
		sc.selectedMove = MoveNone
	}
}

func (sc *Scene) resetForNewMatch() {
	sc.match = NewMatch(sc.localPlayer, sc.opponent)
	sc.state = StateChoosing
	sc.timerSec = 10.0
	sc.selectedMove = MoveNone
	sc.lastResult = ResultDraw
	sc.lastP1Move = MoveNone
	sc.lastP2Move = MoveNone
}

func (sc *Scene) syncServerRound(m1, m2 Move) {
	if sc.serverURL == "" {
		return
	}
	go func() {
		payload := map[string]interface{}{
			"player1": sc.localPlayer,
			"player2": sc.opponent,
			"p1_move": int(m1),
			"p2_move": int(m2),
		}
		b, _ := json.Marshal(payload)
		_, _ = http.Post(sc.serverURL+"/api/minigames/sumo/match", "application/json", bytes.NewReader(b))
	}()
}

func (sc *Scene) syncServerPrize(streak int) {
	if sc.serverURL == "" {
		return
	}
	go func() {
		url := fmt.Sprintf("%s/sumo.do?screenname=%s&prize=%d", sc.serverURL, sc.localPlayer, streak)
		_, _ = http.Get(url)
	}()
}

func (sc *Scene) Draw(screen *ebiten.Image) {
	// 1. Draw Dojo Arena Background (760x520)
	if sc.dojoBg != nil {
		op := &ebiten.DrawImageOptions{}
		bw := float64(sc.dojoBg.Bounds().Dx())
		bh := float64(sc.dojoBg.Bounds().Dy())
		op.GeoM.Scale(760.0/bw, 520.0/bh)
		screen.DrawImage(sc.dojoBg, op)
	} else {
		screen.Fill(color.RGBA{40, 20, 15, 255})
	}

	// 2. Draw Top HUD
	sc.drawTopHUD(screen)

	// 3. Draw Dohyo Ring & V-Ego Avatars
	sc.drawFighters(screen)

	// 4. Draw Move Action Buttons
	if sc.state == StateChoosing {
		sc.drawMoveButtons(screen)
	} else if sc.state == StateResolving {
		sc.drawResolutionBanner(screen)
	}

	// 5. Draw Game Over Overlay
	if sc.state == StateGameOver {
		sc.drawGameOver(screen)
	}
}

func (sc *Scene) drawTopHUD(screen *ebiten.Image) {
	bar := ebiten.NewImage(760, 36)
	bar.Fill(color.RGBA{0, 0, 0, 190})
	screen.DrawImage(bar, nil)

	drawText(screen, "V-EGO SAN SUMO - BEST OF 3", 15, 10, color.RGBA{255, 200, 50, 255})

	scoreStr := fmt.Sprintf("ROUND %d  |  %s: %d  vs  %s: %d",
		sc.match.CurrentRound, sc.localPlayer, sc.match.Player1Score, sc.opponent, sc.match.Player2Score)
	drawText(screen, scoreStr, 280, 10, color.White)

	streakStr := fmt.Sprintf("Streak: %d", sc.streak)
	drawText(screen, streakStr, 660, 10, color.RGBA{100, 255, 100, 255})
}

func (sc *Scene) drawFighters(screen *ebiten.Image) {
	// Left fighter (P1)
	p1X := 310.0
	p1Y := 280.0

	// Right fighter (P2)
	p2X := 450.0
	p2Y := 280.0

	// Resolution lunge/push offsets
	if sc.state == StateResolving {
		progress := sc.resolveTimerSec / 2.0 // 1.0 down to 0.0
		offset := math.Sin((1.0-progress)*math.Pi) * 40.0
		switch sc.lastResult {
		case ResultPlayer1Wins:
			p1X += offset
			p2X += offset * 0.8
		case ResultPlayer2Wins:
			p2X -= offset
			p1X -= offset * 0.8
		case ResultDraw:
			p1X += math.Sin((1.0-progress)*math.Pi*2) * 10.0
			p2X -= math.Sin((1.0-progress)*math.Pi*2) * 10.0
		}
	}

	// Draw P1 V-Ego avatar placeholder / figure
	drawSumoAvatar(screen, p1X, p1Y, sc.localPlayer, color.RGBA{220, 50, 50, 255}, true)

	// Draw P2 V-Ego avatar placeholder / figure
	drawSumoAvatar(screen, p2X, p2Y, sc.opponent, color.RGBA{50, 100, 220, 255}, false)
}

func drawSumoAvatar(screen *ebiten.Image, x, y float64, name string, beltColor color.Color, isP1 bool) {
	// Body
	body := ebiten.NewImage(36, 50)
	body.Fill(color.RGBA{240, 200, 160, 255}) // skin tone
	bop := &ebiten.DrawImageOptions{}
	bop.GeoM.Translate(x-18, y-25)
	screen.DrawImage(body, bop)

	// Mawashi belt
	belt := ebiten.NewImage(38, 14)
	belt.Fill(beltColor)
	mop := &ebiten.DrawImageOptions{}
	mop.GeoM.Translate(x-19, y-2)
	screen.DrawImage(belt, mop)

	// Head & Hair bun
	head := ebiten.NewImage(26, 26)
	head.Fill(color.RGBA{240, 200, 160, 255})
	hop := &ebiten.DrawImageOptions{}
	hop.GeoM.Translate(x-13, y-48)
	screen.DrawImage(head, hop)

	bun := ebiten.NewImage(12, 10)
	bun.Fill(color.RGBA{20, 20, 20, 255})
	bunOp := &ebiten.DrawImageOptions{}
	bunOp.GeoM.Translate(x-6, y-55)
	screen.DrawImage(bun, bunOp)

	// Name tag above head
	drawText(screen, name, int(x)-len(name)*3, int(y)-68, color.White)
}

func (sc *Scene) drawMoveButtons(screen *ebiten.Image) {
	// Panel header
	drawText(screen, fmt.Sprintf("CHOOSE YOUR MOVE! (Time: %.0fs)", sc.timerSec), 270, 420, color.RGBA{255, 230, 80, 255})

	for _, btn := range sc.buttons {
		box := ebiten.NewImage(int(btn.W), int(btn.H))
		if btn.Hover {
			box.Fill(color.RGBA{230, 60, 60, 255})
		} else {
			box.Fill(color.RGBA{180, 40, 40, 230})
		}

		bop := &ebiten.DrawImageOptions{}
		bop.GeoM.Translate(btn.X, btn.Y)
		screen.DrawImage(box, bop)

		// Border
		border := ebiten.NewImage(int(btn.W), 2)
		border.Fill(color.RGBA{255, 220, 100, 255})
		screen.DrawImage(border, bop)

		// Draw move icon if available
		icon, ok := sc.moveIcons[btn.Move]
		textX := int(btn.X) + 8
		if ok && icon != nil {
			iop := &ebiten.DrawImageOptions{}
			iop.GeoM.Translate(btn.X+6, btn.Y+10)
			screen.DrawImage(icon, iop)
			textX += 34
		}

		// Text
		moveName := btn.Move.String()
		drawText(screen, moveName, textX, int(btn.Y)+18, color.White)
	}
}

func (sc *Scene) drawResolutionBanner(screen *ebiten.Image) {
	banner := ebiten.NewImage(500, 70)
	banner.Fill(color.RGBA{0, 0, 0, 220})
	bop := &ebiten.DrawImageOptions{}
	bop.GeoM.Translate(130, 420)
	screen.DrawImage(banner, bop)

	p1MoveStr := sc.lastP1Move.String()
	p2MoveStr := sc.lastP2Move.String()
	clashStr := fmt.Sprintf("%s (%s) vs %s (%s)", sc.localPlayer, p1MoveStr, sc.opponent, p2MoveStr)
	drawText(screen, clashStr, 150, 435, color.RGBA{255, 200, 100, 255})

	var resultStr string
	var clr color.Color
	switch sc.lastResult {
	case ResultPlayer1Wins:
		resultStr = fmt.Sprintf("ROUND WON! %s counters %s!", p1MoveStr, p2MoveStr)
		clr = color.RGBA{100, 255, 100, 255}
	case ResultPlayer2Wins:
		resultStr = fmt.Sprintf("ROUND LOST! %s counters %s!", p2MoveStr, p1MoveStr)
		clr = color.RGBA{255, 100, 100, 255}
	case ResultDraw:
		resultStr = "DRAW! Both fighters clashed equally!"
		clr = color.RGBA{255, 255, 100, 255}
	}
	drawText(screen, resultStr, 150, 455, clr)
}

func (sc *Scene) drawGameOver(screen *ebiten.Image) {
	overlay := ebiten.NewImage(760, 520)
	overlay.Fill(color.RGBA{0, 0, 0, 210})
	screen.DrawImage(overlay, nil)

	modal := ebiten.NewImage(380, 240)
	modal.Fill(color.RGBA{30, 25, 35, 255})
	mop := &ebiten.DrawImageOptions{}
	mop.GeoM.Translate(190, 140)
	screen.DrawImage(modal, mop)

	isWinner := sc.match.WinnerName == sc.localPlayer
	if isWinner {
		drawText(screen, "V-EGO SAN SUMO - MATCH VICTORY!", 220, 160, color.RGBA{255, 215, 0, 255})
		scoreLine := fmt.Sprintf("Final Score: %d - %d", sc.match.Player1Score, sc.match.Player2Score)
		drawText(screen, scoreLine, 220, 190, color.White)
		drawText(screen, sc.rewardNotice, 210, 220, color.RGBA{100, 255, 100, 255})
		if sc.awardedFurni != "" {
			drawText(screen, fmt.Sprintf("Furni Prize: %s placed in backpack!", sc.awardedFurni), 205, 245, color.RGBA{255, 220, 100, 255})
		}
	} else {
		drawText(screen, "V-EGO SAN SUMO - DEFEAT", 250, 160, color.RGBA{255, 100, 100, 255})
		scoreLine := fmt.Sprintf("Final Score: %d - %d", sc.match.Player1Score, sc.match.Player2Score)
		drawText(screen, scoreLine, 220, 190, color.White)
		drawText(screen, "Better luck next time!", 250, 220, color.RGBA{200, 200, 200, 255})
	}

	// Play Again button
	btnPlay := ebiten.NewImage(100, 26)
	btnPlay.Fill(color.RGBA{40, 150, 40, 255})
	bpop := &ebiten.DrawImageOptions{}
	bpop.GeoM.Translate(330, 320)
	screen.DrawImage(btnPlay, bpop)
	drawText(screen, "PLAY AGAIN", 340, 327, color.White)

	// Exit button
	btnExit := ebiten.NewImage(100, 26)
	btnExit.Fill(color.RGBA{180, 40, 40, 255})
	beop := &ebiten.DrawImageOptions{}
	beop.GeoM.Translate(330, 360)
	screen.DrawImage(btnExit, beop)
	drawText(screen, "EXIT", 365, 367, color.White)
}

func (sc *Scene) Layout(ow, oh int) (int, int) {
	return 760, 520
}

// Simple fallback string rasterizer using monospace characters
func drawText(dst *ebiten.Image, str string, x, y int, clr color.Color) {
	dot := ebiten.NewImage(1, 1)
	dot.Fill(clr)

	cx := float64(x)
	cy := float64(y)

	for _, ch := range str {
		if ch == ' ' {
			cx += 7.0
			continue
		}
		if ch == '\n' {
			cx = float64(x)
			cy += 14.0
			continue
		}
		drawChar(dst, dot, ch, cx, cy)
		cx += 7.0
	}
}

func drawChar(dst, dot *ebiten.Image, ch rune, x, y float64) {
	pattern, ok := font5x7[ch]
	if !ok {
		pattern = font5x7['?']
	}
	for row := 0; row < 7; row++ {
		b := pattern[row]
		for col := 0; col < 5; col++ {
			if (b & (1 << (4 - col))) != 0 {
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(x+float64(col), y+float64(row))
				dst.DrawImage(dot, op)
			}
		}
	}
}

var font5x7 = map[rune][7]byte{
	'A': {0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'B': {0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E},
	'C': {0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E},
	'D': {0x1C, 0x12, 0x11, 0x11, 0x11, 0x12, 0x1C},
	'E': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F},
	'F': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10},
	'G': {0x0E, 0x11, 0x10, 0x17, 0x11, 0x11, 0x0F},
	'H': {0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'I': {0x0E, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'J': {0x07, 0x02, 0x02, 0x02, 0x02, 0x12, 0x0C},
	'K': {0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11},
	'L': {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F},
	'M': {0x11, 0x1B, 0x15, 0x11, 0x11, 0x11, 0x11},
	'N': {0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11},
	'O': {0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'P': {0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10},
	'Q': {0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D},
	'R': {0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11},
	'S': {0x0E, 0x11, 0x10, 0x0E, 0x01, 0x11, 0x0E},
	'T': {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'U': {0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'V': {0x11, 0x11, 0x11, 0x11, 0x11, 0x0A, 0x04},
	'W': {0x11, 0x11, 0x11, 0x15, 0x15, 0x1B, 0x11},
	'X': {0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11},
	'Y': {0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04},
	'Z': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F},
	'a': {0x00, 0x00, 0x0E, 0x01, 0x0F, 0x11, 0x0F},
	'b': {0x10, 0x10, 0x16, 0x19, 0x11, 0x11, 0x1E},
	'c': {0x00, 0x00, 0x0E, 0x10, 0x10, 0x11, 0x0E},
	'd': {0x01, 0x01, 0x0D, 0x13, 0x11, 0x11, 0x0F},
	'e': {0x00, 0x00, 0x0E, 0x11, 0x1F, 0x10, 0x0E},
	'f': {0x06, 0x09, 0x08, 0x1C, 0x08, 0x08, 0x08},
	'g': {0x00, 0x00, 0x0F, 0x11, 0x0F, 0x01, 0x0E},
	'h': {0x10, 0x10, 0x16, 0x19, 0x11, 0x11, 0x11},
	'i': {0x04, 0x00, 0x0C, 0x04, 0x04, 0x04, 0x0E},
	'j': {0x02, 0x00, 0x06, 0x02, 0x02, 0x12, 0x0C},
	'k': {0x10, 0x10, 0x12, 0x14, 0x18, 0x14, 0x12},
	'l': {0x0C, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'm': {0x00, 0x00, 0x1A, 0x15, 0x15, 0x11, 0x11},
	'n': {0x00, 0x00, 0x16, 0x19, 0x11, 0x11, 0x11},
	'o': {0x00, 0x00, 0x0E, 0x11, 0x11, 0x11, 0x0E},
	'p': {0x00, 0x00, 0x1E, 0x11, 0x1E, 0x10, 0x10},
	'q': {0x00, 0x00, 0x0D, 0x13, 0x0F, 0x01, 0x01},
	'r': {0x00, 0x00, 0x16, 0x19, 0x10, 0x10, 0x10},
	's': {0x00, 0x00, 0x0E, 0x10, 0x0E, 0x01, 0x1E},
	't': {0x08, 0x08, 0x1C, 0x08, 0x08, 0x09, 0x06},
	'u': {0x00, 0x00, 0x11, 0x11, 0x11, 0x13, 0x0D},
	'v': {0x00, 0x00, 0x11, 0x11, 0x11, 0x0A, 0x04},
	'w': {0x00, 0x00, 0x11, 0x11, 0x15, 0x15, 0x0A},
	'x': {0x00, 0x00, 0x11, 0x0A, 0x04, 0x0A, 0x11},
	'y': {0x00, 0x00, 0x11, 0x11, 0x0F, 0x01, 0x0E},
	'z': {0x00, 0x00, 0x1F, 0x02, 0x04, 0x08, 0x1F},
	'0': {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E},
	'1': {0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'2': {0x0E, 0x11, 0x01, 0x06, 0x08, 0x10, 0x1F},
	'3': {0x1F, 0x02, 0x04, 0x02, 0x01, 0x11, 0x0E},
	'4': {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02},
	'5': {0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E},
	'6': {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E},
	'7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
	'9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},
	':': {0x00, 0x0C, 0x0C, 0x00, 0x0C, 0x0C, 0x00},
	'!': {0x04, 0x04, 0x04, 0x04, 0x04, 0x00, 0x04},
	'?': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x00, 0x04},
	'-': {0x00, 0x00, 0x00, 0x1F, 0x00, 0x00, 0x00},
	'+': {0x00, 0x04, 0x04, 0x1F, 0x04, 0x04, 0x00},
	'#': {0x0A, 0x0A, 0x1F, 0x0A, 0x1F, 0x0A, 0x0A},
	'(': {0x02, 0x04, 0x08, 0x08, 0x08, 0x04, 0x02},
	')': {0x08, 0x04, 0x02, 0x02, 0x02, 0x04, 0x08},
	'=': {0x00, 0x1F, 0x00, 0x1F, 0x00, 0x00, 0x00},
	'>': {0x10, 0x08, 0x04, 0x02, 0x04, 0x08, 0x10},
	'<': {0x01, 0x02, 0x04, 0x08, 0x04, 0x02, 0x01},
	'|': {0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'.': {0x00, 0x00, 0x00, 0x00, 0x00, 0x0C, 0x0C},
	',': {0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x08},
	'_': {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1F},
}

