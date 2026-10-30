package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gocoke/internal/client/net/amf"
)

// TestPlayerRoutesUseGoClient verifies that player launch paths route exclusively
// to the authoritative Go WASM client and contain no engine picker or CokePhase fallback.
func TestPlayerRoutesUseGoClient(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)
	handler := srv.Handler()

	// 1. Verify GET /game/ serves Go WASM shell
	reqGame := httptest.NewRequest("GET", "/game/", nil)
	wGame := httptest.NewRecorder()
	handler.ServeHTTP(wGame, reqGame)

	respGame := wGame.Result()
	if respGame.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /game/, got %d", respGame.StatusCode)
	}
	bodyGame := wGame.Body.String()
	if !strings.Contains(bodyGame, "wasm_exec.js") {
		t.Errorf("expected /game/ to load wasm_exec.js, got: %s", bodyGame[:min(200, len(bodyGame))])
	}
	if !strings.Contains(bodyGame, "main.wasm") {
		t.Errorf("expected /game/ to fetch main.wasm")
	}
	if strings.Contains(bodyGame, "cokephase") || strings.Contains(bodyGame, "CokePhase") {
		t.Errorf("/game/ must not contain CokePhase fallback references")
	}

	// 2. Verify GET / serves the portal and frames exclusively the Go game
	reqPortal := httptest.NewRequest("GET", "/", nil)
	wPortal := httptest.NewRecorder()
	handler.ServeHTTP(wPortal, reqPortal)

	respPortal := wPortal.Result()
	if respPortal.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /, got %d", respPortal.StatusCode)
	}
	bodyPortal := wPortal.Body.String()
	if strings.Contains(bodyPortal, "engineSelector") {
		t.Errorf("portal index must not contain engineSelector dropdown")
	}
	if strings.Contains(bodyPortal, `src="/phase/`) {
		t.Errorf("portal game iframe must not default to /phase/")
	}
	if !strings.Contains(bodyPortal, `src="/game/`) {
		t.Errorf("portal game iframe must point to /game/")
	}

	// 3. Verify registration generates a Go client launch URL
	form := url.Values{}
	form.Set("screenName", "TestDenizen")
	form.Set("password", "secretpass123")
	form.Set("email", "test@example.com")
	form.Set("gender", "1")

	reqReg := httptest.NewRequest("POST", "/api/denizen/register", strings.NewReader(form.Encode()))
	reqReg.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wReg := httptest.NewRecorder()
	handler.ServeHTTP(wReg, reqReg)

	respReg := wReg.Result()
	if respReg.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from registration, got %d", respReg.StatusCode)
	}
	var regData map[string]any
	if err := json.Unmarshal(wReg.Body.Bytes(), &regData); err != nil {
		t.Fatalf("failed to decode register response: %v", err)
	}
	gameURL, _ := regData["gameUrl"].(string)
	if !strings.HasPrefix(gameURL, "/game/?") {
		t.Errorf("expected registration gameUrl to start with /game/?, got: %s", gameURL)
	}
	if strings.Contains(gameURL, "/phase/") {
		t.Errorf("registration gameUrl must not contain /phase/")
	}

	// 4. Verify screensavers route
	reqScreensaver := httptest.NewRequest("GET", "/screensavers/lights_out.swf", nil)
	wScreensaver := httptest.NewRecorder()
	handler.ServeHTTP(wScreensaver, reqScreensaver)
	if wScreensaver.Code != http.StatusOK {
		t.Errorf("expected 200 OK from /screensavers/lights_out.swf, got %d", wScreensaver.Code)
	}
}

