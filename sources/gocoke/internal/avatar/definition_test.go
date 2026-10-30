package avatar

import (
	"strings"
	"testing"
)

func TestNormalizeDefinitionPreservesFullRGB(t *testing.T) {
	full := "hr=005/180,90,40&hd=001/245,226,203&ey=001/60,40,20&fc=001/245,226,203&bd=001/245,226,203&lh=001/245,226,203&rh=001/245,226,203&ch=003/220,30,30&ls=003/220,30,30&rs=003/220,30,30&lg=002/40,60,120&sh=001/20,20,20"
	got := NormalizeDefinition(full)
	if got != full {
		t.Fatalf("expected %q, got %q", full, got)
	}
}

func TestNormalizeDefinitionTranslatesNumericPalettes(t *testing.T) {
	// Hair 5 (blonde), Shirt 1 (red), Legs 1 (blue denim), Shoes 1 (black), Skin 1, Female (g=2)
	numeric := "hr=005/5&hd=001/1&ch=001/1&lg=001/1&sh=001/1&g=2"
	got := NormalizeDefinition(numeric)

	if !strings.Contains(got, "hr=005/230,190,110") {
		t.Errorf("expected blonde hair 230,190,110, got: %s", got)
	}
	if !strings.Contains(got, "ch=001/220,30,30") {
		t.Errorf("expected red shirt 220,30,30, got: %s", got)
	}
	if !strings.Contains(got, "lg=001/45,75,135") {
		t.Errorf("expected blue jeans 45,75,135, got: %s", got)
	}
	if !strings.Contains(got, "bd=002/") {
		t.Errorf("expected female body bd=002, got: %s", got)
	}
	// Must not contain pure white 255,255,255 for colored items
	if strings.Contains(got, "hr=005/255,255,255") {
		t.Errorf("hair was incorrectly hardcoded to white: %s", got)
	}
}

