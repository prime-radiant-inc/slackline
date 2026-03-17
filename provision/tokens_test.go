package provision

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRotateConfigToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tooling.tokens.rotate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":            true,
			"token":         "xoxe-new-config-token",
			"refresh_token": "xoxe-new-refresh-token",
		})
	}))
	defer server.Close()

	newConfig, newRefresh, err := RotateConfigToken(server.URL, "xoxe-old-refresh")
	if err != nil {
		t.Fatalf("RotateConfigToken: %v", err)
	}
	if newConfig != "xoxe-new-config-token" {
		t.Errorf("config token = %q", newConfig)
	}
	if newRefresh != "xoxe-new-refresh-token" {
		t.Errorf("refresh token = %q", newRefresh)
	}
}

func TestRotateConfigToken_Expired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "token_expired",
		})
	}))
	defer server.Close()

	_, _, err := RotateConfigToken(server.URL, "xoxe-expired")
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}
