package lingoassets

import (
	"encoding/csv"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"gocoke/internal/client/fs"
)

// MemberMap maps member name -> absolute PNG path for bitmaps.
type MemberMap struct {
	byName map[string]string
}

func LoadMemberMap(dir string) (*MemberMap, error) {
	f, err := fs.Open(filepath.Join(dir, "Members.csv"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	mm := &MemberMap{byName: make(map[string]string)}
	for i, rec := range records {
		if i == 0 { // header
			continue
		}
		if len(rec) < 5 {
			continue
		}
		idStr := rec[0]
		typeStr := rec[1]
		name := rec[2]
		_ = typeStr
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}
		png := filepath.Join(dir, fmt.Sprintf("%d_%s.png", id, name))
		// Normalize to forward slashes for web URLs
		png = strings.ReplaceAll(png, "\\", "/")
		mm.byName[name] = png
	}
	return mm, nil
}

func (m *MemberMap) PathFor(name string) (string, bool) {
	p, ok := m.byName[name]
	return p, ok
}
