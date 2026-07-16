// Package spam provides spam prevention and content safety for JapaConnect.
//
// This package implements four subsystems that work together to protect
// the platform from abuse:
//
//   - IP blocking: block malicious IPs by CIDR range, with per-block-type
//     granularity (full, registration, posting, api) and optional expiry.
//   - User blocking: let users block other users so their content is
//     excluded from the blocker's feed and search results.
//   - Censored words: admin-configurable word list that can block, replace,
//     or flag content containing matched patterns.
//   - Content deduplication: SHA-256 hash of normalized content stored in
//     the contents table to detect near-duplicate spam.
//
// The censored words list is cached in Valkey (jc:spam:censored:{tenant_id})
// with a 1-hour TTL. Cache misses fall back to a database read and repopulate
// the cache. The IP block list is loaded into a Chi middleware that runs on
// every request, blocking matched IPs before they reach any handler.
package spam

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides database operations for the spam prevention subsystem.
// All queries use the sqlc-generated Queries against the Writer pool for
// writes and Reader pool for reads, following the existing read/write split
// convention.
type Store struct {
	queries *gen.Queries
}

// NewStore creates a Store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Writer),
	}
}

// NewStoreWithReader creates a Store using the Reader pool for read-only
// operations. Use this for the middleware read path to avoid loading the
// primary with cache-miss queries.
func NewStoreWithReader(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
	}
}

// --- IP Blocks ---

// CreateIPBlockInput holds the parameters for creating an IP block.
type CreateIPBlockInput struct {
	TenantID  pgtype.UUID
	CIDR      string
	BlockType string
	Reason    *string
	BlockedBy pgtype.UUID
	ExpiresAt pgtype.Timestamptz
}

// CreateIPBlock inserts a new IP block record.
func (s *Store) CreateIPBlock(ctx context.Context, input CreateIPBlockInput) (gen.IpBlock, error) {
	addr, err := netipParseAddrOrCIDR(input.CIDR)
	if err != nil {
		return gen.IpBlock{}, fmt.Errorf("parsing CIDR: %w", err)
	}

	return s.queries.CreateIPBlock(ctx, gen.CreateIPBlockParams{
		TenantID:  input.TenantID,
		Cidr:      addr,
		BlockType: input.BlockType,
		Reason:    input.Reason,
		BlockedBy: input.BlockedBy,
		ExpiresAt: input.ExpiresAt,
	})
}

// GetIPBlockByCIDR checks if a CIDR is blocked for the given tenant.
func (s *Store) GetIPBlockByCIDR(ctx context.Context, tenantID pgtype.UUID, cidr string) (gen.IpBlock, error) {
	addr, err := netipParseAddrOrCIDR(cidr)
	if err != nil {
		return gen.IpBlock{}, fmt.Errorf("parsing CIDR: %w", err)
	}

	return s.queries.GetIPBlockByCIDR(ctx, gen.GetIPBlockByCIDRParams{
		TenantID: tenantID,
		Cidr:     addr,
	})
}

// ListIPBlocks returns paginated IP blocks for the given tenant.
func (s *Store) ListIPBlocks(ctx context.Context, tenantID pgtype.UUID, limit int32, cursorCreatedAt pgtype.Timestamptz) ([]gen.IpBlock, error) {
	return s.queries.ListIPBlocks(ctx, gen.ListIPBlocksParams{
		TenantID:  tenantID,
		Limit:     limit,
		CreatedAt: cursorCreatedAt,
	})
}

// DeleteIPBlock removes an IP block by ID.
func (s *Store) DeleteIPBlock(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.queries.DeleteIPBlock(ctx, gen.DeleteIPBlockParams{
		ID:       id,
		TenantID: tenantID,
	})
}

// UpdateIPBlockInput holds the parameters for updating an IP block.
type UpdateIPBlockInput struct {
	CIDR      *string
	BlockType *string
	Reason    *string
	ExpiresAt *pgtype.Timestamptz
	BlockedBy pgtype.UUID
}

