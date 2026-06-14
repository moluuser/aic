package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/moluuser/aic/internal/config"
)

// OpenRouter talks to OpenRouter's OpenAI-compatible chat completions API.
// See https://openrouter.ai/docs/api-reference/chat-completion
type OpenRouter struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

// NewOpenRouter builds an OpenRouter provider from cfg. The API key is taken
// from cfg, falling back to the OPENROUTER_API_KEY environment variable.
func NewOpenRouter(cfg config.Config) *OpenRouter {
	endpoint := cfg.OpenRouter.Endpoint
	if endpoint == "" {
		endpoint = "https://openrouter.ai/api/v1"
	}
	apiKey := cfg.OpenRouter.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENROUTER_API_KEY")
	}
	return &OpenRouter{
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    cfg.Model,
		apiKey:   apiKey,
		client:   &http.Client{Timeout: 5 * time.Minute},
	}
}

// Name implements Provider.
func (o *OpenRouter) Name() string { return "openrouter" }

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openRouterMessage `json:"messages"`
	Temperature float64             `json:"temperature"`
}

type openRouterChatResponse struct {
	Choices []struct {
		Message openRouterMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Generate implements Provider.
func (o *OpenRouter) Generate(ctx context.Context, req Request) (string, error) {
	if o.apiKey == "" {
		return "", fmt.Errorf("no OpenRouter API key; set openrouter.api_key in config or the OPENROUTER_API_KEY environment variable")
	}

	model := req.Model
	if model == "" {
		model = o.model
	}
	if model == "" {
		return "", fmt.Errorf("no model configured for openrouter")
	}

	payload := openRouterChatRequest{
		Model: model,
		// Low temperature keeps commit messages focused and deterministic.
		Temperature: 0.2,
		Messages: []openRouterMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.User},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := o.endpoint + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	// Optional attribution headers recognised by OpenRouter.
	httpReq.Header.Set("HTTP-Referer", "https://github.com/moluuser/aic")
	httpReq.Header.Set("X-Title", "aic")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("calling openrouter at %s: %w", o.endpoint, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var chatResp openRouterChatResponse
	if err := json.Unmarshal(data, &chatResp); err != nil {
		return "", fmt.Errorf("decoding openrouter response: %w (status %d)", err, resp.StatusCode)
	}
	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return "", fmt.Errorf("openrouter: %s", chatResp.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrouter returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("openrouter returned no choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}
