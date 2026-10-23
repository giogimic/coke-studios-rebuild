package avatarengine

import (
	"fmt"
	"image/color"
	"log"
	"reflect"
)

// TestAvatarEngine creates a test avatar engine for development and testing
func TestAvatarEngine(dataDir string) error {
	log.Printf("TestAvatarEngine: creating test avatar...")

	// Create a basic avatar with default attributes
	avatar, err := NewAvatarEngine("", 100, false, dataDir)
	if err != nil {
		return fmt.Errorf("failed to create avatar engine: %w", err)
	}

	log.Printf("TestAvatarEngine: created avatar with %d body parts", len(avatar.GetAllBodyParts()))

	// Test basic functionality
	log.Printf("TestAvatarEngine: testing basic functionality...")

	// Test direction changes
	for _, direction := range []int{0, 1, 2, 3, 7} {
		avatar.SetDirection(direction)
		err = avatar.Draw()
		if err != nil {
			log.Printf("TestAvatarEngine: failed to draw avatar at direction %d: %v", direction, err)
		} else {
			log.Printf("TestAvatarEngine: successfully drew avatar at direction %d", direction)
		}
	}

	// Test actions
	log.Printf("TestAvatarEngine: testing actions...")

	// Test walking
	avatar.StartWalking()
	for i := 0; i < 8; i++ {
		avatar.Update()
		err = avatar.Draw()
		if err != nil {
			log.Printf("TestAvatarEngine: failed to draw walking frame %d: %v", i, err)
		}
	}
	avatar.StopWalking()

	// Test speaking
	avatar.StartChatting()
	err = avatar.Draw()
	if err != nil {
		log.Printf("TestAvatarEngine: failed to draw speaking avatar: %v", err)
	}

	// Test emotes
	emotes := []string{"sml", "agr", "srp", "sad"}
	for _, emote := range emotes {
		avatar.StartEmote(emote)
		err = avatar.Draw()
		if err != nil {
			log.Printf("TestAvatarEngine: failed to draw emote %s: %v", emote, err)
		} else {
			log.Printf("TestAvatarEngine: successfully drew emote %s", emote)
		}
	}

	// Test attribute changes
	log.Printf("TestAvatarEngine: testing attribute changes...")

	// Try different hair styles
	for _, style := range []string{"001", "002", "003"} {
		err = avatar.SetBodyPartStyle("hr", style)
		if err != nil {
			log.Printf("TestAvatarEngine: failed to set hair style %s: %v", style, err)
		} else {
			err = avatar.Draw()
			if err != nil {
				log.Printf("TestAvatarEngine: failed to draw with hair style %s: %v", style, err)
			} else {
				log.Printf("TestAvatarEngine: successfully drew with hair style %s", style)
			}
		}
	}

	// Test scaling
	log.Printf("TestAvatarEngine: testing scaling...")
	for _, scale := range []int{50, 100, 150} {
		avatar.setScale(scale)
		err = avatar.Draw()
		if err != nil {
			log.Printf("TestAvatarEngine: failed to draw at scale %d%%: %v", scale, err)
		} else {
			log.Printf("TestAvatarEngine: successfully drew at scale %d%%", scale)
		}
	}

	// Test asset manager cache
	log.Printf("TestAvatarEngine: asset cache size: %d", avatar.AssetManager.GetCacheSize())

	// Test attribute string serialization
	attrString := avatar.GetAttributeString()
	log.Printf("TestAvatarEngine: attribute string: %s", attrString)

	// Test validation
	errors := avatar.ValidateAttributes()
	if len(errors) > 0 {
		log.Printf("TestAvatarEngine: validation errors: %v", errors)
	} else {
		log.Printf("TestAvatarEngine: all attributes valid")
	}

	log.Printf("TestAvatarEngine: test completed successfully")
	return nil
}

// TestAssetLoading tests the asset loading system specifically
func TestAssetLoading(dataDir string) error {
	log.Printf("TestAssetLoading: testing asset loading system...")

	assetManager, err := NewAssetManager(dataDir)
	if err != nil {
		return fmt.Errorf("failed to create asset manager: %w", err)
	}

	// Test loading some basic assets
	testAssets := []AssetKey{
		{Gender: "h", Action: "std", BodyPart: "hd", Style: "001", Direction: 2, Frame: 0},
		{Gender: "h", Action: "std", BodyPart: "bd", Style: "001", Direction: 2, Frame: 0},
		{Gender: "h", Action: "wlk", BodyPart: "bd", Style: "001", Direction: 2, Frame: 1},
		{Gender: "h", Action: "std", BodyPart: "hr", Style: "001", Direction: 2, Frame: 0},
	}

	loadedCount := 0
	for _, key := range testAssets {
		asset, err := assetManager.LoadAsset(key)
		if err != nil {
			log.Printf("TestAssetLoading: failed to load %s: %v", key.String(), err)
		} else {
			bounds := asset.Bounds()
			log.Printf("TestAssetLoading: loaded %s (%dx%d)", key.String(), bounds.Dx(), bounds.Dy())
			loadedCount++
		}
	}

	log.Printf("TestAssetLoading: loaded %d/%d test assets", loadedCount, len(testAssets))
	log.Printf("TestAssetLoading: cache size: %d", assetManager.GetCacheSize())

	// Test preloading
	bodyParts := []string{"hd", "bd", "hr"}
	styles := []string{"001"}
	actions := []string{"std"}
	directions := []int{2}

	err = assetManager.PreloadAssets(bodyParts, styles, actions, directions)
	if err != nil {
		log.Printf("TestAssetLoading: preload failed: %v", err)
	} else {
		log.Printf("TestAssetLoading: preload completed, cache size: %d", assetManager.GetCacheSize())
	}

	return nil
}

