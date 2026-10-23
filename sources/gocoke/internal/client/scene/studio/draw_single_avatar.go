package studio

import (
	"gocoke/internal/client/isoengine"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawSingleAvatar draws a single avatar buffer and shadow at its current interpolated screen position.
// This mirrors the drawing logic in drawAvatars for a single avatar.
func (s *Scene) drawSingleAvatar(screen *ebiten.Image, avatar *isoengine.IsoAvatar) {
	if avatar == nil || avatar.Engine == nil {
		return
	}

	// Update avatar animation
	if err := avatar.Engine.Update(); err != nil {
		return
	}
	// Redraw avatar shadow and buffer
	if err := avatar.Engine.DrawShadow(); err != nil {
		return
	}
	if err := avatar.Engine.Draw(); err != nil {
		return
	}

	avatarBuffer := avatar.Engine.GetBuffer()
	avatarShadowBuffer := avatar.Engine.GetShadowBuffer()
	if avatarBuffer == nil || avatarShadowBuffer == nil {
		return
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

	// Draw shadow and avatar (apply scale 50% in public rooms) with center compensation
	scale := 1.0
	if s.studio != nil && s.studio.Owner == "$SYSTEM$" {
		scale = 0.5
	}

	// Shadow
	shadowOpts := &ebiten.DrawImageOptions{}
	if scale != 1.0 {
		bw := float64(avatarShadowBuffer.Bounds().Dx())
		bh := float64(avatarShadowBuffer.Bounds().Dy())
		ax := (bw * (1.0 - scale)) / 2.0
		ay := (bh * (1.0 - scale)) / 2.0
		shadowOpts.GeoM.Scale(scale, scale)
		shadowOpts.GeoM.Translate(x+ax, y+ay)
	} else {
		shadowOpts.GeoM.Translate(x, y)
	}
	screen.DrawImage(avatarShadowBuffer, shadowOpts)

	// Body
	opts := &ebiten.DrawImageOptions{}
	if scale != 1.0 {
		bw := float64(avatarBuffer.Bounds().Dx())
		bh := float64(avatarBuffer.Bounds().Dy())
		ax := (bw * (1.0 - scale)) / 2.0
		ay := (bh * (1.0 - scale)) / 2.0
		opts.GeoM.Scale(scale, scale)
		opts.GeoM.Translate(x+ax, y-float64(yOffset)+ay)
	} else {
		opts.GeoM.Translate(x, y-float64(yOffset))
	}
	screen.DrawImage(avatarBuffer, opts)
}
