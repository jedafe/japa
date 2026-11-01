package verification

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Managed badge-type taxonomy (§07 amendment, migration 001013). The
// verification_types table is the runtime source of truth for all three
// scopes — organization, user, service_listing — with per-scope custom badge
// artwork (badge_media_id → media, resolved to badge_url via the injected
// MediaResolver). Validation call sites read the active set per scope and
// fall back to the compiled §07/§98 vocabularies when the table is
// unavailable (same fallback contract as servicedir's categorySet).

var (
	// ErrInvalidTypeScope — scope outside the CHECK'd set.
	ErrInvalidTypeScope = errors.New("invalid verification type scope")
	// ErrTypeExists — (scope, value) unique-violation on create.
	ErrTypeExists = errors.New("verification type already exists")
	// ErrTypeProtected — delete attempted on a seeded vocabulary row.
	ErrTypeProtected = errors.New("verification type is protected")
	// ErrTypeNotFound — id did not match a row.
	ErrTypeNotFound = errors.New("verification type not found")
)

// TypeInputError is a field-validation failure on type create/update.
type TypeInputError struct{ Msg string }

func (e *TypeInputError) Error() string { return e.Msg }

// typeValueRe constrains taxonomy values to lowercase snake_case — the same
// shape as every seeded value and the servicedir category slugs.
var typeValueRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,48}$`)

// ValidScopes mirrors the CHECK constraint on verification_types.scope.
var ValidScopes = map[string]struct{}{
	ScopeOrganization:   {},
	ScopeUser:           {},
	ScopeServiceListing: {},
}

// TypeInput carries the mutable fields on create/update.
type TypeInput struct {
	Scope        string
	Value        string
	Label        string
	Description  *string
	BadgeMediaID *string
	Position     *int32
	IsActive     *bool
}

// TypeDTO is the admin/public representation — includes the resolved badge
// artwork URL alongside the media id.
type TypeDTO struct {
	ID          string      `json:"id"`
	Scope       string      `json:"scope"`
	Value       string      `json:"value"`
	Label       string      `json:"label"`
	Description string      `json:"description,omitempty"`
	BadgeMediaID *string    `json:"badge_media_id,omitempty"`
	BadgeURL    string      `json:"badge_url,omitempty"`
	Position    int32       `json:"position"`
	IsActive    bool        `json:"is_active"`
	IsProtected bool        `json:"is_protected"`
	CreatedAt   interface{} `json:"created_at"`
	UpdatedAt   interface{} `json:"updated_at"`
}

// MediaResolver resolves a media id to a displayable URL — the same
// interface shape as users.MediaResolver (mediaURLResolver in server.go).
type MediaResolver interface {
	ResolveMediaURL(ctx context.Context, mediaID pgtype.UUID) (string, error)
}

// SetMediaResolver injects the media URL resolver for badge artwork.
func (s *Service) SetMediaResolver(r MediaResolver) { s.mediaResolver = r }

func typeInputErr(format string, args ...any) *TypeInputError {
	return &TypeInputError{Msg: fmt.Sprintf(format, args...)}
}

func validateTypeInput(in TypeInput, creating bool) error {
	if creating {
		if _, ok := ValidScopes[in.Scope]; !ok {
			return typeInputErr("scope must be one of organization|user|service_listing")
		}
		if !typeValueRe.MatchString(in.Value) {
			return typeInputErr("value must match ^[a-z][a-z0-9_]{1,48}$")
		}
	}
	if strings.TrimSpace(in.Label) == "" {
		return typeInputErr("label is required")
	}
	if len(in.Label) > 80 {
		return typeInputErr("label must be 80 characters or fewer")
	}
	return nil
}

// typeSet resolves the active value set for a scope: the verification_types
// table first, the compiled vocabulary when the store is unwired or the read
// fails (mirrors servicedir.Service.categorySet).
func (s *Service) typeSet(ctx context.Context, scope string) map[string]struct{} {
	if s.store != nil {
		if values, err := s.store.ListActiveTypeValues(ctx, scope); err == nil && len(values) > 0 {
			set := make(map[string]struct{}, len(values))
			for _, v := range values {
				set[v] = struct{}{}
			}
			return set
		}
	}
	set := make(map[string]struct{}, len(FallbackTypeValues(scope)))
	for _, v := range FallbackTypeValues(scope) {
		set[v] = struct{}{}
	}
	return set
}

// IsValidTypeValue reports whether value is an active type in the given
// scope. Exported so sibling services (users, admin, servicedir) can share
// the dynamic check.
func (s *Service) IsValidTypeValue(ctx context.Context, scope, value string) bool {
	_, ok := s.typeSet(ctx, scope)[value]
	return ok
}

// ListTypes returns active types for a scope ('' = all scopes) with badge
// artwork resolved — backs the public /verification-requests/types route.
func (s *Service) ListTypes(ctx context.Context, scope string) ([]TypeDTO, error) {
	rows, err := s.store.ListTypes(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("listing verification types: %w", err)
	}
	out := make([]TypeDTO, 0, len(rows))
	for _, r := range rows {
		if !r.IsActive {
			continue
		}
		out = append(out, s.typeDTO(ctx, r))
	}
	return out, nil
}

// AdminListTypes returns every row — active and inactive — for the taxonomy
// manager.
func (s *Service) AdminListTypes(ctx context.Context, scope string) ([]TypeDTO, error) {
	rows, err := s.store.ListTypes(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("listing verification types: %w", err)
	}
	out := make([]TypeDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.typeDTO(ctx, r))
	}
	return out, nil
}

func (s *Service) typeDTO(ctx context.Context, r gen.VerificationType) TypeDTO {
	d := TypeDTO{
		ID:          uuidStr(r.ID),
		Scope:       r.Scope,
		Value:       r.Value,
		Label:       r.Label,
		Position:    r.Position,
		IsActive:    r.IsActive,
		IsProtected: r.IsProtected,
		CreatedAt:   r.CreatedAt.Time,
		UpdatedAt:   r.UpdatedAt.Time,
	}
	if r.Description != nil {
		d.Description = *r.Description
	}
	if r.BadgeMediaID.Valid {
		id := uuidStr(r.BadgeMediaID)
		d.BadgeMediaID = &id
		if s.mediaResolver != nil {
			if u, err := s.mediaResolver.ResolveMediaURL(ctx, r.BadgeMediaID); err == nil {
				d.BadgeURL = u
			}
		}
	}
	return d
}

func uuidStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}

func scanBadgeMediaID(raw *string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if raw == nil || *raw == "" {
		return id, nil
	}
	if err := id.Scan(*raw); err != nil {
		return id, fmt.Errorf("invalid badge_media_id")
	}
	return id, nil
}

// AdminCreateType inserts a taxonomy row. A (scope, value) unique-violation
// maps to ErrTypeExists (409).
func (s *Service) AdminCreateType(ctx context.Context, in TypeInput) (TypeDTO, error) {
	if err := validateTypeInput(in, true); err != nil {
		return TypeDTO{}, err
	}
	mediaID, err := scanBadgeMediaID(in.BadgeMediaID)
	if err != nil {
		return TypeDTO{}, typeInputErr("invalid badge_media_id")
	}
	// COALESCE'd narg columns take interface{} — a typed nil pointer would
	// still encode as NULL, but explicit nils keep the default obvious.
	var position, isActive any
	if in.Position != nil {
		position = *in.Position
	}
	if in.IsActive != nil {
		isActive = *in.IsActive
	}
	row, err := s.store.CreateType(ctx, gen.CreateVerificationTypeParams{
		Scope:        in.Scope,
		Value:        in.Value,
		Label:        strings.TrimSpace(in.Label),
		Description:  in.Description,
		BadgeMediaID: mediaID,
		Position:     position,
		IsActive:     isActive,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return TypeDTO{}, ErrTypeExists
		}
		return TypeDTO{}, fmt.Errorf("creating verification type: %w", err)
	}
	return s.typeDTO(ctx, row), nil
}

// AdminUpdateType patches label/description/badge/position/is_active.
// scope+value are immutable — entity rows store the value verbatim.
func (s *Service) AdminUpdateType(ctx context.Context, id pgtype.UUID, in TypeInput) (TypeDTO, error) {
	if err := validateTypeInput(in, false); err != nil {
		return TypeDTO{}, err
	}
	mediaID, err := scanBadgeMediaID(in.BadgeMediaID)
	if err != nil {
		return TypeDTO{}, typeInputErr("invalid badge_media_id")
	}
	row, err := s.store.UpdateType(ctx, gen.UpdateVerificationTypeParams{
		ID:           id,
		Label:        strings.TrimSpace(in.Label),
		Description:  in.Description,
		BadgeMediaID: mediaID,
		Position:     in.Position,
		IsActive:     in.IsActive,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TypeDTO{}, ErrTypeNotFound
		}
		return TypeDTO{}, fmt.Errorf("updating verification type: %w", err)
	}
	return s.typeDTO(ctx, row), nil
}

// AdminDeleteType removes a non-protected taxonomy row. Stored
// verification_type values on entities are historical facts — they survive
// the delete like removed service_categories values do.
func (s *Service) AdminDeleteType(ctx context.Context, id pgtype.UUID) error {
	row, err := s.store.GetTypeByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTypeNotFound
		}
		return fmt.Errorf("fetching verification type: %w", err)
	}
	if row.IsProtected {
		return ErrTypeProtected
	}
	affected, err := s.store.DeleteType(ctx, id)
	if err != nil {
		return fmt.Errorf("deleting verification type: %w", err)
	}
	if affected == 0 {
		return ErrTypeNotFound
	}
	return nil
}


