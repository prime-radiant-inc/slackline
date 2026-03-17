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
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":            true,
			"token":         "xoxe-new-config-token",
			"refresh_token": "xoxe-new-refresh-token",
		}); err != nil {
			t.Errorf("Encode: %v", err)
		}
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

func TestRotateConfigToken_NetworkError(t *testing.T) {
	// Use a server that's already closed to force a network error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	_, _, err := RotateConfigToken(server.URL, "xoxe-refresh")
	if err == nil {
		t.Fatal("expected error for closed server, got nil")
	}
}

func TestRotateConfigToken_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("this is not json {{{"))
	}))
	defer server.Close()

	_, _, err := RotateConfigToken(server.URL, "xoxe-refresh")
	if err == nil {
		t.Fatal("expected error for invalid JSON response, got nil")
	}
}

func TestRotateConfigToken_MissingFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			// token and refresh_token deliberately omitted
		}); err != nil {
			t.Errorf("Encode: %v", err)
		}
	}))
	defer server.Close()

	token, refresh, err := RotateConfigToken(server.URL, "xoxe-refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With ok:true but missing fields, the function returns empty strings
	if token != "" {
		t.Errorf("token = %q, want empty", token)
	}
	if refresh != "" {
		t.Errorf("refresh = %q, want empty", refresh)
	}
}

func TestRotateConfigToken_Expired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "token_expired",
		}); err != nil {
			t.Errorf("Encode: %v", err)
		}
	}))
	defer server.Close()

	_, _, err := RotateConfigToken(server.URL, "xoxe-expired")
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}
