package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	appEnvProduction = "production"
	appEnvStaging    = "staging"
	appEnvDev        = "development"
)

// envCheckResult captures the outcome of a single validation rule.
type envCheckResult struct {
	name    string
	ok      bool
	level   string // "error" hard-stops production; "warning" is non-fatal
	message string
}

func (r envCheckResult) String() string {
	if r.ok {
		return fmt.Sprintf("ok  %s", r.name)
	}
	return fmt.Sprintf("%-7s %s: %s", strings.ToUpper(r.level), r.name, r.message)
}

// validateProductionEnv enforces a minimum safety bar when APP_ENV=production.
// Returns a slice of check results (never nil). If any result has ok=false and
// level="error", production startup must be aborted.
func validateProductionEnv() []envCheckResult {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if env == "" {
		env = appEnvDev
	}
	results := []envCheckResult{
		checkDatabaseURL(),
		checkRecordingsPath(),
		checkFrontendOrigin(),
	}

	// Cookie / session hardening is enforced only in production-like envs.
	if env == appEnvProduction || env == appEnvStaging {
		results = append(results, checkCookieSecure(env)...)
		results = append(results, checkBootstrapPassword())
		results = append(results, checkSessionTTL())
		results = append(results, checkPostgresPassword())
		results = append(results, checkCookieDomain(env))
	} else {
		results = append(results, envCheckResult{
			name:    "production hardening",
			ok:      true,
			level:   "warning",
			message: fmt.Sprintf("APP_ENV=%q: production-only safety checks skipped", env),
		})
	}
	return results
}

// mustHaveProductionSafety aborts startup if any production safety check
// failed at the "error" level.
func mustHaveProductionSafety(results []envCheckResult) {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if env != appEnvProduction {
		return
	}
	for _, r := range results {
		if !r.ok && r.level == "error" {
			fmt.Fprintf(os.Stderr, "\nFATAL: refusing to start in production with unsafe configuration.\n")
			for _, line := range results {
				fmt.Fprintln(os.Stderr, "  ", line)
			}
			fmt.Fprintln(os.Stderr, "\nSet the failing variables and retry, or unset APP_ENV if this is not production.")
			os.Exit(2)
		}
	}
}

// logEnvReport prints the validation summary. Errors that did not abort
// startup are still surfaced so operators can see them in container logs.
func logEnvReport(results []envCheckResult) {
	for _, r := range results {
		fmt.Println("env-check:", r)
	}
}

func checkDatabaseURL() envCheckResult {
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		return envCheckResult{
			name:    "DATABASE_URL",
			ok:      false,
			level:   "error",
			message: "DATABASE_URL is required",
		}
	}
	return envCheckResult{name: "DATABASE_URL", ok: true}
}

func checkRecordingsPath() envCheckResult {
	p := strings.TrimSpace(os.Getenv("RECORDINGS_PATH"))
	if p == "" {
		return envCheckResult{
			name:    "RECORDINGS_PATH",
			ok:      false,
			level:   "warning",
			message: "RECORDINGS_PATH is empty; using default /recordings",
		}
	}
	if strings.HasPrefix(p, "~/") {
		return envCheckResult{
			name:    "RECORDINGS_PATH",
			ok:      false,
			level:   "warning",
			message: "RECORDINGS_PATH starts with '~'; the backend container will not expand it. Use an absolute path.",
		}
	}
	return envCheckResult{name: "RECORDINGS_PATH", ok: true}
}

func checkFrontendOrigin() envCheckResult {
	o := strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN"))
	if o == "" {
		return envCheckResult{
			name:    "FRONTEND_ORIGIN",
			ok:      false,
			level:   "warning",
			message: "FRONTEND_ORIGIN is empty; CORS will reject all browser requests. Set it to the public URL of the frontend.",
		}
	}
	if o == "http://localhost:5173" {
		return envCheckResult{
			name:    "FRONTEND_ORIGIN",
			ok:      false,
			level:   "warning",
			message: "FRONTEND_ORIGIN is the dev default http://localhost:5173; set the real public URL behind HTTPS.",
		}
	}
	return envCheckResult{name: "FRONTEND_ORIGIN", ok: true}
}

func checkCookieSecure(env string) []envCheckResult {
	out := []envCheckResult{}
	v := strings.ToLower(strings.TrimSpace(os.Getenv("COOKIE_SECURE")))
	if v == "" {
		// Treat unset as false. Required in production.
		if env == appEnvProduction {
			out = append(out, envCheckResult{
				name:    "COOKIE_SECURE",
				ok:      false,
				level:   "error",
				message: "COOKIE_SECURE must be true in production so the session cookie is only sent over TLS",
			})
		} else {
			out = append(out, envCheckResult{
				name:    "COOKIE_SECURE",
				ok:      false,
				level:   "warning",
				message: "COOKIE_SECURE is unset; session cookie may be sent over plaintext in staging",
			})
		}
	} else {
		parsed, err := parseBoolish(v)
		if err != nil || !parsed {
			if env == appEnvProduction {
				out = append(out, envCheckResult{
					name:    "COOKIE_SECURE",
					ok:      false,
					level:   "error",
					message: fmt.Sprintf("COOKIE_SECURE=%q must be true in production", v),
				})
			} else {
				out = append(out, envCheckResult{
					name:    "COOKIE_SECURE",
					ok:      true,
					level:   "warning",
					message: fmt.Sprintf("COOKIE_SECURE=%q in staging; cookies may be sent over plaintext", v),
				})
			}
		} else {
			out = append(out, envCheckResult{name: "COOKIE_SECURE", ok: true})
		}
	}
	return out
}

