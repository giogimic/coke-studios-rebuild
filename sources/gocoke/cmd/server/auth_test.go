package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPasswordHashSecurity(t *testing.T) {
	pass := "UltraSecretPassword123!"
	hash, err := HashPassword(pass)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !strings.HasPrefix(hash, "pbkdf2_sha256$") {
		t.Errorf("expected hash to have pbkdf2_sha256 prefix, got: %s", hash)
	}

	// Correct password passes
	if !VerifyPassword(pass, hash) {
		t.Errorf("VerifyPassword failed for correct password")
	}

	// Wrong password fails
	if VerifyPassword("WrongPassword!", hash) {
		t.Errorf("VerifyPassword succeeded for incorrect password")
	}

	// Empty password fails
	if VerifyPassword("", hash) {
		t.Errorf("VerifyPassword succeeded for empty password")
	}

	// Empty hash fails
	if VerifyPassword(pass, "") {
		t.Errorf("VerifyPassword succeeded for empty hash")
	}
}

func TestLoginRejectsWrongCredentials(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)

	// 1. Register a test player "Alice" with password "passAlice123"
	_, err := srv.store.RegisterDenizen("Alice", "passAlice123", "1", "defAlice", "Mission Alice")
	if err != nil {
		t.Fatalf("failed to register Alice: %v", err)
	}

	// 2. Try JSP login with wrong password -> ErrorCode 1
	reqWrong := httptest.NewRequest("GET", "/sf/ssldocs/loginDenizen.jsp?user=Alice&password=WrongPassword", nil)
	wWrong := httptest.NewRecorder()
	srv.handleLoginDenizen(wWrong, reqWrong)

	bodyWrong := wWrong.Body.String()
	if !strings.Contains(bodyWrong, "<ErrorCode>1</ErrorCode>") {
		t.Errorf("expected ErrorCode 1 on wrong credentials, got: %s", bodyWrong)
	}

	// 3. Try JSP login for non-existent user "NonExistentUser" -> ErrorCode 1
	reqMissing := httptest.NewRequest("GET", "/sf/ssldocs/loginDenizen.jsp?user=NonExistentUser&password=SomePassword", nil)
	wMissing := httptest.NewRecorder()
	srv.handleLoginDenizen(wMissing, reqMissing)

	bodyMissing := wMissing.Body.String()
	if !strings.Contains(bodyMissing, "<ErrorCode>1</ErrorCode>") {
		t.Errorf("expected ErrorCode 1 for non-existent user, got: %s", bodyMissing)
	}

	// 4. Try legacy login with wrong password -> LOGINFAILED
	form := url.Values{}
	form.Set("screenName", "Alice")
	form.Set("password", "WrongPassword")
	reqLegacyWrong := httptest.NewRequest("POST", "/login/Login", strings.NewReader(form.Encode()))
	reqLegacyWrong.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wLegacyWrong := httptest.NewRecorder()
	srv.handleLegacyLogin(wLegacyWrong, reqLegacyWrong)

	bodyLegacy := wLegacyWrong.Body.String()
	if !strings.Contains(bodyLegacy, "rCode=LOGINFAILED") {
		t.Errorf("expected rCode=LOGINFAILED for wrong credentials, got: %s", bodyLegacy)
	}

	// 5. Try JSP login with correct credentials -> ErrorCode 0
	reqCorrect := httptest.NewRequest("GET", "/sf/ssldocs/loginDenizen.jsp?user=Alice&password=passAlice123", nil)
	wCorrect := httptest.NewRecorder()
	srv.handleLoginDenizen(wCorrect, reqCorrect)

	bodyCorrect := wCorrect.Body.String()
	if !strings.Contains(bodyCorrect, "<ErrorCode>0</ErrorCode>") {
		t.Errorf("expected ErrorCode 0 on valid credentials, got: %s", bodyCorrect)
	}
	if !strings.Contains(bodyCorrect, "<ScreenName>Alice</ScreenName>") {
		t.Errorf("expected ScreenName Alice, got: %s", bodyCorrect)
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)

	d, err := srv.store.RegisterDenizen("Bob", "passBob123", "1", "defBob", "Mission Bob")
	if err != nil {
		t.Fatalf("failed to register Bob: %v", err)
	}

	// Create an already-expired session (-1 hour)
	sess, err := srv.store.CreateSession(d.ID, d.ScreenName, -1*time.Hour)
	if err != nil {
		t.Fatalf("failed to create expired session: %v", err)
	}

	// HTTP request with expired session
	req := httptest.NewRequest("GET", "/sf/ssldocs/loginDenizen.jsp?sid="+sess.Token, nil)
	w := httptest.NewRecorder()
	srv.handleLoginDenizen(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "<ErrorCode>1</ErrorCode>") {
		t.Errorf("expected ErrorCode 1 for expired session, got: %s", body)
	}

	// Realtime socket login with expired session
	_, err = srv.authenticateRealtime("Bob", sess.Token)
	if err == nil {
		t.Errorf("expected authenticateRealtime to reject expired session")
	}
}

