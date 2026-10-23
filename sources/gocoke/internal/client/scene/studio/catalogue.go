package studio

import (
	"bufio"
	"fmt"
	"image/color"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	lingoassets "gocoke/internal/client/assets/lingo"
	"gocoke/internal/client/assets/lingo/window"
	textmgr "gocoke/internal/client/assets/text"
	"gocoke/internal/client/fs"
	"gocoke/internal/client/ui"
	"gocoke/internal/client/net/gateway"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
)

// CatalogueManager handles catalogue windows and simple page switching
type CatalogueManager struct {
	windowManager *ui.WindowManager
	tm            *textmgr.Manager
	currentPage   int
	currentItems  []catalogueItem

	// Parsed catalogue data (from data/Catalogue_English.txt)
	catalogueLoaded bool
	pageToCatID     map[int]int             // page -> catId
	itemsByCat      map[int][]catalogueItem // catId -> items

	// Mappings for price updates
	priceByProdID map[int]int    // prodID -> price (from gateway)
	aliasToProdID map[string]int // imageBase/alias -> prodID (from catalogue data)

	// Gateway providers and hooks
	getScreenName       func() string
	getSecret           func() string
	getGatewayURL       func() string
	onPurchaseSucceeded func(prodID int, posID string)

	// Purchase plumbing (legacy local fallback)
	onPurchase func(imageBase string, price int) bool
	getBalance func() int
}

func NewCatalogueManager(wm *ui.WindowManager, tm *textmgr.Manager) *CatalogueManager {
	return &CatalogueManager{windowManager: wm, tm: tm}
}

// SetPurchaseHandler sets callbacks for purchases and balance query
func (c *CatalogueManager) SetPurchaseHandler(handler func(imageBase string, price int) bool, getBal func() int) {
	c.onPurchase = handler
	c.getBalance = getBal
}

// SetGatewayProviders wires providers used for AMF purchase calls
func (c *CatalogueManager) SetGatewayProviders(getSN func() string, getSec func() string, getURL func() string) {
	c.getScreenName = getSN
	c.getSecret = getSec
	c.getGatewayURL = getURL
}

// SetOnPurchaseSucceeded sets a hook to run after a successful server purchase
// Arguments: prodID (catalog item id) and posID (new possession id), if available
func (c *CatalogueManager) SetOnPurchaseSucceeded(fn func(prodID int, posID string)) { c.onPurchaseSucceeded = fn }

// OpenCatalogue toggles the catalogue and opens page 1 by default
func (c *CatalogueManager) OpenCatalogue() {
	if c == nil || c.windowManager == nil || c.tm == nil {
		return
	}
	// Toggle: if any catalogue window is open, close it
	if c.windowManager.GetWindow("catalogue_page1") != nil ||
		c.windowManager.GetWindow("catalogue_page2") != nil ||
		c.windowManager.GetWindow("catalogue_pagedynamic") != nil {
		c.closeAllCatalogue()
		return
	}
	c.openPage(1)
}

func (c *CatalogueManager) closeAllCatalogue() {
	for _, w := range c.windowManager.GetOpenWindowObjects() {
		if w.Name == "catalogue_page1" || w.Name == "catalogue_page2" || w.Name == "catalogue_pagedynamic" {
			c.windowManager.CloseWindow(w.Name)
		}
	}
}

func (c *CatalogueManager) openPage(page int) {
	var fileName string
	var windowName string
	switch page {
	case 1:
		fileName = "143_catalogue.page1.window.txt"
		windowName = "catalogue_page1"
	case 2:
		fileName = "144_catalogue.page2.window.txt"
		windowName = "catalogue_page2"
	default:
		fileName = "142_catalogue.pagedynamic.window.txt"
		windowName = "catalogue_pagedynamic"
	}

	path := filepath.Join("Lingo", "cc_catalogue[1]", fileName)
	def, err := window.ParseFile(path)
	if err != nil {
		return
	}

	catalogueDir := filepath.Join("Lingo", "cc_catalogue[1]")
	interfaceDir := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, catalogueDir, interfaceDir)
	// Ensure button assets that may not be referenced directly are present
	if extra := lingoassets.LoadAssetsByNames([]string{
		"button.b1.left.active", "button.b1.middle.active", "button.b1.right.active",
		"button.b1.left.pressed", "button.b1.middle.pressed", "button.b1.right.pressed",
	}, interfaceDir); extra != nil {
		for k, v := range extra {
			assets[k] = v
		}
	}
	if extra := lingoassets.LoadAssetsByNames([]string{
		"cc.catalogue.button.left.active", "cc.catalogue.button.left.pressed",
		"cc.catalogue.button.right.active", "cc.catalogue.button.right.pressed",
	}, catalogueDir); extra != nil {
		for k, v := range extra {
			assets[k] = v
		}
	}

	w := c.windowManager.OpenWindow(windowName, def, assets, c.tm)
	if w == nil {
		return
	}
	w.Rect = def.Rect
	c.currentPage = page

	// Page-specific font normalization for page 1 title/labels/description
	if page == 1 && w.Definition != nil {
		for i := range w.Definition.Elements {
			el := &w.Definition.Elements[i]
			// Title, description and section labels
			if el.Member == "catalogue_1_title" || el.Member == "catalogue_1_desc" ||
				el.ID == "catalog_fpic1" || el.ID == "catalog_fpic2" || el.ID == "catalog_fpic3" ||
				el.Member == "cc.catalogue.frontpage.pic1.text" || el.Member == "cc.catalogue.frontpage.pic2.text" || el.Member == "cc.catalogue.frontpage.pic3.text" ||
				el.Member == "cc.catalogue.text.infotext2" {
				el.Font = "Verdana"
				if el.FontSize == 0 {
					el.FontSize = 10
				}
				// Keep original bold/plain from def if set; otherwise default plain for body, bold for labels
				if el.Member == "catalogue_1_title" || strings.Contains(strings.ToLower(el.Member), ".text") {
					if el.FontStyle == "" {
						el.FontStyle = "bold"
					}
				} else {
					if el.FontStyle == "" {
						el.FontStyle = "plain"
					}
				}
			}
			// Lower the main frontpage picture by ~60px
			if el.Member == "cc.catalogue.frontpage.picture" {
				el.LocV += 60
			}
		}
	}

	// Convert model buttons to interactive images and attach handlers
	c.setupButtons(w, page)
	// Initialize static text like page index if present
	c.setPageIndexText(w, page)

	// Page-specific dynamic updates
	if page >= 3 {
		c.populateDynamicPage(w, page)
	} else if page == 2 {
		c.setDynamicTextKeys(w, 1) // CAT_1_* for page 2
		c.ensurePage2Title(w)
	} else if page == 1 {
		c.setDynamicTextKeys(w, 0) // CAT_0_* for page 1
	}
}

