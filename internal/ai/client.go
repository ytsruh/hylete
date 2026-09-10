package ai_coach

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the narrow LLM interface the Coach service accepts.
// The Cloudflare implementation is the only production backend;
// tests substitute a fake. A new provider means a new file
// implementing this one method — the orchestrator never changes.
type Client interface {
	// Chat sends a fully-rendered prompt and returns the raw
	// response body plus best-effort token usage (0s when the
	// backend omits usage).
	Chat(ctx context.Context, prompt string) (body string, tokensIn, tokensOut int, err error)
}

// CFClient is the Cloudflare Workers AI transport over its
// OpenAI-compatible chat-completions endpoint. Stdlib HTTP only —
// no vendor SDK — so the dependency stays swappable.
type CFClient struct {
	cfg        Config
	httpClient *http.Client
}

// NewCFClient builds a transport from cfg. The http client timeout
// bounds a hung inference call; callers pass ctx for cancellation.
func NewCFClient(cfg Config) *CFClient {
	return &CFClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// chatRequest mirrors the OpenAI chat-completions shape the
// Cloudflare endpoint accepts.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse mirrors the subset of the OpenAI response we read:
// first choice content + usage.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Chat POSTs the prompt as a single user message and returns the
// first choice's content. 429/5xx surface as errors for the caller
// (the cron logs them); no retries here so a single slow week
// cannot cascade into overlapping ticks.
func (c *CFClient) Chat(ctx context.Context, prompt string) (string, int, int, error) {
	payload, err := json.Marshal(chatRequest{
		Model:    c.cfg.Model,
		Messages: []chatMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", 0, 0, fmt.Errorf("ai: failed to encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint(), bytes.NewReader(payload))
	if err != nil {
		return "", 0, 0, fmt.Errorf("ai: failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, 0, fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", 0, 0, fmt.Errorf("ai: failed to read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", 0, 0, fmt.Errorf("ai: endpoint returned %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", 0, 0, fmt.Errorf("ai: failed to decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", 0, 0, fmt.Errorf("ai: response contained no choices")
	}
	body := parsed.Choices[0].Message.Content
	if body == "" {
		return "", 0, 0, fmt.Errorf("ai: response choice was empty")
	}
	return body, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
