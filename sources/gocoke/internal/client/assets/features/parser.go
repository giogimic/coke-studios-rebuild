package features

import (
	"bufio"
	"path/filepath"
	"strconv"
	"strings"

	"gocoke/internal/client/fs"
)

// FeatureSet represents the parsed FeatureSet.txt file
// Mirrors the original Lingo gFeatureSet.isEnabled(#BETA) functionality
type FeatureSet struct {
	features map[string]bool
}

// New creates a new empty FeatureSet
func New() *FeatureSet {
	return &FeatureSet{
		features: make(map[string]bool),
	}
}

// LoadFile parses FeatureSet.txt format: FEATURE_NAME=1/0
func LoadFile(path string) (*FeatureSet, error) {
	f, err := fs.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fs := New()
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}

		feature := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		// Parse value as integer (1=enabled, 0=disabled)
		value, err := strconv.Atoi(valueStr)
		if err != nil {
			continue // Skip invalid values
		}

		fs.features[feature] = value == 1
	}

	return fs, scanner.Err()
}

// IsEnabled checks if a feature is enabled
// Mirrors original Lingo: gFeatureSet.isEnabled(#BETA)
func (fs *FeatureSet) IsEnabled(feature string) bool {
	enabled, exists := fs.features[feature]
	return exists && enabled
}

// GetAll returns all features and their states
func (fs *FeatureSet) GetAll() map[string]bool {
	result := make(map[string]bool)
	for k, v := range fs.features {
		result[k] = v
	}
	return result
}

// Set enables or disables a feature (for testing/debugging)
func (fs *FeatureSet) Set(feature string, enabled bool) {
	fs.features[feature] = enabled
}
