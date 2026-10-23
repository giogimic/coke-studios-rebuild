package boot

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/fs"

	"github.com/hajimehoshi/ebiten/v2"
)

type framesManifest struct {
	Fr     float64  `json:"Fr"`
	Width  int      `json:"width"`
	Height int      `json:"height"`
	Frames []string `json:"frames"`
}

type framePlayer struct {
	images []*ebiten.Image
	fps    float64
	accum  time.Duration
	idx    int
}

func loadFramePlayer(dir string) (*framePlayer, error) {
	mfPath := filepath.Join(dir, "manifest.json")
	b, err := fs.ReadFile(mfPath)
	if err != nil {
		return nil, err
	}
	var mf framesManifest
	if err := json.Unmarshal(b, &mf); err != nil {
		return nil, err
	}
	fps := mf.Fr
	imgs := make([]*ebiten.Image, 0, len(mf.Frames))
	for _, fn := range mf.Frames {
		img, err := lingoassets.LoadImage(filepath.Join(dir, fn))
		if err != nil {
			return nil, fmt.Errorf("frame %s: %w", fn, err)
		}
		imgs = append(imgs, img)
	}
	return &framePlayer{images: imgs, fps: fps}, nil
}

func (p *framePlayer) Update(dt time.Duration) {
	if p == nil || p.fps <= 0 || len(p.images) == 0 {
		return
	}
	p.accum += dt
	frameDur := time.Second / time.Duration(p.fps)
	for p.accum >= frameDur {
		p.accum -= frameDur
		p.idx = (p.idx + 1) % len(p.images)
	}
}

func (p *framePlayer) Draw(screen *ebiten.Image, x, y float64) {
	if p == nil || len(p.images) == 0 {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(x, y)
	screen.DrawImage(p.images[p.idx], op)
}
