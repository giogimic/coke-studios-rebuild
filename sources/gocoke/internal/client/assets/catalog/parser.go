package catalog

import (
	"bufio"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gocoke/internal/client/fs"
)

// Page represents a catalog page/category
type Page struct {
	Page    string
	CatID   string
	CatName string
}

// Item represents a catalog item
type Item struct {
	ProdID            string
	Name              string
	Type              string
	Action            string
	CatName           string
	CatID             string
	CatDesc           string
	RoomDesc          string
	ImageBase         string
	Attributes        map[string]interface{}
	Width             int
	Depth             int
	Height            int
	ItemsAllowedOnTop bool
	Modifiable        bool
}

// Catalog represents the parsed Catalogue_English.txt file
type Catalog struct {
	Pages []Page
	Items []Item
}

var (
	// Regex patterns for parsing Lingo-style property lists (be lenient; capture full bracket content)
	rePageOpen = regexp.MustCompile(`^PAGE=\[`) // start of a PAGE block
	reItemOpen = regexp.MustCompile(`^ITEM=\[`) // start of an ITEM block
	reBlockEnd = regexp.MustCompile(`\]$`)      // end of a block

	reProp     = regexp.MustCompile(`#(\w+):\s*"([^"]*)"`)
	reIntProp  = regexp.MustCompile(`#(\w+):\s*(\d+)`)
	reBoolProp = regexp.MustCompile(`#(\w+):\s*(true|false)`) // lowercase true/false in file

	reAttrList = regexp.MustCompile(`#attributes:\s*\[([^\]]*)\]`)
	reAttrPair = regexp.MustCompile(`#(\w+):([^,\]]+)`)
)

// New creates a new empty Catalog
func New() *Catalog {
	return &Catalog{
		Pages: make([]Page, 0),
		Items: make([]Item, 0),
	}
}

// LoadFile parses Catalogue_English.txt format
func LoadFile(path string) (*Catalog, error) {
	f, err := fs.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	catalog := New()
	scanner := bufio.NewScanner(f)

	var buf strings.Builder
	var inPage, inItem bool

	flushBlock := func() {
		content := strings.TrimSpace(buf.String())
		buf.Reset()
		if content == "" {
			return
		}
		// remove leading markers like PAGE=[ or ITEM=[ and trailing ]
		if idx := strings.Index(content, "["); idx >= 0 {
			content = content[idx+1:]
		}
		if strings.HasSuffix(content, "]") {
			content = content[:len(content)-1]
		}
		content = strings.TrimSpace(content)
		if inPage {
			if page := parsePageProps(content); page != nil {
				catalog.Pages = append(catalog.Pages, *page)
			}
		}
		if inItem {
			if item := parseItemProps(content); item != nil {
				catalog.Items = append(catalog.Items, *item)
			}
		}
		inPage, inItem = false, false
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "[") || strings.HasPrefix(line, "# ") {
			continue
		}
		if !inPage && !inItem {
			if rePageOpen.MatchString(line) {
				inPage = true
				buf.WriteString(line)
				if reBlockEnd.MatchString(line) {
					flushBlock()
				}
				continue
			}
			if reItemOpen.MatchString(line) {
				inItem = true
				buf.WriteString(line)
				if reBlockEnd.MatchString(line) {
					flushBlock()
				}
				continue
			}
			continue
		}
		// inside a block
		buf.WriteString(" ")
		buf.WriteString(line)
		if reBlockEnd.MatchString(line) {
			flushBlock()
		}
	}

	return catalog, scanner.Err()
}

func parsePageProps(props string) *Page {
	page := &Page{}
	for _, match := range reProp.FindAllStringSubmatch(props, -1) {
		key, value := match[1], match[2]
		switch key {
		case "page":
			page.Page = value
		case "catId":
			page.CatID = value
		case "catName":
			page.CatName = value
		}
	}
	return page
}

func parseItemProps(props string) *Item {
	item := &Item{Attributes: make(map[string]interface{})}

	// strings
	for _, match := range reProp.FindAllStringSubmatch(props, -1) {
		key, value := match[1], match[2]
		switch key {
		case "prodId":
			item.ProdID = value
		case "name":
			item.Name = value
		case "type":
			item.Type = value
		case "action":
			item.Action = value
		case "catName":
			item.CatName = value
		case "catId":
			item.CatID = value
		case "catDesc":
			item.CatDesc = value
		case "roomDesc":
			item.RoomDesc = value
		case "imageBase":
			item.ImageBase = value
		}
	}
	// ints
	for _, match := range reIntProp.FindAllStringSubmatch(props, -1) {
		key, valueStr := match[1], match[2]
		value, _ := strconv.Atoi(strings.TrimSpace(valueStr))
		switch key {
		case "width":
			item.Width = value
		case "depth":
			item.Depth = value
		case "height":
			item.Height = value
		}
	}
	// bools
	for _, match := range reBoolProp.FindAllStringSubmatch(props, -1) {
		key, valueStr := match[1], strings.ToLower(match[2])
		value := valueStr == "true"
		switch key {
		case "itemsAllowedOnTop":
			item.ItemsAllowedOnTop = value
		case "modifiable":
			item.Modifiable = value
		}
	}
	// attributes
	if attrMatch := reAttrList.FindStringSubmatch(props); len(attrMatch) > 1 {
		attrStr := attrMatch[1]
		for _, m := range reAttrPair.FindAllStringSubmatch(attrStr, -1) {
			key := strings.TrimSpace(m[1])
			valStr := strings.TrimSpace(m[2])
			if n, err := strconv.Atoi(valStr); err == nil {
				item.Attributes[key] = n
			} else {
				item.Attributes[key] = valStr
			}
		}
	}
	return item
}

// GetPageByID returns a page by its category ID
func (c *Catalog) GetPageByID(catID string) *Page {
	for i := range c.Pages {
		if c.Pages[i].CatID == catID {
			return &c.Pages[i]
		}
	}
	return nil
}

// GetItemsByCategory returns all items in a category
func (c *Catalog) GetItemsByCategory(catID string) []Item {
	var items []Item
	for _, item := range c.Items {
		if item.CatID == catID {
			items = append(items, item)
		}
	}
	return items
}

// GetItemByID returns an item by its product ID
func (c *Catalog) GetItemByID(prodID string) *Item {
	for i := range c.Items {
		if c.Items[i].ProdID == prodID {
			return &c.Items[i]
		}
	}
	return nil
}
