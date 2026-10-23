package isoengine

import (
	"github.com/hajimehoshi/ebiten/v2"
)

// Wall asset reporting structures (moved from patterns package to break circular dependency)
type WallAssetInfo struct {
	Member string
	Width  int
	Height int
	Bytes  int
}

type WallTileInfo struct {
	Member    string
	AssetType string
	Direction string
	X, Y      float64
	HasImage  bool
}

type WallAssetsReport struct {
	Cached     []WallAssetInfo
	Tiles      []WallTileInfo
	TotalBytes int
}

// Draw depth reporting structures
type DrawDepthEntry struct {
	Kind  string // "furni" | "avatar"
	Name  string // base or avatar name
	Layer string // sd/a/b/c/d or body/shadow
	Depth int
}

type DrawDepthReport struct {
	Entries []DrawDepthEntry
}

// DepthBoostController allows the owning scene to receive depth boost commands
type DepthBoostController interface {
	SetDepthBoost(prodID string, amount int) string
	SetDepthBoostSlice(prodID string, slice string, amount int) string
}

// PatternManager interface to break circular dependency
type PatternManager interface {
	HasPatternForElement(elementID string) bool
	ApplyPatternToElement(screen *ebiten.Image, elementID string, baseImage *ebiten.Image, x, y float64)
}

// SceneOwnerBridge lets isoengine call back into the high-level scene for pattern rendering.
type SceneOwnerBridge struct {
	patternMgr PatternManager
	isoScene   *IsoScene
	depthProv  DrawDepthProvider
}

func NewSceneOwnerBridge(pm PatternManager) *SceneOwnerBridge {
	return &SceneOwnerBridge{patternMgr: pm}
}

// AttachIsoScene attaches the iso scene so reports can gather tile info
func (b *SceneOwnerBridge) AttachIsoScene(s *IsoScene) { b.isoScene = s }

// DrawDepthProvider provides current draw depths from the owning scene
type DrawDepthProvider interface {
	CollectDrawDepths() []DrawDepthEntry
}

// AttachDrawDepthProvider attaches a provider for draw depth reporting
func (b *SceneOwnerBridge) AttachDrawDepthProvider(p DrawDepthProvider) { b.depthProv = p }

// GetWallTiles returns per-tile info for wall assets (for reporting)
func (b *SceneOwnerBridge) GetWallTiles() []WallTileInfo {
	var tiles []WallTileInfo
	if s := b.isoScene; s != nil && s.wall != nil {
		for _, t := range s.wall.wallTiles {
			tiles = append(tiles, WallTileInfo{
				Member:    t.MemberName,
				AssetType: t.AssetType,
				Direction: t.Direction,
				X:         t.Position.X,
				Y:         t.Position.Y,
				HasImage:  t.Image != nil,
			})
		}
	}
	return tiles
}

// GetWallAssetsReport returns only per-tile info from the isoengine; Manager will merge cached assets
func (b *SceneOwnerBridge) GetWallAssetsReport() WallAssetsReport {
	var report WallAssetsReport
	if s := b.isoScene; s != nil && s.wall != nil {
		for _, t := range s.wall.wallTiles {
			report.Tiles = append(report.Tiles, WallTileInfo{
				Member:    t.MemberName,
				AssetType: t.AssetType,
				Direction: t.Direction,
				X:         t.Position.X,
				Y:         t.Position.Y,
				HasImage:  t.Image != nil,
			})
		}
	}
	return report
}

// GetDrawDepthReport returns the current furniture and avatar draw depths, if a provider is attached
func (b *SceneOwnerBridge) GetDrawDepthReport() DrawDepthReport {
	var r DrawDepthReport
	if b.depthProv != nil {
		r.Entries = b.depthProv.CollectDrawDepths()
	}
	return r
}

// SetDepthBoost forwards a depth boost command to the owning scene if supported
func (b *SceneOwnerBridge) SetDepthBoost(prodID string, amount int) string {
	if b.depthProv != nil {
		if ctrl, ok := b.depthProv.(DepthBoostController); ok {
			return ctrl.SetDepthBoost(prodID, amount)
		}
	}
	return "Depth boost controller not available"
}

// SetDepthBoostSlice forwards a per-slice depth boost command to the owning scene if supported
func (b *SceneOwnerBridge) SetDepthBoostSlice(prodID string, slice string, amount int) string {
	if b.depthProv != nil {
		if ctrl, ok := b.depthProv.(DepthBoostController); ok {
			return ctrl.SetDepthBoostSlice(prodID, slice, amount)
		}
	}
	return "Depth boost controller not available"
}

// SetAvatarDepthBoost forwards a global avatar depth boost to the owning scene if supported
func (b *SceneOwnerBridge) SetAvatarDepthBoost(amount int) string {
	if b.depthProv != nil {
		if ctrl, ok := b.depthProv.(interface{ SetAvatarDepthBoost(int) string }); ok {
			return ctrl.SetAvatarDepthBoost(amount)
		}
	}
	return "Avatar depth boost not supported by scene"
}
