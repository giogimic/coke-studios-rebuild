package uncover

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"math/rand"
	"net/http"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"gocoke/internal/client/fs"
)

type TurnState int

const (
	StateWaitingFirst TurnState = iota
	StateWaitingSecond
	StateEvaluating
	StateGameOver
)

type VisualTile struct {
	Index        int
	Row          int
	Col          int
	X            float64
	Y            float64
	IsRevealed   bool
	IsMatched    bool
	FallProgress float64
	Hover        bool
}

type Scene struct {
	game         *UncoverGame
	localPlayer  string
	players      []string
	turnState    TurnState
	timerSec     float64
	evalTimerSec float64
	firstTileIdx int
	secTileIdx   int
	lastResult   *TurnResult
	aiMoveTimer  float64

	// Assets
	bgImg       *ebiten.Image
	coveredImg  *ebiten.Image
	musicImg    *ebiten.Image
	frogImg     *ebiten.Image
	snakeImg    *ebiten.Image
	scorepadImg *ebiten.Image

	// Audio
	audioCtx     *audio.Context
	soundBuffers map[string][]byte

	// Geometry
	visualTiles [24]VisualTile
	tileWidth   float64
	tileHeight  float64

	// Callbacks
	onExit func()

	// Server sync
	serverURL     string
	dailyDecibels int
	awardedDB     int
	statusMsg     string
}

func NewScene(localPlayer string, extraPlayers []string, serverURL string, onExit func()) (*Scene, error) {
	allPlayers := []string{localPlayer}
	if len(extraPlayers) > 0 {
		allPlayers = append(allPlayers, extraPlayers...)
	}
	for len(allPlayers) < 3 {
		allPlayers = append(allPlayers, fmt.Sprintf("V-Ego_%d", len(allPlayers)+1))
	}
	if len(allPlayers) > 5 {
		allPlayers = allPlayers[:5]
	}

	game, err := NewGame(allPlayers)
	if err != nil {
		return nil, err
	}

	sc := &Scene{
		game:         game,
		localPlayer:  localPlayer,
		players:      allPlayers,
		turnState:    StateWaitingFirst,
		timerSec:     15.0,
		firstTileIdx: -1,
		secTileIdx:   -1,
		soundBuffers: make(map[string][]byte),
		tileWidth:    82.0,
		tileHeight:   41.0,
		onExit:       onExit,
		serverURL:    serverURL,
	}

	sc.initAudio()
	sc.loadAssets()
	sc.initGrid()

	return sc, nil
}

func (sc *Scene) initAudio() {
	sc.audioCtx = audio.CurrentContext()
	if sc.audioCtx == nil {
		sc.audioCtx = audio.NewContext(44100)
	}
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

	sc.bgImg = loadImage("assets/uncover/background-grid.jpg")
	if sc.bgImg == nil {
		sc.bgImg = loadImage("assets/uncover/background-utm.jpg")
	}
	sc.coveredImg = loadImage("assets/uncover/tiles/covered.png")
	sc.musicImg = loadImage("assets/uncover/tiles/music.png")
	sc.frogImg = loadImage("assets/uncover/sheets/frogged.png")
	sc.snakeImg = loadImage("assets/uncover/sheets/snaked.png")
	sc.scorepadImg = loadImage("assets/uncover/scorepad.png")
}

func (sc *Scene) initGrid() {
	space := 4.0
	originX := 380.0
	originY := 130.0

	for r := 0; r < 6; r++ {
		for c := 0; c < 4; c++ {
			i := r*4 + c
			x := originX + float64(c-r)*(sc.tileWidth+space)/2.0
			y := originY + float64(c+r)*(sc.tileHeight+space)/2.0

			sc.visualTiles[i] = VisualTile{
				Index: i,
				Row:   r,
				Col:   c,
				X:     x,
				Y:     y,
			}
		}
	}
}