func (c *CatalogueManager) setPageIndexText(w *ui.Window, page int) {
	if w == nil || w.Definition == nil {
		return
	}
	for i := range w.Definition.Elements {
		el := &w.Definition.Elements[i]
		if el.ID == "catalog_index" {
			// Example: "Page 1"
			label := c.tm.First("CAT_PAGE")
			if label == "" {
				label = "Page"
			}
			// Show "Page X / 15" and enforce Verdana font styling
			el.Key = label + " " + strconv.Itoa(page) + " / 15"
			el.Font = "Verdana"
			if el.FontStyle == "" {
				el.FontStyle = "bold"
			}
			if el.FontSize == 0 {
				el.FontSize = 10
			}
		}
	}
}

func (c *CatalogueManager) setupButtons(w *ui.Window, page int) {
	if w == nil || w.Definition == nil {
		return
	}
	// IDs we care about across pages
	ids := []string{
		"catalog_previous", "catalog_next",
		// page2 specific controls (no-op for now)
		"cat_wall_prev", "cat_wall_next", "cat_walltexture_prev", "cat_walltexture_next",
		"cat_floor_prev", "cat_floor_next", "cat_floortexture_prev", "catfloortexture_next",
		"wall_ok", "floor_ok",
		// dynamic page buy buttons
		"buy_1", "buy_2", "buy_3", "buy_4", "buy_5", "buy_6", "buy_7", "buy_8",
	}

	// Convert Lingo button elements to interactive images matching our UI renderer
	for i := range w.Definition.Elements {
		el := &w.Definition.Elements[i]
		for _, id := range ids {
			if el.ID == id && el.Type == "button" {
				// Map known models to assets
				switch el.Model {
				case 2: // button.b1
					el.Media = "bitmap"
					el.Type = "image"
					el.Member = "button.b1.left.active"
				case 4:
					el.Media = "bitmap"
					el.Type = "image"
					// Use specific assets for small arrow-like buttons
					if id == "cat_wall_prev" || id == "cat_floor_prev" || id == "cat_walltexture_prev" || id == "cat_floortexture_prev" {
						el.Member = "cc.catalogue.button.left.active"
					} else {
						el.Member = "cc.catalogue.button.right.active"
					}
				}
				if w.Interactive == nil {
					w.Interactive = make(map[string]*ui.InteractiveElement)
				}
				if w.Interactive[el.ID] == nil {
					if ie := w.Renderer.CreateInteractiveElement(*el); ie != nil {
						w.Interactive[el.ID] = ie
					}
				}
				break
			}
		}
	}

	// Attach simple handlers for prev/next to flip pages 1..15
	w.RoomListClick = nil
	w.TabTransition = nil
	w.GoButtonClick = nil

	w.ButtonClick = func(elementID string) {
		switch elementID {
		case "catalog_previous":
			if page > 1 {
				c.transitionPage(w, page-1)
			}
		case "catalog_next":
			if page < 15 {
				c.transitionPage(w, page+1)
			}
		case "buy_1", "buy_2", "buy_3", "buy_4", "buy_5", "buy_6", "buy_7", "buy_8":
			idx := int(elementID[len("buy_"):][0] - '0')
			c.openPurchaseInfo(idx)
		case "wall_ok", "floor_ok":
			// TODO: purchase flow integration for walls/floors
		default:
			// Handle catalogue buy page overlays (text rows and small icons)
			if strings.HasPrefix(elementID, "cat_item_overlay_") {
				if n, err := strconv.Atoi(elementID[len("cat_item_overlay_"):]); err == nil {
					c.previewItemByIndex(w, n)
				}
			}
			if strings.HasPrefix(elementID, "cat_icon_overlay_") {
				if n, err := strconv.Atoi(elementID[len("cat_icon_overlay_"):]); err == nil {
					c.previewItemByIndex(w, n)
				}
			}
		}
	}
}

func (c *CatalogueManager) transitionPage(current *ui.Window, toPage int) {
	if current == nil {
		return
	}
	rect := current.Rect
	// Close current and open new page preserving position
	c.closeAllCatalogue()
	c.openPage(toPage)
	if w := c.windowManager.GetWindow(pageName(toPage)); w != nil {
		w.Rect = rect
	}
}

