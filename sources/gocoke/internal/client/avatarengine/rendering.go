package avatarengine

import (
	"fmt"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
)

// Draw renders the complete avatar by compositing all body parts
// Matches Lingo AvatarEngine.draw() behavior with proper Z-ordering
func (e *AvatarEngine) Draw() error {
	if e.Buffer == nil {
		return fmt.Errorf("buffer not initialized")
	}

	// Clear the buffer
	e.Buffer.Fill(color.RGBA{0, 0, 0, 0}) // Transparent

	// Get current action for each body part type
	bodyAction := e.getBodyActionForBodyPart("bd") // Body determines main action
	faceAction := e.getFaceActionForBodyPart("fc") // Face has separate actions

	// Render each body part in the correct Z-order
	for _, bodyPart := range e.DrawOrder {
		err := e.drawBodyPart(bodyPart, bodyAction, faceAction)
		if err != nil {
			// Log error but continue with other body parts
			//log.Printf("AvatarEngine: failed to draw body part %s: %v", bodyPart, err)
		}
	}

	return nil
}

// DrawShadow renders the avatar's shadow
func (e *AvatarEngine) DrawShadow() error {
	if e.ShadowBuffer == nil {
		return fmt.Errorf("shadow buffer not initialized")
	}

	// Clear the shadow buffer
	e.ShadowBuffer.Fill(color.RGBA{0, 0, 0, 0})

	// Dont draw shadow if sitting
	if e.BodyAction == "sit" {
		return nil
	}

	// In Lingo, there are two shadow types: human_shadow_sh and human_shadow_h
	// For now, we'll just use a placeholder asset name.
	shadowAssetName := "1_human_shadow_h.png"
	asset, err := e.AssetManager.GetAssetByName(shadowAssetName)
	if err != nil {
		return fmt.Errorf("failed to load shadow asset %s: %w", shadowAssetName, err)
	}

	// Draw the shadow asset to the shadow buffer
	// (Positioning may need to be adjusted based on the asset)
	opts := &ebiten.DrawImageOptions{}

	// Center the shadow horizontally.
	bufferBounds := e.ShadowBuffer.Bounds()
	assetBounds := asset.Bounds()
	cx := float64(bufferBounds.Dx()-assetBounds.Dx()) / 2.0

	// Position the shadow vertically using its registration point relative to the avatar floor.
	// The effective floor is at y=90, not 110, to account for the 20px of vertical
	// padding at the top of the buffer for hats and tall hair.
	regPoint, hasRegPoint := e.AssetManager.GetRegistrationPoint(shadowAssetName)
	ty := 90.0 // Default to the floor
	if hasRegPoint {
		ty = 90.0 - regPoint.Y
	} else {
		// Fallback if no registration point: align bottom of asset to floor.
		ty = 90.0 - float64(assetBounds.Dy())
	}
	opts.GeoM.Translate(cx+regPoint.X, ty)

	e.ShadowBuffer.DrawImage(asset, opts)

	return nil
}

// drawBodyPart renders a single body part to the avatar buffer
func (e *AvatarEngine) drawBodyPart(bodyPart, bodyAction, faceAction string) error {
	// Check if this body part is configured
	appearance, hasBodyPart := e.Attributes[bodyPart]
	if !hasBodyPart && bodyPart != "ri" {
		// Body part not configured, skip (except for "ri" which may be an item)
		return nil
	}

	// Default style from attributes
	style := ""
	if hasBodyPart {
		style = appearance.Style
	}

	// Determine the action to use for this body part (may be overridden by dance)
	action := e.getActionForBodyPart(bodyPart, bodyAction, faceAction)

	// Get the appropriate frame for this body part and action
	frame := e.getFrameForBodyPart(bodyPart, action)

	// Dance override for action+frame (matches Lingo aDanceOverride when dancing and bodyAction is std)
	if act, fr, ok := e.getDanceOverride(bodyPart); ok {
		action = act
		frame = fr
	}

	// Determine base direction for the part, applying dance offset if needed
	partDirection := e.Direction
	if dirOffset, ok := e.getDanceDirOffset(bodyPart); ok {
		partDirection = getDirectionWithOffset(partDirection, dirOffset)
	}

	// Determine direction to request and whether to flip like Lingo getRealDirection/bFlip
	actualDir, flip := e.getDirectionAndFlip(bodyPart, action, partDirection)

	// Load the asset
	asset, err := e.AssetManager.GetAssetForBodyPart(bodyPart, style, action, actualDir, frame)
	if err != nil {
		// Try fallback to standard action if the specific action fails
		if action != "std" {
			asset, err = e.AssetManager.GetAssetForBodyPart(bodyPart, style, "std", actualDir, 0)
			if err != nil {
				return fmt.Errorf("failed to load asset for %s (style %s, action %s, dir %d, frame %d): %w",
					bodyPart, style, action, actualDir, frame, err)
			}
			// If fallback is successful, we MUST use the action/frame for that asset
			action = "std"
			frame = 0
		} else {
			return fmt.Errorf("failed to load standard asset for %s: %w", bodyPart, err)
		}
	}

	// Per-frame dance location offset (Lingo aLocOff)
	offX, offY := e.getDanceLocOffset(bodyPart, partDirection)

	// Draw the asset to the buffer
	err = e.compositeAsset(asset, bodyPart, action, actualDir, frame, flip, offX, offY)
	if err != nil {
		return fmt.Errorf("failed to composite %s: %w", bodyPart, err)
	}

	// Apply flood fill if enabled
	if e.DoFloodFill {
		// This is a simplified stand-in for Lingo's floodFill.
		// Ebiten doesn't have a direct floodFill, so a real implementation
		// would require a more complex pixel-by-pixel algorithm.
		// For now, we'll just log that it would have been applied.
	}

	return nil
}

