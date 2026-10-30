// Package avatar contains the account/client appearance compatibility boundary.
package avatar

import (
	"fmt"
	"strconv"
	"strings"
)

// NormalizeDefinition translates the numeric palette format emitted by the website
// or legacy records into the RGB definition used by the recovered Go avatar renderer.
// Existing full RGB definitions (which contain commas) are preserved intact.
func NormalizeDefinition(def string) string {
	decoded := strings.ReplaceAll(def, "&amp;", "&")
	if decoded == "" || strings.Contains(decoded, ",") {
		return decoded
	}
	values := make(map[string]string)
	for _, pair := range strings.Split(decoded, "&") {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			values[key] = value
		}
	}
	// Do not reinterpret unrelated malformed records as a new character.
	if !strings.Contains(values["hr"], "/") && !strings.Contains(values["hd"], "/") {
		return decoded
	}
	style := func(part string) string {
		raw, _, _ := strings.Cut(values[part], "/")
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			n = 1
		}
		return fmt.Sprintf("%03d", n)
	}

	colorIndex := func(part string, defaultIdx int) int {
		_, rawColor, ok := strings.Cut(values[part], "/")
		if !ok || rawColor == "" {
			return defaultIdx
		}
		n, err := strconv.Atoi(rawColor)
		if err != nil || n < 1 {
			return defaultIdx
		}
		return n
	}

	skins := []string{
		"251,226,210", "247,213,192", "240,196,168", "232,180,149",
		"213,157,123", "191,132,97", "168,110,73", "138,84,51",
		"109,64,36", "82,47,24", "229,222,212", "158,192,219",
	}

	hairColors := []string{
		"30,30,30",    // 1: Black
		"75,50,35",    // 2: Dark Brown
		"115,75,45",   // 3: Brown
		"160,110,65",  // 4: Light Brown
		"230,190,110", // 5: Blonde
		"245,230,180", // 6: Platinum Blonde
		"140,40,30",   // 7: Dark Red / Auburn
		"200,80,40",   // 8: Ginger / Orange Red
		"180,180,180", // 9: Grey / Silver
		"220,100,160", // 10: Pink
		"50,110,180",  // 11: Blue
		"50,150,80",   // 12: Green
	}

	shirtColors := []string{
		"220,30,30",   // 1: Classic Coke Red
		"255,255,255", // 2: White
		"35,35,35",    // 3: Black
		"30,90,190",   // 4: Royal Blue
		"40,160,70",   // 5: Kelly Green
		"245,200,30",  // 6: Golden Yellow
		"235,120,30",  // 7: Orange
		"140,50,160",  // 8: Purple
		"230,110,160", // 9: Hot Pink
		"110,65,40",   // 10: Brown
		"130,140,150", // 11: Slate Grey
		"30,160,170",  // 12: Teal
	}

	pantsColors := []string{
		"45,75,135",   // 1: Classic Blue Denim
		"30,30,30",    // 2: Black Denim
		"185,165,130", // 3: Khaki / Tan
		"230,230,230", // 4: White
		"100,105,115", // 5: Charcoal Grey
		"180,40,40",   // 6: Red
		"50,110,60",   // 7: Olive Green
		"85,55,35",    // 8: Dark Brown
	}

	shoeColors := []string{
		"30,30,30",    // 1: Black
		"240,240,240", // 2: White
		"120,70,35",   // 3: Brown
		"190,30,30",   // 4: Red
		"35,70,140",   // 5: Blue
		"160,160,160", // 6: Grey
	}

	pickColor := func(colors []string, idx int) string {
		if idx < 1 || idx > len(colors) {
			idx = 1
		}
		return colors[idx-1]
	}

	skinID, _ := strconv.Atoi(style("hd"))
	skin := pickColor(skins, skinID)
	hairColor := pickColor(hairColors, colorIndex("hr", 1))
	shirtColor := pickColor(shirtColors, colorIndex("ch", 1))
	pantsColor := pickColor(pantsColors, colorIndex("lg", 1))
	shoeColor := pickColor(shoeColors, colorIndex("sh", 1))

	body := "001"
	if values["g"] == "2" {
		body = "002"
	}
	face := "fc"
	if values["fa"] != "" {
		face = "fa"
	}
	parts := []string{
		"hr=" + style("hr") + "/" + hairColor,
		"hd=001/" + skin,
		"ey=" + style("ey") + "/" + skin,
		"fc=" + style(face) + "/" + skin,
		"bd=" + body + "/" + skin,
		"lh=001/" + skin,
		"rh=001/" + skin,
		"ch=" + style("ch") + "/" + shirtColor,
		"ls=" + style("ch") + "/" + shirtColor,
		"rs=" + style("ch") + "/" + shirtColor,
		"lg=" + style("lg") + "/" + pantsColor,
		"sh=" + style("sh") + "/" + shoeColor,
	}
	return strings.Join(parts, "&")
}