func pageName(page int) string {
	switch page {
	case 1:
		return "catalogue_page1"
	case 2:
		return "catalogue_page2"
	default:
		return "catalogue_pagedynamic"
	}
}

// ---- Dynamic page support ----
type catalogueItem struct {
	Alias    string
	PrevBase string
	Name     string
	Price    int
	CatID    int
	ProdID   int
}

func (c *CatalogueManager) populateDynamicPage(w *ui.Window, page int) {
	// Ensure catalogue data is loaded
	c.ensureCatalogueData()
	// Pages map to category via parsed PAGE entries
	catID := c.pageToCatFor(page)
	items := c.itemsByCat[catID]
	// Fall back to previous method if no data
	if len(items) == 0 {
		items = c.loadBuyPageItems(page)
	}
	// Limit to 8 items for display
	if len(items) > 8 {
		items = items[:8]
	}
	// Override prices from gateway if available
	if c.priceByProdID != nil {
		for i := range items {
			if items[i].ProdID > 0 {
				if p, ok := c.priceByProdID[items[i].ProdID]; ok && p > 0 {
					items[i].Price = p
				}
			}
		}
	}
	c.currentItems = items
	// Compute text category index for titles/descriptions; skip missing 12
	textCat := c.deriveCatIndexFromPage(page)
	c.setDynamicTextKeys(w, textCat)
	// Ensure category title is visible for dynamic pages
	c.ensureDynamicTitle(w, textCat)
	c.renderItemList(w, items)
	// Render furniture icons as separate elements on top of everything
	c.renderFurnitureIcons(w, items)
	// Hide rank list button if present on this definition
	c.hideRanklistButton(w)
	// Hide preview labels and wire up interactive overlays for items/icons
	c.setupBuyPageInteractive(w, items)
}

// ensureDynamicTitle adds or updates a text element to display CAT_X_TITLE
func (c *CatalogueManager) ensureDynamicTitle(w *ui.Window, catIndex int) {
	if w == nil || w.Definition == nil {
		return
	}
	// Try to find an existing dynamic title by ID
	for i := range w.Definition.Elements {
		if w.Definition.Elements[i].ID == "catalog_dyntitle" {
			w.Definition.Elements[i].Key = "CAT_" + strconv.Itoa(catIndex) + "_TITLE"
			// Enforce Verdana bold style
			w.Definition.Elements[i].Font = "Verdana"
			if w.Definition.Elements[i].FontStyle == "" {
				w.Definition.Elements[i].FontStyle = "bold"
			}
			if w.Definition.Elements[i].FontSize == 0 {
				w.Definition.Elements[i].FontSize = 20
			}
			if w.Definition.Elements[i].TxtColor == "" {
				w.Definition.Elements[i].TxtColor = "#EEDBC6"
			}
			return
		}
	}
	// Determine placement: near "page_title" image or default position
	locH, locV := 21, 17
	width, height := 245, 54
	for _, el := range w.Definition.Elements {
		if el.ID == "page_title" {
			// Position text within the title background area
			locH = el.LocH + 1
			locV = el.LocV - 2
			// Give a comfortable area for multi-line if needed
			width = el.Width
			if width <= 0 {
				width = 245
			}
			height = 54
			break
		}
	}
	titleEl := window.Element{
		Member:    "catalog_dyntitle",
		Media:     "field",
		LocH:      locH,
		LocV:      locV,
		Width:     width,
		Height:    height,
		ID:        "catalog_dyntitle",
		Type:      "text",
		Ink:       36,
		Blend:     100,
		Font:      "Verdana",
		FontSize:  20,
		FontStyle: "bold",
		TxtColor:  "#EEDBC6",
		Alignment: "left",
		Key:       "CAT_" + strconv.Itoa(catIndex) + "_TITLE",
		WordWrap:  1,
	}
	w.Definition.Elements = append(w.Definition.Elements, titleEl)
}

// deriveCatIndexFromPage maps page number to CAT index, skipping missing 12
func (c *CatalogueManager) deriveCatIndexFromPage(page int) int {
	if page < 3 {
		return 0
	}
	idx := page - 1 // 3->2, 4->3, ...
	if idx >= 12 {  // skip CAT_12_*
		idx++ // 13->14, 14->15, 15->16
	}
	return idx
}

// ensurePage2Title styles the page 2 title field
func (c *CatalogueManager) ensurePage2Title(w *ui.Window) {
	if w == nil || w.Definition == nil {
		return
	}
	for i := range w.Definition.Elements {
		el := &w.Definition.Elements[i]
		if el.Member == "catalogue_dyntitle" || strings.EqualFold(el.Key, "CAT_1_TITLE") {
			el.Key = "CAT_1_TITLE"
			el.Font = "Verdana"
			if el.FontStyle == "" {
				el.FontStyle = "bold"
			}
			if el.FontSize == 0 {
				el.FontSize = 20
			}
			if el.TxtColor == "" {
				el.TxtColor = "#EEDBC6"
			}
			return
		}
	}
}

// hideRanklistButton removes the optional rank list button from dynamic page
func (c *CatalogueManager) hideRanklistButton(w *ui.Window) {
	if w == nil || w.Definition == nil {
		return
	}
	if w.Interactive != nil {
		delete(w.Interactive, "open_ranklist")
	}
	// Filter out element with ID open_ranklist
	filtered := make([]window.Element, 0, len(w.Definition.Elements))
	for _, el := range w.Definition.Elements {
		if el.ID == "open_ranklist" {
			continue
		}
		filtered = append(filtered, el)
	}
	w.Definition.Elements = filtered
}

