package studioroom

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gocoke/internal/client/fs"
)

// RoomElement represents a single element in a studio room
type RoomElement struct {
	Member  string // Asset member name
	Media   string // Media type (#bitmap, etc.)
	LocH    int    // Horizontal location
	LocV    int    // Vertical location
	LocZ    int    // Z-order depth
	Ink     int    // Ink mode
	Blend   int    // Blend percentage
	Width   int    // Element width
	Height  int    // Element height
	ID      string // Element ID
	Palette string // Palette name (for floor textures)
	BGColor string // Background color
	Active  int    // Active state (0 or 1)
	Type    string // Element type (e.g., "hiliter")
}

// RoomData represents the room positioning data
type RoomData struct {
	OffsetX int // X offset for positioning
	OffsetY int // Y offset for positioning
	FactorX int // X scaling factor
	FactorY int // Y scaling factor
	FactorH int // Height scaling factor
}

// RoomDefinition represents a complete studio room definition
type RoomDefinition struct {
	Version  string        // Room definition version
	Name     string        // Room model name (e.g., "model_a")
	Elements []RoomElement // All room elements
	Rect     [4]int        // Room rectangle [left, top, right, bottom]
	RoomData RoomData      // Room positioning data
}

// RegistrationPoint represents a member's registration point from Members.csv
type RegistrationPoint struct {
	Number   int    // Member number
	Type     string // Member type (bitmap, field, etc.)
	Name     string // Member name
	RegPoint [2]int // Registration point [x, y]
	Filename string // Associated filename
}

// MemberRegistry holds all registration points from Members.csv
type MemberRegistry struct {
	Members map[string]RegistrationPoint // Map by member name
}

// ParseRoomFile parses a studio room definition file
func ParseRoomFile(path string) (*RoomDefinition, error) {
	file, err := fs.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("failed to open room file %s: %w", path, err)
	}
	defer file.Close()

	room := &RoomDefinition{}
	scanner := bufio.NewScanner(file)

	// Regular expressions for parsing
	versionRe := regexp.MustCompile(`^\s*([0-9.]+)\s*$`)
	nameRe := regexp.MustCompile(`^\s*"([^"]+)"\s*$`)
	elementRe := regexp.MustCompile(`^\s*\[(.+)\]\s*$`)
	rectRe := regexp.MustCompile(`^\s*\[([0-9, ]+)\]\s*$`)
	roomDataRe := regexp.MustCompile(`^\s*\[(.+)\]\s*$`)

	var currentSection string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "<!--") {
			continue
		}

		// Detect sections
		if strings.Contains(line, "<version>") {
			currentSection = "version"
			continue
		} else if strings.Contains(line, "<name>") {
			currentSection = "name"
			continue
		} else if strings.Contains(line, "<elements>") {
			currentSection = "elements"
			continue
		} else if strings.Contains(line, "<rect>") {
			currentSection = "rect"
			continue
		} else if strings.Contains(line, "<roomdata>") {
			currentSection = "roomdata"
			continue
		} else if strings.HasPrefix(line, "</") {
			currentSection = ""
			continue
		}

		// Parse content based on current section
		switch currentSection {
		case "version":
			if matches := versionRe.FindStringSubmatch(line); matches != nil {
				room.Version = matches[1]
			}
		case "name":
			if matches := nameRe.FindStringSubmatch(line); matches != nil {
				room.Name = matches[1]
			}
		case "elements":
			if matches := elementRe.FindStringSubmatch(line); matches != nil {
				element, err := parseElement(matches[1])
				if err != nil {
					return nil, fmt.Errorf("failed to parse element: %w", err)
				}
				room.Elements = append(room.Elements, element)
			}
		case "rect":
			if matches := rectRe.FindStringSubmatch(line); matches != nil {
				rect, err := parseRect(matches[1])
				if err != nil {
					return nil, fmt.Errorf("failed to parse rect: %w", err)
				}
				room.Rect = rect
			}
		case "roomdata":
			if matches := roomDataRe.FindStringSubmatch(line); matches != nil {
				roomData, err := parseRoomData(matches[1])
				if err != nil {
					return nil, fmt.Errorf("failed to parse roomdata: %w", err)
				}
				room.RoomData = roomData
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading room file: %w", err)
	}

	return room, nil
}

