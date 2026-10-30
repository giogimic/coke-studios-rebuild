package avatarengine

import "testing"

func TestPortalDefinitionProducesVisibleBody(t *testing.T) {
	parts, err := ParseAttributeString("hr=004/3&hd=003/1&ch=006/1&lg=002/1&sh=002/1&fa=002/1&ey=001/1&g=1")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"bd", "hd", "lh", "rh", "ch", "ls", "rs", "lg", "sh", "fc", "ey", "hr"} {
		if _, ok := parts[part]; !ok {
			t.Errorf("registered avatar is missing %s", part)
		}
	}
	if parts["hr"].Style != "004" || parts["ch"].Style != "006" {
		t.Error("selected styles were lost")
	}
}

func TestRGBDefinitionRetainsAppearance(t *testing.T) {
	parts, err := ParseAttributeString("hd=001/115,92,69&amp;hr=017/238,238,238")
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := parts["hd"].Color.RGBA()
	if r>>8 != 115 || g>>8 != 92 || b>>8 != 69 || parts["hr"].Style != "017" {
		t.Fatal("original RGB appearance changed")
	}
}