func (c *CatalogueManager) setDynamicTextKeys(w *ui.Window, catIndex int) {
	if w == nil || w.Definition == nil {
		return
	}
	for i := range w.Definition.Elements {
		el := &w.Definition.Elements[i]
		if el.ID == "catalog_infotext" {
			el.Key = "CAT_" + strconv.Itoa(catIndex) + "_Desc"
			// Ensure Verdana font for category description
			el.Font = "Verdana"
			if el.FontStyle == "" {
				el.FontStyle = "plain"
			}
			if el.FontSize == 0 {
				el.FontSize = 10
			}
		}
	}
}

func (c *CatalogueManager) loadBuyPageItems(page int) []catalogueItem {
	idx := page + 8 // 3->11, 12->20
	path := filepath.Join("Lingo", "cc_catalogue[1]", strconv.Itoa(idx)+"_buy_page_"+strconv.Itoa(page)+".txt")
	f, err := fs.Open(filepath.Clean(path))
	if err != nil {
		return nil
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	lines := strings.Split(string(b), "\n")
	var items []catalogueItem
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		// Parse alias and optional prev value (very simple tokenizer)
		fields := strings.Fields(ln)
		if len(fields) < 2 {
			continue
		}
		code := fields[1]
		prevBase := code
		if i := strings.Index(ln, "\"prev\""); i >= 0 {
			if j := strings.Index(ln[i+6:], ":"); j >= 0 {
				seg := ln[i+6+j+1:]
				seg = strings.TrimSpace(seg)
				seg = strings.Trim(seg, " \"[],")
				if k := strings.Index(seg, "\""); k >= 0 {
					seg = seg[k+1:]
					if m := strings.Index(seg, "\""); m > 0 {
						prevBase = seg[:m]
					}
				}
			}
		}
		// Default price until overridden by gateway
		price := 100
		prodID := 0
		if c.aliasToProdID != nil {
			if id, ok := c.aliasToProdID[code]; ok {
				prodID = id
				if c.priceByProdID != nil {
					if p, ok2 := c.priceByProdID[id]; ok2 && p > 0 {
						price = p
					}
				}
			}
		}
		items = append(items, catalogueItem{
			Alias:    code,
			PrevBase: prevBase,
			Name:     prettifyName(code),
			Price:    price,
			ProdID:   prodID,
		})
		if len(items) >= 8 {
			break
		}
	}
	return items
}

func prettifyName(code string) string {
	code = strings.ReplaceAll(code, "_", " ")
	if len(code) == 0 {
		return "Item"
	}
	return strings.Title(code)
}

func (c *CatalogueManager) renderItemList(w *ui.Window, items []catalogueItem) {
	// Find the texts.items element to get target size and offset (only size matters for asset)
	var width, height int
	for _, el := range w.Definition.Elements {
		if el.ID == "texts.items" {
			width = el.Width
			height = el.Height
			break
		}
	}
	if width <= 0 {
		width = 155
	}
	if height <= 0 {
		height = 279
	}
	img := ebiten.NewImage(width, height)
	// Use a local font manager for drawing into the offscreen image
	fm := ui.NewFontManager()
	fm.PreloadCommonFonts()
	fontFace := fm.GetFont("Verdana", "bold", 10)
	if fontFace == nil {
		return
	}
	col := color.RGBA{0x6C, 0x47, 0x2D, 0xFF}
	lineGap := 13
	rowHeight := 36

	textX := 0 // Text starts at left edge
	maxTextWidth := width - textX - 4
	// Start slightly higher (move up a couple of pixels)
	currentY := 9

	for _, it := range items {
		content := it.Name + " - " + strconv.Itoa(it.Price) + " dB"
		lines := wrapTextToWidth(content, maxTextWidth, fontFace)
		// Draw up to two lines per item row for neat wrapping
		for li := 0; li < len(lines) && li < 2; li++ {
			text.Draw(img, lines[li], fontFace, textX, currentY+(li*lineGap), col)
		}
		currentY += rowHeight
	}
	if w.Assets == nil {
		w.Assets = map[string]*ebiten.Image{}
	}
	w.Assets["texts.items"] = img
}

