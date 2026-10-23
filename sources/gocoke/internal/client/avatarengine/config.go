package avatarengine

import (
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	"gocoke/internal/client/fs"
)

// AvatarConfig holds configuration rules for avatar body parts and actions
// Based on the Lingo AvatarConfigSettings.txt file
type AvatarConfig struct {
	BodyParts        map[string]BodyPartConfig
	DefaultDrawOrder []string
	DirectionMap     map[int]string
}

// BodyPartConfig defines the rules for a specific body part
type BodyPartConfig struct {
	Actions map[string]ActionConfig
}

// ActionConfig defines valid directions and frames for a specific action
type ActionConfig struct {
	Directions []int `json:"dir"`
	Frames     []int `json:"fr"`
	Ink        int   `json:"ink,omitempty"` // Optional ink effect for this action
}

// LoadAvatarConfig loads the avatar configuration from the Lingo files
func LoadAvatarConfig(dataDir string) (*AvatarConfig, error) {
	config := &AvatarConfig{
		BodyParts:        make(map[string]BodyPartConfig),
		DefaultDrawOrder: []string{"lh", "ls", "bd", "sh", "lg", "ch", "ri", "rh", "rs", "hd", "fc", "ey", "hr", "ht"},
		DirectionMap: map[int]string{
			0: "North",
			1: "Northeast",
			2: "East",
			3: "Southeast",
			7: "Northwest",
		},
	}

	// Load configuration from AvatarConfigSettings.txt
	configPath := filepath.Join("Lingo", "avatarengine", "2_AvatarConfigSettings.txt")
	err := config.loadConfigFile(configPath)
	if err != nil {
		log.Printf("AvatarConfig: failed to load config file '%s': %v", configPath, err)
		log.Printf("AvatarConfig: using default configuration")
		config.setDefaultConfig()
	}

	log.Printf("AvatarConfig: loaded configuration for %d body parts", len(config.BodyParts))

	// Debug: Show what body parts we have configured (only once)
	if len(config.BodyParts) > 0 {
		log.Printf("AvatarConfig: configured body parts: %v", config.getConfiguredBodyParts())
	}
	return config, nil
}

// loadConfigFile parses the Lingo configuration file
func (c *AvatarConfig) loadConfigFile(path string) error {
	data, err := fs.ReadFile(path)
	if err != nil {
		return err
	}

	content := string(data)

	// Parse the Lingo-style configuration
	// The file contains a complex nested structure that we need to parse
	err = c.parseLingoConfig(content)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	return nil
}

// parseLingoConfig parses the Lingo configuration format
func (c *AvatarConfig) parseLingoConfig(content string) error {
	// This is a simplified parser for the Lingo config format
	// The actual format is quite complex, so we'll implement the key parts

	lines := strings.Split(content, "\n")
	var currentBodyPart string
	var currentAction string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// Look for body part definitions (e.g., "bd":)
		if strings.Contains(line, `":`) && !strings.Contains(line, "#dir") && !strings.Contains(line, "#fr") {
			parts := strings.Split(line, `":`)
			if len(parts) >= 1 {
				bodyPart := strings.Trim(parts[0], `" `)
				if len(bodyPart) == 2 { // Body parts are 2 characters
					currentBodyPart = bodyPart
					if c.BodyParts[currentBodyPart].Actions == nil {
						c.BodyParts[currentBodyPart] = BodyPartConfig{
							Actions: make(map[string]ActionConfig),
						}
					}
				}
			}
		}

		// Look for action definitions (e.g., "std": [#dir: [0, 1, 2, 3, 7], #fr: [0]])
		if strings.Contains(line, `"std":`) || strings.Contains(line, `"wlk":`) ||
			strings.Contains(line, `"sit":`) || strings.Contains(line, `"crr":`) ||
			strings.Contains(line, `"drk":`) || strings.Contains(line, `"spk":`) {

			// Extract action name
			if strings.Contains(line, `"std":`) {
				currentAction = "std"
			} else if strings.Contains(line, `"wlk":`) {
				currentAction = "wlk"
			} else if strings.Contains(line, `"sit":`) {
				currentAction = "sit"
			} else if strings.Contains(line, `"crr":`) {
				currentAction = "crr"
			} else if strings.Contains(line, `"drk":`) {
				currentAction = "drk"
			} else if strings.Contains(line, `"spk":`) {
				currentAction = "spk"
			}

			if currentBodyPart != "" && currentAction != "" {
				// Parse directions and frames
				directions := c.parseDirections(line)
				frames := c.parseFrames(line)

				bodyPartConfig := c.BodyParts[currentBodyPart]
				bodyPartConfig.Actions[currentAction] = ActionConfig{
					Directions: directions,
					Frames:     frames,
				}
				c.BodyParts[currentBodyPart] = bodyPartConfig
			}
		}
	}

	return nil
}

// parseDirections extracts direction values from a config line
func (c *AvatarConfig) parseDirections(line string) []int {
	// Look for #dir: [0, 1, 2, 3, 7] pattern
	dirStart := strings.Index(line, "#dir: [")
	if dirStart == -1 {
		return []int{0, 1, 2, 3, 7} // Default all directions
	}

	dirEnd := strings.Index(line[dirStart:], "]")
	if dirEnd == -1 {
		return []int{0, 1, 2, 3, 7}
	}

	dirStr := line[dirStart+7 : dirStart+dirEnd]
	dirParts := strings.Split(dirStr, ",")

	var directions []int
	for _, part := range dirParts {
		part = strings.TrimSpace(part)
		if dir, err := strconv.Atoi(part); err == nil {
			directions = append(directions, dir)
		}
	}

	if len(directions) == 0 {
		return []int{0, 1, 2, 3, 7} // Default
	}

	return directions
}