func (sc *Scene) playSoundForTile(tile Tile) {
	if sc.audioCtx == nil {
		return
	}
	var soundPath string
	switch tile.Type {
	case CardPair:
		soundPath = fmt.Sprintf("assets/uncover/sounds/%02d.mp3", tile.PairID)
	case CardWild:
		soundPath = "assets/uncover/sounds/frogged.mp3"
	case CardPass:
		soundPath = "assets/uncover/sounds/snaked.mp3"
	}

	data, ok := sc.soundBuffers[soundPath]
	if !ok {
		f, err := fs.Open(soundPath)
		if err == nil {
			b, errRead := io.ReadAll(f)
			_ = f.Close()
			if errRead == nil {
				data = b
				sc.soundBuffers[soundPath] = data
			}
		}
	}
	if len(data) > 0 {
		stream, err := mp3.DecodeWithoutResampling(bytes.NewReader(data))
		if err == nil {
			p, errP := sc.audioCtx.NewPlayer(stream)
			if errP == nil {
				p.Play()
			}
		}
	}
}

func (sc *Scene) isLocalTurn() bool {
	if sc.game == nil || sc.game.GameOver {
		return false
	}
	return sc.game.Players[sc.game.CurrentTurnIdx] == sc.localPlayer
}

func (sc *Scene) Update() error {
	dt := 1.0 / 60.0

	// Handle game over
	if sc.turnState == StateGameOver {
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			mx, my := ebiten.CursorPosition()
			// Check Exit or Play Again
			if mx >= 330 && mx <= 430 && my >= 440 && my <= 470 {
				if sc.onExit != nil {
					sc.onExit()
				}
			}
		}
		return nil
	}

	// Update tile fall animations
	for i := range sc.visualTiles {
		vt := &sc.visualTiles[i]
		if vt.IsMatched && vt.FallProgress < 1.0 {
			vt.FallProgress += dt * 1.5
			if vt.FallProgress > 1.0 {
				vt.FallProgress = 1.0
			}
		}
	}

	// Check mouse hover over tiles
	mx, my := ebiten.CursorPosition()
	for i := range sc.visualTiles {
		vt := &sc.visualTiles[i]
		if vt.IsMatched {
			vt.Hover = false
			continue
		}
		dx := math.Abs(float64(mx) - vt.X)
		dy := math.Abs(float64(my) - vt.Y)
		vt.Hover = (dx/(sc.tileWidth/2.0) + dy/(sc.tileHeight/2.0)) <= 1.0
	}

	// Turn countdown timer
	if sc.turnState == StateWaitingFirst || sc.turnState == StateWaitingSecond {
		sc.timerSec -= dt
		if sc.timerSec <= 0 {
			// Forfeit turn
			sc.advanceTurnOnTimeout()
			return nil
		}
	}

	// Evaluation pause timer
	if sc.turnState == StateEvaluating {
		sc.evalTimerSec -= dt
		if sc.evalTimerSec <= 0 {
			sc.resolveEvaluation()
		}
		return nil
	}

	// AI turn automation if not local player's turn
	if !sc.isLocalTurn() {
		sc.aiMoveTimer += dt
		if sc.aiMoveTimer >= 1.2 {
			sc.aiMoveTimer = 0
			sc.performAIMove()
		}
		return nil
	}

	// Local player click
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		for i := range sc.visualTiles {
			vt := &sc.visualTiles[i]
			if vt.Hover && !vt.IsMatched && !vt.IsRevealed {
				sc.handleTileClick(i)
				break
			}
		}
	}

	return nil
}

func (sc *Scene) handleTileClick(idx int) {
	if sc.turnState == StateWaitingFirst {
		sc.firstTileIdx = idx
		sc.visualTiles[idx].IsRevealed = true
		sc.playSoundForTile(sc.game.Tiles[idx])

		// If Pass Card clicked first, ends turn immediately
		if sc.game.Tiles[idx].Type == CardPass {
			sc.turnState = StateEvaluating
			sc.evalTimerSec = 1.5
			return
		}

		sc.turnState = StateWaitingSecond
	} else if sc.turnState == StateWaitingSecond {
		if idx == sc.firstTileIdx {
			return
		}
		sc.secTileIdx = idx
		sc.visualTiles[idx].IsRevealed = true
		sc.playSoundForTile(sc.game.Tiles[idx])

		sc.turnState = StateEvaluating
		sc.evalTimerSec = 1.5
	}
}