// compositeAsset draws an asset onto the avatar buffer with proper positioning using registration points
func (e *AvatarEngine) compositeAsset(asset *ebiten.Image, bodyPart string, action string, dir int, frame int, flip bool, offX float64, offY float64) error {
	if asset == nil {
		return fmt.Errorf("asset is nil")
	}

	// Get the appearance for registration point and color lookup
	appearance, hasAppearance := e.Attributes[bodyPart]
	if !hasAppearance {
		return fmt.Errorf("no appearance found for body part %s", bodyPart)
	}

	// Construct asset name for registration point lookup
	// Use the provided action/dir/frame so reg-point lookup matches the drawn asset
	assetName := fmt.Sprintf("h_%s_%s_%s_%d_%d", action, bodyPart, appearance.Style, dir, frame)

	// Get registration point from asset manager
	regPoint, hasRegPoint := e.AssetManager.GetRegistrationPoint(assetName)

	opts := &ebiten.DrawImageOptions{}

	// Set ink effect (composite mode)
	ink := e.getInkForBodyPart(bodyPart, action)
	opts.CompositeMode = ink

	// Apply color tinting using a ColorM matrix for all parts except eyes.
	// This mirrors Lingo's behavior of using bgColor to tint assets.
	if bodyPart != "ey" {
		if partColor, ok := appearance.Color.(color.RGBA); ok {
			// Normalize 0-255 to 0-1 for the color matrix
			r := float64(partColor.R) / 255.0
			g := float64(partColor.G) / 255.0
			b := float64(partColor.B) / 255.0
			opts.ColorM.Scale(r, g, b, 1.0)
		}
	}

	if hasRegPoint {
		// Ensure mirrored flip is applied for all parts when original dir was 4/5/6
		forceMirror := (e.Direction == 4 || e.Direction == 5 || e.Direction == 6)
		doFlip := flip || forceMirror

		// To correctly handle mirroring, we apply transformations in a specific order.
		// Ebiten's GeoM operations are applied in reverse order of calls (B(A(p)) from B(), A()).
		// We set them up to logically perform:
		// 1. Position the asset using its registration point.
		// 2. Flip the positioned asset horizontally.
		// 3. Translate the flipped asset to fit within the 77px avatar buffer.
		// 4. Apply any dance-related offsets.
		tx := -regPoint.X
		ty := -(regPoint.Y - 110.0)

		// Base positioning. This is the first logical operation on the asset.
		opts.GeoM.Translate(tx, ty)

		if doFlip {
			// Second, scale to flip horizontally around the new origin.
			opts.GeoM.Scale(-1, 1)
			// Third, translate to place the flipped asset correctly.
			opts.GeoM.Translate(77, 0)
		}

		// Finally, apply any dance location offsets. This is the last logical operation.
		opts.GeoM.Translate(offX, offY)

		// HACK: manual adjustment for west-facing coke bottle
		if bodyPart == "ri" && action == "crr" && e.Direction == 4 { // West
			opts.GeoM.Translate(-13, 0)
		}

		// TODO: Implement dithering with a custom shader
		e.Buffer.DrawImage(asset, opts)
		return nil
	}

	// Fallback: center the asset in the 77x130 buffer
	bufferBounds := e.Buffer.Bounds()
	assetBounds := asset.Bounds()
	cx := float64(bufferBounds.Dx()-assetBounds.Dx()) / 2.0
	cy := float64(bufferBounds.Dy()-assetBounds.Dy()) / 2.0
	// Fallback path: center the asset
	forceMirror := (e.Direction == 4 || e.Direction == 5 || e.Direction == 6)
	doFlip := flip || forceMirror
	if doFlip {
		aw := float64(assetBounds.Dx())
		opts.GeoM.Scale(-1, 1)
		opts.GeoM.Translate(aw, 0)
	}
	opts.GeoM.Translate(cx+offX, cy+offY)

	// HACK: manual adjustment for west-facing coke bottle
	if bodyPart == "ri" && action == "crr" && e.Direction == 4 { // West
		opts.GeoM.Translate(-20, 0)
	}

	// TODO: Implement dithering with a custom shader
	e.Buffer.DrawImage(asset, opts)
	return nil

}

