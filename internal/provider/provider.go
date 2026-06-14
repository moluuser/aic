// Package provider defines the LLM backend abstraction used to generate
// commit messages. Ollama and OpenRouter are the bundled implementations;
// additional providers (OpenAI, Anthropic, LM Studio, vLLM, ...) implement the
// same interface and register themselves through the factory below.
package provider

import (
	"context"
	"fmt"

	"github.com/moluuser/aic/internal/config"
)

// Request carries everything a provider needs to produce a completion.
type Request struct {
	// System is the system / instruction prompt.
	System string
	// User is the user prompt (diff, history, branch, etc.).
	User string
	// Model overrides the provider's configured model when non-empty.
	Model string
}

// Provider generates text from a prompt. Implementations must be safe to
// construct cheaply; network errors should be returned, not panicked.
type Provider interface {
	// Name returns the provider identifier, e.g. "ollama".
	Name() string
	// Generate returns the model's completion for req.
	Generate(ctx context.Context, req Request) (string, error)
}

// New constructs the provider named by cfg.Provider.
//
// To add a provider: implement Provider and add a case here.
func New(cfg config.Config) (Provider, error) {
	switch cfg.Provider {
	case "", "ollama":
		return NewOllama(cfg), nil
	case "openrouter":
		return NewOpenRouter(cfg), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: ollama, openrouter)", cfg.Provider)
	}
}
