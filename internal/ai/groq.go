package ai

// GroqProvider wraps the OpenAI-compatible client for Groq Cloud.
//
// Groq exposes an OpenAI-compatible Chat Completions API at
// https://api.groq.com/openai/v1. This provider uses the same wire format as
// OpenAIProvider but defaults to the Groq endpoint when no base URL is
// supplied. It is registered as a dedicated provider type so the Control Panel
// can show Groq-specific hints and admins can mix Groq models with other
// OpenAI-compatible backends on equal footing.
//
// Ref: japaconnect-engineering/sections/20-ai-core.md.
func NewGroqProvider(name, apiKey, baseURL, model string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.groq.com/openai/v1"
	}

	p := NewOpenAIProvider(name, apiKey, baseURL, model)
	p.UseMaxCompletionTokens()

	return p
}