// Handle buy button clicks and open purchase info
func (c *CatalogueManager) openPurchaseInfo(idx int) {
	if idx < 1 || idx > len(c.currentItems) {
		return
	}
	it := c.currentItems[idx-1]
	defPath := filepath.Join("Lingo", "cc_catalogue[1]", "173_purchase_info.window.txt")
	def, err := window.ParseFile(defPath)
	if err != nil {
		return
	}
	dir := filepath.Join("Lingo", "cc_catalogue[1]")
	iface := filepath.Join("Lingo", "cc_interface[1]")
	assets := lingoassets.LoadAssetsForDefinition(def, dir, iface)
	// Ensure button assets available
	if extra := lingoassets.LoadAssetsByNames([]string{
		"button.b1.left.active", "button.b1.middle.active", "button.b1.right.active",
		"button.b1.left.pressed", "button.b1.middle.pressed", "button.b1.right.pressed",
		"button.e1.left.active", "button.e1.middle.active", "button.e1.right.active",
		"button.e1.left.pressed", "button.e1.middle.pressed", "button.e1.right.pressed",
	}, iface); extra != nil {
		for k, v := range extra {
			assets[k] = v
		}
	}
	// Populate texts based on definition IDs/members (start with loading placeholders)
	for i := range def.Elements {
		el := &def.Elements[i]
		if el.ID == "sanfo_decision_text_a" || el.Member == "purchase_bold_alert_text" {
			line := c.tm.First("PURCHASE_PRICE")
			if line == "" { line = "{item} costs {price} decibels" }
			line = strings.ReplaceAll(line, "{item}", it.Name)
			line = strings.ReplaceAll(line, "{price}", "...")
			el.Key = line
		}
		if el.ID == "sanfo_decision_text_b" || el.Member == "purchase_bigalertext" {
			bal := c.tm.First("PURCHASE_BALANCE")
			if bal == "" { bal = "You have {balance} decibels in your account." }
			bal = strings.ReplaceAll(bal, "{balance}", "...")
			el.Key = bal
		}
		// Style buttons: OK uses Go-style (button.e1), Cancel uses button.b1
		if strings.ToLower(el.Type) == "button" {
			el.Media = "bitmap"
			el.Type = "image"
			if el.ID == "purchase_decision_ok" {
				el.Member = "button.e1.left.active"
			} else if el.ID == "sanfo_decision_cancel" {
				el.Member = "button.b1.left.active"
			}
		}
	}
	w := c.windowManager.OpenWindow("purchase_info", def, assets, c.tm)
	if w != nil {
		w.Rect = def.Rect
		// Create interactives for OK/Cancel
		if w.Interactive == nil { w.Interactive = make(map[string]*ui.InteractiveElement) }
		for i := range w.Definition.Elements {
			e := w.Definition.Elements[i]
			if e.ID == "purchase_decision_ok" || e.ID == "sanfo_decision_cancel" {
				if ie := w.Renderer.CreateInteractiveElement(e); ie != nil { w.Interactive[e.ID] = ie }
			}
		}
		// Kick off prePurchase to fetch live price/balance and properties
		prodID := it.ProdID
		if prodID == 0 && c.aliasToProdID != nil { if id, ok := c.aliasToProdID[it.Alias]; ok { prodID = id } }
		if c.getScreenName != nil && c.getSecret != nil && c.getGatewayURL != nil && c.getGatewayURL() != "" && prodID > 0 {
			go func(w *ui.Window, name string, url string, sec string, pid int) {
				httpClient := &http.Client{Timeout: 15 * time.Second}
				res, err := gateway.PrePurchaseItem(httpClient, url, name, sec, pid, "[:]")
				if err != nil { log.Printf("Catalogue: prePurchase failed: %v", err); return }
				// Update texts
				priceLine := c.tm.First("PURCHASE_PRICE"); if priceLine == "" { priceLine = "{item} costs {price} decibels" }
				priceLine = strings.ReplaceAll(priceLine, "{item}", it.Name)
				priceLine = strings.ReplaceAll(priceLine, "{price}", strconv.Itoa(res.Price))
				balLine := c.tm.First("PURCHASE_BALANCE"); if balLine == "" { balLine = "You have {balance} decibels in your account." }
				balLine = strings.ReplaceAll(balLine, "{balance}", strconv.Itoa(res.Balance))
				for i := range w.Definition.Elements {
					if w.Definition.Elements[i].ID == "sanfo_decision_text_a" || w.Definition.Elements[i].Member == "purchase_bold_alert_text" { w.Definition.Elements[i].Key = priceLine }
					if w.Definition.Elements[i].ID == "sanfo_decision_text_b" || w.Definition.Elements[i].Member == "purchase_bigalertext" { w.Definition.Elements[i].Key = balLine }
				}
				// Stash pending values for OK click
				if w.TempVars == nil { w.TempVars = map[string]interface{}{} }
				w.TempVars["pendingProdID"] = res.CatalogItemID
				w.TempVars["pendingProps"] = res.Properties
				w.TempVars["pendingPrice"] = res.Price
				w.TempVars["pendingBalance"] = res.Balance
			}(w, c.getScreenName(), c.getGatewayURL(), c.getSecret(), prodID)
		} else {
			log.Printf("Catalogue: skipping prePurchase (missing credentials/url or prodID)")
		}
		// Wire button clicks: OK triggers purchase, Cancel closes
		w.ButtonClick = func(elementID string) {
			switch elementID {
			case "sanfo_decision_cancel":
				c.windowManager.CloseWindow("purchase_info")
			case "purchase_decision_ok":
				// Prefer server-side purchase if credentials and prePurchase data are available
				if c.getScreenName != nil && c.getSecret != nil && c.getGatewayURL != nil && c.getGatewayURL() != "" {
					pid, _ := w.TempVars["pendingProdID"].(int)
					props, _ := w.TempVars["pendingProps"].(string)
					if pid > 0 {
						go func(name, url, sec string, prod int, attributes string) {
							httpClient := &http.Client{Timeout: 15 * time.Second}
							resp, err := gateway.PurchaseItem(httpClient, url, name, sec, prod, attributes)
							if err != nil {
								log.Printf("Catalogue: purchaseItem failed: %v", err)
								return
							}
							// Attempt to parse the newly purchased possession to bring it to front
							newProdID := 0
							newPosID := ""
							if arr, ok := resp.([]any); ok && len(arr) > 0 {
								if m, ok2 := arr[0].(map[string]any); ok2 {
									// prodID (catalogItem)
									if v, ok3 := m["catalogItem"]; ok3 {
										switch x := v.(type) {
										case float64:
											newProdID = int(x)
										case int:
											newProdID = x
										}
									}
									// posID (id)
									if v, ok3 := m["id"]; ok3 {
										switch x := v.(type) {
										case float64:
											newPosID = strconv.Itoa(int(x))
										case int:
											newPosID = strconv.Itoa(x)
										case string:
											newPosID = x
										}
									}
								}
							}
							// Notify scene to refresh balance/backpack and open/reorder
							if c.onPurchaseSucceeded != nil { c.onPurchaseSucceeded(newProdID, newPosID) }
						}(c.getScreenName(), c.getGatewayURL(), c.getSecret(), pid, props)
						// Close immediately (match Lingo behavior)
						c.windowManager.CloseWindow("purchase_info")
						return
					}
				}
				// Fallback: client-side purchase handler
				if c.onPurchase != nil {
					success := c.onPurchase(it.Alias, it.Price)
					if success { c.windowManager.CloseWindow("purchase_info") }
				} else {
					c.windowManager.CloseWindow("purchase_info")
				}
			}
		}
	}
}

