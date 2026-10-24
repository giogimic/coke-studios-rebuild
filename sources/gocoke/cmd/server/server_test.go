package main

import (
	"encoding/hex"
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

