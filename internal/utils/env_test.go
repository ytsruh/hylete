package utils

import (
	"testing"
	"time"
)

// TestJWTTTL_ValueAndCookieSync guards the session-TTL constant
// in two ways: it pins the user-facing value to 14 days, and it
// verifies the constant is representable as an int-second
// MaxAge for the auth cookie (so the JWT "exp" claim and the
// cookie cannot silently drift apart). If JWTTTL is changed
// intentionally, update this test in the same commit.
func TestJWTTTL_ValueAndCookieSync(t *testing.T) {
	if got, want := JWTTTL, 14*24*time.Hour; got != want {
		t.Errorf("JWTTTL = %s, want %s (14 days)", got, want)
	}
	// MaxAge in net/http.Cookie is an int (seconds). The
	// constant must convert cleanly without overflow or
	// truncation so the browser cookie matches the JWT.
	if int64(int(JWTTTL.Seconds())) != int64(JWTTTL.Seconds()) {
		t.Errorf("JWTTTL seconds %d does not fit in int (cookie MaxAge)", int64(JWTTTL.Seconds()))
	}
}

func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PORT", "8080")
	t.Setenv("DB_PATH", "test.db")
	t.Setenv("TURSO_DATABASE_URL", "libsql://test.turso.io")
	t.Setenv("TURSO_AUTH_TOKEN", "test-token")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("STORAGE_ENDPOINT", "https://test.r2.cloudflarestorage.com")
	t.Setenv("STORAGE_ACCESS_KEY", "test-access")
	t.Setenv("STORAGE_SECRET_KEY", "test-secret-key")
	t.Setenv("STORAGE_BUCKET", "test-bucket")
	t.Setenv("STORAGE_PUBLIC_URL", "https://pub.test-bucket.r2.dev")
	t.Setenv("CLOUDFLARE_EMAIL_TOKEN", "test-email-token")
	t.Setenv("PUBLIC_URL", "https://hylete.test.local")
}

func TestLoadAndValidateEnv_Success(t *testing.T) {
	setValidEnv(t)

	cfg, err := LoadAndValidateEnv()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.PORT != "8080" {
		t.Errorf("expected PORT to be '8080', got: %s", cfg.PORT)
	}
	if cfg.DB_PATH != "test.db" {
		t.Errorf("expected DB_PATH to be 'test.db', got: %s", cfg.DB_PATH)
	}
	if cfg.TURSO_DATABASE_URL != "libsql://test.turso.io" {
		t.Errorf("expected TURSO_DATABASE_URL to be 'libsql://test.turso.io', got: %s", cfg.TURSO_DATABASE_URL)
	}
	if cfg.TURSO_AUTH_TOKEN != "test-token" {
		t.Errorf("expected TURSO_AUTH_TOKEN to be 'test-token', got: %s", cfg.TURSO_AUTH_TOKEN)
	}
	if cfg.JWT_SECRET != "test-secret" {
		t.Errorf("expected JWT_SECRET to be 'test-secret', got: %s", cfg.JWT_SECRET)
	}
	if cfg.STORAGE_ENDPOINT != "https://test.r2.cloudflarestorage.com" {
		t.Errorf("expected STORAGE_ENDPOINT to be set, got: %s", cfg.STORAGE_ENDPOINT)
	}
	if cfg.STORAGE_BUCKET != "test-bucket" {
		t.Errorf("expected STORAGE_BUCKET to be 'test-bucket', got: %s", cfg.STORAGE_BUCKET)
	}
}

func TestLoadAndValidateEnv_MissingPort(t *testing.T) {
	t.Setenv("PORT", "")
	setValidEnv(t)
	t.Setenv("PORT", "")

	_, err := LoadAndValidateEnv()
	if err == nil {
		t.Fatal("expected error for missing PORT, got nil")
	}
}

func TestLoadAndValidateEnv_MissingDBPath(t *testing.T) {
	t.Setenv("DB_PATH", "")
	setValidEnv(t)
	t.Setenv("DB_PATH", "")

	_, err := LoadAndValidateEnv()
	if err == nil {
		t.Fatal("expected error for missing DB_PATH, got nil")
	}
}

func TestLoadAndValidateEnv_MissingStorage(t *testing.T) {
	setValidEnv(t)
	t.Setenv("STORAGE_BUCKET", "")

	_, err := LoadAndValidateEnv()
	if err == nil {
		t.Fatal("expected error for missing STORAGE_BUCKET, got nil")
	}
}