// ensureCatalogueData parses Catalogue_English.txt into memory on first use
func (c *CatalogueManager) ensureCatalogueData() {
	if c.catalogueLoaded {
		return
	}
	c.pageToCatID = map[int]int{}
	c.itemsByCat = map[int][]catalogueItem{}
	// Prefer data/ then fallback to web/data/
	paths := []string{
		filepath.Join("data", "Catalogue_English.txt"),
		filepath.Join("web", "data", "Catalogue_English.txt"),
	}
	var r io.Reader
	for _, p := range paths {
		if f, err := fs.Open(filepath.Clean(p)); err == nil {
			r = f
			defer f.Close()
			break
		}
	}
	if r == nil {
		c.catalogueLoaded = true // avoid retry spam
		return
	}
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}
		if strings.HasPrefix(line, "PAGE=") {
			// PAGE=[ #page: "10", #catId: "9", ... ]
			page := extractIntField(line, "#page:")
			cat := extractIntField(line, "#catId:")
			if page > 0 && cat >= 0 {
				c.pageToCatID[page] = cat
			}
			continue
		}
		if strings.HasPrefix(line, "ITEM=") {
			cat := extractIntField(line, "#catId:")
			name := extractStringField(line, "#catDesc:")
			imageBase := extractStringField(line, "#imageBase:")
			prodID := extractIntField(line, "#prodId:")
			price := extractIntField(line, "#price:")
			if price <= 0 {
				price = 100 // Default default; may be overridden by gateway
			}
			// Remember alias->prod mapping for fallback pages
			if c.aliasToProdID == nil {
				c.aliasToProdID = make(map[string]int)
			}
			if imageBase != "" && prodID > 0 {
				c.aliasToProdID[imageBase] = prodID
			}
			// Build item using imageBase as both alias and prevBase
			item := catalogueItem{CatID: cat, Name: name, Alias: imageBase, PrevBase: imageBase, Price: price, ProdID: prodID}
			c.itemsByCat[cat] = append(c.itemsByCat[cat], item)
			continue
		}
	}
	c.catalogueLoaded = true
}


// ApplyPriceList updates internal price mappings and refreshes the current dynamic page if open.
func (c *CatalogueManager) ApplyPriceList(prices map[int]int) {
	if c == nil || prices == nil {
		return
	}
	// Store mapping
	c.priceByProdID = prices
	// Update all parsed catalogue items
	for cat, arr := range c.itemsByCat {
		for i := range arr {
			if arr[i].ProdID > 0 {
				if p, ok := prices[arr[i].ProdID]; ok && p > 0 {
					arr[i].Price = p
				}
			}
		}
		c.itemsByCat[cat] = arr
	}
	// If a dynamic page is currently open, refresh its list rendering
	if c.currentPage >= 3 {
		if w := c.windowManager.GetWindow(pageName(c.currentPage)); w != nil {
			catID := c.pageToCatFor(c.currentPage)
			items := c.itemsByCat[catID]
			if len(items) > 8 {
				items = items[:8]
			}
			// Apply price map to items just in case
			for i := range items {
				if items[i].ProdID > 0 {
					if p, ok := prices[items[i].ProdID]; ok && p > 0 {
						items[i].Price = p
					}
				}
			}
			c.currentItems = items
			c.renderItemList(w, items)
		}
	}
}

func (c *CatalogueManager) pageToCatFor(page int) int {
	if c.pageToCatID == nil {
		return -1
	}
	if v, ok := c.pageToCatID[page]; ok {
		return v
	}
	return -1
}

// extract helpers for Catalogue_English.txt format (very lenient)
func extractIntField(line, key string) int {
	idx := strings.Index(line, key)
	if idx < 0 {
		return -1
	}
	rest := strings.TrimSpace(line[idx+len(key):])
	// value may be quoted or plain
	if strings.HasPrefix(rest, "\"") {
		rest = rest[1:]
		if p := strings.Index(rest, "\""); p >= 0 {
			rest = rest[:p]
		}
	} else {
		// up to comma or bracket
		if p := strings.IndexAny(rest, ",]"); p >= 0 {
			rest = rest[:p]
		}
	}
	rest = strings.TrimSpace(rest)
	v, _ := strconv.Atoi(rest)
	return v
}

func extractStringField(line, key string) string {
	idx := strings.Index(line, key)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(line[idx+len(key):])
	if strings.HasPrefix(rest, "\"") {
		rest = rest[1:]
		if p := strings.Index(rest, "\""); p >= 0 {
			rest = rest[:p]
		}
	} else {
		if p := strings.IndexAny(rest, ",]"); p >= 0 {
			rest = rest[:p]
		}
	}
	return strings.TrimSpace(rest)
}