// parseFrames extracts frame values from a config line
func (c *AvatarConfig) parseFrames(line string) []int {
	// Look for #fr: [0] or #fr: [0, 1, 2, 3] pattern
	frStart := strings.Index(line, "#fr: [")
	if frStart == -1 {
		return []int{0} // Default single frame
	}

	frEnd := strings.Index(line[frStart:], "]")
	if frEnd == -1 {
		return []int{0}
	}

	frStr := line[frStart+6 : frStart+frEnd]
	frParts := strings.Split(frStr, ",")

	var frames []int
	for _, part := range frParts {
		part = strings.TrimSpace(part)
		if frame, err := strconv.Atoi(part); err == nil {
			frames = append(frames, frame)
		}
	}

	if len(frames) == 0 {
		return []int{0} // Default
	}

	return frames
}

// setDefaultConfig sets up default configuration if file loading fails
func (c *AvatarConfig) setDefaultConfig() {
	// Define default configurations for key body parts
	bodyParts := map[string]map[string]ActionConfig{
		"bd": { // Body
			"std": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
			"wlk": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0, 1, 2, 3}},
			"sit": {Directions: []int{0, 2}, Frames: []int{0}},
		},
		"hd": { // Head
			"std": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
			"spk": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0, 1}},
		},
		"fc": { // Face
			"std": {Directions: []int{1, 2, 3}, Frames: []int{0}},
			"spk": {Directions: []int{1, 2, 3}, Frames: []int{0, 1}},
			"sml": {Directions: []int{1, 2, 3}, Frames: []int{0}},
			"agr": {Directions: []int{1, 2, 3}, Frames: []int{0}},
			"srp": {Directions: []int{1, 2, 3}, Frames: []int{0}},
			"sad": {Directions: []int{1, 2, 3}, Frames: []int{0}},
		},
		"ey": { // Eyes
			"std": {Directions: []int{1, 2, 3}, Frames: []int{0}},
			"sml": {Directions: []int{1, 2, 3}, Frames: []int{0}},
			"agr": {Directions: []int{1, 2, 3}, Frames: []int{0}},
			"srp": {Directions: []int{1, 2, 3}, Frames: []int{0}},
			"sad": {Directions: []int{1, 2, 3}, Frames: []int{0}},
			"eyb": {Directions: []int{1, 2, 3}, Frames: []int{0}},
		},
		"hr": { // Hair
			"std": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
			"spk": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
		},
		"lg": { // Legs
			"std": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
			"wlk": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0, 1, 2, 3}},
			"sit": {Directions: []int{0, 2}, Frames: []int{0}},
		},
		"sh": { // Shoes
			"std": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
			"wlk": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0, 1, 2, 3}},
			"sit": {Directions: []int{0, 2}, Frames: []int{0}},
		},
		"ch": { // Chest/Shirt
			"std": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
		},
		"lh": { // Left Hand
			"std": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
			"wlk": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{1, 3}},
			"crr": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
			"drk": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
		},
		"rh": { // Right Hand
			"std": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
			"wlk": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{1, 3}},
			"crr": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
			"drk": {Directions: []int{0, 1, 2, 3, 7}, Frames: []int{0}},
		},
	}

	for bodyPart, actions := range bodyParts {
		c.BodyParts[bodyPart] = BodyPartConfig{Actions: actions}
	}
}

// IsValidAction checks if an action is valid for a body part
func (c *AvatarConfig) IsValidAction(bodyPart, action string) bool {
	if partConfig, exists := c.BodyParts[bodyPart]; exists {
		_, actionExists := partConfig.Actions[action]
		return actionExists
	}
	return false
}

// IsValidDirection checks if a direction is valid for a body part and action
func (c *AvatarConfig) IsValidDirection(bodyPart, action string, direction int) bool {
	if partConfig, exists := c.BodyParts[bodyPart]; exists {
		if actionConfig, actionExists := partConfig.Actions[action]; actionExists {
			for _, validDir := range actionConfig.Directions {
				if validDir == direction {
					return true
				}
			}
		}
	}
	return false
}

// IsValidFrame checks if a frame is valid for a body part and action
func (c *AvatarConfig) IsValidFrame(bodyPart, action string, frame int) bool {
	if partConfig, exists := c.BodyParts[bodyPart]; exists {
		if actionConfig, actionExists := partConfig.Actions[action]; actionExists {
			for _, validFrame := range actionConfig.Frames {
				if validFrame == frame {
					return true
				}
			}
		}
	}
	return false
}

// GetValidDirections returns all valid directions for a body part and action
func (c *AvatarConfig) GetValidDirections(bodyPart, action string) []int {
	if partConfig, exists := c.BodyParts[bodyPart]; exists {
		if actionConfig, actionExists := partConfig.Actions[action]; actionExists {
			return actionConfig.Directions
		}
	}
	return []int{0, 1, 2, 3, 7} // Default all directions
}

// GetValidFrames returns all valid frames for a body part and action
func (c *AvatarConfig) GetValidFrames(bodyPart, action string) []int {
	if partConfig, exists := c.BodyParts[bodyPart]; exists {
		if actionConfig, actionExists := partConfig.Actions[action]; actionExists {
			return actionConfig.Frames
		}
	}
	return []int{0} // Default single frame
}

// getConfiguredBodyParts returns a list of configured body parts for debugging
func (c *AvatarConfig) getConfiguredBodyParts() []string {
	var bodyParts []string
	for bodyPart := range c.BodyParts {
		bodyParts = append(bodyParts, bodyPart)
	}
	return bodyParts
}