// getInkForBodyPart determines the correct ink effect for a body part and action
func (e *AvatarEngine) getInkForBodyPart(bodyPart, action string) ebiten.CompositeMode {
	// Check for a custom ink in the body part's action config
	if e.Config != nil {
		if partConfig, ok := e.Config.BodyParts[bodyPart]; ok {
			if actionConfig, ok := partConfig.Actions[action]; ok {
				// Map Lingo ink codes to Ebiten composite modes
				// 36 = copy (Source), 41 = blend (SourceOver)
				switch actionConfig.Ink {
				case 36:
					return ebiten.CompositeModeCopy
				case 41:
					return ebiten.CompositeModeSourceOver
				}
			}
		}
	}
	// Return default ink if no custom ink is found
	return e.DefaultInk
}

// getDirectionAndFlip maps the current direction to a real asset direction and whether to flip
// Mirrors key parts of Lingo's getRealDirection and bFlip calculation using config
func (e *AvatarEngine) getDirectionAndFlip(bodyPart, action string, dir int) (int, bool) {
	// Always mirror for 4/5/6 (SE/NE/NW mirrors of 2/1/0) to avoid requesting non-existent assets
	if dir == 4 {
		return 2, true
	}
	if dir == 5 {
		return 1, true
	}
	if dir == 6 {
		return 0, true
	}

	if e.Config != nil {
		// If current dir supported, use it as-is
		if e.Config.IsValidDirection(bodyPart, action, dir) {
			return dir, false
		}
		// Map to a real (fake) direction and mirror horizontally
		dirMap := map[int]int{0: 6, 1: 5, 2: 4, 3: 7, 4: 2, 5: 1, 6: 0, 7: 3}
		if mapped, ok := dirMap[dir]; ok {
			if e.Config.IsValidDirection(bodyPart, action, mapped) {
				// Special-case face/eyes/hair/hat flip rules from Lingo when looking NW/SE
				flip := true
				// For face group, ensure we never resolve to 0 or 7 which they don't support.
				if bodyPart == "hd" || bodyPart == "fc" || bodyPart == "ey" || bodyPart == "hr" || bodyPart == "ht" {
					if mapped == 0 || mapped == 7 {
						mapped = 2 // Use East as safe face direction, will be flipped by caller if needed
					}
				}
				return mapped, flip
			}
		}
	}
	// Fallback
	return dir, false
}

// getActionForBodyPart determines the appropriate action for a body part
func (e *AvatarEngine) getActionForBodyPart(bodyPart, bodyAction, faceAction string) string {
	// Face and eye parts use face actions
	if bodyPart == "fc" {
		return faceAction
	}
	if bodyPart == "ey" {
		// Eyes use independent eye action for blinking, else follow face
		if e.EyeAction != "" {
			return e.EyeAction
		}
		return faceAction
	}

	// Head and hair follow speaking for face
	if bodyPart == "hd" && faceAction == "spk" {
		return "spk"
	}

	// Carrying/drinking overrides for hands/shoes and carried item
	if e.IsCarrying {
		if bodyPart == "ri" {
			if e.IsDrinking {
				return "drk"
			}
			return "crr"
		}
		// Direction-based side selection like Lingo
		leftSideActive := e.Direction > 3 && e.Direction < 7
		if bodyPart == "lh" || bodyPart == "ls" {
			if leftSideActive {
				if e.IsDrinking {
					return "drk"
				} else {
					return "crr"
				}
			}
			return "std"
		}
		if bodyPart == "rh" || bodyPart == "rs" {
			if !leftSideActive {
				if e.IsDrinking {
					return "drk"
				} else {
					return "crr"
				}
			}
			return "std"
		}
	}

	// Carried items follow hand actions
	if bodyPart == "ri" {
		if e.IsCarrying {
			return "crr"
		}
		if e.IsDrinking {
			return "drk"
		}
	}

	if bodyAction == "wlk" {
		switch bodyPart {
		case "bd", "lg", "sh", "lh", "rh", "ls", "rs":
			return "wlk"
		default:
			// For walking, parts not in the walk cycle use "std"
			return "std"
		}
	}

	// Most body parts follow the main body action
	return bodyAction
}

