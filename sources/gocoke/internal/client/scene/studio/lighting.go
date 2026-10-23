package studio

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// LightingSystem manages Kage shader-based lighting overlays.
type LightingSystem struct {
	shader    *ebiten.Shader
	enabled   bool
	maxLights int
	profiles  map[string]LightProfile // substring match keys -> profile
	startTS   time.Time
	// Global falloff controls to reduce hazy tails
	falloffPow float32 // exponent applied to radial term (>=1)
	minCutoff  float32 // soft cutoff to drop tiny tails (0..1)
}

type Light struct {
	X         float32
	Y         float32
	Radius    float32
	ColorR    float32
	ColorG    float32
	ColorB    float32
	Intensity float32
}

// LightProfile defines configurable light parameters per item type/base.
type LightProfile struct {
	ColorR float32
	ColorG float32
	ColorB float32
	Radius float32
	// Base intensity before pulsation
	Intensity float32
	// Pulsation controls (applied to intensity and optionally to radius)
	PulsateAmp       float32 // 0..1 proportion added to base intensity
	PulsateRadiusAmp float32 // 0..1 proportion of radius modulation
	PulsateFreqHz    float32 // cycles per second
	PulsatePhaseRad  float32 // phase offset in radians
}

// NewLightingSystem initializes the lighting shader (WASM and desktop compatible).
func NewLightingSystem() (*LightingSystem, error) {
	// Kage shader source compatible with Ebiten 2.8.x
	// Renders an additive radial glow for up to MaxLights light sources.
	const src = `
package main

const MaxLights = 32

// Exposed uniforms (must be exported for host to set)
var LightCount float
var LightPosX [MaxLights]float
var LightPosY [MaxLights]float
var LightRadius [MaxLights]float
var LightColorR [MaxLights]float
var LightColorG [MaxLights]float
var LightColorB [MaxLights]float
var LightIntensity [MaxLights]float

// Global falloff controls
var FalloffPow float
var MinContribution float

// Fragment is the entry point for Ebiten's Kage shader
func Fragment(position vec4, texCoord vec2, color vec4) vec4 {
    p := position.xy
    accum := vec3(0.0)
    for i := 0; i < MaxLights; i++ {
        if float(i) >= LightCount {
            break
        }
        lp := vec2(LightPosX[i], LightPosY[i])
        r := LightRadius[i]
        // Distance-based falloff with smooth edge: (1 - d/r)^2 clamped to [0,1]
        d := distance(p, lp)
        a := 1.0 - d/r
        if a < 0.0 {
            a = 0.0
        }
        // Exponential falloff to tighten glow; default ~3.5 when not set
        powv := FalloffPow
        if powv <= 0.0 {
            powv = 3.5
        }
        a = pow(a, powv)
        // Soft cutoff to remove hazy tails, remap to [0,1]
        cut := MinContribution
        if cut < 0.0 { cut = 0.0 }
        if cut > 0.99 { cut = 0.99 }
        t := a - cut
        if t < 0.0 { t = 0.0 }
        denom := 1.0 - cut
        if denom < 1e-5 { denom = 1e-5 }
        a = t / denom
        inten := LightIntensity[i]
        c := vec3(LightColorR[i], LightColorG[i], LightColorB[i]) * (a * inten)
        accum += c
    }
    // Soft clamp to avoid oversaturation (tighter cap)
    accum = min(accum, vec3(1.2, 1.2, 1.2))
    alpha := max(accum.r, max(accum.g, accum.b))
    alpha = min(alpha, 1.0)
    return vec4(accum, alpha)
}
`

	sh, err := ebiten.NewShader([]byte(src))
	if err != nil {
		// Surface shader compile errors to console to aid debugging on WASM
		fmt.Printf("[lighting] shader init failed: %v\n", err)
		return nil, err
	}
	ls := &LightingSystem{
		shader:     sh,
		enabled:    false,
		maxLights:  32,
		profiles:   make(map[string]LightProfile),
		startTS:    time.Now(),
		falloffPow: 3.5,
		minCutoff:  0.06,
	}
	// Default profiles for known items (can be overridden via SetProfile)
	ls.profiles["jukebox"] = LightProfile{ColorR: 1.0, ColorG: 0.75, ColorB: 0.25, Radius: 140, Intensity: 0.5, PulsateAmp: 0.1, PulsateFreqHz: 4}
	ls.profiles["pinball"] = LightProfile{ColorR: 1.0, ColorG: 0.75, ColorB: 0.25, Radius: 140, Intensity: 0.5, PulsateAmp: 0.1, PulsateFreqHz: 2.5}
	ls.profiles["tv"] = LightProfile{ColorR: 0.45, ColorG: 1.0, ColorB: 0.9, Radius: 240, Intensity: 0.4, PulsateAmp: 0.1, PulsateFreqHz: 60}
	ls.profiles["vanillacokeshelf"] = LightProfile{ColorR: 1.0, ColorG: 0.75, ColorB: 0.25, Radius: 200, Intensity: 0.4, PulsateAmp: 0, PulsateFreqHz: 0}
	ls.profiles["neonsign"] = LightProfile{ColorR: 1.0, ColorG: 0.55, ColorB: 0.15, Radius: 260, Intensity: 0.6, PulsateAmp: 0.5, PulsateFreqHz: 0}
	ls.profiles["arcade_sign"] = LightProfile{ColorR: 0.85, ColorG: 0.25, ColorB: 1.0, Radius: 260, Intensity: 0.6, PulsateAmp: 0.5, PulsateFreqHz: 0}
	return ls, nil
}

