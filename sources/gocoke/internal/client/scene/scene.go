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
	if m.cur != nil {
		return m.cur.Layout(ow, oh)
	}
	return 760, 520
}
