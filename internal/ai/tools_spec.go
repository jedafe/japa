// Package ai — tools_spec.go implements the spec.search tool handler
// (§121, ADR-131) — privileged vector retrieval over the engineering spec
// corpus (indexed .jeos files as private spec_doc chunks).
//
// The tool is staff-only: the ai_tools row carries
// required_permission='agent.spec_search' so the executor's RBAC gate hides
// it from users without the node. Defense-in-depth lives below the tool —
// the service path (RetrievePrivileged → SearchSpecEmbeddings) only ever
// touches entity_type='spec_doc' AND visibility='private' rows, so a tool
// or catalog bug can never widen it into user content.
//
// Privacy: spec chunks are platform spec material, not user data — no
// PrivacyTargetFunc is registered.
//
// Ref: japaconnect-engineering/sections/121-engineering-spec-corpus.md
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Knowledge tools)
// Ref: japaconnect-engineering/adr/ADR-131-engineering-spec-corpus.md
package ai

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

// ErrSpecCorpusDisabled is the ai-package mirror of
// rag.ErrSpecCorpusDisabled — the wiring adapter translates the service
// sentinel so handlers distinguish flag-off from real failures without
// importing internal/rag (import cycle).
var ErrSpecCorpusDisabled = errors.New("spec corpus disabled (feature flag rag.spec_corpus)")

// specSearchArgs is the LLM-produced argument shape for spec.search.
type specSearchArgs struct {
	Query      string `json:"query"`
	PathPrefix string `json:"path_prefix,omitempty"`
	Limit      int32  `json:"limit,omitempty"`
}

// SpecSearchHandler returns a ToolHandler that searches the engineering
// spec corpus via privileged retrieval.
func SpecSearchHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.RAG == nil {
			return errorToolResult("spec_search_unavailable", "spec search is not configured"), nil
		}

		var a specSearchArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", "invalid arguments: "+err.Error()), nil
		}

		if a.Query == "" {
			return errorToolResult("missing_query", "query is required"), nil
		}

		limit := a.Limit
		if limit <= 0 {
			limit = 8
		} else if limit > 50 {
			limit = 50
		}

		chunks, err := deps.RAG.RetrievePrivileged(ctx, a.Query, "en", a.PathPrefix, limit)
		if errors.Is(err, ErrSpecCorpusDisabled) {
			return errorToolResult("spec_corpus_disabled", ErrSpecCorpusDisabled.Error()), nil
		}

		if err != nil {
			return errorToolResult("spec_search_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"query":       a.Query,
			"path_prefix": a.PathPrefix,
			"results":     chunks,
			"total":       len(chunks),
		}), nil
	}
}