// loadSmallFurnitureIcons loads small furniture preview icons for the given items
func (c *CatalogueManager) loadSmallFurnitureIconsForItems(items []catalogueItem) map[string]*ebiten.Image {
	smallIconsDir := filepath.Join("Lingo", "cc_furniture_small")

	// Extract unique icon names from the items
	iconNames := make(map[string]bool)
	for _, item := range items {
		if iconName := c.furnitureToSmallName(item.PrevBase); iconName != "" {
			iconNames[iconName] = true
		}
		// Also try the alias if PrevBase doesn't work
		if iconName := c.furnitureToSmallName(item.Alias); iconName != "" {
			iconNames[iconName] = true
		}
	}

	// Convert map keys to slice
	var iconList []string
	for name := range iconNames {
		iconList = append(iconList, name)
	}

	return lingoassets.LoadAssetsByNames(iconList, smallIconsDir)
}

// getSmallIcon converts a furniture name like "northern_chair_a_0_1_1_2_0" to its small icon
func (c *CatalogueManager) getSmallIcon(furnitureName string, iconAssets map[string]*ebiten.Image) *ebiten.Image {
	if furnitureName == "" || iconAssets == nil {
		return nil
	}

	// Convert full furniture name to small icon name
	// e.g., "northern_chair_a_0_1_1_2_0" -> "northern_chair_small"
	smallName := c.furnitureToSmallName(furnitureName)
	if smallName == "" {
		return nil
	}

	return iconAssets[smallName]
}

// furnitureToSmallName converts full furniture names to small icon names
func (c *CatalogueManager) furnitureToSmallName(fullName string) string {
	if fullName == "" {
		return ""
	}

	// Handle different naming patterns:
	// 1. Full variant names: "northern_chair_a_0_1_1_2_0" -> "northern_chair_small"
	// 2. Base names from catalog: "northern_chair" -> "northern_chair_small"
	// 3. Special cases: "poster neonsign" -> "poster neonsign_small"

	// If it already looks like a base name (no variant suffix), just add _small
	if !strings.Contains(fullName, "_a_") && !strings.Contains(fullName, "_b_") &&
		!strings.Contains(fullName, "_c_") && !strings.Contains(fullName, "_d_") &&
		!strings.Contains(fullName, "_e_") {
		// Handle space-separated names like "poster neonsign" - keep spaces for member name lookup
		return fullName + "_small"
	}

	// For variant names, extract the base part
	parts := strings.Split(fullName, "_")
	if len(parts) < 2 {
		return fullName + "_small"
	}

	// Find where the variant pattern starts (usually _a_, _b_, etc.)
	var baseParts []string
	for _, part := range parts {
		// Stop at single letter parts that indicate variant (a, b, c, etc.)
		if len(part) == 1 && (part == "a" || part == "b" || part == "c" || part == "d" || part == "e") {
			break
		}
		baseParts = append(baseParts, part)
	}

	if len(baseParts) == 0 {
		return fullName + "_small"
	}

	return strings.Join(baseParts, "_") + "_small"
}

// renderFurnitureIcons creates separate icon assets positioned to avoid being covered by dotted lines
func (c *CatalogueManager) renderFurnitureIcons(w *ui.Window, items []catalogueItem) {
	if w == nil || w.Assets == nil {
		return
	}

	// Load small furniture icons for the specific items on this page
	smallIconAssets := c.loadSmallFurnitureIconsForItems(items)
	if smallIconAssets == nil {
		return
	}

	// Position icons in the left margin area, before the dotted lines start
	// dotlines.all is at locH: 55, so we position icons at X < 55
	iconAreaWidth := 32 // Width of the icon area
	iconAreaLeft := 18  // Left edge of icon area
	iconBaseY := 54     // Align with dotted lines start (locV from window def)
	rowHeight := 36     // Same as text rows

	for i, item := range items {
		if iconImg := c.getSmallIcon(item.PrevBase, smallIconAssets); iconImg != nil {
			// Create a unique asset name for each icon position
			assetName := fmt.Sprintf("furniture_icon_%d", i+1)

			// Create a small image just for this icon
			iconSize := iconImg.Bounds()
			positionedIcon := ebiten.NewImage(iconSize.Dx(), iconSize.Dy())
			positionedIcon.DrawImage(iconImg, &ebiten.DrawImageOptions{})

			// Store as window asset - this will be rendered by the UI system
			w.Assets[assetName] = positionedIcon

			// Center the icon horizontally within the icon area
			iconCenterX := iconAreaLeft + (iconAreaWidth-iconSize.Dx())/2
			// Center the icon vertically within the row
			iconCenterY := iconBaseY + (i * rowHeight) + (rowHeight-iconSize.Dy())/2

			// Add a dynamic element to the window definition to position this icon
			iconElement := window.Element{
				ID:     assetName,
				Member: assetName,
				Media:  "bitmap",
				Type:   "image",
				LocH:   iconCenterX,
				LocV:   iconCenterY,
				Width:  iconSize.Dx(),
				Height: iconSize.Dy(),
				Ink:    36,
				Blend:  100,
			}

			// Add to window definition elements
			w.Definition.Elements = append(w.Definition.Elements, iconElement)
		}
	}
}