func (ls *LightingSystem) Enabled() bool { return ls != nil && ls.enabled }
func (ls *LightingSystem) SetEnabled(on bool) {
	if ls != nil {
		ls.enabled = on
	}
}

// SetFalloff configures global glow tightening.
func (ls *LightingSystem) SetFalloff(power float32, minContribution float32) {
	if ls == nil {
		return
	}
	if power < 0.5 {
		power = 0.5
	}
	if minContribution < 0 {
		minContribution = 0
	}
	if minContribution > 0.95 {
		minContribution = 0.95
	}
	ls.falloffPow = power
	ls.minCutoff = minContribution
}

// SetProfile sets/overrides a light profile for matching key (substring match against base name).
func (ls *LightingSystem) SetProfile(key string, p LightProfile) {
	if ls == nil {
		return
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return
	}
	ls.profiles[key] = p
}

// matchProfile finds the most specific profile by choosing the longest matching key contained in base.
func (ls *LightingSystem) matchProfile(base string) (LightProfile, bool) {
	if ls == nil || ls.profiles == nil {
		return LightProfile{}, false
	}
	base = strings.ToLower(strings.TrimSpace(base))
	bestLen := -1
	var best LightProfile
	for k, p := range ls.profiles {
		if strings.Contains(base, k) {
			if len(k) > bestLen {
				bestLen = len(k)
				best = p
			}
		}
	}
	if bestLen >= 0 {
		return best, true
	}
	return LightProfile{}, false
}