func TestRealtimeRejectsForgedIdentity(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)

	// Alice and Bob register
	alice, _ := srv.store.RegisterDenizen("Alice", "passAlice123", "1", "defAlice", "Mission Alice")
	bob, _ := srv.store.RegisterDenizen("Bob", "passBob123", "1", "defBob", "Mission Bob")

	// Alice gets a valid session
	aliceSess, _ := srv.store.CreateSession(alice.ID, alice.ScreenName, 24*time.Hour)

	// Bob gets a valid session
	bobSess, _ := srv.store.CreateSession(bob.ID, bob.ScreenName, 24*time.Hour)

	// 1. Alice connecting with Alice's session -> PASS
	d1, err := srv.authenticateRealtime("Alice", aliceSess.Token)
	if err != nil || d1.ScreenName != "Alice" {
		t.Fatalf("expected Alice to authenticate with Alice's session, got: %v", err)
	}

	// 2. Alice socket claiming to be Bob using Alice's session token -> MUST FAIL
	_, err = srv.authenticateRealtime("Bob", aliceSess.Token)
	if err == nil {
		t.Errorf("security vulnerability: Bob authenticated using Alice's session token!")
	}

	// 3. Forged fake secret -> MUST FAIL
	_, err = srv.authenticateRealtime("Bob", "random_fake_token_12345")
	if err == nil {
		t.Errorf("expected forged token to be rejected")
	}

	// 4. Bob connecting with Bob's session -> PASS
	d2, err := srv.authenticateRealtime("Bob", bobSess.Token)
	if err != nil || d2.ScreenName != "Bob" {
		t.Fatalf("expected Bob to authenticate with Bob's session, got: %v", err)
	}
}

func TestTwoPlayersHaveIsolatedState(t *testing.T) {
	srv := NewServer("./web", "127.0.0.1", "8080", nil)
	handler := srv.Handler()

	// 1. Register Alice
	formAlice := url.Values{}
	formAlice.Set("screenName", "AliceIso")
	formAlice.Set("password", "passAlice123")
	formAlice.Set("gender", "2")
	formAlice.Set("avatarDef", "avatar_alice")
	formAlice.Set("mission", "Alice in CokeLand")

	reqRegAlice := httptest.NewRequest("POST", "/api/denizen/register", strings.NewReader(formAlice.Encode()))
	reqRegAlice.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wRegAlice := httptest.NewRecorder()
	handler.ServeHTTP(wRegAlice, reqRegAlice)

	var aliceData map[string]any
	_ = json.Unmarshal(wRegAlice.Body.Bytes(), &aliceData)
	aliceToken := aliceData["sw2"].(string)

	// 2. Register Bob
	formBob := url.Values{}
	formBob.Set("screenName", "BobIso")
	formBob.Set("password", "passBob123")
	formBob.Set("gender", "1")
	formBob.Set("avatarDef", "avatar_bob")
	formBob.Set("mission", "Bob the Builder")

	reqRegBob := httptest.NewRequest("POST", "/api/denizen/register", strings.NewReader(formBob.Encode()))
	reqRegBob.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wRegBob := httptest.NewRecorder()
	handler.ServeHTTP(wRegBob, reqRegBob)

	var bobData map[string]any
	_ = json.Unmarshal(wRegBob.Body.Bytes(), &bobData)
	bobToken := bobData["sw2"].(string)

	// 3. Verify Alice cannot update Bob's profile using Alice's token
	updateBobForm := url.Values{}
	updateBobForm.Set("user", "BobIso")
	updateBobForm.Set("mission", "Hacked by Alice!")

	reqUpdateBob := httptest.NewRequest("POST", "/api/denizen/update?sid="+aliceToken, strings.NewReader(updateBobForm.Encode()))
	reqUpdateBob.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wUpdateBob := httptest.NewRecorder()
	handler.ServeHTTP(wUpdateBob, reqUpdateBob)

	if wUpdateBob.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden when Alice tries to update Bob, got: %d", wUpdateBob.Code)
	}

	// Verify Bob's profile was unchanged
	bobProfile, err := srv.store.GetDenizenByName("BobIso")
	if err != nil {
		t.Fatalf("failed to get Bob's profile: %v", err)
	}
	if bobProfile.Mission != "Bob the Builder" {
		t.Errorf("Bob's mission was modified! Got: %s", bobProfile.Mission)
	}

	// 4. Verify Alice cannot read Bob's private messages using Alice's token
	reqBobMsgs := httptest.NewRequest("GET", "/api/messenger/messages?user=BobIso&sid="+aliceToken, nil)
	wBobMsgs := httptest.NewRecorder()
	handler.ServeHTTP(wBobMsgs, reqBobMsgs)

	if wBobMsgs.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden when Alice tries to view Bob's messages, got: %d", wBobMsgs.Code)
	}

	// 5. Verify isolated backpack items (Alice's purchase goes to Alice only)
	_, _ = srv.store.AddBackpackItem("AliceIso", 45, map[string]any{"color": "red"})
	aliceBackpack, _ := srv.store.GetBackpack("AliceIso")
	bobBackpack, _ := srv.store.GetBackpack("BobIso")

	if len(aliceBackpack) != 1 {
		t.Errorf("expected Alice to have 1 item, got %d", len(aliceBackpack))
	}
	if len(bobBackpack) != 0 {
		t.Errorf("expected Bob to have 0 items, got %d", len(bobBackpack))
	}

	// 6. Verify duplicate registration fails
	reqDup := httptest.NewRequest("POST", "/api/denizen/register", strings.NewReader(formAlice.Encode()))
	reqDup.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wDup := httptest.NewRecorder()
	handler.ServeHTTP(wDup, reqDup)

	if wDup.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request on duplicate registration, got: %d", wDup.Code)
	}

	// Check token usage
	if aliceToken == bobToken {
		t.Errorf("Alice and Bob must not share the same session token!")
	}
}
