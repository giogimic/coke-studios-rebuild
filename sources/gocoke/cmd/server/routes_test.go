package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
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
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