// getBodyActionForBodyPart gets the body action (std, wlk, sit)
func (e *AvatarEngine) getBodyActionForBodyPart(bodyPart string) string {
	return e.BodyAction
}

// getFaceActionForBodyPart gets the face action (std, spk, sml, etc.)
func (e *AvatarEngine) getFaceActionForBodyPart(bodyPart string) string {
	return e.FaceAction
}

// getFrameForBodyPart determines the appropriate frame for a body part and action
func (e *AvatarEngine) getFrameForBodyPart(bodyPart, action string) int {
	// Walking animation uses current animation step
	if action == "wlk" {
		// Walking has 4 frames (0, 1, 2, 3)
		return e.AnimStep % 4
	}

	// Speaking animation alternates between frames 0 and 1
	if action == "spk" {
		return e.AnimStep % 2
	}

	// Most actions use frame 0
	return 0
}

// getDanceOverride returns action/frame overrides for the current anim step when dancing
// Mirrors Lingo's aDanceOverride tables for key parts. Active only when dancing and body is in std.
func (e *AvatarEngine) getDanceOverride(bodyPart string) (string, int, bool) {
	if !e.IsDancing || e.BodyAction != "std" {
		return "", 0, false
	}

	// When carrying/drinking, apply dance overrides only to the NON-carrying side.
	// The carrying side keeps its carry/drink action and does not dance.
	if e.IsCarrying || e.IsDrinking {
		leftSideActive := e.Direction > 3 && e.Direction < 7
		if leftSideActive {
			// Carrying on LEFT: disable overrides for left hand/shoulder, allow right to dance
			if bodyPart == "lh" || bodyPart == "ls" {
				return "", 0, false
			}
		} else {
			// Carrying on RIGHT: disable overrides for right hand/shoulder, allow left to dance
			if bodyPart == "rh" || bodyPart == "rs" {
				return "", 0, false
			}
		}
	}

	step := e.AnimStep % 8
	switch bodyPart {
	case "bd", "lg", "sh":
		// [std0, std0, wlk0, wlk0, std0, std0, wlk2, wlk2]
		switch step {
		case 2, 3:
			return "wlk", 0, true
		case 6, 7:
			return "wlk", 2, true
		default:
			return "std", 0, true
		}
	case "lh":
		// [crr0, wlk1, std0, std0, std0, wlk1, crr0, crr0]
		switch step {
		case 0, 6, 7:
			return "crr", 0, true
		case 1, 5:
			return "wlk", 1, true
		default:
			return "std", 0, true
		}
	case "ls":
		// mirrors lh in Lingo table
		switch step {
		case 0, 6, 7:
			return "crr", 0, true
		case 1, 5:
			return "wlk", 1, true
		default:
			return "std", 0, true
		}
	case "rh":
		// [std0, wlk3, crr0, crr0, crr0, wlk3, std0, std0]
		switch step {
		case 1, 5:
			return "wlk", 3, true
		case 2, 3, 4:
			return "crr", 0, true
		default:
			return "std", 0, true
		}
	case "rs":
		// mirrors rh in Lingo table
		switch step {
		case 1, 5:
			return "wlk", 3, true
		case 2, 3, 4:
			return "crr", 0, true
		default:
			return "std", 0, true
		}
	}
	return "", 0, false
}

// getDanceLocOffset - per-frame translation used by Lingo during dance
func (e *AvatarEngine) getDanceLocOffset(bodyPart string, partDirection int) (float64, float64) {
	if !e.IsDancing || e.BodyAction != "std" {
		return 0, 0
	}

	// Lingo's special head/face nudge for NW/SE directions during dance.
	// This overrides the standard vertical offset.
	switch bodyPart {
	case "hd", "fc", "ey", "hr", "ht":
		if (e.Direction == 3 || e.Direction == 7) && (partDirection == 4 || partDirection == 6) {
			return -4, 0
		}
	}

	// Apply to the same groups Lingo uses for aLocOff: hands, shoes, chest, head group
	switch bodyPart {
	case "lh", "ls", "rh", "rs", "ch", "hd", "fc", "ey", "hr", "ri":
		step := e.AnimStep % 8
		if step == 2 || step == 3 || step == 6 || step == 7 {
			return 0, -1
		}
	}
	return 0, 0
}

