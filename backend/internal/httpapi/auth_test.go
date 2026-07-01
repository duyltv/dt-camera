package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPasswordHashing(t *testing.T) {
	hash, err := hashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	if hash == "correct-horse-battery" {
		t.Fatalf("password hash should not equal plaintext")
	}
	if !verifyPassword(hash, "correct-horse-battery") {
		t.Fatalf("expected password verification to pass")
	}
	if verifyPassword(hash, "wrong-password") {
		t.Fatalf("expected wrong password to fail")
	}
}

func TestAuthHandlersRequireMethods(t *testing.T) {
	server := &Server{cfg: Config{SessionTTLHours: 168}}

	recorder := httptest.NewRecorder()
	server.handleLogin(recorder, httptest.NewRequest(http.MethodGet, "/api/auth/login", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("login GET status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}

	recorder = httptest.NewRecorder()
	server.handleLogout(recorder, httptest.NewRequest(http.MethodGet, "/api/auth/logout", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("logout GET status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestCurrentUserRequiresSession(t *testing.T) {
	server := &Server{}
	recorder := httptest.NewRecorder()
	server.handleCurrentUser(recorder, httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("current user status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestAdminOnlyAPIRejection(t *testing.T) {
	server := &Server{}
	recorder := httptest.NewRecorder()
	server.handleCameras(recorder, httptest.NewRequest(http.MethodGet, "/api/cameras", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("camera API status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestPlaybackPermissionHelpers(t *testing.T) {
	admin := userResponse{ID: "admin", Role: "admin"}
	server := &Server{}
	if !server.canViewPlayback(httptest.NewRequest(http.MethodGet, "/", nil), admin, "camera") {
		t.Fatalf("admin should be allowed playback")
	}
}

func TestUserResponseDoesNotExposePasswordHash(t *testing.T) {
	response := userResponse{
		ID:          "018f5d67-89ab-4def-8123-456789abcdef",
		Email:       "user@example.com",
		DisplayName: "User",
		Role:        "user",
		Active:      true,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal user response: %v", err)
	}
	if strings.Contains(string(payload), "password") {
		t.Fatalf("user response leaked password data: %s", payload)
	}
}