// setupBuyPageInteractive hides preview helper text and adds click overlays
func (c *CatalogueManager) setupBuyPageInteractive(w *ui.Window, items []catalogueItem) {
	if w == nil || w.Definition == nil {
		return
	}

	// Blank out preview helper texts: 'Preview' label and 'Click icon' text
	for i := range w.Definition.Elements {
		el := &w.Definition.Elements[i]
		if el.Key == "cat_preview" || el.Key == "cat_clickicon" {
			el.Key = ""
		}
	}

	if w.Interactive == nil {
		w.Interactive = make(map[string]*ui.InteractiveElement)
	}

	// Clickable regions for each item row in the text list
	var textLocH, textLocV, textWidth int
	const rowHeight = 36
	for _, el := range w.Definition.Elements {
		if el.ID == "texts.items" {
			textLocH = el.LocH
			textLocV = el.LocV
			textWidth = el.Width
			break
		}
	}
	if textWidth == 0 {
		textWidth = 155
	}
	baseY := textLocV + 9
	for i := range items {
		id := fmt.Sprintf("cat_item_overlay_%d", i+1)
		w.Interactive[id] = &ui.InteractiveElement{ID: id, Rect: [4]int{textLocH, baseY + i*rowHeight, textWidth, rowHeight}}
	}

	// Clickable regions matching dynamic icon elements
	for _, el := range w.Definition.Elements {
		if strings.HasPrefix(el.ID, "furniture_icon_") && el.Width > 0 && el.Height > 0 {
			idxStr := strings.TrimPrefix(el.ID, "furniture_icon_")
			id := "cat_icon_overlay_" + idxStr
			w.Interactive[id] = &ui.InteractiveElement{ID: id, Rect: [4]int{el.LocH, el.LocV, el.Width, el.Height}}
		}
	}
}

// previewItemByIndex renders a full-size preview of the item into the preview box
func (c *CatalogueManager) previewItemByIndex(w *ui.Window, idx int) {
	if idx < 1 || idx > len(c.currentItems) || w == nil || w.Definition == nil {
		return
	}
	c.previewItemInWindow(w, c.currentItems[idx-1])
}

func (c *CatalogueManager) previewItemInWindow(w *ui.Window, it catalogueItem) {
	// Locate preview box geometry
	var box window.Element
	found := false
	for _, el := range w.Definition.Elements {
		if el.ID == "cat_previewbox" {
			box = el
			found = true
			break
		}
	}
	if !found || box.Width <= 0 || box.Height <= 0 {
		return
	}

	// Load a representative furniture frame
	base := it.PrevBase
	if base == "" {
		base = it.Alias
	}
	img := c.loadRepresentativeFurnitureFrame(base)
	if img == nil {
		return
	}

	// Render scaled, centered into preview box
	dst := ebiten.NewImage(box.Width, box.Height)
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW == 0 || srcH == 0 {
		return
	}
	margin := 6
	availW := box.Width - margin*2
	availH := box.Height - margin*2
	scaleX := float64(availW) / float64(srcW)
	scaleY := float64(availH) / float64(srcH)
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}
	if scale > 1.0 {
		scale = 1.0
	}
	drawW := int(float64(srcW) * scale)
	drawH := int(float64(srcH) * scale)
	offX := (box.Width - drawW) / 2
	offY := (box.Height - drawH) / 2
	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(offX), float64(offY))
	dst.DrawImage(img, op)

	if w.Assets == nil {
		w.Assets = map[string]*ebiten.Image{}
	}
	w.Assets["catalog_preview"] = dst

	// Ensure a render element exists at the preview box location
	has := false
	for i := range w.Definition.Elements {
		if w.Definition.Elements[i].ID == "catalog_preview" {
			w.Definition.Elements[i].LocH = box.LocH
			w.Definition.Elements[i].LocV = box.LocV
			w.Definition.Elements[i].Width = box.Width
			w.Definition.Elements[i].Height = box.Height
			has = true
			break
		}
	}
	if !has {
		el := window.Element{ID: "catalog_preview", Member: "catalog_preview", Media: "bitmap", Type: "image", LocH: box.LocH, LocV: box.LocV, Width: box.Width, Height: box.Height, Ink: 36, Blend: 100}
		w.Definition.Elements = append(w.Definition.Elements, el)
	}
}

// loadRepresentativeFurnitureFrame fetches a sensible full image for a furniture base
func (c *CatalogueManager) loadRepresentativeFurnitureFrame(base string) *ebiten.Image {
	if base == "" {
		return nil
	}
	mm, _ := lingoassets.LoadMemberMap(filepath.Join("Lingo", "cc_furniture[1]"))
	if mm == nil {
		return nil
	}
	candidates := []string{
		base + "_a_0_1_1_2_0",
		base + "_a_0_1_1_0_0",
		base + "_a_0_1_2_0_0",
	}
	for _, name := range candidates {
		if p, ok := mm.PathFor(name); ok {
			if im, err := lingoassets.LoadImage(p); err == nil {
				return im
			}
		}
	}
	return nil
}

// wrapTextToWidth splits text into lines that fit within maxWidth using the provided font face
func wrapTextToWidth(s string, maxWidth int, face font.Face) []string {
	if s == "" || face == nil || maxWidth <= 0 {
		return []string{s}
	}
	words := strings.Fields(s)
	var lines []string
	var current string
	for _, w := range words {
		test := w
		if current != "" {
			test = current + " " + w
		}
		if text.BoundString(face, test).Dx() <= maxWidth {
			current = test
		} else {
			if current != "" {
				lines = append(lines, current)
				current = w
			} else {
				// Single word too long; hard-break it
				lines = append(lines, w)
				current = ""
			}
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	if len(lines) == 0 {
		lines = append(lines, s)
	}
	return lines
}
