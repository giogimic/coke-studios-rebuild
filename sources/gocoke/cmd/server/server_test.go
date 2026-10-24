package main

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gocoke/internal/client/net/amf"
)

func TestLoginDenizen(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)

	// Test 1: Query param user=Matth
	req := httptest.NewRequest("GET", "/sf/ssldocs/loginDenizen.jsp?user=Matth&gender=M", nil)
	w := httptest.NewRecorder()
	srv.handleLoginDenizen(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<ScreenName>Matth</ScreenName>") {
		t.Errorf("expected ScreenName Matth, got: %s", body)
	}
	if !strings.Contains(body, "<ErrorCode>0</ErrorCode>") {
		t.Errorf("expected ErrorCode 0, got: %s", body)
	}

	// Test 2: Query param sidempty=Alice_token123
	req2 := httptest.NewRequest("GET", "/sf/ssldocs/loginDenizen.jsp?sidempty=Alice_token123", nil)
	w2 := httptest.NewRecorder()
	srv.handleLoginDenizen(w2, req2)
	body2 := w2.Body.String()
	if !strings.Contains(body2, "<ScreenName>Alice</ScreenName>") {
		t.Errorf("expected ScreenName Alice from sidempty, got: %s", body2)
	}
}

func TestAMFGateway(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)

	// Test getTip
	reqBytes, err := amf.EncodeRemotingRequest("com.studiocom.sanfrancisco.denizen.FlashDenizenManager.getTip", nil)
	if err != nil {
		t.Fatalf("encode remoting request failed: %v", err)
	}

	req := httptest.NewRequest("POST", "/sf/gateway", strings.NewReader(string(reqBytes)))
	req.Header.Set("Content-Type", "application/x-amf")
	w := httptest.NewRecorder()
	srv.handleAMFGateway(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	val, err := amf.DecodeRemotingResponse(w.Body.Bytes())
	if err != nil {
		t.Fatalf("decode remoting response failed: %v", err)
	}
	tipStr, ok := val.(string)
	if !ok || len(tipStr) == 0 {
		t.Errorf("unexpected tip value: %v", val)
	}

	// Test getBackpack
	reqBPBytes, _ := amf.EncodeRemotingRequest("com.studiocom.sanfrancisco.catalog.FlashPossessionManager.getBackpack", []any{"Adam", "secret"})
	reqBP := httptest.NewRequest("POST", "/sf/gateway", strings.NewReader(string(reqBPBytes)))
	wBP := httptest.NewRecorder()
	srv.handleAMFGateway(wBP, reqBP)

	valBP, err := amf.DecodeRemotingResponse(wBP.Body.Bytes())
	if err != nil {
		t.Fatalf("decode backpack response failed: %v", err)
	}
	bpMap, ok := valBP.(map[string]any)
	if !ok || bpMap["possessionlist"] == nil {
		t.Fatalf("expected possessionlist in backpack, got: %v", valBP)
	}
	items, ok := bpMap["possessionlist"].([]any)
	if !ok || len(items) == 0 {
		t.Errorf("expected non-empty possessionlist, got: %v", items)
	}
}