func (sc *Scene) performAIMove() {
	var available []int
	for i, t := range sc.game.Tiles {
		if !t.Matched && i != sc.firstTileIdx {
			available = append(available, i)
		}
	}
	if len(available) == 0 {
		return
	}
	choice := available[rand.Intn(len(available))]
	sc.handleTileClick(choice)
}

func (sc *Scene) resolveEvaluation() {
	activePlayer := sc.game.Players[sc.game.CurrentTurnIdx]

	if sc.firstTileIdx >= 0 && sc.secTileIdx < 0 {
		// Pass card clicked on first tile
		t := &sc.game.Tiles[sc.firstTileIdx]
		if t.Type == CardPass {
			t.Matched = true
			sc.visualTiles[sc.firstTileIdx].IsMatched = true
			sc.statusMsg = fmt.Sprintf("%s uncovered the Snake! Turn passed.", activePlayer)
		}
		sc.visualTiles[sc.firstTileIdx].IsRevealed = false
		sc.game.advanceTurn()
	} else if sc.firstTileIdx >= 0 && sc.secTileIdx >= 0 {
		res, err := sc.game.PlayTurn(activePlayer, sc.firstTileIdx, sc.secTileIdx)
		if err == nil && res != nil {
			sc.lastResult = res
			if res.Matched {
				sc.visualTiles[sc.firstTileIdx].IsMatched = true
				sc.visualTiles[sc.secTileIdx].IsMatched = true
				sc.statusMsg = fmt.Sprintf("Match found by %s! (+1 dB)", activePlayer)
				sc.syncServerMatch(activePlayer, sc.firstTileIdx, sc.secTileIdx, res.GameOver)
			} else if res.Passed {
				sc.statusMsg = fmt.Sprintf("%s uncovered the Pass Card! Turn ended.", activePlayer)
			} else {
				sc.statusMsg = "No match. Better luck next time!"
			}
		}
		sc.visualTiles[sc.firstTileIdx].IsRevealed = false
		sc.visualTiles[sc.secTileIdx].IsRevealed = false
	}

	sc.firstTileIdx = -1
	sc.secTileIdx = -1
	sc.timerSec = 15.0

	if sc.game.GameOver {
		sc.turnState = StateGameOver
		sc.statusMsg = "All pairs uncovered! Game Over."
	} else {
		sc.turnState = StateWaitingFirst
	}
}

func (sc *Scene) advanceTurnOnTimeout() {
	if sc.firstTileIdx >= 0 {
		sc.visualTiles[sc.firstTileIdx].IsRevealed = false
	}
	if sc.secTileIdx >= 0 {
		sc.visualTiles[sc.secTileIdx].IsRevealed = false
	}
	sc.firstTileIdx = -1
	sc.secTileIdx = -1
	sc.statusMsg = fmt.Sprintf("%s timed out! Turn passed.", sc.game.Players[sc.game.CurrentTurnIdx])
	sc.game.advanceTurn()
	sc.timerSec = 15.0
	sc.turnState = StateWaitingFirst
}

func (sc *Scene) syncServerMatch(player string, t1, t2 int, gameOver bool) {
	if sc.serverURL == "" {
		return
	}
	go func() {
		payload := map[string]interface{}{
			"player":    player,
			"tile1":     t1,
			"tile2":     t2,
			"game_over": gameOver,
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return
		}
		resp, err := http.Post(sc.serverURL+"/api/minigames/uncover/play", "application/json", bytes.NewReader(b))
		if err != nil || resp == nil {
			return
		}
		defer resp.Body.Close()
		var res map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&res)
		if aw, ok := res["awarded_db"].(float64); ok {
			sc.awardedDB += int(aw)
		}
	}()
}