// UpdateIPBlock updates an existing IP block by ID.
func (s *Store) UpdateIPBlock(ctx context.Context, id, tenantID pgtype.UUID, input UpdateIPBlockInput) (gen.IpBlock, error) {
	current, err := s.queries.GetIPBlockByID(ctx, gen.GetIPBlockByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.IpBlock{}, fmt.Errorf("getting ip block for update: %w", err)
	}

	cidr := current.Cidr

	if input.CIDR != nil {
		addr, parseErr := netipParseAddrOrCIDR(*input.CIDR)
		if parseErr != nil {
			return gen.IpBlock{}, fmt.Errorf("parsing CIDR: %w", parseErr)
		}

		cidr = addr
	}

	blockType := current.BlockType
	if input.BlockType != nil {
		blockType = *input.BlockType
	}

	reason := current.Reason
	if input.Reason != nil {
		reason = input.Reason
	}

	expiresAt := current.ExpiresAt
	if input.ExpiresAt != nil {
		expiresAt = *input.ExpiresAt
	}

	return s.queries.UpdateIPBlock(ctx, gen.UpdateIPBlockParams{
		ID:        id,
		TenantID:  tenantID,
		Cidr:      cidr,
		BlockType: blockType,
		Reason:    reason,
		ExpiresAt: expiresAt,
		BlockedBy: input.BlockedBy,
	})
}

// GetActiveIPBlocks returns all non-expired IP blocks for the tenant.
func (s *Store) GetActiveIPBlocks(ctx context.Context, tenantID pgtype.UUID) ([]gen.IpBlock, error) {
	return s.queries.GetActiveIPBlocks(ctx, tenantID)
}

// IsIPBlocked checks via SQL whether the given IP falls within any
// active CIDR block for the tenant. This uses PostgreSQL's <<= operator
// which correctly handles inet containment.
func (s *Store) IsIPBlocked(ctx context.Context, tenantID pgtype.UUID, ip string) (bool, error) {
	parsedIP, err := netip.ParseAddr(ip)
	if err != nil {
		return false, nil //nolint:nilerr // intentional: invalid IPs are not blocked
	}

	result, err := s.queries.IsIPBlocked(ctx, gen.IsIPBlockedParams{
		TenantID: tenantID,
		Ip:       parsedIP,
	})
	if err != nil {
		return false, fmt.Errorf("checking IP block: %w", err)
	}

	return result, nil
}

// --- User Blocks ---

// CreateUserBlock creates a block relationship.
func (s *Store) CreateUserBlock(ctx context.Context, blockerID, blockedID pgtype.UUID) error {
	return s.queries.CreateUserBlock(ctx, gen.CreateUserBlockParams{
		BlockerID: blockerID,
		BlockedID: blockedID,
	})
}

// DeleteUserBlock removes a block relationship.
func (s *Store) DeleteUserBlock(ctx context.Context, blockerID, blockedID pgtype.UUID) error {
	return s.queries.DeleteUserBlock(ctx, gen.DeleteUserBlockParams{
		BlockerID: blockerID,
		BlockedID: blockedID,
	})
}

// GetUserBlocks returns all users blocked by the given user.
func (s *Store) GetUserBlocks(ctx context.Context, blockerID pgtype.UUID) ([]gen.UserBlock, error) {
	return s.queries.GetUserBlocks(ctx, blockerID)
}

// ListAllUserBlocks returns paginated user blocks across all users.
func (s *Store) ListAllUserBlocks(ctx context.Context, limit int32) ([]gen.UserBlock, error) {
	return s.queries.ListAllUserBlocks(ctx, limit)
}

// IsBlocked checks whether user A has blocked user B or vice versa.
func (s *Store) IsBlocked(ctx context.Context, userA, userB pgtype.UUID) (bool, error) {
	result, err := s.queries.IsBlocked(ctx, gen.IsBlockedParams{
		BlockerID: userA,
		BlockedID: userB,
	})
	if err != nil {
		return false, fmt.Errorf("checking block status: %w", err)
	}

	return result, nil
}

// --- Censored Words ---