// parseElement parses a single element definition
func parseElement(elementStr string) (RoomElement, error) {
	element := RoomElement{
		Active: 1,
		Blend:  100,
	}

	// Parse key-value pairs from the element string
	// Format: #member: "value", #media: #bitmap, #locH: 123, etc.
	pairs := strings.Split(elementStr, ",")

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if !strings.Contains(pair, ":") {
			continue
		}

		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove # prefix from key
		key = strings.TrimPrefix(key, "#")

		// Remove quotes from string values
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = value[1 : len(value)-1]
		}

		// Remove # prefix from symbol values
		value = strings.TrimPrefix(value, "#")

		// Parse based on key
		switch key {
		case "member":
			element.Member = value
		case "media":
			element.Media = value
		case "locH":
			if v, err := strconv.Atoi(value); err == nil {
				element.LocH = v
			}
		case "locV":
			if v, err := strconv.Atoi(value); err == nil {
				element.LocV = v
			}
		case "locZ":
			if v, err := strconv.Atoi(value); err == nil {
				element.LocZ = v
			}
		case "ink":
			if v, err := strconv.Atoi(value); err == nil {
				element.Ink = v
			}
		case "blend":
			if v, err := strconv.Atoi(value); err == nil {
				element.Blend = v
			}
		case "width":
			if v, err := strconv.Atoi(value); err == nil {
				element.Width = v
			}
		case "height":
			if v, err := strconv.Atoi(value); err == nil {
				element.Height = v
			}
		case "id":
			element.ID = value
		case "palette":
			element.Palette = value
		case "bgColor":
			element.BGColor = value
		case "Active":
			if v, err := strconv.Atoi(value); err == nil {
				element.Active = v
			}
		case "type":
			element.Type = value
		}
	}

	return element, nil
}

// parseRect parses a rectangle definition
func parseRect(rectStr string) ([4]int, error) {
	var rect [4]int
	parts := strings.Split(rectStr, ",")

	if len(parts) != 4 {
		return rect, fmt.Errorf("invalid rect format, expected 4 values")
	}

	for i, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return rect, fmt.Errorf("invalid rect value: %s", part)
		}
		rect[i] = value
	}

	return rect, nil
}

// parseRoomData parses room data definition
func parseRoomData(roomDataStr string) (RoomData, error) {
	roomData := RoomData{}

	// Parse key-value pairs similar to elements
	pairs := strings.Split(roomDataStr, ",")

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if !strings.Contains(pair, ":") {
			continue
		}

		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove # prefix from key
		key = strings.TrimPrefix(key, "#")

		// Parse based on key
		switch key {
		case "offsetx":
			if v, err := strconv.Atoi(value); err == nil {
				roomData.OffsetX = v
			}
		case "offsety":
			if v, err := strconv.Atoi(value); err == nil {
				roomData.OffsetY = v
			}
		case "factorx":
			if v, err := strconv.Atoi(value); err == nil {
				roomData.FactorX = v
			}
		case "factory":
			if v, err := strconv.Atoi(value); err == nil {
				roomData.FactorY = v
			}
		case "factorh":
			if v, err := strconv.Atoi(value); err == nil {
				roomData.FactorH = v
			}
		}
	}

	return roomData, nil
}

// LoadMemberRegistry loads registration points from Members.csv
func LoadMemberRegistry(path string) (*MemberRegistry, error) {
	file, err := fs.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("failed to open members file %s: %w", path, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	registry := &MemberRegistry{
		Members: make(map[string]RegistrationPoint),
	}

	// Skip header row
	for i := 1; i < len(records); i++ {
		record := records[i]
		if len(record) < 4 {
			continue
		}

		number, err := strconv.Atoi(record[0])
		if err != nil {
			continue
		}

		// Parse registration point
		regPointStr := record[3]
		regPoint := [2]int{0, 0}

		// Format: "(x, y)"
		regPointStr = strings.Trim(regPointStr, "()")
		parts := strings.Split(regPointStr, ",")
		if len(parts) == 2 {
			if x, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
				regPoint[0] = x
			}
			if y, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
				regPoint[1] = y
			}
		}

		filename := ""
		if len(record) > 4 {
			filename = record[4]
		}

		member := RegistrationPoint{
			Number:   number,
			Type:     record[1],
			Name:     record[2],
			RegPoint: regPoint,
			Filename: filename,
		}

		registry.Members[record[2]] = member
	}

	return registry, nil
}

// GetRegistrationPoint returns the registration point for a member
func (mr *MemberRegistry) GetRegistrationPoint(memberName string) ([2]int, bool) {
	if member, exists := mr.Members[memberName]; exists {
		return member.RegPoint, true
	}
	return [2]int{0, 0}, false
}