func (sc *Scene) Draw(screen *ebiten.Image) {
	// 1. Draw Background
	if sc.bgImg != nil {
		op := &ebiten.DrawImageOptions{}
		bw := float64(sc.bgImg.Bounds().Dx())
		bh := float64(sc.bgImg.Bounds().Dy())
		op.GeoM.Scale(760.0/bw, 520.0/bh)
		screen.DrawImage(sc.bgImg, op)
	} else {
		screen.Fill(color.RGBA{15, 25, 35, 255})
	}

	// 2. Draw Top HUD Banner
	sc.drawTopHUD(screen)

	// 3. Draw 24 Tiles with isometric depth
	for i := range sc.visualTiles {
		vt := &sc.visualTiles[i]
		if vt.FallProgress >= 1.0 {
			continue // Fully removed
		}
		sc.drawTile(screen, vt)
	}

	// 4. Draw Scoreboard
	sc.drawScoreboard(screen)

	// 5. Draw Game Over Overlay
	if sc.turnState == StateGameOver {
		sc.drawGameOver(screen)
	}
}

func (sc *Scene) drawTopHUD(screen *ebiten.Image) {
	// Status text & timer
	activePlayer := sc.game.Players[sc.game.CurrentTurnIdx]
	var bannerText string
	if sc.isLocalTurn() {
		bannerText = fmt.Sprintf("YOUR TURN! (Time: %.0fs)", sc.timerSec)
	} else {
		bannerText = fmt.Sprintf("%s's Turn (Time: %.0fs)", activePlayer, sc.timerSec)
	}

	// Simple styled bar
	bar := ebiten.NewImage(760, 36)
	bar.Fill(color.RGBA{0, 0, 0, 180})
	screen.DrawImage(bar, nil)

	// Draw text
	drawText(screen, "UNCOVER THE MUSIC", 15, 10, color.RGBA{255, 200, 50, 255})
	drawText(screen, bannerText, 320, 10, color.White)
	if sc.statusMsg != "" {
		drawText(screen, sc.statusMsg, 15, 40, color.RGBA{150, 220, 255, 255})
	}
}

func (sc *Scene) drawTile(screen *ebiten.Image, vt *VisualTile) {
	t := sc.game.Tiles[vt.Index]

	op := &ebiten.DrawImageOptions{}
	hw := sc.tileWidth / 2.0
	hh := sc.tileHeight / 2.0

	// Centered origin
	op.GeoM.Translate(-hw, -hh)

	// Hover bobbing
	yOff := 0.0
	if vt.Hover && sc.isLocalTurn() && !vt.IsRevealed {
		yOff = -3.0
	}
	// Fall animation
	if vt.FallProgress > 0 {
		yOff += vt.FallProgress * 300.0
		op.ColorScale.ScaleAlpha(float32(1.0 - vt.FallProgress))
	}

	op.GeoM.Translate(vt.X, vt.Y+yOff)

	if vt.IsRevealed || vt.IsMatched {
		// Draw uncovered / music card
		if sc.musicImg != nil {
			screen.DrawImage(sc.musicImg, op)
		} else {
			placeholder := ebiten.NewImage(int(sc.tileWidth), int(sc.tileHeight))
			placeholder.Fill(color.RGBA{220, 180, 50, 255})
			screen.DrawImage(placeholder, op)
		}

		// Draw card badge / face symbol
		sc.drawTileContent(screen, t, vt.X, vt.Y+yOff)
	} else {
		// Draw covered card
		if sc.coveredImg != nil {
			if vt.Hover && sc.isLocalTurn() {
				op.ColorScale.Scale(1.15, 1.15, 1.3, 1.0)
			}
			screen.DrawImage(sc.coveredImg, op)
		} else {
			placeholder := ebiten.NewImage(int(sc.tileWidth), int(sc.tileHeight))
			placeholder.Fill(color.RGBA{50, 100, 160, 255})
			screen.DrawImage(placeholder, op)
		}
	}
}