// BuildLights scans current draws to produce light sources at correct on-screen positions.
// Only active furniture animation frames emit light; specific wall items can emit always.
func (ls *LightingSystem) BuildLights(scene *Scene) []Light {
	if ls == nil || scene == nil || scene.placement == nil {
		return nil
	}
	// Reuse placement's draw builder to get exact post-offset coordinates
	draws, _ := scene.placement.BuildDrawsSimple()
	if len(draws) == 0 {
		return nil
	}
	// Time for pulsation
	elapsed := float32(time.Since(ls.startTS).Seconds())

	lights := make([]Light, 0, 8)
	for _, d := range draws {
		if d.img == nil {
			continue
		}
		if d.order == "wall" && d.pf != nil && d.pf.IsWallItem {
			// Wall items: if a profile exists, create a light
			if prof, ok := ls.matchProfile(d.pf.Base); ok {
				cx := float32(d.x + float64(d.img.Bounds().Dx())*0.5)
				cy := float32(d.y + float64(d.img.Bounds().Dy())*0.5)
				// Pulsate intensity and radius
				mod := float32(1.0)
				if prof.PulsateAmp != 0 && prof.PulsateFreqHz != 0 {
					mod += prof.PulsateAmp * float32(math.Sin(float64(2*math.Pi)*float64(prof.PulsateFreqHz)*float64(elapsed)+float64(prof.PulsatePhaseRad)))
				}
				radius := prof.Radius * (1 + prof.PulsateRadiusAmp*float32(math.Sin(float64(2*math.Pi)*float64(prof.PulsateFreqHz)*float64(elapsed)+float64(prof.PulsatePhaseRad))))
				if radius < 1 {
					radius = 1
				}
				lights = append(lights, Light{
					X: cx, Y: cy,
					Radius:    radius,
					ColorR:    prof.ColorR,
					ColorG:    prof.ColorG,
					ColorB:    prof.ColorB,
					Intensity: prof.Intensity * mod,
				})
			}
			continue
		}
		// Furniture slices: require active animation frames
		if (d.order == "a" || d.order == "b" || d.order == "c" || d.order == "d") && d.pf != nil && !d.pf.IsWallItem {
			if d.pf.State != 1 || d.pf.Frame <= 0 {
				continue
			}
			prof, ok := ls.matchProfile(d.pf.Base)
			if !ok {
				continue
			}
			// Also ensure this slice animates (avoid static slices)
			if d.pf.ContigMax != nil && d.pf.ContigMax[d.order] <= 0 {
				continue
			}
			cx := float32(d.x + float64(d.img.Bounds().Dx())*0.5)
			cy := float32(d.y + float64(d.img.Bounds().Dy())*0.5)
			mod := float32(1.0)
			if prof.PulsateAmp != 0 && prof.PulsateFreqHz != 0 {
				mod += prof.PulsateAmp * float32(math.Sin(float64(2*math.Pi)*float64(prof.PulsateFreqHz)*float64(elapsed)+float64(prof.PulsatePhaseRad)))
			}
			radius := prof.Radius * (1 + prof.PulsateRadiusAmp*float32(math.Sin(float64(2*math.Pi)*float64(prof.PulsateFreqHz)*float64(elapsed)+float64(prof.PulsatePhaseRad))))
			if radius < 1 {
				radius = 1
			}
			lights = append(lights, Light{
				X: cx, Y: cy,
				Radius:    radius,
				ColorR:    prof.ColorR,
				ColorG:    prof.ColorG,
				ColorB:    prof.ColorB,
				Intensity: prof.Intensity * mod,
			})
		}
	}
	// Cap lights to max supported
	if len(lights) > ls.maxLights {
		lights = lights[:ls.maxLights]
	}
	return lights
}

// Render overlays an additive glow for the given lights.
func (ls *LightingSystem) Render(dst *ebiten.Image, lights []Light) {
	if ls == nil || !ls.enabled || ls.shader == nil || dst == nil || len(lights) == 0 {
		return
	}
	// Prepare uniforms as parallel arrays
	count := len(lights)
	posX := make([]float32, ls.maxLights)
	posY := make([]float32, ls.maxLights)
	radii := make([]float32, ls.maxLights)
	colR := make([]float32, ls.maxLights)
	colG := make([]float32, ls.maxLights)
	colB := make([]float32, ls.maxLights)
	inten := make([]float32, ls.maxLights)
	for i := 0; i < count && i < ls.maxLights; i++ {
		l := lights[i]
		posX[i] = l.X
		posY[i] = l.Y
		radii[i] = float32(math.Max(1, float64(l.Radius)))
		colR[i] = l.ColorR
		colG[i] = l.ColorG
		colB[i] = l.ColorB
		inten[i] = l.Intensity
	}
	uni := map[string]interface{}{
		"LightCount":      float32(count),
		"LightPosX":       posX,
		"LightPosY":       posY,
		"LightRadius":     radii,
		"LightColorR":     colR,
		"LightColorG":     colG,
		"LightColorB":     colB,
		"LightIntensity":  inten,
		"FalloffPow":      float32(ls.falloffPow),
		"MinContribution": float32(ls.minCutoff),
	}
	op := &ebiten.DrawRectShaderOptions{Uniforms: uni}
	// Additive blend on top of scene
	op.Blend = ebiten.BlendLighter
	b := dst.Bounds()
	// Ensure we draw something even if fully black background; set a tiny clear color if needed
	_ = color.Black
	dst.DrawRectShader(b.Dx(), b.Dy(), ls.shader, op)
}
