package scene

import "github.com/hajimehoshi/ebiten/v2"

type Scene interface {
	Update() error
	Draw(screen *ebiten.Image)
	Layout(ow, oh int) (int, int)
}

type Manager struct {
	cur         Scene
	renderScale float64
}

func (m *Manager) Set(s Scene)    { m.cur = s }
func (m *Manager) Current() Scene { return m.cur }
func (m *Manager) Update() error {
	if m.cur != nil {
		return m.cur.Update()
	}
	return nil
}
func (m *Manager) Draw(screen *ebiten.Image) {
	if m.cur != nil {
		m.cur.Draw(screen)
	}
}
func (m *Manager) Layout(ow, oh int) (int, int) {
	var baseW, baseH int
	if m.cur != nil {
		baseW, baseH = m.cur.Layout(ow, oh)
	} else {
		baseW, baseH = 960, 540
	}

	// On smaller viewports (typical mobile), upscale ~1.2x by reducing internal res.
	scale := 1.0
	if ow <= 1280 || oh <= 720 {
		scale = 1.17
	}
	m.renderScale = scale

	if scale != 1.0 {
		newW := int(float64(baseW)/scale + 0.5)
		newH := int(float64(baseH)/scale + 0.5)
		if newW < 320 {
			newW = 320
		}
		if newH < 180 {
			newH = 180
		}
		return newW, newH
	}
	return baseW, baseH
}