func TestMessageHandling(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)
	p := &Player{
		ID:         "test_p1",
		ScreenName: "Adam",
		RoomID:     "VIS$0",
	}
	rm := srv.GetOrCreateRoom("VIS$0")
	rm.Players[p.ID] = p

	// Test AV_GSP path parsing
	pathXML := `<Room.Action roomID="VIS$0"><AV_GSP><P z="4" x="5"/><P z="4" x="6"/></AV_GSP></Room.Action>`
	pts := parsePPoints(pathXML)
	if len(pts) != 2 {
		t.Fatalf("expected 2 points, got %d", len(pts))
	}
	if pts[1][0] != 6 || pts[1][1] != 4 {
		t.Errorf("expected destination (6, 4), got (%d, %d)", pts[1][0], pts[1][1])
	}

	// Test Chat packet
	testMsg := "Hello Coke Studios!"
	hexMsg := hex.EncodeToString([]byte(testMsg))
	chatXML := `<Room.Action roomID="VIS$0"><CH sm="speak" m="` + hexMsg + `"/></Room.Action>`
	srv.handlePlayerMessage(p, chatXML)

	// Test Furniture placement
	posXML := `<Room.Action roomID="VIS$0"><POS_STD st="0" d="2" ho="0" z="4" y="0" x="5" id="11427" cid="45"/></Room.Action>`
	srv.handlePlayerMessage(p, posXML)
	furni, err := srv.store.GetRoomFurniture("VIS$0")
	if err != nil || len(furni) != 1 {
		t.Fatalf("expected 1 furniture item placed in DB, got %d (err: %v)", len(furni), err)
	}

	// Test Furniture pickup
	pickupXML := `<Room.Action roomID="VIS$0"><POS_BKPK id="11427"/></Room.Action>`
	srv.handlePlayerMessage(p, pickupXML)
	furniAfter, _ := srv.store.GetRoomFurniture("VIS$0")
	if len(furniAfter) != 0 {
		t.Errorf("expected 0 furniture items after pickup, got %d", len(furniAfter))
	}

	// Test DJ Performance and Voting Loop
	cdpXML := `<Room.Action roomID="VIS$0"><CD_P s="^50,rockOne_guitar00{2,0{4,4{^" title="Rock Beats"/></Room.Action>`
	srv.handlePlayerMessage(p, cdpXML)
	if rm.Performance == nil || rm.Performance.DJ != "Adam" {
		t.Errorf("expected performance active for DJ Adam, got: %v", rm.Performance)
	}

	// Test Thumbs Up cheer vote from a second player
	p2 := &Player{ID: "test_p2", ScreenName: "Bob", RoomID: "VIS$0"}
	rm.Players[p2.ID] = p2
	voteXML := `<Room.Action roomID="VIS$0"><CD_TU/></Room.Action>`
	srv.handlePlayerMessage(p2, voteXML)
	if rm.Performance.Cheers != 1 {
		t.Errorf("expected 1 cheer vote, got: %d", rm.Performance.Cheers)
	}
	srv.stopPerformance(rm)
}

func TestDenizenProfileAPI(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)

	// 1. Get initial profile
	req := httptest.NewRequest("GET", "/api/denizen/profile?user=Tester", nil)
	w := httptest.NewRecorder()
	srv.handleDenizenProfile(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// 2. Update profile
	reqUp := httptest.NewRequest("POST", "/api/denizen/update?user=Tester&gender=2&avatarDef=hr=003/1&hd=002/1&ch=005/1&mission=Rockstar", nil)
	wUp := httptest.NewRecorder()
	srv.handleUpdateDenizen(wUp, reqUp)
	if wUp.Code != http.StatusOK {
		t.Fatalf("expected 200 from update, got %d", wUp.Code)
	}
	body := wUp.Body.String()
	if !strings.Contains(body, `"gender":2`) || !strings.Contains(body, "Rockstar") {
		t.Errorf("unexpected update response: %s", body)
	}

	// 3. Verify persistence
	d, err := srv.store.GetDenizenByName("Tester")
	if err != nil || d == nil {
		t.Fatalf("failed to retrieve updated denizen: %v", err)
	}
	if d.Gender != 2 || d.Mission != "Rockstar" {
		t.Errorf("expected updated fields, got: %+v", d)
	}
}

