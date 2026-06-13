package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/moluuser/aic/internal/config"
)

// Ollama talks to a local (or remote) Ollama server via its /api/chat
// endpoint. See https://github.com/ollama/ollama/blob/main/docs/api.md
type Ollama struct {
	endpoint string
	model    string
	client   *http.Client
}

// NewOllama builds an Ollama provider from cfg.
func NewOllama(cfg config.Config) *Ollama {
	endpoint := cfg.Ollama.Endpoint
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	return &Ollama{
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    cfg.Model,
		client:   &http.Client{Timeout: 5 * time.Minute},
	}
}

// Name implements Provider.
func (o *Ollama) Name() string { return "ollama" }

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
	Error   string        `json:"error,omitempty"`
}

// Generate implements Provider.
func (o *Ollama) Generate(ctx context.Context, req Request) (string, error) {
	model := req.Model
	if model == "" {
		model = o.model
	}
	if model == "" {
		return "", fmt.Errorf("no model configured for ollama")
	}

	payload := ollamaChatRequest{
		Model:  model,
		Stream: false,
		Messages: []ollamaMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.User},
		},
		Options: map[string]any{
			// Low temperature keeps commit messages focused and deterministic.
			"temperature": 0.2,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := o.endpoint + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("calling ollama at %s: %w\n(is `ollama serve` running?)", o.endpoint, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		// Ollama returns a JSON {"error": "..."} on most failures.
		var errResp ollamaChatResponse
		if json.Unmarshal(data, &errResp) == nil && errResp.Error != "" {
			return "", fmt.Errorf("ollama: %s", errResp.Error)
		}
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var chatResp ollamaChatResponse
	if err := json.Unmarshal(data, &chatResp); err != nil {
		return "", fmt.Errorf("decoding ollama response: %w", err)
	}
	if chatResp.Error != "" {
		return "", fmt.Errorf("ollama: %s", chatResp.Error)
	}

	return chatResp.Message.Content, nil
}
