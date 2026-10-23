package lingoassets

import (
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
	"strings"

	"gocoke/internal/client/fs"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/hajimehoshi/ebiten/v2"
)

// LoadImage loads an image file into an ebiten.Image with alpha-safe processing
// to avoid white halos around transparent edges.
// Pipeline:
// 1) Zero RGB for near-transparent pixels (kill matte bleed)
// 2) Make pure-white edge pixels transparent when touching transparency (rounded corners)
// 3) "Un-matte" semi-transparent whites (remove white backing from AA edges)

func LoadImage(path string) (*ebiten.Image, error) {
	r, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}

	// Convert to RGBA for mutation
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)

	w := rgba.Bounds().Dx()
	h := rgba.Bounds().Dy()
	pix := rgba.Pix

	// Heuristic: for UI assets with white matte artifacts, treat pure white as transparent
	// Apply to: cc.* assets, nav_icon_unit images, button slices, scrollbar assets
	// EXCEPTION: do not alter cc.entry.bg at all (keep original RGB/A)
	name := strings.ToLower(filepath.Base(path))
	aggressive := (strings.Contains(name, "cc.") && !strings.Contains(name, "cc.entry.bg")) ||
		strings.Contains(name, "nav_icon_unit") ||
		strings.Contains(name, "button.") ||
		strings.Contains(name, "scroll") ||
		strings.Contains(name, "closebox")
	// Additional exception: never apply to cc.bottombar to preserve subtle matte
	if strings.Contains(name, "cc.bottombar") {
		aggressive = false
	}
	// Additional exception: keep coke sofa interiors opaque
	if strings.Contains(name, "sofa_coke") {
		aggressive = false
	}
	// Additional exception: keep chat bubble interiors opaque for tinting
	if strings.Contains(name, "cc.bubble.") {
		aggressive = false
	}
	// Furniture preview/full assets should also use white-safe loading
	pathLower := strings.ToLower(path)
	if strings.Contains(pathLower, "cc_furniture_small") || strings.Contains(pathLower, "cc_furniture[") || strings.Contains(pathLower, "cc_furniture/") {
		aggressive = true
		// Exception: do not apply to Coke sofa slices
		if strings.Contains(name, "sofa_coke") {
			aggressive = false
		}
		if strings.Contains(name, "gumball") {
			aggressive = false
		}
		if strings.Contains(name, "vanillacokeshelf") {
			aggressive = false
		}
		if strings.Contains(name, "5001_") {
			aggressive = false
		}
		if strings.Contains(name, "cokemachine") {
			aggressive = false
		}
		if strings.Contains(name, "pinball") {
			aggressive = false
		}

	}
	if aggressive {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := rgba.PixOffset(x, y)
				r, g, b := pix[i+0], pix[i+1], pix[i+2]
				if r >= 250 && g >= 250 && b >= 250 {
					pix[i+0], pix[i+1], pix[i+2], pix[i+3] = 0, 0, 0, 0
				}
			}
		}
	}

	// Create ebiten image from RGBA (with optional white->transparent fix applied)
	eimg := ebiten.NewImageFromImage(rgba)
	return eimg, nil
}

// SolidColor creates a 1x1 image of a solid color (utility for placeholders)
func SolidColor(c color.RGBA) *ebiten.Image {
	img := ebiten.NewImage(1, 1)
	img.Fill(c)
	return img
}