func TestMessengerAndDecor(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)

	// Ensure denizens exist
	_, _ = srv.store.GetOrCreateDenizen("UserA", "M", "defA", "Mission A")
	_, _ = srv.store.GetOrCreateDenizen("UserB", "F", "defB", "Mission B")

	// 1. Add friend relationship via HTTP API
	reqAdd := httptest.NewRequest("POST", "/api/messenger/friend/add?user=UserA&friend=UserB", nil)
	wAdd := httptest.NewRecorder()
	srv.handleMessengerAddFriend(wAdd, reqAdd)
	if wAdd.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wAdd.Code)
	}

	// 2. Query friends list
	reqFriends := httptest.NewRequest("GET", "/api/messenger/friends?user=UserA", nil)
	wFriends := httptest.NewRecorder()
	srv.handleMessengerFriends(wFriends, reqFriends)
	if !strings.Contains(wFriends.Body.String(), "UserB") {
		t.Errorf("expected UserB in friends list: %s", wFriends.Body.String())
	}

	// 3. Send message via HTTP API
	reqSend := httptest.NewRequest("POST", "/api/messenger/send?user=UserA&to=UserB&message=Meet+me+at+Studio+Wayne!", nil)
	wSend := httptest.NewRecorder()
	srv.handleMessengerSend(wSend, reqSend)
	if wSend.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wSend.Code)
	}

	// 4. Query messages for UserB
	reqMsgs := httptest.NewRequest("GET", "/api/messenger/messages?user=UserB", nil)
	wMsgs := httptest.NewRecorder()
	srv.handleMessengerMessages(wMsgs, reqMsgs)
	if !strings.Contains(wMsgs.Body.String(), "Meet me at Studio Wayne!") {
		t.Errorf("expected message content in response: %s", wMsgs.Body.String())
	}

	// 5. Studio Decor HTTP API
	reqDecor := httptest.NewRequest("POST", "/api/studio/decor?studioId=1&wallpaper=5&floor=3&doorbell=2", nil)
	wDecor := httptest.NewRecorder()
	srv.handleStudioDecor(wDecor, reqDecor)
	if wDecor.Code != http.StatusOK {
		t.Fatalf("expected 200 from studio decor, got %d", wDecor.Code)
	}
	rm := srv.GetOrCreateRoom("1")
	rm.mu.RLock()
	if rm.Wallpaper != 5 || rm.Floor != 3 || rm.Doorbell != 2 {
		t.Errorf("expected room decor (5,3,2), got (%d,%d,%d)", rm.Wallpaper, rm.Floor, rm.Doorbell)
	}
	rm.mu.RUnlock()

	// 6. AMF RPC test for getFriends
	reqAMFFriends, _ := amf.EncodeRemotingRequest("com.studiocom.sanfrancisco.denizen.FlashDenizenManager.getFriends", []any{"UserA"})
	reqAF := httptest.NewRequest("POST", "/sf/gateway", strings.NewReader(string(reqAMFFriends)))
	wAF := httptest.NewRecorder()
	srv.handleAMFGateway(wAF, reqAF)
	valAF, err := amf.DecodeRemotingResponse(wAF.Body.Bytes())
	if err != nil {
		t.Fatalf("AMF getFriends decode failed: %v", err)
	}
	friendsArr, ok := valAF.([]any)
	if !ok || len(friendsArr) == 0 {
		t.Fatalf("expected non-empty friends array via AMF, got: %v", valAF)
	}

	// 7. AMF RPC test for updateStudio
	reqAMFDecor, _ := amf.EncodeRemotingRequest("com.studiocom.sanfrancisco.studio.FlashStudioManager.updateStudio", []any{"1", float64(7), float64(4), float64(1)})
	reqAD := httptest.NewRequest("POST", "/sf/gateway", strings.NewReader(string(reqAMFDecor)))
	wAD := httptest.NewRecorder()
	srv.handleAMFGateway(wAD, reqAD)
	rm.mu.RLock()
	if rm.Wallpaper != 7 || rm.Floor != 4 {
		t.Errorf("expected AMF updated decor (7,4), got (%d,%d)", rm.Wallpaper, rm.Floor)
	}
	rm.mu.RUnlock()
}

func TestCDBurnAndModeration(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)

	// Ensure test user exists with initial balance
	d, err := srv.store.GetOrCreateDenizen("DJMaster", "M", "def", "Beats")
	if err != nil {
		t.Fatalf("failed to create denizen: %v", err)
	}
	initialDB := d.Decibels

	// 1. Test In-Game CD Burning HTTP API
	reqBurn := httptest.NewRequest("POST", "/api/mixer/burn?user=DJMaster&title=Neon+Drive&edl=^50,danceOne_bass00{4,0{^", nil)
	wBurn := httptest.NewRecorder()
	srv.handleMixerBurn(wBurn, reqBurn)
	if wBurn.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wBurn.Code)
	}
	body := wBurn.Body.String()
	if !strings.Contains(body, "Neon Drive") || !strings.Contains(body, `"catalog_item":89`) {
		t.Errorf("unexpected burn response: %s", body)
	}

	// Verify balance deduction (-10 dB)
	newDB, _, _ := srv.store.GetBalance("DJMaster")
	if newDB != initialDB-10 {
		t.Errorf("expected balance %d, got %d", initialDB-10, newDB)
	}

	// Verify backpack item was persisted
	items, err := srv.store.GetBackpack("DJMaster")
	if err != nil || len(items) == 0 {
		t.Fatalf("expected burned CD in backpack, got %v (err: %v)", items, err)
	}
	foundCD := false
	for _, it := range items {
		if it.CatalogItemID == 89 && it.Properties["songName"] == "Neon Drive" {
			foundCD = true
			break
		}
	}
	if !foundCD {
		t.Errorf("expected catalog item 89 with songName Neon Drive in backpack")
	}

	// 2. Test Studio Moderation Kick
	p1 := &Player{ID: "p_owner", ScreenName: "OwnerUser", RoomID: "studio_test"}
	p2 := &Player{ID: "p_guest", ScreenName: "TrollUser", RoomID: "studio_test"}
	room := srv.GetOrCreateRoom("studio_test")
	room.mu.Lock()
	room.Players[p1.ID] = p1
	room.Players[p2.ID] = p2
	room.mu.Unlock()

	srv.mu.Lock()
	srv.Players[p1.ID] = p1
	srv.Players[p2.ID] = p2
	srv.mu.Unlock()

	kickXML := `<Room.Action roomID="studio_test"><ST_KICK target="TrollUser"/></Room.Action>`
	srv.handlePlayerMessage(p1, kickXML)

	room.mu.RLock()
	if _, ok := room.Players[p2.ID]; ok {
		t.Errorf("expected TrollUser to be removed from room.Players")
	}
	room.mu.RUnlock()

	if p2.RoomID != "$LOBBY$" {
		t.Errorf("expected kicked player RoomID to be $LOBBY$, got: %s", p2.RoomID)
	}

	// 3. Test Doorbell Knock
	knockXML := `<Room.Action roomID="studio_test"><ST_KNOCK roomID="studio_test"/></Room.Action>`
	srv.handlePlayerMessage(p2, knockXML)
}

