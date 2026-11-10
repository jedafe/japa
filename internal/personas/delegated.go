// Delegated-actor resolution for author_id/actor_id overrides (design:
// docs/superpowers/specs/2026-11-10-persona-attributed-interactions-design.md
// §1.1). Callers keep their own identity; the delegate field picks the bot
// persona backing user that records the action.
// // Ref: japaconnect-engineering/sections/68-ai-personas.md
package personas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

var (
	ErrDelegatedInvalid   = errors.New("delegate id is not a valid user UUID")
	ErrDelegatedForbidden = errors.New("delegate requires content.assign_author")
	ErrDelegatedNotBot    = errors.New("delegate must identify a bot persona account")
)

// BotChecker reports whether a user id is a bot backing account
// (users.is_bot). content.Service already satisfies this; other services
// gain a thin IsBotUser wrapper over gen.GetUserIsBot.
type BotChecker interface {
	IsBotUser(ctx context.Context, userID pgtype.UUID) (bool, error)
}

// PermResolver is the slice of rbac.RBAC the delegate check needs.
// *rbac.RBAC satisfies it; tests substitute fakes.
type PermResolver interface {
	IsSuperAdmin(ctx context.Context, userID pgtype.UUID) bool
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// ResolveDelegatedActor validates a delegate override. nil resolver means
// "no RBAC in context" → forbidden (mirrors content.canAssignAuthor).
func ResolveDelegatedActor(ctx context.Context, bots BotChecker, resolver PermResolver, operatorID pgtype.UUID, raw string) (pgtype.UUID, error) {
	var actorID pgtype.UUID
	if err := actorID.Scan(raw); err != nil || !actorID.Valid {
		return pgtype.UUID{}, ErrDelegatedInvalid
	}
	if resolver == nil || !(resolver.IsSuperAdmin(ctx, operatorID) ||
		resolver.HasPermission(ctx, operatorID, "content.assign_author")) {
		return pgtype.UUID{}, ErrDelegatedForbidden
	}
	isBot, err := bots.IsBotUser(ctx, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, ErrDelegatedInvalid
		}
		return pgtype.UUID{}, fmt.Errorf("personas: delegate bot check: %w", err)
	}
	if !isBot {
		return pgtype.UUID{}, ErrDelegatedNotBot
	}
	return actorID, nil
}

// WriteDelegatedError maps a ResolveDelegatedActor error to the standard
// envelope. param selects the field name in codes ("author_id"/"actor_id").
// Non-sentinel errors are internal failures (e.g. the is_bot DB lookup),
// never a client error.
func WriteDelegatedError(w http.ResponseWriter, err error, param string) {
	switch {
	case errors.Is(err, ErrDelegatedInvalid):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_"+param,
			param+" does not identify a user")
	case errors.Is(err, ErrDelegatedForbidden):
		errpkg.WriteError(w, http.StatusForbidden, "author_assign_forbidden",
			"Setting "+param+" to another user requires the content.assign_author permission")
	case errors.Is(err, ErrDelegatedNotBot):
		errpkg.WriteError(w, http.StatusBadRequest, "author_not_bot",
			param+" must identify a bot persona account — real-user attribution requires impersonation")
	default:
		slog.Error("delegated actor resolution failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error",
			"Failed to validate "+param)
	}
}

// AuditAttribution writes a persona_attrib.* audit_logs row — fire-and-forget
// callers; a failure is logged by the caller, never fails the request
// (same posture as impersonation.auditLog, Rule 10). Callers reach the
// writer pool via their service's DB() accessor.
func AuditAttribution(ctx context.Context, pool *pgxpool.Pool, operatorID, actorID pgtype.UUID, actionType, entityType string, entityID pgtype.UUID) error {
	meta, _ := json.Marshal(map[string]any{"entity_type": entityType, "entity_id": entityID.String()})
	_, err := gen.New(pool).CreateAuditLog(ctx, gen.CreateAuditLogParams{
		ActorUserID:        operatorID,
		ImpersonatedUserID: actorID,
		ActionType:         "persona_attrib." + actionType,
		TargetType:         &entityType,
		TargetID:           entityID,
		Metadata:           meta,
	})
	return err
}
