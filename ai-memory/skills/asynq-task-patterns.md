# Skill: Asynq Task Patterns

## Tier assignment
Every new task type needs an explicit tier (critical/default/low) decided at design time — see `ai-memory/sections/11-job-queue-asynq.md` for the full catalog and current assignments. Don't default to `default` tier without thinking about it.

## Webhook handler pattern — validate fast, process async
Never do business logic inside the HTTP handler for an inbound webhook:
```go
func HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
    event, err := verifySignature(r) // fast
    if err != nil { w.WriteHeader(400); return }
    if alreadyProcessed(event.ID) { w.WriteHeader(200); return } // idempotency
    recordRawEvent(event)
    enqueueTask("payment_webhook_process", event.ID) // critical tier
    w.WriteHeader(200) // return fast — providers retry aggressively on timeout
}
```

## Scheduled (ETA) tasks vs. periodic (cron) tasks
Use ETA-scheduled tasks for one-off future execution (`scheduled_publish`, a specific blog post going live at a specific time). Use Asynq's periodic scheduler for recurring jobs (`digest_email`, `sitemap_generate`) — don't simulate periodic behavior by re-enqueueing a task at the end of its own handler.

## Batching high-frequency low-value events
Don't enqueue a task per event for things like ad impressions or engagement counts. Batch client-side or at ingestion, flush periodically, and let one task process the batch.