func (sc *Scene) drawTileContent(screen *ebiten.Image, t Tile, cx, cy float64) {
	switch t.Type {
	case CardPair:
		txt := fmt.Sprintf("#%d", t.PairID)
		drawText(screen, txt, int(cx)-10, int(cy)-7, color.RGBA{0, 0, 0, 255})
	case CardWild:
		if sc.frogImg != nil {
			fop := &ebiten.DrawImageOptions{}
			fop.GeoM.Scale(0.35, 0.35)
			fop.GeoM.Translate(cx-18, cy-18)
			screen.DrawImage(sc.frogImg, fop)
		} else {
			drawText(screen, "WILD", int(cx)-16, int(cy)-7, color.RGBA{0, 180, 0, 255})
		}
	case CardPass:
		if sc.snakeImg != nil {
			sop := &ebiten.DrawImageOptions{}
			sop.GeoM.Scale(0.35, 0.35)
			sop.GeoM.Translate(cx-18, cy-18)
			screen.DrawImage(sc.snakeImg, sop)
		} else {
			drawText(screen, "PASS", int(cx)-16, int(cy)-7, color.RGBA{200, 0, 0, 255})
		}
	}
}

func (sc *Scene) drawScoreboard(screen *ebiten.Image) {
	// Draw right side score pad
	panel := ebiten.NewImage(180, 140)
	panel.Fill(color.RGBA{0, 0, 0, 200})

	pop := &ebiten.DrawImageOptions{}
	pop.GeoM.Translate(565, 55)
	screen.DrawImage(panel, pop)

	drawText(screen, "PLAYERS & SCORES", 575, 65, color.RGBA{255, 200, 50, 255})

	y := 90
	for _, p := range sc.game.Players {
		score := sc.game.Scores[p]
		var col color.Color = color.White
		if p == sc.localPlayer {
			col = color.RGBA{100, 255, 100, 255}
		}
		if p == sc.game.Players[sc.game.CurrentTurnIdx] {
			drawText(screen, ">", 570, y, color.RGBA{255, 220, 0, 255})
		}
		drawText(screen, fmt.Sprintf("%-10s : %d pairs", p, score), 585, y, col)
		y += 18
	}
}

func (sc *Scene) drawGameOver(screen *ebiten.Image) {
	// Dim background
	overlay := ebiten.NewImage(760, 520)
	overlay.Fill(color.RGBA{0, 0, 0, 220})
	screen.DrawImage(overlay, nil)

	// Modal box
	box := ebiten.NewImage(380, 260)
	box.Fill(color.RGBA{25, 30, 45, 255})
	bop := &ebiten.DrawImageOptions{}
	bop.GeoM.Translate(190, 130)
	screen.DrawImage(box, bop)

	drawText(screen, "UNCOVER THE MUSIC - VICTORY!", 220, 150, color.RGBA{255, 215, 0, 255})

	// Show winners and awards
	bonusMap := sc.game.calculateBonusRewards()
	y := 190
	for _, p := range sc.game.Players {
		bonus := bonusMap[p]
		matchDB := sc.game.Scores[p]
		total := matchDB + bonus
		line := fmt.Sprintf("%-10s : %d pairs (%d dB) + %d bonus = %d dB", p, matchDB, matchDB, bonus, total)
		var col color.Color = color.White
		if p == sc.localPlayer {
			col = color.RGBA{100, 255, 100, 255}
		}
		drawText(screen, line, 210, y, col)
		y += 22
	}

	// Exit button
	btn := ebiten.NewImage(100, 30)
	btn.Fill(color.RGBA{200, 30, 30, 255})
	btnOp := &ebiten.DrawImageOptions{}
	btnOp.GeoM.Translate(330, 340)
	screen.DrawImage(btn, btnOp)
	drawText(screen, "EXIT", 365, 347, color.White)
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
		// Draw 5x7 matrix character
		drawChar(dst, dot, ch, cx, cy)
		cx += 7.0
	}
}

func drawChar(dst, dot *ebiten.Image, ch rune, x, y float64) {
	// Monospace 5x7 bit patterns
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
	'.': {0x00, 0x00, 0x00, 0x00, 0x00, 0x0C, 0x0C},
	',': {0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x08},
	'_': {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1F},
}
