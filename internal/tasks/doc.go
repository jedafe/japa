// Package tasks provides background task logic for JapaConnect.
//
// Tasks in this package contain the business logic for periodic background
// jobs. They are consumed by the Asynq worker handlers in internal/worker.
// Separating task logic from the worker allows it to be tested independently
// and reused across different execution contexts.
package tasks
