package ai_coach

import (
	"strings"
)

// Config authenticates the Cloudflare Workers AI OpenAI-compatible
// endpoint. Values come from utils.EnvVar (all four required at
// startup — a missing key hard-fails boot, same policy as the
// email token). There is deliberately no os.Getenv fallback here:
// EnvVar is the single source so "which env does AI need" has one
// answer (see .env.example).
type Config struct {
	// AccountID is the Cloudflare account ID in the endpoint path.
	AccountID string
	// Token is the API token used as the Bearer credential.
	Token string
	// BaseURL is the Cloudflare API root, normally
	// https://api.cloudflare.com/client/v4.
	BaseURL string
	// Model is the pinned Workers AI model.
	Model string
}

// DefaultBaseURL and DefaultModel document the deployed values
// (kept in .env.example). They are references for tests and docs,
// not fallbacks — production always sets all four explicitly.
const (
	DefaultBaseURL = "https://api.cloudflare.com/client/v4"
	DefaultModel   = "@cf/meta/llama-3.1-8b-instruct"
)

// ConfigFromEnv builds a Config from already-validated EnvVar
// values. Callers pass cfg.CLOUDFLARE_AI_* / cfg.AI_* straight
// through; no defaults are applied because startup validation
// guarantees every value is present.
func ConfigFromEnv(accountID, token, baseURL, model string) Config {
	return Config{
		AccountID: accountID,
		Token:     token,
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Model:     model,
	}
}

// Enabled reports whether credentials exist to call the endpoint.
// Always true in production (startup validation); false only when
// a test constructs an empty Config.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.AccountID) != "" && strings.TrimSpace(c.Token) != ""
}

// Endpoint returns the OpenAI-compatible chat-completions URL.
func (c Config) Endpoint() string {
	return c.BaseURL + "/accounts/" + c.AccountID + "/ai/v1/chat/completions"
}
