package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dt-camera/backend/internal/database"
	"github.com/dt-camera/backend/internal/httpapi"
)

func main() {
	addr := getenv("APP_ADDR", ":8080")
	recordingsPath := getenv("RECORDINGS_PATH", "/recordings")
	frontendOrigin := getenv("FRONTEND_ORIGIN", "http://localhost:5173")
	databaseURL := os.Getenv("DATABASE_URL")

	envResults := validateProductionEnv()
	logEnvReport(envResults)
	mustHaveProductionSafety(envResults)

	db, err := database.Open(databaseURL)
	if err != nil {
		log.Fatalf("database open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := database.Ping(ctx, db); err != nil {
		log.Fatalf("database health check failed: %v", err)
	}

	if err := database.RunMigrations(ctx, db); err != nil {
		log.Fatalf("database migrations failed: %v", err)
	}

	cookieSecure := getenvBool("COOKIE_SECURE", false)
	cookieDomain := os.Getenv("COOKIE_DOMAIN")

	server := httpapi.NewServer(recordingsPath, db, httpapi.Config{
		FrontendOrigin:         frontendOrigin,
		BootstrapAdminEmail:    os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
		SessionTTLHours:        getenvInt("SESSION_TTL_HOURS", 2160),
		HLSRoot:                getenv("HLS_ROOT", "/tmp/dt-camera-hls"),
		HLSInactivitySeconds:   getenvInt("HLS_INACTIVITY_SECONDS", 60),
		HLSWarmEnabled:         getenvBool("HLS_WARM_ENABLED", true),
		HLSWarmInterval:        getenvDuration("HLS_WARM_INTERVAL_SECONDS", 15*time.Second),
		HLSMaxLag:              getenvDuration("HLS_MAX_LAG_SECONDS", 15*time.Second),
		PreviewCacheRoot:       getenv("PREVIEW_CACHE_ROOT", "/tmp/dt-camera-previews"),
		LoginRateLimitPerKey:   getenvInt("LOGIN_RATE_LIMIT_PER_KEY", 10),
		LoginRateLimitPerIP:    getenvInt("LOGIN_RATE_LIMIT_PER_IP", 50),
		LoginRateLimitWindow:   getenvDuration("LOGIN_RATE_LIMIT_WINDOW", time.Minute),
		LoginRateLimitBlockFor: getenvDuration("LOGIN_RATE_LIMIT_BLOCK_FOR", 5*time.Minute),
		SessionCleanupInterval: getenvDuration("SESSION_CLEANUP_INTERVAL_SECONDS", 15*time.Minute),
		CookieSecure:           cookieSecure,
		CookieDomain:           cookieDomain,
		AlertEvalInterval:      getenvDuration("ALERT_EVAL_INTERVAL_SECONDS", 60*time.Second),
		RetentionInterval:      getenvDuration("RETENTION_INTERVAL_SECONDS", 6*time.Hour),
	})
	if err := server.Bootstrap(ctx); err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}

	defer server.ShutdownStreams()

	log.Printf("backend listening on %s", addr)
	log.Printf("recordings path mounted at %s", recordingsPath)
	log.Printf("database migrations applied")
	log.Printf("cookie secure=%t domain=%q", cookieSecure, cookieDomain)
	log.Printf("login rate limit: %d/%s per (ip,login), block %s",
		getenvInt("LOGIN_RATE_LIMIT_PER_KEY", 10),
		getenvDuration("LOGIN_RATE_LIMIT_WINDOW", time.Minute),
		getenvDuration("LOGIN_RATE_LIMIT_BLOCK_FOR", 5*time.Minute),
	)

	if err := http.ListenAndServe(addr, server.Routes()); err != nil {
		log.Fatalf("backend stopped: %v", err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func getenvBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
