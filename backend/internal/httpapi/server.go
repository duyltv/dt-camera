package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/dt-camera/backend/internal/version"
)

type Server struct {
	recordingsPath string
	db             *sql.DB
	startedAt      time.Time
	cfg            Config
	streams        *hlsManager
	loginLimiter   *loginRateLimiter
	hlsWarmStop    chan struct{}
	sessionStop    chan struct{}
	alertStop      chan struct{}
	retentionStop  chan struct{}
	retentionLast  retentionSummary
}

type Config struct {
	FrontendOrigin         string
	BootstrapAdminEmail    string
	BootstrapAdminPassword string
	SessionTTLHours        int
	HLSRoot                string
	HLSInactivitySeconds   int
	HLSWarmEnabled         bool
	HLSWarmInterval        time.Duration
	HLSMaxLag              time.Duration
	PreviewCacheRoot       string
	LoginRateLimitPerKey   int
	LoginRateLimitPerIP    int
	LoginRateLimitWindow   time.Duration
	LoginRateLimitBlockFor time.Duration
	SessionCleanupInterval time.Duration
	CookieSecure           bool
	CookieDomain           string
	AlertEvalInterval      time.Duration
	RetentionInterval      time.Duration
}

func NewServer(recordingsPath string, db *sql.DB, cfg Config) *Server {
	if cfg.SessionTTLHours <= 0 {
		cfg.SessionTTLHours = 2160
	}
	if cfg.HLSRoot == "" {
		cfg.HLSRoot = "/tmp/dt-camera-hls"
	}
	if cfg.HLSInactivitySeconds <= 0 {
		cfg.HLSInactivitySeconds = 60
	}
	if cfg.HLSWarmInterval <= 0 {
		cfg.HLSWarmInterval = 15 * time.Second
	}
	if cfg.HLSMaxLag <= 0 {
		cfg.HLSMaxLag = 30 * time.Second
	}
	if cfg.PreviewCacheRoot == "" {
		cfg.PreviewCacheRoot = "/tmp/dt-camera-previews"
	}
	if cfg.LoginRateLimitPerKey <= 0 {
		cfg.LoginRateLimitPerKey = 10
	}
	if cfg.LoginRateLimitPerIP <= 0 {
		cfg.LoginRateLimitPerIP = cfg.LoginRateLimitPerKey * 5
	}
	if cfg.LoginRateLimitWindow <= 0 {
		cfg.LoginRateLimitWindow = time.Minute
	}
	if cfg.LoginRateLimitBlockFor <= 0 {
		cfg.LoginRateLimitBlockFor = 5 * time.Minute
	}
	if cfg.SessionCleanupInterval <= 0 {
		cfg.SessionCleanupInterval = 15 * time.Minute
	}
	if cfg.AlertEvalInterval <= 0 {
		cfg.AlertEvalInterval = 60 * time.Second
	}
	if cfg.RetentionInterval <= 0 {
		cfg.RetentionInterval = 6 * time.Hour
	}
	manager := newHLSManager(db, cfg.HLSRoot, time.Duration(cfg.HLSInactivitySeconds)*time.Second, cfg.HLSWarmEnabled, cfg.HLSMaxLag)
	limiter := newLoginRateLimiter(
		cfg.LoginRateLimitPerKey,
		cfg.LoginRateLimitPerIP,
		cfg.LoginRateLimitWindow,
		cfg.LoginRateLimitBlockFor,
	)
	srv := &Server{
		recordingsPath: recordingsPath,
		db:             db,
		startedAt:      time.Now().UTC(),
		cfg:            cfg,
		streams:        manager,
		loginLimiter:   limiter,
		hlsWarmStop:    make(chan struct{}),
		sessionStop:    make(chan struct{}),
		alertStop:      make(chan struct{}),
		retentionStop:  make(chan struct{}),
	}
	go manager.cleanupLoop(context.Background())
	if cfg.HLSWarmEnabled {
		go manager.warmLoop(context.Background(), cfg.HLSWarmInterval, srv.hlsWarmStop)
	}
	go srv.sessionCleanupLoop(context.Background(), cfg.SessionCleanupInterval, srv.sessionStop)
	go srv.alertEvaluationLoop(context.Background(), cfg.AlertEvalInterval, srv.alertStop)
	go srv.retentionLoopWithState(context.Background(), cfg.RetentionInterval, srv.retentionStop)
	return srv
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/logout", s.handleLogout)
	mux.HandleFunc("/api/auth/me", s.handleCurrentUser)
	mux.HandleFunc("/api/users", s.handleUsers)
	mux.HandleFunc("/api/users/", s.handleUserByID)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/recorder/status", s.handleRecorderStatus)
	mux.HandleFunc("/api/storage-locations", s.handleStorageLocations)
	mux.HandleFunc("/api/storage-locations/", s.handleStorageLocationByID)
	mux.HandleFunc("/api/cameras", s.handleCameras)
	mux.HandleFunc("/api/cameras/scan", s.handleCameraScan)
	mux.HandleFunc("/api/cameras/onvif/test", s.handleCameraONVIFTest)
	mux.HandleFunc("/api/cameras/onvif/preview", s.handleCameraONVIFPreview)
	mux.HandleFunc("/api/cameras/onvif/import", s.handleCameraONVIFImport)
	mux.HandleFunc("/api/cameras/", s.handleCameraByID)
	mux.HandleFunc("/api/layouts", s.handleLayouts)
	mux.HandleFunc("/api/layouts/", s.handleLayoutByID)
	mux.HandleFunc("/api/recordings/search", s.handleRecordingSearch)
	mux.HandleFunc("/api/recordings/timeline", s.handleRecordingTimeline)
	mux.HandleFunc("/api/recordings/", s.handleRecordingByID)
	mux.HandleFunc("/api/playback/prepare", s.handlePlaybackPrepare)
	mux.HandleFunc("/api/live/cameras/", s.handleLiveCameraByID)
	mux.HandleFunc("/api/live/layouts/", s.handleLiveLayoutByID)
	mux.HandleFunc("/api/alert-rules", s.handleAlertRules)
	mux.HandleFunc("/api/alert-rules/", s.handleAlertRuleByID)
	mux.HandleFunc("/api/alerts", s.handleAlerts)
	mux.HandleFunc("/api/alerts/", s.handleAlertByID)
	mux.HandleFunc("/api/notification-channels", s.handleNotificationChannels)
	mux.HandleFunc("/api/notification-channels/", s.handleNotificationChannelByID)
	mux.HandleFunc("/api/notification-rules", s.handleNotificationRules)
	mux.HandleFunc("/api/notification-rules/", s.handleNotificationRuleByID)
	mux.HandleFunc("/api/migrations", s.handleMigrations)
	mux.HandleFunc("/api/retention/status", s.handleRetentionStatus)
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/hls/", s.handleHLSFile)
	mux.HandleFunc("/", s.handleRoot)
	return s.withCORS(mux)
}

