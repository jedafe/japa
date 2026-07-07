package trending

import "context"

// WorkerAdapter adapts the trending Processor to the worker.TrendingProcessor
// interface, allowing the Asynq worker to call trending computation methods.
type WorkerAdapter struct {
	Processor *Processor
}

// NewWorkerAdapter creates a WorkerAdapter wrapping the given Processor.
func NewWorkerAdapter(processor *Processor) *WorkerAdapter {
	return &WorkerAdapter{Processor: processor}
}

// ComputeTrending delegates to the Processor.
func (a *WorkerAdapter) ComputeTrending(ctx context.Context) error {
	return a.Processor.ComputeTrending(ctx)
}

// ComputeSuggestions delegates to the Processor.
func (a *WorkerAdapter) ComputeSuggestions(ctx context.Context) error {
	return a.Processor.ComputeSuggestions(ctx)
}