// getDanceDirOffset returns a directional offset for the current anim step when dancing.
func (e *AvatarEngine) getDanceDirOffset(bodyPart string) (int, bool) {
	if !e.IsDancing || e.BodyAction != "std" {
		return 0, false
	}
	// Head parts turn side to side during dance
	switch bodyPart {
	case "hd", "fc", "ey", "hr", "ht":
		step := e.AnimStep % 8
		if step < 4 {
			return 1, true // Look right
		}
		return -1, true // Look left
	}
	return 0, false
}

// getDirectionWithOffset applies a directional offset, wrapping around 0-7.
func getDirectionWithOffset(dir, offset int) int {
	newDir := dir + offset
	if newDir < 0 {
		newDir = 8 + newDir
	}
	return newDir % 8
}

// applyBodyPartOffset applies body part specific positioning offsets
func (e *AvatarEngine) applyBodyPartOffset(bodyPart string, x, y int) (int, int) {
	// In a full implementation, each body part would have specific
	// registration points and offsets. For now, we'll use basic adjustments.

	switch bodyPart {
	case "hd": // Head - slightly up
		y -= 10
	case "fc", "ey": // Face and eyes - follow head
		y -= 8
	case "hr": // Hair - follow head
		y -= 12
	case "ht": // Hat - above head
		y -= 15
	case "lg": // Legs - lower
		y += 20
	case "sh", "ls", "rs": // Shoes - bottom
		y += 25
	}

	return x, y
}

// SetBodyAction sets the main body action (std, wlk, sit)
func (e *AvatarEngine) SetBodyAction(action string) {
	validActions := map[string]bool{
		"std": true,
		"wlk": true,
		"sit": true,
	}

	if !validActions[action] {
		log.Printf("AvatarEngine: invalid body action: %s", action)
		return
	}

	e.BodyAction = action
	log.Printf("AvatarEngine: set body action to %s", action)
}

// SetFaceAction sets the face action (std, spk, sml, agr, srp, sad)
func (e *AvatarEngine) SetFaceAction(action string) {
	validActions := map[string]bool{
		"std": true,
		"spk": true,
		"sml": true,
		"agr": true,
		"srp": true,
		"sad": true,
	}

	if !validActions[action] {
		log.Printf("AvatarEngine: invalid face action: %s", action)
		return
	}
	e.FaceAction = action

	// In Lingo, SetFaceAction has a switch that resets parts first
	// then applies the new action.
	e.ResetFace() // Clear existing expression before applying a new one.

	switch action {
	case "std":
		// resetFace already handled this.
	case "spk":
		// Speaking affects head, face, but not eyes.
		e.FaceAction = "spk"
	case "sml", "agr", "srp", "sad":
		// Emotes affect face and eyes.
		e.FaceAction = action
		e.EyeAction = action
	}

	log.Printf("AvatarEngine: set face action to %s", action)
}

// ResetFace resets face and related parts to standard state, like in Lingo
func (e *AvatarEngine) ResetFace() {
	if !e.IsChatting {
		e.FaceAction = "std"
	}
	e.EyeAction = "std"
}

// GetFaceAction returns the current face action
func (e *AvatarEngine) GetFaceAction() string {
	return e.FaceAction
}

// SetAnimationFrame sets the current animation frame
func (e *AvatarEngine) SetAnimationFrame(frame int) {
	if frame < 0 {
		frame = 0
	}
	if frame > e.AnimStepMax {
		frame = e.AnimStepMax
	}

	e.AnimStep = frame
}

// GetAnimationFrame returns the current animation frame
func (e *AvatarEngine) GetAnimationFrame() int {
	return e.AnimStep
}

// AdvanceAnimation advances the animation by one frame
func (e *AvatarEngine) AdvanceAnimation() {
	e.AnimStep++
	if e.AnimStep > e.AnimStepMax {
		e.AnimStep = 0
	}
}

// ResetAnimation resets animation to frame 0
func (e *AvatarEngine) ResetAnimation() {
	e.AnimStep = 0
}
