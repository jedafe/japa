// Package mcp — visibility_filter.go narrows content.* read-tool results to
// the public contract for read-only key/OAuth callers.
//
// The §103 agent tools run with internal visibility (InternalVisibilityFilter
// — the agent is trusted like a staff service and may surface drafts and
// non-public rows to its principal). The §119 read gate, however, is the same
// contract as /api/v1/public: published + public only. JWT callers keep the
// full internal catalog behaviour; elevated agent.write keys keep it too —
// only the read-only gate population is narrowed here.
package mcp

import (
	"context"
	"encoding/json"
)

// gatedContentTools are the read-gate tools whose results carry row-level
// status/visibility and must be post-filtered to the public contract.
var gatedContentTools = map[string]bool{
	"content.read":   true,
	"content.search": true,
}

// applyPublicReadFilter rewrites a content.read / content.search tool result
// for a read-only key/OAuth caller. Returns (newText, denied) — denied is
// true only for content.read on a non-public row, where the correct answer
// is the tool's own not-found shape.
//
// Both filters are idempotent and shape-safe: an error-shaped result
// ({"error":...}) or an unexpected payload is returned unchanged.
func applyPublicReadFilter(toolName, text string) (string, bool) {
	if !gatedContentTools[toolName] {
		return text, false
	}

	switch toolName {
	case "content.read":
		var payload struct {
			Content *struct {
				Status     string `json:"status"`
				Visibility string `json:"visibility"`
				AutoHidden bool   `json:"auto_hidden"`
			} `json:"content"`
		}

		if err := json.Unmarshal([]byte(text), &payload); err != nil || payload.Content == nil {
			return text, false
		}

		if payload.Content.Status == "published" && payload.Content.Visibility == "public" && !payload.Content.AutoHidden {
			return text, false
		}

		// Match the handler's own not-found envelope so the caller cannot
		// distinguish a hidden row from a missing one.
		return `{"error":"content not found","code":"content_not_found"}`, true

	case "content.search":
		var payload struct {
			Results []map[string]any `json:"results"`
			Error   string           `json:"error"`
		}

		if err := json.Unmarshal([]byte(text), &payload); err != nil || payload.Error != "" || payload.Results == nil {
			return text, false
		}

		kept := payload.Results[:0]

		for _, r := range payload.Results {
			if r["status"] == "published" && r["visibility"] == "public" {
				kept = append(kept, r)
			}
		}

		if len(kept) == len(payload.Results) {
			return text, false
		}

		var rewritten map[string]any
		if err := json.Unmarshal([]byte(text), &rewritten); err != nil {
			return text, false
		}

		rewritten["results"] = kept
		rewritten["total"] = len(kept)

		b, err := json.Marshal(rewritten)
		if err != nil {
			return text, false
		}

		return string(b), false
	}

	return text, false
}

// filterReadOnlyResult applies the public-read narrowing for read-only
// key/OAuth callers on the gated content tools. JWT callers and elevated
// agent.write keys are unaffected.
func (s *Server) filterReadOnlyResult(ctx context.Context, toolName, text string) (string, bool) {
	if !isAPIKeyCaller(ctx) || keyHasAgentWrite(ctx) {
		return text, false
	}

	return applyPublicReadFilter(toolName, text)
}
