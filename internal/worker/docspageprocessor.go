// Package worker — docspageprocessor.go defines the DocsPageGenerator
// interface that the docs:generate_page task delegates to. The ai package
// (or a noop implementation) implements this interface and registers itself
// at startup via RegisterDocsPageGenerator (TASK-004).
package worker

import "context"

// DocsPageGenerator defines the contract for generating a docs page via AI
// in the background. The implementing service calls the AI gateway, persists
// the generated page, and updates any task status.
type DocsPageGenerator interface {
	GenerateDocsPage(ctx context.Context, prompt, userID, tenantID string) error
}

var docsPageGenerator DocsPageGenerator

// RegisterDocsPageGenerator registers the docs page generator for the
// docs:generate_page background task. Call once at startup before the worker
// begins processing tasks.
func RegisterDocsPageGenerator(g DocsPageGenerator) {
	docsPageGenerator = g
}
