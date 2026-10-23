package avatarengine

import (
	"fmt"
	"image/color"
	"log"
	"strconv"
	"strings"
)

// BodyPartAppearance holds the style and color for a single body part
type BodyPartAppearance struct {
	Style string
	Color color.Color
}

// SetAttributes parses and sets avatar attributes from a string
// Format: "hd.1.1.1001.hr.4.1002.ch.6.1003" (body part, gender, color, style)
// Matches Lingo AvatarEngine.setAttributes() behavior
func (e *AvatarEngine) SetAttributes(attributeString string) error {
	if attributeString == "" {
		e.setDefaultAttributes()
		return nil
	}

	// Parse the attribute string
	attributes, err := ParseAttributeString(attributeString)
	if err != nil {
		return fmt.Errorf("failed to parse attributes: %w", err)
	}

	// Validate and set attributes
	for bodyPart, style := range attributes {
		if !e.isValidBodyPart(bodyPart) {
			log.Printf("AvatarEngine: ignoring invalid body part: %s", bodyPart)
			continue
		}

		e.Attributes[bodyPart] = style
	}

	log.Printf("AvatarEngine: set attributes for %d body parts", len(e.Attributes))
	return nil
}

// ParseAttributeString parses a Lingo-style attribute string and returns a map of appearances.
// Lingo format: "hr=017/238,238,238&hd=001/115,92,69&..."
func ParseAttributeString(s string) (map[string]BodyPartAppearance, error) {
	appearances := make(map[string]BodyPartAppearance)
	if s == "" {
		return appearances, nil
	}

	// Lingo uses '&' as the delimiter, but Go's test string uses '&amp;'
	s = strings.ReplaceAll(s, "&amp;", "&")

	pairs := strings.Split(s, "&")
	for _, pair := range pairs {
		parts := strings.Split(pair, "=")
		if len(parts) != 2 {
			log.Printf("ParseAttributeString: skipping invalid part: %s", pair)
			continue
		}
		bodyPart := parts[0]

		valueParts := strings.Split(parts[1], "/")
		if len(valueParts) != 2 {
			log.Printf("ParseAttributeString: skipping invalid value for %s: %s", bodyPart, parts[1])
			continue
		}
		style := valueParts[0]
		colorStr := valueParts[1]

		colorParts := strings.Split(colorStr, ",")
		if len(colorParts) != 3 {
			log.Printf("ParseAttributeString: skipping invalid color for %s: %s", bodyPart, colorStr)
			continue
		}

		r, errR := strconv.Atoi(colorParts[0])
		g, errG := strconv.Atoi(colorParts[1])
		b, errB := strconv.Atoi(colorParts[2])
		if errR != nil || errG != nil || errB != nil {
			log.Printf("ParseAttributeString: could not parse color components for %s: %s", bodyPart, colorStr)
			continue
		}

		appearances[bodyPart] = BodyPartAppearance{
			Style: style,
			Color: color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255},
		}
	}

	// Lingo always had a default 'ri' (right item) part.
	if _, ok := appearances["ri"]; !ok {
		appearances["ri"] = BodyPartAppearance{
			Style: "001",
			Color: color.RGBA{R: 255, G: 255, B: 255, A: 255},
		}
	}

	return appearances, nil
}

// GetAttributeString serializes current attributes to a Lingo-style string
func (e *AvatarEngine) GetAttributeString() string {
	var parts []string
	bodyPartOrder := []string{"hr", "hd", "ey", "fc", "bd", "lh", "rh", "ch", "ls", "rs", "lg", "sh"}

	for _, bodyPart := range bodyPartOrder {
		if appearance, exists := e.Attributes[bodyPart]; exists {
			c, ok := appearance.Color.(color.RGBA)
			if !ok {
				c = color.RGBA{255, 255, 255, 255} // Fallback
			}
			part := fmt.Sprintf("%s=%s/%d,%d,%d", bodyPart, appearance.Style, c.R, c.G, c.B)
			parts = append(parts, part)
		}
	}

	return strings.Join(parts, "&")
}

// SetBodyPartStyle sets the style for a specific body part, keeping existing color
func (e *AvatarEngine) SetBodyPartStyle(bodyPart, style string) error {
	if !e.isValidBodyPart(bodyPart) {
		return fmt.Errorf("invalid body part: %s", bodyPart)
	}

	if _, err := strconv.Atoi(style); err != nil {
		return fmt.Errorf("invalid style format: %s", style)
	}

	if appearance, ok := e.Attributes[bodyPart]; ok {
		appearance.Style = style
		e.Attributes[bodyPart] = appearance
	} else {
		e.Attributes[bodyPart] = BodyPartAppearance{
			Style: style,
			Color: color.RGBA{255, 255, 255, 255}, // Default to white
		}
	}
	log.Printf("AvatarEngine: set %s style to %s", bodyPart, style)
	return nil
}

// GetBodyPartStyle returns the style for a body part
func (e *AvatarEngine) GetBodyPartStyle(bodyPart string) (string, bool) {
	if appearance, ok := e.Attributes[bodyPart]; ok {
		return appearance.Style, true
	}
	return "", false
}

// GetBodyPartColor returns the color for a body part
func (e *AvatarEngine) GetBodyPartColor(bodyPart string) (color.Color, bool) {
	if appearance, ok := e.Attributes[bodyPart]; ok {
		return appearance.Color, true
	}
	return nil, false
}

// RemoveBodyPart removes a body part from the avatar
func (e *AvatarEngine) RemoveBodyPart(bodyPart string) {
	delete(e.Attributes, bodyPart)
	log.Printf("AvatarEngine: removed body part %s", bodyPart)
}

