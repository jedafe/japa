# Convention: Error Handling (Go)

- Wrap errors with context using `fmt.Errorf("doing X: %w", err)` — never return a bare upstream error without saying what operation failed.
- Domain errors (not-found, forbidden, validation) are typed/sentinel errors checked with `errors.Is`/`errors.As`, not string-matched.
- HTTP handlers translate domain errors to status codes in one central place (a middleware or helper), not ad hoc per handler.
- Background task handlers (Asynq) return errors normally to trigger the queue's retry/backoff logic — don't swallow errors inside a task handler "to be safe."
- Never log and return the same error at multiple layers — log once, at the layer that actually handles or surfaces it.