// CreateCensoredWordInput holds parameters for creating a censored word.
type CreateCensoredWordInput struct {
	TenantID    pgtype.UUID
	Pattern     string
	Action      string
	Replacement *string
}

// CreateCensoredWord inserts a new censored word pattern.
func (s *Store) CreateCensoredWord(ctx context.Context, input CreateCensoredWordInput) (gen.CensoredWord, error) {
	return s.queries.CreateCensoredWord(ctx, gen.CreateCensoredWordParams{
		TenantID:    input.TenantID,
		Pattern:     input.Pattern,
		Action:      input.Action,
		Replacement: input.Replacement,
	})
}

// ListCensoredWords returns all censored words for the given tenant.
func (s *Store) ListCensoredWords(ctx context.Context, tenantID pgtype.UUID) ([]gen.CensoredWord, error) {
	return s.queries.ListCensoredWords(ctx, tenantID)
}

// DeleteCensoredWord removes a censored word by ID.
func (s *Store) DeleteCensoredWord(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.queries.DeleteCensoredWord(ctx, gen.DeleteCensoredWordParams{
		ID:       id,
		TenantID: tenantID,
	})
}

// UpdateCensoredWordInput holds the parameters for updating a censored word.
type UpdateCensoredWordInput struct {
	Pattern     *string
	Action      *string
	Replacement *string
}

// UpdateCensoredWord updates an existing censored word by ID.
func (s *Store) UpdateCensoredWord(ctx context.Context, id, tenantID pgtype.UUID, input UpdateCensoredWordInput) (gen.CensoredWord, error) {
	current, err := s.queries.GetCensoredWordByID(ctx, gen.GetCensoredWordByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.CensoredWord{}, fmt.Errorf("getting censored word for update: %w", err)
	}

	pattern := current.Pattern
	if input.Pattern != nil {
		pattern = *input.Pattern
	}

	action := current.Action
	if input.Action != nil {
		action = *input.Action
	}

	replacement := current.Replacement
	if input.Replacement != nil {
		replacement = input.Replacement
	}

	return s.queries.UpdateCensoredWord(ctx, gen.UpdateCensoredWordParams{
		ID:          id,
		TenantID:    tenantID,
		Pattern:     pattern,
		Action:      action,
		Replacement: replacement,
	})
}

// GetCensoredWordsByTenant returns all censored words for cache population.
func (s *Store) GetCensoredWordsByTenant(ctx context.Context, tenantID pgtype.UUID) ([]gen.CensoredWord, error) {
	return s.queries.GetCensoredWordsByTenant(ctx, tenantID)
}

// --- Content Hash ---

// UpdateContentHash sets the content hash on an existing content item.
func (s *Store) UpdateContentHash(ctx context.Context, contentID, tenantID pgtype.UUID, hash string) error {
	return s.queries.UpdateContentHash(ctx, gen.UpdateContentHashParams{
		ID:          contentID,
		TenantID:    tenantID,
		ContentHash: &hash,
	})
}

// CheckDuplicateHash checks whether a content hash already exists for the tenant.
func (s *Store) CheckDuplicateHash(ctx context.Context, tenantID pgtype.UUID, hash string) (bool, error) {
	result, err := s.queries.CheckDuplicateHash(ctx, gen.CheckDuplicateHashParams{
		TenantID:    tenantID,
		ContentHash: &hash,
	})
	if err != nil {
		return false, fmt.Errorf("checking duplicate hash: %w", err)
	}

	return result, nil
}

// --- helpers ---

// netipParseAddrOrCIDR parses a CIDR or IP string into a netip.Addr.
// For CIDR notation like "192.168.0.0/24", it extracts just the address.
// Returns an error for invalid input instead of panicking.
func netipParseAddrOrCIDR(s string) (netip.Addr, error) {
	// Try parsing as a plain IP first
	addr, err := netip.ParseAddr(s)
	if err == nil {
		return addr, nil
	}

	// Try parsing as CIDR and extract the address
	cidr, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid CIDR or IP: %s", s)
	}

	return cidr.Addr(), nil
}
