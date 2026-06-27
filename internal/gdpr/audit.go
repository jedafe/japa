package gdpr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// LogAuditEvent is a standalone helper for creating audit log entries.
// It can be called from any package that has access to the Store.
func LogAuditEvent(ctx context.Context, store *Store, tenantID pgtype.UUID, params LogAuditParams) error {
	return logAuditEvent(ctx, store, tenantID, params)
}

func logAuditEvent(ctx context.Context, store *Store, tenantID pgtype.UUID, params LogAuditParams) error {
	metadataJSON := []byte("{}")
	if len(params.Metadata) > 0 {
		b, err := json.Marshal(params.Metadata)
		if err != nil {
			return fmt.Errorf("marshaling audit metadata: %w", err)
		}
		metadataJSON = b
	}

	var targetType *string
	if params.TargetType != "" {
		targetType = &params.TargetType
	}

	var ipAddress *netip.Addr
	if params.IPAddress != nil {
		addr, err := netip.ParseAddr(*params.IPAddress)
		if err == nil {
			ipAddress = &addr
		}
	}

	_, err := store.createAuditLog(ctx, gen.CreateAuditLogParams{
		TenantID:           tenantID,
		ActorUserID:        params.ActorUserID,
		ImpersonatedUserID: params.ImpersonatedUserID,
		ActionType:         params.ActionType,
		TargetType:         targetType,
		TargetID:           params.TargetID,
		Metadata:           metadataJSON,
		IpAddress:          ipAddress,
	})
	return err
}