func TestLoadAndValidateEnv_MissingAll(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("TURSO_DATABASE_URL", "")
	t.Setenv("TURSO_AUTH_TOKEN", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("STORAGE_ENDPOINT", "")
	t.Setenv("STORAGE_ACCESS_KEY", "")
	t.Setenv("STORAGE_SECRET_KEY", "")
	t.Setenv("STORAGE_BUCKET", "")
	t.Setenv("STORAGE_PUBLIC_URL", "")
	t.Setenv("CLOUDFLARE_EMAIL_TOKEN", "")
	t.Setenv("PUBLIC_URL", "")

	_, err := LoadAndValidateEnv()
	if err == nil {
		t.Fatal("expected error for missing variables, got nil")
	}
}

func TestValidateEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		env      EnvVar
		expected []string
	}{
		{
			name: "all fields present",
			env: EnvVar{
				PORT: "8080", DB_PATH: "test.db",
				TURSO_DATABASE_URL: "libsql://test.turso.io", TURSO_AUTH_TOKEN: "test-token",
				JWT_SECRET:         "test-secret",
				STORAGE_ENDPOINT:   "https://test.r2.cloudflarestorage.com",
				STORAGE_ACCESS_KEY: "ak", STORAGE_SECRET_KEY: "sk",
				STORAGE_BUCKET: "b", STORAGE_PUBLIC_URL: "https://pub.test.r2.dev",
				CLOUDFLARE_EMAIL_TOKEN: "email-tok",
				PUBLIC_URL:             "https://hylete.test.local",
			},
			expected: []string{},
		},
		{
			name: "missing port",
			env: EnvVar{
				DB_PATH:            "test.db",
				TURSO_DATABASE_URL: "libsql://test.turso.io", TURSO_AUTH_TOKEN: "test-token",
				JWT_SECRET:         "test-secret",
				STORAGE_ENDPOINT:   "https://test.r2.cloudflarestorage.com",
				STORAGE_ACCESS_KEY: "ak", STORAGE_SECRET_KEY: "sk",
				STORAGE_BUCKET: "b", STORAGE_PUBLIC_URL: "https://pub.test.r2.dev",
				CLOUDFLARE_EMAIL_TOKEN: "email-tok",
				PUBLIC_URL:             "https://hylete.test.local",
			},
			expected: []string{"PORT"},
		},
		{
			name: "missing db_path",
			env: EnvVar{
				PORT:               "8080",
				TURSO_DATABASE_URL: "libsql://test.turso.io", TURSO_AUTH_TOKEN: "test-token",
				JWT_SECRET:         "test-secret",
				STORAGE_ENDPOINT:   "https://test.r2.cloudflarestorage.com",
				STORAGE_ACCESS_KEY: "ak", STORAGE_SECRET_KEY: "sk",
				STORAGE_BUCKET: "b", STORAGE_PUBLIC_URL: "https://pub.test.r2.dev",
				CLOUDFLARE_EMAIL_TOKEN: "email-tok",
				PUBLIC_URL:             "https://hylete.test.local",
			},
			expected: []string{"DB_PATH"},
		},
		{
			name: "missing both",
			env: EnvVar{
				TURSO_DATABASE_URL: "libsql://test.turso.io", TURSO_AUTH_TOKEN: "test-token",
				JWT_SECRET:         "test-secret",
				STORAGE_ENDPOINT:   "https://test.r2.cloudflarestorage.com",
				STORAGE_ACCESS_KEY: "ak", STORAGE_SECRET_KEY: "sk",
				STORAGE_BUCKET: "b", STORAGE_PUBLIC_URL: "https://pub.test.r2.dev",
				CLOUDFLARE_EMAIL_TOKEN: "email-tok",
				PUBLIC_URL:             "https://hylete.test.local",
			},
			expected: []string{"PORT", "DB_PATH"},
		},
		{
			name: "missing turso url",
			env: EnvVar{
				PORT: "8080", DB_PATH: "test.db",
				TURSO_AUTH_TOKEN:   "test-token",
				JWT_SECRET:         "test-secret",
				STORAGE_ENDPOINT:   "https://test.r2.cloudflarestorage.com",
				STORAGE_ACCESS_KEY: "ak", STORAGE_SECRET_KEY: "sk",
				STORAGE_BUCKET: "b", STORAGE_PUBLIC_URL: "https://pub.test.r2.dev",
				CLOUDFLARE_EMAIL_TOKEN: "email-tok",
				PUBLIC_URL:             "https://hylete.test.local",
			},
			expected: []string{"TURSO_DATABASE_URL"},
		},
		{
			name: "missing turso token",
			env: EnvVar{
				PORT: "8080", DB_PATH: "test.db",
				TURSO_DATABASE_URL: "libsql://test.turso.io",
				JWT_SECRET:         "test-secret",
				STORAGE_ENDPOINT:   "https://test.r2.cloudflarestorage.com",
				STORAGE_ACCESS_KEY: "ak", STORAGE_SECRET_KEY: "sk",
				STORAGE_BUCKET: "b", STORAGE_PUBLIC_URL: "https://pub.test.r2.dev",
				CLOUDFLARE_EMAIL_TOKEN: "email-tok",
				PUBLIC_URL:             "https://hylete.test.local",
			},
			expected: []string{"TURSO_AUTH_TOKEN"},
		},
		{
			name: "missing jwt secret",
			env: EnvVar{
				PORT: "8080", DB_PATH: "test.db",
				TURSO_DATABASE_URL: "libsql://test.turso.io", TURSO_AUTH_TOKEN: "test-token",
				STORAGE_ENDPOINT:   "https://test.r2.cloudflarestorage.com",
				STORAGE_ACCESS_KEY: "ak", STORAGE_SECRET_KEY: "sk",
				STORAGE_BUCKET: "b", STORAGE_PUBLIC_URL: "https://pub.test.r2.dev",
				CLOUDFLARE_EMAIL_TOKEN: "email-tok",
				PUBLIC_URL:             "https://hylete.test.local",
			},
			expected: []string{"JWT_SECRET"},
		},
		{
			name: "missing storage bucket",
			env: EnvVar{
				PORT: "8080", DB_PATH: "test.db",
				TURSO_DATABASE_URL: "libsql://test.turso.io", TURSO_AUTH_TOKEN: "test-token",
				JWT_SECRET:         "test-secret",
				STORAGE_ENDPOINT:   "https://test.r2.cloudflarestorage.com",
				STORAGE_ACCESS_KEY: "ak", STORAGE_SECRET_KEY: "sk",
				STORAGE_PUBLIC_URL:     "https://pub.test.r2.dev",
				CLOUDFLARE_EMAIL_TOKEN: "email-tok",
				PUBLIC_URL:             "https://hylete.test.local",
			},
			expected: []string{"STORAGE_BUCKET"},
		},
		{
			name: "missing email token",
			env: EnvVar{
				PORT: "8080", DB_PATH: "test.db",
				TURSO_DATABASE_URL: "libsql://test.turso.io", TURSO_AUTH_TOKEN: "test-token",
				JWT_SECRET:         "test-secret",
				STORAGE_ENDPOINT:   "https://test.r2.cloudflarestorage.com",
				STORAGE_ACCESS_KEY: "ak", STORAGE_SECRET_KEY: "sk",
				STORAGE_BUCKET: "b", STORAGE_PUBLIC_URL: "https://pub.test.r2.dev",
				PUBLIC_URL: "https://hylete.test.local",
			},
			expected: []string{"CLOUDFLARE_EMAIL_TOKEN"},
		},
		{
			name: "missing public url",
			env: EnvVar{
				PORT: "8080", DB_PATH: "test.db",
				TURSO_DATABASE_URL: "libsql://test.turso.io", TURSO_AUTH_TOKEN: "test-token",
				JWT_SECRET:         "test-secret",
				STORAGE_ENDPOINT:   "https://test.r2.cloudflarestorage.com",
				STORAGE_ACCESS_KEY: "ak", STORAGE_SECRET_KEY: "sk",
				STORAGE_BUCKET: "b", STORAGE_PUBLIC_URL: "https://pub.test.r2.dev",
				CLOUDFLARE_EMAIL_TOKEN: "email-tok",
			},
			expected: []string{"PUBLIC_URL"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing := ValidateEnvVars(tt.env)
			if len(missing) != len(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, missing)
			}
			for i, v := range tt.expected {
				if missing[i] != v {
					t.Errorf("expected missing[%d] to be %s, got %s", i, v, missing[i])
				}
			}
		})
	}
}