package worker

// Asynq handlers for the native AT Protocol mini-PDS (§127, ADR-139).
// The service is registered by server setup — the handler is a thin
// decode+dispatch shell; all repo semantics live in internal/atproto.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// atprotoRepoWriter is the seam the publish handler writes through —
// *atproto.Service satisfies it in production. A local interface (not the
// concrete type) keeps worker out of atproto's auth-heavy import graph —
// internal/auth itself imports this package.
type atprotoRepoWriter interface {
	PublishRecordJSON(ctx context.Context, accountID pgtype.UUID, collection, rkey string, raw json.RawMessage, contentID pgtype.UUID) (string, error)
}

var atprotoSvc atprotoRepoWriter

// RegisterAtprotoService wires the mini-PDS service for atproto:* task
// handlers. Called once during server setup.
func RegisterAtprotoService(s atprotoRepoWriter) { atprotoSvc = s }

// HandleAtprotoPublishRecord processes atproto:publish_record (§127 A2):
// one record write into the account's repo — MST update + signed commit +
// records index, all inside the service's repo transaction.
func HandleAtprotoPublishRecord(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:atproto:publish_record")

	if atprotoSvc == nil {
		slog.Warn("task:atproto:publish_record: service not registered, dropping task")
		return nil
	}

	var payload struct {
		AccountID  string          `json:"account_id"`
		Collection string          `json:"collection"`
		Rkey       string          `json:"rkey,omitempty"`
		ContentID  string          `json:"content_id,omitempty"`
		Record     json.RawMessage `json:"record"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:atproto:publish_record: invalid payload", "error", err)
		return err
	}
	if payload.AccountID == "" || payload.Collection == "" || len(payload.Record) == 0 {
		slog.Error("task:atproto:publish_record: payload missing account_id, collection or record")
		return fmt.Errorf("task:atproto:publish_record: incomplete payload: %w", asynq.SkipRetry)
	}

	accountID, err := scanUUID(payload.AccountID)
	if err != nil {
		return fmt.Errorf("task:atproto:publish_record: %w: %w", err, asynq.SkipRetry)
	}
	contentID, err := scanOptionalUUID(payload.ContentID)
	if err != nil {
		return fmt.Errorf("task:atproto:publish_record: %w: %w", err, asynq.SkipRetry)
	}

	uri, err := atprotoSvc.PublishRecordJSON(ctx, accountID, payload.Collection, payload.Rkey, payload.Record, contentID)
	if err != nil {
		slog.Error("task:atproto:publish_record: publish failed",
			"account_id", payload.AccountID, "collection", payload.Collection, "error", err)
		return err
	}
	slog.Info("task:atproto:publish_record: committed", "uri", uri)
	return nil
}