// HasBodyPart checks if the avatar has a specific body part
func (e *AvatarEngine) HasBodyPart(bodyPart string) bool {
	_, exists := e.Attributes[bodyPart]
	return exists
}

// GetAllBodyParts returns all body parts currently configured
func (e *AvatarEngine) GetAllBodyParts() []string {
	var bodyParts []string
	for bodyPart := range e.Attributes {
		bodyParts = append(bodyParts, bodyPart)
	}
	return bodyParts
}

// isValidBodyPart checks if a body part identifier is valid
func (e *AvatarEngine) isValidBodyPart(bodyPart string) bool {
	validBodyParts := map[string]bool{
		"bd": true, // Body
		"hd": true, // Head
		"fc": true, // Face
		"ey": true, // Eyes
		"hr": true, // Hair
		"ch": true, // Chest/Shirt
		"ls": true, // Left Shoe
		"rs": true, // Right Shoe
		"lg": true, // Legs/Pants
		"lh": true, // Left Hand
		"rh": true, // Right Hand
		"sh": true, // Shoes (both)
		"ri": true, // Right Item (carried)
		"ht": true, // Hat
	}

	return validBodyParts[bodyPart]
}

// ValidateAttributes validates all current attributes against the configuration
func (e *AvatarEngine) ValidateAttributes() []string {
	var errors []string

	for bodyPart, appearance := range e.Attributes {
		// Check if body part is valid
		if !e.isValidBodyPart(bodyPart) {
			errors = append(errors, fmt.Sprintf("invalid body part: %s", bodyPart))
			continue
		}

		// Check if style is numeric
		if _, err := strconv.Atoi(appearance.Style); err != nil {
			errors = append(errors, fmt.Sprintf("invalid style for %s: %s", bodyPart, appearance.Style))
		}

		// Check if body part supports current action
		if e.Config != nil {
			if !e.Config.IsValidAction(bodyPart, e.BodyAction) {
				errors = append(errors, fmt.Sprintf("body part %s does not support action %s", bodyPart, e.BodyAction))
			}

			// Check if body part supports current direction
			if !e.Config.IsValidDirection(bodyPart, e.BodyAction, e.Direction) {
				errors = append(errors, fmt.Sprintf("body part %s does not support direction %d for action %s", bodyPart, e.Direction, e.BodyAction))
			}
		}
	}

	return errors
}

// GetRequiredBodyParts returns the minimum required body parts for a basic avatar
func GetRequiredBodyParts() []string {
	return []string{"bd", "hd", "lg"} // Body, head, legs are minimum
}

// GetOptionalBodyParts returns optional body parts that can be added
func GetOptionalBodyParts() []string {
	return []string{"fc", "ey", "hr", "ch", "ls", "rs", "sh", "lh", "rh", "ri", "ht"}
}

// SetDefaultAttributesForBodyPart sets a default style for a body part if not already set
func (e *AvatarEngine) SetDefaultAttributesForBodyPart(bodyPart string) {
	if !e.HasBodyPart(bodyPart) {
		defaultAppearance := BodyPartAppearance{
			Style: "001",
			Color: color.RGBA{255, 255, 255, 255}, // Default to white
		}
		e.Attributes[bodyPart] = defaultAppearance
		log.Printf("AvatarEngine: set default appearance for body part %s", bodyPart)
	}
}

// EnsureRequiredBodyParts ensures all required body parts have styles set
func (e *AvatarEngine) EnsureRequiredBodyParts() {
	required := GetRequiredBodyParts()
	for _, bodyPart := range required {
		e.SetDefaultAttributesForBodyPart(bodyPart)
	}
}

// CloneAttributes creates a copy of the current attributes
func (e *AvatarEngine) CloneAttributes() map[string]BodyPartAppearance {
	clone := make(map[string]BodyPartAppearance)
	for bodyPart, appearance := range e.Attributes {
		clone[bodyPart] = appearance
	}
	return clone
}

// SetAttributesFromMap sets attributes from a map
func (e *AvatarEngine) SetAttributesFromMap(attributes map[string]BodyPartAppearance) error {
	// Validate all attributes first
	for bodyPart, appearance := range attributes {
		if !e.isValidBodyPart(bodyPart) {
			return fmt.Errorf("invalid body part: %s", bodyPart)
		}

		if _, err := strconv.Atoi(appearance.Style); err != nil {
			return fmt.Errorf("invalid style for %s: %s", bodyPart, appearance.Style)
		}
	}

	// Clear existing attributes
	e.Attributes = make(map[string]BodyPartAppearance)

	// Set new attributes
	for bodyPart, appearance := range attributes {
		e.Attributes[bodyPart] = appearance
	}

	log.Printf("AvatarEngine: set attributes from map, %d body parts", len(e.Attributes))
	return nil
}

// GetBodyPartDisplayName returns a human-readable name for a body part
func GetBodyPartDisplayName(bodyPart string) string {
	displayNames := map[string]string{
		"bd": "Body",
		"hd": "Head",
		"fc": "Face",
		"ey": "Eyes",
		"hr": "Hair",
		"ch": "Chest/Shirt",
		"ls": "Left Shoe",
		"rs": "Right Shoe",
		"lg": "Legs/Pants",
		"lh": "Left Hand",
		"rh": "Right Hand",
		"sh": "Shoes",
		"ri": "Right Item",
		"ht": "Hat",
	}

	if name, exists := displayNames[bodyPart]; exists {
		return name
	}

	return bodyPart // Fallback to original
}
