package window

import (
	"bytes"
	"io"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gocoke/internal/client/fs"
)

type Element struct {
	Member    string
	Media     string
	LocH      int
	LocV      int
	Width     int
	Height    int
	ID        string
	Type      string
	Ink       int
	Blend     int
	Font      string
	FontSize  int
	FontStyle string
	TxtColor  string
	Alignment string
	Key       string
	Editable  int
	AutoTab   int
	BoxType   string
	WordWrap  int
	Strech    string // Lingo stretch attribute: #fixed, #strechH, #strechV, #moveH, #moveV, #moveHV
	// Additional navigator/window metadata
	Client string // for scrollbars: maps to target list id (e.g., nav_public_rooms_list)
	Model  int    // scrollbar model: 1=button, 3=mms
	Offset int    // scrollbar offset (e.g., 14)
}

type Definition struct {
	Name       string
	Rect       [4]int
	ClientRect [4]int
	Elements   []Element
	Path       string
}

var (
	reMember = regexp.MustCompile(`#member:\s*"([^"]+)"`)
	reMedia  = regexp.MustCompile(`#media:\s*#([a-zA-Z]+)`) // bitmap, field
	reIntKV  = regexp.MustCompile(`#(locH|locV|width|height|ink|blend|fontSize|editable|autoTab|wordWrap):\s*([0-9]+)`)
	reID     = regexp.MustCompile(`#id:\s*"([^"]+)"`)
	reType   = regexp.MustCompile(`#type:\s*"([^"]+)"`)
	reFont   = regexp.MustCompile(`#font:\s*"([^"]+)"`)
	// Support both quoted fontStyle and array token syntax like [#bold]
	reFStyle = regexp.MustCompile(`#fontStyle:\s*(?:\"([^\"]+)\"|\[\s*#([a-zA-Z]+)\s*\])`)
	reTxtCol = regexp.MustCompile(`#txtColor:\s*"(#[0-9A-Fa-f]{6})"`)
	// Support quoted alignment (rare) and token syntax like #center/#left/#right
	reAlignment  = regexp.MustCompile(`#alignment:\s*(?:\"([^\"]+)\"|#([a-zA-Z]+))`)
	reKey        = regexp.MustCompile(`#key:\s*"([^"]*)"`)
	reBoxType    = regexp.MustCompile(`#boxType:\s*#([a-zA-Z]+)`)
	reStrech     = regexp.MustCompile(`#strech:\s*#([a-zA-Z]+)`) // fixed, strechH, strechV, moveH, moveV, moveHV
	reClient     = regexp.MustCompile(`#client:\s*"([^"]+)"`)
	reModel      = regexp.MustCompile(`#model:\s*([0-9]+)`)
	reOffset     = regexp.MustCompile(`#offset:\s*([0-9]+)`)
	reRect       = regexp.MustCompile(`\[\s*([0-9]+),\s*([0-9]+),\s*([0-9]+),\s*([0-9]+)\s*\]`)
	reClientRect = regexp.MustCompile(`rect\(\s*([0-9]+),\s*([0-9]+),\s*([0-9]+),\s*([0-9]+)\s*\)`)
)