func TestCreateStudioAPI(t *testing.T) {
	tempDB := t.TempDir() + "/test_studio_create.db"
	store, err := OpenStore(tempDB)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	srv := NewServer("./web", "127.0.0.1", "8080", store)
	handler := srv.Handler()

	// Register user
	d, err := store.RegisterDenizen("StudioBuilder", "Pass123!", "1", "hr=001/1&hd=001/1", "Building Studios")
	if err != nil {
		t.Fatal(err)
	}

	sess, err := store.CreateSession(d.ID, d.ScreenName, defaultSessionTTL)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Unauthorized request must fail
	reqUnauth := httptest.NewRequest("POST", "/api/studio/create", strings.NewReader(`{"roomType":"studio_model_b"}`))
	reqUnauth.Header.Set("Content-Type", "application/json")
	wUnauth := httptest.NewRecorder()
	handler.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", wUnauth.Code)
	}

	// 2. Authorized request must succeed and register room
	payload := `{"studioName":"Builder Lounge","roomType":"studio_model_b"}`
	reqAuth := httptest.NewRequest("POST", "/api/studio/create", strings.NewReader(payload))
	reqAuth.Header.Set("Content-Type", "application/json")
	reqAuth.Header.Set("Authorization", "Bearer "+sess.Token)
	wAuth := httptest.NewRecorder()
	handler.ServeHTTP(wAuth, reqAuth)

	if wAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", wAuth.Code, wAuth.Body.String())
	}

	var res map[string]any
	if err := json.Unmarshal(wAuth.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res["status"] != "ok" || res["studioId"] != "studio_studiobuilder" || int(res["layout"].(float64)) != 2 {
		t.Fatalf("unexpected studio creation result: %+v", res)
	}

	// Verify room is registered in server memory
	srv.mu.RLock()
	rm, ok := srv.Rooms["studio_studiobuilder"]
	srv.mu.RUnlock()
	if !ok || rm == nil {
		t.Fatal("created studio room was not registered in srv.Rooms")
	}
	if rm.Layout != 2 || rm.OwnerName != "StudioBuilder" {
		t.Fatalf("room properties mismatch: layout=%d, owner=%s", rm.Layout, rm.OwnerName)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestStudioAMFManagement(t *testing.T) {
	tempDB := t.TempDir() + "/test_studio_amf.db"
	store, err := OpenStore(tempDB)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	srv := NewServer("./web", "127.0.0.1", "8080", store)
	handler := srv.Handler()

	d, err := store.RegisterDenizen("AMFArchitect", "Pass123!", "1", "hr=001/1&hd=001/1", "Architect")
	if err != nil {
		t.Fatal(err)
	}

	sess, err := store.CreateSession(d.ID, d.ScreenName, defaultSessionTTL)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create Studio 1 via AMF (params: screenName, secret, name, desc, layout, location)
	createReq1, _ := amf.EncodeRemotingRequest("com.studiocom.sanfrancisco.studio.FlashStudioManager.createStudio",
		[]any{d.ScreenName, sess.Token, "Main Studio", "First Studio", float64(1), float64(1)})
	r1 := httptest.NewRequest("POST", "/sf/gateway", bytes.NewReader(createReq1))
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("AMF createStudio 1 failed: %d", w1.Code)
	}
	resVal1, err := amf.DecodeRemotingResponse(w1.Body.Bytes())
	if err != nil {
		t.Fatalf("decode AMF createStudio 1 failed: %v", err)
	}
	m1, ok := resVal1.(map[string]any)
	if !ok || m1["studioId"] == nil || m1["studioId"] == "" {
		t.Fatalf("unexpected AMF createStudio 1 response: %+v", resVal1)
	}
	stID1 := m1["studioId"].(string)

	// 2. Create Studio 2 via AMF (layout 3, location 2)
	createReq2, _ := amf.EncodeRemotingRequest("com.studiocom.sanfrancisco.studio.FlashStudioManager.createStudio",
		[]any{d.ScreenName, sess.Token, "Second Studio", "Second Studio Desc", float64(3), float64(2)})
	r2 := httptest.NewRequest("POST", "/sf/gateway", bytes.NewReader(createReq2))
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("AMF createStudio 2 failed: %d", w2.Code)
	}
	resVal2, err := amf.DecodeRemotingResponse(w2.Body.Bytes())
	if err != nil {
		t.Fatalf("decode AMF createStudio 2 failed: %v", err)
	}
	m2, ok := resVal2.(map[string]any)
	if !ok || m2["studioId"] == nil || m2["studioId"] == "" {
		t.Fatalf("unexpected AMF createStudio 2 response: %+v", resVal2)
	}
	stID2 := m2["studioId"].(string)

	if stID1 == stID2 {
		t.Fatalf("expected distinct studio IDs for user's multiple studios, got %s and %s", stID1, stID2)
	}

	// 3. Modify Studio 1 via AMF
	modReq, _ := amf.EncodeRemotingRequest("com.studiocom.sanfrancisco.studio.FlashStudioManager.modifyStudio",
		[]any{d.ScreenName, sess.Token, stID1, "Renamed Main Studio", "Updated Description"})
	rMod := httptest.NewRequest("POST", "/sf/gateway", bytes.NewReader(modReq))
	wMod := httptest.NewRecorder()
	handler.ServeHTTP(wMod, rMod)
	if wMod.Code != http.StatusOK {
		t.Fatalf("AMF modifyStudio failed: %d", wMod.Code)
	}

	// Verify in memory and in store
	srv.mu.RLock()
	rm1 := srv.Rooms[stID1]
	srv.mu.RUnlock()
	if rm1 == nil || rm1.Name != "Renamed Main Studio" {
		t.Fatalf("room name not updated in memory: %+v", rm1)
	}

	// 4. Delete Studio 2 via AMF
	delReq, _ := amf.EncodeRemotingRequest("com.studiocom.sanfrancisco.studio.FlashStudioManager.deleteStudio",
		[]any{d.ScreenName, sess.Token, stID2})
	rDel := httptest.NewRequest("POST", "/sf/gateway", bytes.NewReader(delReq))
	wDel := httptest.NewRecorder()
	handler.ServeHTTP(wDel, rDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("AMF deleteStudio failed: %d", wDel.Code)
	}

	srv.mu.RLock()
	_, stillExists := srv.Rooms[stID2]
	srv.mu.RUnlock()
	if stillExists {
		t.Fatalf("deleted studio %s still in srv.Rooms", stID2)
	}
}
