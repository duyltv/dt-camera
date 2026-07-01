package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "dt_camera_session"

type contextKey string

const userContextKey contextKey = "user"

type userResponse struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	Username    *string   `json:"username,omitempty"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type authUser struct {
	userResponse
	PasswordHash string
}

func (s *Server) Bootstrap(ctx context.Context) error {
	email := strings.TrimSpace(s.cfg.BootstrapAdminEmail)
	password := s.cfg.BootstrapAdminPassword
	if email == "" && password == "" {
		return nil
	}
	if email == "" || password == "" {
		return fmt.Errorf("both BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD are required")
	}
	if len(password) < 12 {
		return fmt.Errorf("bootstrap admin password must be at least 12 characters")
	}

	var adminCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&adminCount); err != nil {
		return err
	}
	if adminCount > 0 {
		return nil
	}

	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO users (email, display_name, password_hash, role, is_active)
		VALUES ($1, $2, $3, 'admin', TRUE)
	`, email, "Bootstrap Admin", hash)
	return err
}

func hashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", fmt.Errorf("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func newSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Server) currentUser(r *http.Request) (userResponse, bool) {
	user, ok := r.Context().Value(userContextKey).(userResponse)
	return user, ok
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (userResponse, bool) {
	user, ok := s.authenticateRequest(w, r)
	if !ok {
		return userResponse{}, false
	}
	return user, true
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (userResponse, bool) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return userResponse{}, false
	}
	if user.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required", nil)
		return userResponse{}, false
	}
	return user, true
}

func (s *Server) authenticateRequest(w http.ResponseWriter, r *http.Request) (userResponse, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required", nil)
		return userResponse{}, false
	}
	user, err := s.findUserBySession(r.Context(), cookie.Value)
	if err != nil {
		s.clearSessionCookie(w)
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired session", nil)
		return userResponse{}, false
	}
	return user, true
}

func (s *Server) withUser(r *http.Request, user userResponse) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userContextKey, user))
}

func (s *Server) findUserBySession(ctx context.Context, token string) (userResponse, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.email, u.username, u.display_name, u.role, u.is_active, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
			AND s.revoked_at IS NULL
			AND s.expires_at > now()
			AND u.is_active = TRUE
	`, hashSessionToken(token))
	return scanUser(row)
}

func (s *Server) findUserByLogin(ctx context.Context, login string) (authUser, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, email, username, display_name, role, is_active, created_at, updated_at, password_hash
		FROM users
		WHERE lower(email) = lower($1) OR lower(COALESCE(username, '')) = lower($1)
	`, login)
	return scanAuthUser(row)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
	}
	if s.cfg.CookieDomain != "" {
		cookie.Domain = s.cfg.CookieDomain
	}
	http.SetCookie(w, cookie)
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
	}
	if s.cfg.CookieDomain != "" {
		cookie.Domain = s.cfg.CookieDomain
	}
	http.SetCookie(w, cookie)
}

func scanUser(scanner interface{ Scan(dest ...any) error }) (userResponse, error) {
	var user userResponse
	var username sql.NullString
	if err := scanner.Scan(&user.ID, &user.Email, &username, &user.DisplayName, &user.Role, &user.Active, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return userResponse{}, err
	}
	if username.Valid {
		user.Username = &username.String
	}
	return user, nil
}

func scanAuthUser(scanner interface{ Scan(dest ...any) error }) (authUser, error) {
	var user authUser
	var username sql.NullString
	if err := scanner.Scan(&user.ID, &user.Email, &username, &user.DisplayName, &user.Role, &user.Active, &user.CreatedAt, &user.UpdatedAt, &user.PasswordHash); err != nil {
		return authUser{}, err
	}
	if username.Valid {
		user.Username = &username.String
	}
	return user, nil
}