func ParseFile(path string) (*Definition, error) {
	f, err := fs.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read the entire file and normalize line endings to \n
	content, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	content = bytes.ReplaceAll(content, []byte("\r"), []byte("\n"))
	lines := strings.Split(string(content), "\n")

	def := &Definition{Path: path}
	inElements := false
	inRect := false
	inClientRect := false

	var currentElementBuffer string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		if strings.HasPrefix(trimmedLine, "<elements>") {
			inElements = true
			continue
		}
		if strings.HasPrefix(trimmedLine, "</elements>") {
			inElements = false
			continue
		}
		if strings.HasPrefix(trimmedLine, "<rect>") {
			inRect = true
			log.Printf("Window parser: entering rect section")
			continue
		}
		if strings.HasPrefix(trimmedLine, "</rect>") {
			inRect = false
			log.Printf("Window parser: exiting rect section")
			continue
		}
		if strings.HasPrefix(trimmedLine, "<clientrect>") {
			inClientRect = true
			log.Printf("Window parser: entering clientrect section")
			continue
		}
		if strings.HasPrefix(trimmedLine, "</clientrect>") {
			inClientRect = false
			log.Printf("Window parser: exiting clientrect section")
			continue
		}

		if inElements {
			if strings.HasPrefix(trimmedLine, "[") {
				// Start of a new element
				if currentElementBuffer != "" {
					// This should not happen if files are well-formed, but handle it
					log.Printf("Window parser: warning - new element started before previous was finalized.")
				}
				currentElementBuffer = trimmedLine
			} else {
				// Continuation of the current element
				currentElementBuffer += " " + trimmedLine
			}

			if strings.HasSuffix(trimmedLine, "]") {
				// End of an element definition, parse the buffer
				el := parseElement(currentElementBuffer)
				def.Elements = append(def.Elements, el)
				log.Printf("  -> Parsed element: Member=%-30s ID=%-20s Type=%-10s Loc=(%4d, %4d) Size=(%4d, %4d) Strech=%-15s Blend=%-4d",
					el.Member, el.ID, el.Type, el.LocH, el.LocV, el.Width, el.Height, el.Strech, el.Blend)
				currentElementBuffer = "" // Reset buffer
			}
			continue
		}

		if inRect {
			log.Printf("Window parser: parsing rect line: %s", trimmedLine)
			if m := reRect.FindStringSubmatch(trimmedLine); m != nil {
				for i := 0; i < 4; i++ {
					def.Rect[i], _ = strconv.Atoi(m[i+1])
				}
				log.Printf("Window parser: parsed rect: [%d, %d, %d, %d]", def.Rect[0], def.Rect[1], def.Rect[2], def.Rect[3])
			}
			continue
		}

		if inClientRect {
			if m := reClientRect.FindStringSubmatch(trimmedLine); m != nil {
				for i := 0; i < 4; i++ {
					def.ClientRect[i], _ = strconv.Atoi(m[i+1])
				}
				log.Printf("Window parser: parsed clientrect: rect(%d, %d, %d, %d)", def.ClientRect[0], def.ClientRect[1], def.ClientRect[2], def.ClientRect[3])
			}
			continue
		}
	}
	return def, nil
}

func parseElement(line string) Element {
	el := Element{}
	if m := reMember.FindStringSubmatch(line); m != nil {
		el.Member = m[1]
	}
	if m := reMedia.FindStringSubmatch(line); m != nil {
		el.Media = m[1]
	}
	for _, m := range reIntKV.FindAllStringSubmatch(line, -1) {
		v, _ := strconv.Atoi(m[2])
		switch m[1] {
		case "locH":
			el.LocH = v
		case "locV":
			el.LocV = v
		case "width":
			el.Width = v
		case "height":
			el.Height = v
		case "ink":
			el.Ink = v
		case "blend":
			el.Blend = v
		case "fontSize":
			el.FontSize = v
		case "editable":
			el.Editable = v
		case "autoTab":
			el.AutoTab = v
		case "wordWrap":
			el.WordWrap = v
		}
	}

	// Extra attributes for navigator controls
	if m := reClient.FindStringSubmatch(line); m != nil {
		el.Client = m[1]
	}
	if m := reModel.FindStringSubmatch(line); m != nil {
		el.Model, _ = strconv.Atoi(m[1])
	}
	if m := reOffset.FindStringSubmatch(line); m != nil {
		el.Offset, _ = strconv.Atoi(m[1])
	}

	if m := reID.FindStringSubmatch(line); m != nil {
		el.ID = m[1]
	}
	if m := reType.FindStringSubmatch(line); m != nil {
		el.Type = m[1]
	}
	if m := reFont.FindStringSubmatch(line); m != nil {
		el.Font = m[1]
	}
	if m := reFStyle.FindStringSubmatch(line); m != nil {
		if m[1] != "" {
			el.FontStyle = m[1]
		} else if m[2] != "" {
			el.FontStyle = m[2]
		}
	}
	if m := reTxtCol.FindStringSubmatch(line); m != nil {
		el.TxtColor = m[1]
	}
	if m := reAlignment.FindStringSubmatch(line); m != nil {
		if m[1] != "" {
			el.Alignment = m[1]
		} else if m[2] != "" {
			el.Alignment = m[2]
		}
	}
	if m := reKey.FindStringSubmatch(line); m != nil {
		el.Key = m[1]
	}
	if m := reBoxType.FindStringSubmatch(line); m != nil {
		el.BoxType = m[1]
	}
	if m := reStrech.FindStringSubmatch(line); m != nil {
		el.Strech = m[1]
	}
	return el
}
