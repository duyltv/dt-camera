package main

import (
	"os"
	"strings"
	"testing"
)

// withEnv sets env vars for the duration of the test.
func withEnv(t *testing.T, kv map[string]string, fn func()) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
	fn()
}

func TestPasswordStrengthError(t *testing.T) {
	cases := []struct {
		name    string
		pw      string
		wantErr bool
	}{
		{"too short", "Ab1!", true},
		{"only lowercase", "abcdefghijkl", true},
		{"only digits", "123456789012", true},
		{"two classes", "abcdefgh1234", true}, // 12 chars but only lower+digit
		{"three classes lower+upper+digit", "Abcdefgh1234", false},
		{"three classes lower+upper+symbol", "Abcdefgh!@#$", false},
		{"four classes", "Abcdefg1234!@#", false},
		{"long four classes", "ThisIsAStrongPassword-1234!@#", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := passwordStrengthError(c.pw)
			if (err != nil) != c.wantErr {
				t.Fatalf("passwordStrengthError(%q) err=%v, wantErr=%v", c.pw, err, c.wantErr)
			}
		})
	}
}

func TestValidateProductionEnvRequiresDatabaseURL(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	os.Unsetenv("DATABASE_URL")
	results := validateProductionEnv()
	found := false
	for _, r := range results {
		if r.name == "DATABASE_URL" && !r.ok && r.level == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected DATABASE_URL error in production; results=%v", results)
	}
}

func TestValidateProductionEnvCookieSecureMustBeTrue(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://u:p@h/d")
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "StrongP@ssword-1234")
	t.Setenv("SESSION_TTL_HOURS", "24")
	results := validateProductionEnv()
	for _, r := range results {
		if r.name == "COOKIE_SECURE" {
			if r.ok {
				t.Fatalf("expected COOKIE_SECURE failure, got ok=true: %+v", r)
			}
			if r.level != "error" {
				t.Fatalf("expected COOKIE_SECURE level=error, got %q", r.level)
			}
		}
	}
}

func TestValidateProductionEnvCookieSecureTruePasses(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://u:p@h/d")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "StrongP@ssword-1234")
	t.Setenv("SESSION_TTL_HOURS", "24")
	results := validateProductionEnv()
	for _, r := range results {
		if r.name == "COOKIE_SECURE" && !r.ok {
			t.Fatalf("expected COOKIE_SECURE ok with COOKIE_SECURE=true: %+v", r)
		}
	}
}

func TestValidateProductionEnvWeakBootstrapPasswordFails(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://u:p@h/d")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "short")
	t.Setenv("SESSION_TTL_HOURS", "24")
	results := validateProductionEnv()
	for _, r := range results {
		if r.name == "BOOTSTRAP_ADMIN_PASSWORD" {
			if r.ok || r.level != "error" {
				t.Fatalf("expected weak password error, got %+v", r)
			}
		}
	}
}

func TestValidateProductionEnvEmptyBootstrapOK(t *testing.T) {
	// An empty BOOTSTRAP_ADMIN_PASSWORD is allowed because it means
	// "use an existing admin".
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://u:p@h/d")
	t.Setenv("COOKIE_SECURE", "true")
	os.Unsetenv("BOOTSTRAP_ADMIN_PASSWORD")
	t.Setenv("SESSION_TTL_HOURS", "24")
	results := validateProductionEnv()
	for _, r := range results {
		if r.name == "BOOTSTRAP_ADMIN_PASSWORD" && !r.ok {
			t.Fatalf("empty BOOTSTRAP_ADMIN_PASSWORD should be ok: %+v", r)
		}
	}
}

func TestValidateProductionEnvLongSessionTTLWarns(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://u:p@h/d")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("SESSION_TTL_HOURS", "2161")
	results := validateProductionEnv()
	for _, r := range results {
		if r.name == "SESSION_TTL_HOURS" && r.ok {
			t.Fatalf("expected SESSION_TTL_HOURS warning, got ok: %+v", r)
		}
	}
}

func TestValidateProductionEnvThreeMonthSessionTTLIsOK(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://u:p@h/d")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("SESSION_TTL_HOURS", "2160")
	results := validateProductionEnv()
	for _, r := range results {
		if r.name == "SESSION_TTL_HOURS" && !r.ok {
			t.Fatalf("SESSION_TTL_HOURS=2160 should be ok: %+v", r)
		}
	}
}

func TestValidateProductionEnvDevSkipsProductionChecks(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	os.Unsetenv("DATABASE_URL")
	results := validateProductionEnv()
	// In dev, no production-only rules should fail at "error" level.
	for _, r := range results {
		if !r.ok && r.level == "error" && r.name != "DATABASE_URL" {
			t.Fatalf("dev mode should not error on %s: %+v", r.name, r)
		}
	}
	// DATABASE_URL error is still flagged in dev so users notice the
	// misconfiguration.
	dbURLFound := false
	for _, r := range results {
		if r.name == "DATABASE_URL" && !r.ok && r.level == "error" {
			dbURLFound = true
		}
	}
	if !dbURLFound {
		t.Fatalf("expected DATABASE_URL error in dev too: %+v", results)
	}
}

func TestValidateProductionEnvLocalhostFrontendOriginWarns(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://u:p@h/d")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("FRONTEND_ORIGIN", "http://localhost:5173")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "StrongP@ssword-1234")
	t.Setenv("SESSION_TTL_HOURS", "24")
	results := validateProductionEnv()
	for _, r := range results {
		if r.name == "FRONTEND_ORIGIN" && r.ok {
			t.Fatalf("expected FRONTEND_ORIGIN warning for localhost, got ok: %+v", r)
		}
	}
}

func TestValidateProductionEnvPostgresPasswordStrength(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://u:weak@h/d")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "StrongP@ssword-1234")
	t.Setenv("SESSION_TTL_HOURS", "24")
	results := validateProductionEnv()
	for _, r := range results {
		if r.name == "POSTGRES_PASSWORD" && (r.ok || r.level != "error") {
			t.Fatalf("expected POSTGRES_PASSWORD error for weak db password: %+v", r)
		}
	}
}

func TestParseBoolish(t *testing.T) {
	cases := map[string]bool{
		"1": true, "true": true, "TRUE": true, "yes": true, "on": true, "Y": true,
		"0": false, "false": false, "FALSE": false, "no": false, "off": false, "": false,
		"maybe": false, // unknown -> error path, returns false
	}
	for in, want := range cases {
		got, err := parseBoolish(in)
		if err != nil && want {
			t.Errorf("parseBoolish(%q) unexpected err: %v", in, err)
		}
		if got != want {
			t.Errorf("parseBoolish(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestHasClassHelpers(t *testing.T) {
	if !hasLower("abc") {
		t.Error("hasLower(abc) = false")
	}
	if hasLower("ABC123!@#") {
		t.Error("hasLower(ABC123) = true")
	}
	if !hasUpper("Abc") {
		t.Error("hasUpper(Abc) = false")
	}
	if !hasDigit("a1b") {
		t.Error("hasDigit(a1b) = false")
	}
	if !hasSymbol("a!b") {
		t.Error("hasSymbol(a!b) = false")
	}
	if hasSymbol("abc123") {
		t.Error("hasSymbol(abc123) = true")
	}
}

func TestCheckRecordingsPathTildeWarns(t *testing.T) {
	os.Unsetenv("RECORDINGS_PATH")
	t.Setenv("RECORDINGS_PATH", "~/recordings")
	r := checkRecordingsPath()
	if r.ok || r.level != "warning" || !strings.Contains(r.message, "~") {
		t.Fatalf("expected warning about ~, got %+v", r)
	}
}