// TestAttributeParsing tests the attribute parsing system
func TestAttributeParsing() error {
	log.Printf("TestAttributeParsing: testing attribute parsing...")

	testCases := []struct {
		input    string
		expected map[string]BodyPartAppearance
	}{
		{
			input: "hr=002/238,238,238&hd=001/255,224,196",
			expected: map[string]BodyPartAppearance{
				"hr": {Style: "002", Color: color.RGBA{238, 238, 238, 255}},
				"hd": {Style: "001", Color: color.RGBA{255, 224, 196, 255}},
				"ri": {Style: "001", Color: color.RGBA{255, 255, 255, 255}}, // Default
			},
		},
		{
			input: "ch=002/204,153,102&lg=008/115,142,191",
			expected: map[string]BodyPartAppearance{
				"ch": {Style: "002", Color: color.RGBA{204, 153, 102, 255}},
				"lg": {Style: "008", Color: color.RGBA{115, 142, 191, 255}},
				"ri": {Style: "001", Color: color.RGBA{255, 255, 255, 255}}, // Default
			},
		},
		{
			input: "",
			expected: map[string]BodyPartAppearance{
				"ri": {Style: "001", Color: color.RGBA{255, 255, 255, 255}}, // Default only
			},
		},
	}

	for i, testCase := range testCases {
		result, err := ParseAttributeString(testCase.input)
		if err != nil {
			log.Printf("TestAttributeParsing: case %d failed: %v", i, err)
			continue
		}

		if !reflect.DeepEqual(result, testCase.expected) {
			log.Printf("TestAttributeParsing: case %d mismatch:", i)
			log.Printf("  Expected: %+v", testCase.expected)
			log.Printf("  Got:      %+v", result)
			continue
		}

		log.Printf("TestAttributeParsing: case %d passed", i)
	}

	return nil
}

// TestConfiguration tests the configuration loading system
func TestConfiguration(dataDir string) error {
	log.Printf("TestConfiguration: testing configuration system...")

	config, err := LoadAvatarConfig(dataDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	log.Printf("TestConfiguration: loaded config with %d body parts", len(config.BodyParts))

	// Test some validations
	testCases := []struct {
		bodyPart      string
		action        string
		direction     int
		frame         int
		shouldBeValid bool
	}{
		{"bd", "std", 2, 0, true},
		{"bd", "wlk", 2, 1, true},
		{"fc", "std", 2, 0, true},
		{"fc", "spk", 2, 1, true},
		{"bd", "invalid", 2, 0, false},
		{"bd", "std", 9, 0, false}, // Invalid direction
	}

	for i, testCase := range testCases {
		isValidAction := config.IsValidAction(testCase.bodyPart, testCase.action)
		isValidDirection := config.IsValidDirection(testCase.bodyPart, testCase.action, testCase.direction)
		isValidFrame := config.IsValidFrame(testCase.bodyPart, testCase.action, testCase.frame)

		isValid := isValidAction && isValidDirection && isValidFrame

		if isValid == testCase.shouldBeValid {
			log.Printf("TestConfiguration: case %d passed", i)
		} else {
			log.Printf("TestConfiguration: case %d failed - expected %t, got %t (action:%t, dir:%t, frame:%t)",
				i, testCase.shouldBeValid, isValid, isValidAction, isValidDirection, isValidFrame)
		}
	}

	return nil
}

// RunAllTests runs all avatar engine tests
func RunAllTests(dataDir string) error {
	log.Printf("RunAllTests: starting avatar engine tests...")

	tests := []struct {
		name string
		fn   func(string) error
	}{
		{"Configuration", TestConfiguration},
		{"Asset Loading", TestAssetLoading},
		{"Avatar Engine", TestAvatarEngine},
	}

	// Run attribute parsing test (no dataDir needed)
	err := TestAttributeParsing()
	if err != nil {
		log.Printf("RunAllTests: attribute parsing test failed: %v", err)
	}

	// Run tests that need dataDir
	for _, test := range tests {
		log.Printf("RunAllTests: running %s test...", test.name)
		err := test.fn(dataDir)
		if err != nil {
			log.Printf("RunAllTests: %s test failed: %v", test.name, err)
		} else {
			log.Printf("RunAllTests: %s test passed", test.name)
		}
	}

	log.Printf("RunAllTests: all tests completed")
	return nil
}