func checkBootstrapPassword() envCheckResult {
	pw := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if pw == "" {
		// Empty bootstrap is fine if an admin already exists. Don't fail.
		return envCheckResult{name: "BOOTSTRAP_ADMIN_PASSWORD", ok: true}
	}
	if err := passwordStrengthError(pw); err != nil {
		return envCheckResult{
			name:    "BOOTSTRAP_ADMIN_PASSWORD",
			ok:      false,
			level:   "error",
			message: err.Error(),
		}
	}
	return envCheckResult{name: "BOOTSTRAP_ADMIN_PASSWORD", ok: true}
}

func checkSessionTTL() envCheckResult {
	v := strings.TrimSpace(os.Getenv("SESSION_TTL_HOURS"))
	if v == "" {
		return envCheckResult{
			name:    "SESSION_TTL_HOURS",
			ok:      false,
			level:   "warning",
			message: "SESSION_TTL_HOURS is unset; default of 168h (1 week) is too long for production; prefer 24h or less",
		}
	}
	var hours int
	if _, err := fmt.Sscanf(v, "%d", &hours); err != nil || hours <= 0 {
		return envCheckResult{
			name:    "SESSION_TTL_HOURS",
			ok:      false,
			level:   "warning",
			message: fmt.Sprintf("SESSION_TTL_HOURS=%q is not a positive integer; using the configured default", v),
		}
	}
	if hours > 168 {
		return envCheckResult{
			name:    "SESSION_TTL_HOURS",
			ok:      false,
			level:   "warning",
			message: fmt.Sprintf("SESSION_TTL_HOURS=%d is unusually long; consider 24h or less", hours),
		}
	}
	return envCheckResult{name: "SESSION_TTL_HOURS", ok: true}
}

func checkPostgresPassword() envCheckResult {
	url := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if url == "" {
		return envCheckResult{name: "POSTGRES_PASSWORD", ok: true}
	}
	// Find the password segment of the URL. After the "://" scheme separator,
	// the password is the substring between the LAST ":" (which separates it
	// from the username) and the LAST "@" (which separates it from the host).
	schemeSep := strings.Index(url, "://")
	if schemeSep == -1 {
		return envCheckResult{name: "POSTGRES_PASSWORD", ok: true}
	}
	rest := url[schemeSep+3:]
	at := strings.LastIndex(rest, "@")
	if at == -1 {
		return envCheckResult{name: "POSTGRES_PASSWORD", ok: true}
	}
	userInfo := rest[:at]
	colon := strings.LastIndex(userInfo, ":")
	if colon == -1 {
		// No password in URL; we can't grade it.
		return envCheckResult{name: "POSTGRES_PASSWORD", ok: true}
	}
	pass := userInfo[colon+1:]
	if err := passwordStrengthError(pass); err != nil {
		return envCheckResult{
			name:    "POSTGRES_PASSWORD",
			ok:      false,
			level:   "error",
			message: err.Error(),
		}
	}
	return envCheckResult{name: "POSTGRES_PASSWORD", ok: true}
}

func checkCookieDomain(env string) envCheckResult {
	dom := strings.TrimSpace(os.Getenv("COOKIE_DOMAIN"))
	if env == appEnvProduction && dom == "" {
		return envCheckResult{
			name:    "COOKIE_DOMAIN",
			ok:      false,
			level:   "warning",
			message: "COOKIE_DOMAIN is unset; cookies will not be sent across subdomains. Set this if backend and frontend are on different subdomains of the same parent domain.",
		}
	}
	return envCheckResult{name: "COOKIE_DOMAIN", ok: true}
}

// passwordStrengthError returns nil when the password meets the minimum bar
// (>= 12 chars, mixed character classes), otherwise a human-readable
// explanation. The same predicate is also used by Bootstrap().
func passwordStrengthError(pw string) error {
	const minLen = 12
	if len(pw) < minLen {
		return fmt.Errorf("password must be at least %d characters", minLen)
	}
	classes := 0
	if hasLower(pw) {
		classes++
	}
	if hasUpper(pw) {
		classes++
	}
	if hasDigit(pw) {
		classes++
	}
	if hasSymbol(pw) {
		classes++
	}
	if classes < 3 {
		return fmt.Errorf("password must include at least 3 of: lowercase, uppercase, digit, symbol")
	}
	return nil
}

func hasLower(s string) bool {
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return true
		}
	}
	return false
}
func hasUpper(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}
func hasDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}
func hasSymbol(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return true
		}
	}
	return false
}

func parseBoolish(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes", "y", "on":
		return true, nil
	case "0", "false", "f", "no", "n", "off", "":
		return false, nil
	}
	return false, fmt.Errorf("not a boolean: %q", v)
}