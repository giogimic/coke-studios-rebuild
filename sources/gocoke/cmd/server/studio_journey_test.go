package main

import (
	"bytes"
	"encoding/json"
	"gocoke/internal/client/net/gateway"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

type handlerTransport struct{ handler http.Handler }

func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, req)
	return recorder.Result(), nil
}

func TestRegistrationLaunchAndStudioDirectory(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "journey.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := NewServer("../../web", "localhost", "8080", store)
	payload := `{"screenName":"StudioOwner","password":"Password123!","gender":"1","avatarDef":"hr=001/1&hd=001/1&ch=001/1&lg=001/1&sh=001/1&g=1","studioType":"studio_model_b"}`
	req := httptest.NewRequest("POST", "/api/denizen/register", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, req)
	var result map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["success"] != true {
		t.Fatalf("registration failed: %s", recorder.Body.String())
	}
	launch, _ := url.Parse(result["launchUrl"].(string))
	if launch.Query().Get("room") != result["studioId"] {
		t.Errorf("launch does not target the newly owned studio: %s", launch.Path)
	}
	if !strings.Contains(result["avatarDef"].(string), "bd=") {
		t.Error("registered profile has no body")
	}
	// Exercise the real Go gateway encoder/decoder against the actual HTTP handler,
	// without local sockets or a mocked AMF response.
	client := &http.Client{Transport: handlerTransport{srv.Handler()}}
	v, err := gateway.GetStudiosByOwnerName(client, "http://studio.test/sf/gateway", "StudioOwner")
	if err != nil {
		t.Fatal(err)
	}
	list, ok := v.([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("My Studios returned %T %v", v, v)
	}
	row := list[0].(map[string]any)
	if row["studioid"] != result["studioId"] || row["ownername"] != "StudioOwner" || row["layout"] != float64(2) {
		t.Fatalf("record does not match Go navigator fields: %v", row)
	}
	v, err = gateway.GetStudiosByOwnerName(client, "http://studio.test/sf/gateway", "SomeoneElse")
	if err != nil {
		t.Fatal(err)
	}
	if list, ok := v.([]any); !ok || len(list) != 0 {
		t.Fatalf("owner filter leaked other studios: %v", v)
	}
}
