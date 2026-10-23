package studios

import (
	"encoding/xml"
	"path/filepath"
	"strconv"

	"gocoke/internal/client/fs"
)

// Studio represents a studio from PublicStudios_English.xml
type Studio struct {
	StudioID    string `xml:"studioId,attr"`
	Name        string `xml:"name,attr"`
	Description string `xml:"description,attr"`
	UserCount   int    `xml:"userCount,attr"`
	Capacity    int    `xml:"capacity,attr"`
	Location    int    `xml:"location,attr"`
	Layout      int    `xml:"layout,attr"`
	Owner       string `xml:"owner,attr"`
	Server      string `xml:"server,attr"`
}

// StudioList represents the parsed PublicStudios_English.xml file
type StudioList struct {
	XMLName xml.Name `xml:"PublicStudios"`
	Studios []Studio `xml:"Studio"`
}

// New creates a new empty StudioList
func New() *StudioList {
	return &StudioList{
		Studios: make([]Studio, 0),
	}
}

// AddHardcodedPrivateStudios adds hardcoded private studios for testing
func (sl *StudioList) AddHardcodedPrivateStudios() {
	privateStudios := []Studio{
		{
			StudioID:    "testuser$1",
			Name:        "Test Studio A",
			Description: "Private studio with model A layout for testing",
			UserCount:   0,
			Capacity:    25,
			Location:    1, // London
			Layout:      1, // Model A
			Owner:       "testuser",
			Server:      "",
		},
		{
			StudioID:    "ADAM$2",
			Name:        "Good - Test Studio B",
			Description: "Private studio with model B layout for testing",
			UserCount:   0,
			Capacity:    25,
			Location:    2, // Miami
			Layout:      2, // Model B
			Owner:       "Adam",
			Server:      "",
		},
		{
			StudioID:    "testuser$3",
			Name:        "Test Studio C",
			Description: "Private studio with model C layout for testing",
			UserCount:   0,
			Capacity:    25,
			Location:    3, // Seattle
			Layout:      3, // Model C
			Owner:       "testuser",
			Server:      "",
		},
		{
			StudioID:    "testuser$4",
			Name:        "Test Studio D",
			Description: "Private studio with model D layout for testing",
			UserCount:   0,
			Capacity:    25,
			Location:    4, // Sydney
			Layout:      4, // Model D
			Owner:       "testuser",
			Server:      "",
		},
	}

	sl.Studios = append(sl.Studios, privateStudios...)
}

// LoadFile parses PublicStudios_English.xml format
func LoadFile(path string) (*StudioList, error) {
	f, err := fs.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var studioList StudioList
	decoder := xml.NewDecoder(f)

	err = decoder.Decode(&studioList)
	if err != nil {
		return nil, err
	}

	return &studioList, nil
}

// GetByLocation returns all studios for a specific location
func (sl *StudioList) GetByLocation(location int) []Studio {
	var studios []Studio
	for _, studio := range sl.Studios {
		if studio.Location == location {
			studios = append(studios, studio)
		}
	}
	return studios
}

// GetByID returns a studio by its ID
func (sl *StudioList) GetByID(studioID string) *Studio {
	for i := range sl.Studios {
		if sl.Studios[i].StudioID == studioID {
			return &sl.Studios[i]
		}
	}
	return nil
}

// GetSystemStudios returns all system-owned studios (public studios)
func (sl *StudioList) GetSystemStudios() []Studio {
	var studios []Studio
	for _, studio := range sl.Studios {
		if studio.Owner == "$SYSTEM$" {
			studios = append(studios, studio)
		}
	}
	return studios
}

// GetByOwner returns all studios owned by a specific user
func (sl *StudioList) GetByOwner(owner string) []Studio {
	var studios []Studio
	for _, studio := range sl.Studios {
		if studio.Owner == owner {
			studios = append(studios, studio)
		}
	}
	return studios
}

// LocationNames maps location IDs to location names
// Based on the studio names in the data and extended from test output
var LocationNames = map[int]string{
	1:  "London",
	2:  "Miami",
	3:  "Mombasa",
	4:  "New York",
	5:  "Rio de Janeiro",
	6:  "Seattle",
	7:  "Sydney",
	8:  "Tokyo",
	9:  "Goa",
	10: "Alaska",
	11: "Moscow",
	12: "Coca-Cola Red Room",
	13: "San Francisco",
	14: "Central Park",
	15: "New Orleans",
	16: "Location 16", // Additional locations found in data
	17: "Location 17",
	18: "Location 18",
	19: "Location 19",
	20: "Location 20",
	22: "Location 22",
	23: "Location 23",
	26: "Location 26",
	27: "Location 27",
	29: "Location 29",
	30: "Coca-Cola Red Room", // Special location
}

// GetLocationName returns the human-readable name for a location ID
func GetLocationName(locationID int) string {
	if name, exists := LocationNames[locationID]; exists {
		return name
	}
	return "Unknown Location " + strconv.Itoa(locationID)
}

// GetUniqueLocations returns a list of all unique location IDs
func (sl *StudioList) GetUniqueLocations() []int {
	locationSet := make(map[int]bool)
	for _, studio := range sl.Studios {
		locationSet[studio.Location] = true
	}

	var locations []int
	for location := range locationSet {
		locations = append(locations, location)
	}
	return locations
}
