# ADR-010: Multi-Provider AI Gateway With Circuit-Breaker Fallback

**Status:** Accepted

**Context:** Relying on a single AI provider creates a single point of failure for every AI-powered feature, and provider pricing/capability tradeoffs vary by task type.

**Decision:** AI Gateway routes by task type across Anthropic, OpenRouter, OpenAI, Google Gemini, and self-hosted Ollama, with circuit-breaker fallback if a provider fails.

**Rationale:** Task-based routing lets cheaper/local models (Ollama) handle low-stakes tasks while reserving higher-capability providers for tasks that need them. Circuit-breaker fallback means a single provider outage doesn't take down every AI feature simultaneously.

**Consequences:** This exact shape — multi-provider, circuit-breaker fallback — was reused later for the email provider abstraction (ADR-015) rather than inventing a new pattern.
