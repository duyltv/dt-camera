package httpapi

import (
	"net/http"
	"strings"
	"time"
)

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

const genericLoginError = "invalid login or password"
const rateLimitedMessage = "too many login attempts, please try again later"

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	login := strings.TrimSpace(req.Login)
	if login == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "login and password are required", nil)
		return
	}

	ip := clientIP(r)
	loginKey := login

	// Rate limit check happens before any user lookup so that failed lookups
	// can't be used to enumerate valid accounts.
	if s.loginLimiter != nil && s.loginLimiter.isBlocked(ip, loginKey) {
		s.recordFailedLogin(r, ip, loginKey, "rate_limited", nil)
		writeError(w, http.StatusTooManyRequests, "rate_limited", rateLimitedMessage, nil)
		return
	}

	user, err := s.findUserByLogin(r.Context(), login)
	if err != nil {
		if s.loginLimiter != nil && s.loginLimiter.recordFailure(ip, loginKey) {
			s.recordFailedLogin(r, ip, loginKey, "rate_limited", nil)
			writeError(w, http.StatusTooManyRequests, "rate_limited", rateLimitedMessage, nil)
			return
		}
		s.recordFailedLogin(r, ip, loginKey, "user_not_found", nil)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", genericLoginError, nil)
		return
	}
	if !user.Active {
		if s.loginLimiter != nil && s.loginLimiter.recordFailure(ip, loginKey) {
			s.recordFailedLogin(r, ip, loginKey, "rate_limited", nil)
			writeError(w, http.StatusTooManyRequests, "rate_limited", rateLimitedMessage, nil)
			return
		}
		s.recordFailedLogin(r, ip, loginKey, "inactive", &user.ID)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", genericLoginError, nil)
		return
	}
	if !verifyPassword(user.PasswordHash, req.Password) {
		if s.loginLimiter != nil && s.loginLimiter.recordFailure(ip, loginKey) {
			s.recordFailedLogin(r, ip, loginKey, "rate_limited", &user.ID)
			writeError(w, http.StatusTooManyRequests, "rate_limited", rateLimitedMessage, nil)
			return
		}
		s.recordFailedLogin(r, ip, loginKey, "wrong_password", &user.ID)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", genericLoginError, nil)
		return
	}

	token, err := newSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session_error", "could not create session", nil)
		return
	}
	expires := time.Now().UTC().Add(time.Duration(s.cfg.SessionTTLHours) * time.Hour)
	tokenPrefix := token
	if len(tokenPrefix) > 12 {
		tokenPrefix = tokenPrefix[:12]
	}
	if _, err := s.db.ExecContext(r.Context(), `
		INSERT INTO sessions (user_id, token_hash, token_prefix, expires_at, last_seen_at)
		VALUES ($1, $2, $3, $4, now())
	`, user.ID, hashSessionToken(token), tokenPrefix, expires); err != nil {
		writeDBError(w, err)
		return
	}
	if s.loginLimiter != nil {
		s.loginLimiter.recordSuccess(ip, loginKey)
	}
	s.recordEvent(r, eventRecord{
		EventType:   "auth.login",
		EntityType:  "user",
		EntityID:    &user.ID,
		ActorUserID: &user.ID,
		Message:     "user logged in",
	})
	s.setSessionCookie(w, token, expires)
	writeJSON(w, http.StatusOK, map[string]any{"user": user.userResponse, "expires_at": expires})
}

// recordFailedLogin writes an auth.login_failed event. The metadata includes a
// machine-readable reason and the client IP, but never the attempted password
// or any session token material.
func (s *Server) recordFailedLogin(r *http.Request, ip, login, reason string, userID *string) {
	metadata := map[string]any{
		"reason": reason,
		"ip":     ip,
	}
	if login != "" {
		// Store a short hash of the attempted login for grouping without
		// retaining the raw identifier in the event log.
		metadata["login_fingerprint"] = hashSessionToken(strings.ToLower(strings.TrimSpace(login)))[:16]
	}
	severity := "info"
	if reason == "rate_limited" {
		severity = "warning"
	}
	s.recordEvent(r, eventRecord{
		EventType:   "auth.login_failed",
		EntityType:  "user",
		EntityID:    userID,
		ActorUserID: userID,
		Severity:    severity,
		Message:     "login attempt failed",
		Metadata:    metadata,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if user, err := s.findUserBySession(r.Context(), cookie.Value); err == nil {
			s.recordEvent(r, eventRecord{
				EventType:   "auth.logout",
				EntityType:  "user",
				EntityID:    &user.ID,
				ActorUserID: &user.ID,
				Message:     "user logged out",
			})
		}
		_, _ = s.db.ExecContext(r.Context(), `UPDATE sessions SET revoked_at = now() WHERE token_hash = $1`, hashSessionToken(cookie.Value))
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"logged_out": true})
}

func (s *Server) handleCurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	_, _ = s.db.ExecContext(r.Context(), `UPDATE sessions SET last_seen_at = now() WHERE token_hash = $1`, hashSessionToken(mustCookieValue(r)))
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func mustCookieValue(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
