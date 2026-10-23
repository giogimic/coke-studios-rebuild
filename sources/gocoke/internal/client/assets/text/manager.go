package textmgr

import (
	"bufio"
	"path/filepath"
	"strings"

	"gocoke/internal/client/fs"
)

// Manager loads simple KEY=Value text resources from Text_English.txt.
// - Supports duplicate keys by storing all values; First returns the first.
// - Ignores blank lines and section headers like: [ Section Name ]
// - No variable interpolation yet.

type Manager struct {
	values map[string][]string
}

func New() *Manager { return &Manager{values: make(map[string][]string)} }

func (m *Manager) LoadFile(path string) error {
	f, err := fs.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			continue
		}
		// Expect KEY=Value
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		m.values[k] = append(m.values[k], v)
	}
	return s.Err()
}

// First returns the first value for a key or an empty string.
func (m *Manager) First(key string) string {
	vs := m.values[key]
	if len(vs) == 0 {
		return ""
	}
	return vs[0]
}

// All returns all values for a key (may be empty).
func (m *Manager) All(key string) []string { return m.values[key] }