func (s *Server) ShutdownStreams() {
	if s.streams != nil {
		s.streams.stopAll()
		log.Printf("hls streams stopped")
	}
	if s.hlsWarmStop != nil {
		close(s.hlsWarmStop)
		s.hlsWarmStop = nil
	}
	if s.sessionStop != nil {
		close(s.sessionStop)
		s.sessionStop = nil
	}
	if s.alertStop != nil {
		close(s.alertStop)
		s.alertStop = nil
	}
	if s.retentionStop != nil {
		close(s.retentionStop)
		s.retentionStop = nil
	}
}

// sessionCleanupLoop periodically deletes expired sessions and writes a
// summary event so admins can see when cleanup ran and how much it removed.
func (s *Server) sessionCleanupLoop(ctx context.Context, interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.purgeExpiredSessions(ctx)
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.purgeExpiredSessions(ctx)
		}
	}
}

// purgeExpiredSessions removes expired and previously-revoked sessions whose
// expires_at is in the past and records a summary event.
func (s *Server) purgeExpiredSessions(ctx context.Context) {
	if s.db == nil {
		return
	}
	cancelCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := s.db.ExecContext(cancelCtx, `DELETE FROM sessions WHERE expires_at <= now()`)
	if err != nil {
		log.Printf("session cleanup failed: %v", err)
		_ = insertSystemEvent(cancelCtx, s.db, eventRecord{
			EventType:  "auth.session_cleanup",
			EntityType: "session",
			Severity:   "warning",
			Message:    "expired session cleanup failed",
			Metadata:   map[string]any{"error": err.Error()},
		})
		return
	}
	deleted, _ := result.RowsAffected()
	if deleted == 0 {
		return
	}
	log.Printf("session cleanup deleted %d expired sessions", deleted)
	_ = insertSystemEvent(cancelCtx, s.db, eventRecord{
		EventType:  "auth.session_cleanup",
		EntityType: "session",
		Severity:   "info",
		Message:    "expired sessions purged",
		Metadata:   map[string]any{"deleted": deleted, "interval_seconds": int(s.cfg.SessionCleanupInterval.Seconds())},
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dbStatus := "ok"
	appStatus := "ok"
	statusCode := http.StatusOK
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		dbStatus = "unhealthy"
		appStatus = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	}

	migMax, migCount, _ := s.fetchMigrationStats(ctx)
	writeJSON(w, statusCode, map[string]any{
		"database":           dbStatus,
		"status":             appStatus,
		"service":            "backend",
		"started_at":         s.startedAt.Format(time.RFC3339),
		"app_version":        version.Snapshot().AppVersion,
		"latest_migration":   migMax,
		"migrations_applied": migCount,
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "dt-camera-backend",
		"health":  "/healthz",
	})
}

type errorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func readJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, errorResponse{
		Error: apiError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func writeDBError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "resource not found", nil)
		return
	}
	writeError(w, http.StatusInternalServerError, "database_error", "database operation failed", nil)
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if s.cfg.FrontendOrigin != "" && origin == s.cfg.FrontendOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,PUT,DELETE,OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