func TestRegisterDenizen(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)

	// Test 5-step registration payload
	regJSON := `{
		"screenName": "NeoCokeDJ",
		"password": "secretPassword123",
		"email": "neo@coke.com",
		"gender": 2,
		"avatarDef": "hr=002/255,100,50&ch=005/200,50,50&lg=003/50,50,200&sh=001/0,0,0",
		"mission": "Be the #1 DJ in Central Park",
		"studioType": "studio_model_rooftop",
		"promoCode": "COKE-MUSIC-2026"
	}`

	req := httptest.NewRequest("POST", "/api/denizen/register", strings.NewReader(regJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleRegisterDenizen(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}

	if resp["status"] != "success" {
		t.Errorf("expected status success, got %v", resp["status"])
	}
	if resp["screenName"] != "NeoCokeDJ" {
		t.Errorf("expected NeoCokeDJ, got %v", resp["screenName"])
	}
	if resp["studioId"] != "studio_neocokedj" {
		t.Errorf("expected studio_neocokedj, got %v", resp["studioId"])
	}

	// Verify denizen in store
	d, err := srv.store.GetDenizenByName("NeoCokeDJ")
	if err != nil {
		t.Fatalf("denizen not found in store: %v", err)
	}
	if d.Gender != 2 {
		t.Errorf("expected gender 2, got %d", d.Gender)
	}

	// Verify starting studio exists in room list
	room := srv.GetOrCreateRoom("studio_neocokedj")
	if room.Layout != 5 {
		t.Errorf("expected rooftop studio layout 5, got %d", room.Layout)
	}

	// Verify legacy login compatibility endpoint
	reqLogin := httptest.NewRequest("POST", "/login/Login?screenName=NeoCokeDJ", nil)
	wLogin := httptest.NewRecorder()
	srv.handleLegacyLogin(wLogin, reqLogin)
	if !strings.Contains(wLogin.Body.String(), "rCode=CMLOGGEDIN") {
		t.Errorf("expected CMLOGGEDIN in legacy login response, got: %s", wLogin.Body.String())
	}
}

func TestSoundLibraryResolver(t *testing.T) {
	srv := NewServer("../../web", "127.0.0.1", "8080", nil)

	// Test prefixed resolution
	reqPrefixed := httptest.NewRequest("GET", "/Lingo/soundlibrary/62_rockOne_beatz00.wav", nil)
	wPrefixed := httptest.NewRecorder()
	srv.handleSoundLibrary(wPrefixed, reqPrefixed)
	if wPrefixed.Code != http.StatusOK {
		t.Fatalf("expected 200 for prefixed sound, got %d", wPrefixed.Code)
	}

	// Test un-prefixed resolution
	reqUnprefixed := httptest.NewRequest("GET", "/sounds/rockOne_beatz00.wav", nil)
	wUnprefixed := httptest.NewRecorder()
	srv.handleSoundLibrary(wUnprefixed, reqUnprefixed)
	if wUnprefixed.Code != http.StatusOK {
		t.Fatalf("expected 200 for un-prefixed sound, got %d", wUnprefixed.Code)
	}
}

