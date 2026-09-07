// Package utils provides shared utility functions used across the application.
// The env sub-package handles loading and validating environment variables on startup.
package utils

import (
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/joho/godotenv"
)

// JWTTTL is the lifetime of an issued session JWT. It is the
// single source of truth for both the JWT's "exp" claim (set
// in jwt.go) and the browser auth cookie's MaxAge (set in
// routes.go), so the two cannot drift apart. Changing this
// value extends or shortens how long a login session lasts.
const JWTTTL = 14 * 24 * time.Hour

// EnvVar holds all environment variables used by the application.
// Each field must have an `env` tag matching the environment variable name.
// These values are loaded from the environment or a local .env file on startup.
type EnvVar struct {
	PORT               string `env:"PORT"`
	DB_PATH            string `env:"DB_PATH"`
	TURSO_DATABASE_URL string `env:"TURSO_DATABASE_URL"`
	TURSO_AUTH_TOKEN   string `env:"TURSO_AUTH_TOKEN"`
	JWT_SECRET         string `env:"JWT_SECRET"`
	STORAGE_ENDPOINT   string `env:"STORAGE_ENDPOINT"`
	STORAGE_ACCESS_KEY string `env:"STORAGE_ACCESS_KEY"`
	STORAGE_SECRET_KEY string `env:"STORAGE_SECRET_KEY"`
	STORAGE_BUCKET     string `env:"STORAGE_BUCKET"`
	STORAGE_PUBLIC_URL string `env:"STORAGE_PUBLIC_URL"`
	// CLOUDFLARE_EMAIL_TOKEN authenticates outbound SMTP to
	// Cloudflare Email Sending (smtp.mx.cloudflare.net:465).
	// Required: startup hard-fails if empty. The token must
	// have the "Email Sending: Edit" permission.
	CLOUDFLARE_EMAIL_TOKEN string `env:"CLOUDFLARE_EMAIL_TOKEN"`
	// PUBLIC_URL is the absolute origin the app is served
	// from, e.g. "https://www.hyleteapp.com". Used by the
	// email subsystem to build links in transactional
	// emails (password-reset URL, welcome dashboard link).
	// Required: startup hard-fails if empty. Must be a
	// valid http or https URL with no trailing slash.
	PUBLIC_URL string `env:"PUBLIC_URL"`
}

// LoadAndValidateEnv loads environment variables from a .env file (if present)
// and the system environment, then validates that all required variables are set.
// It returns the loaded configuration and an error if any required variable is missing.
func LoadAndValidateEnv() (*EnvVar, error) {
	// Load from .env file if it exists (typically in development).
	// This is silently ignored in production where env vars are set in the environment.
	_ = godotenv.Load()

	env := EnvVar{
		PORT:                   os.Getenv("PORT"),
		DB_PATH:                os.Getenv("DB_PATH"),
		TURSO_DATABASE_URL:     os.Getenv("TURSO_DATABASE_URL"),
		TURSO_AUTH_TOKEN:       os.Getenv("TURSO_AUTH_TOKEN"),
		JWT_SECRET:             os.Getenv("JWT_SECRET"),
		STORAGE_ENDPOINT:       os.Getenv("STORAGE_ENDPOINT"),
		STORAGE_ACCESS_KEY:     os.Getenv("STORAGE_ACCESS_KEY"),
		STORAGE_SECRET_KEY:     os.Getenv("STORAGE_SECRET_KEY"),
		STORAGE_BUCKET:         os.Getenv("STORAGE_BUCKET"),
		STORAGE_PUBLIC_URL:     os.Getenv("STORAGE_PUBLIC_URL"),
		CLOUDFLARE_EMAIL_TOKEN: os.Getenv("CLOUDFLARE_EMAIL_TOKEN"),
		PUBLIC_URL:             os.Getenv("PUBLIC_URL"),
	}

	// Validate that all required environment variables are set
	missingVars := ValidateEnvVars(env)
	if len(missingVars) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missingVars)
	}

	return &env, nil
}

// ValidateEnvVars checks if all fields in the EnvVar struct are non-empty.
// It uses reflection to inspect struct fields and their `env` tags.
// Returns a slice of environment variable names (from the `env` tag) that are missing.
func ValidateEnvVars(env EnvVar) []string {
	v := reflect.ValueOf(env)
	t := reflect.TypeOf(env)

	var missingVars []string
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Only check string fields that have an env tag
		if field.Kind() == reflect.String {
			tag := fieldType.Tag.Get("env")
			if tag != "" && field.String() == "" {
				missingVars = append(missingVars, tag)
			}
		}
	}

	return missingVars
}